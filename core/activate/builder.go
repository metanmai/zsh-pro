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
	envIndexes := map[string]int{}
	functionIndexes := map[string]int{}
	optionIndexes := map[string]int{}
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
				scalar := model.Scalar{Name: e.Names[0], Applied: value, Dynamic: boolPtr(dynamic), Exported: boolPtr(e.Exported)}
				if previous, ok := envIndexes[scalar.Name]; ok {
					// A plain assignment does not clear a shell's export attribute.
					// Preserve any managed export transition while replacing the final
					// value and moving the identity to its final source occurrence.
					*scalar.Exported = *m.Env[previous].Exported || e.Exported
					m.Env = append(m.Env[:previous], m.Env[previous+1:]...)
					for name, index := range envIndexes {
						if index > previous {
							envIndexes[name] = index - 1
						}
					}
				}
				envIndexes[scalar.Name] = len(m.Env)
				m.Env = append(m.Env, scalar)
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
				name := e.Names[0]
				if previous, ok := functionIndexes[name]; ok {
					m.Functions.Added = append(m.Functions.Added[:previous], m.Functions.Added[previous+1:]...)
					for function, index := range functionIndexes {
						if index > previous {
							functionIndexes[function] = index - 1
						}
					}
				}
				functionIndexes[name] = len(m.Functions.Added)
				m.Functions.Added = append(m.Functions.Added, name)
				m.Functions.Bodies[name] = *e.FunctionBody
			}
		case model.KindCommand:
			if e.Category != model.CatOptions || (e.CmdName != "setopt" && e.CmdName != unsetOptCommand) {
				continue
			}
			for _, n := range e.Names {
				if optionNameRE.MatchString(n) {
					if previous, ok := optionIndexes[n]; ok {
						m.Options = append(m.Options[:previous], m.Options[previous+1:]...)
						for option, index := range optionIndexes {
							if index > previous {
								optionIndexes[option] = index - 1
							}
						}
					}
					optionIndexes[n] = len(m.Options)
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
	var adds []string
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
