package cli

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"zsh-pro/core/ir"
	"zsh-pro/core/model"
	"zsh-pro/core/shell/zsh"
	"zsh-pro/core/store"
)

const transitionProfileA = `
export ZP_A_ONLY=from-a
alias zp_a_only='print -r -- A'
zp_a_only_fn() { print -r -- A }
setopt extendedglob
export PATH=/zp-a/bin:$PATH
`

const transitionProfileB = `
export ZP_B_ONLY=from-b
alias zp_b_only='print -r -- B'
zp_b_only_fn() { print -r -- B }
export PATH=/zp-b/bin:$PATH
`

const runtimeSecretFixture = "phase5-runtime-fixture"

type secretProfileStore struct {
	profile model.Profile
	current string
}

func (s *secretProfileStore) Branches(context.Context) ([]string, error) { return nil, nil }
func (s *secretProfileStore) Current() string                            { return s.current }
func (s *secretProfileStore) Checkout(context.Context, string) error     { return nil }
func (s *secretProfileStore) Read(context.Context, string) (model.Profile, error) {
	return s.profile, nil
}

type deterministicSecretResolver struct {
	kind   model.SecretRefKind
	values map[string]string
	err    error
}

func (r deterministicSecretResolver) Kind() model.SecretRefKind { return r.kind }

func (r deterministicSecretResolver) Retrieve(key string) (string, error) {
	if r.err != nil {
		return "", r.err
	}
	value, ok := r.values[key]
	if !ok {
		return "", errors.New("fixture resolver key unavailable")
	}
	return value, nil
}

func newSecretRuntimeEmitter(s Store, resolver SecretResolver) Emitter {
	return NewRuntimeEmitter(s, zsh.Provider{}, resolver)
}

func redactedSecretProfile() model.Profile {
	return model.Profile{Entries: []model.Entry{{
		Text:                    "ZP_RUNTIME_SECRET='<zsh-pro secret file:runtime-fixture>'",
		Value:                   "'<zsh-pro secret file:runtime-fixture>'",
		Category:                model.CatSecrets,
		Kind:                    model.KindAssignment,
		Names:                   []string{"ZP_RUNTIME_SECRET"},
		Exported:                true,
		Managed:                 true,
		StructuralFidelityKnown: true,
		Secret:                  &model.SecretRef{Kind: model.SecretRefFile, Key: "runtime-fixture"},
		ValueMode:               model.ValueModeUnsupported,
	}}}
}

func TestRuntimeEmitterResolvesSecretRefBeforeBuild(t *testing.T) {
	realZsh, err := exec.LookPath("zsh")
	if err != nil {
		t.Skip("zsh not installed")
	}
	ctx := context.Background()
	profile := redactedSecretProfile()
	backingStore := &secretProfileStore{profile: profile}
	resolver := deterministicSecretResolver{
		kind:   model.SecretRefFile,
		values: map[string]string{"runtime-fixture": runtimeSecretFixture},
	}

	source, err := newSecretRuntimeEmitter(backingStore, resolver).Emit(ctx, "apply", "main")
	if err != nil {
		t.Fatal("secret-backed profile did not emit")
	}
	if !strings.Contains(source, "ZP_RUNTIME_SECRET") {
		t.Fatal("resolved secret assignment was omitted from emitted source")
	}

	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "secret-apply.zsh")
	if err := os.WriteFile(sourcePath, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(realZsh, "-f", "-c", `source "$1"; [[ "${ZP_RUNTIME_SECRET-}" == "$2" ]]`, "zsh-pro-secret-test", sourcePath, runtimeSecretFixture)
	if err := cmd.Run(); err != nil {
		t.Fatal("resolved secret assignment did not reach the live shell")
	}

	persisted := backingStore.profile.Entries[0]
	if persisted.RuntimeValue != nil || persisted.ValueMode != model.ValueModeUnsupported || persisted.Value == "" {
		t.Fatal("runtime resolution mutated the redacted stored profile")
	}
}

func TestRuntimeEmitterSecretResolverFailuresEmitNothing(t *testing.T) {
	ctx := context.Background()
	resolverFailure := errors.New("fixture resolver failed")
	for _, tc := range []struct {
		name     string
		resolver SecretResolver
	}{
		{name: "missing resolver"},
		{name: "kind mismatch", resolver: deterministicSecretResolver{kind: model.SecretRefKeychain, values: map[string]string{"runtime-fixture": runtimeSecretFixture}}},
		{name: "missing key", resolver: deterministicSecretResolver{kind: model.SecretRefFile, values: map[string]string{}}},
		{name: "resolver error", resolver: deterministicSecretResolver{kind: model.SecretRefFile, err: resolverFailure}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source, err := newSecretRuntimeEmitter(&secretProfileStore{profile: redactedSecretProfile()}, tc.resolver).Emit(ctx, "apply", "main")
			if err == nil {
				t.Fatal("resolver failure unexpectedly emitted source")
			}
			if source != "" {
				t.Fatal("resolver failure returned partial source")
			}
			if strings.Contains(err.Error(), runtimeSecretFixture) || strings.Contains(err.Error(), resolverFailure.Error()) {
				t.Fatal("resolver failure disclosed fixture data")
			}
		})
	}
}

func TestRuntimeEmitterEmitsCompleteTransitionSource(t *testing.T) {
	ctx := context.Background()
	s := newTransitionStore(t, ctx)
	r := NewRuntimeEmitter(s, zsh.Provider{})

	t.Setenv("ZSHPRO_PROFILE", "")
	first, err := r.Emit(ctx, "apply", "A")
	if err != nil {
		t.Fatalf("first activation emit: %v", err)
	}
	assertCompleteApplySource(t, first, false)

	t.Setenv("ZSHPRO_PROFILE", "A")
	transition, err := r.Emit(ctx, "apply", "B")
	if err != nil {
		t.Fatalf("A-to-B emit: %v", err)
	}
	assertCompleteApplySource(t, transition, true)
	if !strings.Contains(transition, "unalias zp_a_only") || !strings.Contains(transition, "export ZP_B_ONLY") {
		t.Fatalf("A-to-B source does not own both halves:\n%s", transition)
	}

	same, err := r.Emit(ctx, "apply", "A")
	if err != nil {
		t.Fatalf("same-profile emit: %v", err)
	}
	assertCompleteApplySource(t, same, false)

	t.Setenv("ZSHPRO_PROFILE", "B")
	deactivate, err := r.Emit(ctx, "deactivate", "B")
	if err != nil {
		t.Fatalf("deactivate emit: %v", err)
	}
	if !strings.HasSuffix(strings.TrimSpace(deactivate), "zp_deactivate") {
		t.Fatalf("deactivate source is not directly executable:\n%s", deactivate)
	}
}

func TestRuntimeEmitterLiveTransitionRemovesAOnlyState(t *testing.T) {
	testRuntimeEmitterLiveTransition(t, "activate B")
}

func TestRuntimeEmitterLiveCheckoutTransitionRemovesAOnlyState(t *testing.T) {
	testRuntimeEmitterLiveTransition(t, "checkout B")
}

func testRuntimeEmitterLiveTransition(t *testing.T, switchCommand string) {
	realZsh, err := exec.LookPath("zsh")
	if err != nil {
		t.Skip("zsh not installed")
	}
	ctx := context.Background()
	s := newTransitionStore(t, ctx)
	r := NewRuntimeEmitter(s, zsh.Provider{})

	t.Setenv("ZSHPRO_PROFILE", "")
	applyA := mustEmitTransition(t, r, ctx, "apply", "A")
	t.Setenv("ZSHPRO_PROFILE", "A")
	applyB := mustEmitTransition(t, r, ctx, "apply", "B")
	t.Setenv("ZSHPRO_PROFILE", "B")
	deactivateB := mustEmitTransition(t, r, ctx, "deactivate", "B")

	dir := t.TempDir()
	loader := filepath.Join(dir, "loader.zsh")
	if err := os.WriteFile(loader, []byte((zsh.Provider{}).HookScript()), 0o600); err != nil {
		t.Fatal(err)
	}
	applyAPath := writeTransitionSource(t, dir, "apply-a.zsh", applyA)
	applyBPath := writeTransitionSource(t, dir, "apply-b.zsh", applyB)
	deactivateBPath := writeTransitionSource(t, dir, "deactivate-b.zsh", deactivateB)
	shim := filepath.Join(dir, "zsh-pro")
	shimSource := `#!/bin/sh
case "$1:$2:$3" in
  emit:apply:A) cat "$ZP_APPLY_A" ;;
  emit:apply:B) cat "$ZP_APPLY_B" ;;
  emit:deactivate:B) cat "$ZP_DEACTIVATE_B" ;;
  *) exit 64 ;;
esac
`
	if err := os.WriteFile(shim, []byte(shimSource), 0o700); err != nil {
		t.Fatal(err)
	}
	validatorLog := filepath.Join(dir, "validator.log")
	validator := filepath.Join(dir, "zsh")
	validatorSource := "#!/bin/sh\nprintf . >> \"$ZP_VALIDATOR_LOG\"\nexec \"$ZP_REAL_ZSH\" \"$@\"\n"
	if err := os.WriteFile(validator, []byte(validatorSource), 0o700); err != nil {
		t.Fatal(err)
	}

	body := `
unset ZSHPRO_PROFILE
source "$1"
before_path_count=$#path
activate A
[[ "$ZSHPRO_PROFILE" == A ]] || exit 60
[[ "$ZP_A_ONLY" == from-a ]] || exit 61
alias zp_a_only >/dev/null || exit 62
(( ${+functions[zp_a_only_fn]} )) || exit 63
[[ -o extendedglob ]] || exit 64
(( $#path == before_path_count + 1 )) || exit 65

activate A
(( $#path == before_path_count + 1 )) || exit 66
` + switchCommand + `
[[ "$ZSHPRO_PROFILE" == B ]] || exit 66
[[ -z "${ZP_A_ONLY+x}" ]] || exit 67
alias zp_a_only >/dev/null 2>&1 && exit 68
(( ${+functions[zp_a_only_fn]} )) && exit 69
[[ -o extendedglob ]] && exit 70
[[ "$ZP_B_ONLY" == from-b ]] || exit 71
alias zp_b_only >/dev/null || exit 72
(( ${+functions[zp_b_only_fn]} )) || exit 73
(( $#path == before_path_count + 1 )) || exit 74

deactivate
[[ -z "${ZSHPRO_PROFILE+x}" ]] || exit 75
[[ -z "${ZP_A_ONLY+x}" && -z "${ZP_B_ONLY+x}" ]] || exit 76
alias zp_a_only >/dev/null 2>&1 && exit 77
alias zp_b_only >/dev/null 2>&1 && exit 78
(( ${+functions[zp_a_only_fn]} || ${+functions[zp_b_only_fn]} )) && exit 79
[[ -o extendedglob ]] && exit 80
(( $#path == before_path_count )) || exit 81
`
	cmd := exec.Command(realZsh, "-f", "-c", body, "zsh-pro-transition-test", loader)
	cmd.Env = append(os.Environ(),
		"PATH="+dir+":"+os.Getenv("PATH"),
		"TMPDIR="+dir,
		"ZP_APPLY_A="+applyAPath,
		"ZP_APPLY_B="+applyBPath,
		"ZP_DEACTIVATE_B="+deactivateBPath,
		"ZP_REAL_ZSH="+realZsh,
		"ZP_VALIDATOR_LOG="+validatorLog,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("real Store A-to-B transition left residue: %v\n%s", err, out)
	}
	validations, err := os.ReadFile(validatorLog)
	if err != nil {
		t.Fatalf("read validation log: %v", err)
	}
	if string(validations) != "...." {
		t.Fatalf("validation count = %q, want one complete source validation per public transition", validations)
	}
}

func newTransitionStore(t *testing.T, ctx context.Context) *store.Store {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	s, err := store.New(t.TempDir(), zsh.Provider{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Init(ctx); err != nil {
		t.Fatal(err)
	}
	for _, profile := range []struct {
		name   string
		source string
	}{
		{name: "A", source: transitionProfileA},
		{name: "B", source: transitionProfileB},
	} {
		if err := s.Create(ctx, profile.name); err != nil {
			t.Fatalf("Create(%s): %v", profile.name, err)
		}
		if _, err := s.Commit(ctx, profile.name, transitionProfile(t, profile.source), "transition fixture "+profile.name); err != nil {
			t.Fatalf("Commit(%s): %v", profile.name, err)
		}
	}
	return s
}

func transitionProfile(t *testing.T, source string) model.Profile {
	t.Helper()
	provider := zsh.Provider{}
	blocks, err := provider.Parse([]byte(source))
	if err != nil {
		t.Fatal(err)
	}
	return ir.Build(blocks, provider)
}

func assertCompleteApplySource(t *testing.T, source string, wantDeactivate bool) {
	t.Helper()
	trimmed := strings.TrimSpace(source)
	if !strings.Contains(source, "zp_apply()") || !strings.HasSuffix(trimmed, "zp_apply") {
		t.Fatalf("apply source is not directly executable:\n%s", source)
	}
	if wantDeactivate {
		if !strings.HasSuffix(trimmed, "zp_deactivate\nzp_apply") {
			t.Fatalf("transition source does not run deactivate before apply:\n%s", source)
		}
		return
	}
	if strings.Contains(source, "zp_deactivate") {
		t.Fatalf("first/same-profile apply unexpectedly deactivates:\n%s", source)
	}
}

func mustEmitTransition(t *testing.T, r Emitter, ctx context.Context, mode, name string) string {
	t.Helper()
	source, err := r.Emit(ctx, mode, name)
	if err != nil {
		t.Fatalf("Emit(%q, %q): %v", mode, name, err)
	}
	return source
}

func writeTransitionSource(t *testing.T, dir, name, source string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
