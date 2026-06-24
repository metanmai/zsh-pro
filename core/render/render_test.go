package render

import (
	"encoding/json"
	"strings"
	"testing"

	"zsh-pro/core/model"
)

func sampleAnalysis() model.Analysis {
	return model.Analysis{
		Path:       "/home/u/.zshrc",
		Lines:      42,
		BlockCount: 7,
		Categories: []model.CategorySummary{
			{Category: model.CatAliases, Count: 2, Items: []string{"gs", "ll"}},
		},
		Issues:       []model.Issue{{Kind: model.IssueDuplicateAlias, Name: "gs", Lines: []int{1, 9}, Note: "last definition wins"}},
		Introspected: true,
	}
}

// advisoryOnlyAnalysis builds an analysis whose only finding is an advisory.
// Under SEV-02 this must read as clean: exit 0, issues_found:false, while the
// issue itself still carries severity:"advisory" on the wire (D-04 / D-02).
func advisoryOnlyAnalysis() model.Analysis {
	return model.Analysis{
		Path:         "/home/u/.zshrc",
		Lines:        12,
		BlockCount:   3,
		Issues:       []model.Issue{{Kind: model.IssueDuplicatePath, Name: "./scripts", Lines: []int{4}, Severity: model.SevAdvisory}},
		Introspected: true,
	}
}

func TestHumanIncludesCategoriesAndIssues(t *testing.T) {
	b, _ := (HumanRenderer{}).Render(sampleAnalysis())
	out := string(b)
	for _, want := range []string{"aliases", "gs", "duplicate", "1", "9"} {
		if !strings.Contains(out, want) {
			t.Errorf("human output missing %q\n---\n%s", want, out)
		}
	}
}

// TestHumanAdvisoryMarkerAndTally pins D-05 + D-06: an actionable issue uses
// "!" and an advisory uses the softer "~" in the SINGLE ISSUES list (no second
// section heading), and the summary tallies advisories distinctly from the
// actionable issue count.
func TestHumanAdvisoryMarkerAndTally(t *testing.T) {
	a := model.Analysis{
		Path:       "/home/u/.zshrc",
		Lines:      20,
		BlockCount: 5,
		Issues: []model.Issue{
			{Kind: model.IssueDuplicateAlias, Name: "gs", Lines: []int{1, 9}}, // zero-value ⇒ actionable
			{Kind: model.IssueDuplicatePath, Name: "./bin", Lines: []int{4}, Severity: model.SevAdvisory},
		},
		Introspected: true,
	}
	b, _ := (HumanRenderer{}).Render(a)
	out := string(b)

	// The actionable issue's line carries "!" with its name.
	actLine := findLine(out, "gs")
	if !strings.Contains(actLine, "!") {
		t.Errorf("actionable issue line missing %q marker: %q", "!", actLine)
	}
	if strings.Contains(actLine, "~") {
		t.Errorf("actionable issue line must not carry the advisory marker: %q", actLine)
	}

	// The advisory issue's line carries the softer "~" and not "!".
	advLine := findLine(out, "./bin")
	if !strings.Contains(advLine, "~") {
		t.Errorf("advisory issue line missing softer %q marker: %q", "~", advLine)
	}
	if strings.Contains(advLine, "!") {
		t.Errorf("advisory issue line must not carry the actionable %q marker: %q", "!", advLine)
	}

	// Single ISSUES list — exactly one ISSUES header, no separate advisory section.
	if n := strings.Count(out, "ISSUES"); n != 1 {
		t.Errorf("expected exactly one ISSUES header, got %d\n---\n%s", n, out)
	}

	// Summary tallies advisories distinctly from actionable issues. There is one
	// of each here, so the rendered output must show an advisory count of 1 that
	// is presented separately from the actionable issue count (not folded in).
	if !strings.Contains(out, "advisor") {
		t.Errorf("summary missing a distinct advisory tally:\n%s", out)
	}
	tally := findLine(out, "advisor")
	if !strings.Contains(tally, "1") {
		t.Errorf("advisory tally line missing the advisory count: %q", tally)
	}
}

// findLine returns the first line in s that contains sub (or "" if none).
func findLine(s, sub string) string {
	for _, ln := range strings.Split(s, "\n") {
		if strings.Contains(ln, sub) {
			return ln
		}
	}
	return ""
}

func TestJSONIsOneObjectWithContract(t *testing.T) {
	b, err := (JSONRenderer{}).Render(sampleAnalysis())
	if err != nil {
		t.Fatal(err)
	}
	var env map[string]any
	if err := json.Unmarshal(b, &env); err != nil {
		t.Fatalf("output is not a single JSON object: %v", err)
	}
	if env["tool"] != "zsh-pro" {
		t.Errorf("tool = %v, want zsh-pro", env["tool"])
	}
	if env["ok"] != true {
		t.Errorf("ok = %v, want true", env["ok"])
	}
	if env["exit_code"].(float64) != 3 {
		t.Errorf("exit_code = %v, want 3", env["exit_code"])
	}
	if env["issues_found"] != true {
		t.Errorf("issues_found = %v, want true", env["issues_found"])
	}
}

// issuesFromJSON decodes a rendered envelope and returns its issues slice.
func issuesFromJSON(t *testing.T, a model.Analysis) []map[string]any {
	t.Helper()
	b, err := (JSONRenderer{}).Render(a)
	if err != nil {
		t.Fatal(err)
	}
	var env struct {
		Analysis struct {
			Issues []map[string]any `json:"issues"`
		} `json:"analysis"`
	}
	if err := json.Unmarshal(b, &env); err != nil {
		t.Fatalf("output is not a single JSON object: %v\n%s", err, b)
	}
	return env.Analysis.Issues
}

// TestJSONIssueHasSeverityOnEveryIssue pins D-02: the severity field is present
// and non-empty on every issue, an actionable issue reads "actionable", and a
// hand-set advisory reads "advisory".
func TestJSONIssueHasSeverityOnEveryIssue(t *testing.T) {
	// Actionable case — the existing sampleAnalysis() duplicate_alias.
	issues := issuesFromJSON(t, sampleAnalysis())
	if len(issues) != 1 {
		t.Fatalf("issues len = %d, want 1", len(issues))
	}
	sev, ok := issues[0]["severity"]
	if !ok {
		t.Fatalf("issues[0] has no severity key: %v", issues[0])
	}
	if s, _ := sev.(string); s != "actionable" {
		t.Errorf("issues[0].severity = %q, want %q", sev, "actionable")
	}

	// Advisory case — a hand-set advisory issue.
	advIssues := issuesFromJSON(t, advisoryOnlyAnalysis())
	if len(advIssues) != 1 {
		t.Fatalf("advisory issues len = %d, want 1", len(advIssues))
	}
	if s, _ := advIssues[0]["severity"].(string); s != "advisory" {
		t.Errorf("advisory issues[0].severity = %q, want %q", advIssues[0]["severity"], "advisory")
	}
}

// TestJSONAdvisoryOnlyEnvelope pins D-04: an advisory-only analysis renders
// issues_found:false AND exit_code:0 — the two agree — while the issue still
// carries severity:"advisory".
func TestJSONAdvisoryOnlyEnvelope(t *testing.T) {
	b, err := (JSONRenderer{}).Render(advisoryOnlyAnalysis())
	if err != nil {
		t.Fatal(err)
	}
	var env map[string]any
	if err := json.Unmarshal(b, &env); err != nil {
		t.Fatalf("output is not a single JSON object: %v", err)
	}
	if env["issues_found"] != false {
		t.Errorf("issues_found = %v, want false (advisory-only is clean)", env["issues_found"])
	}
	if env["exit_code"].(float64) != 0 {
		t.Errorf("exit_code = %v, want 0 (advisory must not bump the exit code)", env["exit_code"])
	}
	// The two fields must never disagree.
	if (env["issues_found"] == true) != (env["exit_code"].(float64) == 3) {
		t.Errorf("issues_found %v and exit_code %v disagree", env["issues_found"], env["exit_code"])
	}
	issues, _ := env["analysis"].(map[string]any)["issues"].([]any)
	if len(issues) != 1 {
		t.Fatalf("issues len = %d, want 1", len(issues))
	}
	if s, _ := issues[0].(map[string]any)["severity"].(string); s != "advisory" {
		t.Errorf("issue severity = %q, want %q", issues[0].(map[string]any)["severity"], "advisory")
	}
}
