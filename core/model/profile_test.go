package model

import "testing"

// TestEffectiveManaged pins D-07: a set Override wins over the auto Managed
// verdict (OverrideManaged -> true, OverrideUnmanaged -> false); OverrideAuto
// defers to the Managed bool.
func TestEffectiveManaged(t *testing.T) {
	cases := []struct {
		name     string
		managed  bool
		override ManagedOverride
		want     bool
	}{
		{"auto-managed", true, OverrideAuto, true},
		{"auto-unmanaged", false, OverrideAuto, false},
		{"forced-managed-over-imperative", false, OverrideManaged, true},
		{"forced-unmanaged-over-managed", true, OverrideUnmanaged, false},
		{"zero-value-override-is-auto", true, "", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := Entry{Managed: c.managed, Override: c.override}
			if got := e.EffectiveManaged(); got != c.want {
				t.Errorf("EffectiveManaged() = %v, want %v (managed=%v override=%q)",
					got, c.want, c.managed, c.override)
			}
		})
	}
}

func TestEntryDeclarationRepresentability(t *testing.T) {
	for _, tc := range []struct {
		name string
		cmd  string
	}{
		{name: "integer", cmd: "typeset"},
		{name: "readonly", cmd: "readonly"},
		{name: "tied list", cmd: "typeset"},
		{name: "local", cmd: "local"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := Entry{Kind: KindAssignment, CmdName: tc.cmd, Override: OverrideManaged}
			if !e.EffectiveManaged() {
				t.Fatal("forced-managed declaration lost persisted override semantics")
			}
			if e.DeclarationRepresentable() {
				t.Fatalf("%s declaration was treated as representable", tc.cmd)
			}
		})
	}

	for _, e := range []Entry{
		{Kind: KindAssignment, Names: []string{"EDITOR"}, Override: OverrideManaged},
		{Kind: KindAssignment, CmdName: "export", Names: []string{"EDITOR"}, Override: OverrideManaged, Exported: true},
	} {
		if !e.DeclarationRepresentable() {
			t.Fatalf("ordinary assignment became unrepresentable: %#v", e)
		}
	}
}

func TestEntryRepresentabilityRequiresKnownStructuralFidelity(t *testing.T) {
	cases := []struct {
		name string
		e    Entry
		want bool
	}{
		{name: "unknown", e: Entry{Kind: KindAssignment}, want: false},
		{name: "plain scalar", e: Entry{Kind: KindAssignment, StructuralFidelityKnown: true}, want: true},
		{name: "exported scalar", e: Entry{Kind: KindAssignment, Exported: true, StructuralFidelityKnown: true}, want: true},
		{name: "append", e: Entry{Kind: KindAssignment, StructuralFidelityKnown: true, Append: true}, want: false},
		{name: "array", e: Entry{Kind: KindAssignment, StructuralFidelityKnown: true, Array: true}, want: false},
		{name: "flagged alias", e: Entry{Kind: KindAlias, StructuralFidelityKnown: true, Flagged: true}, want: false},
		{name: "declaration", e: Entry{Kind: KindAssignment, CmdName: "typeset", StructuralFidelityKnown: true}, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.e.Representable(); got != tc.want {
				t.Fatalf("Representable() = %v, want %v for %#v", got, tc.want, tc.e)
			}
		})
	}
}
