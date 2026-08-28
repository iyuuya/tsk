package task

// Adaptor wraps a task runner (mise, npm, make, rake, ...) and exposes its
// tasks through a common interface.
//
// An Adaptor is not a singleton: in a monorepo the same task runner can show
// up in several directories (e.g. a package.json at the repo root and
// another under ./hoge), each becoming its own Adaptor instance rooted at
// that directory.
type Adaptor interface {
	// TaskDefinition identifies the adaptor kind, e.g. "mise".
	TaskDefinition() string
	// Dir is the directory this adaptor instance operates in.
	Dir() string
	// List returns the tasks the adaptor knows about in Dir.
	List() ([]Task, error)
	// Run executes the named task in Dir.
	Run(name string) error
}

// Discoverer knows how to detect an Adaptor's presence on disk and create
// instances of it.
type Discoverer interface {
	// DefinitionFiles are filenames that mark a directory as owned by this
	// adaptor, e.g. ["mise.toml", ".mise.toml"].
	DefinitionFiles() []string
	// New creates an Adaptor instance rooted at dir.
	New(dir string) Adaptor
}

// Matcher is an optional refinement a Discoverer can implement. Some
// definition files (e.g. package.json) don't uniquely identify an adaptor
// on their own, so when one is found, Discover calls Matches to confirm
// the directory actually belongs to this adaptor before creating an
// instance for it.
type Matcher interface {
	Matches(dir string) bool
}
