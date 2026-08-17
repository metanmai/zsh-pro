package model

import "errors"

const (
	WorktreeSchemaV1        = "v1"
	MaxSnapshotBytes        = 2 * 1024 * 1024
	MaxSnapshotRecords      = 10_000
	MaxResolutionTokenBytes = 128
)

// LiveKind is one supported shell-neutral live-state category.
type LiveKind string

const (
	LiveEnv      LiveKind = "env"
	LiveAlias    LiveKind = "alias"
	LiveFunction LiveKind = "function"
	LivePath     LiveKind = "path"
	LiveFPath    LiveKind = "fpath"
	LiveOption   LiveKind = "option"
)

// Identity is the exact category/name key for one managed shell value.
type Identity struct {
	Kind LiveKind
	Name string
}

// LiveValue preserves presence separately from its kind-selected payload.
type LiveValue struct {
	Present bool
	Scalar  *string
	List    []string
	Option  *bool
}

// LiveIdentityState is one observed final value.
type LiveIdentityState struct {
	Identity Identity
	Value    LiveValue
}

// LiveSnapshot is one bounded decoded capture. ByteSize is the complete
// encoded frame size observed by the decoder.
type LiveSnapshot struct {
	ByteSize uint64
	States   []LiveIdentityState
}

type LiveChangeKind string

const (
	LiveAdd    LiveChangeKind = "add"
	LiveUpdate LiveChangeKind = "update"
	LiveRemove LiveChangeKind = "remove"
)

// LiveChange carries the final value for adds/updates and no value for removes.
type LiveChange struct {
	Kind     LiveChangeKind
	Identity Identity
	Value    LiveValue
}

// OverlayEntry is an explicit final value or tombstone.
type OverlayEntry struct {
	Identity  Identity
	Value     LiveValue
	Tombstone bool
}

// LiveProjection is the versioned committed final-state projection.
type LiveProjection struct {
	Schema     string
	States     []LiveIdentityState
	Tombstones []Identity
}

// CommittedWorktree keeps the complete source IR separate from final state.
type CommittedWorktree struct {
	Schema     string
	Source     Profile
	Projection LiveProjection
}

// ResolutionToken binds one pending apply/acknowledge transition.
type ResolutionToken string

type ConflictKind string

const (
	ConflictOverlap    ConflictKind = "overlap"
	ConflictHistoryGap ConflictKind = "history-gap"
)

// RevisionEvent is a value-free causal record.
type RevisionEvent struct {
	Revision uint64
	Changed  []Identity
}

// Conflict deliberately records identity and causality but no captured value.
type Conflict struct {
	Kind           ConflictKind
	Identity       Identity
	BaseRevision   uint64
	SharedRevision uint64
	Token          ResolutionToken
}

// Exclusion is a value-free admission decision.
type Exclusion struct {
	Identity Identity
	Reason   string
}

type AttachRequest struct {
	OperationID string
	ShellID     string
	Initial     LiveSnapshot
}

type AttachResult struct {
	Revision   uint64
	Attached   bool
	Exclusions []Exclusion
}

type PublishRequest struct {
	OperationID          string
	ShellID              string
	AcknowledgedRevision uint64
	Delta                []LiveChange
}

type PublishResult struct {
	SharedRevision uint64
	Accepted       []Identity
	Conflicts      []Conflict
}

type PreparePullRequest struct {
	OperationID     string
	ShellID         string
	AppliedRevision uint64
}

type PreparePullResult struct {
	PendingRevision uint64
	Token           ResolutionToken
	Changes         []LiveChange
}

type PullRequest = PreparePullRequest
type PullResult = PreparePullResult

type AcknowledgeRequest struct {
	OperationID string
	ShellID     string
	Revision    uint64
	Token       ResolutionToken
	Snapshot    LiveSnapshot
}

type AcknowledgeResult struct {
	AppliedRevision uint64
	Acknowledged    bool
}

type ResolveSharedRequest struct {
	OperationID string
	ShellID     string
	Conflict    Identity
	Token       ResolutionToken
	Snapshot    LiveSnapshot
}

type ResolveSharedResult struct {
	PendingRevision uint64
	Token           ResolutionToken
	Changes         []LiveChange
}

// DiffEntry is the value-free public form of one semantic change.
type DiffEntry struct {
	Kind     LiveChangeKind
	Identity Identity
}

type CategorizedDiff struct {
	Environment []DiffEntry
	Aliases     []DiffEntry
	Functions   []DiffEntry
	Path        []DiffEntry
	FPath       []DiffEntry
	Options     []DiffEntry
}

type ShellWorktreeStatus struct {
	Attached        bool
	AppliedRevision uint64
	Behind          bool
	AutoApply       bool
	ConflictCount   int
}

type WorktreeStatus struct {
	Branch        string
	BaseOID       string
	Revision      uint64
	DirtyCount    int
	ConflictCount int
	Shell         *ShellWorktreeStatus
}

type WorktreeCommitResult struct {
	Committed        bool
	OID              string
	Conflict         bool
	RecoveryRequired bool
}

func ScalarLiveValue(value string) LiveValue {
	return LiveValue{Present: true, Scalar: &value}
}

func ListLiveValue(value []string) LiveValue {
	return LiveValue{Present: true, List: append([]string(nil), value...)}
}

func OptionLiveValue(value bool) LiveValue {
	return LiveValue{Present: true, Option: &value}
}

func RemovedLiveValue() LiveValue { return LiveValue{} }

func CloneLiveValue(value LiveValue) LiveValue {
	out := value
	if value.Scalar != nil {
		scalar := *value.Scalar
		out.Scalar = &scalar
	}
	if value.List != nil {
		out.List = append([]string(nil), value.List...)
	}
	if value.Option != nil {
		option := *value.Option
		out.Option = &option
	}
	return out
}

func ValidateIdentity(Identity) error { return errors.New("live identity validation not implemented") }

func ValidateLiveIdentityState(LiveIdentityState) error {
	return errors.New("live state validation not implemented")
}

func ValidateLiveSnapshot(LiveSnapshot) error {
	return errors.New("live snapshot validation not implemented")
}

func EqualLiveValue(LiveKind, LiveValue, LiveValue) bool { return false }

func NormalizeLiveStates([]LiveIdentityState) ([]LiveIdentityState, error) {
	return nil, errors.New("live state normalization not implemented")
}

func NormalizeOverlay([]OverlayEntry) ([]OverlayEntry, error) {
	return nil, errors.New("live overlay normalization not implemented")
}

func NextRevision(revision uint64) (uint64, error) {
	return revision, errors.New("revision transition not implemented")
}

func NewCommittedWorktree(Profile, LiveProjection) CommittedWorktree { return CommittedWorktree{} }

func (token ResolutionToken) Validate() error {
	return errors.New("resolution token validation not implemented")
}
