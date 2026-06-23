package model

// IssueKind enumerates the problems the engine reports.
type IssueKind string

const (
	IssueDuplicateAlias IssueKind = "duplicate_alias"
	IssueReassignedEnv  IssueKind = "reassigned_env"
	IssueDuplicatePath  IssueKind = "duplicate_path"
	IssueShadowed       IssueKind = "shadowed"
)

// Issue is one reported problem.
type Issue struct {
	Kind  IssueKind
	Name  string
	Lines []int
	Note  string
}
