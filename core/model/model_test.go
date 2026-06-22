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
