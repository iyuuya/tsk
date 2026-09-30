package spec

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/iyuuya/tsk/task"
)

// instance is the generic task.Adaptor built from a Definition, rooted at
// dir.
type instance struct {
	def Definition
	dir string
}

// New creates an Adaptor for def, rooted at dir.
func New(def Definition, dir string) task.Adaptor {
	return &instance{def: def, dir: dir}
}

// TaskDefinition identifies the adaptor kind, e.g. "mise".
func (a *instance) TaskDefinition() string {
	return a.def.Kind
}

// Dir is the directory this adaptor instance operates in.
func (a *instance) Dir() string {
	return a.dir
}

// List enumerates this adaptor's tasks using the strategy named by
// def.List.Kind.
func (a *instance) List() ([]task.Task, error) {
	switch a.def.List.Kind {
	case ListJSONCommand:
		return a.listJSONCommand()
	case ListRegexCommand:
		return a.listRegexCommand()
	case ListJSONFileMap:
		return a.listJSONFileMap()
	case ListTOMLFileMap:
		return a.listTOMLFileMap()
	default:
		return nil, fmt.Errorf("adaptor %q: unknown list kind %q", a.def.Kind, a.def.List.Kind)
	}
}

// Run executes the task named name using def.Run, substituting "{{name}}"
// for it, in Dir, connecting the child process to the current process's
// stdio.
func (a *instance) Run(name string) error {
	if len(a.def.Run) == 0 {
		return fmt.Errorf("adaptor %q: no run command configured", a.def.Kind)
	}
	args := make([]string, len(a.def.Run))
	for i, arg := range a.def.Run {
		args[i] = strings.ReplaceAll(arg, "{{name}}", name)
	}

	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = a.dir
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// discoverer implements task.Discoverer (and task.Matcher, when def.Match
// is set) for def. It's used as a map key by task.Discover, so it's kept
// as a pointer: Definition holds slices, which aren't comparable, so the
// discoverer value itself can't be a map key — but a pointer to it always
// is.
type discoverer struct {
	def Definition
}

// Discoverer returns a task.Discoverer for def.
func Discoverer(def Definition) task.Discoverer {
	return &discoverer{def: def}
}

func (d *discoverer) DefinitionFiles() []string {
	return d.def.DefinitionFiles
}

func (d *discoverer) New(dir string) task.Adaptor {
	return New(d.def, dir)
}

// Matches reports whether dir's definition file actually belongs to this
// adaptor. With no Match configured, every candidate is accepted.
func (d *discoverer) Matches(dir string) bool {
	m := d.def.Match
	if m == nil {
		return true
	}
	if hasLockfile(dir, m.Lockfiles...) {
		return true
	}
	if mg := m.Manager; mg != nil {
		file := mg.File
		if file == "" {
			file = "package.json"
		}
		if declaresManager(dir, file, mg.Field, mg.Name) {
			return true
		}
	}
	if at := m.AncestorTool; at != nil && ancestorTOMLHasKey(dir, at.Files, at.Table, at.Key) {
		return true
	}
	return false
}
