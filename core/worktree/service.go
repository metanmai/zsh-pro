package worktree

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"reflect"

	"zsh-pro/core/model"
)

var (
	ErrWorktreeUnmaterialized     = errors.New("worktree is not materialized")
	ErrRecoveryRequired           = errors.New("worktree recovery is required")
	ErrUnauthorized               = errors.New("worktree shell authorization failed")
	ErrOperationReplay            = errors.New("worktree operation ID was reused")
	ErrShellAlreadyAttached       = errors.New("worktree shell is already attached")
	ErrNeedsReconcile             = errors.New("worktree shell requires reconciliation")
	ErrConflictRequiresResolution = errors.New("worktree conflict requires resolution")
	ErrAcknowledgeMismatch        = errors.New("worktree acknowledgement does not match pending state")
)

// Service applies the shared-worktree state machine to one canonical Store.
// Distinct Service values may safely share authority through the same
// descriptor-relative worktree.json generation.
type Service struct {
	store    *StateStore
	registry *Registry
}

func NewService(store *StateStore, registry *Registry) (*Service, error) {
	if store == nil || registry == nil {
		return nil, errors.New("worktree service dependencies are unavailable")
	}
	return &Service{store: store, registry: registry}, nil
}

// Materialize creates the first exact branch/OID generation. A repeat is a
// no-op only when all immutable materialization inputs match.
func (s *Service) Materialize(ctx context.Context, branch, oid string, committed model.CommittedWorktree) error {
	if s == nil || s.store == nil || s.registry == nil {
		return errors.New("worktree service is unavailable")
	}
	canonical := model.NewCommittedWorktree(committed.Source, committed.Projection)
	sharedChanges, err := DiffSnapshot(model.LiveSnapshot{}, model.LiveSnapshot{States: canonical.Projection.States})
	if err != nil {
		return ErrRecoveryRequired
	}
	shared, err := applyStateChanges(nil, sharedChanges)
	if err != nil {
		return ErrRecoveryRequired
	}
	candidate := State{
		SchemaVersion:      StateSchemaVersion,
		Materialized:       true,
		Branch:             branch,
		BaseOID:            oid,
		Committed:          canonical,
		HeadRevision:       1,
		AutoApplyDefault:   true,
		Shared:             shared,
		CompactionFloor:    0,
		CompactionBaseline: nil,
		Events: []StateEvent{{
			Revision: 1, OperationID: "materialize:1", Changes: sharedChanges,
		}},
		Shells: make(map[string]ShellState),
	}
	if err := ValidateState(candidate); err != nil {
		return ErrRecoveryRequired
	}
	return s.store.WithTransaction(ctx, func(state *State) error {
		if state.Materialized {
			if state.Branch != branch || state.BaseOID != oid || !reflect.DeepEqual(state.Committed, canonical) {
				return ErrRecoveryRequired
			}
			if err := s.registry.Seed(state.Committed.Source); err != nil {
				return ErrRecoveryRequired
			}
			return nil
		}
		if err := s.registry.Seed(canonical.Source); err != nil {
			return ErrRecoveryRequired
		}
		*state = cloneState(candidate)
		return nil
	})
}

func (s *Service) Attach(ctx context.Context, request model.AttachRequest) (model.AttachResult, error) {
	if err := s.available(); err != nil {
		return model.AttachResult{}, err
	}
	if err := validatePrivateRequest(request.OperationID, request.Credential); err != nil {
		return model.AttachResult{}, err
	}
	if err := model.ValidateLiveSnapshot(request.Initial); err != nil {
		return model.AttachResult{}, err
	}
	requestFingerprint, err := fingerprintParts("attach", request.Initial)
	if err != nil {
		return model.AttachResult{}, err
	}
	var result model.AttachResult
	err = s.store.WithTransaction(ctx, func(state *State) error {
		if !state.Materialized {
			return ErrWorktreeUnmaterialized
		}
		verifier, deriveErr := model.DeriveShellCapabilityVerifier(request.Credential.Capability)
		if deriveErr != nil {
			return ErrUnauthorized
		}
		if existing, ok := state.Shells[request.Credential.ShellID]; ok {
			if !model.VerifyShellCapability(existing.CapabilityVerifier, request.Credential.Capability) {
				return ErrUnauthorized
			}
			receipt, found := receiptFor(*state, request.Credential.ShellID, request.OperationID)
			if !found {
				return ErrShellAlreadyAttached
			}
			if receipt.Kind != ReceiptAttach || receipt.RequestFingerprint != requestFingerprint || receipt.Attach == nil {
				return ErrOperationReplay
			}
			result = *cloneAttachResult(receipt.Attach)
			return nil
		}
		if len(state.Shells) >= MaxShellRecords {
			return errors.New("worktree shell limit reached")
		}
		if err := s.registry.Seed(state.Committed.Source); err != nil {
			return ErrRecoveryRequired
		}
		attachment, attachErr := s.registry.Attach(request.Initial)
		if attachErr != nil {
			return attachErr
		}
		baseline := presentOnly(attachment.Baseline())
		present := presentIdentities(request.Initial)
		shell := ShellState{
			CapabilityVerifier: verifier,
			AttachedRevision:   state.HeadRevision,
			CaptureBaseline:    baseline,
			PresentAtAttach:    present,
			Exclusions:         attachment.Exclusions(),
			Transition:         TransitionAttach,
			Pending:            PendingTransition{Kind: PendingNone},
			LastActiveRevision: state.HeadRevision,
		}
		if equalLiveStates(baseline, state.Shared) {
			shell.AttachState = AttachStateAtHead
			shell.AppliedRevision = state.HeadRevision
			shell.AppliedBaseline = model.CloneLiveStates(state.Shared)
		} else {
			shell.AttachState = AttachStateCleanReconcile
			shell.Behind = true
		}
		result = model.AttachResult{Revision: state.HeadRevision, Attached: true, Exclusions: attachment.Exclusions()}
		state.Shells[request.Credential.ShellID] = shell
		appendReceipt(state, OperationReceipt{
			ShellID: request.Credential.ShellID, OperationID: request.OperationID,
			Kind: ReceiptAttach, RequestFingerprint: requestFingerprint, Attach: cloneAttachResult(&result),
		})
		return nil
	})
	return result, err
}

func (s *Service) Publish(ctx context.Context, request model.PublishRequest) (model.PublishResult, error) {
	if err := s.available(); err != nil {
		return model.PublishResult{}, err
	}
	if err := validatePrivateRequest(request.OperationID, request.Credential); err != nil {
		return model.PublishResult{}, err
	}
	if err := validateChanges(request.Delta); err != nil {
		return model.PublishResult{}, err
	}
	requestFingerprint, err := fingerprintParts("publish", request.AcknowledgedRevision, request.Delta)
	if err != nil {
		return model.PublishResult{}, err
	}
	var result model.PublishResult
	err = s.store.WithTransaction(ctx, func(state *State) error {
		shell, authErr := authorizeShell(*state, request.Credential)
		if authErr != nil {
			return authErr
		}
		if receipt, found := receiptFor(*state, request.Credential.ShellID, request.OperationID); found {
			if receipt.Kind != ReceiptPublish || receipt.RequestFingerprint != requestFingerprint || receipt.Publish == nil {
				return ErrOperationReplay
			}
			result = *clonePublishResult(receipt.Publish)
			return nil
		}
		if shell.Pending.Kind != PendingNone || request.AcknowledgedRevision != shell.AppliedRevision || shell.AppliedRevision == 0 {
			return ErrNeedsReconcile
		}
		if shell.Conflict != nil || len(shell.UnpublishedDelta) != 0 {
			return ErrConflictRequiresResolution
		}
		if err := s.registry.Seed(state.Committed.Source); err != nil {
			return ErrRecoveryRequired
		}
		admitted := s.admittedChanges(shell, request.Delta)
		observed, applyErr := applyStateChanges(shell.CaptureBaseline, admitted)
		if applyErr != nil {
			return applyErr
		}
		semantic, diffErr := DiffSnapshot(model.LiveSnapshot{States: shell.CaptureBaseline}, model.LiveSnapshot{States: observed})
		if diffErr != nil {
			return diffErr
		}
		gap := request.AcknowledgedRevision < state.CompactionFloor
		remote := changedIdentitiesAfter(*state, request.AcknowledgedRevision)
		accepted := make([]model.LiveChange, 0, len(semantic))
		losing := make([]model.LiveChange, 0, len(semantic))
		for _, change := range semantic {
			if changeMatchesStates(change, state.Shared) {
				continue
			}
			if gap || remote[change.Identity] {
				losing = append(losing, change)
				continue
			}
			accepted = append(accepted, change)
		}
		if len(accepted) != 0 {
			next, nextErr := model.NextRevision(state.HeadRevision)
			if nextErr != nil {
				return nextErr
			}
			state.Shared, applyErr = applyStateChanges(state.Shared, accepted)
			if applyErr != nil {
				return applyErr
			}
			state.HeadRevision = next
			state.Events = append(state.Events, StateEvent{Revision: next, OperationID: request.OperationID, ShellID: request.Credential.ShellID, Changes: model.CloneLiveChanges(accepted)})
			if compactErr := CompactStateHistory(state); compactErr != nil {
				return compactErr
			}
		}
		result.SharedRevision = state.HeadRevision
		for _, change := range accepted {
			result.Accepted = append(result.Accepted, change.Identity)
		}
		if len(losing) != 0 {
			kind := model.ConflictOverlap
			if gap {
				kind = model.ConflictHistoryGap
			}
			token := newResolutionToken("conflict", request.OperationID, requestFingerprint, state.HeadRevision)
			for _, change := range losing {
				result.Conflicts = append(result.Conflicts, model.Conflict{
					Kind: kind, Identity: change.Identity, BaseRevision: request.AcknowledgedRevision,
					SharedRevision: state.HeadRevision, Token: token,
				})
			}
			conflict := result.Conflicts[0]
			shell.Conflict = &conflict
			shell.UnpublishedDelta = model.CloneLiveChanges(losing)
			shell.Transition = TransitionUnpublished
		} else {
			shell.Transition = TransitionPublish
		}
		shell.CaptureBaseline = observed
		shell.Behind = shell.AppliedRevision < state.HeadRevision || shell.Conflict != nil
		shell.LastActiveRevision = state.HeadRevision
		state.Shells[request.Credential.ShellID] = shell
		appendReceipt(state, OperationReceipt{
			ShellID: request.Credential.ShellID, OperationID: request.OperationID,
			Kind: ReceiptPublish, RequestFingerprint: requestFingerprint, Publish: clonePublishResult(&result),
		})
		return nil
	})
	return result, err
}

func (s *Service) PreparePull(ctx context.Context, request model.PreparePullRequest) (model.PreparePullResult, error) {
	if err := s.available(); err != nil {
		return model.PreparePullResult{}, err
	}
	if err := validatePrivateRequest(request.OperationID, request.Credential); err != nil {
		return model.PreparePullResult{}, err
	}
	requestFingerprint, err := fingerprintParts("prepare", request.AppliedRevision)
	if err != nil {
		return model.PreparePullResult{}, err
	}
	var result model.PreparePullResult
	err = s.store.WithTransaction(ctx, func(state *State) error {
		shell, authErr := authorizeShell(*state, request.Credential)
		if authErr != nil {
			return authErr
		}
		if receipt, found := receiptFor(*state, request.Credential.ShellID, request.OperationID); found {
			if receipt.Kind != ReceiptPreparePull || receipt.RequestFingerprint != requestFingerprint || receipt.PreparePull == nil {
				return ErrOperationReplay
			}
			result = *clonePrepareResult(receipt.PreparePull)
			return nil
		}
		if shell.Conflict != nil || len(shell.UnpublishedDelta) != 0 {
			return ErrConflictRequiresResolution
		}
		if shell.Pending.Kind != PendingNone {
			return ErrNeedsReconcile
		}
		base := shell.AppliedBaseline
		if request.AppliedRevision != shell.AppliedRevision {
			base = shell.CaptureBaseline
			shell.AttachState = AttachStateCleanReconcile
			shell.AppliedRevision = 0
			shell.AppliedBaseline = nil
		}
		changes, diffErr := DiffSnapshot(model.LiveSnapshot{States: base}, model.LiveSnapshot{States: state.Shared})
		if diffErr != nil {
			return diffErr
		}
		if len(changes) == 0 && request.AppliedRevision == state.HeadRevision && shell.AppliedRevision == state.HeadRevision {
			result = model.PreparePullResult{}
			shell.LastActiveRevision = state.HeadRevision
			state.Shells[request.Credential.ShellID] = shell
			appendReceipt(state, OperationReceipt{ShellID: request.Credential.ShellID, OperationID: request.OperationID, Kind: ReceiptPreparePull, RequestFingerprint: requestFingerprint, PreparePull: clonePrepareResult(&result)})
			return nil
		}
		target := model.LiveSnapshot{States: state.Shared}
		fingerprint, fingerprintErr := FingerprintSnapshot(target)
		if fingerprintErr != nil {
			return fingerprintErr
		}
		token := newResolutionToken("pull", request.OperationID, requestFingerprint, state.HeadRevision)
		result = model.PreparePullResult{PendingRevision: state.HeadRevision, Token: token, Changes: model.CloneLiveChanges(changes)}
		shell.Pending = PendingTransition{Kind: PendingPull, Revision: state.HeadRevision, Fingerprint: fingerprint, Token: token, Changes: model.CloneLiveChanges(changes)}
		shell.Transition = TransitionPrepare
		shell.Behind = true
		shell.LastActiveRevision = state.HeadRevision
		state.Shells[request.Credential.ShellID] = shell
		appendReceipt(state, OperationReceipt{ShellID: request.Credential.ShellID, OperationID: request.OperationID, Kind: ReceiptPreparePull, RequestFingerprint: requestFingerprint, PreparePull: clonePrepareResult(&result)})
		return nil
	})
	return result, err
}

func (s *Service) ResolveShared(ctx context.Context, request model.ResolveSharedRequest) (model.ResolveSharedResult, error) {
	if err := s.available(); err != nil {
		return model.ResolveSharedResult{}, err
	}
	if err := validatePrivateRequest(request.OperationID, request.Credential); err != nil {
		return model.ResolveSharedResult{}, err
	}
	if err := model.ValidateIdentity(request.Conflict); err != nil || request.Token.Validate() != nil || model.ValidateLiveSnapshot(request.Snapshot) != nil {
		return model.ResolveSharedResult{}, ErrConflictRequiresResolution
	}
	var result model.ResolveSharedResult
	err := s.store.WithTransaction(ctx, func(state *State) error {
		shell, authErr := authorizeShell(*state, request.Credential)
		if authErr != nil {
			return authErr
		}
		if err := s.registry.Seed(state.Committed.Source); err != nil {
			return ErrRecoveryRequired
		}
		observed, filterErr := s.admittedSnapshot(shell, request.Snapshot)
		if filterErr != nil {
			return filterErr
		}
		requestFingerprint, fingerprintErr := fingerprintParts("resolve", request.Conflict, request.Token, observed)
		if fingerprintErr != nil {
			return fingerprintErr
		}
		if receipt, found := receiptFor(*state, request.Credential.ShellID, request.OperationID); found {
			if receipt.Kind != ReceiptResolve || receipt.RequestFingerprint != requestFingerprint || receipt.Resolve == nil {
				return ErrOperationReplay
			}
			result = *cloneResolveResult(receipt.Resolve)
			return nil
		}
		if shell.Conflict == nil || shell.Conflict.Identity != request.Conflict || shell.Conflict.Token != request.Token || !changesContainIdentity(shell.UnpublishedDelta, request.Conflict) || shell.Pending.Kind != PendingNone {
			return ErrConflictRequiresResolution
		}
		if !equalLiveStates(observed, shell.CaptureBaseline) {
			return ErrConflictRequiresResolution
		}
		changes, diffErr := DiffSnapshot(model.LiveSnapshot{States: observed}, model.LiveSnapshot{States: state.Shared})
		if diffErr != nil {
			return diffErr
		}
		targetFingerprint, targetErr := FingerprintSnapshot(model.LiveSnapshot{States: state.Shared})
		if targetErr != nil {
			return targetErr
		}
		result = model.ResolveSharedResult{PendingRevision: state.HeadRevision, Token: request.Token, Changes: model.CloneLiveChanges(changes)}
		conflictIdentity := request.Conflict
		shell.Pending = PendingTransition{Kind: PendingResolution, Revision: state.HeadRevision, Fingerprint: targetFingerprint, Token: request.Token, Conflict: &conflictIdentity, Changes: model.CloneLiveChanges(changes)}
		shell.Transition = TransitionResolve
		shell.Behind = true
		shell.LastActiveRevision = state.HeadRevision
		state.Shells[request.Credential.ShellID] = shell
		appendReceipt(state, OperationReceipt{ShellID: request.Credential.ShellID, OperationID: request.OperationID, Kind: ReceiptResolve, RequestFingerprint: requestFingerprint, Resolve: cloneResolveResult(&result)})
		return nil
	})
	return result, err
}

func (s *Service) Acknowledge(ctx context.Context, request model.AcknowledgeRequest) (model.AcknowledgeResult, error) {
	if err := s.available(); err != nil {
		return model.AcknowledgeResult{}, err
	}
	if err := validatePrivateRequest(request.OperationID, request.Credential); err != nil {
		return model.AcknowledgeResult{}, err
	}
	if request.Revision == 0 || request.Token.Validate() != nil || model.ValidateLiveSnapshot(request.Snapshot) != nil {
		return model.AcknowledgeResult{}, ErrAcknowledgeMismatch
	}
	var result model.AcknowledgeResult
	err := s.store.WithTransaction(ctx, func(state *State) error {
		shell, authErr := authorizeShell(*state, request.Credential)
		if authErr != nil {
			return authErr
		}
		if err := s.registry.Seed(state.Committed.Source); err != nil {
			return ErrRecoveryRequired
		}
		observed, filterErr := s.admittedSnapshot(shell, request.Snapshot)
		if filterErr != nil {
			return filterErr
		}
		requestFingerprint, fingerprintErr := fingerprintParts("acknowledge", request.Revision, request.Token, observed)
		if fingerprintErr != nil {
			return fingerprintErr
		}
		if receipt, found := receiptFor(*state, request.Credential.ShellID, request.OperationID); found {
			if receipt.Kind != ReceiptAcknowledge || receipt.RequestFingerprint != requestFingerprint || receipt.Acknowledge == nil {
				return ErrOperationReplay
			}
			result = *cloneAcknowledgeResult(receipt.Acknowledge)
			return nil
		}
		pending := shell.Pending
		if pending.Kind == PendingNone || pending.Revision != request.Revision || pending.Token != request.Token {
			return ErrAcknowledgeMismatch
		}
		if pending.Kind == PendingResolution && (pending.Conflict == nil || shell.Conflict == nil || *pending.Conflict != shell.Conflict.Identity || pending.Token != shell.Conflict.Token) {
			return ErrAcknowledgeMismatch
		}
		observedFingerprint, observedErr := FingerprintSnapshot(model.LiveSnapshot{States: observed})
		if observedErr != nil || observedFingerprint != pending.Fingerprint {
			return ErrAcknowledgeMismatch
		}
		result = model.AcknowledgeResult{AppliedRevision: pending.Revision, Acknowledged: true}
		shell.AttachState = AttachStateAtHead
		shell.AppliedRevision = pending.Revision
		shell.AppliedBaseline = model.CloneLiveStates(observed)
		shell.CaptureBaseline = model.CloneLiveStates(observed)
		if pending.Kind == PendingResolution {
			shell.Conflict = nil
			shell.UnpublishedDelta = nil
		}
		shell.Pending = PendingTransition{Kind: PendingNone}
		shell.Transition = TransitionAcknowledge
		shell.Behind = shell.AppliedRevision < state.HeadRevision || shell.Conflict != nil
		shell.LastActiveRevision = state.HeadRevision
		state.Shells[request.Credential.ShellID] = shell
		appendReceipt(state, OperationReceipt{ShellID: request.Credential.ShellID, OperationID: request.OperationID, Kind: ReceiptAcknowledge, RequestFingerprint: requestFingerprint, Acknowledge: cloneAcknowledgeResult(&result)})
		return nil
	})
	return result, err
}

func (s *Service) Status(ctx context.Context, shellID string) (model.WorktreeStatus, error) {
	if err := s.available(); err != nil {
		return model.WorktreeStatus{}, err
	}
	if shellID != "" && !validStateToken(shellID) {
		return model.WorktreeStatus{}, errors.New("shell status ID is invalid")
	}
	state, err := s.store.Read(ctx)
	if err != nil {
		return model.WorktreeStatus{}, err
	}
	if !state.Materialized {
		return model.WorktreeStatus{}, ErrWorktreeUnmaterialized
	}
	changes, err := DiffSnapshot(model.LiveSnapshot{States: state.Committed.Projection.States}, model.LiveSnapshot{States: state.Shared})
	if err != nil {
		return model.WorktreeStatus{}, err
	}
	status := model.WorktreeStatus{Branch: state.Branch, BaseOID: state.BaseOID, Revision: state.HeadRevision, DirtyCount: len(changes)}
	for _, shell := range state.Shells {
		if shell.Conflict != nil {
			status.ConflictCount++
		}
	}
	if shell, ok := state.Shells[shellID]; ok {
		autoApply := state.AutoApplyDefault
		if shell.AutoApplyOverride != nil {
			autoApply = *shell.AutoApplyOverride
		}
		status.Shell = &model.ShellWorktreeStatus{Attached: true, AppliedRevision: shell.AppliedRevision, Behind: shell.Behind, AutoApply: autoApply}
		if shell.Conflict != nil {
			status.Shell.ConflictCount = 1
		}
	}
	return status, nil
}

func (s *Service) Diff(ctx context.Context) (model.CategorizedDiff, error) {
	if err := s.available(); err != nil {
		return model.CategorizedDiff{}, err
	}
	state, err := s.store.Read(ctx)
	if err != nil {
		return model.CategorizedDiff{}, err
	}
	if !state.Materialized {
		return model.CategorizedDiff{}, ErrWorktreeUnmaterialized
	}
	changes, err := DiffSnapshot(model.LiveSnapshot{States: state.Committed.Projection.States}, model.LiveSnapshot{States: state.Shared})
	if err != nil {
		return model.CategorizedDiff{}, err
	}
	return CategorizeDiff(changes)
}

func (s *Service) available() error {
	if s == nil || s.store == nil || s.registry == nil {
		return errors.New("worktree service is unavailable")
	}
	return nil
}

func validatePrivateRequest(operationID string, credential model.ShellCredential) error {
	if !validStateToken(operationID) {
		return errors.New("worktree operation ID is invalid")
	}
	if err := credential.Validate(); err != nil {
		return ErrUnauthorized
	}
	return nil
}

func authorizeShell(state State, credential model.ShellCredential) (ShellState, error) {
	if !state.Materialized {
		return ShellState{}, ErrUnauthorized
	}
	shell, ok := state.Shells[credential.ShellID]
	if !ok || !model.VerifyShellCapability(shell.CapabilityVerifier, credential.Capability) {
		return ShellState{}, ErrUnauthorized
	}
	return shell, nil
}

func receiptFor(state State, shellID, operationID string) (OperationReceipt, bool) {
	for index := len(state.OperationReceipts) - 1; index >= 0; index-- {
		receipt := state.OperationReceipts[index]
		if receipt.ShellID == shellID && receipt.OperationID == operationID {
			return receipt, true
		}
	}
	return OperationReceipt{}, false
}

func appendReceipt(state *State, receipt OperationReceipt) {
	count := 0
	remove := -1
	for index, existing := range state.OperationReceipts {
		if existing.ShellID == receipt.ShellID {
			count++
			if remove < 0 {
				remove = index
			}
		}
	}
	if count >= MaxOperationReceiptsPerShell && remove >= 0 {
		state.OperationReceipts = append(state.OperationReceipts[:remove], state.OperationReceipts[remove+1:]...)
	}
	state.OperationReceipts = append(state.OperationReceipts, receipt)
}

func fingerprintParts(domain string, parts ...any) (SnapshotFingerprint, error) {
	digest := sha256.New()
	_, _ = digest.Write([]byte("zsh-pro/worktree-operation/v1:" + domain + "\x00"))
	for _, part := range parts {
		payload, err := json.Marshal(part)
		if err != nil {
			return SnapshotFingerprint{}, errors.New("worktree request fingerprint failed")
		}
		var size [8]byte
		binary.BigEndian.PutUint64(size[:], uint64(len(payload)))
		_, _ = digest.Write(size[:])
		_, _ = digest.Write(payload)
	}
	var fingerprint SnapshotFingerprint
	copy(fingerprint[:], digest.Sum(nil))
	return fingerprint, nil
}

func newResolutionToken(prefix, operationID string, fingerprint SnapshotFingerprint, revision uint64) model.ResolutionToken {
	digest := sha256.New()
	_, _ = digest.Write([]byte("zsh-pro/worktree-resolution/v1\x00"))
	_, _ = digest.Write([]byte(operationID))
	_, _ = digest.Write(fingerprint[:])
	var encodedRevision [8]byte
	binary.BigEndian.PutUint64(encodedRevision[:], revision)
	_, _ = digest.Write(encodedRevision[:])
	return model.ResolutionToken(prefix + ":" + hex.EncodeToString(digest.Sum(nil)[:16]))
}

func presentOnly(states []model.LiveIdentityState) []model.LiveIdentityState {
	out := make([]model.LiveIdentityState, 0, len(states))
	for _, state := range states {
		if state.Value.Present {
			out = append(out, model.LiveIdentityState{Identity: state.Identity, Value: model.CloneLiveValue(state.Value)})
		}
	}
	return out
}

func presentIdentities(snapshot model.LiveSnapshot) []model.Identity {
	normalized, err := model.NormalizeLiveStates(snapshot.States)
	if err != nil {
		return nil
	}
	identities := make([]model.Identity, 0, len(normalized))
	for _, state := range normalized {
		if state.Value.Present {
			identities = append(identities, state.Identity)
		}
	}
	return identities
}

func attachmentFromShell(registry *Registry, shell ShellState) AttachmentResult {
	present := make(map[model.Identity]struct{}, len(shell.PresentAtAttach))
	for _, identity := range shell.PresentAtAttach {
		present[identity] = struct{}{}
	}
	return AttachmentResult{
		baseline: model.CloneLiveStates(shell.CaptureBaseline), exclusions: cloneExclusions(shell.Exclusions),
		presentAtAttach: present, registry: registry,
	}
}

func (s *Service) admittedChanges(shell ShellState, changes []model.LiveChange) []model.LiveChange {
	attachment := attachmentFromShell(s.registry, shell)
	out := make([]model.LiveChange, 0, len(changes))
	for _, change := range changes {
		if !s.registry.AllowsLiveValue(change.Identity) && !s.registry.Admit(attachment, change.Identity).Admitted {
			continue
		}
		out = append(out, model.LiveChange{Kind: change.Kind, Identity: change.Identity, Value: model.CloneLiveValue(change.Value)})
	}
	return out
}

func (s *Service) admittedSnapshot(shell ShellState, snapshot model.LiveSnapshot) ([]model.LiveIdentityState, error) {
	normalized, err := model.NormalizeLiveStates(snapshot.States)
	if err != nil {
		return nil, err
	}
	attachment := attachmentFromShell(s.registry, shell)
	out := make([]model.LiveIdentityState, 0, len(normalized))
	for _, state := range normalized {
		if !state.Value.Present {
			continue
		}
		if !s.registry.AllowsLiveValue(state.Identity) && !s.registry.Admit(attachment, state.Identity).Admitted {
			continue
		}
		out = append(out, model.LiveIdentityState{Identity: state.Identity, Value: model.CloneLiveValue(state.Value)})
	}
	return out, nil
}

func changedIdentitiesAfter(state State, revision uint64) map[model.Identity]bool {
	out := make(map[model.Identity]bool)
	for _, event := range state.Events {
		if event.Revision <= revision {
			continue
		}
		for _, change := range event.Changes {
			out[change.Identity] = true
		}
	}
	return out
}

func changesContainIdentity(changes []model.LiveChange, identity model.Identity) bool {
	for _, change := range changes {
		if change.Identity == identity {
			return true
		}
	}
	return false
}

func changeMatchesStates(change model.LiveChange, states []model.LiveIdentityState) bool {
	for _, state := range states {
		if state.Identity != change.Identity {
			continue
		}
		if change.Kind == model.LiveRemove {
			return false
		}
		return model.EqualLiveValue(change.Identity.Kind, change.Value, state.Value)
	}
	return change.Kind == model.LiveRemove
}
