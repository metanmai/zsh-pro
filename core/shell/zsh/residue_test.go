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
	"zsh-pro/core/model"
)

const (
	residueSeed        int64 = 0x5eed0405
	residueActionCount       = 24
)

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
  local ZP__name ZP__type ZP__scalar ZP__key ZP__value ZP__path_value
  local -a ZP__values ZP__items
  local -i ZP__i ZP__j ZP__count ZP__seen
  {
    print -r -- PARAMETERS
    for ZP__name in "${(@ok)parameters}"; do
      [[ "$ZP__name" == ZP__* ]] && continue
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

    print -r -- PATH
    print -r -- "$#path"
    for (( ZP__i = 1; ZP__i <= $#path; ZP__i++ )); do
      print -r -- "$ZP__i"
      print -r -- "${(qqqq)path[$ZP__i]}"
    done
    print -r -- PATH_COUNTS
    for (( ZP__i = 1; ZP__i <= $#path; ZP__i++ )); do
      ZP__path_value="${path[$ZP__i]}"
      ZP__seen=0
      for (( ZP__j = 1; ZP__j < ZP__i; ZP__j++ )); do
        if [[ "${path[$ZP__j]}" == "$ZP__path_value" ]]; then
          ZP__seen=1
          break
        fi
      done
      (( ZP__seen )) && continue
      ZP__count=0
      for ZP__value in "${(@)path}"; do
        [[ "$ZP__value" == "$ZP__path_value" ]] && (( ZP__count++ ))
      done
      print -r -- C
      print -r -- "${(qqqq)ZP__path_value}"
      print -r -- "$ZP__count"
    done

    print -r -- BASE_PATH
    print -r -- "${(qqqq)ZP_BASE_PATH}"
  } >| "$ZP__dest"
}
`

func residueBool(v bool) *bool { return &v }

func residueManifests() []model.Manifest {
	return []model.Manifest{
		{
			Profile: "residue-a", Schema: model.SchemaV1,
			Env: []model.Scalar{
				{Name: "ZP_MANAGED_SET", Applied: "profile-a", Dynamic: residueBool(false)},
				{Name: "ZP_MANAGED_UNSET_A", Applied: "created-a", Dynamic: residueBool(false)},
				{Name: "ZP_DYNAMIC_HOME", Applied: "$HOME/profile-a", Dynamic: residueBool(true)},
			},
			Lists: []model.ListDelta{{Name: "PATH", Additions: []string{"/usr/local/bin", "/opt/tool*", "$HOME/bin"}}},
			Aliases: model.AliasSet{
				Added:   map[string]string{"ll": `print -r -- "profile a's alias"`},
				Dynamic: map[string]bool{"ll": false},
			},
			Functions: model.FuncSet{
				Added:  []string{"ff"},
				Bodies: map[string]string{"ff": `print -r -- "profile a's function"`},
			},
			Options: []model.OptionSet{{Name: "extendedglob", Enabled: false}},
		},
		{
			Profile: "residue-b", Schema: model.SchemaV1,
			Env: []model.Scalar{
				{Name: "ZP_MANAGED_SET", Applied: "profile-b", Dynamic: residueBool(false)},
				{Name: "ZP_MANAGED_UNSET_B", Applied: "created-b", Dynamic: residueBool(false)},
				{Name: "ZP_DYNAMIC_HOME", Applied: "$HOME/profile-b", Dynamic: residueBool(true)},
			},
			Lists: []model.ListDelta{{Name: "PATH", Additions: []string{"/opt/profile-b", "$HOME/bin"}}},
			Aliases: model.AliasSet{
				Added:   map[string]string{"ll": `print -r -- "profile b's alias"`},
				Dynamic: map[string]bool{"ll": false},
			},
			Functions: model.FuncSet{
				Added:  []string{"ff"},
				Bodies: map[string]string{"ff": `print -r -- "profile b's function"`},
			},
			Options: []model.OptionSet{{Name: "bareglobqual", Enabled: true}},
		},
	}
}

type residueRun struct {
	before []byte
	noop   []byte
	after  []byte
	output []byte
}

func runResidueSequence(t *testing.T, actions []residueAction) residueRun {
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
	script.WriteString("PATH=/usr/local/bin:/usr/bin:/bin\nZP_BASE_PATH=$PATH\n")
	script.WriteString("export ZP_MANAGED_SET='base value'\nunset ZP_MANAGED_UNSET_A ZP_MANAGED_UNSET_B ZP_DYNAMIC_HOME\n")
	script.WriteString(`
typeset ZP_LOCAL_ONLY=$'plain local\n\\value'
typeset ZP_ORACLE_NUL=$'raw\0nul\n\\tail'
typeset -a ZP_ORACLE_ARRAY=(first '' $'raw\0nul\n\\tail')
typeset -A ZP_ORACLE_ASSOC
ZP_ORACLE_ASSOC=(beta $'bee\n\\value' alpha $'aye\0nul')
setopt extendedglob
unsetopt bareglobqual
`)
	script.WriteString("alias ll=" + zquote(`print -r -- "old alias's body"`) + "\n")
	script.WriteString("ff() { print -r -- \"old function's body\"; }\n")

	provider := Provider{}
	for i, manifest := range residueManifests() {
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

	script.WriteString(`
typeset ZP__baseline_path="$PATH"
typeset ZP__baseline_base="$ZP_BASE_PATH"
typeset -gi ZP__baseline_count=$#path
typeset -gi ZP__owned_count=0
typeset ZP__e=''
`)
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
	script.WriteString(`
[[ "$PATH" == "$ZP__baseline_path" ]] || { print -u2 -r -- "PATH changed: $PATH"; exit 81; }
(( $#path == ZP__baseline_count )) || { print -u2 -r -- "path count changed: $#path"; exit 82; }
for ZP__e in "${(@)path}"; do
  [[ "$ZP__e" == /usr/local/bin ]] && (( ZP__owned_count++ ))
  [[ "$ZP__e" == '/opt/tool*' ]] && { print -u2 -r -- 'profile-only path survived'; exit 83; }
done
(( ZP__owned_count == 1 )) || { print -u2 -r -- "base ownership count=$ZP__owned_count"; exit 84; }
[[ "$ZP_BASE_PATH" == "$ZP__baseline_base" ]] || { print -u2 -r -- 'ZP_BASE_PATH changed'; exit 85; }
print -r -- PASS
`)

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
	actions := generateBalancedActions(rand.New(rand.NewSource(residueSeed)), residueActionCount, len(residueManifests()))
	trace := formatResidueTrace(actions)
	run := runResidueSequence(t, actions)
	if diff := snapshotDifference(run.before, run.noop); diff != "" {
		t.Fatalf("seed=%d trace=%s: two no-op snapshots differ: %s", residueSeed, trace, diff)
	}
	if diff := snapshotDifference(run.before, run.after); diff != "" {
		t.Fatalf("seed=%d trace=%s: residue detected: %s", residueSeed, trace, diff)
	}
	if !bytes.Contains(run.output, []byte("PASS")) {
		t.Fatalf("seed=%d trace=%s: missing PASS marker: %q", residueSeed, trace, run.output)
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
		{name: "exported scalar value", script: "ZP_META_EXPORT=changed-export\n"},
		{name: "indexed-array element", script: "ZP_META_ARRAY[2]=changed-array\n"},
		{name: "associative-array value", script: "ZP_META_ASSOC[key]=changed-assoc\n"},
		{name: "alias body", script: "alias ZP_META_ALIAS='print -r -- changed-alias'\n"},
		{name: "function body", script: "functions[ZP_META_FUNC]='print -r -- changed-function'\n"},
		{name: "option toggle", script: "setopt bareglobqual\n"},
		{name: "count-preserving PATH reorder", script: "path=($path[2] $path[1])\n"},
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
	actions := generateBalancedActions(rand.New(rand.NewSource(residueSeed)), residueActionCount, 2)
	run := runResidueSequence(t, actions)
	if diff := snapshotDifference(run.before, run.after); diff != "" {
		t.Fatalf("production emitter failed after mutant cleanup: %s", diff)
	}
}
