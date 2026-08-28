package task

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// skipDirs are directories whose contents are never scanned for task
// definitions.
var skipDirs = map[string]bool{
	".git":         true,
	"node_modules": true,
	"vendor":       true,
}

// DiscoveredAdaptor pairs an Adaptor instance with the definition file that
// caused Discover to create it, plus that file's modification time. This is
// the fingerprint callers (e.g. the state cache) use to tell whether an
// adaptor's tasks may have changed, without invoking the adaptor's own
// tooling just to check.
type DiscoveredAdaptor struct {
	Adaptor        Adaptor
	DefinitionFile string
	ModTime        time.Time
}

// Discover walks root looking for files that mark a directory as belonging
// to one of the given discoverers, and returns one DiscoveredAdaptor per
// (discoverer, directory) match. A monorepo with a definition file in
// several directories yields one Adaptor per directory.
//
// Several discoverers can share the same definition filename (e.g. bun,
// pnpm, yarn and npm all key off package.json). When that happens, they are
// tried in the order they're passed to Discover and the first one whose
// Matches accepts the directory wins — later discoverers for that file are
// not consulted. Pass ambiguous discoverers most-specific first.
//
// root's immediate subdirectories are each an independent subtree, so they
// are walked concurrently.
func Discover(root string, discoverers ...Discoverer) ([]DiscoveredAdaptor, error) {
	fileToDiscoverers := make(map[string][]Discoverer)
	for _, d := range discoverers {
		for _, f := range d.DefinitionFiles() {
			fileToDiscoverers[f] = append(fileToDiscoverers[f], d)
		}
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}

	var found []DiscoveredAdaptor

	// root's own files are matched once, up front, since they don't belong
	// to any of the concurrent subtree walks below.
	seenTop := make(map[Discoverer]bool)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		d, ok := pickDiscoverer(fileToDiscoverers[entry.Name()], root)
		if !ok || seenTop[d] {
			continue
		}
		seenTop[d] = true

		info, err := entry.Info()
		if err != nil {
			return nil, err
		}
		found = append(found, DiscoveredAdaptor{
			Adaptor:        d.New(root),
			DefinitionFile: filepath.Join(root, entry.Name()),
			ModTime:        info.ModTime(),
		})
	}

	var (
		mu       sync.Mutex
		wg       sync.WaitGroup
		firstErr error
	)
	for _, entry := range entries {
		if !entry.IsDir() || skipDirs[entry.Name()] {
			continue
		}

		wg.Add(1)
		go func(subRoot string) {
			defer wg.Done()
			sub, err := discoverSubtree(subRoot, fileToDiscoverers)

			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				if firstErr == nil {
					firstErr = err
				}
				return
			}
			found = append(found, sub...)
		}(filepath.Join(root, entry.Name()))
	}
	wg.Wait()

	if firstErr != nil {
		return nil, firstErr
	}

	// Subtrees finish in whatever order their goroutines happen to
	// complete, so the merge order above is nondeterministic. Sort to give
	// callers (and their output) a stable, repeatable order regardless.
	sort.Slice(found, func(i, j int) bool {
		if found[i].Adaptor.Dir() != found[j].Adaptor.Dir() {
			return found[i].Adaptor.Dir() < found[j].Adaptor.Dir()
		}
		return found[i].Adaptor.TaskDefinition() < found[j].Adaptor.TaskDefinition()
	})
	return found, nil
}

// pickDiscoverer returns the first of ds (in order) whose Matches accepts
// dir. Discoverers that don't implement Matcher are unconditionally
// accepted.
func pickDiscoverer(ds []Discoverer, dir string) (Discoverer, bool) {
	for _, d := range ds {
		if m, ok := d.(Matcher); ok && !m.Matches(dir) {
			continue
		}
		return d, true
	}
	return nil, false
}

// discoverSubtree sequentially walks a single subtree; it is the unit of
// work Discover parallelizes across root's immediate subdirectories.
func discoverSubtree(root string, fileToDiscoverers map[string][]Discoverer) ([]DiscoveredAdaptor, error) {
	seen := make(map[Discoverer]map[string]bool)

	var found []DiscoveredAdaptor
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if skipDirs[entry.Name()] {
				return filepath.SkipDir
			}
			return nil
		}

		dir := filepath.Dir(path)
		d, ok := pickDiscoverer(fileToDiscoverers[entry.Name()], dir)
		if !ok {
			return nil
		}

		if seen[d] == nil {
			seen[d] = make(map[string]bool)
		}
		if seen[d][dir] {
			return nil
		}
		seen[d][dir] = true

		info, err := entry.Info()
		if err != nil {
			return err
		}

		found = append(found, DiscoveredAdaptor{
			Adaptor:        d.New(dir),
			DefinitionFile: path,
			ModTime:        info.ModTime(),
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return found, nil
}
