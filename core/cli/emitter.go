package cli

import (
	"context"
	"errors"
	"fmt"

	"zsh-pro/core/activate"
	"zsh-pro/core/shell"
)

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
	store    Store
	emit     shell.Emitter
	resolver SecretResolver
}

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

func (r runtimeEmitter) Emit(ctx context.Context, mode, name string) (string, error) {
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
	if mode == "deactivate" {
		plan, err := activate.Diff(&targetManifest, nil)
		if err != nil {
			return "", err
		}
		_, deactivate, err := r.emit.Emit(plan)
		if err != nil {
			return "", err
		}
		return deactivate + "\nzp_deactivate\n", nil
	}

	applyPlan, err := activate.Diff(nil, &targetManifest)
	if err != nil {
		return "", err
	}
	reversePlan, err := activate.Diff(&targetManifest, nil)
	if err != nil {
		return "", err
	}
	apply, _, err := r.emit.Emit(applyPlan)
	if err != nil {
		return "", err
	}
	_, reverse, err := r.emit.Emit(reversePlan)
	if err != nil {
		return "", err
	}
	// The caller retains this target-specific reverse in its current shell.
	// A later target switch invokes it before the later payload replaces it.
	return reverse + "\n" + apply + "\nzp_apply\n", nil
}
