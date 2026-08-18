package worktree

import (
	"fmt"
	"reflect"
	"testing"

	"zsh-pro/core/model"
	"zsh-pro/core/shell/zsh"
)

type fakeLiveSecretPolicy map[model.Identity]bool

func (policy fakeLiveSecretPolicy) IsLiveSecretIdentity(identity model.Identity) bool {
	return policy[identity]
}

type pointerLiveSecretPolicy struct{}

func (*pointerLiveSecretPolicy) IsLiveSecretIdentity(model.Identity) bool { return false }

func TestRegistrySeedOwnsOnlyFinalManagedRepresentable(t *testing.T) {
	pinned := materializedSecretEntry("PINNED_TOKEN")
	profile := model.Profile{Entries: []model.Entry{
		materializedAssignment("EDITOR", model.CatEnvironment),
		materializedAssignment("SHADOWED", model.CatEnvironment),
		{Kind: model.KindAssignment, Names: []string{"UNREPRESENTABLE"}, Managed: true},
		materializedAlias("gs"),
		materializedFunction("greet"),
		materializedAssignment("PATH", model.CatPath),
		materializedAssignment("FPATH", model.CatPath),
		materializedOptions("setopt", "AUTO_CD", "NOMATCH"),
		pinned,
		unmanagedAssignment("SHADOWED"),
	}}

	registry := NewRegistry(fakeLiveSecretPolicy{})
	if err := registry.Seed(profile); err != nil {
		t.Fatalf("Seed() error = %v", err)
	}

	owned := []model.Identity{
		{Kind: model.LiveEnv, Name: "EDITOR"},
		{Kind: model.LiveAlias, Name: "gs"},
		{Kind: model.LiveFunction, Name: "greet"},
		{Kind: model.LivePath, Name: "PATH"},
		{Kind: model.LiveFPath, Name: "FPATH"},
		{Kind: model.LiveOption, Name: "AUTO_CD"},
		{Kind: model.LiveOption, Name: "NOMATCH"},
		{Kind: model.LiveEnv, Name: "PINNED_TOKEN"},
	}
	for _, identity := range owned {
		if !registry.Owns(identity) {
			t.Errorf("registry does not own %#v", identity)
		}
	}
	for _, identity := range []model.Identity{
		{Kind: model.LiveEnv, Name: "UNREPRESENTABLE"},
		{Kind: model.LiveEnv, Name: "SHADOWED"},
	} {
		if registry.Owns(identity) {
			t.Errorf("registry unexpectedly owns %#v", identity)
		}
	}
	pinnedIdentity := model.Identity{Kind: model.LiveEnv, Name: "PINNED_TOKEN"}
	if !registry.IsPinnedSecret(pinnedIdentity) {
		t.Fatalf("SecretRef identity %#v is not pinned", pinnedIdentity)
	}
	if registry.AllowsLiveValue(pinnedIdentity) {
		t.Fatalf("pinned identity %#v is eligible for a live value", pinnedIdentity)
	}
	if editor := (model.Identity{Kind: model.LiveEnv, Name: "EDITOR"}); !registry.AllowsLiveValue(editor) {
		t.Fatalf("ordinary managed identity %#v is not eligible for a live value", editor)
	}
}

func TestPinnedSecretNeverEntersAttachmentValues(t *testing.T) {
	const pinnedCanary = "pinned-resolved-literal-canary"
	const inheritedCanary = "inherited-unmanaged-canary"
	registry := NewRegistry(fakeLiveSecretPolicy{})
	if err := registry.Seed(model.Profile{Entries: []model.Entry{
		materializedAssignment("EDITOR", model.CatEnvironment),
		materializedSecretEntry("PINNED_TOKEN"),
	}}); err != nil {
		t.Fatalf("Seed() error = %v", err)
	}

	attachment, err := registry.Attach(model.LiveSnapshot{States: []model.LiveIdentityState{
		liveScalar(model.LiveEnv, "EDITOR", "nvim"),
		liveScalar(model.LiveEnv, "PINNED_TOKEN", pinnedCanary),
		liveScalar(model.LiveEnv, "INHERITED", inheritedCanary),
	}})
	if err != nil {
		t.Fatalf("Attach() error = %v", err)
	}

	wantBaseline := []model.LiveIdentityState{liveScalar(model.LiveEnv, "EDITOR", "nvim")}
	if got := attachment.Baseline(); !reflect.DeepEqual(got, wantBaseline) {
		t.Fatalf("Baseline() = %#v, want %#v", got, wantBaseline)
	}
	wantExclusions := []model.Exclusion{
		{Identity: model.Identity{Kind: model.LiveEnv, Name: "PINNED_TOKEN"}, Reason: string(AdmissionPinnedSecret)},
		{Identity: model.Identity{Kind: model.LiveEnv, Name: "INHERITED"}, Reason: string(AdmissionPresentAtAttach)},
	}
	if got := attachment.Exclusions(); !reflect.DeepEqual(got, wantExclusions) {
		t.Fatalf("Exclusions() = %#v, want %#v", got, wantExclusions)
	}
	if rendered := fmt.Sprintf("%#v", attachment); containsAny(rendered, pinnedCanary, inheritedCanary) {
		t.Fatalf("attachment retained excluded value: %s", rendered)
	}

	baseline := attachment.Baseline()
	baseline[0].Identity.Name = "MUTATED"
	exclusions := attachment.Exclusions()
	exclusions[0].Reason = "mutated"
	if got := attachment.Baseline(); !reflect.DeepEqual(got, wantBaseline) {
		t.Fatalf("Baseline() was mutable through returned slice: %#v", got)
	}
	if got := attachment.Exclusions(); !reflect.DeepEqual(got, wantExclusions) {
		t.Fatalf("Exclusions() was mutable through returned slice: %#v", got)
	}
	if !attachment.PresentAtAttach(model.Identity{Kind: model.LiveEnv, Name: "INHERITED"}) ||
		attachment.PresentAtAttach(model.Identity{Kind: model.LiveAlias, Name: "INHERITED"}) {
		t.Fatal("PresentAtAttach is not exact by kind and name")
	}
}

func TestAdmissionOnlyAllowsSafeAbsentAtAttach(t *testing.T) {
	secretIdentity := model.Identity{Kind: model.LiveEnv, Name: "SPECIAL_SECRET"}
	registry := NewRegistry(fakeLiveSecretPolicy{secretIdentity: true})
	if err := registry.Seed(model.Profile{Entries: []model.Entry{materializedSecretEntry("PINNED_TOKEN")}}); err != nil {
		t.Fatalf("Seed() error = %v", err)
	}
	attachment, err := registry.Attach(model.LiveSnapshot{States: []model.LiveIdentityState{
		liveScalar(model.LiveEnv, "INHERITED", "ordinary"),
	}})
	if err != nil {
		t.Fatalf("Attach() error = %v", err)
	}

	tests := []struct {
		name     string
		identity model.Identity
		admitted bool
		reason   AdmissionReason
	}{
		{name: "safe new env", identity: model.Identity{Kind: model.LiveEnv, Name: "NEW_EDITOR"}, admitted: true},
		{name: "safe new alias", identity: model.Identity{Kind: model.LiveAlias, Name: "new-alias"}, admitted: true},
		{name: "inherited remains unmanaged", identity: model.Identity{Kind: model.LiveEnv, Name: "INHERITED"}, reason: AdmissionPresentAtAttach},
		{name: "volatile absent env", identity: model.Identity{Kind: model.LiveEnv, Name: "PWD"}, reason: AdmissionVolatile},
		{name: "bookkeeping env", identity: model.Identity{Kind: model.LiveEnv, Name: "ZP_PRIVATE_STATE"}, reason: AdmissionBookkeeping},
		{name: "bookkeeping function", identity: model.Identity{Kind: model.LiveFunction, Name: "_zp_runtime_helper"}, reason: AdmissionBookkeeping},
		{name: "dynamic reverse bookkeeping", identity: model.Identity{Kind: model.LiveFunction, Name: "__zp_worktree_reverse_123_4"}, reason: AdmissionBookkeeping},
		{name: "case folded loader bookkeeping", identity: model.Identity{Kind: model.LiveAlias, Name: "__ZP_PRIVATE_ALIAS"}, reason: AdmissionBookkeeping},
		{name: "ordinary underscore symbol", identity: model.Identity{Kind: model.LiveFunction, Name: "_user_helper"}, admitted: true},
		{name: "live secret", identity: secretIdentity, reason: AdmissionLiveSecret},
		{name: "pinned secret", identity: model.Identity{Kind: model.LiveEnv, Name: "PINNED_TOKEN"}, reason: AdmissionPinnedSecret},
		{name: "unsupported kind", identity: model.Identity{Kind: model.LiveKind("job"), Name: "SAFE"}, reason: AdmissionUnsupported},
		{name: "unsafe name", identity: model.Identity{Kind: model.LiveEnv, Name: "BAD-NAME"}, reason: AdmissionUnsafeName},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := registry.Admit(attachment, test.identity)
			if got.Identity != test.identity || got.Admitted != test.admitted || got.Reason != test.reason {
				t.Fatalf("Admit() = %#v, want admitted=%v reason=%q", got, test.admitted, test.reason)
			}
		})
	}
	for _, identity := range []model.Identity{
		{Kind: model.LiveEnv, Name: "NEW_EDITOR"},
		{Kind: model.LiveAlias, Name: "new-alias"},
	} {
		if !registry.Owns(identity) {
			t.Errorf("admitted identity is not owned: %#v", identity)
		}
	}
}

func TestAttachAndAdmissionWithRealZshProvider(t *testing.T) {
	const inheritedSecretCanary = "inherited-secret-canary"
	const pinnedCanary = "pinned-secret-canary"
	registry := NewRegistry(zsh.Provider{})
	if err := registry.Seed(model.Profile{Entries: []model.Entry{materializedSecretEntry("PINNED_TOKEN")}}); err != nil {
		t.Fatalf("Seed() error = %v", err)
	}
	attachment, err := registry.Attach(model.LiveSnapshot{States: []model.LiveIdentityState{
		liveScalar(model.LiveEnv, "INHERITED", "ordinary"),
		liveScalar(model.LiveEnv, "GITHUB_TOKEN", inheritedSecretCanary),
		liveScalar(model.LiveEnv, "PINNED_TOKEN", pinnedCanary),
	}})
	if err != nil {
		t.Fatalf("Attach() error = %v", err)
	}

	wantExclusions := []model.Exclusion{
		{Identity: model.Identity{Kind: model.LiveEnv, Name: "INHERITED"}, Reason: string(AdmissionPresentAtAttach)},
		{Identity: model.Identity{Kind: model.LiveEnv, Name: "GITHUB_TOKEN"}, Reason: string(AdmissionLiveSecret)},
		{Identity: model.Identity{Kind: model.LiveEnv, Name: "PINNED_TOKEN"}, Reason: string(AdmissionPinnedSecret)},
	}
	if got := attachment.Exclusions(); !reflect.DeepEqual(got, wantExclusions) {
		t.Fatalf("Exclusions() = %#v, want %#v", got, wantExclusions)
	}
	if rendered := fmt.Sprintf("%#v", attachment); containsAny(rendered, inheritedSecretCanary, pinnedCanary) {
		t.Fatalf("real-provider attachment retained secret value: %s", rendered)
	}

	if got := registry.Admit(attachment, model.Identity{Kind: model.LiveEnv, Name: "CREATED_AFTER_ATTACH"}); !got.Admitted {
		t.Fatalf("safe post-attach admission = %#v", got)
	}
	if got := registry.Admit(attachment, model.Identity{Kind: model.LiveEnv, Name: "CREATED_API_TOKEN"}); got.Admitted || got.Reason != AdmissionLiveSecret {
		t.Fatalf("secret post-attach admission = %#v", got)
	}
}

func TestRegistryRejectsInvalidSeedAndSnapshotWithoutPartialResults(t *testing.T) {
	registry := NewRegistry(fakeLiveSecretPolicy{})
	badSecret := materializedSecretEntry("TOKEN")
	badSecret.Secret.Key = "OTHER"
	if err := registry.Seed(model.Profile{Entries: []model.Entry{
		materializedAssignment("EDITOR", model.CatEnvironment),
		badSecret,
	}}); err == nil {
		t.Fatal("Seed() error = nil for invalid SecretRef")
	}
	if registry.Owns(model.Identity{Kind: model.LiveEnv, Name: "EDITOR"}) {
		t.Fatal("failed Seed() partially replaced ownership")
	}

	attachment, err := registry.Attach(model.LiveSnapshot{States: []model.LiveIdentityState{{
		Identity: model.Identity{Kind: model.LiveEnv, Name: "BAD-NAME"},
		Value:    model.ScalarLiveValue("canary"),
	}}})
	if err == nil {
		t.Fatal("Attach() error = nil for malformed snapshot")
	}
	if attachment.Baseline() != nil || attachment.Exclusions() != nil {
		t.Fatalf("failed Attach() returned partial data: %#v", attachment)
	}
}

func TestAdmissionFailsClosedWithoutAttachmentOrSecretPolicy(t *testing.T) {
	var typedNilPolicy *pointerLiveSecretPolicy
	registry := NewRegistry(typedNilPolicy)
	identity := model.Identity{Kind: model.LiveEnv, Name: "CREATED_AFTER_ATTACH"}
	if got := registry.Admit(AttachmentResult{}, identity); got.Admitted || got.Reason != AdmissionNotAttached {
		t.Fatalf("Admit() without attachment = %#v", got)
	}

	attachment, err := registry.Attach(model.LiveSnapshot{States: []model.LiveIdentityState{
		liveScalar(model.LiveEnv, "INHERITED", "policy-missing-canary"),
	}})
	if err != nil {
		t.Fatalf("Attach() error = %v", err)
	}
	want := []model.Exclusion{{
		Identity: model.Identity{Kind: model.LiveEnv, Name: "INHERITED"},
		Reason:   string(AdmissionPolicyMissing),
	}}
	if got := attachment.Exclusions(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Exclusions() = %#v, want %#v", got, want)
	}
	if got := registry.Admit(attachment, identity); got.Admitted || got.Reason != AdmissionPolicyMissing {
		t.Fatalf("Admit() without policy = %#v", got)
	}
}

func TestRegistryRestoreAdmittedRevalidatesPolicy(t *testing.T) {
	secretIdentity := model.Identity{Kind: model.LiveEnv, Name: "RESTORED_API_TOKEN"}
	policy := fakeLiveSecretPolicy{secretIdentity: true}
	profile := model.Profile{Entries: []model.Entry{
		materializedAssignment("EDITOR", model.CatEnvironment),
		materializedSecretEntry("PINNED_TOKEN"),
	}}
	restored := []model.Identity{
		{Kind: model.LiveAlias, Name: "late-alias"},
		{Kind: model.LiveEnv, Name: "LATE_EDITOR"},
		{Kind: model.LiveEnv, Name: "EDITOR"},
	}

	first := NewRegistry(policy)
	if err := first.Seed(profile); err != nil {
		t.Fatal(err)
	}
	if err := restoreAdmittedForTest(t, first, restored); err != nil {
		t.Fatalf("RestoreAdmitted() error = %v", err)
	}
	if err := restoreAdmittedForTest(t, first, restored); err != nil {
		t.Fatalf("idempotent RestoreAdmitted() error = %v", err)
	}
	second := NewRegistry(policy)
	if err := second.Seed(profile); err != nil {
		t.Fatal(err)
	}
	if err := restoreAdmittedForTest(t, second, restored); err != nil {
		t.Fatal(err)
	}
	for _, identity := range restored {
		if !first.Owns(identity) || first.Owns(identity) != second.Owns(identity) {
			t.Errorf("restored ownership differs for %#v", identity)
		}
	}

	attachment, err := second.Attach(model.LiveSnapshot{States: []model.LiveIdentityState{
		liveScalar(model.LiveEnv, "INHERITED", "ambient-value-canary"),
	}})
	if err != nil {
		t.Fatal(err)
	}
	if second.Owns(model.Identity{Kind: model.LiveEnv, Name: "INHERITED"}) ||
		second.Admit(attachment, model.Identity{Kind: model.LiveEnv, Name: "INHERITED"}).Reason != AdmissionPresentAtAttach {
		t.Fatal("ambient present-at-attach identity became restored ownership")
	}

	unsafe := []struct {
		name     string
		identity model.Identity
	}{
		{name: "malformed", identity: model.Identity{Kind: model.LiveEnv, Name: "BAD-NAME"}},
		{name: "bookkeeping", identity: model.Identity{Kind: model.LiveFunction, Name: "_zp_private_helper"}},
		{name: "volatile", identity: model.Identity{Kind: model.LiveEnv, Name: "PWD"}},
		{name: "live secret", identity: secretIdentity},
		{name: "pinned secret", identity: model.Identity{Kind: model.LiveEnv, Name: "PINNED_TOKEN"}},
	}
	for _, test := range unsafe {
		t.Run(test.name, func(t *testing.T) {
			registry := NewRegistry(policy)
			if err := registry.Seed(profile); err != nil {
				t.Fatal(err)
			}
			partial := model.Identity{Kind: model.LiveAlias, Name: "must-not-partially-restore"}
			if err := restoreAdmittedForTest(t, registry, []model.Identity{partial, test.identity}); err == nil {
				t.Fatal("unsafe durable admission accepted")
			}
			if registry.Owns(partial) {
				t.Fatal("failed durable restoration partially mutated ownership")
			}
		})
	}

	if err := restoreAdmittedForTest(t, first, []model.Identity{{Kind: model.LiveEnv, Name: "DUP"}, {Kind: model.LiveEnv, Name: "DUP"}}); err == nil {
		t.Fatal("duplicate durable admission accepted")
	}
	missingPolicy := NewRegistry(nil)
	if err := missingPolicy.Seed(model.Profile{}); err != nil {
		t.Fatal(err)
	}
	if err := restoreAdmittedForTest(t, missingPolicy, []model.Identity{{Kind: model.LiveEnv, Name: "SAFE"}}); err == nil {
		t.Fatal("durable admission accepted without a live-secret policy")
	}
}

func restoreAdmittedForTest(t *testing.T, registry *Registry, identities []model.Identity) error {
	t.Helper()
	restorer, ok := any(registry).(interface {
		RestoreAdmitted([]model.Identity) error
	})
	if !ok {
		t.Fatal("Registry.RestoreAdmitted is missing")
	}
	return restorer.RestoreAdmitted(identities)
}

func materializedAssignment(name string, category model.Category) model.Entry {
	value := "value"
	return model.Entry{
		Kind:                    model.KindAssignment,
		Category:                category,
		Names:                   []string{name},
		Managed:                 true,
		Override:                model.OverrideAuto,
		StructuralFidelityKnown: true,
		ValueMode:               model.ValueModeLiteral,
		RuntimeValue:            &value,
	}
}

func unmanagedAssignment(name string) model.Entry {
	entry := materializedAssignment(name, model.CatEnvironment)
	entry.Managed = false
	return entry
}

func materializedAlias(name string) model.Entry {
	value := "print ok"
	return model.Entry{
		Kind:                    model.KindAlias,
		Category:                model.CatAliases,
		Names:                   []string{name},
		Managed:                 true,
		Override:                model.OverrideAuto,
		StructuralFidelityKnown: true,
		AliasAssignment:         true,
		ValueMode:               model.ValueModeLiteral,
		RuntimeValue:            &value,
	}
}

func materializedFunction(name string) model.Entry {
	body := "print ok"
	return model.Entry{
		Kind:                    model.KindFuncDecl,
		Category:                model.CatFunctions,
		Names:                   []string{name},
		Managed:                 true,
		Override:                model.OverrideAuto,
		StructuralFidelityKnown: true,
		FunctionBody:            &body,
	}
}

func materializedOptions(command string, names ...string) model.Entry {
	return model.Entry{
		Kind:                    model.KindCommand,
		Category:                model.CatOptions,
		CmdName:                 command,
		Names:                   append([]string(nil), names...),
		Managed:                 true,
		Override:                model.OverrideAuto,
		StructuralFidelityKnown: true,
	}
}

func materializedSecretEntry(name string) model.Entry {
	entry := materializedAssignment(name, model.CatSecrets)
	entry.ValueMode = model.ValueModeUnsupported
	entry.RuntimeValue = nil
	entry.Secret = &model.SecretRef{Kind: model.SecretRefFile, Key: name}
	return entry
}

func liveScalar(kind model.LiveKind, name, value string) model.LiveIdentityState {
	return model.LiveIdentityState{Identity: model.Identity{Kind: kind, Name: name}, Value: model.ScalarLiveValue(value)}
}

func containsAny(value string, needles ...string) bool {
	for _, needle := range needles {
		if needle != "" && stringsContains(value, needle) {
			return true
		}
	}
	return false
}

func stringsContains(value, needle string) bool {
	for index := 0; index+len(needle) <= len(value); index++ {
		if value[index:index+len(needle)] == needle {
			return true
		}
	}
	return false
}
