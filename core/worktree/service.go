package worktree

import (
	"context"
	"errors"

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

type Service struct{}

func NewService(*StateStore, *Registry) (*Service, error) {
	return nil, errors.New("worktree service not implemented")
}

func (*Service) Materialize(context.Context, string, string, model.CommittedWorktree) error {
	return errors.New("worktree service not implemented")
}

func (*Service) Attach(context.Context, model.AttachRequest) (model.AttachResult, error) {
	return model.AttachResult{}, errors.New("worktree service not implemented")
}

func (*Service) Publish(context.Context, model.PublishRequest) (model.PublishResult, error) {
	return model.PublishResult{}, errors.New("worktree service not implemented")
}

func (*Service) PreparePull(context.Context, model.PreparePullRequest) (model.PreparePullResult, error) {
	return model.PreparePullResult{}, errors.New("worktree service not implemented")
}

func (*Service) Acknowledge(context.Context, model.AcknowledgeRequest) (model.AcknowledgeResult, error) {
	return model.AcknowledgeResult{}, errors.New("worktree service not implemented")
}

func (*Service) ResolveShared(context.Context, model.ResolveSharedRequest) (model.ResolveSharedResult, error) {
	return model.ResolveSharedResult{}, errors.New("worktree service not implemented")
}

func (*Service) Status(context.Context, string) (model.WorktreeStatus, error) {
	return model.WorktreeStatus{}, errors.New("worktree service not implemented")
}

func (*Service) Diff(context.Context) (model.CategorizedDiff, error) {
	return model.CategorizedDiff{}, errors.New("worktree service not implemented")
}
