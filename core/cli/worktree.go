package cli

import (
	"io"
)

// runWorktree is introduced as a compile-safe RED seam. The closed parser and
// value-safe renderer are implemented in the GREEN commit.
func (c *CLI) runWorktree(_ []string, stdout, stderr io.Writer) int {
	return c.fail(stdout, stderr, false, "worktree workflow unavailable")
}
