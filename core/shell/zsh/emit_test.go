package zsh

import (
	"os/exec"
	"strings"
	"testing"

	"zsh-pro/core/activate"
)

func TestEmitStaticDynamicAndSyntax(t *testing.T) {
	p := activate.Plan{Activate: []activate.Op{
		activate.SetScalar{Name: "EDITOR", Applied: "it's; echo bad", Dynamic: false},
		activate.SetScalar{Name: "GOPATH", Applied: "$HOME/go", Dynamic: true, Exported: true},
		activate.ApplyListDelta{Name: "PATH", Additions: []string{"/opt/tool*", "$HOME/bin"}},
		activate.AddAlias{Name: "gs", Body: "git status", Dynamic: false},
		activate.SetOption{Name: "extendedglob", Enabled: true},
	}, Deactivate: []activate.Op{
		activate.RestoreScalar{Name: "EDITOR", Applied: "it's; echo bad"},
		activate.RebuildListFromBase{Name: "PATH"},
		activate.Unalias{Name: "gs"}, activate.RestoreShadowedAlias{Name: "gs"},
		activate.RestoreOption{Name: "extendedglob"},
	}}
	a, d, err := (Provider{}).Emit(p)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(a, "'it'\\''s; echo bad'") {
		t.Errorf("static value was not quoted: %s", a)
	}
	if !strings.Contains(a, "export GOPATH=$HOME/go") {
		t.Errorf("dynamic value was quoted: %s", a)
	}
	if !strings.Contains(a, "'/opt/tool*'") || !strings.Contains(a, "$HOME/bin") {
		t.Errorf("list rendering lost static/dynamic split: %s", a)
	}
	for name, src := range map[string]string{"apply": a, "deactivate": d} {
		cmd := exec.Command("zsh", "-n")
		cmd.Stdin = strings.NewReader(src)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%s syntax: %v\n%s\n%s", name, err, out, src)
		}
	}
}

func TestEmitRejectsHostileNamesWithoutOutput(t *testing.T) {
	a, d, err := (Provider{}).Emit(activate.Plan{Activate: []activate.Op{
		activate.SetOption{Name: "foo;touch", Enabled: true},
		activate.AddAlias{Name: "x$(touch CANARY)", Body: "bad"},
		activate.AddFunc{Name: "a|b", Body: "bad"},
	}, Deactivate: []activate.Op{
		activate.RestoreOption{Name: "foo;touch"},
		activate.Unalias{Name: "x$(touch CANARY)"},
		activate.UnsetFunc{Name: "a|b"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(a, "foo;touch") || strings.Contains(a, "CANARY") || strings.Contains(d, "CANARY") {
		t.Fatalf("hostile names reached output: apply=%s deactivate=%s", a, d)
	}
}

func TestEmitAppliesAndRestoresShadowAndOption(t *testing.T) {
	base := activate.Plan{Activate: []activate.Op{
		activate.SetScalar{Name: "ZP_DEMO_VALUE", Applied: "changed"},
		activate.AddAlias{Name: "ll", Body: "new alias"},
		activate.AddFunc{Name: "ff", Body: "{ print new-func; }"},
		activate.SetOption{Name: "extendedglob", Enabled: false},
	}, Deactivate: []activate.Op{
		activate.RestoreScalar{Name: "ZP_DEMO_VALUE", Applied: "changed"},
		activate.Unalias{Name: "ll"}, activate.RestoreShadowedAlias{Name: "ll"},
		activate.UnsetFunc{Name: "ff"}, activate.RestoreShadowedFunc{Name: "ff"},
		activate.RestoreOption{Name: "extendedglob"},
	}}
	a, d, err := (Provider{}).Emit(base)
	if err != nil {
		t.Fatal(err)
	}
	script := "ZP_DEMO_VALUE=before\nalias ll='old alias'\nff() { print old-func; }\nsetopt extendedglob\n" + a + "\nzp_apply\n" + d + "\nzp_deactivate\nprintf '%s\\n' \"$ZP_DEMO_VALUE\" \"${aliases[ll]}\" \"${functions[ff]}\" \"$options[extendedglob]\"\n"
	out, err := exec.Command("zsh", "-f", "-c", script).CombinedOutput()
	if err != nil {
		t.Fatalf("zsh: %v\n%s\n%s", err, out, script)
	}
	if !strings.Contains(string(out), "before") || !strings.Contains(string(out), "old alias") || !strings.Contains(string(out), "old-func") || !strings.Contains(string(out), "on") {
		t.Fatalf("restore output=%q", out)
	}
}

func TestEmitDefinesEmptyAndMultilineFunctions(t *testing.T) {
	if _, err := exec.LookPath("zsh"); err != nil {
		t.Skip("zsh not available")
	}
	p := activate.Plan{Activate: []activate.Op{
		activate.AddFunc{Name: "zp04_empty", Body: ""},
		activate.AddFunc{Name: "zp04_multiline", Body: "print -r -- line-one\nprint -r -- line-two"},
	}}
	apply, _, err := (Provider{}).Emit(p)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(apply, `functions[zp04_empty]=""`) {
		t.Fatalf("empty function assignment missing:\n%s", apply)
	}

	check := exec.Command("zsh", "-n")
	check.Stdin = strings.NewReader(apply)
	if out, err := check.CombinedOutput(); err != nil {
		t.Fatalf("syntax: %v\n%s\n%s", err, out, apply)
	}

	script := apply + `
zp_apply
(( ${+functions[zp04_empty]} )) || exit 11
zp04_empty || exit 12
[[ "$(zp04_multiline)" == $'line-one\nline-two' ]] || exit 13
`
	if out, err := exec.Command("zsh", "-f", "-c", script).CombinedOutput(); err != nil {
		t.Fatalf("zsh: %v\n%s\n%s", err, out, script)
	}
}

func TestEncodeSlotIsInjectiveAndIdentifierSafe(t *testing.T) {
	names := []string{"foo-bar", "foo.bar", "foo_bar", "", "€"}
	seen := map[string]bool{}
	for _, name := range names {
		slot := encodeSlot("SCALAR", name)
		if !envNameRe.MatchString(slot) {
			t.Fatalf("slot %q for %q is not a valid identifier", slot, name)
		}
		if seen[slot] {
			t.Fatalf("slot collision for %q: %q", name, slot)
		}
		seen[slot] = true
	}
}

func TestEmitRestoresPresenceSentinelAndExportState(t *testing.T) {
	p := activate.Plan{Activate: []activate.Op{
		activate.SetScalar{Name: "ZP04_UNSET", Applied: "plain", Exported: false},
		activate.SetScalar{Name: "ZP04_EMPTY_PLAIN", Applied: "exported", Exported: true},
		activate.SetScalar{Name: "ZP04_SENTINEL", Applied: "changed", Exported: false},
		activate.AddAlias{Name: "zp04_alias", Body: "changed"},
		activate.AddFunc{Name: "zp04_func", Body: "print changed"},
	}, Deactivate: []activate.Op{
		activate.UnsetScalar{Name: "ZP04_UNSET", Applied: "plain"},
		activate.RestoreScalar{Name: "ZP04_EMPTY_PLAIN", Applied: "exported"},
		activate.RestoreScalar{Name: "ZP04_SENTINEL", Applied: "changed"},
		activate.Unalias{Name: "zp04_alias"}, activate.RestoreShadowedAlias{Name: "zp04_alias"},
		activate.UnsetFunc{Name: "zp04_func"}, activate.RestoreShadowedFunc{Name: "zp04_func"},
	}}
	apply, deactivate, err := (Provider{}).Emit(p)
	if err != nil {
		t.Fatal(err)
	}
	script := strings.Join([]string{
		"typeset ZP04_EMPTY_PLAIN=''",
		"typeset -gx ZP04_SENTINEL='__ZP_UNSET__'",
		"alias zp04_alias='__ZP_UNSET__'",
		"zp04_func() { print __ZP_UNSET__; }",
		apply,
		"zp_apply",
		deactivate,
		"zp_deactivate",
		"[[ ${+ZP04_UNSET} == 0 ]] || exit 11",
		"[[ ${+ZP04_EMPTY_PLAIN} == 1 && $ZP04_EMPTY_PLAIN == '' ]] || exit 12",
		"[[ ${parameters[ZP04_EMPTY_PLAIN]} != *export* ]] || exit 13",
		"[[ $ZP04_SENTINEL == __ZP_UNSET__ && ${parameters[ZP04_SENTINEL]} == *export* ]] || exit 14",
		"[[ ${aliases[zp04_alias]} == __ZP_UNSET__ ]] || exit 15",
		"[[ ${functions[zp04_func]} == *'__ZP_UNSET__'* ]] || exit 16",
	}, "\n")
	if out, err := exec.Command("zsh", "-f", "-c", script).CombinedOutput(); err != nil {
		t.Fatalf("zsh: %v\n%s\n%s", err, out, script)
	}
}

func TestEmitRepeatedScalarTracksFinalAppliedValue(t *testing.T) {
	p := activate.Plan{Activate: []activate.Op{
		activate.SetScalar{Name: "ZP04_REPEATED", Applied: "one"},
		activate.SetScalar{Name: "ZP04_REPEATED", Applied: "two"},
	}, Deactivate: []activate.Op{
		activate.RestoreScalar{Name: "ZP04_REPEATED", Applied: "two"},
	}}
	apply, deactivate, err := (Provider{}).Emit(p)
	if err != nil {
		t.Fatal(err)
	}
	script := "ZP04_REPEATED=before\n" + apply + "\nzp_apply\n" + deactivate + "\nzp_deactivate\n[[ $ZP04_REPEATED == before ]] || exit 21\n"
	if out, err := exec.Command("zsh", "-f", "-c", script).CombinedOutput(); err != nil {
		t.Fatalf("zsh: %v\n%s\n%s", err, out, script)
	}
}
