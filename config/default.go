// Package config holds tsk's configuration: the built-in defaults
// (Default), and loading/saving a user's own config to/from a TOML file
// (Load/Save, see config.go).
package config

import (
	"fmt"

	"github.com/BurntSushi/toml"
)

// defaultTOML is tsk's built-in configuration, written in the exact same
// TOML shape as the user's config file so Default can parse it through the
// same path Load uses for that file (and so it can serve as a reference for
// what a hand-written config looks like).
//
// The adaptors appear in priority order: bun, pnpm, yarn and npm all key
// off package.json, so where a directory could match more than one of
// them, task.Discover picks whichever comes first here (see spec.Discoverer
// and task.Discover).
const defaultTOML = `
# The scope list/run use when --scope isn't given: "global" (every project
# tsk has cached state for), "repo" (the enclosing git repository) or "dir"
# (the current directory and below).
default_scope = "repo"

[[adaptor]]
kind = "mise"
definition_files = ["mise.toml", ".mise.toml"]
run = ["mise", "run", "{{name}}"]

[adaptor.list]
kind = "json_command"
command = "mise"
args = ["tasks", "ls", "--json"]
name_field = "name"
description_field = "description"
source_field = "source"

[[adaptor]]
kind = "bun"
definition_files = ["package.json"]
run = ["bun", "run", "{{name}}"]

[adaptor.match]
lockfiles = ["bun.lock", "bun.lockb"]

[adaptor.match.manager]
field = "packageManager"
name = "bun"

# mise's [tools] declarations are inherited from ancestor directories, so
# each ancestor_tool check names the same files mise itself reads.
[adaptor.match.ancestor_tool]
files = ["mise.toml", ".mise.toml"]
table = "tools"
key = "bun"

[adaptor.list]
kind = "json_file_map"
file = "package.json"
scripts_field = "scripts"

[[adaptor]]
kind = "pnpm"
definition_files = ["package.json"]
run = ["pnpm", "run", "{{name}}"]

[adaptor.match]
lockfiles = ["pnpm-lock.yaml"]

[adaptor.match.manager]
field = "packageManager"
name = "pnpm"

[adaptor.match.ancestor_tool]
files = ["mise.toml", ".mise.toml"]
table = "tools"
key = "pnpm"

[adaptor.list]
kind = "json_file_map"
file = "package.json"
scripts_field = "scripts"

[[adaptor]]
kind = "yarn"
definition_files = ["package.json"]
run = ["yarn", "run", "{{name}}"]

[adaptor.match]
lockfiles = ["yarn.lock"]

[adaptor.match.manager]
field = "packageManager"
name = "yarn"

[adaptor.match.ancestor_tool]
files = ["mise.toml", ".mise.toml"]
table = "tools"
key = "yarn"

[adaptor.list]
kind = "json_file_map"
file = "package.json"
scripts_field = "scripts"

# npm has no match table: it's the fallback package manager for a
# package.json that none of bun/pnpm/yarn claimed first.
[[adaptor]]
kind = "npm"
definition_files = ["package.json"]
run = ["npm", "run", "{{name}}"]

[adaptor.list]
kind = "json_file_map"
file = "package.json"
scripts_field = "scripts"

# uv has no task runner of its own; "uv run" runs the entry points a
# project declares in pyproject.toml's [project.scripts].
[[adaptor]]
kind = "uv"
definition_files = ["pyproject.toml"]
run = ["uv", "run", "{{name}}"]

[adaptor.match]
lockfiles = ["uv.lock"]

[adaptor.match.ancestor_tool]
files = ["mise.toml", ".mise.toml"]
table = "tools"
key = "uv"

[adaptor.list]
kind = "toml_file_map"
file = "pyproject.toml"
scripts_field = "project.scripts"

[[adaptor]]
kind = "rake"
definition_files = ["Rakefile", "rakefile"]
run = ["rake", "{{name}}"]

[adaptor.list]
kind = "regex_command"
command = "rake"
args = ["-T", "-A"]
pattern = '^rake\s+(\S+)(?:\[[^\]]*\])?(?:\s+#\s+(.*))?$'

# make itself has no subcommand for listing targets (unlike mise/rake), so
# listing shells out to a small pipeline instead: pick whichever of
# definition_files exists, drop tab-indented recipe lines and anything that
# isn't a plain "name: prereqs" line (variable assignments, .PHONY-style
# and %-pattern rules), then hand the survivors to the name/description
# regex in pattern.
[[adaptor]]
kind = "make"
definition_files = ["Makefile", "makefile", "GNUmakefile"]
run = ["make", "{{name}}"]

[adaptor.list]
kind = "regex_command"
command = "sh"
args = [
  "-c",
  "{ cat Makefile 2>/dev/null || cat makefile 2>/dev/null || cat GNUmakefile 2>/dev/null; } | grep -v '^\t' | grep -E '^[^.%[:space:]][^:%]*:([^=]|$)'",
]
pattern = '^([^:\s]+):(?:.*?#+\s*(.*))?$'
`

// Default returns tsk's built-in configuration by parsing defaultTOML the
// same way Load parses the user's config file. defaultTOML is a
// compile-time constant, so a returned error means a bug in the string
// itself (config.TestDefault guards against that).
func Default() (Config, error) {
	var cfg Config
	if err := toml.Unmarshal([]byte(defaultTOML), &cfg); err != nil {
		return Config{}, fmt.Errorf("config: built-in default TOML is invalid: %w", err)
	}
	return cfg, nil
}
