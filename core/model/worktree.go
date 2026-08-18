package model

import (
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strings"
)

const (
	WorktreeSchemaV1        = "v1"
	MaxSnapshotBytes        = 2 * 1024 * 1024
	MaxSnapshotRecords      = 10_000
	MaxResolutionTokenBytes = 128
	ShellCapabilityBytes    = 32
)

const shellCapabilityDomain = "zsh-pro/worktree-shell-capability/v1"

var (
	liveEnvNameRE    = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	liveSymbolNameRE = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]*$`)
	liveTokenRE      = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]*$`)
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

// ShellCapability is an opaque presented bearer. Its bytes are deliberately
// held behind an unexported indirection so ordinary JSON/text encoding and Go
// formatting cannot disclose them accidentally.
type ShellCapability struct {
	material *shellCapabilityMaterial
}

type shellCapabilityMaterial struct {
	bytes [ShellCapabilityBytes]byte
}

// ShellCapabilityVerifier is the only credential material permitted at rest.
type ShellCapabilityVerifier [sha256.Size]byte

// ShellCredential authorizes one shell-private transition. It is intentionally
// not serializable; the runtime transport owns its separate framed encoding.
type ShellCredential struct {
	ShellID    string
	Capability ShellCapability
}

func NewShellCapability(raw []byte) (ShellCapability, error) {
	if len(raw) != ShellCapabilityBytes {
		return ShellCapability{}, fmt.Errorf("shell capability must be exactly %d bytes", ShellCapabilityBytes)
	}
	material := &shellCapabilityMaterial{}
	copy(material.bytes[:], raw)
	return ShellCapability{material: material}, nil
}

func (capability ShellCapability) Bytes() ([ShellCapabilityBytes]byte, bool) {
	if capability.material == nil {
		return [ShellCapabilityBytes]byte{}, false
	}
	return capability.material.bytes, true
}

func (capability ShellCapability) String() string { return "<redacted-shell-capability>" }

func (credential ShellCredential) String() string {
	return fmt.Sprintf("ShellCredential{ShellID:%q, Capability:<redacted>}", credential.ShellID)
}

func (ShellCredential) MarshalJSON() ([]byte, error) {
	return nil, errors.New("shell credential is not serializable")
}

func (ShellCredential) MarshalText() ([]byte, error) {
	return nil, errors.New("shell credential is not serializable")
}

func DeriveShellCapabilityVerifier(capability ShellCapability) (ShellCapabilityVerifier, error) {
	raw, ok := capability.Bytes()
	if !ok {
		return ShellCapabilityVerifier{}, errors.New("shell capability is missing")
	}
	digest := sha256.New()
	_, _ = digest.Write([]byte(shellCapabilityDomain))
	_, _ = digest.Write(raw[:])
	var verifier ShellCapabilityVerifier
	copy(verifier[:], digest.Sum(nil))
	return verifier, nil
}

func VerifyShellCapability(verifier ShellCapabilityVerifier, capability ShellCapability) bool {
	derived, err := DeriveShellCapabilityVerifier(capability)
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare(verifier[:], derived[:]) == 1
}

func (credential ShellCredential) Validate() error {
	if len(credential.ShellID) == 0 || len(credential.ShellID) > MaxResolutionTokenBytes || !liveTokenRE.MatchString(credential.ShellID) {
		return errors.New("shell credential ID is invalid")
	}
	if _, ok := credential.Capability.Bytes(); !ok {
		return errors.New("shell credential capability is missing")
	}
	return nil
}

type AttachRequest struct {
	OperationID string
	Credential  ShellCredential
	Initial     LiveSnapshot
}

type AttachResult struct {
	Revision          uint64
	Attached          bool
	ReconcileRequired bool
	Exclusions        []Exclusion
}

type PublishRequest struct {
	OperationID          string
	Credential           ShellCredential
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
	Credential      ShellCredential
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
	Credential  ShellCredential
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
	Credential  ShellCredential
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

// ValidateIdentity accepts only the closed live kind set and conservative
// shell-neutral names that the existing activation layer can reproduce.
func ValidateIdentity(identity Identity) error {
	if identity.Name == "" || strings.IndexByte(identity.Name, 0) >= 0 {
		return errors.New("live identity name is invalid")
	}
	switch identity.Kind {
	case LiveEnv:
		if !liveEnvNameRE.MatchString(identity.Name) {
			return fmt.Errorf("environment identity %q is invalid", identity.Name)
		}
	case LiveAlias, LiveFunction:
		if !liveSymbolNameRE.MatchString(identity.Name) {
			return fmt.Errorf("symbol identity %q is invalid", identity.Name)
		}
	case LivePath:
		if identity.Name != "PATH" {
			return fmt.Errorf("path identity %q is invalid", identity.Name)
		}
	case LiveFPath:
		if identity.Name != "FPATH" {
			return fmt.Errorf("fpath identity %q is invalid", identity.Name)
		}
	case LiveOption:
		if !liveEnvNameRE.MatchString(identity.Name) {
			return fmt.Errorf("option identity %q is invalid", identity.Name)
		}
	default:
		return fmt.Errorf("live identity kind %q is unsupported", identity.Kind)
	}
	return nil
}

// ValidateLiveValue verifies presence, the kind-selected payload, and framed
// NUL exclusion before a value crosses into diff or persistence code.
func ValidateLiveValue(kind LiveKind, value LiveValue) error {
	if !value.Present {
		if value.Scalar != nil || value.List != nil || value.Option != nil {
			return errors.New("removed live value carries a payload")
		}
		return nil
	}
	switch kind {
	case LiveEnv, LiveAlias, LiveFunction:
		if value.Scalar == nil || value.List != nil || value.Option != nil {
			return fmt.Errorf("live kind %q requires a scalar payload", kind)
		}
		if strings.IndexByte(*value.Scalar, 0) >= 0 {
			return errors.New("live scalar contains NUL")
		}
	case LivePath, LiveFPath:
		if value.Scalar != nil || value.Option != nil {
			return fmt.Errorf("live kind %q requires a list payload", kind)
		}
		for _, element := range value.List {
			if strings.IndexByte(element, 0) >= 0 {
				return errors.New("live list element contains NUL")
			}
		}
	case LiveOption:
		if value.Option == nil || value.Scalar != nil || value.List != nil {
			return errors.New("live option requires a boolean payload")
		}
	default:
		return fmt.Errorf("live value kind %q is unsupported", kind)
	}
	return nil
}

func ValidateLiveIdentityState(state LiveIdentityState) error {
	if err := ValidateIdentity(state.Identity); err != nil {
		return err
	}
	return ValidateLiveValue(state.Identity.Kind, state.Value)
}

func ValidateLiveSnapshot(snapshot LiveSnapshot) error {
	if snapshot.ByteSize > MaxSnapshotBytes {
		return fmt.Errorf("live snapshot is %d bytes; maximum is %d", snapshot.ByteSize, MaxSnapshotBytes)
	}
	if len(snapshot.States) > MaxSnapshotRecords {
		return fmt.Errorf("live snapshot has %d records; maximum is %d", len(snapshot.States), MaxSnapshotRecords)
	}
	var semanticBytes uint64
	for _, state := range snapshot.States {
		if err := ValidateLiveIdentityState(state); err != nil {
			return err
		}
		var err error
		semanticBytes, err = checkedLiveAdd(semanticBytes, uint64(len(state.Identity.Kind)))
		if err == nil {
			semanticBytes, err = checkedLiveAdd(semanticBytes, uint64(len(state.Identity.Name)))
		}
		if err == nil {
			semanticBytes, err = checkedLiveAdd(semanticBytes, liveValueBytes(state.Value))
		}
		if err != nil || semanticBytes > MaxSnapshotBytes {
			return fmt.Errorf("live snapshot semantic payload exceeds %d bytes", MaxSnapshotBytes)
		}
	}
	return nil
}

func checkedLiveAdd(left, right uint64) (uint64, error) {
	if math.MaxUint64-left < right {
		return left, errors.New("live snapshot size overflow")
	}
	return left + right, nil
}

func liveValueBytes(value LiveValue) uint64 {
	if !value.Present {
		return 0
	}
	if value.Scalar != nil {
		return uint64(len(*value.Scalar))
	}
	if value.Option != nil {
		return 1
	}
	var size uint64
	for _, element := range value.List {
		if math.MaxUint64-size < uint64(len(element))+1 {
			return math.MaxUint64
		}
		size += uint64(len(element)) + 1
	}
	return size
}

func EqualLiveValue(kind LiveKind, left, right LiveValue) bool {
	if ValidateLiveValue(kind, left) != nil || ValidateLiveValue(kind, right) != nil {
		return false
	}
	if left.Present != right.Present {
		return false
	}
	if !left.Present {
		return true
	}
	switch kind {
	case LiveEnv, LiveAlias, LiveFunction:
		return *left.Scalar == *right.Scalar
	case LivePath, LiveFPath:
		if len(left.List) != len(right.List) {
			return false
		}
		for index := range left.List {
			if left.List[index] != right.List[index] {
				return false
			}
		}
		return true
	case LiveOption:
		return *left.Option == *right.Option
	default:
		return false
	}
}

// NormalizeLiveStates keeps one defensively copied final occurrence per exact
// identity while preserving the relative order of those final occurrences.
func NormalizeLiveStates(states []LiveIdentityState) ([]LiveIdentityState, error) {
	last := make(map[Identity]int, len(states))
	for index, state := range states {
		if err := ValidateLiveIdentityState(state); err != nil {
			return nil, err
		}
		last[state.Identity] = index
	}
	out := make([]LiveIdentityState, 0, len(last))
	for index, state := range states {
		if last[state.Identity] != index {
			continue
		}
		out = append(out, LiveIdentityState{Identity: state.Identity, Value: CloneLiveValue(state.Value)})
	}
	return out, nil
}

// NormalizeOverlay applies the same final-occurrence authority to explicit
// values and tombstones. A tombstone can never smuggle a value payload.
func NormalizeOverlay(entries []OverlayEntry) ([]OverlayEntry, error) {
	last := make(map[Identity]int, len(entries))
	for index, entry := range entries {
		if err := ValidateIdentity(entry.Identity); err != nil {
			return nil, err
		}
		if entry.Tombstone {
			if err := ValidateLiveValue(entry.Identity.Kind, entry.Value); err != nil || entry.Value.Present {
				return nil, errors.New("live tombstone carries a value")
			}
		} else {
			if err := ValidateLiveValue(entry.Identity.Kind, entry.Value); err != nil {
				return nil, err
			}
			if !entry.Value.Present {
				return nil, errors.New("live overlay value is not present")
			}
		}
		last[entry.Identity] = index
	}
	out := make([]OverlayEntry, 0, len(last))
	for index, entry := range entries {
		if last[entry.Identity] != index {
			continue
		}
		out = append(out, OverlayEntry{Identity: entry.Identity, Value: CloneLiveValue(entry.Value), Tombstone: entry.Tombstone})
	}
	return out, nil
}

func NextRevision(revision uint64) (uint64, error) {
	if revision == math.MaxUint64 {
		return revision, errors.New("worktree revision overflow")
	}
	return revision + 1, nil
}

func NewCommittedWorktree(source Profile, projection LiveProjection) CommittedWorktree {
	projection.Schema = WorktreeSchemaV1
	return CommittedWorktree{
		Schema:     WorktreeSchemaV1,
		Source:     CloneProfile(source),
		Projection: CloneLiveProjection(projection),
	}
}

// CloneProfile copies every slice and pointer reachable from the source IR.
func CloneProfile(profile Profile) Profile {
	out := Profile{Entries: make([]Entry, len(profile.Entries))}
	for index, entry := range profile.Entries {
		out.Entries[index] = cloneProfileEntry(entry)
	}
	if profile.Entries == nil {
		out.Entries = nil
	}
	return out
}

func cloneProfileEntry(entry Entry) Entry {
	out := entry
	out.Names = cloneStrings(entry.Names)
	out.DeclarationFlags = cloneStrings(entry.DeclarationFlags)
	out.OptionFlags = cloneStrings(entry.OptionFlags)
	if entry.Secret != nil {
		secret := *entry.Secret
		out.Secret = &secret
	}
	if entry.RuntimeValue != nil {
		value := *entry.RuntimeValue
		out.RuntimeValue = &value
	}
	if entry.FunctionBody != nil {
		body := *entry.FunctionBody
		out.FunctionBody = &body
	}
	if entry.ListValue != nil {
		value := &ListValue{Segments: append([]ListSegment(nil), entry.ListValue.Segments...)}
		if entry.ListValue.Segments == nil {
			value.Segments = nil
		}
		out.ListValue = value
	}
	return out
}

func CloneLiveStates(states []LiveIdentityState) []LiveIdentityState {
	if states == nil {
		return nil
	}
	out := make([]LiveIdentityState, len(states))
	for index, state := range states {
		out[index] = LiveIdentityState{Identity: state.Identity, Value: CloneLiveValue(state.Value)}
	}
	return out
}

func CloneLiveChanges(changes []LiveChange) []LiveChange {
	if changes == nil {
		return nil
	}
	out := make([]LiveChange, len(changes))
	for index, change := range changes {
		out[index] = LiveChange{Kind: change.Kind, Identity: change.Identity, Value: CloneLiveValue(change.Value)}
	}
	return out
}

func CloneLiveProjection(projection LiveProjection) LiveProjection {
	out := projection
	out.States = CloneLiveStates(projection.States)
	out.Tombstones = append([]Identity(nil), projection.Tombstones...)
	if projection.Tombstones == nil {
		out.Tombstones = nil
	}
	return out
}

func cloneStrings(values []string) []string {
	if values == nil {
		return nil
	}
	return append([]string(nil), values...)
}

func (token ResolutionToken) Validate() error {
	if len(token) == 0 || len(token) > MaxResolutionTokenBytes || !liveTokenRE.MatchString(string(token)) {
		return errors.New("resolution token is invalid")
	}
	return nil
}
