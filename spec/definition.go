// Package spec lets a task-runner adaptor be described as data instead of
// Go code: how to detect it on disk, how to list its tasks, and how to run
// one. tsk's built-in adaptors (mise, bun, pnpm, yarn, npm, rake, make) are
// all just Definition values — see the config package for the default set
// — none of them has its own .go file.
package spec

// Definition declaratively describes one adaptor kind. Its fields carry
// `toml` tags so a []Definition can be read from or written to a user's
// config file as-is (see the config package).
type Definition struct {
	// Kind identifies the adaptor, e.g. "mise". Surfaced as
	// Task.AdaptorName().
	Kind string `toml:"kind"`

	// DefinitionFiles are filenames that mark a directory as a candidate
	// for this adaptor.
	DefinitionFiles []string `toml:"definition_files"`

	// Match further narrows candidates for a DefinitionFile shared with
	// other adaptors (e.g. package.json is shared by bun/pnpm/yarn/npm).
	// A directory matches if Match is nil, or if any one of its
	// conditions holds.
	Match *MatchSpec `toml:"match,omitempty"`

	// List describes how to enumerate this adaptor's tasks in a
	// directory.
	List ListSpec `toml:"list"`

	// Run is the command used to execute a task, with the literal
	// "{{name}}" replaced by the task's name, e.g.
	// []string{"mise", "run", "{{name}}"}.
	Run []string `toml:"run"`
}

// MatchSpec is a set of signals, combined with OR, that confirm a
// DefinitionFile shared with other adaptors really belongs to this one.
// None of its conditions name a specific tool or ecosystem — the file,
// field and table names they check are entirely up to the Definition.
type MatchSpec struct {
	// Lockfiles: any of these filenames existing alongside the definition
	// file confirms the match.
	Lockfiles []string `toml:"lockfiles,omitempty"`

	// Manager confirms the match via a "name@version"-style field, e.g.
	// npm's own "packageManager" convention in package.json.
	Manager *ManagerFieldSpec `toml:"manager,omitempty"`

	// AncestorTool confirms the match via a TOML table entry in the
	// directory or an ancestor, e.g. mise's own inherited [tools]
	// declarations.
	AncestorTool *AncestorTOMLSpec `toml:"ancestor_tool,omitempty"`
}

// ManagerFieldSpec checks File (relative to the candidate directory) for a
// JSON field named Field whose value, in "name@version" form, names Name —
// the shape of npm's "packageManager": "pnpm@8.6.0" convention, but not
// tied to it: point File/Field at any similarly-shaped convention.
type ManagerFieldSpec struct {
	// File defaults to "package.json" when empty.
	File  string `toml:"file,omitempty"`
	Field string `toml:"field"`
	Name  string `toml:"name"`
}

// AncestorTOMLSpec walks a directory and its ancestors looking for a TOML
// file (one of Files) whose [Table] declares Key — the shape of tools
// like mise that inherit per-directory declarations from parent
// directories, but not tied to mise specifically.
type AncestorTOMLSpec struct {
	Files []string `toml:"files"`
	Table string   `toml:"table"`
	Key   string   `toml:"key"`
}

// ListKind selects one of tsk's built-in strategies for enumerating a
// directory's tasks.
type ListKind string

const (
	// ListJSONCommand runs Command/Args and parses stdout as a JSON array
	// of objects, taking NameField/DescriptionField off each. If
	// SourceField is set, an element is kept only when the directory part
	// of that field equals the adaptor's own directory — this is how
	// mise's task list, which includes tasks inherited from parent
	// directories, is filtered down to the ones actually defined there.
	ListJSONCommand ListKind = "json_command"

	// ListRegexCommand runs Command/Args and matches Pattern against each
	// line of stdout; capture group 1 is the task name, group 2 (when
	// present) its description.
	ListRegexCommand ListKind = "regex_command"

	// ListJSONFileMap reads File (relative to the adaptor's directory) as
	// JSON and takes ScriptsField as a map[string]string; keys become
	// task names, values their descriptions.
	ListJSONFileMap ListKind = "json_file_map"
)

// ListSpec configures the ListKind strategy picked for a Definition. Only
// the fields relevant to the chosen Kind need to be set.
type ListSpec struct {
	Kind ListKind `toml:"kind"`

	// Command/Args: for ListJSONCommand and ListRegexCommand, the command
	// run in the adaptor's directory to produce output.
	Command string   `toml:"command,omitempty"`
	Args    []string `toml:"args,omitempty"`

	// NameField/DescriptionField/SourceField: for ListJSONCommand, the
	// JSON field names read off each element of the top-level array.
	NameField        string `toml:"name_field,omitempty"`
	DescriptionField string `toml:"description_field,omitempty"`
	SourceField      string `toml:"source_field,omitempty"`

	// Pattern: for ListRegexCommand, a regexp with two capture groups —
	// name and description — applied to each line of output.
	Pattern string `toml:"pattern,omitempty"`

	// File/ScriptsField: for ListJSONFileMap, the JSON file and the field
	// on it that holds the name/description map.
	File         string `toml:"file,omitempty"`
	ScriptsField string `toml:"scripts_field,omitempty"`
}
