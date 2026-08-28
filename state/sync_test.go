package state

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/iyuuya/tsk/task"
)

// countingAdaptor records how many times List() is invoked, standing in for
// an adaptor whose List shells out to a real task runner.
type countingAdaptor struct {
	kind      string
	dir       string
	tasks     []task.Task
	listCalls int
	listErr   error
}

func (a *countingAdaptor) TaskDefinition() string { return a.kind }
func (a *countingAdaptor) Dir() string            { return a.dir }
func (a *countingAdaptor) List() ([]task.Task, error) {
	a.listCalls++
	return a.tasks, a.listErr
}
func (a *countingAdaptor) Run(string) error { return nil }

func discovered(a *countingAdaptor, defFile string, mod time.Time) task.DiscoveredAdaptor {
	return task.DiscoveredAdaptor{Adaptor: a, DefinitionFile: defFile, ModTime: mod}
}

func TestSyncInitialRunListsAndCaches(t *testing.T) {
	setStateHome(t)
	root := t.TempDir()

	a := &countingAdaptor{
		kind:  "npm",
		dir:   root,
		tasks: []task.Task{{Name: "build", Description: "compile"}},
	}
	mod := time.Date(2026, 8, 28, 10, 0, 0, 0, time.UTC)

	tasks, err := Sync(root, []task.DiscoveredAdaptor{discovered(a, filepath.Join(root, "package.json"), mod)}, false)
	if err != nil {
		t.Fatal(err)
	}
	if a.listCalls != 1 {
		t.Errorf("List() called %d times on first sync, want 1", a.listCalls)
	}
	if len(tasks) != 1 || tasks[0].Name != "build" {
		t.Fatalf("Sync() returned %+v, want the adaptor's single task", tasks)
	}
	if tasks[0].Adaptor != a {
		t.Error("returned task does not carry the live adaptor reference")
	}

	s, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if s == nil || len(s.Adaptors) != 1 {
		t.Fatalf("cache after sync = %+v, want one adaptor", s)
	}
	ca := s.Adaptors[0]
	if ca.Kind != "npm" || ca.Dir != "." || ca.DefinitionFile != "package.json" {
		t.Errorf("cached adaptor = %+v, want kind npm with paths relative to root", ca)
	}
	if !ca.ModTime.Equal(mod) {
		t.Errorf("cached ModTime = %v, want %v", ca.ModTime, mod)
	}
}

func TestSyncReusesCacheWhenModTimeUnchanged(t *testing.T) {
	setStateHome(t)
	root := t.TempDir()

	a := &countingAdaptor{kind: "npm", dir: root, tasks: []task.Task{{Name: "build"}}}
	mod := time.Date(2026, 8, 28, 10, 0, 0, 0, time.UTC)
	d := []task.DiscoveredAdaptor{discovered(a, filepath.Join(root, "package.json"), mod)}

	if _, err := Sync(root, d, false); err != nil {
		t.Fatal(err)
	}
	tasks, err := Sync(root, d, false)
	if err != nil {
		t.Fatal(err)
	}
	if a.listCalls != 1 {
		t.Errorf("List() called %d times across two syncs with unchanged mtime, want 1", a.listCalls)
	}
	if len(tasks) != 1 || tasks[0].Name != "build" || tasks[0].Adaptor != a {
		t.Fatalf("cached sync returned %+v, want the cached task with a live adaptor", tasks)
	}
}

func TestSyncRelistsWhenModTimeChanges(t *testing.T) {
	setStateHome(t)
	root := t.TempDir()

	a := &countingAdaptor{kind: "npm", dir: root, tasks: []task.Task{{Name: "build"}}}
	def := filepath.Join(root, "package.json")
	mod := time.Date(2026, 8, 28, 10, 0, 0, 0, time.UTC)

	if _, err := Sync(root, []task.DiscoveredAdaptor{discovered(a, def, mod)}, false); err != nil {
		t.Fatal(err)
	}

	a.tasks = []task.Task{{Name: "build"}, {Name: "test"}}
	tasks, err := Sync(root, []task.DiscoveredAdaptor{discovered(a, def, mod.Add(time.Minute))}, false)
	if err != nil {
		t.Fatal(err)
	}
	if a.listCalls != 2 {
		t.Errorf("List() called %d times after an mtime change, want 2", a.listCalls)
	}
	if len(tasks) != 2 {
		t.Fatalf("got %d tasks after re-list, want 2", len(tasks))
	}
}

func TestSyncForceRelists(t *testing.T) {
	setStateHome(t)
	root := t.TempDir()

	a := &countingAdaptor{kind: "npm", dir: root, tasks: []task.Task{{Name: "build"}}}
	d := []task.DiscoveredAdaptor{discovered(a, filepath.Join(root, "package.json"), time.Now())}

	if _, err := Sync(root, d, false); err != nil {
		t.Fatal(err)
	}
	if _, err := Sync(root, d, true); err != nil {
		t.Fatal(err)
	}
	if a.listCalls != 2 {
		t.Errorf("List() called %d times with force, want 2", a.listCalls)
	}
}

func TestSyncDropsRemovedAdaptors(t *testing.T) {
	setStateHome(t)
	root := t.TempDir()

	mod := time.Now()
	a := &countingAdaptor{kind: "npm", dir: root, tasks: []task.Task{{Name: "build"}}}
	b := &countingAdaptor{kind: "make", dir: root, tasks: []task.Task{{Name: "all"}}}

	both := []task.DiscoveredAdaptor{
		discovered(a, filepath.Join(root, "package.json"), mod),
		discovered(b, filepath.Join(root, "Makefile"), mod),
	}
	if _, err := Sync(root, both, false); err != nil {
		t.Fatal(err)
	}

	// make's Makefile disappeared from disk; only npm remains discovered.
	tasks, err := Sync(root, both[:1], false)
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 || tasks[0].Name != "build" {
		t.Fatalf("got %+v, want only npm's task", tasks)
	}

	s, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Adaptors) != 1 || s.Adaptors[0].Kind != "npm" {
		t.Errorf("cache after removal = %+v, want only the npm adaptor", s.Adaptors)
	}
}

func TestSyncPropagatesListError(t *testing.T) {
	setStateHome(t)
	root := t.TempDir()

	wantErr := errors.New("tool exploded")
	a := &countingAdaptor{kind: "npm", dir: root, listErr: wantErr}
	d := []task.DiscoveredAdaptor{discovered(a, filepath.Join(root, "package.json"), time.Now())}

	if _, err := Sync(root, d, false); !errors.Is(err, wantErr) {
		t.Errorf("Sync() error = %v, want %v", err, wantErr)
	}
}

func TestSyncFallsBackToAbsolutePathsWhenRelFails(t *testing.T) {
	// filepath.Rel can't relate a relative root to an absolute adaptor dir;
	// Sync must then store the absolute paths rather than fail.
	setStateHome(t)

	a := &countingAdaptor{kind: "npm", dir: "/abs/proj", tasks: []task.Task{{Name: "build"}}}
	d := []task.DiscoveredAdaptor{discovered(a, "/abs/proj/package.json", time.Now())}

	if _, err := Sync("relative-root", d, false); err != nil {
		t.Fatal(err)
	}
	s, err := Load("relative-root")
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Adaptors) != 1 || s.Adaptors[0].Dir != "/abs/proj" {
		t.Errorf("cached adaptors = %+v, want Dir kept absolute as a fallback", s.Adaptors)
	}
}

func TestSyncEmptyDiscoveryWritesEmptyCache(t *testing.T) {
	setStateHome(t)
	root := t.TempDir()

	tasks, err := Sync(root, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 0 {
		t.Errorf("Sync() with nothing discovered = %+v, want no tasks", tasks)
	}
	s, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if s == nil {
		t.Fatal("no cache written for a first sync with nothing discovered")
	}
	if len(s.Adaptors) != 0 {
		t.Errorf("cache adaptors = %+v, want empty", s.Adaptors)
	}
}
