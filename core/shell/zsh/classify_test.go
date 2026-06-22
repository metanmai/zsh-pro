package zsh

import (
	"testing"

	"zsh-pro/core/model"
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
