package spec

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// hasLockfile reports whether dir contains any of the given filenames.
func hasLockfile(dir string, names ...string) bool {
	for _, n := range names {
		if _, err := os.Stat(filepath.Join(dir, n)); err == nil {
			return true
		}
	}
	return false
}

// declaresManager reports whether file (relative to dir) has a JSON field
// named field whose value, split on "@", names manager.
func declaresManager(dir, file, field, manager string) bool {
	data, err := os.ReadFile(filepath.Join(dir, file))
	if err != nil {
		return false
	}
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(data, &doc); err != nil {
		return false
	}
	raw, ok := doc[field]
	if !ok {
		return false
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return false
	}
	name, _, ok := strings.Cut(value, "@")
	return ok && name == manager
}

// ancestorTOMLHasKey walks dir and its ancestors looking for a TOML file
// (one of files) whose [table] declares key.
func ancestorTOMLHasKey(dir string, files []string, table, key string) bool {
	for {
		for _, f := range files {
			var doc map[string]any
			if _, err := toml.DecodeFile(filepath.Join(dir, f), &doc); err != nil {
				continue
			}
			if t, ok := doc[table].(map[string]any); ok {
				if _, ok := t[key]; ok {
					return true
				}
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return false
		}
		dir = parent
	}
}
