package analyze

import (
	"testing"

	"zsh-pro/core/model"
)

// mockProvider returns pre-baked blocks and identities so the reconciler can
// be tested deterministically without invoking zsh. Its existence here is also
// load-bearing: analyze must depend ONLY on the shell interfaces + model, so a
// hand-rolled provider is enough to exercise the full reconciler.
type mockProvider struct {
	blocks []model.Block
	ids    model.IdentitySet
}

func (m mockProvider) Parse(_ []byte) ([]model.Block, error) { return m.blocks, nil }
func (m mockProvider) Classify(b model.Block) (model.Category, model.Confidence) {
	return b.Category, b.Conf
}
func (m mockProvider) Introspect(_ string) (model.IdentitySet, error) { return m.ids, nil }
func (m mockProvider) Categories() []model.Category                   { return model.Categories() }

func TestAnalyzeDetectsDuplicateAlias(t *testing.T) {
	p := mockProvider{
		blocks: []model.Block{
			{Kind: model.KindAlias, Names: []string{"gs"}, StartLine: 1, Category: model.CatAliases, Conf: model.ConfHigh},
			{Kind: model.KindAlias, Names: []string{"gs"}, StartLine: 9, Category: model.CatAliases, Conf: model.ConfHigh},
		},
		ids: model.IdentitySet{Available: true, Aliases: map[string]bool{"gs": true}},
	}
	a := New(p).Analyze([]byte("x\ny\n"), "/tmp/rc")
	if a.ExitCode() != 3 {
		t.Fatalf("expected actionable exit code 3, got %d", a.ExitCode())
	}
	if len(a.Issues) != 1 || a.Issues[0].Kind != model.IssueDuplicateAlias || a.Issues[0].Name != "gs" {
		t.Fatalf("expected one duplicate_alias issue for gs, got %+v", a.Issues)
	}
	if want := []int{1, 9}; len(a.Issues[0].Lines) != 2 || a.Issues[0].Lines[0] != want[0] || a.Issues[0].Lines[1] != want[1] {
		t.Errorf("lines = %v, want %v", a.Issues[0].Lines, want)
	}
	if !a.Introspected {
		t.Error("expected Introspected=true")
	}
}

func TestAnalyzeDegradesWhenIntrospectionUnavailable(t *testing.T) {
	p := mockProvider{
		blocks: []model.Block{{Kind: model.KindAssignment, Names: []string{"EDITOR"}, Category: model.CatEnvironment, StartLine: 1}},
		ids:    model.IdentitySet{Available: false},
	}
	a := New(p).Analyze([]byte("export EDITOR=nvim\n"), "/tmp/rc")
	if a.Introspected {
		t.Error("expected Introspected=false")
	}
	if len(a.Notes) == 0 {
		t.Error("expected a note explaining static-only degradation")
	}
}

// TestAnalyzeIgnoresInheritedShadowIdentities is the integration-fix guard.
//
// Task 5's introspection returns the RESOLVED end-state, which includes
// `zsh -f`'s own default aliases/functions (e.g. run-help) and inherited
// exported env vars — NOT just what the file defines. A naive shadow detector
// that iterates the raw IdentitySet would flag these inherited names as
// "defined as both an alias and a function" even though the user's file never
// touched them. Shadow detection MUST be scoped to file-defined names.
//
// Here the file defines only `gs` (an alias). The IdentitySet additionally
// reports an inherited name `run-help` resolving as BOTH alias and function.
// We assert NO shadow issue is produced, because the file never defined it.
func TestAnalyzeIgnoresInheritedShadowIdentities(t *testing.T) {
	p := mockProvider{
		blocks: []model.Block{
			{Kind: model.KindAlias, Names: []string{"gs"}, StartLine: 1, Category: model.CatAliases, Conf: model.ConfHigh},
		},
		ids: model.IdentitySet{
			Available: true,
			Aliases:   map[string]bool{"gs": true, "run-help": true},
			Functions: map[string]bool{"run-help": true, "compinit": true},
		},
	}
	a := New(p).Analyze([]byte("alias gs='git status'\n"), "/tmp/rc")
	for _, iss := range a.Issues {
		if iss.Kind == model.IssueShadowed {
			t.Fatalf("false-positive shadow issue for inherited identity %q: %+v", iss.Name, a.Issues)
		}
	}
	if !a.Introspected {
		t.Error("expected Introspected=true")
	}
}

// TestAnalyzeDetectsFileDefinedShadow is the positive counterpart: when the
// FILE itself defines a name both as an alias and as a function, that is a real
// cross-type shadow and MUST be reported.
func TestAnalyzeDetectsFileDefinedShadow(t *testing.T) {
	p := mockProvider{
		blocks: []model.Block{
			{Kind: model.KindAlias, Names: []string{"ll"}, StartLine: 2, Category: model.CatAliases, Conf: model.ConfHigh},
			{Kind: model.KindFuncDecl, Names: []string{"ll"}, StartLine: 8, Category: model.CatFunctions, Conf: model.ConfHigh},
		},
		ids: model.IdentitySet{
			Available: true,
			// End-state: the function wins, but both definitions are in the file.
			// Inherited noise is present too and must not produce its own issue.
			Aliases:   map[string]bool{"run-help": true},
			Functions: map[string]bool{"ll": true, "run-help": true},
		},
	}
	a := New(p).Analyze([]byte("# ll\nalias ll='ls -la'\n\n\n\n\n\nll() { ls -la }\n"), "/tmp/rc")

	var shadow []model.Issue
	for _, iss := range a.Issues {
		if iss.Kind == model.IssueShadowed {
			shadow = append(shadow, iss)
		}
	}
	if len(shadow) != 1 || shadow[0].Name != "ll" {
		t.Fatalf("expected exactly one shadow issue for file-defined ll, got %+v", a.Issues)
	}
}

// TestAnalyzeBucketsCategoriesAndFlagsSecrets exercises the rest of the
// reconciler surface: per-category rollups in taxonomy order, the secrets flag,
// opaque counting, and the top-level Analysis metadata.
func TestAnalyzeBucketsCategoriesAndFlagsSecrets(t *testing.T) {
	src := []byte("export EDITOR=nvim\nexport AWS_SECRET_ACCESS_KEY=xxx\nalias gs='git status'\n# weird\n{ : }\n")
	p := mockProvider{
		blocks: []model.Block{
			{Kind: model.KindAssignment, Names: []string{"EDITOR"}, StartLine: 1, Category: model.CatEnvironment, Conf: model.ConfHigh},
			{Kind: model.KindAssignment, Names: []string{"AWS_SECRET_ACCESS_KEY"}, StartLine: 2, Category: model.CatSecrets, Conf: model.ConfHigh},
			{Kind: model.KindAlias, Names: []string{"gs"}, StartLine: 3, Category: model.CatAliases, Conf: model.ConfHigh},
			{Kind: model.KindCompound, StartLine: 5, Text: "{ : }", Category: model.CatMisc, Conf: model.ConfLow, Opaque: true},
		},
		ids: model.IdentitySet{Available: true},
	}
	a := New(p).Analyze(src, "/home/u/.zshrc")

	if a.Path != "/home/u/.zshrc" {
		t.Errorf("Path = %q", a.Path)
	}
	if a.BlockCount != 4 {
		t.Errorf("BlockCount = %d, want 4", a.BlockCount)
	}
	if a.OpaqueBlocks != 1 {
		t.Errorf("OpaqueBlocks = %d, want 1", a.OpaqueBlocks)
	}
	if a.Lines != 6 { // 5 newlines + 1
		t.Errorf("Lines = %d, want 6", a.Lines)
	}
	if !a.HasSecrets {
		t.Error("expected HasSecrets=true")
	}
	// Categories must appear in taxonomy (load) order: environment, secrets,
	// aliases, misc — and only non-empty buckets.
	wantOrder := []model.Category{model.CatEnvironment, model.CatSecrets, model.CatAliases, model.CatMisc}
	if len(a.Categories) != len(wantOrder) {
		t.Fatalf("got %d category summaries, want %d: %+v", len(a.Categories), len(wantOrder), a.Categories)
	}
	for i, c := range a.Categories {
		if c.Category != wantOrder[i] {
			t.Errorf("category[%d] = %q, want %q", i, c.Category, wantOrder[i])
		}
	}
	if len(a.Issues) != 0 {
		t.Errorf("expected no issues, got %+v", a.Issues)
	}
}

// TestAnalyzeDetectsReassignedEnvAndDuplicatePath covers the remaining two
// static issue kinds together, including that secrets-bucket assignments are
// folded into env reassignment detection.
func TestAnalyzeDetectsReassignedEnvAndDuplicatePath(t *testing.T) {
	p := mockProvider{
		blocks: []model.Block{
			{Kind: model.KindAssignment, Names: []string{"EDITOR"}, StartLine: 1, Category: model.CatEnvironment, Conf: model.ConfHigh},
			{Kind: model.KindAssignment, Names: []string{"EDITOR"}, StartLine: 7, Category: model.CatEnvironment, Conf: model.ConfHigh},
			{Kind: model.KindCommand, CmdName: "export", StartLine: 3, Text: `export PATH="$HOME/bin:$PATH"`, Category: model.CatPath, Conf: model.ConfHigh},
			{Kind: model.KindCommand, CmdName: "export", StartLine: 5, Text: `export PATH="$HOME/bin:$PATH"`, Category: model.CatPath, Conf: model.ConfHigh},
		},
		ids: model.IdentitySet{Available: true},
	}
	a := New(p).Analyze([]byte("a\nb\nc\nd\ne\nf\ng\n"), "/tmp/rc")

	var gotEnv, gotPath int
	for _, iss := range a.Issues {
		switch iss.Kind {
		case model.IssueReassignedEnv:
			gotEnv++
			if iss.Name != "EDITOR" || len(iss.Lines) != 2 {
				t.Errorf("reassigned_env issue = %+v", iss)
			}
		case model.IssueDuplicatePath:
			gotPath++
			if iss.Name != "$HOME/bin" {
				t.Errorf("duplicate_path name = %q, want $HOME/bin", iss.Name)
			}
		}
	}
	if gotEnv != 1 {
		t.Errorf("expected 1 reassigned_env issue, got %d (%+v)", gotEnv, a.Issues)
	}
	if gotPath != 1 {
		t.Errorf("expected 1 duplicate_path issue, got %d (%+v)", gotPath, a.Issues)
	}
}

// TestAnalyzeDegradesOnIntrospectError ensures an Introspect that returns an
// error (not just Available:false) also degrades gracefully without crashing.
func TestAnalyzeDegradesOnIntrospectError(t *testing.T) {
	p := errProvider{mockProvider{
		blocks: []model.Block{{Kind: model.KindAlias, Names: []string{"gs"}, StartLine: 1, Category: model.CatAliases, Conf: model.ConfHigh}},
	}}
	a := New(p).Analyze([]byte("alias gs='git status'\n"), "/tmp/rc")
	if a.Introspected {
		t.Error("expected Introspected=false on introspect error")
	}
	if len(a.Notes) == 0 {
		t.Error("expected a degradation note on introspect error")
	}
}

// errProvider wraps mockProvider but fails introspection, simulating a missing
// or broken zsh.
type errProvider struct{ mockProvider }

func (errProvider) Introspect(_ string) (model.IdentitySet, error) {
	return model.IdentitySet{Available: false}, errIntrospect
}

var errIntrospect = errTest("introspection failed")

type errTest string

func (e errTest) Error() string { return string(e) }

// TestCountLines locks the editor-style line-count semantics (LINE-01, D-03):
// an empty file is 0 lines; a trailing newline is not double-counted; an
// unterminated final line still counts. The empty case also guards against an
// index-out-of-range panic on a zero-length slice.
func TestCountLines(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want int
	}{
		{"empty", "", 0},
		{"no_newline", "foo", 1},
		{"trailing_newline", "foo\n", 1},
		{"unterminated_final_line", "foo\nbar", 2},
		{"two_lines_trailing_newline", "foo\nbar\n", 2},
		{"only_newlines", "\n\n\n\n\n", 5},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := countLines([]byte(tc.src)); got != tc.want {
				t.Errorf("countLines(%q) = %d, want %d", tc.src, got, tc.want)
			}
		})
	}
}
