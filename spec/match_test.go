package spec

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestHasLockfile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "bun.lock"), "")

	if !hasLockfile(dir, "bun.lockb", "bun.lock") {
		t.Error("hasLockfile = false, want true when one of the names exists")
	}
	if hasLockfile(dir, "pnpm-lock.yaml") {
		t.Error("hasLockfile = true for a filename that doesn't exist")
	}
	if hasLockfile(dir) {
		t.Error("hasLockfile = true with no names")
	}
}

func TestDeclaresManager(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "package.json"),
		`{"name": "x", "packageManager": "pnpm@8.6.0"}`)

	tests := []struct {
		name    string
		file    string
		field   string
		manager string
		want    bool
	}{
		{"matching manager", "package.json", "packageManager", "pnpm", true},
		{"different manager", "package.json", "packageManager", "yarn", false},
		{"missing field", "package.json", "engines", "pnpm", false},
		{"missing file", "nope.json", "packageManager", "pnpm", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := declaresManager(dir, tt.file, tt.field, tt.manager); got != tt.want {
				t.Errorf("declaresManager(%q, %q, %q) = %v, want %v",
					tt.file, tt.field, tt.manager, got, tt.want)
			}
		})
	}

	t.Run("value without @version", func(t *testing.T) {
		d := t.TempDir()
		writeFile(t, filepath.Join(d, "package.json"), `{"packageManager": "pnpm"}`)
		if declaresManager(d, "package.json", "packageManager", "pnpm") {
			t.Error("declaresManager = true for a value with no @version part")
		}
	})

	t.Run("field is not a string", func(t *testing.T) {
		d := t.TempDir()
		writeFile(t, filepath.Join(d, "package.json"), `{"packageManager": 42}`)
		if declaresManager(d, "package.json", "packageManager", "pnpm") {
			t.Error("declaresManager = true for a non-string field")
		}
	})

	t.Run("invalid JSON", func(t *testing.T) {
		d := t.TempDir()
		writeFile(t, filepath.Join(d, "package.json"), `{not json`)
		if declaresManager(d, "package.json", "packageManager", "pnpm") {
			t.Error("declaresManager = true for invalid JSON")
		}
	})
}

func TestAncestorTOMLHasKey(t *testing.T) {
	// Use a filename unlikely to exist in ancestors of TempDir, since the
	// walk continues up to the filesystem root.
	const tomlName = "tsk-test-tools.toml"

	root := t.TempDir()
	child := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(child, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, tomlName), "[tools]\ngo = \"latest\"\n")

	if !ancestorTOMLHasKey(root, []string{tomlName}, "tools", "go") {
		t.Error("key in the directory itself not found")
	}
	if !ancestorTOMLHasKey(child, []string{tomlName}, "tools", "go") {
		t.Error("key in an ancestor directory not found")
	}
	if ancestorTOMLHasKey(child, []string{tomlName}, "tools", "ruby") {
		t.Error("found a key the table doesn't declare")
	}
	if ancestorTOMLHasKey(child, []string{tomlName}, "env", "go") {
		t.Error("found a key in a table that doesn't exist")
	}
	if ancestorTOMLHasKey(child, []string{"tsk-test-absent.toml"}, "tools", "go") {
		t.Error("found a key in a file that doesn't exist")
	}
}
