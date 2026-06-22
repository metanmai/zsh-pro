package analyze

import (
	"strings"

	"zsh-pro/core/model"
)

// primaryName is the short label used for a block in a category rollup. It
// prefers the block's first declared name (alias/var/function); for blocks
// without names (commands, compounds) it falls back to a truncated first line
// of the source text so the summary still says something useful.
func primaryName(b model.Block) string {
	if len(b.Names) > 0 {
		return b.Names[0]
	}
	first := strings.TrimSpace(strings.SplitN(b.Text, "\n", 2)[0])
	const max = 48
	if len(first) > max {
		return first[:max] + "…"
	}
	return first
}
