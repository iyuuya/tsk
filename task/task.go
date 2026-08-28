package task

// Task represents a single runnable task exposed by an Adaptor instance.
type Task struct {
	Name        string
	Description string
	Adaptor     Adaptor
}

// AdaptorName is the kind of adaptor this task came from, e.g. "mise".
func (t Task) AdaptorName() string {
	return t.Adaptor.TaskDefinition()
}

// Dir is the directory this task runs in.
func (t Task) Dir() string {
	return t.Adaptor.Dir()
}

// Run executes the task via its owning Adaptor.
func (t Task) Run() error {
	return t.Adaptor.Run(t.Name)
}
