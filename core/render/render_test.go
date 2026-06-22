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

func TestHumanIncludesCategoriesAndIssues(t *testing.T) {
	out := Human(sampleAnalysis())
	for _, want := range []string{"aliases", "gs", "duplicate", "1", "9"} {
		if !strings.Contains(out, want) {
			t.Errorf("human output missing %q\n---\n%s", want, out)
		}
	}
}

func TestJSONIsOneObjectWithContract(t *testing.T) {
	b, err := JSON(sampleAnalysis())
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
