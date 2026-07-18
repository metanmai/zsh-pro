package activate

import (
	"regexp"
	"strings"
	"zsh-pro/core/model"
)

var optionNameRE = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
var symbolNameRE = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]*$`)

const unsetOptCommand = "unset" + "opt"

// Build converts only effectively managed profile entries into declarative intent.
func Build(p model.Profile) model.Manifest {
	m := model.Manifest{
		Schema: model.SchemaV1,
		Aliases: model.AliasSet{
			Added:    map[string]string{},
			Shadowed: map[string]string{},
			Dynamic:  map[string]bool{},
		},
		Functions: model.FuncSet{
			Added:    []string{},
			Shadowed: map[string]string{},
			Bodies:   map[string]string{},
		},
	}
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
				value, dynamic, ok := activationValue(e)
				if !ok {
					continue
				}
				m.Env = append(m.Env, model.Scalar{Name: e.Names[0], Applied: value, Dynamic: boolPtr(dynamic)})
				continue
			}
			if e.Category == model.CatPath {
				if d, ok := pathDelta(e.Names[0], e.Value); ok {
					m.Lists = append(m.Lists, d)
				}
			}
		case model.KindAlias:
			if len(e.Names) == 1 && symbolNameRE.MatchString(e.Names[0]) {
				value, dynamic, ok := activationValue(e)
				if !ok {
					continue
				}
				m.Aliases.Added[e.Names[0]] = value
				m.Aliases.Dynamic[e.Names[0]] = dynamic
			}
		case model.KindFuncDecl:
			if len(e.Names) == 1 && symbolNameRE.MatchString(e.Names[0]) && e.FunctionBody != nil {
				m.Functions.Added = append(m.Functions.Added, e.Names[0])
				m.Functions.Bodies[e.Names[0]] = *e.FunctionBody
			}
		case model.KindCommand:
			if e.Category != model.CatOptions || (e.CmdName != "setopt" && e.CmdName != unsetOptCommand) {
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

// activationValue selects activation data solely from the semantic contract.
// Only legacy entries may use the historical Value/Dynamic fallback.
func activationValue(e model.Entry) (value string, dynamic, ok bool) {
	switch e.ValueMode {
	case model.ValueModeLegacy:
		return e.Value, e.Dynamic, true
	case model.ValueModeLiteral:
		if e.RuntimeValue == nil {
			return "", false, false
		}
		return *e.RuntimeValue, false, true
	case model.ValueModeDynamic:
		if !e.Dynamic {
			return "", false, false
		}
		return e.Value, true, true
	case model.ValueModeUnsupported:
		return "", false, false
	default:
		return "", false, false
	}
}

func boolPtr(v bool) *bool { return &v }

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
