package worktree

import (
	"zsh-pro/core/model"
	"zsh-pro/core/shell"
)

// AdmissionReason is a stable, value-free explanation for an identity that
// cannot cross the live-state ownership boundary.
type AdmissionReason string

const (
	AdmissionUnsupported     AdmissionReason = "unsupported"
	AdmissionUnsafeName      AdmissionReason = "unsafe-name"
	AdmissionBookkeeping     AdmissionReason = "bookkeeping"
	AdmissionVolatile        AdmissionReason = "volatile"
	AdmissionLiveSecret      AdmissionReason = "live-secret"
	AdmissionPinnedSecret    AdmissionReason = "pinned-secret"
	AdmissionPresentAtAttach AdmissionReason = "present-at-attach"
)

// AdmissionResult contains identity metadata only; captured values cannot be
// represented in an admission decision.
type AdmissionResult struct {
	Identity model.Identity
	Admitted bool
	Reason   AdmissionReason
}

// AttachmentResult keeps its slices and attachment set private so callers can
// only obtain defensive copies or exact membership answers.
type AttachmentResult struct {
	baseline        []model.LiveIdentityState
	exclusions      []model.Exclusion
	presentAtAttach map[model.Identity]struct{}
}

func (result AttachmentResult) Baseline() []model.LiveIdentityState {
	return model.CloneLiveStates(result.baseline)
}

func (result AttachmentResult) Exclusions() []model.Exclusion {
	if result.exclusions == nil {
		return nil
	}
	return append([]model.Exclusion(nil), result.exclusions...)
}

func (result AttachmentResult) PresentAtAttach(identity model.Identity) bool {
	_, present := result.presentAtAttach[identity]
	return present
}

// Registry is introduced as a compile-safe RED scaffold. Its ownership and
// admission behavior is implemented after the contract tests fail.
type Registry struct {
	policy shell.LiveSecretPolicy
}

func NewRegistry(policy shell.LiveSecretPolicy) *Registry { return &Registry{policy: policy} }
func (registry *Registry) Seed(model.Profile) error       { return nil }
func (registry *Registry) Owns(model.Identity) bool       { return false }
func (registry *Registry) IsPinnedSecret(model.Identity) bool {
	return false
}
func (registry *Registry) Attach(model.LiveSnapshot) (AttachmentResult, error) {
	return AttachmentResult{}, nil
}
func (registry *Registry) Admit(_ AttachmentResult, identity model.Identity) AdmissionResult {
	return AdmissionResult{Identity: identity, Reason: AdmissionUnsupported}
}
