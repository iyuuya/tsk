package spec

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestListJSONFileMap(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "package.json"),
		`{"name": "x", "scripts": {"test": "vitest", "build": "tsc", "dev": "vite"}}`)

	a := &instance{
		def: Definition{
			Kind: "npm",
			List: ListSpec{Kind: ListJSONFileMap, File: "package.json", ScriptsField: "scripts"},
		},
		dir: dir,
	}
	tasks, err := a.listJSONFileMap()
	if err != nil {
		t.Fatal(err)
	}

	// Keys of a map, so the implementation sorts them for stable output.
	wantNames := []string{"build", "dev", "test"}
	if len(tasks) != len(wantNames) {
		t.Fatalf("got %d tasks, want %d", len(tasks), len(wantNames))
	}
	for i, want := range wantNames {
		if tasks[i].Name != want {
			t.Errorf("tasks[%d].Name = %q, want %q (sorted order)", i, tasks[i].Name, want)
		}
	}
	if tasks[2].Description != "vitest" {
		t.Errorf("tasks[2].Description = %q, want %q", tasks[2].Description, "vitest")
	}
	if tasks[0].Adaptor != a {
		t.Error("task does not reference its owning adaptor")
	}
}

func TestListJSONFileMapMissingScriptsField(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "package.json"), `{"name": "x"}`)

	a := &instance{
		def: Definition{List: ListSpec{Kind: ListJSONFileMap, File: "package.json", ScriptsField: "scripts"}},
		dir: dir,
	}
	tasks, err := a.listJSONFileMap()
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 0 {
		t.Errorf("got %d tasks for a file with no scripts field, want 0", len(tasks))
	}
}

func TestListJSONFileMapErrors(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "broken.json"), `{not json`)

	tests := []struct {
		name string
		file string
	}{
		{"missing file", "absent.json"},
		{"invalid JSON", "broken.json"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &instance{
				def: Definition{List: ListSpec{Kind: ListJSONFileMap, File: tt.file, ScriptsField: "scripts"}},
				dir: dir,
			}
			if _, err := a.listJSONFileMap(); err == nil {
				t.Error("expected an error, got nil")
			}
		})
	}
}

func TestListTOMLFileMap(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "pyproject.toml"),
		"[project]\nname = \"x\"\n\n[project.scripts]\nserve = \"x.app:serve\"\ncli = \"x.cli:main\"\n")

	a := &instance{
		def: Definition{List: ListSpec{Kind: ListTOMLFileMap, File: "pyproject.toml", ScriptsField: "project.scripts"}},
		dir: dir,
	}
	tasks, err := a.listTOMLFileMap()
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 2 || tasks[0].Name != "cli" || tasks[1].Name != "serve" {
		t.Fatalf("tasks = %+v, want cli and serve in sorted order", tasks)
	}
	if tasks[1].Description != "x.app:serve" {
		t.Errorf("tasks[1].Description = %q, want %q", tasks[1].Description, "x.app:serve")
	}
}

func TestListTOMLFileMapMissingScriptsField(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "pyproject.toml"), "[project]\nname = \"x\"\n")

	a := &instance{
		def: Definition{List: ListSpec{Kind: ListTOMLFileMap, File: "pyproject.toml", ScriptsField: "project.scripts"}},
		dir: dir,
	}
	tasks, err := a.listTOMLFileMap()
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 0 {
		t.Errorf("got %d tasks for a file with no scripts table, want 0", len(tasks))
	}
}

func TestListTOMLFileMapErrors(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "broken.toml"), "[project\n")
	writeFile(t, filepath.Join(dir, "scalar.toml"), "[project]\nscripts = \"x\"\n")

	tests := []struct {
		name string
		file string
	}{
		{"missing file", "absent.toml"},
		{"invalid TOML", "broken.toml"},
		{"scripts field not a table", "scalar.toml"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &instance{
				def: Definition{List: ListSpec{Kind: ListTOMLFileMap, File: tt.file, ScriptsField: "project.scripts"}},
				dir: dir,
			}
			if _, err := a.listTOMLFileMap(); err == nil {
				t.Error("expected an error, got nil")
			}
		})
	}
}

func TestListJSONCommand(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "tasks.json"),
		`[{"name": "build", "description": "compile"}, {"name": "test", "description": "run tests"}]`)

	a := &instance{
		def: Definition{
			Kind: "mise",
			List: ListSpec{
				Kind:             ListJSONCommand,
				Command:          "cat",
				Args:             []string{"tasks.json"},
				NameField:        "name",
				DescriptionField: "description",
			},
		},
		dir: dir,
	}
	tasks, err := a.listJSONCommand()
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 2 {
		t.Fatalf("got %d tasks, want 2", len(tasks))
	}
	if tasks[0].Name != "build" || tasks[0].Description != "compile" {
		t.Errorf("tasks[0] = %q/%q, want build/compile", tasks[0].Name, tasks[0].Description)
	}
}

func TestListJSONCommandSourceFieldFiltersOtherDirs(t *testing.T) {
	dir := t.TempDir()
	abs, err := filepath.Abs(dir)
	if err != nil {
		t.Fatal(err)
	}

	// One task defined in dir itself, one inherited from a parent — only
	// the former should survive the SourceField filter.
	out, err := json.Marshal([]map[string]string{
		{"name": "local", "source": filepath.Join(abs, "mise.toml")},
		{"name": "inherited", "source": filepath.Join(filepath.Dir(abs), "mise.toml")},
	})
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, "tasks.json"), string(out))

	a := &instance{
		def: Definition{
			List: ListSpec{
				Kind:        ListJSONCommand,
				Command:     "cat",
				Args:        []string{"tasks.json"},
				NameField:   "name",
				SourceField: "source",
			},
		},
		dir: dir,
	}
	tasks, err := a.listJSONCommand()
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 || tasks[0].Name != "local" {
		t.Fatalf("got %+v, want exactly the task whose source is in the adaptor's own dir", tasks)
	}
}

func TestListJSONCommandBadOutput(t *testing.T) {
	a := &instance{
		def: Definition{
			List: ListSpec{Kind: ListJSONCommand, Command: "sh", Args: []string{"-c", "echo not-json"}, NameField: "name"},
		},
		dir: t.TempDir(),
	}
	if _, err := a.listJSONCommand(); err == nil {
		t.Error("expected a parse error for non-JSON output, got nil")
	}
}

func TestListRegexCommand(t *testing.T) {
	a := &instance{
		def: Definition{
			Kind: "rake",
			List: ListSpec{
				Kind:    ListRegexCommand,
				Command: "sh",
				Args: []string{"-c",
					`printf 'rake build    # Build the gem\nrake test     # Run tests\nnot a task line\n'`},
				Pattern: `^rake (\S+)\s+# (.+)$`,
			},
		},
		dir: t.TempDir(),
	}
	tasks, err := a.listRegexCommand()
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 2 {
		t.Fatalf("got %d tasks, want 2 (non-matching lines skipped)", len(tasks))
	}
	if tasks[0].Name != "build" || tasks[0].Description != "Build the gem" {
		t.Errorf("tasks[0] = %q/%q, want build/Build the gem", tasks[0].Name, tasks[0].Description)
	}
	if tasks[1].Name != "test" || tasks[1].Description != "Run tests" {
		t.Errorf("tasks[1] = %q/%q, want test/Run tests", tasks[1].Name, tasks[1].Description)
	}
}

func TestListRegexCommandSingleGroup(t *testing.T) {
	a := &instance{
		def: Definition{
			List: ListSpec{
				Kind:    ListRegexCommand,
				Command: "sh",
				Args:    []string{"-c", `printf 'build:\ntest:\n'`},
				Pattern: `^([a-z]+):$`,
			},
		},
		dir: t.TempDir(),
	}
	tasks, err := a.listRegexCommand()
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 2 {
		t.Fatalf("got %d tasks, want 2", len(tasks))
	}
	if tasks[0].Description != "" {
		t.Errorf("tasks[0].Description = %q, want empty when the pattern has one group", tasks[0].Description)
	}
}

func TestListRegexCommandBadPattern(t *testing.T) {
	a := &instance{
		def: Definition{List: ListSpec{Kind: ListRegexCommand, Command: "true", Pattern: `([`}},
		dir: t.TempDir(),
	}
	if _, err := a.listRegexCommand(); err == nil {
		t.Error("expected an error for an invalid pattern, got nil")
	}
}

func TestListCommandStrategiesPropagateCommandFailure(t *testing.T) {
	for _, kind := range []ListKind{ListJSONCommand, ListRegexCommand} {
		t.Run(string(kind), func(t *testing.T) {
			a := New(Definition{
				Kind: "x",
				List: ListSpec{Kind: kind, Command: "false", Pattern: `(x)`},
			}, t.TempDir())
			if _, err := a.List(); err == nil {
				t.Error("expected the command failure to propagate, got nil")
			}
		})
	}
}

func TestListJSONFileMapScriptsFieldWrongType(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "package.json"), `{"scripts": "not a map"}`)

	a := &instance{
		def: Definition{List: ListSpec{Kind: ListJSONFileMap, File: "package.json", ScriptsField: "scripts"}},
		dir: dir,
	}
	if _, err := a.listJSONFileMap(); err == nil {
		t.Error("expected an error for a non-map scripts field, got nil")
	}
}

func TestListDispatchesAllKinds(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "tasks.json"), `[{"name": "build"}]`)

	tests := []struct {
		name string
		list ListSpec
	}{
		{"json_command", ListSpec{
			Kind: ListJSONCommand, Command: "cat", Args: []string{"tasks.json"}, NameField: "name",
		}},
		{"regex_command", ListSpec{
			Kind: ListRegexCommand, Command: "sh", Args: []string{"-c", "printf 'build:\\n'"}, Pattern: `^(\S+):$`,
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := New(Definition{Kind: "x", List: tt.list}, dir)
			tasks, err := a.List()
			if err != nil {
				t.Fatal(err)
			}
			if len(tasks) != 1 || tasks[0].Name != "build" {
				t.Fatalf("List() via %s = %+v, want the single task \"build\"", tt.list.Kind, tasks)
			}
		})
	}
}

func TestCommandOutputFailureNamesCommand(t *testing.T) {
	a := &instance{
		def: Definition{List: ListSpec{Command: "sh", Args: []string{"-c", "exit 3"}}},
		dir: t.TempDir(),
	}
	_, err := a.commandOutput()
	if err == nil {
		t.Fatal("expected an error for a failing command, got nil")
	}
	if !strings.Contains(err.Error(), "sh -c") {
		t.Errorf("error %q does not name the command that failed", err)
	}
}
