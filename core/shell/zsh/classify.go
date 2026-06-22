package zsh

import (
	"regexp"
	"strings"

	"zsh-pro/core/model"
)

var secretRe = regexp.MustCompile(`(?i)(SECRET|TOKEN|PASSWD|PASSWORD|API[_-]?KEY|ACCESS[_-]?KEY|PRIVATE[_-]?KEY|CLIENT[_-]?SECRET|AUTH[_-]?TOKEN|APIKEY)`)

var pluginHints = []string{
	"oh-my-zsh", "ohmyzsh", "zinit", "antigen", "zplug", "zgen",
	"antibody", "sheldon", "zcomet", "starship", "zoxide", "pyenv", "nvm",
}

// Classify maps a parsed block to a category with a confidence. Low-confidence
// results land in CatMisc so the report can flag them for review rather than
// silently misfiling them.
func (Provider) Classify(b model.Block) (model.Category, model.Confidence) {
	if b.Opaque {
		return model.CatMisc, model.ConfLow
	}
	text := strings.ToLower(b.Text)

	switch b.Kind {
	case model.KindAlias:
		return model.CatAliases, model.ConfHigh
	case model.KindFuncDecl:
		return model.CatFunctions, model.ConfHigh
	case model.KindAssignment:
		for _, n := range b.Names {
			if secretRe.MatchString(n) {
				return model.CatSecrets, model.ConfHigh
			}
		}
		for _, n := range b.Names {
			u := strings.ToUpper(n)
			if u == "PATH" || u == "FPATH" || u == "MANPATH" || u == "CDPATH" || strings.Contains(u, "PATH") {
				return model.CatPath, model.ConfMedium
			}
		}
		return model.CatEnvironment, model.ConfHigh
	case model.KindCommand:
		switch b.CmdName {
		case "setopt", "unsetopt", "zstyle", "autoload", "compinit", "compdef", "zmodload":
			return model.CatOptions, model.ConfHigh
		case "bindkey":
			return model.CatKeybindings, model.ConfHigh
		case "source", ".":
			return model.CatPlugins, model.ConfMedium
		case "eval":
			if strings.Contains(text, "init") || strings.Contains(text, "hook") || strings.Contains(text, "shellenv") {
				return model.CatPlugins, model.ConfMedium
			}
			return model.CatMisc, model.ConfLow
		}
		for _, h := range pluginHints {
			if strings.Contains(text, h) {
				return model.CatPlugins, model.ConfMedium
			}
		}
		return model.CatMisc, model.ConfLow
	case model.KindCompound:
		if strings.Contains(text, "ostype") || strings.Contains(text, "uname") ||
			strings.Contains(text, "darwin") || strings.Contains(text, "linux") ||
			strings.Contains(text, "$host") || strings.Contains(text, "hostname") {
			return model.CatLocal, model.ConfMedium
		}
		return model.CatMisc, model.ConfLow
	default:
		for _, h := range pluginHints {
			if strings.Contains(text, h) {
				return model.CatPlugins, model.ConfLow
			}
		}
		return model.CatMisc, model.ConfLow
	}
}
