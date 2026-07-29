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

type targetOnlyStore struct {
	profiles     map[string]model.Profile
	checkouts    []string
	reads        []string
	current      string
	currentCalls int
}

func (s *targetOnlyStore) Branches(context.Context) ([]string, error) { return nil, nil }

func (s *targetOnlyStore) Current() string {
	s.currentCalls++
	return s.current
}

func (s *targetOnlyStore) Checkout(_ context.Context, name string) error {
	if _, ok := s.profiles[name]; !ok {
		return errors.New("fixture profile unavailable")
	}
	s.checkouts = append(s.checkouts, name)
	return nil
}

func (s *targetOnlyStore) Read(_ context.Context, name string) (model.Profile, error) {
	p, ok := s.profiles[name]
	if !ok {
		return model.Profile{}, errors.New("fixture profile unavailable")
	}
	s.reads = append(s.reads, name)
	return p, nil
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

type statefulSecretResolver struct {
	kind      model.SecretRefKind
	values    map[string]string
	available bool
	calls     []string
}

func (r *statefulSecretResolver) Kind() model.SecretRefKind { return r.kind }

func (r *statefulSecretResolver) Retrieve(key string) (string, error) {
	r.calls = append(r.calls, key)
	if !r.available {
		return "", errors.New("fixture resolver unavailable")
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

func TestRuntimeEmitterApplyUsesOnlyTargetAndRetainsTargetReverse(t *testing.T) {
	realZsh, err := exec.LookPath("zsh")
	if err != nil {
		t.Skip("zsh not installed")
	}
	ctx := context.Background()
	s := &targetOnlyStore{
		profiles: map[string]model.Profile{
			"main": transitionProfile(t, transitionProfileA),
			"B":    transitionProfile(t, transitionProfileB),
		},
		current: "B",
	}

	source, err := NewRuntimeEmitter(s, zsh.Provider{}).Emit(ctx, "apply", "main")
	if err != nil {
		t.Fatalf("Emit(apply, main): %v", err)
	}
	if s.currentCalls != 0 {
		t.Fatalf("apply consulted Store.Current %d times, want target-only emission", s.currentCalls)
	}
	if got := strings.Join(s.checkouts, ","); got != "main" {
		t.Fatalf("validated profiles = %q, want main", got)
	}
	if got := strings.Join(s.reads, ","); got != "main" {
		t.Fatalf("read profiles = %q, want main", got)
	}
	assertPairedApplySource(t, source)

	dir := t.TempDir()
	sourcePath := writeTransitionSource(t, dir, "main-apply.zsh", source)
	cmd := exec.Command(realZsh, "-f", "-c", `
typeset -g ZP_BASE_PATH="$PATH"
source "$1"
[[ "$ZP_A_ONLY" == from-a ]] || exit 10
(( ${+functions[zp_deactivate]} )) || exit 11
zp_deactivate
[[ -z "${ZP_A_ONLY+x}" ]] || exit 12
`, "zsh-pro-target-only-test", sourcePath)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("target reverse did not clean target state: %v\n%s", err, out)
	}
}

func TestRuntimeEmitterSwitchDoesNotResolveAnActiveSecretAgain(t *testing.T) {
	ctx := context.Background()
	s := &targetOnlyStore{
		profiles: map[string]model.Profile{
			"secret": redactedSecretProfile(),
			"B":      transitionProfile(t, transitionProfileB),
		},
		current: "secret",
	}
	resolver := &statefulSecretResolver{
		kind:      model.SecretRefFile,
		values:    map[string]string{"runtime-fixture": runtimeSecretFixture},
		available: true,
	}
	r := newSecretRuntimeEmitter(s, resolver)
	secretSource, err := r.Emit(ctx, "apply", "secret")
	if err != nil {
		t.Fatalf("secret activation emit: %v", err)
	}
	assertPairedApplySource(t, secretSource)

	resolver.available = false
	bSource, err := r.Emit(ctx, "apply", "B")
	if err != nil {
		t.Fatalf("switch target emission re-resolved the active secret: %v", err)
	}
	assertPairedApplySource(t, bSource)
	if got := strings.Join(resolver.calls, ","); got != "runtime-fixture" {
		t.Fatalf("resolver calls = %q, want only the initial target secret", got)
	}
	if s.currentCalls != 0 {
		t.Fatalf("switch consulted Store.Current %d times, which can reintroduce active-secret resolution", s.currentCalls)
	}
	if got := strings.Join(s.reads, ","); got != "secret,B" {
		t.Fatalf("read profiles = %q, want secret then new target only", got)
	}
}

func TestRuntimeEmitterEmitsCompleteTransitionSource(t *testing.T) {
	ctx := context.Background()
	s := newTransitionStore(t, ctx)
	r := NewRuntimeEmitter(s, zsh.Provider{})

	for _, name := range []string{"main", "B"} {
		source, err := r.Emit(ctx, "apply", name)
		if err != nil {
			t.Fatalf("Emit(apply, %s): %v", name, err)
		}
		assertPairedApplySource(t, source)
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
	applyMain := mustEmitTransition(t, r, ctx, "apply", "main")
	t.Setenv("ZSHPRO_PROFILE", "main")
	applyB := mustEmitTransition(t, r, ctx, "apply", "B")

	dir := t.TempDir()
	runtimeRoot := filepath.Join(dir, "runtime")
	if err := os.Mkdir(runtimeRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	loader := filepath.Join(dir, "loader.zsh")
	if err := os.WriteFile(loader, []byte((zsh.Provider{}).HookScript()), 0o600); err != nil {
		t.Fatal(err)
	}
	applyMainPath := writeTransitionSource(t, dir, "apply-main.zsh", applyMain)
	applyBPath := writeTransitionSource(t, dir, "apply-b.zsh", applyB)
	shim := filepath.Join(dir, "zsh-pro")
	shimSource := `#!/bin/sh
case "$1:$2:$3" in
  emit:apply:main) printf '%s:%s\n' "$2" "$3" >> "$ZP_EMIT_LOG"; cat "$ZP_APPLY_MAIN" ;;
  emit:apply:B) printf '%s:%s\n' "$2" "$3" >> "$ZP_EMIT_LOG"; cat "$ZP_APPLY_B" ;;
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
unset ZSHPRO_PROFILE ZP_ACTIVE_PROFILE
source "$1"
before_path_count=$#path
activate main
[[ "$ZSHPRO_PROFILE" == main ]] || exit 60
[[ "$ZP_ACTIVE_PROFILE" == main ]] || exit 61
[[ "$ZP_A_ONLY" == from-a ]] || exit 61
alias zp_a_only >/dev/null || exit 62
(( ${+functions[zp_a_only_fn]} )) || exit 63
[[ -o extendedglob ]] || exit 64
(( $#path == before_path_count + 1 )) || exit 65

activate main
(( $#path == before_path_count + 1 )) || exit 66
` + switchCommand + `
[[ "$ZSHPRO_PROFILE" == B ]] || exit 66
[[ "$ZP_ACTIVE_PROFILE" == B ]] || exit 67
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
[[ -z "${ZP_ACTIVE_PROFILE+x}" ]] || exit 76
[[ -z "${ZP_A_ONLY+x}" && -z "${ZP_B_ONLY+x}" ]] || exit 76
alias zp_a_only >/dev/null 2>&1 && exit 77
alias zp_b_only >/dev/null 2>&1 && exit 78
(( ${+functions[zp_a_only_fn]} || ${+functions[zp_b_only_fn]} )) && exit 79
[[ -o extendedglob ]] && exit 80
(( $#path == before_path_count )) || exit 81
`
	emitLog := filepath.Join(dir, "emit.log")
	cmd := exec.Command(realZsh, "-f", "-c", body, "zsh-pro-transition-test", loader)
	cmd.Env = transitionEnv(
		"PATH="+dir+":"+os.Getenv("PATH"),
		"ZSHPRO_HOME="+runtimeRoot,
		"ZP_APPLY_MAIN="+applyMainPath,
		"ZP_APPLY_B="+applyBPath,
		"ZP_REAL_ZSH="+realZsh,
		"ZP_VALIDATOR_LOG="+validatorLog,
		"ZP_EMIT_LOG="+emitLog,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("real Store A-to-B transition left residue: %v\n%s", err, out)
	}
	validations, err := os.ReadFile(validatorLog)
	if err != nil {
		t.Fatalf("read validation log: %v", err)
	}
	if string(validations) != "..." {
		t.Fatalf("validation count = %q, want one complete source validation per public transition", validations)
	}
	emissions, err := os.ReadFile(emitLog)
	if err != nil {
		t.Fatalf("read emit log: %v", err)
	}
	if got := string(emissions); got != "apply:main\napply:B\n" {
		t.Fatalf("runtime emitter calls = %q, want one apply per distinct target and no binary deactivate", got)
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
	if _, err := s.Commit(ctx, "main", transitionProfile(t, transitionProfileA), "transition fixture main"); err != nil {
		t.Fatalf("Commit(main): %v", err)
	}
	if err := s.Create(ctx, "B"); err != nil {
		t.Fatalf("Create(B): %v", err)
	}
	if _, err := s.Commit(ctx, "B", transitionProfile(t, transitionProfileB), "transition fixture B"); err != nil {
		t.Fatalf("Commit(B): %v", err)
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

func assertPairedApplySource(t *testing.T, source string) {
	t.Helper()
	trimmed := strings.TrimSpace(source)
	if !strings.Contains(source, "zp_apply()") || !strings.HasSuffix(trimmed, "zp_apply") {
		t.Fatalf("apply source is not directly executable:\n%s", source)
	}
	if !strings.Contains(source, "zp_deactivate()") {
		t.Fatalf("apply source does not retain a target-specific reverse:\n%s", source)
	}
	if strings.Contains(source, "\nzp_deactivate\nzp_apply") {
		t.Fatalf("target-only apply unexpectedly invokes a prior reverse:\n%s", source)
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

func transitionEnv(overrides ...string) []string {
	replaced := make(map[string]bool, len(overrides))
	for _, override := range overrides {
		key, _, _ := strings.Cut(override, "=")
		replaced[key] = true
	}
	env := make([]string, 0, len(os.Environ())+len(overrides))
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if !replaced[key] {
			env = append(env, entry)
		}
	}
	return append(env, overrides...)
}
