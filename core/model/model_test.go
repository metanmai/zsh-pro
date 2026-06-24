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

func TestExitCode(t *testing.T) {
	clean := Analysis{}
	if clean.ExitCode() != ExitClean {
		t.Errorf("clean analysis: got %d want 0", clean.ExitCode())
	}
	dirty := Analysis{Issues: []Issue{{Kind: IssueDuplicateAlias, Name: "gs"}}}
	if dirty.ExitCode() != ExitActionable {
		t.Errorf("analysis with issues: got %d want 3", dirty.ExitCode())
	}
}
