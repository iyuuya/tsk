// Package state persists each project's discovered tasks under
// $XDG_STATE_HOME/tsk (~/.local/state/tsk by default), one file per project
// root, so tsk doesn't have to re-invoke every adaptor's CLI on every
// invocation.
package state

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// CachedTask is a task's persisted name and description.
type CachedTask struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// CachedAdaptor is one Adaptor instance's cached task list, fingerprinted by
// its definition file's modification time at the moment it was cached.
type CachedAdaptor struct {
	Kind           string       `json:"kind"`
	Dir            string       `json:"dir"`             // relative to Root
	DefinitionFile string       `json:"definition_file"` // relative to Root
	ModTime        time.Time    `json:"mod_time"`
	Tasks          []CachedTask `json:"tasks"`
}

// State is one project root's cached task inventory.
type State struct {
	Root      string          `json:"root"`
	UpdatedAt time.Time       `json:"updated_at"`
	Adaptors  []CachedAdaptor `json:"adaptors"`
}

// Dir returns tsk's state directory, creating it if necessary.
func Dir() (string, error) {
	base := os.Getenv("XDG_STATE_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(home, ".local", "state")
	}
	dir := filepath.Join(base, "tsk")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

// pathForRoot maps a project root to its cache file, named by the hash of
// its absolute path so arbitrary root paths are always valid filenames.
func pathForRoot(root string) (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(root))
	return filepath.Join(dir, hex.EncodeToString(sum[:])+".json"), nil
}

// Load reads the cached State for root. It returns (nil, nil) if root has
// no cache yet.
func Load(root string) (*State, error) {
	path, err := pathForRoot(root)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// Save writes s to disk, replacing any existing cache for s.Root.
func Save(s *State) error {
	path, err := pathForRoot(s.Root)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	// Write to a temp file and rename so a crash mid-write can't leave a
	// corrupt cache behind.
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// LoadAll reads every project's cached State, for cross-project listing.
func LoadAll() ([]*State, error) {
	dir, err := Dir()
	if err != nil {
		return nil, err
	}
	matches, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil, err
	}
	states := make([]*State, 0, len(matches))
	for _, m := range matches {
		data, err := os.ReadFile(m)
		if err != nil {
			continue
		}
		var s State
		if err := json.Unmarshal(data, &s); err != nil {
			continue
		}
		states = append(states, &s)
	}
	return states, nil
}
