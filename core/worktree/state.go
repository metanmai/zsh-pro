package worktree

import (
	"errors"

	"zsh-pro/core/model"
)

const (
	StateSchemaVersion           = 1
	MaxStateEvents               = 128
	MaxShellRecords              = 128
	MaxOperationReceiptsPerShell = 64
	MaxStateBytes                = 16 * 1024 * 1024
)

var errStateNotImplemented = errors.New("worktree state contract is not implemented")

type AttachState string

const (
	AttachStateAtHead         AttachState = "at-head"
	AttachStateCleanReconcile AttachState = "clean-reconcile"
)

type ShellTransition string

const (
	TransitionAttach       ShellTransition = "attach"
	TransitionCapture      ShellTransition = "capture"
	TransitionUnpublished  ShellTransition = "unpublished"
	TransitionPublish      ShellTransition = "publish"
	TransitionPrepare      ShellTransition = "prepare"
	TransitionResolve      ShellTransition = "resolve"
	TransitionApply        ShellTransition = "apply"
	TransitionFreshCapture ShellTransition = "fresh-capture"
	TransitionAcknowledge  ShellTransition = "acknowledge"
)

type PendingKind string

const (
	PendingNone       PendingKind = ""
	PendingPull       PendingKind = "pull"
	PendingResolution PendingKind = "resolution"
)

type ReceiptKind string

const (
	ReceiptAttach      ReceiptKind = "attach"
	ReceiptPublish     ReceiptKind = "publish"
	ReceiptPreparePull ReceiptKind = "prepare-pull"
	ReceiptAcknowledge ReceiptKind = "acknowledge"
	ReceiptResolve     ReceiptKind = "resolve-shared"
)

type SnapshotFingerprint [32]byte

type StateEvent struct {
	Revision    uint64
	OperationID string
	ShellID     string
	Changes     []model.LiveChange
}

type PendingTransition struct {
	Kind        PendingKind
	Revision    uint64
	Fingerprint SnapshotFingerprint
	Token       model.ResolutionToken
	Conflict    *model.Identity
	Changes     []model.LiveChange
}

type RecoveredErrorCode string

const (
	RecoveredLockUnavailable RecoveredErrorCode = "lock-unavailable"
	RecoveredMalformedState  RecoveredErrorCode = "malformed-state"
	RecoveredPersistence     RecoveredErrorCode = "persistence-failed"
	RecoveredApply           RecoveredErrorCode = "apply-failed"
)

type ShellState struct {
	CapabilityVerifier model.ShellCapabilityVerifier
	AttachState        AttachState
	AttachedRevision   uint64
	AppliedRevision    uint64
	AppliedBaseline    []model.LiveIdentityState
	CaptureBaseline    []model.LiveIdentityState
	UnpublishedDelta   []model.LiveChange
	PresentAtAttach    []model.Identity
	Exclusions         []model.Exclusion
	Transition         ShellTransition
	Pending            PendingTransition
	Behind             bool
	AutoApplyOverride  *bool
	Conflict           *model.Conflict
	LastRecoveredError RecoveredErrorCode
	LastActiveRevision uint64
}

type OperationReceipt struct {
	ShellID            string
	OperationID        string
	Kind               ReceiptKind
	RequestFingerprint SnapshotFingerprint
	Attach             *model.AttachResult
	Publish            *model.PublishResult
	PreparePull        *model.PreparePullResult
	Acknowledge        *model.AcknowledgeResult
	Resolve            *model.ResolveSharedResult
}

type State struct {
	SchemaVersion      int
	Materialized       bool
	Branch             string
	BaseOID            string
	Committed          model.CommittedWorktree
	HeadRevision       uint64
	AutoApplyDefault   bool
	Shared             []model.LiveIdentityState
	CompactionFloor    uint64
	CompactionBaseline []model.LiveIdentityState
	Events             []StateEvent
	Shells             map[string]ShellState
	OperationReceipts  []OperationReceipt
}

func MarshalState(State) ([]byte, error) { return nil, errStateNotImplemented }

func UnmarshalState([]byte) (State, error) { return State{}, errStateNotImplemented }

func ValidateState(State) error { return errStateNotImplemented }

func CompactStateHistory(*State) error { return errStateNotImplemented }

func FingerprintSnapshot(model.LiveSnapshot) (SnapshotFingerprint, error) {
	return SnapshotFingerprint{}, errStateNotImplemented
}

func IsLegalShellTransition(ShellTransition, ShellTransition) bool { return false }

func CanGarbageCollectShell(State, string) bool { return false }
