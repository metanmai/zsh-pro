// Package worktree owns shell-neutral shared-worktree policy and transforms.
package worktree

import (
	"errors"
	"zsh-pro/core/model"
)

func ValidateSnapshot(model.LiveSnapshot) error {
	return errors.New("snapshot validation not implemented")
}

func DiffSnapshot(model.LiveSnapshot, model.LiveSnapshot) ([]model.LiveChange, error) {
	return nil, errors.New("snapshot diff not implemented")
}

func ApplyOverlay([]model.LiveIdentityState, []model.OverlayEntry) ([]model.LiveIdentityState, error) {
	return nil, errors.New("overlay application not implemented")
}

func BuildProjection([]model.LiveIdentityState, []model.OverlayEntry) (model.LiveProjection, error) {
	return model.LiveProjection{}, errors.New("projection construction not implemented")
}

func CategorizeDiff([]model.LiveChange) (model.CategorizedDiff, error) {
	return model.CategorizedDiff{}, errors.New("diff categorization not implemented")
}
