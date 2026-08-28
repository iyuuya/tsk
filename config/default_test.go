package config

import "testing"

// TestDefault guards defaultTOML: it's parsed at runtime, so a syntax error
// or misspelled key there is invisible to the compiler and only shows up as
// an error from Default.
func TestDefault(t *testing.T) {
	cfg, err := Default()
	if err != nil {
		t.Fatalf("Default() = %v; defaultTOML is broken", err)
	}
	if cfg.DefaultScope != "repo" {
		t.Errorf("Default().DefaultScope = %q, want %q", cfg.DefaultScope, "repo")
	}
	defs := cfg.Adaptor

	want := []string{"mise", "bun", "pnpm", "yarn", "npm", "rake", "make"}
	if len(defs) != len(want) {
		t.Fatalf("Default() returned %d definitions, want %d", len(defs), len(want))
	}
	for i, kind := range want {
		d := defs[i]
		if d.Kind != kind {
			t.Errorf("defs[%d].Kind = %q, want %q (priority order matters)", i, d.Kind, kind)
		}
		if len(d.DefinitionFiles) == 0 {
			t.Errorf("defs[%d] (%s): no definition_files", i, d.Kind)
		}
		if d.List.Kind == "" {
			t.Errorf("defs[%d] (%s): list.kind is empty", i, d.Kind)
		}
		if len(d.Run) == 0 {
			t.Errorf("defs[%d] (%s): no run command", i, d.Kind)
		}
	}
}
