package main

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iyuuya/go/exitcode"

	"github.com/iyuuya/tsk/config"
	"github.com/iyuuya/tsk/state"
	"github.com/iyuuya/tsk/task"
)

// isolateEnv points config and state at fresh directories so tests never
// touch the user's real files.
func isolateEnv(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
}

// fakeAdaptorConfig is a user config declaring a single adaptor that needs
// nothing but sh: it lists tasks from fake.json's "scripts" map and its run
// command writes out.txt as proof of execution.
const fakeAdaptorConfig = `
[[adaptor]]
kind = "fake"
definition_files = ["fake.json"]
run = ["sh", "-c", "printf ran-{{name}} > out.txt"]

[adaptor.list]
kind = "json_file_map"
file = "fake.json"
scripts_field = "scripts"
`

func writeFakeConfig(t *testing.T) {
	t.Helper()
	path, err := config.Path()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(fakeAdaptorConfig), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// setupProject creates a git-rooted project with a fake.json task file and
// chdirs into it, returning the root as git reports it (symlinks resolved) —
// the exact value ProjectRoot-based commands will use.
func setupProject(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	isolateEnv(t)
	writeFakeConfig(t)

	dir := t.TempDir()
	if out, err := exec.Command("git", "init", dir).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	writeFile(t, filepath.Join(dir, "fake.json"), `{"scripts": {"hello": "greets"}}`)
	t.Chdir(dir)

	root, err := task.ProjectRoot(".")
	if err != nil {
		t.Fatal(err)
	}
	return root
}

// captureStdout runs fn with os.Stdout redirected to a pipe and returns what
// it printed alongside fn's error.
func captureStdout(t *testing.T, fn func() error) (string, error) {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	fnErr := fn()
	os.Stdout = old
	w.Close()
	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(data), fnErr
}

// runMain invokes realMain as if tsk had been started with args, capturing
// stdout (so listings don't pollute test output) and stderr (where usage and
// errors go).
func runMain(t *testing.T, args ...string) (code exitcode.ExitCode, stdout, stderr string) {
	t.Helper()
	oldArgs, oldStdout, oldStderr := os.Args, os.Stdout, os.Stderr
	defer func() { os.Args, os.Stdout, os.Stderr = oldArgs, oldStdout, oldStderr }()

	outR, outW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	errR, errW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Args = append([]string{"tsk"}, args...)
	os.Stdout, os.Stderr = outW, errW

	code = realMain()

	os.Stdout, os.Stderr = oldStdout, oldStderr
	outW.Close()
	errW.Close()
	outData, err := io.ReadAll(outR)
	if err != nil {
		t.Fatal(err)
	}
	errData, err := io.ReadAll(errR)
	if err != nil {
		t.Fatal(err)
	}
	return code, string(outData), string(errData)
}

func TestRealMainNoArgs(t *testing.T) {
	root := setupProject(t)

	oldSelectTask := selectTask
	t.Cleanup(func() { selectTask = oldSelectTask })
	selectTask = func(rows []string) (int, error) {
		if len(rows) != 1 {
			t.Errorf("picker rows = %q, want one task", rows)
		}
		return 0, nil
	}

	code, _, stderr := runMain(t)
	if code != exitcode.ExitOK {
		t.Fatalf("realMain() = %v, stderr: %s", code, stderr)
	}
	data, err := os.ReadFile(filepath.Join(root, "out.txt"))
	if err != nil {
		t.Fatalf("tsk did not execute the selected task: %v", err)
	}
	if string(data) != "ran-hello" {
		t.Errorf("tsk wrote %q, want %q", data, "ran-hello")
	}
}

func TestRealMainUnknownCommand(t *testing.T) {
	code, _, stderr := runMain(t, "frobnicate")
	if code == exitcode.ExitOK {
		t.Error("realMain with an unknown command: expected a non-OK exit code")
	}
	if !strings.Contains(stderr, "usage:") {
		t.Errorf("stderr = %q, want usage text", stderr)
	}
}

func TestRealMainCommandError(t *testing.T) {
	isolateEnv(t)
	code, _, stderr := runMain(t, "list", "--scope=global", "--refresh")
	if code == exitcode.ExitOK {
		t.Error("realMain with a failing command: expected a non-OK exit code")
	}
	if !strings.Contains(stderr, "--refresh") {
		t.Errorf("stderr = %q, want the command's error printed", stderr)
	}
}

func TestRealMainList(t *testing.T) {
	root := setupProject(t)

	code, stdout, stderr := runMain(t, "list")
	if code != exitcode.ExitOK {
		t.Fatalf("realMain(list) = %v, stderr: %s", code, stderr)
	}
	want := filepath.Base(root) + "\t.\tfake\thello\tgreets\n"
	if stdout != want {
		t.Errorf("stdout = %q, want %q", stdout, want)
	}
}

func TestRealMainRun(t *testing.T) {
	root := setupProject(t)

	code, _, stderr := runMain(t, "run", "fake", "hello")
	if code != exitcode.ExitOK {
		t.Fatalf("realMain(run) = %v, stderr: %s", code, stderr)
	}
	data, err := os.ReadFile(filepath.Join(root, "out.txt"))
	if err != nil {
		t.Fatalf("run command did not execute: %v", err)
	}
	if string(data) != "ran-hello" {
		t.Errorf("run wrote %q, want %q", data, "ran-hello")
	}
}

func TestRealMainConfigInit(t *testing.T) {
	isolateEnv(t)

	code, stdout, stderr := runMain(t, "config", "init")
	if code != exitcode.ExitOK {
		t.Fatalf("realMain(config init) = %v, stderr: %s", code, stderr)
	}
	path, err := config.Path()
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(stdout) != path {
		t.Errorf("stdout = %q, want the config path %q", strings.TrimSpace(stdout), path)
	}
	exists, err := config.Exists()
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Error("config init did not create the config file")
	}
}

func TestDiscoverers(t *testing.T) {
	isolateEnv(t)

	ds, err := discoverers()
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Default()
	if err != nil {
		t.Fatal(err)
	}
	defs := cfg.Adaptor
	if len(ds) != len(defs) {
		t.Fatalf("got %d discoverers, want %d", len(ds), len(defs))
	}
	for i, want := range defs {
		if got := ds[i].New(".").TaskDefinition(); got != want.Kind {
			t.Errorf("discoverers()[%d] builds kind %q, want %q (priority order)", i, got, want.Kind)
		}
	}
}

func TestProjectLabel(t *testing.T) {
	if got := projectLabel("/home/u/src/github.com/iyuuya/tsk"); got != "iyuuya/tsk" {
		t.Errorf("projectLabel() = %q, want %q", got, "iyuuya/tsk")
	}
}

func TestResolveDir(t *testing.T) {
	if got := resolveDir("/root", "sub/dir"); got != filepath.Join("/root", "sub", "dir") {
		t.Errorf("resolveDir() relative = %q", got)
	}
	if got := resolveDir("/root", "/abs/dir"); got != "/abs/dir" {
		t.Errorf("resolveDir() absolute = %q, want it used as-is", got)
	}
}

func TestListTasks(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "sub")
	tasks := []task.Task{
		{Name: "build", Description: "compile", Adaptor: stubAdaptor{kind: "npm", dir: sub}},
	}
	rows := listTasks(root, tasks)
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}
	want := listTask{Root: root, Dir: "sub", Adaptor: "npm", Task: "build", Description: "compile"}
	if rows[0] != want {
		t.Errorf("listTasks() = %+v, want %+v", rows[0], want)
	}
}

// stubAdaptor is the minimal task.Adaptor needed by listTasks/printTasks.
type stubAdaptor struct {
	kind, dir string
}

func (a stubAdaptor) TaskDefinition() string     { return a.kind }
func (a stubAdaptor) Dir() string                { return a.dir }
func (a stubAdaptor) List() ([]task.Task, error) { return nil, nil }
func (a stubAdaptor) Run(string) error           { return nil }

func TestPrintTasks(t *testing.T) {
	root := t.TempDir()
	out, err := captureStdout(t, func() error {
		printTasks(root, []task.Task{
			{Name: "hello", Description: "greets", Adaptor: stubAdaptor{kind: "fake", dir: root}},
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Base(root) + "\t.\tfake\thello\tgreets\n"
	if out != want {
		t.Errorf("printTasks() output = %q, want %q", out, want)
	}
}

func TestCmdList(t *testing.T) {
	root := setupProject(t)

	out, err := captureStdout(t, func() error { return cmdList(nil) })
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Base(root) + "\t.\tfake\thello\tgreets\n"
	if out != want {
		t.Errorf("cmdList() output = %q, want %q", out, want)
	}
}

func TestCmdListJSON(t *testing.T) {
	root := setupProject(t)

	out, err := captureStdout(t, func() error { return cmdList([]string{"--json"}) })
	if err != nil {
		t.Fatal(err)
	}
	var rows []listTask
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatalf("--json output is not valid JSON: %v\n%s", err, out)
	}
	want := listTask{Root: root, Dir: ".", Adaptor: "fake", Task: "hello", Description: "greets"}
	if len(rows) != 1 || rows[0] != want {
		t.Errorf("cmdList --json = %+v, want [%+v]", rows, want)
	}
}

func TestCmdListGlobalRefreshRejected(t *testing.T) {
	isolateEnv(t)
	if err := cmdList([]string{"--scope=global", "--refresh"}); err == nil {
		t.Error("cmdList --scope=global --refresh: expected an error, got nil")
	}
}

func TestCmdListGlobal(t *testing.T) {
	isolateEnv(t)

	if err := state.Save(&state.State{
		Root: "/src/github.com/iyuuya/proj",
		Adaptors: []state.CachedAdaptor{{
			Kind: "npm", Dir: ".",
			Tasks: []state.CachedTask{{Name: "build", Description: "compile"}},
		}},
	}); err != nil {
		t.Fatal(err)
	}

	out, err := captureStdout(t, func() error { return cmdList([]string{"--scope=global"}) })
	if err != nil {
		t.Fatal(err)
	}
	if want := "iyuuya/proj\t.\tnpm\tbuild\tcompile\n"; out != want {
		t.Errorf("cmdList --scope=global output = %q, want %q", out, want)
	}

	out, err = captureStdout(t, func() error { return cmdList([]string{"--scope=global", "--json"}) })
	if err != nil {
		t.Fatal(err)
	}
	var rows []listTask
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatalf("--scope=global --json output is not valid JSON: %v\n%s", err, out)
	}
	want := listTask{Root: "/src/github.com/iyuuya/proj", Dir: ".", Adaptor: "npm", Task: "build", Description: "compile"}
	if len(rows) != 1 || rows[0] != want {
		t.Errorf("cmdList --scope=global --json = %+v, want [%+v]", rows, want)
	}
}

func TestCmdRun(t *testing.T) {
	root := setupProject(t)

	if err := cmdRun([]string{"fake", "hello"}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, "out.txt"))
	if err != nil {
		t.Fatalf("run command did not execute: %v", err)
	}
	if string(data) != "ran-hello" {
		t.Errorf("run wrote %q, want %q", data, "ran-hello")
	}
}

func TestCmdRunPicker(t *testing.T) {
	root := setupProject(t)

	oldSelectTask := selectTask
	t.Cleanup(func() { selectTask = oldSelectTask })
	var rows []string
	selectTask = func(input []string) (int, error) {
		rows = input
		return 0, nil
	}

	if err := cmdRun(nil); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || !strings.Contains(rows[0], "\tfake\thello\tgreets") {
		t.Errorf("picker rows = %q, want the discovered task", rows)
	}
	data, err := os.ReadFile(filepath.Join(root, "out.txt"))
	if err != nil {
		t.Fatalf("picker did not execute the selected task: %v", err)
	}
	if string(data) != "ran-hello" {
		t.Errorf("picker run wrote %q, want %q", data, "ran-hello")
	}
}

func TestCmdRunScopeDirPickerHidesDir(t *testing.T) {
	root := setupProject(t)

	oldSelectTask := selectTask
	t.Cleanup(func() { selectTask = oldSelectTask })
	selectTask = func(rows []string) (int, error) {
		if len(rows) != 1 || !strings.HasPrefix(rows[0], "0\t\tfake\thello") {
			t.Errorf("dir-scope picker rows = %q, want no directory field", rows)
		}
		return -1, nil
	}

	if err := cmdRun([]string{"--scope=dir"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "out.txt")); !os.IsNotExist(err) {
		t.Errorf("canceled picker executed a task: %v", err)
	}
}

func TestCmdRunGlobalPicker(t *testing.T) {
	isolateEnv(t)
	writeFakeConfig(t)
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "fake.json"), `{"scripts": {"hello": ""}}`)
	if err := state.Save(&state.State{
		Root: root,
		Adaptors: []state.CachedAdaptor{{
			Kind: "fake", Dir: ".",
			Tasks: []state.CachedTask{{Name: "hello"}},
		}},
	}); err != nil {
		t.Fatal(err)
	}

	oldSelectTask := selectTask
	t.Cleanup(func() { selectTask = oldSelectTask })
	selectTask = func(rows []string) (int, error) {
		if len(rows) != 1 || !strings.Contains(rows[0], "\tfake\thello") {
			t.Errorf("global picker rows = %q, want the cached task", rows)
		}
		return 0, nil
	}

	if err := cmdRun([]string{"--scope=global"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "out.txt")); err != nil {
		t.Errorf("global picker did not execute the selected task: %v", err)
	}
}

func TestCmdRunWithDir(t *testing.T) {
	root := setupProject(t)
	writeFile(t, filepath.Join(root, "sub", "fake.json"), `{"scripts": {"deep": ""}}`)

	if err := cmdRun([]string{"sub", "fake", "deep"}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, "sub", "out.txt"))
	if err != nil {
		t.Fatalf("run command did not execute in sub/: %v", err)
	}
	if string(data) != "ran-deep" {
		t.Errorf("run wrote %q, want %q", data, "ran-deep")
	}
}

func TestCmdRunGlobal(t *testing.T) {
	// Global scope takes the root as an argument, so no git repo or chdir
	// is involved.
	isolateEnv(t)
	writeFakeConfig(t)
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "fake.json"), `{"scripts": {"hello": ""}}`)

	if err := cmdRun([]string{"--scope=global", root, "fake", "hello"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "out.txt")); err != nil {
		t.Errorf("run command did not execute in the given root: %v", err)
	}
}

func TestCmdRunTaskNotFound(t *testing.T) {
	setupProject(t)

	err := cmdRun([]string{"fake", "nope"})
	if err == nil {
		t.Fatal("cmdRun for an unknown task: expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "task not found") {
		t.Errorf("error = %v, want it to say the task was not found", err)
	}
}

func TestCmdConfigInit(t *testing.T) {
	isolateEnv(t)

	path, err := config.Path()
	if err != nil {
		t.Fatal(err)
	}

	out, err := captureStdout(t, func() error { return cmdConfigInit(nil) })
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out) != path {
		t.Errorf("config init printed %q, want the config path %q", strings.TrimSpace(out), path)
	}
	exists, err := config.Exists()
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Fatal("config init did not create the config file")
	}

	// A second init must refuse to clobber the file...
	if _, err := captureStdout(t, func() error { return cmdConfigInit(nil) }); err == nil {
		t.Error("config init over an existing file: expected an error, got nil")
	}
	// ...unless forced.
	if _, err := captureStdout(t, func() error { return cmdConfigInit([]string{"--force"}) }); err != nil {
		t.Errorf("config init --force: %v", err)
	}
}

func TestCmdConfigUsageErrors(t *testing.T) {
	if err := cmdConfig(nil); !errors.Is(err, errUsage) {
		t.Errorf("cmdConfig with no subcommand = %v, want errUsage", err)
	}
	if err := cmdConfig([]string{"frobnicate"}); !errors.Is(err, errUsage) {
		t.Errorf("cmdConfig with an unknown subcommand = %v, want errUsage", err)
	}
}

func TestCmdRunUsageErrors(t *testing.T) {
	isolateEnv(t)

	// Global scope without positional arguments opens the picker and fails
	// only because this isolated state directory has no cached tasks.
	if err := cmdRun([]string{"--scope=global"}); err == nil || errors.Is(err, errUsage) {
		t.Errorf("cmdRun --scope=global with no cache = %v, want a picker error", err)
	}
	// Too few and too many positional arguments. Global scope sidesteps
	// ProjectRoot so no git repo is needed.
	root := t.TempDir()
	if err := cmdRun([]string{"--scope=global", root, "onlyadaptor"}); !errors.Is(err, errUsage) {
		t.Errorf("cmdRun with 1 positional arg = %v, want errUsage", err)
	}
	if err := cmdRun([]string{"--scope=global", root, "dir", "adaptor", "task", "extra"}); !errors.Is(err, errUsage) {
		t.Errorf("cmdRun with 4 positional args = %v, want errUsage", err)
	}
}

func TestRealMainUsageErrorPrintsUsage(t *testing.T) {
	isolateEnv(t)

	code, _, stderr := runMain(t, "config")
	if code == exitcode.ExitOK {
		t.Error("realMain(config) with no subcommand: expected a non-OK exit code")
	}
	if !strings.Contains(stderr, "usage:") {
		t.Errorf("stderr = %q, want usage text for a usage error", stderr)
	}
	if strings.Contains(stderr, "invalid arguments") {
		t.Errorf("stderr = %q, want usage instead of the raw errUsage message", stderr)
	}
}

func TestFlagParseErrors(t *testing.T) {
	isolateEnv(t)

	if err := cmdList([]string{"--bogus"}); err == nil {
		t.Error("cmdList with an unknown flag: expected an error, got nil")
	}
	if err := cmdRun([]string{"--bogus"}); err == nil {
		t.Error("cmdRun with an unknown flag: expected an error, got nil")
	}
	if err := cmdConfigInit([]string{"--bogus"}); err == nil {
		t.Error("cmdConfigInit with an unknown flag: expected an error, got nil")
	}
}

func TestCmdListOutsideRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	isolateEnv(t)
	dir := t.TempDir()
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(dir))
	t.Chdir(dir)

	if err := cmdList(nil); err == nil {
		t.Skip("temp dir is inside a git work tree; cannot exercise the failure path")
	}
	if err := cmdRun([]string{"fake", "hello"}); err == nil {
		t.Error("cmdRun outside a repo: expected an error, got nil")
	}
}

func TestSyncTasksBrokenConfig(t *testing.T) {
	isolateEnv(t)
	path, err := config.Path()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("not [valid toml"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := syncTasks(t.TempDir(), false); err == nil {
		t.Error("syncTasks with a broken config: expected an error, got nil")
	}
}

func TestSyncTasksMissingRoot(t *testing.T) {
	isolateEnv(t)
	if _, err := syncTasks(filepath.Join(t.TempDir(), "absent"), false); err == nil {
		t.Error("syncTasks of a nonexistent root: expected an error, got nil")
	}
}

func TestCmdConfigInitUnresolvableConfigDir(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", "")

	if err := cmdConfigInit(nil); err == nil {
		t.Error("cmdConfigInit with no resolvable config dir: expected an error, got nil")
	}
}

func TestRelFallbacksKeepAbsolutePaths(t *testing.T) {
	// A relative root can't be related to an absolute task dir; the rows
	// and the printed listing must then carry the absolute dir.
	tasks := []task.Task{{Name: "x", Adaptor: stubAdaptor{kind: "npm", dir: "/abs/dir"}}}

	rows := listTasks("relative-root", tasks)
	if len(rows) != 1 || rows[0].Dir != "/abs/dir" {
		t.Errorf("listTasks() rows = %+v, want Dir kept absolute", rows)
	}

	out, err := captureStdout(t, func() error { printTasks("relative-root", tasks); return nil })
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "\t/abs/dir\t") {
		t.Errorf("printTasks() output = %q, want the absolute dir printed", out)
	}
}

func TestCmdListScopeDir(t *testing.T) {
	root := setupProject(t)
	writeFile(t, filepath.Join(root, "sub", "fake.json"), `{"scripts": {"deep": "digs"}}`)
	t.Chdir(filepath.Join(root, "sub"))

	out, err := captureStdout(t, func() error { return cmdList([]string{"--scope=dir"}) })
	if err != nil {
		t.Fatal(err)
	}
	// Only sub's task is listed; the repo root's is filtered out, and the
	// labels stay relative to the repo root.
	want := filepath.Base(root) + "\tsub\tfake\tdeep\tdigs\n"
	if out != want {
		t.Errorf("cmdList --scope=dir output = %q, want %q", out, want)
	}
}

func TestCmdListScopeDirOutsideRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	isolateEnv(t)
	writeFakeConfig(t)
	dir := t.TempDir()
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(dir))
	writeFile(t, filepath.Join(dir, "fake.json"), `{"scripts": {"hello": "greets"}}`)
	t.Chdir(dir)

	if _, err := task.ProjectRoot("."); err == nil {
		t.Skip("temp dir is inside a git work tree; cannot exercise the no-repo path")
	}

	out, err := captureStdout(t, func() error { return cmdList([]string{"--scope=dir"}) })
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "\t.\tfake\thello\tgreets\n") {
		t.Errorf("cmdList --scope=dir outside a repo = %q, want the local task listed", out)
	}
}

func TestCmdRunScopeDir(t *testing.T) {
	root := setupProject(t)
	writeFile(t, filepath.Join(root, "sub", "fake.json"), `{"scripts": {"deep": ""}}`)
	t.Chdir(filepath.Join(root, "sub"))

	if err := cmdRun([]string{"--scope=dir", "fake", "deep"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "sub", "out.txt")); err != nil {
		t.Errorf("run command did not execute in sub/: %v", err)
	}

	// The repo root's task lives outside the current directory, so a bare
	// invocation under dir scope must not reach it.
	err := cmdRun([]string{"--scope=dir", "fake", "hello"})
	if err == nil || !strings.Contains(err.Error(), "task not found") {
		t.Errorf("cmdRun --scope=dir for a repo-root task = %v, want task not found", err)
	}
}

func TestDefaultScopeFromConfig(t *testing.T) {
	isolateEnv(t)
	path, err := config.Path()
	if err != nil {
		t.Fatal(err)
	}
	// Top-level keys must precede the [[adaptor]] tables.
	if err := os.WriteFile(path, []byte("default_scope = \"global\"\n"+fakeAdaptorConfig), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := state.Save(&state.State{
		Root: "/src/github.com/iyuuya/proj",
		Adaptors: []state.CachedAdaptor{{
			Kind: "npm", Dir: ".",
			Tasks: []state.CachedTask{{Name: "build", Description: "compile"}},
		}},
	}); err != nil {
		t.Fatal(err)
	}

	// No --scope flag and no git repo involved: with default_scope = "global"
	// the listing must come from cached state alone.
	out, err := captureStdout(t, func() error { return cmdList(nil) })
	if err != nil {
		t.Fatal(err)
	}
	if want := "iyuuya/proj\t.\tnpm\tbuild\tcompile\n"; out != want {
		t.Errorf("cmdList with default_scope=global = %q, want %q", out, want)
	}
}

func TestInvalidScope(t *testing.T) {
	isolateEnv(t)

	if err := cmdList([]string{"--scope=bogus"}); err == nil || !strings.Contains(err.Error(), "invalid scope") {
		t.Errorf("cmdList --scope=bogus = %v, want an invalid-scope error", err)
	}

	// An invalid default_scope in the config file is reported too...
	path, err := config.Path()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("default_scope = \"sometimes\"\n"+fakeAdaptorConfig), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := cmdList(nil); err == nil || !strings.Contains(err.Error(), "default_scope") {
		t.Errorf("cmdList with a bad default_scope = %v, want a default_scope error", err)
	}
	// ...but an explicit --scope never consults it.
	if _, err := captureStdout(t, func() error { return cmdList([]string{"--scope=global"}) }); err != nil {
		t.Errorf("cmdList --scope=global with a bad default_scope = %v, want the config value ignored", err)
	}
}

func TestSyncTasksUsesCacheAcrossCalls(t *testing.T) {
	// End-to-end over discoverers + Discover + Sync: the second call must
	// serve tasks from cache (same mtime), and refresh must still work.
	root := setupProject(t)

	for _, refresh := range []bool{false, false, true} {
		tasks, err := syncTasks(root, refresh)
		if err != nil {
			t.Fatal(err)
		}
		if len(tasks) != 1 || tasks[0].Name != "hello" {
			t.Fatalf("syncTasks(refresh=%v) = %+v, want the single fake task", refresh, tasks)
		}
	}
}
