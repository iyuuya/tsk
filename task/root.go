package task

import (
	"fmt"
	"os/exec"
	"strings"
)

// ProjectRoot finds the project root starting from dir by asking git for
// its working tree's top-level directory. This is the base Discover should
// walk from, so adaptors are found across the whole project rather than
// only below the current working directory.
func ProjectRoot(dir string) (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git rev-parse --show-toplevel: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}
