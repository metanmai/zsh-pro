package model

// IssueKind enumerates the problems the engine reports.
type IssueKind string

const (
	IssueDuplicateAlias IssueKind = "duplicate_alias"
	IssueReassignedEnv  IssueKind = "reassigned_env"
	IssueDuplicatePath  IssueKind = "duplicate_path"
	IssueShadowed       IssueKind = "shadowed"
)

// Severity ranks an issue. SevActionable is the zero value: an Issue literal
// that omits Severity is actionable, so the four existing issue kinds keep
// their exit-3 behavior with no construction-site edits. Only an advisory
// opts in. The enum stays extensible without breaking that zero-value contract.
type Severity int

const (
	SevActionable Severity = iota // genuine problem; drives exit 3 and issues_found
	SevAdvisory                   // informational; never bumps the exit code
)

// String is the self-describing wire/report label ("actionable" | "advisory").
// It mirrors Category.Description() — a model type owning its string projection
// so both renderers (and toDTO) share one mapping and core/dto stays string-only.
// The default arm deliberately covers SevActionable and any future-unset value,
// so an un-handled severity degrades to the safe, exit-bumping label.
func (s Severity) String() string {
	switch s {
	case SevAdvisory:
		return "advisory"
	default: // SevActionable (and any future-unset value) reads as actionable
		return "actionable"
	}
}

// Issue is one reported problem.
type Issue struct {
	Kind     IssueKind
	Name     string
	Lines    []int
	Note     string
	Severity Severity // zero value (SevActionable) for the four existing kinds
}
