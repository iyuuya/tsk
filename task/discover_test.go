package task_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/iyuuya/tsk/task"
)

// fakeAdaptor is a minimal task.Adaptor for exercising discovery and Task
// without shelling out to any real tool.
type fakeAdaptor struct {
	kind  string
	dir   string
	tasks []task.Task
	ran   []string
	err   error
}

func (a *fakeAdaptor) TaskDefinition() string     { return a.kind }
func (a *fakeAdaptor) Dir() string                { return a.dir }
func (a *fakeAdaptor) List() ([]task.Task, error) { return a.tasks, a.err }
func (a *fakeAdaptor) Run(name string) error      { a.ran = append(a.ran, name); return a.err }

// fakeDiscoverer implements task.Discoverer but NOT task.Matcher, so every
// candidate directory is accepted.
type fakeDiscoverer struct {
	kind  string
	files []string
}

func (d *fakeDiscoverer) DefinitionFiles() []string { return d.files }
func (d *fakeDiscoverer) New(dir string) task.Adaptor {
	return &fakeAdaptor{kind: d.kind, dir: dir}
}

// matchingDiscoverer adds a Matches implementation on top.
type matchingDiscoverer struct {
	fakeDiscoverer
	matches func(dir string) bool
}

func (d *matchingDiscoverer) Matches(dir string) bool { return d.matches(dir) }

func touch(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDiscoverFindsRootAndSubdirs(t *testing.T) {
	root := t.TempDir()
	touch(t, filepath.Join(root, "package.json"))
	touch(t, filepath.Join(root, "sub", "package.json"))
	touch(t, filepath.Join(root, "sub", "deep", "nested", "package.json"))

	d := &fakeDiscoverer{kind: "npm", files: []string{"package.json"}}
	found, err := task.Discover(root, d)
	if err != nil {
		t.Fatal(err)
	}

	wantDirs := []string{
		root,
		filepath.Join(root, "sub"),
		filepath.Join(root, "sub", "deep", "nested"),
	}
	if len(found) != len(wantDirs) {
		t.Fatalf("got %d adaptors, want %d: %+v", len(found), len(wantDirs), found)
	}
	for i, want := range wantDirs {
		if got := found[i].Adaptor.Dir(); got != want {
			t.Errorf("found[%d].Dir() = %q, want %q (sorted by dir)", i, got, want)
		}
		if got := found[i].DefinitionFile; got != filepath.Join(want, "package.json") {
			t.Errorf("found[%d].DefinitionFile = %q, want it under %q", i, got, want)
		}
		if found[i].ModTime.IsZero() {
			t.Errorf("found[%d].ModTime is zero", i)
		}
	}
}

func TestDiscoverSkipsIgnoredDirs(t *testing.T) {
	root := t.TempDir()
	touch(t, filepath.Join(root, "node_modules", "dep", "package.json"))
	touch(t, filepath.Join(root, ".git", "package.json"))
	touch(t, filepath.Join(root, "vendor", "package.json"))
	touch(t, filepath.Join(root, "app", "node_modules", "dep", "package.json"))
	touch(t, filepath.Join(root, "app", "package.json"))

	d := &fakeDiscoverer{kind: "npm", files: []string{"package.json"}}
	found, err := task.Discover(root, d)
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 1 || found[0].Adaptor.Dir() != filepath.Join(root, "app") {
		t.Fatalf("got %+v, want exactly one adaptor in app/ (ignored dirs must be skipped)", found)
	}
}

func TestDiscoverSharedFilePriority(t *testing.T) {
	root := t.TempDir()
	touch(t, filepath.Join(root, "a", "package.json"))
	touch(t, filepath.Join(root, "b", "package.json"))

	dirA := filepath.Join(root, "a")
	// bun claims only a/; npm (no Matcher) is the fallback for everything.
	bun := &matchingDiscoverer{
		fakeDiscoverer: fakeDiscoverer{kind: "bun", files: []string{"package.json"}},
		matches:        func(dir string) bool { return dir == dirA },
	}
	npm := &fakeDiscoverer{kind: "npm", files: []string{"package.json"}}

	found, err := task.Discover(root, bun, npm)
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 2 {
		t.Fatalf("got %d adaptors, want 2: %+v", len(found), found)
	}

	byDir := map[string]string{}
	for _, f := range found {
		byDir[f.Adaptor.Dir()] = f.Adaptor.TaskDefinition()
	}
	if byDir[dirA] != "bun" {
		t.Errorf("a/ matched %q, want bun (first matching discoverer wins)", byDir[dirA])
	}
	if byDir[filepath.Join(root, "b")] != "npm" {
		t.Errorf("b/ matched %q, want npm (fallback when the matcher rejects)", byDir[filepath.Join(root, "b")])
	}
}

func TestDiscoverOneAdaptorPerDirDespiteMultipleFiles(t *testing.T) {
	root := t.TempDir()
	// Both at the root and in a subtree, two definition files of the same
	// discoverer in one directory must yield a single adaptor instance.
	touch(t, filepath.Join(root, "mise.toml"))
	touch(t, filepath.Join(root, ".mise.toml"))
	touch(t, filepath.Join(root, "sub", "mise.toml"))
	touch(t, filepath.Join(root, "sub", ".mise.toml"))

	d := &fakeDiscoverer{kind: "mise", files: []string{"mise.toml", ".mise.toml"}}
	found, err := task.Discover(root, d)
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 2 {
		t.Fatalf("got %d adaptors, want 2 (one per directory): %+v", len(found), found)
	}
}

func TestDiscoverNoMatches(t *testing.T) {
	root := t.TempDir()
	touch(t, filepath.Join(root, "README.md"))

	d := &fakeDiscoverer{kind: "npm", files: []string{"package.json"}}
	found, err := task.Discover(root, d)
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 0 {
		t.Fatalf("got %+v, want no adaptors", found)
	}
}

func TestDiscoverUnreadableSubtreeErrors(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root; permission bits don't restrict reads")
	}
	root := t.TempDir()
	sub := filepath.Join(root, "sub")
	touch(t, filepath.Join(sub, "package.json"))
	if err := os.Chmod(sub, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(sub, 0o755) })

	d := &fakeDiscoverer{kind: "npm", files: []string{"package.json"}}
	if _, err := task.Discover(root, d); err == nil {
		t.Error("Discover() over an unreadable subtree: expected an error, got nil")
	}
}

func TestDiscoverMissingRoot(t *testing.T) {
	d := &fakeDiscoverer{kind: "npm", files: []string{"package.json"}}
	if _, err := task.Discover(filepath.Join(t.TempDir(), "absent"), d); err == nil {
		t.Error("Discover() of a nonexistent root: expected an error, got nil")
	}
}
