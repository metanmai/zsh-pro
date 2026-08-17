package worktree

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"zsh-pro/core/activate"
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
	ErrWorktreeDirty              = errors.New("worktree has uncommitted changes")
	ErrWorkflowUnavailable        = errors.New("worktree workflow is unavailable")
)

// Repository is the exact Git boundary required by the shared worktree
// workflow. It deliberately excludes every legacy process-local Store adapter.
type Repository interface {
	Branches(context.Context) ([]string, error)
	ResolveWorktreeRevision(context.Context, string) (string, error)
	ReadWorktreeRevision(context.Context, string) (model.CommittedWorktree, error)
	CreateFrom(context.Context, string, string) error
	CommitWorktree(context.Context, string, string, model.CommittedWorktree, string) (model.WorktreeCommitResult, error)
}

type AutoApplySource string

const (
	AutoApplyDefaultSource AutoApplySource = "default"
	AutoApplyShellSource   AutoApplySource = "shell"
)

// WorkflowStatus adds effective configuration provenance without widening the
// public model's value-free shared/shell status contract.
type WorkflowStatus struct {
	Worktree         model.WorktreeStatus
	PersistedDefault bool
	Effective        bool
	Source           AutoApplySource
}

// Service applies the shared-worktree state machine to one canonical Store.
// Distinct Service values may safely share authority through the same
// descriptor-relative worktree.json generation.
type Service struct {
	store    *StateStore
	registry *Registry
	repo     Repository
}

func NewService(store *StateStore, registry *Registry, repositories ...Repository) (*Service, error) {
	if store == nil || registry == nil {
		return nil, errors.New("worktree service dependencies are unavailable")
	}
	if len(repositories) > 1 {
		return nil, errors.New("worktree service received multiple repositories")
	}
	var repository Repository
	if len(repositories) == 1 && !isNilWorkflowRepository(repositories[0]) {
		repository = repositories[0]
	}
	return &Service{store: store, registry: registry, repo: repository}, nil
}

// Commit publishes every effective dirty identity without a staging or partial
// selection input. The worktree lock covers the dirty decision, expected base,
// repository CAS, and canonical-state update.
func (s *Service) Commit(ctx context.Context, message string) (model.WorktreeCommitResult, error) {
	if err := s.availableWorkflow(); err != nil {
		return model.WorktreeCommitResult{}, err
	}
	if message == "" || strings.IndexByte(message, 0) >= 0 {
		return model.WorktreeCommitResult{}, errors.New("worktree commit message is invalid")
	}
	var result model.WorktreeCommitResult
	var repositoryErr error
	err := s.store.WithTransaction(ctx, func(state *State) error {
		if !state.Materialized {
			return ErrWorktreeUnmaterialized
		}
		changes, diffErr := workflowChanges(*state)
		if diffErr != nil {
			return diffErr
		}
		if len(changes) == 0 {
			return nil
		}
		document, buildErr := activate.BuildEffective(state.Committed.Source, workflowOverlay(changes))
		if buildErr != nil {
			return buildErr
		}
		if !equalLiveStates(document.Projection.States, state.Shared) {
			return ErrRecoveryRequired
		}

		result, repositoryErr = s.repo.CommitWorktree(ctx, state.Branch, state.BaseOID, document, message)
		if !result.Committed {
			if repositoryErr != nil {
				return repositoryErr
			}
			return nil
		}
		if !stateOIDRE.MatchString(result.OID) {
			result.RecoveryRequired = true
			return nil
		}
		exact, readErr := s.repo.ReadWorktreeRevision(ctx, result.OID)
		if readErr != nil || validateCommitted(exact) != nil || !equalLiveStates(exact.Projection.States, state.Shared) {
			result.RecoveryRequired = true
			return nil
		}
		if seedErr := s.registry.Seed(exact.Source); seedErr != nil {
			result.RecoveryRequired = true
			return nil
		}
		state.BaseOID = result.OID
		state.Committed = model.NewCommittedWorktree(exact.Source, exact.Projection)
		return nil
	})
	if err != nil {
		if result.Committed {
			result.RecoveryRequired = true
			return result, ErrRecoveryRequired
		}
		return result, err
	}
	if result.Committed && result.RecoveryRequired {
		return result, ErrRecoveryRequired
	}
	return result, repositoryErr
}

func (s *Service) Branches(ctx context.Context) ([]string, error) {
	if err := s.availableWorkflow(); err != nil {
		return nil, err
	}
	var branches []string
	err := s.store.WithTransaction(ctx, func(state *State) error {
		if !state.Materialized {
			return ErrWorktreeUnmaterialized
		}
		if err := s.repairPublishedState(ctx, state); err != nil {
			return err
		}
		var err error
		branches, err = s.repo.Branches(ctx)
		return err
	})
	branches = append([]string(nil), branches...)
	sort.Strings(branches)
	return branches, err
}

func (s *Service) Branch(ctx context.Context, branch string) error {
	if err := s.availableWorkflow(); err != nil {
		return err
	}
	if !validStateBranch(branch) {
		return errors.New("worktree branch is invalid")
	}
	return s.store.WithTransaction(ctx, func(state *State) error {
		if !state.Materialized {
			return ErrWorktreeUnmaterialized
		}
		if err := s.repairPublishedState(ctx, state); err != nil {
			return err
		}
		return s.repo.CreateFrom(ctx, branch, state.BaseOID)
	})
}

func (s *Service) Checkout(ctx context.Context, branch string, create bool) error {
	if err := s.availableWorkflow(); err != nil {
		return err
	}
	if !validStateBranch(branch) {
		return errors.New("worktree branch is invalid")
	}
	return s.store.WithTransaction(ctx, func(state *State) error {
		if !state.Materialized {
			return ErrWorktreeUnmaterialized
		}
		changes, err := workflowChanges(*state)
		if err != nil {
			return err
		}
		if len(changes) != 0 {
			return ErrWorktreeDirty
		}
		for _, shell := range state.Shells {
			if shell.Conflict != nil || len(shell.UnpublishedDelta) != 0 || shell.Pending.Kind != PendingNone {
				return ErrConflictRequiresResolution
			}
		}
		if err := s.repairPublishedState(ctx, state); err != nil {
			return err
		}
		if create {
			if err := s.repo.CreateFrom(ctx, branch, state.BaseOID); err != nil {
				return err
			}
		}
		targetOID, err := s.repo.ResolveWorktreeRevision(ctx, branch)
		if err != nil {
			return err
		}
		if !stateOIDRE.MatchString(targetOID) {
			return ErrRecoveryRequired
		}
		target, err := s.repo.ReadWorktreeRevision(ctx, targetOID)
		if err != nil || validateCommitted(target) != nil {
			return ErrRecoveryRequired
		}
		return s.replaceWorkflowGeneration(state, branch, targetOID, target, "checkout")
	})
}

func (s *Service) ResetHard(ctx context.Context) error {
	if err := s.availableWorkflow(); err != nil {
		return err
	}
	return s.store.WithTransaction(ctx, func(state *State) error {
		if !state.Materialized {
			return ErrWorktreeUnmaterialized
		}
		target, err := s.repo.ReadWorktreeRevision(ctx, state.BaseOID)
		if err != nil || validateCommitted(target) != nil {
			return ErrRecoveryRequired
		}
		return s.replaceWorkflowGeneration(state, state.Branch, state.BaseOID, target, "reset")
	})
}

func (s *Service) SetAutoApplyDefault(ctx context.Context, enabled bool) error {
	if err := s.available(); err != nil {
		return err
	}
	return s.store.WithTransaction(ctx, func(state *State) error {
		if !state.Materialized {
			return ErrWorktreeUnmaterialized
		}
		state.AutoApplyDefault = enabled
		return nil
	})
}

func (s *Service) WorkflowStatus(ctx context.Context, shellID string) (WorkflowStatus, error) {
	if err := s.available(); err != nil {
		return WorkflowStatus{}, err
	}
	if shellID != "" && !validStateToken(shellID) {
		return WorkflowStatus{}, errors.New("shell status ID is invalid")
	}
	if s.repo != nil {
		if err := s.repairPublishedRef(ctx); err != nil {
			return WorkflowStatus{}, err
		}
	}
	state, err := s.store.Read(ctx)
	if err != nil {
		return WorkflowStatus{}, err
	}
	status, err := workflowStatusFromState(state, shellID)
	if err != nil {
		return WorkflowStatus{}, err
	}
	effective := state.AutoApplyDefault
	source := AutoApplyDefaultSource
	if shell, ok := state.Shells[shellID]; ok && shell.AutoApplyOverride != nil {
		effective = *shell.AutoApplyOverride
		source = AutoApplyShellSource
	}
	return WorkflowStatus{Worktree: status, PersistedDefault: state.AutoApplyDefault, Effective: effective, Source: source}, nil
}

func isNilWorkflowRepository(repository Repository) bool {
	if repository == nil {
		return true
	}
	value := reflect.ValueOf(repository)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
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
	initial := model.LiveSnapshot{ByteSize: request.Initial.ByteSize, States: model.CloneLiveStates(request.Initial.States)}
	requestFingerprint, err := fingerprintParts("attach", initial)
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
		attachment, attachErr := s.registry.Attach(initial)
		if attachErr != nil {
			return attachErr
		}
		baseline := presentOnly(attachment.Baseline())
		present := presentIdentities(initial)
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
	delta := model.CloneLiveChanges(request.Delta)
	requestFingerprint, err := fingerprintParts("publish", request.AcknowledgedRevision, delta)
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
		if err := validateChanges(delta); err != nil {
			return err
		}
		admitted := s.admittedChanges(shell, delta)
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
	snapshot := model.LiveSnapshot{ByteSize: request.Snapshot.ByteSize, States: model.CloneLiveStates(request.Snapshot.States)}
	var result model.ResolveSharedResult
	err := s.store.WithTransaction(ctx, func(state *State) error {
		shell, authErr := authorizeShell(*state, request.Credential)
		if authErr != nil {
			return authErr
		}
		if err := s.registry.Seed(state.Committed.Source); err != nil {
			return ErrRecoveryRequired
		}
		observed, filterErr := s.admittedSnapshot(shell, snapshot)
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
		observedFingerprint, fingerprintErr := FingerprintSnapshot(model.LiveSnapshot{States: observed})
		if fingerprintErr != nil {
			return ErrConflictRequiresResolution
		}
		baselineFingerprint, fingerprintErr := FingerprintSnapshot(model.LiveSnapshot{States: shell.CaptureBaseline})
		if fingerprintErr != nil || observedFingerprint != baselineFingerprint {
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
	snapshot := model.LiveSnapshot{ByteSize: request.Snapshot.ByteSize, States: model.CloneLiveStates(request.Snapshot.States)}
	var result model.AcknowledgeResult
	err := s.store.WithTransaction(ctx, func(state *State) error {
		shell, authErr := authorizeShell(*state, request.Credential)
		if authErr != nil {
			return authErr
		}
		if err := s.registry.Seed(state.Committed.Source); err != nil {
			return ErrRecoveryRequired
		}
		observed, filterErr := s.admittedSnapshot(shell, snapshot)
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
	status, err := s.WorkflowStatus(ctx, shellID)
	return status.Worktree, err
}

func (s *Service) Diff(ctx context.Context) (model.CategorizedDiff, error) {
	if err := s.available(); err != nil {
		return model.CategorizedDiff{}, err
	}
	if s.repo != nil {
		if err := s.repairPublishedRef(ctx); err != nil {
			return model.CategorizedDiff{}, err
		}
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

func (s *Service) availableWorkflow() error {
	if err := s.available(); err != nil {
		return err
	}
	if isNilWorkflowRepository(s.repo) {
		return ErrWorkflowUnavailable
	}
	return nil
}

func workflowChanges(state State) ([]model.LiveChange, error) {
	return DiffSnapshot(
		model.LiveSnapshot{States: state.Committed.Projection.States},
		model.LiveSnapshot{States: state.Shared},
	)
}

func workflowOverlay(changes []model.LiveChange) []model.OverlayEntry {
	overlay := make([]model.OverlayEntry, 0, len(changes))
	for _, change := range changes {
		entry := model.OverlayEntry{Identity: change.Identity}
		if change.Kind == model.LiveRemove {
			entry.Tombstone = true
		} else {
			entry.Value = model.CloneLiveValue(change.Value)
		}
		overlay = append(overlay, entry)
	}
	return overlay
}

func workflowStatusFromState(state State, shellID string) (model.WorktreeStatus, error) {
	if !state.Materialized {
		return model.WorktreeStatus{}, ErrWorktreeUnmaterialized
	}
	changes, err := workflowChanges(state)
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
		appliedFingerprint, err := FingerprintSnapshot(model.LiveSnapshot{States: shell.AppliedBaseline})
		if err != nil {
			return model.WorktreeStatus{}, err
		}
		sharedFingerprint, err := FingerprintSnapshot(model.LiveSnapshot{States: state.Shared})
		if err != nil {
			return model.WorktreeStatus{}, err
		}
		behind := shell.Behind || shell.Conflict != nil || shell.LastRecoveredError != "" ||
			shell.AppliedRevision < state.HeadRevision || appliedFingerprint != sharedFingerprint
		status.Shell = &model.ShellWorktreeStatus{Attached: true, AppliedRevision: shell.AppliedRevision, Behind: behind, AutoApply: autoApply}
		if shell.Conflict != nil {
			status.Shell.ConflictCount = 1
		}
	}
	return status, nil
}

// repairPublishedRef reconciles the only safe split outcome: the current
// branch ref advanced to an exact object whose projection already equals the
// locked shared generation. A different object is never imported implicitly.
func (s *Service) repairPublishedRef(ctx context.Context) error {
	if err := s.availableWorkflow(); err != nil {
		return err
	}
	return s.store.WithTransaction(ctx, func(state *State) error {
		if !state.Materialized {
			return ErrWorktreeUnmaterialized
		}
		return s.repairPublishedState(ctx, state)
	})
}

func (s *Service) repairPublishedState(ctx context.Context, state *State) error {
	observed, err := s.repo.ResolveWorktreeRevision(ctx, state.Branch)
	if err != nil {
		return err
	}
	if observed == state.BaseOID {
		return nil
	}
	if !stateOIDRE.MatchString(observed) {
		return ErrRecoveryRequired
	}
	exact, err := s.repo.ReadWorktreeRevision(ctx, observed)
	if err != nil || validateCommitted(exact) != nil || !equalLiveStates(exact.Projection.States, state.Shared) {
		return ErrRecoveryRequired
	}
	state.BaseOID = observed
	state.Committed = model.NewCommittedWorktree(exact.Source, exact.Projection)
	if err := s.registry.Seed(state.Committed.Source); err != nil {
		return ErrRecoveryRequired
	}
	return nil
}

func (s *Service) replaceWorkflowGeneration(state *State, branch, oid string, document model.CommittedWorktree, operation string) error {
	target := model.CloneLiveStates(document.Projection.States)
	changes, err := DiffSnapshot(model.LiveSnapshot{States: state.Shared}, model.LiveSnapshot{States: target})
	if err != nil {
		return err
	}
	state.Branch = branch
	state.BaseOID = oid
	state.Committed = model.NewCommittedWorktree(document.Source, document.Projection)
	state.Shared = target
	if err := s.registry.Seed(state.Committed.Source); err != nil {
		return ErrRecoveryRequired
	}
	if len(changes) != 0 {
		next, err := model.NextRevision(state.HeadRevision)
		if err != nil {
			return err
		}
		state.HeadRevision = next
		state.Events = append(state.Events, StateEvent{
			Revision: next, OperationID: fmt.Sprintf("workflow-%s:%d", operation, next), Changes: model.CloneLiveChanges(changes),
		})
		if err := CompactStateHistory(state); err != nil {
			return err
		}
	}
	for shellID, shell := range state.Shells {
		shell.Behind = shell.AppliedRevision < state.HeadRevision || !equalLiveStates(shell.AppliedBaseline, state.Shared) || shell.Conflict != nil
		shell.LastActiveRevision = state.HeadRevision
		state.Shells[shellID] = shell
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
	verifier := model.ShellCapabilityVerifier(sha256.Sum256([]byte("zsh-pro/worktree-missing-shell-verifier/v1")))
	if ok {
		verifier = shell.CapabilityVerifier
	}
	verified := model.VerifyShellCapability(verifier, credential.Capability)
	if !ok || !verified {
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
