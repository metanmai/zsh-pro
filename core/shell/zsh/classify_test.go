package zsh

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"zsh-pro/core/model"
	"zsh-pro/core/shell"
)

// legacyProvider deliberately implements only the pre-Phase-7 composite. This
// compile assertion fails if an optional live capability is added to the broad
// shell.Provider interface.
type legacyProvider struct{}

func (legacyProvider) Parse([]byte) ([]model.Block, error) { return nil, nil }
func (legacyProvider) Classify(model.Block) (model.Category, model.Confidence) {
	return model.CatMisc, model.ConfLow
}
func (legacyProvider) Categories() []model.Category { return nil }
func (legacyProvider) Introspect(string) (model.IdentitySet, error) {
	return model.IdentitySet{}, nil
}
func (legacyProvider) Regenerate(model.Entry) string { return "" }
func (legacyProvider) HookScript() string            { return "" }

var (
	_ shell.Provider         = legacyProvider{}
	_ shell.LiveSecretPolicy = Provider{}
)

func TestClassify(t *testing.T) {
	cases := []struct {
		name string
		b    model.Block
		want model.Category
		conf model.Confidence
	}{
		{"alias", model.Block{Kind: model.KindAlias, CmdName: "alias", Names: []string{"gs"}}, model.CatAliases, model.ConfHigh},
		{"func", model.Block{Kind: model.KindFuncDecl, Names: []string{"greet"}}, model.CatFunctions, model.ConfHigh},
		{"env", model.Block{Kind: model.KindAssignment, Names: []string{"EDITOR"}, Text: "export EDITOR=nvim"}, model.CatEnvironment, model.ConfHigh},
		{"path", model.Block{Kind: model.KindAssignment, Names: []string{"PATH"}, Text: `PATH="$HOME/bin:$PATH"`}, model.CatPath, model.ConfMedium},
		{"secret", model.Block{Kind: model.KindAssignment, Names: []string{"GITHUB_TOKEN"}, Text: "export GITHUB_TOKEN=abc"}, model.CatSecrets, model.ConfHigh},
		{"setopt", model.Block{Kind: model.KindCommand, CmdName: "setopt", Text: "setopt AUTO_CD"}, model.CatOptions, model.ConfHigh},
		{"bindkey", model.Block{Kind: model.KindCommand, CmdName: "bindkey", Text: "bindkey '^R' history-incremental-search-backward"}, model.CatKeybindings, model.ConfHigh},
		{"eval-init", model.Block{Kind: model.KindCommand, CmdName: "eval", Text: `eval "$(starship init zsh)"`}, model.CatPlugins, model.ConfMedium},
		{"source", model.Block{Kind: model.KindCommand, CmdName: "source", Text: "source ~/.oh-my-zsh/oh-my-zsh.sh"}, model.CatPlugins, model.ConfMedium},
		{"os-conditional", model.Block{Kind: model.KindCompound, Text: `if [[ "$OSTYPE" == darwin* ]]; then alias ls='ls -G'; fi`}, model.CatLocal, model.ConfMedium},
		{"opaque", model.Block{Opaque: true, Text: "garbled"}, model.CatMisc, model.ConfLow},
		{"unknown-cmd", model.Block{Kind: model.KindCommand, CmdName: "fortune", Text: "fortune"}, model.CatMisc, model.ConfLow},
	}
	for _, c := range cases {
		gotCat, gotConf := (Provider{}).Classify(c.b)
		if gotCat != c.want || gotConf != c.conf {
			t.Errorf("%s: got (%q, %d), want (%q, %d)", c.name, gotCat, gotConf, c.want, c.conf)
		}
	}
}

func TestLiveSecretUsesClassifierPolicy(t *testing.T) {
	p := Provider{}
	tests := []struct {
		name     string
		identity model.Identity
		want     bool
	}{
		{name: "upper case secret environment", identity: model.Identity{Kind: model.LiveEnv, Name: "GITHUB_TOKEN"}, want: true},
		{name: "lower case preserves classifier case folding", identity: model.Identity{Kind: model.LiveEnv, Name: "github_token"}, want: true},
		{name: "mixed case compact api key", identity: model.Identity{Kind: model.LiveEnv, Name: "apiKey"}, want: true},
		{name: "classifier substring behavior stays exact", identity: model.Identity{Kind: model.LiveEnv, Name: "TOKENIZED"}, want: true},
		{name: "ordinary environment", identity: model.Identity{Kind: model.LiveEnv, Name: "EDITOR"}, want: false},
		{name: "secret-like alias follows alias category", identity: model.Identity{Kind: model.LiveAlias, Name: "GITHUB_TOKEN"}, want: false},
		{name: "secret-like function follows function category", identity: model.Identity{Kind: model.LiveFunction, Name: "API_KEY"}, want: false},
		{name: "path follows path category", identity: model.Identity{Kind: model.LivePath, Name: "PATH"}, want: false},
		{name: "fpath follows path category", identity: model.Identity{Kind: model.LiveFPath, Name: "FPATH"}, want: false},
		{name: "secret-like option follows option category", identity: model.Identity{Kind: model.LiveOption, Name: "AUTH_TOKEN"}, want: false},
		{name: "unknown kind rejected", identity: model.Identity{Kind: model.LiveKind("unknown"), Name: "GITHUB_TOKEN"}, want: false},
		{name: "malformed env rejected", identity: model.Identity{Kind: model.LiveEnv, Name: "BAD-NAME_TOKEN"}, want: false},
		{name: "malformed symbol rejected", identity: model.Identity{Kind: model.LiveAlias, Name: "bad name"}, want: false},
		{name: "wrong path identity rejected", identity: model.Identity{Kind: model.LivePath, Name: "OTHER_PATH"}, want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := p.IsLiveSecretIdentity(test.identity); got != test.want {
				t.Fatalf("IsLiveSecretIdentity(%#v) = %v, want %v", test.identity, got, test.want)
			}
		})
	}
}

func TestLiveInterfacesStayNarrow(t *testing.T) {
	var _ shell.WorktreeRegenerator
	var _ shell.LiveCaptureSource
	var _ shell.LiveSnapshotDecoder
	var _ shell.LivePatchEmitter
}

func TestLiveSecretImportBoundaries(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate classifier test")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
	for _, packageDir := range []string{"core/model", "core/worktree", "core/store", "core/cli"} {
		entries, err := os.ReadDir(filepath.Join(root, packageDir))
		if err != nil {
			t.Fatalf("read %s: %v", packageDir, err)
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
				continue
			}
			path := filepath.Join(root, packageDir, entry.Name())
			parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
			if err != nil {
				t.Fatalf("parse %s: %v", path, err)
			}
			for _, imported := range parsed.Imports {
				if imported.Path.Value == `"zsh-pro/core/shell/zsh"` {
					t.Fatalf("shell-agnostic production file imports concrete zsh: %s", path)
				}
			}
		}
	}
}
