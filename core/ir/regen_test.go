package ir

import (
	"strings"
	"testing"

	"zsh-pro/core/model"
)

// stubRegenerator is a minimal in-test shell.Regenerator so core/ir stays free
// of core/shell/zsh. It returns a recognizable marker so the test can assert a
// managed entry went through the Regenerator (not emitted verbatim).
type stubRegenerator struct{}

func (stubRegenerator) Regenerate(e model.Entry) string {
	return "TEMPLATED<" + strings.Join(e.Names, ",") + ">"
}

// TestRegenerateSourceOrderAndRouting pins ir.Regenerate: managed entries go
// through the Regenerator, everything else is emitted verbatim, and output
// preserves source order (D-09 / D-10 / D-01).
func TestRegenerateSourceOrderAndRouting(t *testing.T) {
	profile := model.Profile{Entries: []model.Entry{
		// 1: imperative — emitted verbatim
		{Text: "eval \"$(starship init zsh)\"", Kind: model.KindCommand, CmdName: "eval", Managed: false},
		// 2: managed declarative — templated
		{Text: "alias gs='git status'", Kind: model.KindAlias, Names: []string{"gs"}, Value: "'git status'", Managed: true, StructuralFidelityKnown: true},
		// 3: Opaque/other — emitted verbatim
		{Text: "if [[ -f ~/.x ]]; then source ~/.x; fi", Kind: model.KindOther, Managed: false},
		// 4: managed but forced-unmanaged (override wins, D-07) — verbatim
		{Text: "export EDITOR=nvim", Kind: model.KindAssignment, Names: []string{"EDITOR"}, Value: "nvim", Exported: true, Managed: true, Override: model.OverrideUnmanaged},
	}}

	out := string(Regenerate(profile, stubRegenerator{}))
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")

	if len(lines) != 4 {
		t.Fatalf("expected 4 output lines, got %d: %q", len(lines), out)
	}

	// (1) imperative entry emitted verbatim
	if lines[0] != "eval \"$(starship init zsh)\"" {
		t.Errorf("line 1: imperative not verbatim: %q", lines[0])
	}
	// (2) managed declarative goes through the Regenerator (marker present, raw Text absent)
	if lines[1] != "TEMPLATED<gs>" {
		t.Errorf("line 2: managed entry not templated: %q", lines[1])
	}
	if strings.Contains(out, "alias gs='git status'") {
		t.Errorf("managed entry emitted its raw Text instead of being templated: %q", out)
	}
	// (3) opaque/other emitted verbatim
	if lines[2] != "if [[ -f ~/.x ]]; then source ~/.x; fi" {
		t.Errorf("line 3: opaque not verbatim: %q", lines[2])
	}
	// (4) override-unmanaged wins → verbatim, NOT templated (D-07)
	if lines[3] != "export EDITOR=nvim" {
		t.Errorf("line 4: forced-unmanaged not verbatim: %q", lines[3])
	}
	if strings.Contains(out, "TEMPLATED<EDITOR>") {
		t.Errorf("forced-unmanaged entry was templated despite Override=OverrideUnmanaged: %q", out)
	}
}

// TestRegenerateDynamicValueDelegated proves ir.Regenerate performs no
// resolution: a dynamic managed entry is handed to the Regenerator unchanged
// (the Regenerator owns verbatim Value emission — D-05). Here the stub echoes
// the Value to confirm core/ir never touches it.
func TestRegenerateDynamicValueDelegated(t *testing.T) {
	echo := echoValueRegenerator{}
	profile := model.Profile{Entries: []model.Entry{
		{Text: "export GOPATH=$HOME/go", Kind: model.KindAssignment, Names: []string{"GOPATH"}, Value: "$HOME/go", Exported: true, Managed: true, Dynamic: true, StructuralFidelityKnown: true},
	}}
	out := string(Regenerate(profile, echo))
	if !strings.Contains(out, "$HOME/go") {
		t.Fatalf("dynamic value resolved or lost in regeneration: %q", out)
	}
}

func TestRegenerateDeclarationOverridesStayVerbatim(t *testing.T) {
	entries := []model.Entry{
		{Text: "typeset -i COUNT=2", Kind: model.KindAssignment, CmdName: "typeset", Names: []string{"COUNT"}, Value: "2", Override: model.OverrideManaged},
		{Text: "readonly LOCKED=value", Kind: model.KindAssignment, CmdName: "readonly", Names: []string{"LOCKED"}, Value: "value", Override: model.OverrideManaged},
		{Text: "typeset -T PATH path", Kind: model.KindAssignment, CmdName: "typeset", Names: []string{"PATH"}, Override: model.OverrideManaged},
		{Text: "local scoped=value", Kind: model.KindAssignment, CmdName: "local", Names: []string{"scoped"}, Value: "value", Override: model.OverrideManaged},
	}
	for _, entry := range entries {
		t.Run(entry.CmdName+"/"+entry.Names[0], func(t *testing.T) {
			got := string(Regenerate(model.Profile{Entries: []model.Entry{entry}}, stubRegenerator{}))
			if got != entry.Text+"\n" {
				t.Fatalf("Regenerate() = %q, want original declaration %q", got, entry.Text+"\n")
			}
		})
	}
}

func TestRegenerateForcedManagedStructuralShapesStayVerbatim(t *testing.T) {
	entries := []model.Entry{
		{Text: "FOO+=bar", Kind: model.KindAssignment, Names: []string{"FOO"}, Value: "bar", Override: model.OverrideManaged, StructuralFidelityKnown: true, Append: true},
		{Text: "plugins=(git zsh-autosuggestions)", Kind: model.KindAssignment, Names: []string{"plugins"}, Override: model.OverrideManaged, StructuralFidelityKnown: true, Array: true},
		{Text: "alias -g G='| grep'", Kind: model.KindAlias, CmdName: "alias", Names: []string{"G"}, Override: model.OverrideManaged, StructuralFidelityKnown: true, Flagged: true},
		{Text: "FOO=legacy", Kind: model.KindAssignment, Names: []string{"FOO"}, Value: "legacy", Override: model.OverrideManaged},
	}
	for _, entry := range entries {
		t.Run(entry.Text, func(t *testing.T) {
			got := string(Regenerate(model.Profile{Entries: []model.Entry{entry}}, stubRegenerator{}))
			if got != entry.Text+"\n" {
				t.Fatalf("Regenerate() = %q, want verbatim %q", got, entry.Text+"\n")
			}
		})
	}
}

type echoValueRegenerator struct{}

func (echoValueRegenerator) Regenerate(e model.Entry) string { return e.Value }
