package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/iyuuya/go/exitcode"

	"github.com/iyuuya/tsk/config"
	"github.com/iyuuya/tsk/spec"
	"github.com/iyuuya/tsk/state"
	"github.com/iyuuya/tsk/task"
)

// discoverers builds a task.Discoverer for each of tsk's configured
// adaptors: the user's config file (config.Load), falling back to the
// built-in set (config.Default) when none exists. Their relative order is
// the priority task.Discover picks among discoverers that share a
// definition file: bun, pnpm, yarn and npm all key off package.json, so
// config.Default lists them bun > pnpm > yarn > npm (npm has no Match, so
// it's the fallback when none of the others claim the directory).
func discoverers() ([]task.Discoverer, error) {
	defs, err := config.Load()
	if err != nil {
		return nil, err
	}
	ds := make([]task.Discoverer, len(defs))
	for i, def := range defs {
		ds[i] = spec.Discoverer(def)
	}
	return ds, nil
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: tsk list [--refresh] [--global] [--json]")
	fmt.Fprintln(os.Stderr, "       tsk run [--refresh] [--global] [<root>] [<dir>] <adaptor> <task>")
	fmt.Fprintln(os.Stderr, "       tsk config init [--force]")
}

func main() {
	os.Exit(int(realMain()))
}

// errUsage signals that a command was invoked with arguments that don't
// form any valid invocation; realMain responds by printing usage instead of
// an error message.
var errUsage = errors.New("invalid arguments")

func realMain() exitcode.ExitCode {
	if len(os.Args) < 2 {
		usage()
		return exitcode.ExitError
	}

	var err error
	switch os.Args[1] {
	case "list":
		err = cmdList(os.Args[2:])
	case "run":
		err = cmdRun(os.Args[2:])
	case "config":
		err = cmdConfig(os.Args[2:])
	default:
		err = errUsage
	}

	if err != nil {
		if errors.Is(err, errUsage) {
			usage()
		} else {
			fmt.Fprintln(os.Stderr, err)
		}
		return exitcode.ExitError
	}

	return exitcode.ExitOK
}

// cmdConfig dispatches tsk's "config" subcommands.
func cmdConfig(args []string) error {
	if len(args) < 1 {
		return errUsage
	}
	switch args[0] {
	case "init":
		return cmdConfigInit(args[1:])
	default:
		return errUsage
	}
}

// cmdConfigInit writes tsk's built-in default adaptor definitions to the
// user's config file, so they have a starting point to edit. It refuses to
// overwrite an existing file unless --force is given.
func cmdConfigInit(args []string) error {
	fs := flag.NewFlagSet("config init", flag.ContinueOnError)
	force := fs.Bool("force", false, "overwrite an existing config file")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if !*force {
		exists, err := config.Exists()
		if err != nil {
			return err
		}
		if exists {
			path, err := config.Path()
			if err != nil {
				return err
			}
			return fmt.Errorf("config file already exists at %s (use --force to overwrite)", path)
		}
	}

	defs, err := config.Default()
	if err != nil {
		return err
	}
	if err := config.Save(defs); err != nil {
		return err
	}
	path, err := config.Path()
	if err != nil {
		return err
	}
	fmt.Println(path)
	return nil
}

func cmdList(args []string) error {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	global := fs.Bool("global", false, "list tasks across every project tsk has cached state for")
	refresh := fs.Bool("refresh", false, "bypass the cache and re-discover tasks")
	jsonOut := fs.Bool("json", false, "print tasks as JSON, with each task's project root")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *global {
		if *refresh {
			return fmt.Errorf("--refresh is not supported with --global: global listing only reads cached state, it never walks other projects")
		}
		return listGlobal(*jsonOut)
	}

	root, err := task.ProjectRoot(".")
	if err != nil {
		return err
	}
	tasks, err := syncTasks(root, *refresh)
	if err != nil {
		return err
	}
	if *jsonOut {
		return printJSON(listTasks(root, tasks))
	}
	printTasks(root, tasks)
	return nil
}

// listTask is one element of `tsk list --json` output: a task plus the
// project root and root-relative directory a consumer needs to run it from
// anywhere (`tsk run --global <root> <dir> <adaptor> <task>`) — the tab-
// separated human output only carries display labels.
type listTask struct {
	Root        string `json:"root"`
	Dir         string `json:"dir"`
	Adaptor     string `json:"adaptor"`
	Task        string `json:"task"`
	Description string `json:"description"`
}

// listTasks flattens synced tasks into listTask rows, with Dir relative to
// root — the same shape the cached global state stores.
func listTasks(root string, tasks []task.Task) []listTask {
	rows := make([]listTask, 0, len(tasks))
	for _, t := range tasks {
		dir, err := filepath.Rel(root, t.Dir())
		if err != nil {
			dir = t.Dir()
		}
		rows = append(rows, listTask{Root: root, Dir: dir, Adaptor: t.AdaptorName(), Task: t.Name, Description: t.Description})
	}
	return rows
}

func printJSON(rows []listTask) error {
	data, err := json.Marshal(rows)
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}

func listGlobal(jsonOut bool) error {
	states, err := state.LoadAll()
	if err != nil {
		return err
	}
	sort.Slice(states, func(i, j int) bool { return projectLabel(states[i].Root) < projectLabel(states[j].Root) })
	if jsonOut {
		rows := make([]listTask, 0)
		for _, s := range states {
			for _, a := range s.Adaptors {
				for _, t := range a.Tasks {
					rows = append(rows, listTask{Root: s.Root, Dir: a.Dir, Adaptor: a.Kind, Task: t.Name, Description: t.Description})
				}
			}
		}
		return printJSON(rows)
	}
	for _, s := range states {
		label := projectLabel(s.Root)
		for _, a := range s.Adaptors {
			for _, t := range a.Tasks {
				fmt.Printf("%s\t%s\t%s\t%s\t%s\n", label, a.Dir, a.Kind, t.Name, t.Description)
			}
		}
	}
	return nil
}

// projectLabel is a project's display name for listings. Projects managed
// with ghq-style layouts sit at .../{user_or_org}/{repo}, so pairing the
// root's directory name with its parent's gives a recognizable label (e.g.
// "iyuuya/tsk") without printing the full path.
func projectLabel(root string) string {
	return filepath.Join(filepath.Base(filepath.Dir(root)), filepath.Base(root))
}

func cmdRun(args []string) error {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	global := fs.Bool("global", false, "target a project other than the current one")
	refresh := fs.Bool("refresh", false, "bypass the cache and re-discover tasks")
	if err := fs.Parse(args); err != nil {
		return err
	}
	rest := fs.Args()

	var root string
	if *global {
		if len(rest) == 0 {
			return errUsage
		}
		r, err := filepath.Abs(rest[0])
		if err != nil {
			return err
		}
		root = r
		rest = rest[1:]
	} else {
		r, err := task.ProjectRoot(".")
		if err != nil {
			return err
		}
		root = r
	}

	var dirArg, adaptorName, taskName string
	switch len(rest) {
	case 2:
		adaptorName, taskName = rest[0], rest[1]
	case 3:
		dirArg, adaptorName, taskName = rest[0], rest[1], rest[2]
	default:
		return errUsage
	}

	dir := root
	if dirArg != "" {
		dir = resolveDir(root, dirArg)
	}

	tasks, err := syncTasks(root, *refresh)
	if err != nil {
		return err
	}
	for _, t := range tasks {
		if t.AdaptorName() == adaptorName && t.Dir() == dir && t.Name == taskName {
			return t.Run()
		}
	}
	return fmt.Errorf("task not found: %s %s %s", dir, adaptorName, taskName)
}

// syncTasks discovers root's adaptors and returns their tasks, reusing the
// on-disk cache for any adaptor whose definition file hasn't changed.
func syncTasks(root string, refresh bool) ([]task.Task, error) {
	ds, err := discoverers()
	if err != nil {
		return nil, err
	}
	discovered, err := task.Discover(root, ds...)
	if err != nil {
		return nil, err
	}
	return state.Sync(root, discovered, refresh)
}

// resolveDir turns a dir argument into an absolute path. An absolute dir is
// used as-is; a relative one is resolved against the project root.
func resolveDir(root, dir string) string {
	if filepath.IsAbs(dir) {
		return dir
	}
	return filepath.Join(root, dir)
}

func printTasks(root string, tasks []task.Task) {
	name := filepath.Base(root)
	for _, t := range tasks {
		dir, err := filepath.Rel(root, t.Dir())
		if err != nil {
			dir = t.Dir()
		}
		fmt.Printf("%s\t%s\t%s\t%s\t%s\n", name, dir, t.AdaptorName(), t.Name, t.Description)
	}
}
