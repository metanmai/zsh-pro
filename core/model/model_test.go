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
		desc := CategoryDescription(cat)
		if desc == "" {
			t.Errorf("CategoryDescription(%q) returned empty string", cat)
		}
	}

	// Test CatMisc specifically returns expected description
	miscDesc := CategoryDescription(CatMisc)
	if miscDesc != "Uncategorized — review by hand" {
		t.Errorf("CategoryDescription(CatMisc) = %q, want %q", miscDesc, "Uncategorized — review by hand")
	}

	// Test unknown category returns appropriate message
	unknownDesc := CategoryDescription(Category("bogus"))
	if unknownDesc != "Unknown category" {
		t.Errorf("CategoryDescription(Category(\"bogus\")) = %q, want %q", unknownDesc, "Unknown category")
	}
}

func TestExitCode(t *testing.T) {
	clean := Analysis{}
	if clean.ExitCode() != 0 {
		t.Errorf("clean analysis: got %d want 0", clean.ExitCode())
	}
	dirty := Analysis{Issues: []Issue{{Kind: IssueDuplicateAlias, Name: "gs"}}}
	if dirty.ExitCode() != 3 {
		t.Errorf("analysis with issues: got %d want 3", dirty.ExitCode())
	}
}
