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
	Kind  IssueKind `json:"kind"`
	Name  string    `json:"name"`
	Lines []int     `json:"lines,omitempty"`
	Note  string    `json:"note,omitempty"`
}
