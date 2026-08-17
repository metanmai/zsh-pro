package worktree

import (
	"errors"
	"reflect"
	"strings"

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
	AdmissionPolicyMissing   AdmissionReason = "policy-missing"
	AdmissionNotAttached     AdmissionReason = "not-attached"
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
	registry        *Registry
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

// Registry owns exact live identities and the classifier policy used before a
// newly observed identity may enter any value-bearing live-state path.
type Registry struct {
	policy       shell.LiveSecretPolicy
	owned        map[model.Identity]struct{}
	pinnedSecret map[model.Identity]struct{}
}

func NewRegistry(policy shell.LiveSecretPolicy) *Registry {
	if isNilLiveSecretPolicy(policy) {
		policy = nil
	}
	return &Registry{
		policy:       policy,
		owned:        make(map[model.Identity]struct{}),
		pinnedSecret: make(map[model.Identity]struct{}),
	}
}

// Seed replaces registry ownership from the final source occurrence of each
// supported identity. Only materialized entries that are both effectively
// managed and representable become owned. The replacement is all-or-nothing.
func (registry *Registry) Seed(profile model.Profile) error {
	if registry == nil {
		return errors.New("live registry is unavailable")
	}

	type seedDecision struct {
		owned bool
		pin   bool
	}
	final := make(map[model.Identity]seedDecision)
	for _, entry := range profile.Entries {
		if entry.Secret != nil {
			if err := validateRegistrySecretRef(entry); err != nil {
				return err
			}
		}

		identities := registryEntryIdentities(entry)
		for _, identity := range identities {
			decision := seedDecision{}
			if entry.EffectiveManaged() && entry.Representable() && model.ValidateIdentity(identity) == nil {
				decision.owned = true
				decision.pin = entry.Secret != nil
			}
			final[identity] = decision
		}
	}

	owned := make(map[model.Identity]struct{}, len(final))
	pinned := make(map[model.Identity]struct{})
	for identity, decision := range final {
		if !decision.owned {
			continue
		}
		owned[identity] = struct{}{}
		if decision.pin {
			pinned[identity] = struct{}{}
		}
	}
	registry.owned = owned
	registry.pinnedSecret = pinned
	return nil
}

func (registry *Registry) Owns(identity model.Identity) bool {
	if registry == nil {
		return false
	}
	_, owned := registry.owned[identity]
	return owned
}

func (registry *Registry) IsPinnedSecret(identity model.Identity) bool {
	if registry == nil {
		return false
	}
	_, pinned := registry.pinnedSecret[identity]
	return pinned
}

// AllowsLiveValue is the single eligibility check for value-bearing baseline,
// overlay, event, patch, and projection paths. Pinned or otherwise excluded
// identities remain owned as metadata but can never carry a live value.
func (registry *Registry) AllowsLiveValue(identity model.Identity) bool {
	return registry != nil && registry.Owns(identity) && registry.exclusionReason(identity) == ""
}

// Attach validates the complete initial snapshot, then retains values only for
// exact profile-owned, non-secret identities. All other present identities are
// remembered by identity and value-free reason only.
func (registry *Registry) Attach(snapshot model.LiveSnapshot) (AttachmentResult, error) {
	if registry == nil {
		return AttachmentResult{}, errors.New("live registry is unavailable")
	}
	if err := model.ValidateLiveSnapshot(snapshot); err != nil {
		return AttachmentResult{}, err
	}

	last := make(map[model.Identity]int, len(snapshot.States))
	for index, state := range snapshot.States {
		last[state.Identity] = index
	}
	result := AttachmentResult{
		presentAtAttach: make(map[model.Identity]struct{}, len(last)),
		registry:        registry,
	}
	for index, state := range snapshot.States {
		if last[state.Identity] != index {
			continue
		}
		if !state.Value.Present {
			if registry.AllowsLiveValue(state.Identity) {
				result.baseline = append(result.baseline, cloneLiveState(state))
			}
			continue
		}

		result.presentAtAttach[state.Identity] = struct{}{}
		if reason := registry.exclusionReason(state.Identity); reason != "" {
			result.exclusions = append(result.exclusions, model.Exclusion{Identity: state.Identity, Reason: string(reason)})
			continue
		}
		if registry.AllowsLiveValue(state.Identity) {
			result.baseline = append(result.baseline, cloneLiveState(state))
			continue
		}
		result.exclusions = append(result.exclusions, model.Exclusion{
			Identity: state.Identity,
			Reason:   string(AdmissionPresentAtAttach),
		})
	}
	return result, nil
}

// Admit adds one safe identity only when this registry produced the attachment
// result and the exact category/name was absent from its initial snapshot.
func (registry *Registry) Admit(attachment AttachmentResult, identity model.Identity) AdmissionResult {
	result := AdmissionResult{Identity: identity}
	if registry == nil || attachment.registry != registry {
		result.Reason = AdmissionNotAttached
		return result
	}
	if reason := registry.exclusionReason(identity); reason != "" {
		result.Reason = reason
		return result
	}
	if attachment.PresentAtAttach(identity) {
		result.Reason = AdmissionPresentAtAttach
		return result
	}
	if registry.owned == nil {
		registry.owned = make(map[model.Identity]struct{})
	}
	registry.owned[identity] = struct{}{}
	result.Admitted = true
	return result
}

func (registry *Registry) exclusionReason(identity model.Identity) AdmissionReason {
	switch identity.Kind {
	case model.LiveEnv, model.LiveAlias, model.LiveFunction, model.LivePath, model.LiveFPath, model.LiveOption:
	default:
		return AdmissionUnsupported
	}
	if model.ValidateIdentity(identity) != nil {
		return AdmissionUnsafeName
	}
	if registry.IsPinnedSecret(identity) {
		return AdmissionPinnedSecret
	}
	if isBookkeepingIdentity(identity) {
		return AdmissionBookkeeping
	}
	if isVolatileIdentity(identity) {
		return AdmissionVolatile
	}
	if registry.policy == nil {
		return AdmissionPolicyMissing
	}
	if registry.policy.IsLiveSecretIdentity(identity) {
		return AdmissionLiveSecret
	}
	return ""
}

func registryEntryIdentities(entry model.Entry) []model.Identity {
	identities := make([]model.Identity, 0, len(entry.Names))
	for _, name := range entry.Names {
		var kind model.LiveKind
		switch entry.Kind {
		case model.KindAssignment:
			switch name {
			case "PATH":
				kind = model.LivePath
			case "FPATH":
				kind = model.LiveFPath
			default:
				kind = model.LiveEnv
			}
		case model.KindAlias:
			kind = model.LiveAlias
		case model.KindFuncDecl:
			kind = model.LiveFunction
		case model.KindCommand:
			if entry.CmdName != "setopt" && entry.CmdName != "unsetopt" {
				continue
			}
			kind = model.LiveOption
		default:
			continue
		}
		identities = append(identities, model.Identity{Kind: kind, Name: name})
	}
	return identities
}

func validateRegistrySecretRef(entry model.Entry) error {
	ref := entry.Secret
	identity := model.Identity{Kind: model.LiveEnv, Name: refKey(ref)}
	if ref == nil || entry.Kind != model.KindAssignment || entry.Category != model.CatSecrets ||
		len(entry.Names) != 1 || entry.Names[0] == "" || ref.Key != entry.Names[0] ||
		(ref.Kind != model.SecretRefKeychain && ref.Kind != model.SecretRefFile) ||
		!entry.EffectiveManaged() || !entry.Representable() || entry.Dynamic ||
		entry.ValueMode != model.ValueModeUnsupported || entry.RuntimeValue != nil ||
		model.ValidateIdentity(identity) != nil {
		return errors.New("materialized SecretRef is invalid")
	}
	return nil
}

func isBookkeepingIdentity(identity model.Identity) bool {
	upper := strings.ToUpper(identity.Name)
	return strings.HasPrefix(upper, "ZP_") || strings.HasPrefix(upper, "_ZP_") ||
		strings.HasPrefix(upper, "ZSHPRO_") || strings.EqualFold(identity.Name, "zsh-pro")
}

func isVolatileIdentity(identity model.Identity) bool {
	if identity.Kind != model.LiveEnv {
		return false
	}
	switch identity.Name {
	case "_", "ARGC", "COLUMNS", "EPOCHREALTIME", "EPOCHSECONDS", "HISTCMD", "LINENO",
		"LINES", "OLDPWD", "PPID", "PWD", "RANDOM", "SECONDS", "SHLVL", "TTY", "TTYIDLE",
		"pipestatus", "status", "ZSH_SUBSHELL":
		return true
	default:
		return false
	}
}

func cloneLiveState(state model.LiveIdentityState) model.LiveIdentityState {
	return model.LiveIdentityState{Identity: state.Identity, Value: model.CloneLiveValue(state.Value)}
}

func refKey(ref *model.SecretRef) string {
	if ref == nil {
		return ""
	}
	return ref.Key
}

func isNilLiveSecretPolicy(policy shell.LiveSecretPolicy) bool {
	if policy == nil {
		return true
	}
	value := reflect.ValueOf(policy)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}
