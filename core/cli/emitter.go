package cli

import (
	"context"
	"errors"
	"fmt"

	"zsh-pro/core/activate"
	"zsh-pro/core/model"
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
	store Store
	emit  shell.Emitter
}

// NewRuntimeEmitter adapts the Phase 4 profile -> manifest -> plan -> source
// pipeline to the CLI-local Emitter interface. It leaves all zsh generation in
// the injected shell emitter.
func NewRuntimeEmitter(s Store, e shell.Emitter) Emitter {
	if s == nil || e == nil {
		return NotReadyEmitter()
	}
	return runtimeEmitter{store: s, emit: e}
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

	var activeManifest *model.Manifest
	if current := r.store.Current(); current != "" && current != "main" && current != name {
		active, err := r.store.Read(ctx, current)
		if err != nil {
			return "", err
		}
		m := activate.Build(active)
		activeManifest = &m
	}
	plan, err := activate.Diff(activeManifest, &targetManifest)
	if err != nil {
		return "", err
	}
	apply, deactivate, err := r.emit.Emit(plan)
	if err != nil {
		return "", err
	}
	if activeManifest != nil {
		// The loader validates and evaluates this exact one-source transaction.
		// Define both halves first, then run the active-profile cleanup before the
		// target apply so no A-only identity can escape into B.
		return deactivate + "\n" + apply + "\nzp_deactivate\nzp_apply\n", nil
	}
	return apply + "\nzp_apply\n", nil
}
