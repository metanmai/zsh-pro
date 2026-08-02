package model

import (
	"crypto/rand"
	"encoding/hex"
)

// InstallInitializationID is an opaque Store-issued initialization authority.
type InstallInitializationID struct {
	token string
}

// NewInstallInitializationID creates a new opaque initialization identifier.
func NewInstallInitializationID() (InstallInitializationID, error) {
	token, err := newOpaqueTransactionToken()
	if err != nil {
		return InstallInitializationID{}, err
	}
	return InstallInitializationID{token: token}, nil
}

// IsZero reports whether no authority was assigned.
func (id InstallInitializationID) IsZero() bool { return id.token == "" }

// IngestTransactionID is an opaque transaction token nested under one
// InstallInitializationID.
type IngestTransactionID struct {
	token string
}

// NewIngestTransactionID creates a new opaque transaction identifier.
func NewIngestTransactionID() (IngestTransactionID, error) {
	token, err := newOpaqueTransactionToken()
	if err != nil {
		return IngestTransactionID{}, err
	}
	return IngestTransactionID{token: token}, nil
}

// IsZero reports whether no transaction was assigned.
func (id IngestTransactionID) IsZero() bool { return id.token == "" }

type errIngestModel string

func (e errIngestModel) Error() string { return string(e) }

const errInvalidIngestEvidence errIngestModel = "invalid ingest transaction evidence"

// NewExpectedRevision validates and copies one Git object identifier.
func NewExpectedRevision(value string) (*string, error) {
	if !validGitObjectID(value) {
		return nil, errInvalidIngestEvidence
	}
	copy := value
	return &copy, nil
}

// IngestBaseline is the exact main revision and Profile observed by Begin.
type IngestBaseline struct {
	InitializationID     InstallInitializationID
	TransactionID        IngestTransactionID
	ExpectedRevision     *string
	RefPresent           bool
	Profile              Profile
	ProfileObjectPresent bool
}

// NewIngestBaseline constructs a main-only baseline after checking revision
// presence invariants.
func NewIngestBaseline(
	initializationID InstallInitializationID,
	transactionID IngestTransactionID,
	refPresent bool,
	expectedRevision *string,
	profile Profile,
	profileObjectPresent bool,
) (IngestBaseline, error) {
	baseline := IngestBaseline{
		InitializationID:     initializationID,
		TransactionID:        transactionID,
		RefPresent:           refPresent,
		Profile:              profile,
		ProfileObjectPresent: profileObjectPresent,
	}
	if expectedRevision != nil {
		copy := *expectedRevision
		baseline.ExpectedRevision = &copy
	}
	if err := baseline.Validate(); err != nil {
		return IngestBaseline{}, err
	}
	return baseline, nil
}

// Validate rejects contradictory or malformed baseline evidence. Callers must
// revalidate immediately before durable effects because the exported optional
// pointer can be mutated by a caller after construction.
func (b IngestBaseline) Validate() error {
	if b.InitializationID.IsZero() || b.TransactionID.IsZero() {
		return errInvalidIngestEvidence
	}
	if b.RefPresent != (b.ExpectedRevision != nil) {
		return errInvalidIngestEvidence
	}
	if b.ExpectedRevision != nil && !validGitObjectID(*b.ExpectedRevision) {
		return errInvalidIngestEvidence
	}
	if !b.RefPresent && b.ProfileObjectPresent {
		return errInvalidIngestEvidence
	}
	return nil
}

func newOpaqueTransactionToken() (string, error) {
	var token [32]byte
	if _, err := rand.Read(token[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(token[:]), nil
}

func validGitObjectID(value string) bool {
	if len(value) != 40 && len(value) != 64 {
		return false
	}
	for _, ch := range value {
		switch {
		case ch >= '0' && ch <= '9':
		case ch >= 'a' && ch <= 'f':
		case ch >= 'A' && ch <= 'F':
		default:
			return false
		}
	}
	return true
}

// IngestLifecycle records the immutable registry state of a transaction.
type IngestLifecycle string

const (
	IngestLifecycleProvisional IngestLifecycle = "provisional"
	IngestLifecycleActive      IngestLifecycle = "active"
	IngestLifecycleCommitting  IngestLifecycle = "committing"
	IngestLifecycleFinalizing  IngestLifecycle = "finalizing"
	IngestLifecycleTerminal    IngestLifecycle = "terminal"
)

// IngestCommitStatus records the ref-publication truth independently of cleanup.
type IngestCommitStatus string

const (
	IngestCommitNotCommitted     IngestCommitStatus = "not_committed"
	IngestCommitCommitted        IngestCommitStatus = "committed"
	IngestCommitConflict         IngestCommitStatus = "conflict"
	IngestCommitRecoveryRequired IngestCommitStatus = "recovery_required"
)

// IngestRefState classifies authoritative ref observation.
type IngestRefState string

const (
	IngestRefExpected  IngestRefState = "expected"
	IngestRefCandidate IngestRefState = "candidate"
	IngestRefOther     IngestRefState = "other"
	IngestRefUnknown   IngestRefState = "unknown"
)

// IngestBackendState classifies secret-backend effects.
type IngestBackendState string

const (
	IngestBackendUnchanged IngestBackendState = "unchanged"
	IngestBackendApplied   IngestBackendState = "applied"
	IngestBackendRestored  IngestBackendState = "restored"
	IngestBackendUncertain IngestBackendState = "uncertain"
)

// IngestObjectsState classifies candidate/final object effects.
type IngestObjectsState string

const (
	IngestObjectsQuarantined IngestObjectsState = "quarantined"
	IngestObjectsRemoved     IngestObjectsState = "removed"
	IngestObjectsPublished   IngestObjectsState = "published"
	IngestObjectsRetained    IngestObjectsState = "retained"
	IngestObjectsUncertain   IngestObjectsState = "uncertain"
)

// QuarantineCleanupState records cleanup separately from ref publication.
type QuarantineCleanupState string

const (
	QuarantineCleanupRemoved   QuarantineCleanupState = "removed"
	QuarantineCleanupRetained  QuarantineCleanupState = "retained"
	QuarantineCleanupUncertain QuarantineCleanupState = "uncertain"
)

// IngestFailureCode is a stable, value-free failure classification.
type IngestFailureCode string

const (
	IngestFailureNone             IngestFailureCode = "none"
	IngestFailureInvalidAuthority IngestFailureCode = "invalid_authority"
	IngestFailureBaselineRead     IngestFailureCode = "baseline_read"
	IngestFailureQuarantine       IngestFailureCode = "quarantine"
	IngestFailureCleanup          IngestFailureCode = "cleanup"
)

// IngestBeginOutcome is the value-only evidence returned by BeginIngest.
type IngestBeginOutcome struct {
	InitializationID        InstallInitializationID
	TransactionID           IngestTransactionID
	Baseline                IngestBaseline
	Lifecycle               IngestLifecycle
	Cleanup                 QuarantineCleanupState
	FailureCode             IngestFailureCode
	RecoveryRequired        bool
	InitializerRollbackSafe bool
}

// IngestCommitOutcome records ref, backend, object, and cleanup axes without
// exposing paths or raw subprocess/backend errors.
type IngestCommitOutcome struct {
	InitializationID        InstallInitializationID
	TransactionID           IngestTransactionID
	Status                  IngestCommitStatus
	RefState                IngestRefState
	Backend                 IngestBackendState
	Objects                 IngestObjectsState
	Cleanup                 QuarantineCleanupState
	FailureCode             IngestFailureCode
	RecoveryRequired        bool
	InitializerRollbackSafe bool
	Withheld                WithheldReport
}

// IngestAbortOutcome is the immutable result of one cleanup attempt.
type IngestAbortOutcome struct {
	InitializationID        InstallInitializationID
	TransactionID           IngestTransactionID
	Lifecycle               IngestLifecycle
	Cleanup                 QuarantineCleanupState
	FailureCode             IngestFailureCode
	RecoveryRequired        bool
	InitializerRollbackSafe bool
}
