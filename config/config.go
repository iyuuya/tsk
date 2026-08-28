package config

import (
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"

	"github.com/iyuuya/tsk/spec"
)

// Config is the parsed shape of tsk's TOML config file: top-level settings
// followed by an array of tables named "adaptor", each unmarshaling
// straight into a spec.Definition.
type Config struct {
	// DefaultScope is the scope `tsk list`/`tsk run` use when --scope isn't
	// given: "global", "repo" or "dir". Empty means repo. The CLI validates
	// the value; config only carries it.
	DefaultScope string `toml:"default_scope,omitempty"`
	// Adaptor is the adaptor definitions, in priority order.
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

// Load returns the user's configuration from the config file. It falls
// back to Default when the file doesn't exist.
func Load() (Config, error) {
	path, err := Path()
	if err != nil {
		return Config{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Default()
		}
		return Config{}, err
	}
	var cfg Config
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// Save writes cfg to the config file, replacing any existing one.
func Save(cfg Config) error {
	path, err := Path()
	if err != nil {
		return err
	}
	data, err := toml.Marshal(cfg)
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
