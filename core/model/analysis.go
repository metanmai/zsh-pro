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

// HasActionableIssues reports whether any issue is actionable. It is the single
// source of truth for both the exit code and the envelope's issues_found flag,
// so the two can never disagree: an advisory-only analysis is clean (exit 0,
// issues_found:false), while any actionable issue makes it actionable.
func (a Analysis) HasActionableIssues() bool {
	for _, is := range a.Issues {
		if is.Severity == SevActionable {
			return true
		}
	}
	return false
}

// ExitCode is 3 when actionable issues exist, else 0. Runtime (1) and usage
// (2) errors are handled by the CLI, not derived from a successful Analysis.
// It consults HasActionableIssues so advisories never bump the exit code.
func (a Analysis) ExitCode() ExitCode {
	if a.HasActionableIssues() {
		return ExitActionable
	}
	return ExitClean
}
