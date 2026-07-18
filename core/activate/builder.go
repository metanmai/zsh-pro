package activate

import (
	"regexp"
	"strings"
	"zsh-pro/core/model"
)

var optionNameRE = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
var symbolNameRE = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]*$`)

// Build converts only effectively managed profile entries into declarative intent.
func Build(p model.Profile) model.Manifest {
	m := model.Manifest{Schema: model.SchemaV1, Aliases: model.AliasSet{Added: map[string]string{}, Shadowed: map[string]string{}}, Functions: model.FuncSet{Added: []string{}, Shadowed: map[string]string{}}}
	for _, e := range p.Entries {
		if !e.EffectiveManaged() {
			continue
		}
		switch e.Kind {
		case model.KindAssignment:
			if len(e.Names) != 1 {
				continue
			}
			if e.Category == model.CatEnvironment || e.Category == model.CatSecrets {
				m.Env = append(m.Env, model.Scalar{Name: e.Names[0], Applied: e.Value})
				continue
			}
			if e.Category == model.CatPath {
				if d, ok := pathDelta(e.Names[0], e.Value); ok {
					m.Lists = append(m.Lists, d)
				}
			}
		case model.KindAlias:
			if len(e.Names) == 1 && symbolNameRE.MatchString(e.Names[0]) {
				m.Aliases.Added[e.Names[0]] = e.Value
			}
		case model.KindFuncDecl:
			if len(e.Names) == 1 && symbolNameRE.MatchString(e.Names[0]) {
				m.Functions.Added = append(m.Functions.Added, e.Names[0])
			}
		case model.KindCommand:
			if e.Category != model.CatOptions || (e.CmdName != "setopt" && e.CmdName != "unsetopt") {
				continue
			}
			for _, n := range e.Names {
				if optionNameRE.MatchString(n) {
					m.Options = append(m.Options, model.OptionSet{Name: n, Enabled: e.CmdName == "setopt"})
				}
			}
		}
	}
	return m
}

func pathDelta(name, value string) (model.ListDelta, bool) {
	if name != "PATH" && name != "path" && name != "FPATH" && name != "fpath" {
		return model.ListDelta{}, false
	}
	base := map[string]bool{"$PATH": true, "$path": true, "${PATH}": true, "$FPATH": true, "$fpath": true, "${FPATH}": true}
	parts := strings.Split(value, ":")
	idx := -1
	for i, p := range parts {
		if base[p] {
			if idx != -1 {
				return model.ListDelta{}, false
			}
			idx = i
		}
	}
	if idx != 0 && idx != len(parts)-1 {
		return model.ListDelta{}, false
	}
	if idx == -1 {
		return model.ListDelta{}, false
	}
	adds := parts
	if idx == 0 {
		adds = parts[1:]
	} else {
		adds = parts[:len(parts)-1]
	}
	for _, a := range adds {
		if unsafeStatic(a) {
			return model.ListDelta{}, false
		}
	}
	canon := name
	if canon == "path" {
		canon = "PATH"
	}
	if canon == "fpath" {
		canon = "FPATH"
	}
	return model.ListDelta{Name: canon, Additions: adds, Deletions: []string{}}, true
}

func unsafeStatic(s string) bool {
	if strings.Contains(s, "$(") {
		return true
	}
	if strings.ContainsAny(s, ";&|`()< >\n") {
		return true
	}
	return false
}
