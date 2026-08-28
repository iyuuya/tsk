package spec

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/iyuuya/tsk/task"
)

func TestNewAccessors(t *testing.T) {
	dir := t.TempDir()
	a := New(Definition{Kind: "mise"}, dir)

	if got := a.TaskDefinition(); got != "mise" {
		t.Errorf("TaskDefinition() = %q, want %q", got, "mise")
	}
	if got := a.Dir(); got != dir {
		t.Errorf("Dir() = %q, want %q", got, dir)
	}
}

func TestListUnknownKind(t *testing.T) {
	a := New(Definition{Kind: "x", List: ListSpec{Kind: "bogus"}}, t.TempDir())
	if _, err := a.List(); err == nil {
		t.Error("List() with an unknown kind: expected an error, got nil")
	}
}

func TestListDispatch(t *testing.T) {
	// List() should route to the strategy named by List.Kind; json_file_map
	// is the one exercisable without running a command.
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "package.json"), `{"scripts": {"dev": "vite"}}`)

	a := New(Definition{
		Kind: "npm",
		List: ListSpec{Kind: ListJSONFileMap, File: "package.json", ScriptsField: "scripts"},
	}, dir)
	tasks, err := a.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 || tasks[0].Name != "dev" {
		t.Fatalf("List() = %+v, want the single task \"dev\"", tasks)
	}
}

func TestRunSubstitutesName(t *testing.T) {
	dir := t.TempDir()
	a := New(Definition{
		Kind: "x",
		Run:  []string{"sh", "-c", "printf %s {{name}} > ran.txt"},
	}, dir)

	if err := a.Run("hello"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "ran.txt"))
	if err != nil {
		t.Fatalf("run command did not execute in the adaptor's dir: %v", err)
	}
	if string(data) != "hello" {
		t.Errorf("run output = %q, want %q ({{name}} substitution)", data, "hello")
	}
}

func TestRunNoCommand(t *testing.T) {
	a := New(Definition{Kind: "x"}, t.TempDir())
	if err := a.Run("anything"); err == nil {
		t.Error("Run() with no run command: expected an error, got nil")
	}
}

func TestRunFailurePropagates(t *testing.T) {
	a := New(Definition{Kind: "x", Run: []string{"false"}}, t.TempDir())
	if err := a.Run("anything"); err == nil {
		t.Error("Run() of a failing command: expected an error, got nil")
	}
}

func TestDiscovererDefinitionFilesAndNew(t *testing.T) {
	d := Discoverer(Definition{Kind: "mise", DefinitionFiles: []string{"mise.toml", ".mise.toml"}})

	files := d.DefinitionFiles()
	if len(files) != 2 || files[0] != "mise.toml" {
		t.Errorf("DefinitionFiles() = %v, want [mise.toml .mise.toml]", files)
	}

	dir := t.TempDir()
	a := d.New(dir)
	if a.TaskDefinition() != "mise" || a.Dir() != dir {
		t.Errorf("New() built adaptor %q in %q, want mise in %q", a.TaskDefinition(), a.Dir(), dir)
	}
}

func TestMatchesNilMatchAcceptsEverything(t *testing.T) {
	d := Discoverer(Definition{Kind: "npm"}).(task.Matcher)
	if !d.Matches(t.TempDir()) {
		t.Error("Matches() = false with no Match configured, want true")
	}
}

func TestMatchesLockfile(t *testing.T) {
	d := Discoverer(Definition{
		Kind:  "pnpm",
		Match: &MatchSpec{Lockfiles: []string{"pnpm-lock.yaml"}},
	}).(task.Matcher)

	dir := t.TempDir()
	if d.Matches(dir) {
		t.Error("Matches() = true without the lockfile")
	}
	writeFile(t, filepath.Join(dir, "pnpm-lock.yaml"), "")
	if !d.Matches(dir) {
		t.Error("Matches() = false with the lockfile present")
	}
}

func TestMatchesManagerField(t *testing.T) {
	d := Discoverer(Definition{
		Kind: "pnpm",
		Match: &MatchSpec{
			// File empty on purpose: it should default to package.json.
			Manager: &ManagerFieldSpec{Field: "packageManager", Name: "pnpm"},
		},
	}).(task.Matcher)

	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "package.json"), `{"packageManager": "pnpm@9.0.0"}`)
	if !d.Matches(dir) {
		t.Error("Matches() = false with a matching packageManager field")
	}

	other := t.TempDir()
	writeFile(t, filepath.Join(other, "package.json"), `{"packageManager": "yarn@4.0.0"}`)
	if d.Matches(other) {
		t.Error("Matches() = true for a different manager")
	}
}

func TestMatchesAncestorTool(t *testing.T) {
	const tomlName = "tsk-test-mise.toml"
	d := Discoverer(Definition{
		Kind: "mise",
		Match: &MatchSpec{
			AncestorTool: &AncestorTOMLSpec{Files: []string{tomlName}, Table: "tools", Key: "bun"},
		},
	}).(task.Matcher)

	root := t.TempDir()
	child := filepath.Join(root, "sub")
	if err := os.MkdirAll(child, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, tomlName), "[tools]\nbun = \"latest\"\n")

	if !d.Matches(child) {
		t.Error("Matches() = false with the key declared in an ancestor's TOML")
	}
	if d.Matches(t.TempDir()) {
		t.Error("Matches() = true with no ancestor declaration")
	}
}
