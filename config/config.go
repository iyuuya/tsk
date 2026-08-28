package config

import (
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"

	"github.com/iyuuya/tsk/spec"
)

// document is the TOML shape of tsk's config file: an array of tables named
// "adaptor", each unmarshaling straight into a spec.Definition.
type document struct {
	Adaptor []spec.Definition `toml:"adaptor"`
}

// Dir returns tsk's config directory (e.g. ~/.config/tsk, honoring
// $XDG_CONFIG_HOME), creating it if necessary.
func Dir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, "tsk")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

// Path returns the path to tsk's config file.
func Path() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.toml"), nil
}

// Load returns the user's adaptor definitions from the config file. It
// falls back to Default when the file doesn't exist.
func Load() ([]spec.Definition, error) {
	path, err := Path()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Default()
		}
		return nil, err
	}
	var doc document
	if err := toml.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	return doc.Adaptor, nil
}

// Save writes defs to the config file, replacing any existing one.
func Save(defs []spec.Definition) error {
	path, err := Path()
	if err != nil {
		return err
	}
	data, err := toml.Marshal(document{Adaptor: defs})
	if err != nil {
		return err
	}
	// Write to a temp file and rename so a crash mid-write can't leave a
	// corrupt config behind.
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Exists reports whether a config file is already present.
func Exists() (bool, error) {
	path, err := Path()
	if err != nil {
		return false, err
	}
	_, err = os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}
