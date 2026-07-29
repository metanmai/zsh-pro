package cli

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
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
	loaderPath := filepath.Join(dir, "loader.zsh")
	if err := os.WriteFile(loaderPath, []byte((zsh.Provider{}).HookScript()), 0o600); err != nil {
		t.Fatal(err)
	}
	sourcePath := filepath.Join(dir, "secret-apply.zsh")
	if err := os.WriteFile(sourcePath, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(realZsh, "-f", "-c", `source "$1"; source "$2"; [[ "${ZP_RUNTIME_SECRET-}" == "$3" ]]`, "zsh-pro-secret-test", loaderPath, sourcePath, runtimeSecretFixture)
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
	loaderPath := filepath.Join(dir, "loader.zsh")
	if err := os.WriteFile(loaderPath, []byte((zsh.Provider{}).HookScript()), 0o600); err != nil {
		t.Fatal(err)
	}
	sourcePath := writeTransitionSource(t, dir, "main-apply.zsh", source)
	cmd := exec.Command(realZsh, "-f", "-c", `
typeset -g ZP_BASE_PATH="$PATH"
source "$1"
source "$2"
[[ "$ZP_A_ONLY" == from-a ]] || exit 10
[[ -n "${ZP_ACTIVE_REVERSE_FN-}" && ${+functions[$ZP_ACTIVE_REVERSE_FN]} == 1 ]] || exit 11
typeset -g +x ZP_ACTIVE_PROFILE=main
export ZSHPRO_PROFILE=main
deactivate
[[ -z "${ZP_A_ONLY+x}" ]] || exit 12
`, "zsh-pro-target-only-test", loaderPath, sourcePath)
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

func TestRuntimeEmitterPreservesUserFunctionsAndScrubsResolvedSecrets(t *testing.T) {
	realZsh, err := exec.LookPath("zsh")
	if err != nil {
		t.Skip("zsh not installed")
	}
	ctx := context.Background()
	profile := redactedSecretProfile()
	source, err := newSecretRuntimeEmitter(
		&secretProfileStore{profile: profile},
		deterministicSecretResolver{kind: model.SecretRefFile, values: map[string]string{"runtime-fixture": runtimeSecretFixture}},
	).Emit(ctx, "apply", "secret")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(source, "zp_apply()") || strings.Contains(source, "zp_deactivate()") || strings.Contains(source, "zp_capture_scalar()") || strings.Contains(source, "zp_restore_scalar()") {
		t.Fatalf("runtime payload retained a generic helper name:\n%s", source)
	}
	dir := t.TempDir()
	loaderPath := filepath.Join(dir, "loader.zsh")
	if err := os.WriteFile(loaderPath, []byte((zsh.Provider{}).HookScript()), 0o600); err != nil {
		t.Fatal(err)
	}
	sourcePath := writeTransitionSource(t, dir, "secret-apply.zsh", source)
	body := `
source "$1"
zp_apply() { print -r -- user-apply; }
zp_deactivate() { print -r -- user-deactivate; }
zp_capture_scalar() { print -r -- user-capture; }
zp_restore_scalar() { print -r -- user-restore; }
before_apply="${functions[zp_apply]}"
before_deactivate="${functions[zp_deactivate]}"
before_capture="${functions[zp_capture_scalar]}"
before_restore="${functions[zp_restore_scalar]}"
source "$2"
[[ "${ZP_RUNTIME_SECRET-}" == "$3" ]] || exit 10
[[ "${functions[zp_apply]}" == "$before_apply" ]] || exit 11
[[ "${functions[zp_deactivate]}" == "$before_deactivate" ]] || exit 12
[[ "${functions[zp_capture_scalar]}" == "$before_capture" ]] || exit 13
[[ "${functions[zp_restore_scalar]}" == "$before_restore" ]] || exit 14
[[ -n "${ZP_ACTIVE_REVERSE_FN-}" && ${+functions[$ZP_ACTIVE_REVERSE_FN]} == 1 ]] || exit 15
for function_name in ${(k)functions}; do
  case "$function_name" in
    __zp_apply_*) exit 16 ;;
    __zp_deactivate_*) [[ "$function_name" == "$ZP_ACTIVE_REVERSE_FN" ]] || exit 17 ;;
  esac
done
typeset -g +x ZP_ACTIVE_PROFILE=secret
export ZSHPRO_PROFILE=secret
deactivate
[[ -z "${ZP_RUNTIME_SECRET+x}" && -z "${ZP_ACTIVE_REVERSE_FN+x}" ]] || exit 20
[[ -z "${ZP_ACTIVE_PROFILE+x}" && -z "${ZSHPRO_PROFILE+x}" ]] || exit 21
[[ "${functions[zp_apply]}" == "$before_apply" ]] || exit 22
[[ "${functions[zp_deactivate]}" == "$before_deactivate" ]] || exit 23
[[ "${functions[zp_capture_scalar]}" == "$before_capture" ]] || exit 24
[[ "${functions[zp_restore_scalar]}" == "$before_restore" ]] || exit 25
for function_name in ${(k)functions}; do
  case "$function_name" in
    __zp_apply_*|__zp_deactivate_*) exit 26 ;;
  esac
  [[ "${functions[$function_name]}" != *"$3"* ]] || exit 27
done
`
	cmd := exec.Command(realZsh, "-f", "-c", body, "zsh-pro-runtime-secret-test", loaderPath, sourcePath, runtimeSecretFixture)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("runtime payload did not preserve user functions or scrub secret state: %v\n%s", err, out)
	}
}

func TestRuntimeEmitterFailedSwitchScrubsResolvedSecretPayload(t *testing.T) {
	realZsh, err := exec.LookPath("zsh")
	if err != nil {
		t.Skip("zsh not installed")
	}
	source, err := newSecretRuntimeEmitter(
		&secretProfileStore{profile: redactedSecretProfile()},
		deterministicSecretResolver{kind: model.SecretRefFile, values: map[string]string{"runtime-fixture": runtimeSecretFixture}},
	).Emit(context.Background(), "apply", "secret")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	runtimeRoot := filepath.Join(dir, "runtime")
	if err := os.Mkdir(runtimeRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	loaderPath := filepath.Join(dir, "loader.zsh")
	if err := os.WriteFile(loaderPath, []byte((zsh.Provider{}).HookScript()), 0o600); err != nil {
		t.Fatal(err)
	}
	secretPath := writeTransitionSource(t, dir, "secret-apply.zsh", source)
	bPath := writeTransitionSource(t, dir, "bad-apply.zsh", `
__zp_deactivate_bad() { unset ZP_FAILED_TARGET; }
__zp_apply_bad() { export ZP_FAILED_TARGET=1; return 9; }
_zp_run_payload __zp_apply_bad __zp_deactivate_bad
`)
	shim := filepath.Join(dir, "zsh-pro")
	if err := os.WriteFile(shim, []byte("#!/bin/sh\ncase \"$1:$2:$3\" in\nemit:apply:B) cat \"$ZP_APPLY_B\" ;;\n*) exit 64 ;;\nesac\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	body := `
source "$1"
source "$2"
typeset -g +x ZP_ACTIVE_PROFILE=secret
export ZSHPRO_PROFILE=secret
activate B
[[ "$ZP_LAST_RUNTIME_STATUS" -ne 0 ]] || exit 10
[[ -z "${ZP_RUNTIME_SECRET+x}" && -z "${ZP_FAILED_TARGET+x}" ]] || exit 11
[[ -z "${ZP_ACTIVE_PROFILE+x}" && -z "${ZSHPRO_PROFILE+x}" && -z "${ZP_ACTIVE_REVERSE_FN+x}" ]] || exit 12
for function_name in ${(k)functions}; do
  case "$function_name" in
    __zp_apply_*|__zp_deactivate_*) exit 13 ;;
  esac
  [[ "${functions[$function_name]}" != *"$3"* ]] || exit 14
done
`
	cmd := exec.Command(realZsh, "-f", "-c", body, "zsh-pro-secret-switch-test", loaderPath, secretPath, runtimeSecretFixture)
	cmd.Env = transitionEnv(
		"PATH="+dir+":"+os.Getenv("PATH"),
		"ZSHPRO_HOME="+runtimeRoot,
		"ZP_APPLY_B="+bPath,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed switch retained resolved secret source: %v\n%s", err, out)
	}
}

func TestRuntimePayloadCollisionLeavesPreexistingFunctionUntouched(t *testing.T) {
	realZsh, err := exec.LookPath("zsh")
	if err != nil {
		t.Skip("zsh not installed")
	}
	names := runtimeFunctionNames{apply: "__zp_apply_collision", deactivate: "__zp_deactivate_collision"}
	payload := runtimeApplyPayload(
		"__zp_deactivate_collision() { export ZP_COLLISION_TARGET=reverse; }\n",
		"__zp_apply_collision() { export ZP_COLLISION_TARGET=apply; }\n",
		names,
	)
	dir := t.TempDir()
	loaderPath := filepath.Join(dir, "loader.zsh")
	if err := os.WriteFile(loaderPath, []byte((zsh.Provider{}).HookScript()), 0o600); err != nil {
		t.Fatal(err)
	}
	payloadPath := writeTransitionSource(t, dir, "collision.zsh", payload)
	body := `
source "$1"
__zp_apply_collision() { print -r -- user-function; }
before="${functions[__zp_apply_collision]}"
if source "$2"; then exit 10; fi
[[ "${functions[__zp_apply_collision]}" == "$before" ]] || exit 11
[[ -z "${ZP_COLLISION_TARGET+x}" ]] || exit 12
[[ ${+functions[__zp_deactivate_collision]} == 0 ]] || exit 13
`
	cmd := exec.Command(realZsh, "-f", "-c", body, "zsh-pro-runtime-collision-test", loaderPath, payloadPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("collision-safe runtime payload overwrote user state: %v\n%s", err, out)
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
	if string(validations) != ".." {
		t.Fatalf("validation count = %q, want one complete source validation per emitted target payload", validations)
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
	apply := regexp.MustCompile(`__zp_apply_[0-9a-f]{32}\(\) \{`)
	reverse := regexp.MustCompile(`__zp_deactivate_[0-9a-f]{32}\(\) \{`)
	if !apply.MatchString(source) || !reverse.MatchString(source) || !strings.Contains(source, "_zp_run_payload __zp_apply_") {
		t.Fatalf("apply source is not a secure paired runtime payload:\n%s", source)
	}
	for _, generic := range []string{"zp_apply()", "zp_deactivate()", "zp_capture_scalar()", "zp_restore_scalar()"} {
		if strings.Contains(source, generic) {
			t.Fatalf("runtime payload retained generic helper %q:\n%s", generic, source)
		}
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
