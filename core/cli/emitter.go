package cli

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"

	"zsh-pro/core/activate"
	"zsh-pro/core/model"
	"zsh-pro/core/shell"
	"zsh-pro/core/worktree"
)

// RuntimePatchMetadata is the value-free public description of one pending
// current-shell transition. The executable source remains in a private type.
type RuntimePatchMetadata struct {
	Revision    uint64
	Token       uint64
	ChangeCount uint64
	Fingerprint [sha256.Size]byte
}

type runtimePatchPayload struct {
	transition bool
	source     []byte
	metadata   RuntimePatchMetadata
}

type runtimeAttachCredential struct {
	result     model.AttachResult
	credential model.ShellCredential
}

// RuntimeTransitionEmitter is consumer-owned; concrete shells implement this
// exact transport without importing the CLI package.
type RuntimeTransitionEmitter interface {
	EmitRuntimeTransition(forward, replacementReverse []activate.Op, applyName, reverseName string, revision, token uint64, fingerprint string) ([]byte, error)
}

// RuntimeWorktree is deliberately limited to the five credential-bearing
// shell runtime operations.
type RuntimeWorktree interface {
	Attach(context.Context, model.ShellCredential, string, []byte) (runtimeAttachCredential, error)
	Publish(context.Context, model.ShellCredential, string, uint64, model.LiveSnapshot, []byte) (model.PublishResult, error)
	Prepare(context.Context, model.ShellCredential, string, uint64, []byte, string, string) (runtimePatchPayload, error)
	Acknowledge(context.Context, model.ShellCredential, string, uint64, uint64, []byte) (model.AcknowledgeResult, error)
	Resolve(context.Context, model.ShellCredential, string, model.Identity, model.ResolutionToken, []byte, string, string) (runtimePatchPayload, error)
}

type runtimeWorktreeService interface {
	Attach(context.Context, model.AttachRequest) (model.AttachResult, error)
	Publish(context.Context, model.PublishRequest) (model.PublishResult, error)
	PreparePull(context.Context, model.PreparePullRequest) (model.PreparePullResult, error)
	Acknowledge(context.Context, model.AcknowledgeRequest) (model.AcknowledgeResult, error)
	ResolveShared(context.Context, model.ResolveSharedRequest) (model.ResolveSharedResult, error)
}

type runtimeWorktreeAdapter struct {
	service     runtimeWorktreeService
	decoder     shell.LiveSnapshotDecoder
	emitter     RuntimeTransitionEmitter
	tokenSource runtimeResolutionTokenSource
}

type runtimeResolutionTokenSource interface {
	resolveRuntimeToken(context.Context, string, uint64, uint64) (model.ResolutionToken, error)
}

type runtimeStateResolutionTokenSource struct {
	state *worktree.StateStore
}

type runtimeStateStoreConstructor func(*os.File) (*worktree.StateStore, error)
type runtimeServiceConstructor func(*worktree.StateStore, *worktree.Registry) (runtimeWorktreeService, error)

// RuntimeWorktreeFactory retains constructors and shell policy only. Bind is
// the first point at which an authenticated RuntimeRoot becomes a StateStore,
// Registry, Service, and operation-scoped adapter.
type RuntimeWorktreeFactory struct {
	decoder    shell.LiveSnapshotDecoder
	emitter    RuntimeTransitionEmitter
	policy     shell.LiveSecretPolicy
	newState   runtimeStateStoreConstructor
	newService runtimeServiceConstructor
}

// NewRuntimeWorktreeFactory configures late descriptor-bound worktree wiring.
func NewRuntimeWorktreeFactory(decoder shell.LiveSnapshotDecoder, emitter RuntimeTransitionEmitter, policy shell.LiveSecretPolicy) RuntimeWorktreeFactory {
	return RuntimeWorktreeFactory{
		decoder:  decoder,
		emitter:  emitter,
		policy:   policy,
		newState: worktree.NewStateStoreFromAuthenticatedRoot,
		newService: func(store *worktree.StateStore, registry *worktree.Registry) (runtimeWorktreeService, error) {
			return worktree.NewService(store, registry)
		},
	}
}

// Bind duplicates the authenticated repository descriptor into a fresh state
// store. The returned closer owns only that duplicate; RuntimeRoot remains
// caller-owned and is never closed here.
func (factory RuntimeWorktreeFactory) Bind(root *RuntimeRoot) (RuntimeWorktree, io.Closer, error) {
	if root == nil || isNilLike(factory.decoder) || isNilLike(factory.emitter) || isNilLike(factory.policy) || factory.newState == nil || factory.newService == nil {
		return nil, nil, errors.New("runtime worktree factory is not configured")
	}
	repository, _ := root.Files()
	if repository == nil {
		return nil, nil, errors.New("runtime worktree root descriptor is unavailable")
	}
	state, err := factory.newState(repository)
	if err != nil {
		return nil, nil, err
	}
	service, err := factory.newService(state, worktree.NewRegistry(factory.policy))
	if err != nil {
		_ = state.Close()
		return nil, nil, err
	}
	return &runtimeWorktreeAdapter{
		service:     service,
		decoder:     factory.decoder,
		emitter:     factory.emitter,
		tokenSource: runtimeStateResolutionTokenSource{state: state},
	}, state, nil
}

func (runtime *runtimeWorktreeAdapter) Attach(ctx context.Context, credential model.ShellCredential, operationID string, frame []byte) (runtimeAttachCredential, error) {
	if err := runtime.ready(); err != nil {
		return runtimeAttachCredential{}, err
	}
	snapshot, err := runtime.decoder.DecodeLiveSnapshot(frame)
	if err != nil {
		return runtimeAttachCredential{}, err
	}
	result, err := runtime.service.Attach(ctx, model.AttachRequest{
		OperationID: operationID,
		Credential:  credential,
		Initial:     snapshot,
	})
	if err != nil {
		return runtimeAttachCredential{}, err
	}
	return runtimeAttachCredential{result: result, credential: credential}, nil
}

func (runtime *runtimeWorktreeAdapter) Publish(ctx context.Context, credential model.ShellCredential, operationID string, acknowledgedRevision uint64, baseline model.LiveSnapshot, frame []byte) (model.PublishResult, error) {
	if err := runtime.ready(); err != nil {
		return model.PublishResult{}, err
	}
	snapshot, err := runtime.decoder.DecodeLiveSnapshot(frame)
	if err != nil {
		return model.PublishResult{}, err
	}
	delta, err := worktree.DiffSnapshot(baseline, snapshot)
	if err != nil {
		return model.PublishResult{}, err
	}
	return runtime.service.Publish(ctx, model.PublishRequest{
		OperationID:          operationID,
		Credential:           credential,
		AcknowledgedRevision: acknowledgedRevision,
		Delta:                delta,
	})
}

func (runtime *runtimeWorktreeAdapter) Prepare(ctx context.Context, credential model.ShellCredential, operationID string, appliedRevision uint64, frame []byte, applyName, reverseName string) (runtimePatchPayload, error) {
	if err := runtime.ready(); err != nil {
		return runtimePatchPayload{}, err
	}
	current, err := runtime.decoder.DecodeLiveSnapshot(frame)
	if err != nil {
		return runtimePatchPayload{}, err
	}
	result, err := runtime.service.PreparePull(ctx, model.PreparePullRequest{
		OperationID:     operationID,
		Credential:      credential,
		AppliedRevision: appliedRevision,
	})
	if err != nil {
		return runtimePatchPayload{}, err
	}
	if result.PendingRevision == 0 {
		if result.Token != "" || len(result.Changes) != 0 {
			return runtimePatchPayload{}, errors.New("at-head prepare returned transition data")
		}
		return runtimePatchPayload{}, nil
	}
	return runtime.buildTransition(current, result.PendingRevision, result.Token, result.Changes, applyName, reverseName)
}

func (runtime *runtimeWorktreeAdapter) Acknowledge(ctx context.Context, credential model.ShellCredential, operationID string, revision, transportToken uint64, frame []byte) (model.AcknowledgeResult, error) {
	if err := runtime.ready(); err != nil {
		return model.AcknowledgeResult{}, err
	}
	if isNilLike(runtime.tokenSource) {
		return model.AcknowledgeResult{}, errors.New("runtime acknowledgement token source is unavailable")
	}
	snapshot, err := runtime.decoder.DecodeLiveSnapshot(frame)
	if err != nil {
		return model.AcknowledgeResult{}, err
	}
	serviceToken, err := runtime.tokenSource.resolveRuntimeToken(ctx, credential.ShellID, revision, transportToken)
	if err != nil {
		return model.AcknowledgeResult{}, err
	}
	return runtime.service.Acknowledge(ctx, model.AcknowledgeRequest{
		OperationID: operationID,
		Credential:  credential,
		Revision:    revision,
		Token:       serviceToken,
		Snapshot:    snapshot,
	})
}

func (runtime *runtimeWorktreeAdapter) Resolve(ctx context.Context, credential model.ShellCredential, operationID string, conflict model.Identity, token model.ResolutionToken, frame []byte, applyName, reverseName string) (runtimePatchPayload, error) {
	if err := runtime.ready(); err != nil {
		return runtimePatchPayload{}, err
	}
	current, err := runtime.decoder.DecodeLiveSnapshot(frame)
	if err != nil {
		return runtimePatchPayload{}, err
	}
	result, err := runtime.service.ResolveShared(ctx, model.ResolveSharedRequest{
		OperationID: operationID,
		Credential:  credential,
		Conflict:    conflict,
		Token:       token,
		Snapshot:    current,
	})
	if err != nil {
		return runtimePatchPayload{}, err
	}
	if result.PendingRevision == 0 {
		if result.Token != "" || len(result.Changes) != 0 {
			return runtimePatchPayload{}, errors.New("resolved transition omitted its revision")
		}
		return runtimePatchPayload{}, nil
	}
	return runtime.buildTransition(current, result.PendingRevision, result.Token, result.Changes, applyName, reverseName)
}

func (runtime *runtimeWorktreeAdapter) ready() error {
	if runtime == nil || isNilLike(runtime.service) || isNilLike(runtime.decoder) || isNilLike(runtime.emitter) {
		return errors.New("runtime worktree adapter is unavailable")
	}
	return nil
}

func (runtime *runtimeWorktreeAdapter) buildTransition(current model.LiveSnapshot, revision uint64, token model.ResolutionToken, changes []model.LiveChange, applyName, reverseName string) (runtimePatchPayload, error) {
	if revision == 0 {
		return runtimePatchPayload{}, errors.New("runtime transition revision is missing")
	}
	if err := token.Validate(); err != nil {
		return runtimePatchPayload{}, err
	}
	overlay := make([]model.OverlayEntry, 0, len(changes))
	for _, change := range changes {
		switch change.Kind {
		case model.LiveAdd, model.LiveUpdate:
			overlay = append(overlay, model.OverlayEntry{Identity: change.Identity, Value: model.CloneLiveValue(change.Value)})
		case model.LiveRemove:
			overlay = append(overlay, model.OverlayEntry{Identity: change.Identity, Tombstone: true})
		default:
			return runtimePatchPayload{}, fmt.Errorf("runtime transition change kind %q is invalid", change.Kind)
		}
	}
	targetStates, err := worktree.ApplyOverlay(current.States, overlay)
	if err != nil {
		return runtimePatchPayload{}, err
	}
	target := model.LiveSnapshot{States: targetStates}
	patch, err := activate.BuildLivePatch(current.States, targetStates)
	if err != nil {
		return runtimePatchPayload{}, err
	}
	fingerprint, err := worktree.FingerprintSnapshot(target)
	if err != nil {
		return runtimePatchPayload{}, err
	}
	transportToken := runtimeTransitionToken(token)
	if transportToken == 0 {
		return runtimePatchPayload{}, errors.New("runtime transition token cannot be represented")
	}
	metadata := RuntimePatchMetadata{
		Revision:    revision,
		Token:       transportToken,
		ChangeCount: uint64(len(changes)),
		Fingerprint: [sha256.Size]byte(fingerprint),
	}
	encodedFingerprint := hex.EncodeToString(metadata.Fingerprint[:])
	source, err := runtime.emitter.EmitRuntimeTransition(patch.Forward, patch.ReplacementReverse, applyName, reverseName, metadata.Revision, metadata.Token, encodedFingerprint)
	if err != nil {
		return runtimePatchPayload{}, err
	}
	return runtimePatchPayload{
		transition: true,
		source:     append([]byte(nil), source...),
		metadata:   metadata,
	}, nil
}

func runtimeTransitionToken(token model.ResolutionToken) uint64 {
	digest := sha256.Sum256([]byte(token))
	return binary.BigEndian.Uint64(digest[:8])
}

// resolveRuntimeToken reverses the fixed-size parent-shell handle through the
// descriptor-bound durable pending transition. It does not inspect a receipt,
// compare a capability, or authorize the shell; Service.Acknowledge still
// performs credential and opaque-token verification under its transaction.
func (source runtimeStateResolutionTokenSource) resolveRuntimeToken(ctx context.Context, shellID string, revision, transportToken uint64) (model.ResolutionToken, error) {
	if source.state == nil || shellID == "" || revision == 0 || transportToken == 0 {
		return "", errors.New("runtime acknowledgement token is invalid")
	}
	state, err := source.state.Read(ctx)
	if err != nil {
		return "", errors.New("runtime acknowledgement token is unavailable")
	}
	shellState, ok := state.Shells[shellID]
	if !ok || shellState.Pending.Kind == worktree.PendingNone || shellState.Pending.Revision != revision || shellState.Pending.Token.Validate() != nil || runtimeTransitionToken(shellState.Pending.Token) != transportToken {
		return "", errors.New("runtime acknowledgement token is invalid")
	}
	return shellState.Pending.Token, nil
}

// Emitter is the CLI-to-runtime-code seam. It returns a single emitted shell
// block for either apply or deactivate, never a concrete zsh implementation.
type Emitter interface {
	Emit(ctx context.Context, mode, name string) (string, error)
}

type notReadyEmitter struct{}

// NotReadyEmitter fails closed while a caller has no active emit pipeline.
func NotReadyEmitter() Emitter { return notReadyEmitter{} }

func (notReadyEmitter) Emit(context.Context, string, string) (string, error) {
	return "", errors.New("emit path not yet available")
}

type runtimeEmitter struct {
	store        Store
	emit         shell.Emitter
	resolver     SecretResolver
	runtimeStore RuntimeStoreFactory
}

type runtimeFunctionNames struct {
	apply, deactivate string
}

// RuntimeStoreFactory binds a runtime emission to descriptors that have already
// been authenticated by the private runtime helper. It returns a fresh
// read-only store and matching secret resolver for exactly that descriptor pair.
// The factory is supplied only by the composition root, which is allowed to
// wire concrete store implementations into this CLI-local seam.
type RuntimeStoreFactory func(*RuntimeRoot) (Store, SecretResolver, error)

// NewRuntimeEmitter adapts the Phase 4 profile -> manifest -> plan -> source
// pipeline to the CLI-local Emitter interface. It leaves all zsh generation in
// the injected shell emitter.
func NewRuntimeEmitter(s Store, e shell.Emitter, resolvers ...SecretResolver) Emitter {
	if isNilLike(s) || isNilLike(e) {
		return NotReadyEmitter()
	}
	var resolver SecretResolver
	if len(resolvers) > 0 && !isNilLike(resolvers[0]) {
		resolver = resolvers[0]
	}
	return runtimeEmitter{store: s, emit: e, resolver: resolver}
}

// NewRuntimeEmitterWithRuntimeStore returns an emitter that serves ordinary
// `emit` calls through s and sourced-loader capture through factory. The latter
// never reuses s: the loader's helper must emit from the exact RuntimeRoot it
// authenticated rather than a mutable pathname captured at process startup.
func NewRuntimeEmitterWithRuntimeStore(s Store, e shell.Emitter, resolver SecretResolver, factory RuntimeStoreFactory) Emitter {
	if isNilLike(e) || factory == nil {
		return NotReadyEmitter()
	}
	if isNilLike(s) {
		s = nil
	}
	if isNilLike(resolver) {
		resolver = nil
	}
	return runtimeEmitter{store: s, emit: e, resolver: resolver, runtimeStore: factory}
}

func (r runtimeEmitter) Emit(ctx context.Context, mode, name string) (string, error) {
	if isNilLike(r.store) {
		return "", errors.New("profile store unavailable")
	}
	if mode != "apply" && mode != "deactivate" {
		return "", fmt.Errorf("unknown emit mode %q", mode)
	}
	if name == "" {
		return "", errors.New("profile name is required")
	}
	if mode == "apply" {
		if err := r.store.Checkout(ctx, name); err != nil {
			return "", err
		}
	}
	target, err := r.store.Read(ctx, name)
	if err != nil {
		return "", err
	}
	target, err = resolveSecretRefs(target, r.resolver)
	if err != nil {
		return "", err
	}
	targetManifest := activate.Build(target)
	names, err := newRuntimeFunctionNames()
	if err != nil {
		return "", err
	}
	emitter, ok := r.emit.(shell.RuntimeEmitter)
	if !ok {
		return "", errors.New("runtime emitter does not support secure payload emission")
	}
	if mode == "deactivate" {
		plan, err := activate.Diff(&targetManifest, nil)
		if err != nil {
			return "", err
		}
		_, deactivate, err := emitter.EmitRuntime(plan, names.apply, names.deactivate)
		if err != nil {
			return "", err
		}
		return runtimeDeactivatePayload(deactivate, names), nil
	}

	applyPlan, err := activate.Diff(nil, &targetManifest)
	if err != nil {
		return "", err
	}
	reversePlan, err := activate.Diff(&targetManifest, nil)
	if err != nil {
		return "", err
	}
	apply, _, err := emitter.EmitRuntime(applyPlan, names.apply, names.deactivate)
	if err != nil {
		return "", err
	}
	_, reverse, err := emitter.EmitRuntime(reversePlan, names.apply, names.deactivate)
	if err != nil {
		return "", err
	}
	return runtimeApplyPayload(reverse, apply, names), nil
}

// emitFromRuntimeRoot emits from a freshly bound Store. It intentionally keeps
// the store local to this call so neither profile reads nor secret resolution
// can fall back to the composition root's path-based store after validation.
func (r runtimeEmitter) emitFromRuntimeRoot(ctx context.Context, root *RuntimeRoot, mode, name string) (string, error) {
	if r.runtimeStore == nil {
		return "", errors.New("runtime emitter is not descriptor-bound")
	}
	s, resolver, err := r.runtimeStore(root)
	if err != nil {
		return "", err
	}
	return runtimeEmitter{store: s, emit: r.emit, resolver: resolver}.Emit(ctx, mode, name)
}

// listFromRuntimeRoot reads branch names from the same descriptor-bound Store
// used by runtime capture. It prevents the private helper's list path from
// silently reopening ZSHPRO_HOME by name after its safety check.
func (r runtimeEmitter) listFromRuntimeRoot(ctx context.Context, root *RuntimeRoot) ([]string, error) {
	if r.runtimeStore == nil {
		return nil, errors.New("runtime emitter is not descriptor-bound")
	}
	s, _, err := r.runtimeStore(root)
	if err != nil {
		return nil, err
	}
	return s.Branches(ctx)
}

func newRuntimeFunctionNames() (runtimeFunctionNames, error) {
	var token [16]byte
	if _, err := rand.Read(token[:]); err != nil {
		return runtimeFunctionNames{}, fmt.Errorf("generate runtime function names: %w", err)
	}
	suffix := hex.EncodeToString(token[:])
	return runtimeFunctionNames{
		apply:      "__zp_apply_" + suffix,
		deactivate: "__zp_deactivate_" + suffix,
	}, nil
}

func runtimeApplyPayload(reverse, apply string, names runtimeFunctionNames) string {
	return fmt.Sprintf(`if (( ${+functions[%[1]s]} || ${+functions[%[2]s]} )); then
  _zp_runtime_error 1 "generated runtime function collision"
  false
else
%[3]s
%[4]s
  _zp_run_payload %[1]s %[2]s
fi
`, names.apply, names.deactivate, reverse, apply)
}

func runtimeDeactivatePayload(reverse string, names runtimeFunctionNames) string {
	return fmt.Sprintf(`if (( ${+functions[%[1]s]} )); then
  _zp_runtime_error 1 "generated runtime function collision"
  false
else
%[2]s
  _zp_run_transient_reverse_payload %[1]s
fi
`, names.deactivate, reverse)
}
