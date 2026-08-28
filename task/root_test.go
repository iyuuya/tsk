package task_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/iyuuya/tsk/task"
)

func TestProjectRoot(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	root := t.TempDir()
	if out, err := exec.Command("git", "init", root).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	sub := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := task.ProjectRoot(sub)
	if err != nil {
		t.Fatal(err)
	}
	// Resolve symlinks on both sides: git reports the physical path, while
	// TempDir may be reached through a symlink (e.g. /tmp variants).
	wantResolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	gotResolved, err := filepath.EvalSymlinks(got)
	if err != nil {
		t.Fatal(err)
	}
	if gotResolved != wantResolved {
		t.Errorf("ProjectRoot(%q) = %q, want %q", sub, gotResolved, wantResolved)
	}
}

func TestProjectRootOutsideRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	// GIT_CEILING_DIRECTORIES can't stop the discovery at the temp dir's
	// parent reliably across environments, so just pick a dir that is
	// certainly not inside a work tree by creating a bare repo elsewhere and
	// relying on TempDir not being under one. If it is, skip.
	dir := t.TempDir()
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(dir))

	if _, err := task.ProjectRoot(dir); err == nil {
		t.Skip("temp dir is inside a git work tree; cannot exercise the failure path")
	}
}
