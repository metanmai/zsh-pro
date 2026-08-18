package worktree

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"

	"zsh-pro/core/model"
)

const (
	StateSchemaVersion           = 1
	MaxStateEvents               = 128
	MaxShellRecords              = 128
	MaxOperationReceiptsPerShell = 64
	MaxStateBytes                = 16 * 1024 * 1024
)

var (
	stateTokenRE  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]*$`)
	stateOIDRE    = regexp.MustCompile(`^[0-9a-f]{40}([0-9a-f]{24})?$`)
	stateBranchRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]*$`)
)

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
	AdmittedIdentities []model.Identity
	Shared             []model.LiveIdentityState
	CompactionFloor    uint64
	CompactionBaseline []model.LiveIdentityState
	Events             []StateEvent
	Shells             map[string]ShellState
	OperationReceipts  []OperationReceipt

	admittedIdentitiesMissing bool
}

type stateDTO struct {
	SchemaVersion      int                       `json:"schema_version"`
	Materialized       bool                      `json:"materialized"`
	Branch             string                    `json:"branch"`
	BaseOID            string                    `json:"base_oid"`
	Committed          model.CommittedWorktree   `json:"committed"`
	HeadRevision       uint64                    `json:"head_revision"`
	AutoApplyDefault   bool                      `json:"auto_apply_default"`
	AdmittedIdentities *[]model.Identity         `json:"admitted_identities,omitempty"`
	Shared             []model.LiveIdentityState `json:"shared"`
	CompactionFloor    uint64                    `json:"compaction_floor"`
	CompactionBaseline []model.LiveIdentityState `json:"compaction_baseline"`
	Events             []StateEvent              `json:"events"`
	Shells             map[string]shellStateDTO  `json:"shells"`
	OperationReceipts  []operationReceiptDTO     `json:"operation_receipts"`
}

type shellStateDTO struct {
	CapabilityVerifier string                    `json:"capability_verifier"`
	AttachState        AttachState               `json:"attach_state"`
	AttachedRevision   uint64                    `json:"attached_revision"`
	AppliedRevision    uint64                    `json:"applied_revision"`
	AppliedBaseline    []model.LiveIdentityState `json:"applied_baseline"`
	CaptureBaseline    []model.LiveIdentityState `json:"capture_baseline"`
	UnpublishedDelta   []model.LiveChange        `json:"unpublished_delta"`
	PresentAtAttach    []model.Identity          `json:"present_at_attach"`
	Exclusions         []model.Exclusion         `json:"exclusions"`
	Transition         ShellTransition           `json:"transition"`
	Pending            pendingTransitionDTO      `json:"pending"`
	Behind             bool                      `json:"behind"`
	AutoApplyOverride  *bool                     `json:"auto_apply_override"`
	Conflict           *model.Conflict           `json:"conflict"`
	LastRecoveredError RecoveredErrorCode        `json:"last_recovered_error"`
	LastActiveRevision uint64                    `json:"last_active_revision"`
}

type pendingTransitionDTO struct {
	Kind        PendingKind           `json:"kind"`
	Revision    uint64                `json:"revision"`
	Fingerprint string                `json:"fingerprint"`
	Token       model.ResolutionToken `json:"token"`
	Conflict    *model.Identity       `json:"conflict"`
	Changes     []model.LiveChange    `json:"changes"`
}

type operationReceiptDTO struct {
	ShellID            string                     `json:"shell_id"`
	OperationID        string                     `json:"operation_id"`
	Kind               ReceiptKind                `json:"kind"`
	RequestFingerprint string                     `json:"request_fingerprint"`
	Attach             *model.AttachResult        `json:"attach"`
	Publish            *model.PublishResult       `json:"publish"`
	PreparePull        *model.PreparePullResult   `json:"prepare_pull"`
	Acknowledge        *model.AcknowledgeResult   `json:"acknowledge"`
	Resolve            *model.ResolveSharedResult `json:"resolve"`
}

func MarshalState(state State) ([]byte, error) {
	if err := ValidateState(state); err != nil {
		return nil, err
	}
	payload, err := json.MarshalIndent(toStateDTO(state), "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal worktree state: %w", err)
	}
	if len(payload)+1 > MaxStateBytes {
		return nil, fmt.Errorf("worktree state exceeds %d bytes", MaxStateBytes)
	}
	return append(payload, '\n'), nil
}

func UnmarshalState(payload []byte) (State, error) {
	if len(payload) == 0 || len(payload) > MaxStateBytes {
		return State{}, errors.New("worktree state size is invalid")
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	var dto stateDTO
	if err := decoder.Decode(&dto); err != nil {
		return State{}, fmt.Errorf("decode worktree state: %w", err)
	}
	if err := requireStateEOF(decoder); err != nil {
		return State{}, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(payload, &fields); err != nil {
		return State{}, fmt.Errorf("decode worktree state fields: %w", err)
	}
	if admitted, present := fields["admitted_identities"]; present && bytes.Equal(bytes.TrimSpace(admitted), []byte("null")) {
		return State{}, errors.New("admitted identities cannot be null")
	}
	state, err := fromStateDTO(dto)
	if err != nil {
		return State{}, err
	}
	if err := ValidateState(state); err != nil {
		return State{}, err
	}
	return cloneState(state), nil
}

func requireStateEOF(decoder *json.Decoder) error {
	var extra json.RawMessage
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("worktree state has trailing JSON")
		}
		return fmt.Errorf("decode worktree state trailer: %w", err)
	}
	return nil
}

func ValidateState(state State) error {
	if state.SchemaVersion != StateSchemaVersion {
		return fmt.Errorf("worktree state schema %d is unsupported", state.SchemaVersion)
	}
	if !state.Materialized {
		if state.Branch != "" || state.BaseOID != "" || state.HeadRevision != 0 || len(state.AdmittedIdentities) != 0 || len(state.Shared) != 0 || len(state.Events) != 0 || len(state.Shells) != 0 || len(state.OperationReceipts) != 0 {
			return errors.New("unmaterialized worktree state carries causal data")
		}
		return nil
	}
	if !validStateBranch(state.Branch) || !stateOIDRE.MatchString(state.BaseOID) {
		return errors.New("materialized worktree branch or base OID is invalid")
	}
	if state.HeadRevision == 0 || state.HeadRevision == ^uint64(0) || state.CompactionFloor > state.HeadRevision {
		return errors.New("worktree revision bounds are invalid")
	}
	if err := validateCommitted(state.Committed); err != nil {
		return err
	}
	if err := validateAdmittedIdentities(state.AdmittedIdentities); err != nil {
		return err
	}
	if err := validatePresentStates(state.Shared); err != nil {
		return fmt.Errorf("validate shared state: %w", err)
	}
	if err := validatePresentStates(state.CompactionBaseline); err != nil {
		return fmt.Errorf("validate compaction baseline: %w", err)
	}
	if len(state.Events) > MaxStateEvents {
		return fmt.Errorf("worktree state has %d events; maximum is %d", len(state.Events), MaxStateEvents)
	}
	if err := validateEventSequence(state.CompactionFloor, state.HeadRevision, state.Events); err != nil {
		return err
	}
	if err := validateSharedGeneration(state.CompactionBaseline, state.Events, state.Shared); err != nil {
		return err
	}
	if len(state.Shells) > MaxShellRecords {
		return fmt.Errorf("worktree state has %d shells; maximum is %d", len(state.Shells), MaxShellRecords)
	}
	for shellID, shell := range state.Shells {
		if err := validateShellState(shellID, shell, state.HeadRevision); err != nil {
			return err
		}
	}
	return validateReceipts(state)
}

func validateCommitted(committed model.CommittedWorktree) error {
	if committed.Schema != model.WorktreeSchemaV1 || committed.Projection.Schema != model.WorktreeSchemaV1 {
		return errors.New("committed worktree schema is invalid")
	}
	if err := validatePresentStates(committed.Projection.States); err != nil {
		return fmt.Errorf("validate committed projection: %w", err)
	}
	seen := make(map[model.Identity]bool, len(committed.Projection.Tombstones))
	for _, identity := range committed.Projection.Tombstones {
		if err := model.ValidateIdentity(identity); err != nil {
			return err
		}
		if seen[identity] {
			return errors.New("committed projection has duplicate tombstone")
		}
		seen[identity] = true
	}
	return nil
}

func validatePresentStates(states []model.LiveIdentityState) error {
	if len(states) > model.MaxSnapshotRecords {
		return errors.New("live state record limit exceeded")
	}
	normalized, err := model.NormalizeLiveStates(states)
	if err != nil {
		return err
	}
	if len(normalized) != len(states) {
		return errors.New("live state contains duplicate identities")
	}
	for _, state := range states {
		if !state.Value.Present {
			return errors.New("baseline contains an absent live value")
		}
	}
	return nil
}

func validateEventSequence(floor, head uint64, events []StateEvent) error {
	if len(events) == 0 {
		if floor != head {
			return errors.New("empty history does not reach head revision")
		}
		return nil
	}
	expected := floor + 1
	for _, event := range events {
		if event.Revision != expected || event.Revision == 0 {
			return errors.New("revision events are nonmonotonic")
		}
		if !validStateToken(event.OperationID) {
			return errors.New("revision event operation ID is invalid")
		}
		if event.ShellID != "" && !validStateToken(event.ShellID) {
			return errors.New("revision event shell ID is invalid")
		}
		if err := validateChanges(event.Changes); err != nil {
			return err
		}
		expected++
	}
	if events[len(events)-1].Revision != head {
		return errors.New("revision history does not end at head")
	}
	return nil
}

func validateSharedGeneration(baseline []model.LiveIdentityState, events []StateEvent, shared []model.LiveIdentityState) error {
	current := model.CloneLiveStates(baseline)
	var err error
	for _, event := range events {
		current, err = applyStateChanges(current, event.Changes)
		if err != nil {
			return err
		}
	}
	if !equalLiveStates(current, shared) {
		return errors.New("shared state does not match compacted causal history")
	}
	return nil
}

func validateShellState(shellID string, shell ShellState, head uint64) error {
	if !validStateToken(shellID) {
		return errors.New("shell state ID is invalid")
	}
	if verifierZero(shell.CapabilityVerifier) {
		return errors.New("shell state capability verifier is missing")
	}
	if shell.AttachState != AttachStateAtHead && shell.AttachState != AttachStateCleanReconcile {
		return errors.New("shell attachment state is invalid")
	}
	if shell.AttachedRevision > head || shell.AppliedRevision > head || shell.LastActiveRevision > head {
		return errors.New("shell revision exceeds shared head")
	}
	if shell.AttachState == AttachStateAtHead && shell.AttachedRevision == 0 {
		return errors.New("attached shell has no attachment revision")
	}
	if shell.AttachState == AttachStateCleanReconcile && (shell.AppliedRevision != 0 || len(shell.AppliedBaseline) != 0) {
		return errors.New("clean-reconcile shell carries an applied baseline")
	}
	if err := validatePresentStates(shell.AppliedBaseline); err != nil {
		return err
	}
	if err := validatePresentStates(shell.CaptureBaseline); err != nil {
		return err
	}
	if err := validateChanges(shell.UnpublishedDelta); err != nil {
		return err
	}
	if err := validateIdentityList(shell.PresentAtAttach); err != nil {
		return err
	}
	for _, exclusion := range shell.Exclusions {
		if err := model.ValidateIdentity(exclusion.Identity); err != nil || !validStateReason(exclusion.Reason) {
			return errors.New("shell attachment exclusion is invalid")
		}
	}
	if !validTransitionValue(shell.Transition) {
		return errors.New("shell transition state is invalid")
	}
	if err := validatePending(shell.Pending, shell, head); err != nil {
		return err
	}
	if shell.Conflict != nil {
		if err := validateConflict(*shell.Conflict, head); err != nil {
			return err
		}
	}
	if len(shell.UnpublishedDelta) != 0 && shell.Conflict == nil {
		return errors.New("unpublished shell delta has no durable conflict")
	}
	if shell.Conflict != nil && len(shell.UnpublishedDelta) == 0 {
		return errors.New("shell conflict lost its unpublished delta")
	}
	if shell.Transition == TransitionUnpublished && (shell.Conflict == nil || len(shell.UnpublishedDelta) == 0) {
		return errors.New("unpublished transition is incomplete")
	}
	if shell.Transition == TransitionPrepare && shell.Pending.Kind != PendingPull {
		return errors.New("prepare transition lost its pending pull")
	}
	if shell.Transition == TransitionResolve && shell.Pending.Kind != PendingResolution {
		return errors.New("resolve transition lost its pending resolution")
	}
	if shell.LastRecoveredError != "" && shell.LastRecoveredError != RecoveredLockUnavailable && shell.LastRecoveredError != RecoveredMalformedState && shell.LastRecoveredError != RecoveredPersistence && shell.LastRecoveredError != RecoveredApply {
		return errors.New("shell recovered error code is invalid")
	}
	return nil
}

func validatePending(pending PendingTransition, shell ShellState, head uint64) error {
	if pending.Kind == PendingNone {
		if pending.Revision != 0 || pending.Fingerprint != (SnapshotFingerprint{}) || pending.Token != "" || pending.Conflict != nil || len(pending.Changes) != 0 {
			return errors.New("empty pending transition carries state")
		}
		return nil
	}
	if pending.Kind != PendingPull && pending.Kind != PendingResolution {
		return errors.New("pending transition kind is invalid")
	}
	if pending.Revision == 0 || pending.Revision > head || pending.Revision < shell.AppliedRevision || pending.Fingerprint == (SnapshotFingerprint{}) || pending.Token.Validate() != nil {
		return errors.New("pending transition target is invalid")
	}
	if err := validateChanges(pending.Changes); err != nil {
		return err
	}
	if pending.Kind == PendingPull && pending.Conflict != nil {
		return errors.New("pending pull carries a conflict identity")
	}
	if pending.Kind == PendingResolution && (pending.Conflict == nil || shell.Conflict == nil || len(shell.UnpublishedDelta) == 0) {
		return errors.New("pending resolution is not backed by a durable conflict")
	}
	return nil
}

func validateConflict(conflict model.Conflict, head uint64) error {
	if conflict.Kind != model.ConflictOverlap && conflict.Kind != model.ConflictHistoryGap {
		return errors.New("shell conflict kind is invalid")
	}
	if err := model.ValidateIdentity(conflict.Identity); err != nil {
		return err
	}
	if conflict.SharedRevision == 0 || conflict.SharedRevision > head || conflict.BaseRevision > conflict.SharedRevision || conflict.Token.Validate() != nil {
		return errors.New("shell conflict causality is invalid")
	}
	return nil
}

func validateReceipts(state State) error {
	counts := make(map[string]int)
	seen := make(map[string]bool, len(state.OperationReceipts))
	for _, receipt := range state.OperationReceipts {
		if _, ok := state.Shells[receipt.ShellID]; !ok || !validStateToken(receipt.OperationID) {
			return errors.New("operation receipt authority is invalid")
		}
		key := receipt.ShellID + "\x00" + receipt.OperationID
		if seen[key] {
			return errors.New("duplicate operation receipt")
		}
		seen[key] = true
		counts[receipt.ShellID]++
		if counts[receipt.ShellID] > MaxOperationReceiptsPerShell {
			return errors.New("shell operation receipt limit exceeded")
		}
		if err := validateReceiptUnion(receipt); err != nil {
			return err
		}
	}
	return nil
}

func validateReceiptUnion(receipt OperationReceipt) error {
	count := 0
	if receipt.Attach != nil {
		count++
	}
	if receipt.Publish != nil {
		count++
	}
	if receipt.PreparePull != nil {
		count++
	}
	if receipt.Acknowledge != nil {
		count++
	}
	if receipt.Resolve != nil {
		count++
	}
	if count != 1 {
		return errors.New("operation receipt has a partial or contradictory result")
	}
	switch receipt.Kind {
	case ReceiptAttach:
		if receipt.Attach == nil {
			return errors.New("attach receipt result is missing")
		}
	case ReceiptPublish:
		if receipt.Publish == nil {
			return errors.New("publish receipt result is missing")
		}
	case ReceiptPreparePull:
		if receipt.PreparePull == nil {
			return errors.New("prepare receipt result is missing")
		}
	case ReceiptAcknowledge:
		if receipt.Acknowledge == nil {
			return errors.New("acknowledge receipt result is missing")
		}
	case ReceiptResolve:
		if receipt.Resolve == nil {
			return errors.New("resolve receipt result is missing")
		}
	default:
		return errors.New("operation receipt kind is invalid")
	}
	return nil
}

func validateChanges(changes []model.LiveChange) error {
	seen := make(map[model.Identity]bool, len(changes))
	for _, change := range changes {
		if seen[change.Identity] {
			return errors.New("duplicate live change identity")
		}
		seen[change.Identity] = true
		if err := validateLiveChange(change); err != nil {
			return err
		}
	}
	return nil
}

func validateIdentityList(identities []model.Identity) error {
	seen := make(map[model.Identity]bool, len(identities))
	for _, identity := range identities {
		if err := model.ValidateIdentity(identity); err != nil || seen[identity] {
			return errors.New("identity list is invalid")
		}
		seen[identity] = true
	}
	return nil
}

func validateAdmittedIdentities(identities []model.Identity) error {
	if len(identities) > model.MaxSnapshotRecords {
		return errors.New("admitted identity record limit exceeded")
	}
	var semanticBytes uint64
	for index, identity := range identities {
		if err := model.ValidateIdentity(identity); err != nil {
			return errors.New("admitted identity is invalid")
		}
		semanticBytes += uint64(len(identity.Kind)) + uint64(len(identity.Name)) + 2
		if semanticBytes > model.MaxSnapshotBytes {
			return errors.New("admitted identity payload limit exceeded")
		}
		if index == 0 {
			continue
		}
		previous := identities[index-1]
		if previous == identity {
			return errors.New("admitted identity list contains a duplicate")
		}
		if !admittedIdentityLess(previous, identity) {
			return errors.New("admitted identity list is not canonical")
		}
	}
	return nil
}

func admittedIdentityLess(left, right model.Identity) bool {
	if left.Kind != right.Kind {
		return left.Kind < right.Kind
	}
	return left.Name < right.Name
}

func CompactStateHistory(state *State) error {
	if state == nil || state.CompactionFloor > state.HeadRevision {
		return errors.New("worktree state cannot be compacted")
	}
	if err := validateEventSequence(state.CompactionFloor, state.HeadRevision, state.Events); err != nil {
		return err
	}
	for len(state.Events) > MaxStateEvents {
		dropped := state.Events[0]
		baseline, err := applyStateChanges(state.CompactionBaseline, dropped.Changes)
		if err != nil {
			return err
		}
		state.CompactionBaseline = baseline
		state.CompactionFloor = dropped.Revision
		state.Events = append([]StateEvent(nil), state.Events[1:]...)
	}
	return nil
}

func FingerprintSnapshot(snapshot model.LiveSnapshot) (SnapshotFingerprint, error) {
	if err := model.ValidateLiveSnapshot(snapshot); err != nil {
		return SnapshotFingerprint{}, err
	}
	states, err := model.NormalizeLiveStates(snapshot.States)
	if err != nil {
		return SnapshotFingerprint{}, err
	}
	sort.SliceStable(states, func(left, right int) bool {
		return identityLess(states[left].Identity, states[right].Identity)
	})
	payload, err := json.Marshal(states)
	if err != nil {
		return SnapshotFingerprint{}, err
	}
	return sha256.Sum256(payload), nil
}

func IsLegalShellTransition(from, to ShellTransition) bool {
	edges := map[ShellTransition]map[ShellTransition]bool{
		"":                     {TransitionAttach: true},
		TransitionAttach:       {TransitionCapture: true, TransitionPrepare: true},
		TransitionCapture:      {TransitionPublish: true, TransitionUnpublished: true, TransitionPrepare: true},
		TransitionUnpublished:  {TransitionPublish: true, TransitionResolve: true},
		TransitionPublish:      {TransitionCapture: true, TransitionPrepare: true},
		TransitionPrepare:      {TransitionApply: true},
		TransitionResolve:      {TransitionApply: true},
		TransitionApply:        {TransitionFreshCapture: true},
		TransitionFreshCapture: {TransitionAcknowledge: true},
		TransitionAcknowledge:  {TransitionCapture: true, TransitionPrepare: true},
	}
	return edges[from][to]
}

func CanGarbageCollectShell(state State, shellID string) bool {
	shell, ok := state.Shells[shellID]
	if !ok || shell.LastActiveRevision >= state.HeadRevision || shell.Behind || shell.Conflict != nil || shell.Pending.Kind != PendingNone || len(shell.UnpublishedDelta) != 0 {
		return false
	}
	return true
}

func applyStateChanges(states []model.LiveIdentityState, changes []model.LiveChange) ([]model.LiveIdentityState, error) {
	if err := validateChanges(changes); err != nil {
		return nil, err
	}
	current, err := model.NormalizeLiveStates(states)
	if err != nil {
		return nil, err
	}
	index := make(map[model.Identity]int, len(current))
	for i, state := range current {
		index[state.Identity] = i
	}
	active := make([]bool, len(current))
	for i := range active {
		active[i] = current[i].Value.Present
	}
	for _, change := range changes {
		i, exists := index[change.Identity]
		if change.Kind == model.LiveRemove {
			if exists {
				active[i] = false
			}
			continue
		}
		state := model.LiveIdentityState{Identity: change.Identity, Value: model.CloneLiveValue(change.Value)}
		if exists {
			current[i], active[i] = state, true
			continue
		}
		index[change.Identity] = len(current)
		current = append(current, state)
		active = append(active, true)
	}
	out := make([]model.LiveIdentityState, 0, len(current))
	for i, state := range current {
		if active[i] {
			out = append(out, state)
		}
	}
	return out, nil
}

func equalLiveStates(left, right []model.LiveIdentityState) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i].Identity != right[i].Identity || !model.EqualLiveValue(left[i].Identity.Kind, left[i].Value, right[i].Value) {
			return false
		}
	}
	return true
}

func validStateToken(value string) bool {
	return len(value) > 0 && len(value) <= model.MaxResolutionTokenBytes && stateTokenRE.MatchString(value)
}
func validStateReason(value string) bool {
	return value != "" && len(value) <= model.MaxResolutionTokenBytes && stateTokenRE.MatchString(value)
}
func validStateBranch(value string) bool {
	return len(value) <= 255 && !strings.Contains(value, "..") && !strings.HasSuffix(value, "/") && stateBranchRE.MatchString(value)
}
func verifierZero(verifier model.ShellCapabilityVerifier) bool {
	return verifier == (model.ShellCapabilityVerifier{})
}
func validTransitionValue(value ShellTransition) bool {
	switch value {
	case TransitionAttach, TransitionCapture, TransitionUnpublished, TransitionPublish, TransitionPrepare, TransitionResolve, TransitionApply, TransitionFreshCapture, TransitionAcknowledge:
		return true
	}
	return false
}

func toStateDTO(state State) stateDTO {
	dto := stateDTO{SchemaVersion: state.SchemaVersion, Materialized: state.Materialized, Branch: state.Branch, BaseOID: state.BaseOID, Committed: cloneCommitted(state.Committed), HeadRevision: state.HeadRevision, AutoApplyDefault: state.AutoApplyDefault, Shared: model.CloneLiveStates(state.Shared), CompactionFloor: state.CompactionFloor, CompactionBaseline: model.CloneLiveStates(state.CompactionBaseline), Events: cloneEvents(state.Events), Shells: make(map[string]shellStateDTO, len(state.Shells)), OperationReceipts: make([]operationReceiptDTO, len(state.OperationReceipts))}
	if !state.admittedIdentitiesMissing {
		admitted := cloneIdentities(state.AdmittedIdentities)
		if admitted == nil {
			admitted = make([]model.Identity, 0)
		}
		dto.AdmittedIdentities = &admitted
	}
	for id, shell := range state.Shells {
		dto.Shells[id] = toShellDTO(shell)
	}
	if state.Shells == nil {
		dto.Shells = nil
	}
	for i, receipt := range state.OperationReceipts {
		dto.OperationReceipts[i] = toReceiptDTO(receipt)
	}
	return dto
}

func fromStateDTO(dto stateDTO) (State, error) {
	state := State{SchemaVersion: dto.SchemaVersion, Materialized: dto.Materialized, Branch: dto.Branch, BaseOID: dto.BaseOID, Committed: cloneCommitted(dto.Committed), HeadRevision: dto.HeadRevision, AutoApplyDefault: dto.AutoApplyDefault, Shared: model.CloneLiveStates(dto.Shared), CompactionFloor: dto.CompactionFloor, CompactionBaseline: model.CloneLiveStates(dto.CompactionBaseline), Events: cloneEvents(dto.Events), Shells: make(map[string]ShellState, len(dto.Shells)), OperationReceipts: make([]OperationReceipt, len(dto.OperationReceipts)), admittedIdentitiesMissing: dto.AdmittedIdentities == nil}
	if dto.AdmittedIdentities != nil {
		state.AdmittedIdentities = cloneIdentities(*dto.AdmittedIdentities)
	}
	for id, shellDTO := range dto.Shells {
		shell, err := fromShellDTO(shellDTO)
		if err != nil {
			return State{}, err
		}
		state.Shells[id] = shell
	}
	if dto.Shells == nil {
		state.Shells = nil
	}
	for i, receiptDTO := range dto.OperationReceipts {
		receipt, err := fromReceiptDTO(receiptDTO)
		if err != nil {
			return State{}, err
		}
		state.OperationReceipts[i] = receipt
	}
	return state, nil
}

func toShellDTO(shell ShellState) shellStateDTO {
	return shellStateDTO{CapabilityVerifier: hex.EncodeToString(shell.CapabilityVerifier[:]), AttachState: shell.AttachState, AttachedRevision: shell.AttachedRevision, AppliedRevision: shell.AppliedRevision, AppliedBaseline: model.CloneLiveStates(shell.AppliedBaseline), CaptureBaseline: model.CloneLiveStates(shell.CaptureBaseline), UnpublishedDelta: model.CloneLiveChanges(shell.UnpublishedDelta), PresentAtAttach: cloneIdentities(shell.PresentAtAttach), Exclusions: cloneExclusions(shell.Exclusions), Transition: shell.Transition, Pending: toPendingDTO(shell.Pending), Behind: shell.Behind, AutoApplyOverride: cloneBool(shell.AutoApplyOverride), Conflict: cloneConflict(shell.Conflict), LastRecoveredError: shell.LastRecoveredError, LastActiveRevision: shell.LastActiveRevision}
}

func fromShellDTO(dto shellStateDTO) (ShellState, error) {
	verifier, err := decodeVerifier(dto.CapabilityVerifier)
	if err != nil {
		return ShellState{}, err
	}
	pending, err := fromPendingDTO(dto.Pending)
	if err != nil {
		return ShellState{}, err
	}
	return ShellState{CapabilityVerifier: verifier, AttachState: dto.AttachState, AttachedRevision: dto.AttachedRevision, AppliedRevision: dto.AppliedRevision, AppliedBaseline: model.CloneLiveStates(dto.AppliedBaseline), CaptureBaseline: model.CloneLiveStates(dto.CaptureBaseline), UnpublishedDelta: model.CloneLiveChanges(dto.UnpublishedDelta), PresentAtAttach: cloneIdentities(dto.PresentAtAttach), Exclusions: cloneExclusions(dto.Exclusions), Transition: dto.Transition, Pending: pending, Behind: dto.Behind, AutoApplyOverride: cloneBool(dto.AutoApplyOverride), Conflict: cloneConflict(dto.Conflict), LastRecoveredError: dto.LastRecoveredError, LastActiveRevision: dto.LastActiveRevision}, nil
}

func toPendingDTO(pending PendingTransition) pendingTransitionDTO {
	fingerprint := ""
	if pending.Fingerprint != (SnapshotFingerprint{}) {
		fingerprint = hex.EncodeToString(pending.Fingerprint[:])
	}
	return pendingTransitionDTO{Kind: pending.Kind, Revision: pending.Revision, Fingerprint: fingerprint, Token: pending.Token, Conflict: cloneIdentityPointer(pending.Conflict), Changes: model.CloneLiveChanges(pending.Changes)}
}
func fromPendingDTO(dto pendingTransitionDTO) (PendingTransition, error) {
	fingerprint, err := decodeFingerprint(dto.Fingerprint, dto.Kind == PendingNone)
	if err != nil {
		return PendingTransition{}, err
	}
	return PendingTransition{Kind: dto.Kind, Revision: dto.Revision, Fingerprint: fingerprint, Token: dto.Token, Conflict: cloneIdentityPointer(dto.Conflict), Changes: model.CloneLiveChanges(dto.Changes)}, nil
}
func toReceiptDTO(receipt OperationReceipt) operationReceiptDTO {
	return operationReceiptDTO{ShellID: receipt.ShellID, OperationID: receipt.OperationID, Kind: receipt.Kind, RequestFingerprint: hex.EncodeToString(receipt.RequestFingerprint[:]), Attach: cloneAttachResult(receipt.Attach), Publish: clonePublishResult(receipt.Publish), PreparePull: clonePrepareResult(receipt.PreparePull), Acknowledge: cloneAcknowledgeResult(receipt.Acknowledge), Resolve: cloneResolveResult(receipt.Resolve)}
}
func fromReceiptDTO(dto operationReceiptDTO) (OperationReceipt, error) {
	fingerprint, err := decodeFingerprint(dto.RequestFingerprint, false)
	if err != nil {
		return OperationReceipt{}, err
	}
	return OperationReceipt{ShellID: dto.ShellID, OperationID: dto.OperationID, Kind: dto.Kind, RequestFingerprint: fingerprint, Attach: cloneAttachResult(dto.Attach), Publish: clonePublishResult(dto.Publish), PreparePull: clonePrepareResult(dto.PreparePull), Acknowledge: cloneAcknowledgeResult(dto.Acknowledge), Resolve: cloneResolveResult(dto.Resolve)}, nil
}

func decodeVerifier(encoded string) (model.ShellCapabilityVerifier, error) {
	raw, err := hex.DecodeString(encoded)
	if err != nil || len(raw) != sha256.Size {
		return model.ShellCapabilityVerifier{}, errors.New("shell capability verifier encoding is invalid")
	}
	var out model.ShellCapabilityVerifier
	copy(out[:], raw)
	return out, nil
}
func decodeFingerprint(encoded string, allowEmpty bool) (SnapshotFingerprint, error) {
	if allowEmpty && encoded == "" {
		return SnapshotFingerprint{}, nil
	}
	raw, err := hex.DecodeString(encoded)
	if err != nil || len(raw) != sha256.Size {
		return SnapshotFingerprint{}, errors.New("snapshot fingerprint encoding is invalid")
	}
	var out SnapshotFingerprint
	copy(out[:], raw)
	return out, nil
}

func cloneState(state State) State {
	dto := toStateDTO(state)
	out, err := fromStateDTO(dto)
	if err != nil {
		panic(err)
	}
	return out
}
func cloneCommitted(value model.CommittedWorktree) model.CommittedWorktree {
	return model.NewCommittedWorktree(value.Source, value.Projection)
}
func cloneEvents(events []StateEvent) []StateEvent {
	if events == nil {
		return nil
	}
	out := make([]StateEvent, len(events))
	for i, event := range events {
		out[i] = StateEvent{Revision: event.Revision, OperationID: event.OperationID, ShellID: event.ShellID, Changes: model.CloneLiveChanges(event.Changes)}
	}
	return out
}
func cloneIdentities(values []model.Identity) []model.Identity {
	if values == nil {
		return nil
	}
	return append([]model.Identity(nil), values...)
}
func cloneExclusions(values []model.Exclusion) []model.Exclusion {
	if values == nil {
		return nil
	}
	return append([]model.Exclusion(nil), values...)
}
func cloneBool(value *bool) *bool {
	if value == nil {
		return nil
	}
	out := *value
	return &out
}
func cloneIdentityPointer(value *model.Identity) *model.Identity {
	if value == nil {
		return nil
	}
	out := *value
	return &out
}
func cloneConflict(value *model.Conflict) *model.Conflict {
	if value == nil {
		return nil
	}
	out := *value
	return &out
}
func cloneAttachResult(value *model.AttachResult) *model.AttachResult {
	if value == nil {
		return nil
	}
	out := *value
	out.Exclusions = cloneExclusions(value.Exclusions)
	return &out
}
func clonePublishResult(value *model.PublishResult) *model.PublishResult {
	if value == nil {
		return nil
	}
	out := *value
	out.Accepted = cloneIdentities(value.Accepted)
	if value.Conflicts != nil {
		out.Conflicts = append([]model.Conflict(nil), value.Conflicts...)
	}
	return &out
}
func clonePrepareResult(value *model.PreparePullResult) *model.PreparePullResult {
	if value == nil {
		return nil
	}
	out := *value
	out.Changes = model.CloneLiveChanges(value.Changes)
	return &out
}
func cloneAcknowledgeResult(value *model.AcknowledgeResult) *model.AcknowledgeResult {
	if value == nil {
		return nil
	}
	out := *value
	return &out
}
func cloneResolveResult(value *model.ResolveSharedResult) *model.ResolveSharedResult {
	if value == nil {
		return nil
	}
	out := *value
	out.Changes = model.CloneLiveChanges(value.Changes)
	return &out
}
