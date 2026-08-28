package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/iyuuya/tsk/spec"
)

// setConfigHome points os.UserConfigDir at a fresh directory so tests never
// touch the user's real config.
func setConfigHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)
	return home
}

func TestPathHonorsXDGConfigHome(t *testing.T) {
	home := setConfigHome(t)

	path, err := Path()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(home, "tsk", "config.toml"); path != want {
		t.Errorf("Path() = %q, want %q", path, want)
	}
}

func TestLoadFallsBackToDefault(t *testing.T) {
	setConfigHome(t)

	defs, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	want, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(defs, want) {
		t.Error("Load() without a config file differs from Default()")
	}
}

func TestSaveLoadRoundtrip(t *testing.T) {
	setConfigHome(t)

	want := []spec.Definition{{
		Kind:            "custom",
		DefinitionFiles: []string{"Custom.toml"},
		Match: &spec.MatchSpec{
			Lockfiles: []string{"custom.lock"},
			Manager:   &spec.ManagerFieldSpec{File: "meta.json", Field: "packageManager", Name: "custom"},
			AncestorTool: &spec.AncestorTOMLSpec{
				Files: []string{"tools.toml"}, Table: "tools", Key: "custom",
			},
		},
		List: spec.ListSpec{
			Kind:             spec.ListJSONCommand,
			Command:          "custom",
			Args:             []string{"tasks", "--json"},
			NameField:        "name",
			DescriptionField: "desc",
			SourceField:      "source",
		},
		Run: []string{"custom", "run", "{{name}}"},
	}}
	if err := Save(want); err != nil {
		t.Fatal(err)
	}

	got, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Load() after Save() = %+v, want %+v", got, want)
	}
}

func TestDefaultSurvivesRoundtrip(t *testing.T) {
	// The built-in set must serialize to the config-file format and come
	// back identical — this is what `tsk config init` relies on.
	setConfigHome(t)

	defs, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	if err := Save(defs); err != nil {
		t.Fatal(err)
	}
	got, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, defs) {
		t.Error("Default() does not survive a Save/Load roundtrip")
	}
}

func TestExists(t *testing.T) {
	setConfigHome(t)

	ok, err := Exists()
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Error("Exists() = true before any config was written")
	}

	if err := Save(nil); err != nil {
		t.Fatal(err)
	}
	ok, err = Exists()
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Error("Exists() = false after Save()")
	}
}

func TestSaveLeavesNoTempFile(t *testing.T) {
	setConfigHome(t)

	defs, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	if err := Save(defs); err != nil {
		t.Fatal(err)
	}
	dir, err := Dir()
	if err != nil {
		t.Fatal(err)
	}
	tmps, err := filepath.Glob(filepath.Join(dir, "*.tmp"))
	if err != nil {
		t.Fatal(err)
	}
	if len(tmps) != 0 {
		t.Errorf("temp files left behind after Save(): %v", tmps)
	}
}

func TestErrorsWhenConfigDirUnresolvable(t *testing.T) {
	// With neither XDG_CONFIG_HOME nor HOME set, os.UserConfigDir fails and
	// every path-dependent entry point must surface that.
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", "")

	if _, err := Dir(); err == nil {
		t.Error("Dir(): expected an error, got nil")
	}
	if _, err := Path(); err == nil {
		t.Error("Path(): expected an error, got nil")
	}
	if _, err := Load(); err == nil {
		t.Error("Load(): expected an error, got nil")
	}
	if err := Save(nil); err == nil {
		t.Error("Save(): expected an error, got nil")
	}
	if _, err := Exists(); err == nil {
		t.Error("Exists(): expected an error, got nil")
	}
}

func TestDirCreateFails(t *testing.T) {
	// Point XDG_CONFIG_HOME below a regular file so MkdirAll must fail.
	home := t.TempDir()
	file := filepath.Join(home, "occupied")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(file, "nested"))

	if _, err := Dir(); err == nil {
		t.Error("Dir() with an uncreatable path: expected an error, got nil")
	}
}

func TestLoadUnreadableConfigErrors(t *testing.T) {
	// A config path that exists but can't be read as a file (it's a
	// directory) must be reported, not treated as "no config".
	setConfigHome(t)
	path, err := Path()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(); err == nil {
		t.Error("Load() of an unreadable config: expected an error, got nil")
	}
}

func TestLoadBrokenConfigErrors(t *testing.T) {
	setConfigHome(t)

	path, err := Path()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("not [valid toml"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(); err == nil {
		t.Error("Load() of an invalid config file: expected an error, got nil")
	}
}
