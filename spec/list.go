package spec

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/iyuuya/tsk/task"
)

// commandOutput runs def.List.Command/Args in Dir and returns its stdout,
// for the command-based list strategies.
func (a *instance) commandOutput() ([]byte, error) {
	cmd := exec.Command(a.def.List.Command, a.def.List.Args...)
	cmd.Dir = a.dir
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", strings.Join(append([]string{a.def.List.Command}, a.def.List.Args...), " "), err)
	}
	return out, nil
}

// listJSONCommand implements ListJSONCommand.
func (a *instance) listJSONCommand() ([]task.Task, error) {
	out, err := a.commandOutput()
	if err != nil {
		return nil, err
	}

	var raw []map[string]any
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("parse output: %w", err)
	}

	absDir, err := filepath.Abs(a.dir)
	if err != nil {
		return nil, fmt.Errorf("resolve dir: %w", err)
	}

	var tasks []task.Task
	for _, obj := range raw {
		if a.def.List.SourceField != "" {
			src, _ := obj[a.def.List.SourceField].(string)
			if filepath.Dir(src) != absDir {
				continue
			}
		}
		name, _ := obj[a.def.List.NameField].(string)
		description, _ := obj[a.def.List.DescriptionField].(string)
		tasks = append(tasks, task.Task{Name: name, Description: description, Adaptor: a})
	}
	return tasks, nil
}

// listRegexCommand implements ListRegexCommand.
func (a *instance) listRegexCommand() ([]task.Task, error) {
	re, err := regexp.Compile(a.def.List.Pattern)
	if err != nil {
		return nil, fmt.Errorf("compile pattern: %w", err)
	}

	out, err := a.commandOutput()
	if err != nil {
		return nil, err
	}

	var tasks []task.Task
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		m := re.FindStringSubmatch(scanner.Text())
		if m == nil {
			continue
		}
		description := ""
		if len(m) > 2 {
			description = m[2]
		}
		tasks = append(tasks, task.Task{Name: m[1], Description: description, Adaptor: a})
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("parse output: %w", err)
	}
	return tasks, nil
}

// listJSONFileMap implements ListJSONFileMap.
func (a *instance) listJSONFileMap() ([]task.Task, error) {
	data, err := os.ReadFile(filepath.Join(a.dir, a.def.List.File))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", a.def.List.File, err)
	}

	var doc map[string]json.RawMessage
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse %s: %w", a.def.List.File, err)
	}

	scripts := map[string]string{}
	if raw, ok := doc[a.def.List.ScriptsField]; ok {
		if err := json.Unmarshal(raw, &scripts); err != nil {
			return nil, fmt.Errorf("parse %s.%s: %w", a.def.List.File, a.def.List.ScriptsField, err)
		}
	}

	names := make([]string, 0, len(scripts))
	for name := range scripts {
		names = append(names, name)
	}
	sort.Strings(names)

	tasks := make([]task.Task, 0, len(names))
	for _, name := range names {
		tasks = append(tasks, task.Task{Name: name, Description: scripts[name], Adaptor: a})
	}
	return tasks, nil
}
