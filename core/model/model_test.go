package model

import "testing"

func TestCategoriesOrderedAndComplete(t *testing.T) {
	got := Categories()
	want := []Category{
		CatEnvironment, CatPath, CatSecrets, CatPlugins, CatOptions,
		CatKeybindings, CatFunctions, CatAliases, CatLocal, CatMisc,
	}
	if len(got) != len(want) {
		t.Fatalf("got %d categories, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("position %d: got %q want %q", i, got[i], want[i])
		}
	}
}

func TestCategoryDescription(t *testing.T) {
	// Test all 10 real categories have non-empty descriptions
	categories := Categories()
	for _, cat := range categories {
		desc := cat.Description()
		if desc == "" {
			t.Errorf("%q.Description() returned empty string", cat)
		}
	}

	// Test CatMisc specifically returns expected description
	miscDesc := CatMisc.Description()
	if miscDesc != "Uncategorized — review by hand" {
		t.Errorf("CatMisc.Description() = %q, want %q", miscDesc, "Uncategorized — review by hand")
	}

	// Test unknown category returns appropriate message
	unknownDesc := Category("bogus").Description()
	if unknownDesc != "Unknown category" {
		t.Errorf("Category(\"bogus\").Description() = %q, want %q", unknownDesc, "Unknown category")
	}
}

func TestSeverityString(t *testing.T) {
	if got := SevActionable.String(); got != "actionable" {
		t.Errorf("SevActionable.String() = %q, want %q", got, "actionable")
	}
	if got := SevAdvisory.String(); got != "advisory" {
		t.Errorf("SevAdvisory.String() = %q, want %q", got, "advisory")
	}

	// The zero value of Severity must stringify to "actionable" — this proves
	// SevActionable is the zero value (D-01), so an Issue literal that omits
	// Severity stays actionable and the four existing kinds keep exit-3 behavior.
	var zero Severity
	if got := zero.String(); got != "actionable" {
		t.Errorf("zero-value Severity.String() = %q, want %q (SevActionable must be the zero value)", got, "actionable")
	}

	// An Issue literal that omits Severity must default to SevActionable.
	is := Issue{Kind: IssueDuplicateAlias}
	if is.Severity != SevActionable {
		t.Errorf("Issue{} omitted Severity = %v, want SevActionable (%v)", is.Severity, SevActionable)
	}
}

// TestSeverityIsActionable pins the single "actionable vs advisory" predicate
// (WR-01). The cut is "not advisory", so SevActionable and the zero value are
// actionable, SevAdvisory is not, and — critically — any future non-zero,
// non-advisory value defaults to the safe, exit-bumping (actionable) side. This
// keeps HasActionableIssues, the human marker/tally, and String() on one
// polarity so they cannot diverge when the enum is extended.
func TestSeverityIsActionable(t *testing.T) {
	cases := []struct {
		name string
		s    Severity
		want bool
	}{
		{"SevActionable", SevActionable, true},
		{"SevAdvisory", SevAdvisory, false},
		{"zero value", Severity(0), true},
		{"future non-advisory value defaults to actionable", Severity(2), true},
	}
	for _, tc := range cases {
		if got := tc.s.IsActionable(); got != tc.want {
			t.Errorf("%s: Severity(%d).IsActionable() = %v, want %v", tc.name, int(tc.s), got, tc.want)
		}
	}
}

func TestHasActionableIssues(t *testing.T) {
	cases := []struct {
		name string
		a    Analysis
		want bool
	}{
		{"empty", Analysis{}, false},
		{
			"advisory-only",
			Analysis{Issues: []Issue{{Kind: IssueDuplicatePath, Name: "/scratch", Severity: SevAdvisory}}},
			false,
		},
		{
			"zero-value-severity is actionable",
			Analysis{Issues: []Issue{{Kind: IssueDuplicateAlias, Name: "gs"}}},
			true,
		},
		{
			"mixed actionable + advisory",
			Analysis{Issues: []Issue{
				{Kind: IssueDuplicatePath, Name: "/scratch", Severity: SevAdvisory},
				{Kind: IssueDuplicateAlias, Name: "gs"},
			}},
			true,
		},
	}
	for _, tc := range cases {
		if got := tc.a.HasActionableIssues(); got != tc.want {
			t.Errorf("%s: HasActionableIssues() = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestExitCode(t *testing.T) {
	cases := []struct {
		name string
		a    Analysis
		want ExitCode
	}{
		{"clean", Analysis{}, ExitClean},
		{
			"actionable duplicate_alias (zero-value Severity)",
			Analysis{Issues: []Issue{{Kind: IssueDuplicateAlias, Name: "gs"}}},
			ExitActionable,
		},
		{
			"advisory-only is clean",
			Analysis{Issues: []Issue{{Kind: IssueDuplicatePath, Name: "/scratch", Severity: SevAdvisory}}},
			ExitClean,
		},
		{
			"mixed is actionable",
			Analysis{Issues: []Issue{
				{Kind: IssueDuplicatePath, Name: "/scratch", Severity: SevAdvisory},
				{Kind: IssueDuplicateAlias, Name: "gs"},
			}},
			ExitActionable,
		},
	}
	for _, tc := range cases {
		if got := tc.a.ExitCode(); got != tc.want {
			t.Errorf("%s: ExitCode() = %d, want %d", tc.name, got, tc.want)
		}
		// Invariant: ExitCode()==ExitActionable iff HasActionableIssues() (the two never disagree).
		if (tc.a.ExitCode() == ExitActionable) != tc.a.HasActionableIssues() {
			t.Errorf("%s: ExitCode()/HasActionableIssues() disagree", tc.name)
		}
	}
}
