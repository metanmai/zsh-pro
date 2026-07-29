package cli

import (
	"context"
	"crypto/rand"
	"encoding/hex"
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

type runtimeFunctionNames struct {
	apply, deactivate string
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
