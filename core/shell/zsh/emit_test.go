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
		activate.SetScalar{Name: "GOPATH", Applied: "$HOME/go", Dynamic: true},
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
