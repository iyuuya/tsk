package state

import (
	"path/filepath"
	"time"

	"github.com/iyuuya/tsk/task"
)

// Sync reconciles the on-disk cache for root against discovered. For each
// discovered adaptor instance, it reuses the cached task list when that
// adaptor's definition file's modification time matches what's on record,
// and only falls back to calling Adaptor.List() (which shells out to the
// adaptor's own CLI) when the file changed, the adaptor is new, or force is
// set. Adaptors that disappeared from disk are dropped from the cache.
//
// The cache is rewritten only when something actually changed, and the
// returned tasks always carry live Adaptor references from discovered so
// callers can Run() them.
func Sync(root string, discovered []task.DiscoveredAdaptor, force bool) ([]task.Task, error) {
	cached, err := Load(root)
	if err != nil {
		return nil, err
	}

	cachedByKey := make(map[string]CachedAdaptor)
	if cached != nil {
		for _, a := range cached.Adaptors {
			cachedByKey[a.Kind+"\x00"+a.Dir] = a
		}
	}

	changed := cached == nil
	newAdaptors := make([]CachedAdaptor, 0, len(discovered))
	var tasks []task.Task

	for _, d := range discovered {
		relDir, err := filepath.Rel(root, d.Adaptor.Dir())
		if err != nil {
			relDir = d.Adaptor.Dir()
		}
		relDef, err := filepath.Rel(root, d.DefinitionFile)
		if err != nil {
			relDef = d.DefinitionFile
		}
		key := d.Adaptor.TaskDefinition() + "\x00" + relDir

		prev, wasCached := cachedByKey[key]
		delete(cachedByKey, key) // whatever remains afterwards was removed from disk

		var cts []CachedTask
		if !force && wasCached && prev.ModTime.Equal(d.ModTime) {
			cts = prev.Tasks
		} else {
			changed = true
			live, err := d.Adaptor.List()
			if err != nil {
				return nil, err
			}
			cts = make([]CachedTask, len(live))
			for i, t := range live {
				cts[i] = CachedTask{Name: t.Name, Description: t.Description}
			}
		}

		newAdaptors = append(newAdaptors, CachedAdaptor{
			Kind:           d.Adaptor.TaskDefinition(),
			Dir:            relDir,
			DefinitionFile: relDef,
			ModTime:        d.ModTime,
			Tasks:          cts,
		})
		for _, ct := range cts {
			tasks = append(tasks, task.Task{Name: ct.Name, Description: ct.Description, Adaptor: d.Adaptor})
		}
	}

	if len(cachedByKey) > 0 {
		changed = true
	}

	if changed {
		if err := Save(&State{Root: root, UpdatedAt: time.Now(), Adaptors: newAdaptors}); err != nil {
			return nil, err
		}
	}

	return tasks, nil
}
