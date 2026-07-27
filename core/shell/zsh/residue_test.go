package zsh

import (
	"bytes"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"zsh-pro/core/activate"
	"zsh-pro/core/ir"
	"zsh-pro/core/model"
)

const residueSeed int64 = 0x5eed0405

const residueActionCount = 24

type residueAction struct {
	profile int
	apply   bool
}

func generateBalancedActions(rng *rand.Rand, count, profileCount int) []residueAction {
	if profileCount < 1 {
		return nil
	}
	if count < 2*profileCount {
		count = 2 * profileCount
	}
	if count%2 != 0 {
		count++
	}
	actions := make([]residueAction, 0, count)
	for pair := 0; pair < count/2; pair++ {
		profile := rng.Intn(profileCount)
		if pair < profileCount {
			profile = pair
		}
		actions = append(actions,
			residueAction{profile: profile, apply: true},
			residueAction{profile: profile, apply: false},
		)
	}
	return actions
}

func actionsAreBalanced(actions []residueAction) bool {
	active := -1
	for _, action := range actions {
		if action.profile < 0 {
			return false
		}
		if action.apply {
			if active != -1 {
				return false
			}
			active = action.profile
			continue
		}
		if active != action.profile {
			return false
		}
		active = -1
	}
	return active == -1
}

func formatResidueTrace(actions []residueAction) string {
	parts := make([]string, 0, len(actions))
	for _, action := range actions {
		verb := "deactivate"
		if action.apply {
			verb = "apply"
		}
		parts = append(parts, fmt.Sprintf("%s(%d)", verb, action.profile))
	}
	return strings.Join(parts, " -> ")
}

func TestBalancedResidueActionGenerator(t *testing.T) {
	actions := generateBalancedActions(rand.New(rand.NewSource(residueSeed)), residueActionCount, 2)
	if len(actions) < residueActionCount {
		t.Fatalf("seed=%d: got %d actions, want at least %d", residueSeed, len(actions), residueActionCount)
	}
	if !actionsAreBalanced(actions) {
		t.Fatalf("seed=%d: generated an unbalanced trace: %s", residueSeed, formatResidueTrace(actions))
	}
	seen := map[int]bool{}
	for _, action := range actions {
		if action.apply {
			seen[action.profile] = true
		}
	}
	if len(seen) < 2 {
		t.Fatalf("seed=%d: trace did not exercise two profiles: %s", residueSeed, formatResidueTrace(actions))
	}
}

// residueSnapshotFunction writes a collision-safe byte oracle without command
// substitution. Every arbitrary string field is independently ${(qqqq)}
// encoded, while tags, indexes, and counts stay canonical plain lines.
const residueSnapshotFunction = `
ZP__snapshot() {
  local ZP__dest="$1"
  local ZP__name ZP__type ZP__scalar ZP__key ZP__value ZP__path_value ZP__list
  local -a ZP__values ZP__items
  local -i ZP__i ZP__j ZP__count ZP__seen
  {
    print -r -- PARAMETERS
    for ZP__name in "${(@ok)parameters}"; do
      [[ "$ZP__name" == ZP__* || "$ZP__name" == ZP_BASE_* ]] && continue
      ZP__type="${parameters[$ZP__name]}"
      if [[ "$ZP__type" == *special* || "$ZP__type" == *tied* || "$ZP__type" == *hide* || "$ZP__type" == undefined ]]; then
        continue
      fi
      case "$ZP__type" in
        scalar*|integer*|float*)
          ZP__scalar="${(P)ZP__name}"
          print -r -- P
          print -r -- "${(qqqq)ZP__name}"
          print -r -- "${(qqqq)ZP__type}"
          print -r -- "${(qqqq)ZP__scalar}"
          ;;
        array*)
          ZP__values=("${(@P)ZP__name}")
          print -r -- A
          print -r -- "${(qqqq)ZP__name}"
          print -r -- "${(qqqq)ZP__type}"
          print -r -- "$#ZP__values"
          for (( ZP__i = 1; ZP__i <= $#ZP__values; ZP__i++ )); do
            print -r -- "$ZP__i"
            print -r -- "${(qqqq)ZP__values[$ZP__i]}"
          done
          ;;
        association*)
          ZP__items=("${(@okvP)ZP__name}")
          print -r -- H
          print -r -- "${(qqqq)ZP__name}"
          print -r -- "${(qqqq)ZP__type}"
          print -r -- "$(( $#ZP__items / 2 ))"
          for (( ZP__i = 1; ZP__i <= $#ZP__items; ZP__i += 2 )); do
            ZP__key="${ZP__items[$ZP__i]}"
            ZP__value="${ZP__items[$(( ZP__i + 1 ))]}"
            print -r -- "${(qqqq)ZP__key}"
            print -r -- "${(qqqq)ZP__value}"
          done
          ;;
        *)
          print -u2 -r -- "unhandled parameter type: $ZP__name=$ZP__type"
          return 91
          ;;
      esac
    done

    print -r -- ALIASES
    for ZP__name in "${(@ok)aliases}"; do
      [[ "$ZP__name" == ZP__* ]] && continue
      print -r -- L
      print -r -- "${(qqqq)ZP__name}"
      print -r -- "${(qqqq)aliases[$ZP__name]}"
    done

    print -r -- OPTIONS
    for ZP__name in "${(@ok)options}"; do
      print -r -- O
      print -r -- "${(qqqq)ZP__name}"
      print -r -- "${options[$ZP__name]}"
    done

    print -r -- FUNCTIONS
    for ZP__name in "${(@ok)functions}"; do
      [[ "$ZP__name" == ZP__* ]] && continue
      print -r -- F
      print -r -- "${(qqqq)ZP__name}"
      print -r -- "${(qqqq)functions[$ZP__name]}"
    done

    for ZP__list in PATH FPATH; do
      print -r -- "${ZP__list}_PRESENT"
      print -r -- "${(P)+ZP__list}"
      (( ${(P)+ZP__list} )) || continue
      if [[ "$ZP__list" == PATH ]]; then ZP__values=("${(@)path}"); else ZP__values=("${(@)fpath}"); fi
      print -r -- "$ZP__list"
      print -r -- "$#ZP__values"
      for (( ZP__i = 1; ZP__i <= $#ZP__values; ZP__i++ )); do
        print -r -- "$ZP__i"
        print -r -- "${(qqqq)ZP__values[$ZP__i]}"
      done
      print -r -- "${ZP__list}_COUNTS"
      for (( ZP__i = 1; ZP__i <= $#ZP__values; ZP__i++ )); do
        ZP__path_value="${ZP__values[$ZP__i]}"
        ZP__seen=0
        for (( ZP__j = 1; ZP__j < ZP__i; ZP__j++ )); do
          if [[ "${ZP__values[$ZP__j]}" == "$ZP__path_value" ]]; then ZP__seen=1; break; fi
        done
        (( ZP__seen )) && continue
        ZP__count=0
        for ZP__value in "${(@)ZP__values}"; do [[ "$ZP__value" == "$ZP__path_value" ]] && (( ZP__count++ )); done
        print -r -- C
        print -r -- "${(qqqq)ZP__path_value}"
        print -r -- "$ZP__count"
      done
    done
  } >| "$ZP__dest"
}
`

type residueFixture struct {
	label  string
	source string
}

// residueFixtures deliberately use only accepted zsh source. Keeping the
// profile input as source (rather than duplicating Build's reduction in a test
// manifest) makes this oracle cover Parse -> IR -> Build -> Diff -> Emit.
func residueFixtures() []residueFixture {
	return []residueFixture{
		{label: "repeated-scalar-function-path-fpath", source: `
ZP_MANAGED_SET=first
export ZP_MANAGED_SET=second
ZP_MANAGED_SET=profile-a
ZP_MANAGED_EMPTY=profile-a-empty
export ZP_MANAGED_EXPORTED=profile-a-exported
ZP_MANAGED_UNSET=profile-a-created
alias foo-bar='profile-a-dash'
alias foo.bar='profile-a-dot'
alias foo_bar='profile-a-underscore'
func-bar() { print -r -- first-function; }
func-bar() { print -r -- final-function-a; }
func.bar() { print -r -- dot-function-a; }
func_bar() { print -r -- underscore-function-a; }
PATH='/head-a':$PATH:'/head-a'
PATH=$PATH:'/tail-a':$EXTRA
FPATH='/f-head-a':$FPATH
FPATH=$FPATH:'/f-tail-a':$EXTRA
unsetopt bareglobqual
`},
		{label: "second-profile-repeated-identities", source: `
ZP_MANAGED_SET=first-b
ZP_MANAGED_SET=profile-b
ZP_MANAGED_EMPTY=profile-b-empty
export ZP_MANAGED_EXPORTED=profile-b-exported
ZP_MANAGED_UNSET=profile-b-created
alias foo-bar='profile-b-dash'
alias foo.bar='profile-b-dot'
alias foo_bar='profile-b-underscore'
func-bar() { print -r -- first-function-b; }
func-bar() { print -r -- final-function-b; }
func.bar() { print -r -- dot-function-b; }
func_bar() { print -r -- underscore-function-b; }
PATH=$EXTRA:'/head-b':$PATH
PATH=$PATH:'/tail-b':'/tail-b'
FPATH=$EXTRA:'/f-head-b':$FPATH
FPATH=$FPATH:'/f-tail-b':'/f-tail-b'
setopt bareglobqual
`},
	}
}

func buildResidueManifests(t *testing.T) []model.Manifest {
	t.Helper()
	p := Provider{}
	fixtures := residueFixtures()
	manifests := make([]model.Manifest, 0, len(fixtures))
	for _, fixture := range fixtures {
		blocks, err := p.Parse([]byte(fixture.source))
		if err != nil {
			t.Fatalf("fixture=%s parse: %v", fixture.label, err)
		}
		manifest := activate.Build(ir.Build(blocks, p))
		manifest.Profile = fixture.label
		if len(manifest.Env) == 0 || len(manifest.Lists) != 2 || len(manifest.Functions.Added) != 3 {
			t.Fatalf("fixture=%s did not reach full production manifest: %#v", fixture.label, manifest)
		}
		manifests = append(manifests, manifest)
	}
	return manifests
}

type residueRun struct {
	before []byte
	noop   []byte
	after  []byte
	output []byte
}

func runResidueSequence(t *testing.T, actions []residueAction, baseline string) residueRun {
	t.Helper()
	tmp := t.TempDir()
	warmupPath := filepath.Join(tmp, "warmup.snapshot")
	beforePath := filepath.Join(tmp, "before.snapshot")
	noopPath := filepath.Join(tmp, "noop.snapshot")
	afterPath := filepath.Join(tmp, "after.snapshot")

	var script strings.Builder
	script.WriteString("zmodload zsh/parameter 2>/dev/null || exit 90\n")
	script.WriteString(residueSnapshotFunction)
	script.WriteString("export HOME=" + zquote(filepath.Join(tmp, "home")) + "\n")
	script.WriteString(baseline)
	script.WriteString("\nEXTRA='/one:/two'\n")
	script.WriteString(`
typeset ZP_LOCAL_ONLY=$'plain local\n\\value'
typeset ZP_ORACLE_NUL=$'raw\0nul\n\\tail'
typeset -a ZP_ORACLE_ARRAY=(first '' $'raw\0nul\n\\tail')
typeset -A ZP_ORACLE_ASSOC
ZP_ORACLE_ASSOC=(beta $'bee\n\\value' alpha $'aye\0nul')
setopt extendedglob
unsetopt bareglobqual
`)
	script.WriteString("alias foo-bar='__ZP_UNSET__'\nalias foo.bar='old-dot\\nbody'\nalias foo_bar='old-underscore'\n")
	script.WriteString("func-bar() { print -r -- __ZP_UNSET__; }\nfunc.bar() { print -r -- $'old-dot\\nbody'; }\nfunc_bar() { print -r -- old-underscore; }\n")

	provider := Provider{}
	for i, manifest := range buildResidueManifests(t) {
		applyPlan, err := activate.Diff(nil, &manifest)
		if err != nil {
			t.Fatal(err)
		}
		deactivatePlan, err := activate.Diff(&manifest, nil)
		if err != nil {
			t.Fatal(err)
		}
		plan := activate.Plan{Activate: applyPlan.Activate, Deactivate: deactivatePlan.Deactivate}
		apply, deactivate, err := provider.Emit(plan)
		if err != nil {
			t.Fatal(err)
		}
		script.WriteString(apply)
		script.WriteString(deactivate)
		fmt.Fprintf(&script, "functions[ZP__apply_%d]=\"${functions[zp_apply]}\"\n", i)
		fmt.Fprintf(&script, "functions[ZP__deactivate_%d]=\"${functions[zp_deactivate]}\"\n", i)
	}

	// The parameter module lazily registers a few builtins the first time it is
	// enumerated. Prime that registration before the baseline; all compared
	// snapshots still use the exact same type-filtered oracle.
	script.WriteString("ZP__snapshot " + zquote(warmupPath) + " || exit $?\n")
	script.WriteString("ZP__snapshot " + zquote(beforePath) + " || exit $?\n")
	script.WriteString("ZP__snapshot " + zquote(noopPath) + " || exit $?\n")
	for _, action := range actions {
		verb := "deactivate"
		if action.apply {
			verb = "apply"
		}
		fmt.Fprintf(&script, "ZP__%s_%d || exit $?\n", verb, action.profile)
	}
	script.WriteString("ZP__snapshot " + zquote(afterPath) + " || exit $?\n")
	script.WriteString("print -r -- PASS\n")

	cmd := exec.Command("zsh", "-f", "-c", script.String())
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("seed=%d trace=%s: zsh failed: %v\n%s\nscript:\n%s", residueSeed, formatResidueTrace(actions), err, out, script.String())
	}
	read := func(path string) []byte {
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("seed=%d trace=%s: read %s: %v", residueSeed, formatResidueTrace(actions), path, err)
		}
		return b
	}
	return residueRun{before: read(beforePath), noop: read(noopPath), after: read(afterPath), output: out}
}

func snapshotDifference(a, b []byte) string {
	if bytes.Equal(a, b) {
		return ""
	}
	aLines := bytes.Split(a, []byte{'\n'})
	bLines := bytes.Split(b, []byte{'\n'})
	limit := len(aLines)
	if len(bLines) < limit {
		limit = len(bLines)
	}
	for i := 0; i < limit; i++ {
		if !bytes.Equal(aLines[i], bLines[i]) {
			return fmt.Sprintf("line %d: before=%q after=%q", i+1, aLines[i], bLines[i])
		}
	}
	return fmt.Sprintf("line count: before=%d after=%d", len(aLines), len(bLines))
}

const residueMetaSetup = `
PATH=/meta/one:/meta/two
FPATH=/meta/f-one:/meta/f-two
ZP_BASE_PATH=$PATH
typeset ZP_META_LOCAL=known-scalar
export ZP_META_EXPORT=known-export
typeset -a ZP_META_ARRAY=(first known-array)
typeset -A ZP_META_ASSOC
ZP_META_ASSOC=(key known-assoc other stable)
typeset ZP_META_NUL_SCALAR=$'visible\0tail\n\\'
typeset ZP_META_PLAIN_SCALAR=$'visibletail\n\\'
typeset -a ZP_META_NUL_ARRAY=($'visible\0tail\n\\' $'visibletail\n\\')
alias ZP_META_ALIAS='print -r -- known-alias'
ZP_META_FUNC() { print -r -- known-function; }
unsetopt bareglobqual
`

func runSnapshotMutation(t *testing.T, setup, mutation string) residueRun {
	t.Helper()
	tmp := t.TempDir()
	warmupPath := filepath.Join(tmp, "warmup.snapshot")
	beforePath := filepath.Join(tmp, "before.snapshot")
	noopPath := filepath.Join(tmp, "noop.snapshot")
	afterPath := filepath.Join(tmp, "after.snapshot")

	var script strings.Builder
	script.WriteString("zmodload zsh/parameter 2>/dev/null || exit 90\n")
	script.WriteString(residueSnapshotFunction)
	script.WriteString(setup)
	script.WriteString("\nZP__snapshot " + zquote(warmupPath) + " || exit $?\n")
	script.WriteString("ZP__snapshot " + zquote(beforePath) + " || exit $?\n")
	script.WriteString("ZP__snapshot " + zquote(noopPath) + " || exit $?\n")
	script.WriteString(mutation)
	script.WriteString("\nZP__snapshot " + zquote(afterPath) + " || exit $?\n")

	out, err := exec.Command("zsh", "-f", "-c", script.String()).CombinedOutput()
	if err != nil {
		t.Fatalf("snapshot mutation failed: %v\n%s\nscript:\n%s", err, out, script.String())
	}
	read := func(path string) []byte {
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		return b
	}
	return residueRun{before: read(beforePath), noop: read(noopPath), after: read(afterPath), output: out}
}

func TestZeroResidueFullStateProperty(t *testing.T) {
	if _, err := exec.LookPath("zsh"); err != nil {
		t.Skip("zsh not installed")
	}
	baselines := []struct {
		label string
		src   string
	}{
		{"non-empty", "PATH='/usr/local/bin:/usr/bin:/usr/local/bin'\nFPATH='/fbase:/fbase'\ntypeset ZP_MANAGED_SET='base value'\ntypeset ZP_MANAGED_EMPTY=''\ntypeset -gx ZP_MANAGED_EXPORTED='base exported'\nunset ZP_MANAGED_UNSET\n"},
		{"present-empty", "PATH=''\nFPATH=''\ntypeset ZP_MANAGED_SET=''\ntypeset -gx ZP_MANAGED_EMPTY=''\ntypeset ZP_MANAGED_EXPORTED=''\nunset ZP_MANAGED_UNSET\n"},
		{"unset-fpath", "PATH='/usr/bin'\nunset FPATH fpath\ntypeset -gx ZP_MANAGED_SET='__ZP_UNSET__'\ntypeset ZP_MANAGED_EMPTY='__ZP_UNSET__'\ntypeset -gx ZP_MANAGED_EXPORTED=''\nunset ZP_MANAGED_UNSET\n"},
	}
	actions := generateBalancedActions(rand.New(rand.NewSource(residueSeed)), residueActionCount, len(residueFixtures()))
	trace := formatResidueTrace(actions)
	for _, baseline := range baselines {
		t.Run(baseline.label, func(t *testing.T) {
			run := runResidueSequence(t, actions, baseline.src)
			if diff := snapshotDifference(run.before, run.noop); diff != "" {
				t.Fatalf("seed=%d fixture=%s trace=%s: two no-op snapshots differ: %s", residueSeed, baseline.label, trace, diff)
			}
			if diff := snapshotDifference(run.before, run.after); diff != "" {
				t.Fatalf("seed=%d fixture=%s trace=%s: residue detected: %s", residueSeed, baseline.label, trace, diff)
			}
			if !bytes.Contains(run.output, []byte("PASS")) {
				t.Fatalf("seed=%d fixture=%s trace=%s: missing PASS marker: %q", residueSeed, baseline.label, trace, run.output)
			}
		})
	}
}

func TestResidueStoreRoundTripLegacy(t *testing.T) {
	if _, err := exec.LookPath("zsh"); err != nil {
		t.Skip("zsh not installed")
	}
	provider := Provider{}
	semantic := func(t *testing.T, source string) model.Entry {
		t.Helper()
		blocks, err := provider.Parse([]byte(source))
		if err != nil {
			t.Fatal(err)
		}
		profile := ir.Build(blocks, provider)
		if len(profile.Entries) != 1 {
			t.Fatalf("semantic source %q yielded %#v", source, profile)
		}
		return profile.Entries[0]
	}
	legacy := func(name, value string) model.Entry {
		return model.Entry{Text: name + "=" + value, Category: model.CatPath, Kind: model.KindAssignment, Names: []string{name}, Value: value, Managed: true, ValueMode: model.ValueModeLegacy, StructuralFidelityKnown: true}
	}
	persisted := roundTripPipelineProfile(t, model.Profile{Entries: []model.Entry{
		legacy("PATH", "$PATH:$HOME/bin"),
		semantic(t, "PATH=$PATH:$EXTRA\n"),
		semantic(t, "FPATH=$FPATH:$EXTRA\n"),
		legacy("FPATH", "$FPATH:$EXTRA/bin"),
	}})
	manifest := activate.Build(persisted)
	if len(manifest.Lists) != 2 {
		t.Fatalf("manifest lists=%#v", manifest.Lists)
	}
	apply, deactivate := emitPipelineListPlan(t, provider, manifest)
	run := runSnapshotMutation(t,
		"HOME=/runtime-home\nPATH=/base\nFPATH=/fbase\nEXTRA=/runtime-extra\n",
		apply+"\nzp_apply\n"+deactivate+"\nzp_deactivate\nunset -f zp_apply zp_deactivate zp_capture_scalar zp_restore_scalar\n",
	)
	if diff := snapshotDifference(run.before, run.after); diff != "" {
		t.Fatalf("persisted mixed list residue: %s", diff)
	}
}

func TestResiduePersistedRejectedStructuralNoop(t *testing.T) {
	if _, err := exec.LookPath("zsh"); err != nil {
		t.Skip("zsh not installed")
	}

	provider := Provider{}
	sources := []string{
		"FOO[2]=bar",
		"typeset -A MAP",
		"MAP[key]=bar",
		"export -i INTEGER=1",
	}
	blocks, err := provider.Parse([]byte(strings.Join(sources, "\n") + "\n"))
	if err != nil {
		t.Fatal(err)
	}
	profile := ir.Build(blocks, provider)
	for i := range profile.Entries {
		profile.Entries[i].Override = model.OverrideManaged
	}
	persisted := roundTripPipelineProfile(t, profile)
	if got := string(ir.Regenerate(persisted, provider)); got != strings.Join(sources, "\n")+"\n" {
		t.Fatalf("regenerated=%q, want verbatim source", got)
	}
	manifest := activate.Build(persisted)
	if !pipelineManifestEmpty(manifest) {
		t.Fatalf("persisted rejected source created activation intent: %#v", manifest)
	}
	applyPlan, err := activate.Diff(nil, &manifest)
	if err != nil {
		t.Fatal(err)
	}
	deactivatePlan, err := activate.Diff(&manifest, nil)
	if err != nil {
		t.Fatal(err)
	}
	apply, deactivate, err := provider.Emit(activate.Plan{Activate: applyPlan.Activate, Deactivate: deactivatePlan.Deactivate})
	if err != nil {
		t.Fatal(err)
	}

	setup := strings.Join([]string{
		"PATH=/rejected/one:/rejected/two",
		"FPATH=/rejected/f-one:/rejected/f-two",
		"ZP_BASE_PATH=$PATH",
		"typeset ZP_REJECTED_SCALAR=scalar-before",
		"typeset -a FOO=(zero indexed-before tail)",
		"typeset -A MAP=(key assoc-before other stable)",
		"typeset -ix INTEGER=7",
	}, "\n") + "\n"
	mutation := strings.Join([]string{
		apply,
		"zp_apply",
		"[[ $ZP_REJECTED_SCALAR == scalar-before && ${(t)FOO} == array && $FOO[2] == indexed-before ]] || exit 171",
		"[[ ${(t)MAP} == association && ${MAP[key]} == assoc-before ]] || exit 172",
		"ZP__integer_before=$INTEGER",
		"INTEGER+=2",
		"[[ ${parameters[INTEGER]} == integer-export && $INTEGER == 9 ]] || exit 173",
		"INTEGER=$ZP__integer_before",
		"unset ZP__integer_before",
		deactivate,
		"zp_deactivate",
		"unset -f zp_apply zp_deactivate zp_capture_scalar zp_restore_scalar",
	}, "\n")
	run := runSnapshotMutation(t, setup, mutation)
	if diff := snapshotDifference(run.before, run.noop); diff != "" {
		t.Fatalf("persisted rejected profile was not self-stable: %s", diff)
	}
	if diff := snapshotDifference(run.before, run.after); diff != "" {
		t.Fatalf("persisted rejected profile left residue: %s", diff)
	}
	for _, want := range [][]byte{[]byte("$'indexed-before'"), []byte("$'assoc-before'"), []byte("$'integer-export'")} {
		if !bytes.Contains(run.before, want) {
			t.Fatalf("snapshot lacks rejected-source evidence %q", want)
		}
	}
}

// TestResiduePersistedSourceShapeNoop binds the complete rejected source-shape
// matrix to the durable byte-for-byte full-state oracle. It is intentionally a
// persisted OverrideManaged profile, not a hand-written empty manifest.
func TestResiduePersistedSourceShapeNoop(t *testing.T) {
	if _, err := exec.LookPath("zsh"); err != nil {
		t.Skip("zsh not installed")
	}

	manifest := buildPersistedRejectedSourceShapeManifest(t)
	if !pipelineManifestEmpty(manifest) {
		t.Fatalf("persisted mixed rejected profile created activation intent: %#v", manifest)
	}
	provider := Provider{}
	applyPlan, err := activate.Diff(nil, &manifest)
	if err != nil {
		t.Fatal(err)
	}
	deactivatePlan, err := activate.Diff(&manifest, nil)
	if err != nil {
		t.Fatal(err)
	}
	apply, deactivate, err := provider.Emit(activate.Plan{Activate: applyPlan.Activate, Deactivate: deactivatePlan.Deactivate})
	if err != nil {
		t.Fatal(err)
	}

	setup := strings.Join([]string{
		"PATH=/rejected/one:/rejected/two", "FPATH=/rejected/f-one:/rejected/f-two", "ZP_BASE_PATH=$PATH",
		"typeset A=before-a B=before-b C=before-c D=before-d E=before-e F=before-f",
		"alias ll='query-before'", "alias one='one-before'", "alias two='two-before'", "setopt extendedglob",
	}, "\n") + "\n"
	mutation := strings.Join([]string{
		apply, "zp_apply", deactivate, "zp_deactivate",
		"unset -f zp_apply zp_deactivate zp_capture_scalar zp_restore_scalar",
	}, "\n")
	run := runSnapshotMutation(t, setup, mutation)
	if diff := snapshotDifference(run.before, run.noop); diff != "" {
		t.Fatalf("persisted mixed rejected profile was not self-stable: %s", diff)
	}
	if diff := snapshotDifference(run.before, run.after); diff != "" {
		t.Fatalf("persisted mixed rejected profile left residue: %s", diff)
	}

	for _, mutation := range []string{"alias one='alias-only-mutation'", "unsetopt extendedglob"} {
		mutated := runSnapshotMutation(t, setup, mutation)
		if snapshotDifference(mutated.before, mutated.after) == "" {
			t.Fatalf("snapshot oracle missed deliberate mutation %q", mutation)
		}
	}
}

func buildPersistedRejectedSourceShapeManifest(t *testing.T) model.Manifest {
	t.Helper()
	provider := Provider{}
	source := strings.Join([]string{
		"A=one B=two",
		"export C=one D=two",
		"export -- E=one F=two",
		"export -- PATH=$PATH:$EXTRA FPATH=$FPATH:$EXTRA",
		"alias ll",
		"alias one=profile-one two=profile-two",
		"setopt +o extendedglob",
		"unsetopt +o extendedglob",
		"setopt -m extendedglob",
		"unsetopt -m extendedglob",
	}, "\n") + "\n"
	blocks, err := provider.Parse([]byte(source))
	if err != nil {
		t.Fatal(err)
	}
	persisted := roundTripPipelineProfile(t, ir.Build(blocks, provider))
	for i := range persisted.Entries {
		persisted.Entries[i].Override = model.OverrideManaged
	}
	if got := string(ir.Regenerate(persisted, provider)); got != source {
		t.Fatalf("rejected persisted profile regenerated as %q, want %q", got, source)
	}
	return activate.Build(persisted)
}

func TestEffectiveIdentitySourcePipeline(t *testing.T) {
	if _, err := exec.LookPath("zsh"); err != nil {
		t.Skip("zsh not installed")
	}
	fixture := residueFixtures()[0]
	provider := Provider{}
	blocks, err := provider.Parse([]byte(fixture.source))
	if err != nil {
		t.Fatal(err)
	}
	manifest := activate.Build(ir.Build(blocks, provider))
	if len(manifest.Env) == 0 || manifest.Env[0].Name != "ZP_MANAGED_SET" || manifest.Env[0].Applied != "profile-a" {
		t.Fatalf("fixture=%s did not retain its final scalar identity: %#v", fixture.label, manifest.Env)
	}
	if body := manifest.Functions.Bodies["func-bar"]; !strings.Contains(body, "final-function-a") {
		t.Fatalf("fixture=%s did not retain final function body: %q", fixture.label, body)
	}
	applyPlan, err := activate.Diff(nil, &manifest)
	if err != nil {
		t.Fatal(err)
	}
	deactivatePlan, err := activate.Diff(&manifest, nil)
	if err != nil {
		t.Fatal(err)
	}
	apply, deactivate, err := provider.Emit(activate.Plan{Activate: applyPlan.Activate, Deactivate: deactivatePlan.Deactivate})
	if err != nil {
		t.Fatal(err)
	}
	script := strings.Join([]string{
		"PATH=/base", "FPATH=/fbase", "EXTRA=/one:/two",
		"typeset -gx ZP_MANAGED_SET='before'", "func-bar() { print -r -- before-function; }",
		apply, "zp_apply",
		"[[ $ZP_MANAGED_SET == profile-a && ${parameters[ZP_MANAGED_SET]} == *export* ]] || exit 61",
		"[[ $(func-bar) == final-function-a ]] || exit 62",
		deactivate, "zp_deactivate",
		"[[ $ZP_MANAGED_SET == before && ${parameters[ZP_MANAGED_SET]} == *export* ]] || exit 63",
		"[[ $(func-bar) == before-function ]] || exit 64",
	}, "\n")
	if out, err := exec.Command("zsh", "-f", "-c", script).CombinedOutput(); err != nil {
		t.Fatalf("fixture=%s final effective identity: %v\n%s", fixture.label, err, out)
	}
}

func TestDynamicCardinalitySourcePipeline(t *testing.T) {
	if _, err := exec.LookPath("zsh"); err != nil {
		t.Skip("zsh not installed")
	}
	const source = "PATH=$EXTRA:$PATH\nFPATH=$EXTRA:$FPATH\n"
	p := Provider{}
	blocks, err := p.Parse([]byte(source))
	if err != nil {
		t.Fatal(err)
	}
	manifest := activate.Build(ir.Build(blocks, p))
	applyPlan, err := activate.Diff(nil, &manifest)
	if err != nil {
		t.Fatal(err)
	}
	apply, _, err := p.Emit(applyPlan)
	if err != nil {
		t.Fatal(err)
	}
	for _, extra := range []string{"/one:/two", ""} {
		t.Run(fmt.Sprintf("extra=%q", extra), func(t *testing.T) {
			// The left side executes exactly the accepted source; the right side
			// executes the production Parse -> IR -> Build -> Diff -> Emit result.
			script := strings.Join([]string{
				"EXTRA=" + zquote(extra), "PATH=/base", "FPATH=/fbase", source,
				"typeset ZP__direct_path=$PATH ZP__direct_fpath=$FPATH",
				"PATH=/base", "FPATH=/fbase", apply, "zp_apply",
				"[[ $PATH == $ZP__direct_path && $FPATH == $ZP__direct_fpath ]] || exit 71",
			}, "\n")
			if out, err := exec.Command("zsh", "-f", "-c", script).CombinedOutput(); err != nil {
				t.Fatalf("extra=%q dynamic cardinality differs from direct source: %v\n%s", extra, err, out)
			}
		})
	}
}

func TestSnapshotOracleMetaSensitivity(t *testing.T) {
	if _, err := exec.LookPath("zsh"); err != nil {
		t.Skip("zsh not installed")
	}
	stable := runSnapshotMutation(t, residueMetaSetup, ":\n")
	if diff := snapshotDifference(stable.before, stable.noop); diff != "" {
		t.Fatalf("no-op snapshots differ: %s", diff)
	}
	if diff := snapshotDifference(stable.before, stable.after); diff != "" {
		t.Fatalf("no-op mutation changed snapshot: %s", diff)
	}
	for _, value := range []string{"known-scalar", "known-array", "known-assoc"} {
		encoded := []byte("$'" + value + "'")
		if !bytes.Contains(stable.before, encoded) {
			t.Fatalf("snapshot lacks dereferenced fixture value %q", value)
		}
	}

	mutations := []struct {
		name   string
		script string
	}{
		{name: "non-exported scalar", script: "ZP_META_LOCAL=changed-local\n"},
		{name: "scalar presence", script: "unset ZP_META_LOCAL\n"},
		{name: "exported scalar value", script: "ZP_META_EXPORT=changed-export\n"},
		{name: "export attribute", script: "typeset +x ZP_META_EXPORT\n"},
		{name: "indexed-array element", script: "ZP_META_ARRAY[2]=changed-array\n"},
		{name: "associative-array value", script: "ZP_META_ASSOC[key]=changed-assoc\n"},
		{name: "alias body", script: "alias ZP_META_ALIAS='print -r -- changed-alias'\n"},
		{name: "function body", script: "functions[ZP_META_FUNC]='print -r -- changed-function'\n"},
		{name: "option toggle", script: "setopt bareglobqual\n"},
		{name: "count-preserving PATH reorder", script: "path=($path[2] $path[1])\n"},
		{name: "PATH base presence", script: "unset PATH path\n"},
		{name: "count-preserving FPATH reorder", script: "fpath=($fpath[2] $fpath[1])\n"},
		{name: "FPATH base presence", script: "unset FPATH fpath\n"},
	}
	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			run := runSnapshotMutation(t, residueMetaSetup, mutation.script)
			if diff := snapshotDifference(run.before, run.noop); diff != "" {
				t.Fatalf("oracle was not self-stable before %s: %s", mutation.name, diff)
			}
			if diff := snapshotDifference(run.before, run.after); diff == "" {
				t.Fatalf("oracle missed %s", mutation.name)
			}
		})
	}
}

func TestSnapshotEscapesEmbeddedNULWithoutCollision(t *testing.T) {
	if _, err := exec.LookPath("zsh"); err != nil {
		t.Skip("zsh not installed")
	}
	run := runSnapshotMutation(t, residueMetaSetup, ":\n")
	if diff := snapshotDifference(run.before, run.noop); diff != "" {
		t.Fatalf("NUL-bearing snapshot was not self-stable: %s", diff)
	}
	if diff := snapshotDifference(run.before, run.after); diff != "" {
		t.Fatalf("NUL-bearing no-op snapshot changed: %s", diff)
	}
	if bytes.ContainsRune(run.before, 0) {
		t.Fatal("snapshot contains a raw NUL delimiter")
	}
	nulField := []byte(`$'visible\0tail\n\\'`)
	plainField := []byte(`$'visibletail\n\\'`)
	if bytes.Equal(nulField, plainField) {
		t.Fatal("test instrumentation conflated NUL and non-NUL fields")
	}
	if bytes.Count(run.before, nulField) < 2 {
		t.Fatalf("NUL-bearing scalar and array fields missing: %q", nulField)
	}
	if bytes.Count(run.before, plainField) < 2 {
		t.Fatalf("visible non-NUL scalar and array fields missing: %q", plainField)
	}
}

func residueFixtureLoaders(t *testing.T) (string, string) {
	t.Helper()
	p := Provider{}
	fixture := residueFixtures()[0]
	blocks, err := p.Parse([]byte(fixture.source))
	if err != nil {
		t.Fatal(err)
	}
	manifest := activate.Build(ir.Build(blocks, p))
	applyPlan, err := activate.Diff(nil, &manifest)
	if err != nil {
		t.Fatal(err)
	}
	deactivatePlan, err := activate.Diff(&manifest, nil)
	if err != nil {
		t.Fatal(err)
	}
	return mustEmitResiduePlan(t, p, activate.Plan{Activate: applyPlan.Activate, Deactivate: deactivatePlan.Deactivate})
}

func mustEmitResiduePlan(t *testing.T, p Provider, plan activate.Plan) (string, string) {
	t.Helper()
	apply, deactivate, err := p.Emit(plan)
	if err != nil {
		t.Fatal(err)
	}
	return apply, deactivate
}

func TestResidueStateBookkeepingMutants(t *testing.T) {
	if _, err := exec.LookPath("zsh"); err != nil {
		t.Skip("zsh not installed")
	}
	apply, deactivate := residueFixtureLoaders(t)
	base := strings.Join([]string{
		"PATH=/base", "FPATH=/fbase", "EXTRA=/one:/two",
		"typeset -gx ZP_MANAGED_SET=before", "typeset ZP_MANAGED_EMPTY=''", "typeset -gx ZP_MANAGED_EXPORTED=before-exported", "unset ZP_MANAGED_UNSET",
		"alias foo-bar='__ZP_UNSET__'", "alias foo.bar=old-dot", "alias foo_bar=old-underscore",
		"func-bar() { print -r -- __ZP_UNSET__; }", "func.bar() { print -r -- old-dot; }", "func_bar() { print -r -- old-underscore; }",
	}, "\n")
	runMutant := func(t *testing.T, mutatedApply, mutatedDeactivate, sequence, check, marker string) {
		t.Helper()
		run := runSnapshotMutation(t, base+"\n"+mutatedApply+"\n"+mutatedDeactivate, sequence+"\n"+check+"\nprint -r -- "+marker)
		if diff := snapshotDifference(run.before, run.after); diff == "" {
			t.Fatalf("%s mutant left the oracle unchanged", marker)
		}
		if !bytes.Contains(run.output, []byte(marker)) {
			t.Fatalf("%s mutant failed for the wrong reason: %q", marker, run.output)
		}
	}

	t.Run("restore first instead of final scalar", func(t *testing.T) {
		slot := encodeSlot("APPLIED_SCALAR", "ZP_MANAGED_SET")
		bad := strings.Replace(apply, "typeset -g "+slot+"=\"$ZP_MANAGED_SET\"", "typeset -g "+slot+"='first'", 1)
		runMutant(t, bad, deactivate, "zp_apply\nzp_deactivate", "[[ $ZP_MANAGED_SET == profile-a ]] || exit 81", "MUTANT_STALE_APPLIED")
	})
	t.Run("force plain scalar export", func(t *testing.T) {
		bad := strings.Replace(apply, "typeset -g ZP_MANAGED_EMPTY='profile-a-empty'", "export ZP_MANAGED_EMPTY='profile-a-empty'", 1)
		runMutant(t, bad, deactivate, "zp_apply", "[[ ${parameters[ZP_MANAGED_EMPTY]} == *export* ]] || exit 82", "MUTANT_FORCE_EXPORT")
	})
	t.Run("drop prior export restoration", func(t *testing.T) {
		bad := strings.Replace(deactivate, "if [[ \"${(P)exportSlot}\" == 1 ]]; then export \"$var\"; else typeset +x \"$var\"; fi", "typeset +x \"$var\"", 1)
		runMutant(t, apply, bad, "zp_apply\nzp_deactivate", "[[ ${parameters[ZP_MANAGED_SET]} != *export* ]] || exit 83", "MUTANT_DROP_EXPORT_RESTORE")
	})
	t.Run("punctuation slot collision", func(t *testing.T) {
		dash, dot := encodeSlot("PRESENT_ALIAS", "foo-bar"), encodeSlot("PRESENT_ALIAS", "foo.bar")
		origDash, origDot := encodeSlot("ORIGINAL_ALIAS", "foo-bar"), encodeSlot("ORIGINAL_ALIAS", "foo.bar")
		badApply := strings.NewReplacer(dot, dash, origDot, origDash).Replace(apply)
		badDeactivate := strings.NewReplacer(dot, dash, origDot, origDash).Replace(deactivate)
		runMutant(t, badApply, badDeactivate, "zp_apply\nzp_deactivate", "[[ ${aliases[foo.bar]} != old-dot ]] || exit 84", "MUTANT_SLOT_COLLISION")
	})
	t.Run("sentinel treated as absence", func(t *testing.T) {
		presence := encodeSlot("PRESENT_ALIAS", "foo-bar")
		bad := strings.Replace(apply, "typeset -g "+presence+"=1", "typeset -g "+presence+"=0", 1)
		runMutant(t, bad, deactivate, "zp_apply\nzp_deactivate", "[[ ${+aliases[foo-bar]} == 0 ]] || exit 85", "MUTANT_SENTINEL_ABSENT")
	})
	t.Run("unset FPATH collapsed to empty", func(t *testing.T) {
		bad := strings.Replace(deactivate, "unset FPATH fpath", "FPATH=''", 1)
		run := runSnapshotMutation(t, strings.Replace(base, "FPATH=/fbase", "unset FPATH fpath", 1)+"\n"+apply+"\n"+bad, "zp_apply\nzp_deactivate\n[[ ${+FPATH} == 1 && -z $FPATH ]] || exit 86\nprint -r -- MUTANT_FPATH_PRESENCE")
		if diff := snapshotDifference(run.before, run.after); diff == "" || !bytes.Contains(run.output, []byte("MUTANT_FPATH_PRESENCE")) {
			t.Fatalf("FPATH presence mutant was not uniquely rejected: diff=%q output=%q", diff, run.output)
		}
	})
	t.Run("dynamic multi-element becomes literal", func(t *testing.T) {
		bad := strings.ReplaceAll(apply, "$EXTRA", "'$EXTRA'")
		runMutant(t, bad, deactivate, "zp_apply", "[[ \"$PATH\" == *'$EXTRA' ]] || exit 87", "MUTANT_DYNAMIC_CARDINALITY")
	})
}

func TestResidueRendererMutants(t *testing.T) {
	if _, err := exec.LookPath("zsh"); err != nil {
		t.Skip("zsh not installed")
	}
	t.Run("blind PATH append", func(t *testing.T) {
		production := renderList
		t.Cleanup(func() { renderList = production })
		renderList = func(_ string, additions []string, _ bool) string {
			parts := make([]string, 0, len(additions))
			for _, addition := range additions {
				parts = append(parts, renderValue(addition, dynamicSegment(addition)))
			}
			return fmt.Sprintf("path=(%s $path)", strings.Join(parts, " "))
		}
		plan := activate.Plan{Activate: []activate.Op{
			activate.ApplyListDelta{Name: "PATH", Additions: []string{"/usr/local/bin"}},
		}}
		apply, _, err := (Provider{}).Emit(plan)
		if err != nil {
			t.Fatal(err)
		}
		setup := "PATH=/usr/local/bin:/usr/bin\nZP_BASE_PATH=$PATH\n" + apply
		mutation := "zp_apply\nzp_apply\n(( $#path > 2 )) || exit 71\nprint -r -- MUTANT_BLIND_APPEND\n"
		run := runSnapshotMutation(t, setup, mutation)
		if diff := snapshotDifference(run.before, run.after); diff == "" {
			t.Fatal("blind-append mutant did not grow or reorder PATH")
		}
		if !bytes.Contains(run.output, []byte("MUTANT_BLIND_APPEND")) {
			t.Fatalf("blind-append mutant failed for the wrong reason: %q", run.output)
		}
	})

	t.Run("dropped static quoting", func(t *testing.T) {
		production := renderValue
		t.Cleanup(func() { renderValue = production })
		renderValue = func(applied string, _ bool) string { return applied }
		canary := filepath.Join(t.TempDir(), "quote-canary")
		payload := "literal; : > " + canary
		plan := activate.Plan{Activate: []activate.Op{
			activate.SetScalar{Name: "ZP_MUTANT_VALUE", Applied: payload, Dynamic: false},
		}}
		apply, _, err := (Provider{}).Emit(plan)
		if err != nil {
			t.Fatal(err)
		}
		setup := "PATH=/usr/bin\nZP_BASE_PATH=$PATH\nZP_MUTANT_VALUE=base\n" + apply
		mutation := "zp_apply\n[[ -e " + zquote(canary) + " ]] || exit 72\n[[ \"$ZP_MUTANT_VALUE\" != " + zquote(payload) + " ]] || exit 73\nprint -r -- MUTANT_DROPPED_ZQUOTE\n"
		run := runSnapshotMutation(t, setup, mutation)
		if diff := snapshotDifference(run.before, run.after); diff == "" {
			t.Fatal("dropped-zquote mutant preserved an exact literal value")
		}
		if !bytes.Contains(run.output, []byte("MUTANT_DROPPED_ZQUOTE")) {
			t.Fatalf("dropped-zquote mutant failed for the wrong reason: %q", run.output)
		}
	})

	t.Run("base stripping", func(t *testing.T) {
		production := renderList
		t.Cleanup(func() { renderList = production })
		renderList = func(name string, _ []string, _ bool) string {
			return name + "=" + zquote("/usr/bin")
		}
		plan := activate.Plan{Activate: []activate.Op{
			activate.ApplyListDelta{Name: "PATH", Additions: []string{"/opt/profile"}},
		}}
		apply, _, err := (Provider{}).Emit(plan)
		if err != nil {
			t.Fatal(err)
		}
		setup := "PATH=/usr/local/bin:/usr/bin\nZP_BASE_PATH=$PATH\n" + apply
		mutation := "zp_apply\n[[ \"$PATH\" == /usr/bin ]] || exit 74\n(( $#path == 1 )) || exit 75\nprint -r -- MUTANT_BASE_STRIP\n"
		run := runSnapshotMutation(t, setup, mutation)
		if diff := snapshotDifference(run.before, run.after); diff == "" {
			t.Fatal("base-strip mutant did not lose a baseline PATH entry")
		}
		if !bytes.Contains(run.output, []byte("MUTANT_BASE_STRIP")) {
			t.Fatalf("base-strip mutant failed for the wrong reason: %q", run.output)
		}
	})

	// Subtest cleanups restore both package seams before this final real-emitter
	// property run. Keeping it here prevents a false green caused by leaked
	// mutable renderer state.
	actions := generateBalancedActions(rand.New(rand.NewSource(residueSeed)), residueActionCount, len(residueFixtures()))
	run := runResidueSequence(t, actions, "PATH='/usr/bin'\nFPATH='/fbase'\ntypeset ZP_MANAGED_SET=base\ntypeset ZP_MANAGED_EMPTY=''\ntypeset -gx ZP_MANAGED_EXPORTED=base\nunset ZP_MANAGED_UNSET\n")
	if diff := snapshotDifference(run.before, run.after); diff != "" {
		t.Fatalf("production emitter failed after mutant cleanup: %s", diff)
	}
}
