package state

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// setStateHome points the state package at a fresh directory so tests never
// touch the user's real cache.
func setStateHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("XDG_STATE_HOME", home)
	return home
}

func TestDirHonorsXDGStateHome(t *testing.T) {
	home := setStateHome(t)

	dir, err := Dir()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(home, "tsk"); dir != want {
		t.Errorf("Dir() = %q, want %q", dir, want)
	}
}

func TestDirFallsBackToHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_STATE_HOME", "")
	t.Setenv("HOME", home)

	dir, err := Dir()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(home, ".local", "state", "tsk"); dir != want {
		t.Errorf("Dir() = %q, want %q (fallback under $HOME)", dir, want)
	}
}

func TestErrorsWhenStateDirUnresolvable(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "")
	t.Setenv("HOME", "")

	if _, err := Dir(); err == nil {
		t.Error("Dir(): expected an error, got nil")
	}
	if _, err := Load("/p"); err == nil {
		t.Error("Load(): expected an error, got nil")
	}
	if err := Save(&State{Root: "/p"}); err == nil {
		t.Error("Save(): expected an error, got nil")
	}
	if _, err := LoadAll(); err == nil {
		t.Error("LoadAll(): expected an error, got nil")
	}
}

func TestLoadUnreadableCacheErrors(t *testing.T) {
	// A cache path that exists but is a directory must be reported, not
	// treated as "no cache".
	setStateHome(t)
	path, err := pathForRoot("/p")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Load("/p"); err == nil {
		t.Error("Load() of an unreadable cache: expected an error, got nil")
	}
}

func TestLoadInvalidJSONErrors(t *testing.T) {
	setStateHome(t)
	path, err := pathForRoot("/p")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load("/p"); err == nil {
		t.Error("Load() of a corrupt cache: expected an error, got nil")
	}
}

func TestLoadMissingReturnsNilNil(t *testing.T) {
	setStateHome(t)

	s, err := Load("/some/project")
	if err != nil {
		t.Fatal(err)
	}
	if s != nil {
		t.Errorf("Load() of an uncached root = %+v, want nil", s)
	}
}

func TestSaveLoadRoundtrip(t *testing.T) {
	setStateHome(t)

	want := &State{
		Root:      "/some/project",
		UpdatedAt: time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC),
		Adaptors: []CachedAdaptor{{
			Kind:           "npm",
			Dir:            ".",
			DefinitionFile: "package.json",
			ModTime:        time.Date(2026, 8, 28, 11, 0, 0, 0, time.UTC),
			Tasks:          []CachedTask{{Name: "build", Description: "compile"}},
		}},
	}
	if err := Save(want); err != nil {
		t.Fatal(err)
	}

	got, err := Load(want.Root)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("Load() = nil after Save()")
	}
	if got.Root != want.Root || len(got.Adaptors) != 1 {
		t.Fatalf("Load() = %+v, want %+v", got, want)
	}
	a := got.Adaptors[0]
	if a.Kind != "npm" || a.Dir != "." || a.DefinitionFile != "package.json" {
		t.Errorf("adaptor = %+v, want the saved one", a)
	}
	if !a.ModTime.Equal(want.Adaptors[0].ModTime) {
		t.Errorf("ModTime = %v, want %v", a.ModTime, want.Adaptors[0].ModTime)
	}
	if len(a.Tasks) != 1 || a.Tasks[0].Name != "build" || a.Tasks[0].Description != "compile" {
		t.Errorf("tasks = %+v, want the saved ones", a.Tasks)
	}
}

func TestSaveLeavesNoTempFile(t *testing.T) {
	setStateHome(t)

	if err := Save(&State{Root: "/p"}); err != nil {
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

func TestDistinctRootsGetDistinctFiles(t *testing.T) {
	setStateHome(t)

	if err := Save(&State{Root: "/project/a"}); err != nil {
		t.Fatal(err)
	}
	if err := Save(&State{Root: "/project/b"}); err != nil {
		t.Fatal(err)
	}

	a, err := Load("/project/a")
	if err != nil || a == nil || a.Root != "/project/a" {
		t.Fatalf("Load(/project/a) = %+v, %v", a, err)
	}
	b, err := Load("/project/b")
	if err != nil || b == nil || b.Root != "/project/b" {
		t.Fatalf("Load(/project/b) = %+v, %v", b, err)
	}
}

func TestLoadAll(t *testing.T) {
	setStateHome(t)

	roots := []string{"/project/a", "/project/b", "/project/c"}
	for _, r := range roots {
		if err := Save(&State{Root: r}); err != nil {
			t.Fatal(err)
		}
	}

	// A corrupt sibling file must be skipped, not fail the whole listing.
	dir, err := Dir()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "corrupt.json"), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}

	states, err := LoadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(states) != len(roots) {
		t.Fatalf("LoadAll() returned %d states, want %d", len(states), len(roots))
	}
	seen := map[string]bool{}
	for _, s := range states {
		seen[s.Root] = true
	}
	for _, r := range roots {
		if !seen[r] {
			t.Errorf("LoadAll() is missing root %q; got roots %s", r, strings.Join(roots, ", "))
		}
	}
}

func TestLoadAllEmpty(t *testing.T) {
	setStateHome(t)

	states, err := LoadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(states) != 0 {
		t.Errorf("LoadAll() with no cache = %+v, want empty", states)
	}
}
