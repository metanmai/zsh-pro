package model

// CategorySummary is the per-category rollup for the report.
type CategorySummary struct {
	Category Category
	Count    int
	Items    []string
}

// Analysis is the complete read-only result the renderers consume.
type Analysis struct {
	Path         string
	Lines        int
	BlockCount   int
	OpaqueBlocks int
	Categories   []CategorySummary
	Issues       []Issue
	HasSecrets   bool
	Introspected bool
	Notes        []string
}

// ExitCode is 3 when actionable issues exist, else 0. Runtime (1) and usage
// (2) errors are handled by the CLI, not derived from a successful Analysis.
func (a Analysis) ExitCode() int {
	if len(a.Issues) > 0 {
		return 3
	}
	return 0
}
