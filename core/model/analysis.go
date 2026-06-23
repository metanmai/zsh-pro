package model

// CategorySummary is the per-category rollup for the report.
type CategorySummary struct {
	Category Category `json:"category"`
	Count    int      `json:"count"`
	Items    []string `json:"items,omitempty"`
}

// Analysis is the complete read-only result the renderers consume.
type Analysis struct {
	Path         string            `json:"path"`
	Lines        int               `json:"lines"`
	BlockCount   int               `json:"blocks"`
	OpaqueBlocks int               `json:"opaque_blocks"`
	Categories   []CategorySummary `json:"categories"`
	Issues       []Issue           `json:"issues"`
	HasSecrets   bool              `json:"has_secrets"`
	Introspected bool              `json:"introspected"`
	Notes        []string          `json:"notes,omitempty"`
}

// ExitCode is 3 when actionable issues exist, else 0. Runtime (1) and usage
// (2) errors are handled by the CLI, not derived from a successful Analysis.
func (a Analysis) ExitCode() int {
	if len(a.Issues) > 0 {
		return 3
	}
	return 0
}
