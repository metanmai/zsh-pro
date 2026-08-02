package model

// InstallInitializationID is an opaque Store-issued initialization authority.
type InstallInitializationID struct {
	token string
}

// NewInstallInitializationID creates a new opaque initialization identifier.
func NewInstallInitializationID() (InstallInitializationID, error) {
	return InstallInitializationID{}, nil
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
	return IngestTransactionID{}, nil
}

// IsZero reports whether no transaction was assigned.
func (id IngestTransactionID) IsZero() bool { return id.token == "" }

type errIngestModel string

func (e errIngestModel) Error() string { return string(e) }

const errIngestModelUnavailable errIngestModel = "ingest transaction model is not implemented"

// NewExpectedRevision validates and copies one Git object identifier.
func NewExpectedRevision(value string) (*string, error) {
	_ = value
	return nil, errIngestModelUnavailable
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
	_, _, _, _, _, _ = initializationID, transactionID, refPresent, expectedRevision, profile, profileObjectPresent
	return IngestBaseline{}, errIngestModelUnavailable
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
