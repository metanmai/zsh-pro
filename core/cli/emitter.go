package cli

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"

	"zsh-pro/core/activate"
	"zsh-pro/core/model"
	"zsh-pro/core/shell"
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
	Acknowledge(context.Context, model.ShellCredential, string, uint64, model.ResolutionToken, []byte) (model.AcknowledgeResult, error)
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
	service runtimeWorktreeService
	decoder shell.LiveSnapshotDecoder
	emitter RuntimeTransitionEmitter
}

func (*runtimeWorktreeAdapter) Attach(_ context.Context, credential model.ShellCredential, _ string, _ []byte) (runtimeAttachCredential, error) {
	return runtimeAttachCredential{result: model.AttachResult{}, credential: credential}, errors.New("runtime worktree attach is not implemented")
}

func (*runtimeWorktreeAdapter) Publish(context.Context, model.ShellCredential, string, uint64, model.LiveSnapshot, []byte) (model.PublishResult, error) {
	return model.PublishResult{}, errors.New("runtime worktree publish is not implemented")
}

func (*runtimeWorktreeAdapter) Prepare(context.Context, model.ShellCredential, string, uint64, []byte, string, string) (runtimePatchPayload, error) {
	return runtimePatchPayload{}, errors.New("runtime worktree prepare is not implemented")
}

func (*runtimeWorktreeAdapter) Acknowledge(context.Context, model.ShellCredential, string, uint64, model.ResolutionToken, []byte) (model.AcknowledgeResult, error) {
	return model.AcknowledgeResult{}, errors.New("runtime worktree acknowledge is not implemented")
}

func (*runtimeWorktreeAdapter) Resolve(context.Context, model.ShellCredential, string, model.Identity, model.ResolutionToken, []byte, string, string) (runtimePatchPayload, error) {
	return runtimePatchPayload{}, errors.New("runtime worktree resolve is not implemented")
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
