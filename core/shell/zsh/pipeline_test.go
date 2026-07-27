package zsh

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"zsh-pro/core/activate"
	"zsh-pro/core/ir"
	"zsh-pro/core/model"
	"zsh-pro/core/store"
)

func TestPipelinePreservesValuesAndFunctionsInLiveZsh(t *testing.T) {
	if _, err := exec.LookPath("zsh"); err != nil {
		t.Skip("zsh not available")
	}

	tmp := t.TempDir()
	home := filepath.Join(tmp, "controlled-home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	canary := filepath.Join(tmp, "canary")
	redirected := filepath.Join(tmp, "redirected-output")
	quotedWant := "space ; * [x] $(touch " + canary + ")"

	source := fmt.Sprintf(`LITERAL='$HOME'
DYNAMIC=$HOME
QUOTED=%s
alias literal='echo $HOME'
alias dynamic='echo '$HOME
alias with_eq='printf %%s a=b'
empty(){}
multiline() {
  print -r -- line-one
  print -r -- line-two
}
subshell() ( print -r -- subshell-hi )
redirected_fn() { print -r -- redirected-hi } > %s
`, zquote(quotedWant), zquote(redirected))

	provider := Provider{}
	blocks, err := provider.Parse([]byte(source))
	if err != nil {
		t.Fatal(err)
	}
	profile := ir.Build(blocks, provider)
	manifest := activate.Build(profile)
	manifest.Profile = "pipeline"

	applyPlan, err := activate.Diff(nil, &manifest)
	if err != nil {
		t.Fatal(err)
	}
	deactivatePlan, err := activate.Diff(&manifest, nil)
	if err != nil {
		t.Fatal(err)
	}
	apply, _, err := provider.Emit(applyPlan)
	if err != nil {
		t.Fatal(err)
	}
	_, deactivate, err := provider.Emit(deactivatePlan)
	if err != nil {
		t.Fatal(err)
	}

	for name, src := range map[string]string{"apply": apply, "deactivate": deactivate} {
		cmd := exec.Command("zsh", "-n")
		cmd.Stdin = strings.NewReader(src)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%s syntax: %v\n%s\n%s", name, err, out, src)
		}
	}

	script := fmt.Sprintf(`
unset LITERAL DYNAMIC QUOTED
unalias literal dynamic with_eq 2>/dev/null
unset -f empty multiline subshell redirected_fn 2>/dev/null
HOME=%s
ZP_BASE_PATH="$PATH"
ZP_BASE_FPATH="$FPATH"
%s
zp_apply
[[ "$LITERAL" == '$HOME' ]] || { print -u2 -- "literal=$LITERAL"; exit 11; }
[[ "$DYNAMIC" == %s ]] || { print -u2 -- "dynamic=$DYNAMIC"; exit 12; }
[[ "$QUOTED" == %s ]] || { print -u2 -- "quoted=$QUOTED"; exit 13; }
[[ "${aliases[literal]}" == 'echo $HOME' ]] || { print -u2 -- "literal-alias=${aliases[literal]}"; exit 14; }
[[ "${aliases[dynamic]}" == %s ]] || { print -u2 -- "dynamic-alias=${aliases[dynamic]}"; exit 15; }
[[ "${aliases[with_eq]}" == 'printf %%s a=b' ]] || { print -u2 -- "equals-alias=${aliases[with_eq]}"; exit 16; }
(( ${+functions[empty]} )) || exit 17
empty || exit 18
[[ "$(multiline)" == $'line-one\nline-two' ]] || exit 19
[[ "$(subshell)" == 'subshell-hi' ]] || exit 20
redirect_stdout="$(redirected_fn)"
[[ -z "$redirect_stdout" ]] || { print -u2 -- "redirect-stdout=$redirect_stdout"; exit 21; }
[[ ! -e %s ]] || { print -u2 -- 'canary fired'; exit 22; }
%s
zp_deactivate
(( ! ${+LITERAL} && ! ${+DYNAMIC} && ! ${+QUOTED} )) || exit 23
(( ! ${+aliases[literal]} && ! ${+aliases[dynamic]} && ! ${+aliases[with_eq]} )) || exit 24
(( ! ${+functions[empty]} && ! ${+functions[multiline]} && ! ${+functions[subshell]} && ! ${+functions[redirected_fn]} )) || exit 25
`, zquote(home), apply, zquote(home), zquote(quotedWant), zquote("echo "+home), zquote(canary), deactivate)

	if out, err := exec.Command("zsh", "-f", "-c", script).CombinedOutput(); err != nil {
		t.Fatalf("live zsh pipeline: %v\n%s\nsource:\n%s\nscript:\n%s", err, out, source, script)
	}
	got, err := os.ReadFile(redirected)
	if err != nil {
		t.Fatalf("redirected function did not write output: %v", err)
	}
	if string(got) != "redirected-hi\n" {
		t.Fatalf("redirected output=%q", got)
	}
	if _, err := os.Stat(canary); !os.IsNotExist(err) {
		t.Fatalf("canary exists after pipeline: %v", err)
	}
}

func TestPipelineRepeatedCollisionSentinelAndExportRestoration(t *testing.T) {
	if _, err := exec.LookPath("zsh"); err != nil {
		t.Skip("zsh not available")
	}

	source := `
FOO=one
export FOO=two
FOO=three
EMPTY=managed
export EXPORTED=managed
UNSET=managed
alias foo-bar='new-dash'
alias foo.bar='new-dot'
alias foo_bar='new-underscore'
func-bar() { print new-dash; }
func.bar() { print new-dot; }
func_bar() { print new-underscore; }
func-bar() { print final-dash; }
`
	provider := Provider{}
	blocks, err := provider.Parse([]byte(source))
	if err != nil {
		t.Fatal(err)
	}
	manifest := activate.Build(ir.Build(blocks, provider))
	manifest.Profile = "pipeline-identities"
	if len(manifest.Env) != 4 || len(manifest.Aliases.Added) != 3 || len(manifest.Functions.Added) != 3 {
		t.Fatalf("manifest did not reduce final identities: %#v", manifest)
	}

	applyPlan, err := activate.Diff(nil, &manifest)
	if err != nil {
		t.Fatal(err)
	}
	deactivatePlan, err := activate.Diff(&manifest, nil)
	if err != nil {
		t.Fatal(err)
	}
	apply, _, err := provider.Emit(applyPlan)
	if err != nil {
		t.Fatal(err)
	}
	_, deactivate, err := provider.Emit(deactivatePlan)
	if err != nil {
		t.Fatal(err)
	}

	script := strings.Join([]string{
		"FOO=before",
		"typeset EMPTY=''",
		"typeset -gx EXPORTED='before-exported'",
		"unset UNSET",
		"alias foo-bar='__ZP_UNSET__'",
		"alias foo.bar='old-dot'",
		"alias foo_bar='old-underscore'",
		"func-bar() { print __ZP_UNSET__; }",
		"func.bar() { print old-dot; }",
		"func_bar() { print old-underscore; }",
		apply,
		"zp_apply",
		"[[ $FOO == three && ${parameters[FOO]} == *export* ]] || exit 31",
		"[[ $EMPTY == managed && ${parameters[EMPTY]} != *export* ]] || exit 32",
		"[[ $EXPORTED == managed && ${parameters[EXPORTED]} == *export* ]] || exit 33",
		"[[ $UNSET == managed ]] || exit 34",
		"[[ $(func-bar) == final-dash && $(func.bar) == new-dot && $(func_bar) == new-underscore ]] || exit 35",
		deactivate,
		"zp_deactivate",
		"[[ $FOO == before && ${parameters[FOO]} != *export* ]] || exit 36",
		"[[ ${+EMPTY} == 1 && $EMPTY == '' && ${parameters[EMPTY]} != *export* ]] || exit 37",
		"[[ $EXPORTED == before-exported && ${parameters[EXPORTED]} == *export* ]] || exit 38",
		"[[ ${+UNSET} == 0 ]] || exit 39",
		"[[ ${aliases[foo-bar]} == __ZP_UNSET__ && ${aliases[foo.bar]} == old-dot && ${aliases[foo_bar]} == old-underscore ]] || exit 40",
		"[[ $(func-bar) == __ZP_UNSET__ && $(func.bar) == old-dot && $(func_bar) == old-underscore ]] || exit 41",
	}, "\n")
	if out, err := exec.Command("zsh", "-f", "-c", script).CombinedOutput(); err != nil {
		t.Fatalf("live identity pipeline: %v\n%s\nsource:\n%s\nscript:\n%s", err, out, source, script)
	}
}

func TestPipelineComposedPathAndFPathDynamicExpansion(t *testing.T) {
	if _, err := exec.LookPath("zsh"); err != nil {
		t.Skip("zsh not available")
	}
	source := "PATH=/a:$PATH\nPATH=$PATH:/b\nFPATH=$EXTRA:$FPATH\n"
	p := Provider{}
	blocks, err := p.Parse([]byte(source))
	if err != nil {
		t.Fatal(err)
	}
	m := activate.Build(ir.Build(blocks, p))
	m.Profile = "lists"
	applyPlan, err := activate.Diff(nil, &m)
	if err != nil {
		t.Fatal(err)
	}
	deactivatePlan, err := activate.Diff(&m, nil)
	if err != nil {
		t.Fatal(err)
	}
	apply, _, err := p.Emit(applyPlan)
	if err != nil {
		t.Fatal(err)
	}
	_, deactivate, err := p.Emit(deactivatePlan)
	if err != nil {
		t.Fatal(err)
	}
	for _, extra := range []string{"/one:/two", ""} {
		script := strings.Join([]string{
			"PATH=/base", "FPATH=/fbase", "EXTRA=" + zquote(extra), apply, "zp_apply",
			"[[ $PATH == /a:/base:/b ]] || exit 71",
			"if [[ -n $EXTRA ]]; then [[ $FPATH == /one:/two:/fbase ]] || exit 72; else [[ $FPATH == :/fbase ]] || exit 73; fi",
			deactivate, "zp_deactivate", "[[ $PATH == /base && $FPATH == /fbase ]] || exit 74",
		}, "\n")
		if out, err := exec.Command("zsh", "-f", "-c", script).CombinedOutput(); err != nil {
			t.Fatalf("extra case did not match direct scalar behavior: %v %s", err, out)
		}
	}
}

func TestPipelineStoreRoundTripLegacy(t *testing.T) {
	if _, err := exec.LookPath("zsh"); err != nil {
		t.Skip("zsh not available")
	}
	provider := Provider{}
	legacy := func(name, value string) model.Entry {
		return model.Entry{Text: name + "=" + value, Category: model.CatPath, Kind: model.KindAssignment, Names: []string{name}, Value: value, Managed: true, ValueMode: model.ValueModeLegacy}
	}
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
	cases := []struct {
		name        string
		list        string
		legacy      []string
		semantic    string
		legacyFirst bool
		base        string
		want        string
	}{
		{name: "repeated legacy PATH", list: "PATH", legacy: []string{"$PATH:/one", "$PATH:/two"}, base: "/base", want: "/base:/one:/two"},
		{name: "repeated legacy FPATH", list: "FPATH", legacy: []string{"$FPATH:/one", "$FPATH:/two"}, base: "/fbase", want: "/fbase:/one:/two"},
		{name: "legacy then semantic PATH", list: "PATH", legacy: []string{"$PATH:/legacy"}, semantic: "PATH=$PATH:$EXTRA\n", legacyFirst: true, base: "/base", want: "/base:/legacy:/one:/two"},
		{name: "semantic then legacy PATH", list: "PATH", legacy: []string{"$PATH:/legacy"}, semantic: "PATH=$PATH:$EXTRA\n", base: "/base", want: "/base:/one:/two:/legacy"},
		{name: "legacy then semantic FPATH", list: "FPATH", legacy: []string{"$FPATH:/legacy"}, semantic: "FPATH=$FPATH:$EXTRA\n", legacyFirst: true, base: "/fbase", want: "/fbase:/legacy:/one:/two"},
		{name: "semantic then legacy FPATH", list: "FPATH", legacy: []string{"$FPATH:/legacy"}, semantic: "FPATH=$FPATH:$EXTRA\n", base: "/fbase", want: "/fbase:/one:/two:/legacy"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			entries := make([]model.Entry, 0, len(tc.legacy)+1)
			appendLegacy := func() {
				for _, value := range tc.legacy {
					entries = append(entries, legacy(tc.list, value))
				}
			}
			if tc.legacyFirst {
				appendLegacy()
			}
			if tc.semantic != "" {
				entries = append(entries, semantic(t, tc.semantic))
			}
			if !tc.legacyFirst {
				appendLegacy()
			}
			persisted := roundTripPipelineProfile(t, model.Profile{Entries: entries})
			manifest := activate.Build(persisted)
			if len(manifest.Lists) != 1 || manifest.Lists[0].Name != tc.list {
				t.Fatalf("manifest lists=%#v", manifest.Lists)
			}
			if tc.semantic != "" && len(manifest.Lists[0].AdditionDynamic) == 0 {
				t.Fatalf("mixed profile lost dynamic provenance: %#v", manifest.Lists[0])
			}
			apply, deactivate := emitPipelineListPlan(t, provider, manifest)
			baseline := tc.list + "=" + zquote(tc.base)
			before := "before=$" + tc.list
			assertApply := "[[ $" + tc.list + " == " + zquote(tc.want) + " ]] || exit 111"
			assertRestore := "[[ $" + tc.list + " == $before ]] || exit 112"
			script := strings.Join([]string{baseline, "EXTRA=/one:/two", before, apply, "zp_apply", assertApply, deactivate, "zp_deactivate", assertRestore}, "\n")
			if out, err := exec.Command("zsh", "-f", "-c", script).CombinedOutput(); err != nil {
				t.Fatalf("live list pipeline: %v\n%s\nscript:\n%s", err, out, script)
			}
		})
	}
}

func TestPipelinePersistedOverrideManaged(t *testing.T) {
	if _, err := exec.LookPath("zsh"); err != nil {
		t.Skip("zsh not available")
	}
	provider := Provider{}
	sources := []string{
		"typeset -i COUNT=2",
		"readonly LOCKED=value",
		"typeset -T PATH path",
		"local scoped=value",
		"FOO+=bar",
		"export PATH+=:/x",
		"plugins=(git zsh-autosuggestions)",
		"alias -g G='| grep'",
	}
	blocks, err := provider.Parse([]byte(strings.Join(sources, "\n") + "\n"))
	if err != nil {
		t.Fatal(err)
	}
	profile := ir.Build(blocks, provider)
	if len(profile.Entries) != len(sources) {
		t.Fatalf("profile=%#v", profile)
	}
	for i := range profile.Entries {
		profile.Entries[i].Override = model.OverrideManaged
	}
	persisted := roundTripPipelineProfile(t, profile)
	regenerated := string(ir.Regenerate(persisted, provider))
	for _, source := range sources {
		if !strings.Contains(regenerated, source+"\n") {
			t.Fatalf("regeneration lost declaration %q: %q", source, regenerated)
		}
	}
	manifest := activate.Build(persisted)
	if len(manifest.Env) != 0 || len(manifest.Lists) != 0 || len(manifest.Aliases.Added) != 0 {
		t.Fatalf("persisted declarations produced activation operations: %#v", manifest)
	}
	apply, deactivate := emitPipelineListPlan(t, provider, manifest)
	script := strings.Join([]string{
		"PATH=/baseline", "FPATH=/fbaseline", "ZP_DECL_SENTINEL=before", "FOO=before", "plugins=(before)", "alias G='before'",
		"before_path=$PATH", "before_fpath=$FPATH", "before_sentinel=$ZP_DECL_SENTINEL", "before_foo=$FOO", "before_plugins=${plugins[*]}", "before_g=${aliases[G]}",
		apply, "zp_apply",
		"[[ $PATH == $before_path && $FPATH == $before_fpath && $ZP_DECL_SENTINEL == $before_sentinel && $FOO == $before_foo && ${plugins[*]} == $before_plugins && ${aliases[G]} == $before_g ]] || exit 121",
		deactivate, "zp_deactivate",
		"[[ $PATH == $before_path && $FPATH == $before_fpath && $ZP_DECL_SENTINEL == $before_sentinel && $FOO == $before_foo && ${plugins[*]} == $before_plugins && ${aliases[G]} == $before_g ]] || exit 122",
	}, "\n")
	if out, err := exec.Command("zsh", "-f", "-c", script).CombinedOutput(); err != nil {
		t.Fatalf("persisted declaration pipeline changed live state: %v\n%s\nscript:\n%s", err, out, script)
	}
}

func TestPipelineLegacyStructuralFidelityMatrix(t *testing.T) {
	if _, err := exec.LookPath("zsh"); err != nil {
		t.Skip("zsh not available")
	}
	provider := Provider{}
	rows := []struct {
		name string
		raw  string
		text string
	}{
		{name: "absent append", text: "FOO+=bar", raw: `{"entries":[{"text":"FOO+=bar","startLine":1,"category":"environment","kind":"assignment","cmdName":"","names":["FOO"],"value":"bar","exported":false,"managed":false,"override":"forced-managed","dynamic":false}]}`},
		{name: "partial array", text: "plugins=(git zsh-autosuggestions)", raw: `{"entries":[{"text":"plugins=(git zsh-autosuggestions)","startLine":1,"category":"environment","kind":"assignment","cmdName":"","names":["plugins"],"value":"","exported":false,"managed":false,"override":"forced-managed","dynamic":false,"structuralFidelity":{"version":1,"append":false,"flagged":false}}]}`},
		{name: "partial flagged alias", text: "alias -g G='| grep'", raw: `{"entries":[{"text":"alias -g G='| grep'","startLine":1,"category":"aliases","kind":"alias","cmdName":"alias","names":["G"],"value":"| grep","exported":false,"managed":false,"override":"forced-managed","dynamic":false,"structuralFidelity":{"version":1,"append":false,"array":false}}]}`},
		{name: "unsupported version", text: "FOO+=bar", raw: `{"entries":[{"text":"FOO+=bar","startLine":1,"category":"environment","kind":"assignment","cmdName":"","names":["FOO"],"value":"bar","exported":false,"managed":false,"override":"forced-managed","dynamic":false,"structuralFidelity":{"version":99,"append":true,"array":false,"flagged":false}}]}`},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			first, err := store.UnmarshalProfile([]byte(row.raw))
			if err != nil {
				t.Fatal(err)
			}
			saved, err := store.MarshalProfile(first)
			if err != nil {
				t.Fatal(err)
			}
			if bytes.Contains(saved, []byte(`"structuralFidelity"`)) {
				t.Fatalf("historical shape became current on re-save:\n%s", saved)
			}
			persisted, err := store.UnmarshalProfile(saved)
			if err != nil {
				t.Fatal(err)
			}
			if got := string(ir.Regenerate(persisted, provider)); got != row.text+"\n" {
				t.Fatalf("regenerated=%q, want verbatim %q", got, row.text+"\n")
			}
			manifest := activate.Build(persisted)
			if len(manifest.Env) != 0 || len(manifest.Lists) != 0 || len(manifest.Aliases.Added) != 0 || len(manifest.Functions.Added) != 0 || len(manifest.Options) != 0 {
				t.Fatalf("historical shape created manifest intent: %#v", manifest)
			}
			apply, deactivate := emitPipelineListPlan(t, provider, manifest)
			script := strings.Join([]string{
				"PATH=/baseline", "FPATH=/fbaseline", "FOO=before", "plugins=(before)", "alias G='before'",
				"before_path=$PATH", "before_fpath=$FPATH", "before_foo=$FOO", "before_plugins=${plugins[*]}", "before_g=${aliases[G]}",
				apply, "zp_apply",
				"[[ $PATH == $before_path && $FPATH == $before_fpath && $FOO == $before_foo && ${plugins[*]} == $before_plugins && ${aliases[G]} == $before_g ]] || exit 131",
				deactivate, "zp_deactivate",
				"[[ $PATH == $before_path && $FPATH == $before_fpath && $FOO == $before_foo && ${plugins[*]} == $before_plugins && ${aliases[G]} == $before_g ]] || exit 132",
			}, "\n")
			if out, err := exec.Command("zsh", "-f", "-c", script).CombinedOutput(); err != nil {
				t.Fatalf("historical fidelity pipeline changed sentinels: %v\n%s\nscript:\n%s", err, out, script)
			}
		})
	}
}

func roundTripPipelineProfile(t *testing.T, profile model.Profile) model.Profile {
	t.Helper()
	payload, err := store.MarshalProfile(profile)
	if err != nil {
		t.Fatal(err)
	}
	persisted, err := store.UnmarshalProfile(payload)
	if err != nil {
		t.Fatal(err)
	}
	return persisted
}

func emitPipelineListPlan(t *testing.T, provider Provider, manifest model.Manifest) (string, string) {
	t.Helper()
	applyPlan, err := activate.Diff(nil, &manifest)
	if err != nil {
		t.Fatal(err)
	}
	deactivatePlan, err := activate.Diff(&manifest, nil)
	if err != nil {
		t.Fatal(err)
	}
	apply, _, err := provider.Emit(applyPlan)
	if err != nil {
		t.Fatal(err)
	}
	_, deactivate, err := provider.Emit(deactivatePlan)
	if err != nil {
		t.Fatal(err)
	}
	for name, source := range map[string]string{"apply": apply, "deactivate": deactivate} {
		cmd := exec.Command("zsh", "-n")
		cmd.Stdin = strings.NewReader(source)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%s syntax: %v\n%s\n%s", name, err, out, source)
		}
	}
	return apply, deactivate
}

func TestPipelineRejectsUnsupportedListForms(t *testing.T) {
	if _, err := exec.LookPath("zsh"); err != nil {
		t.Skip("zsh not available")
	}

	provider := Provider{}
	for _, tc := range []struct {
		name string
		src  string
	}{
		{name: "PATH from FPATH", src: "PATH=$FPATH:/a\n"},
		{name: "FPATH from PATH", src: "FPATH=$PATH:/a\n"},
		{name: "unsupported parameter expression", src: "PATH=$PATH:${^EXTRA}\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			blocks, err := provider.Parse([]byte(tc.src))
			if err != nil {
				t.Fatal(err)
			}
			manifest := activate.Build(ir.Build(blocks, provider))
			if len(manifest.Lists) != 0 {
				t.Fatalf("rejected source produced list manifest: %#v", manifest.Lists)
			}

			applyPlan, err := activate.Diff(nil, &manifest)
			if err != nil {
				t.Fatal(err)
			}
			deactivatePlan, err := activate.Diff(&manifest, nil)
			if err != nil {
				t.Fatal(err)
			}
			apply, _, err := provider.Emit(applyPlan)
			if err != nil {
				t.Fatal(err)
			}
			_, deactivate, err := provider.Emit(deactivatePlan)
			if err != nil {
				t.Fatal(err)
			}
			for name, source := range map[string]string{"apply": apply, "deactivate": deactivate} {
				cmd := exec.Command("zsh", "-n")
				cmd.Stdin = strings.NewReader(source)
				if out, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("%s syntax: %v\n%s", name, err, out)
				}
			}

			script := strings.Join([]string{
				"PATH=/path-baseline", "FPATH=/fpath-baseline", "EXTRA=/extra",
				"before_path=$PATH", "before_fpath=$FPATH", apply, "zp_apply",
				"[[ $PATH == $before_path && $FPATH == $before_fpath ]] || exit 81",
				deactivate, "zp_deactivate",
				"[[ $PATH == $before_path && $FPATH == $before_fpath ]] || exit 82",
			}, "\n")
			if out, err := exec.Command("zsh", "-f", "-c", script).CombinedOutput(); err != nil {
				t.Fatalf("rejected-list pipeline changed live state: %v\n%s\nsource:\n%s\nscript:\n%s", err, out, tc.src, script)
			}
		})
	}
}

func TestPipelineMultiNameFunctionRoundTrip(t *testing.T) {
	if _, err := exec.LookPath("zsh"); err != nil {
		t.Skip("zsh not available")
	}

	provider := Provider{}
	blocks, err := provider.Parse([]byte("function one two { print -r -- profile-body }\n"))
	if err != nil {
		t.Fatal(err)
	}
	manifest := activate.Build(ir.Build(blocks, provider))
	if len(manifest.Functions.Added) != 2 || manifest.Functions.Added[0] != "one" || manifest.Functions.Added[1] != "two" {
		t.Fatalf("multi-name function manifest=%#v", manifest.Functions)
	}

	applyPlan, err := activate.Diff(nil, &manifest)
	if err != nil {
		t.Fatal(err)
	}
	deactivatePlan, err := activate.Diff(&manifest, nil)
	if err != nil {
		t.Fatal(err)
	}
	apply, _, err := provider.Emit(applyPlan)
	if err != nil {
		t.Fatal(err)
	}
	_, deactivate, err := provider.Emit(deactivatePlan)
	if err != nil {
		t.Fatal(err)
	}
	for name, source := range map[string]string{"apply": apply, "deactivate": deactivate} {
		cmd := exec.Command("zsh", "-n")
		cmd.Stdin = strings.NewReader(source)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%s syntax: %v\n%s", name, err, out)
		}
	}

	script := strings.Join([]string{
		"one() { print -r -- old-one; }", "two() { print -r -- old-two; }",
		"before_one=${functions[one]}", "before_two=${functions[two]}", apply, "zp_apply",
		"[[ $(one) == profile-body && $(two) == profile-body ]] || exit 91",
		deactivate, "zp_deactivate",
		"[[ $(one) == old-one && $(two) == old-two ]] || exit 92",
		"[[ ${functions[one]} == $before_one && ${functions[two]} == $before_two ]] || exit 93",
	}, "\n")
	if out, err := exec.Command("zsh", "-f", "-c", script).CombinedOutput(); err != nil {
		t.Fatalf("multi-name function pipeline: %v\n%s\nscript:\n%s", err, out, script)
	}
}
