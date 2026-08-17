package activate

import (
	"regexp"
	"strings"
	"zsh-pro/core/model"
)

var optionNameRE = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
var symbolNameRE = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]*$`)
var legacyDynamicPathRE = regexp.MustCompile(`^\$(?:[A-Za-z_][A-Za-z0-9_]*|\{[A-Za-z_][A-Za-z0-9_]*\})(?:/[A-Za-z0-9_@%+=,.-]+)*$`)

const unsetOptCommand = "unset" + "opt"

// BuildEffective retains the complete source profile and constructs a separate
// versioned final-state projection. Source entries are never rewritten or
// filtered: tombstones are authoritative only inside Projection.
func BuildEffective(base model.Profile, overlay []model.OverlayEntry) (model.CommittedWorktree, error) {
	states, err := model.NormalizeLiveStates(liveStatesFromManifest(base, Build(base)))
	if err != nil {
		return model.CommittedWorktree{}, err
	}
	normalizedOverlay, err := model.NormalizeOverlay(overlay)
	if err != nil {
		return model.CommittedWorktree{}, err
	}
	projection := effectiveProjection(states, normalizedOverlay)
	return model.NewCommittedWorktree(base, projection), nil
}

func effectiveProjection(base []model.LiveIdentityState, overlay []model.OverlayEntry) model.LiveProjection {
	positions := make(map[model.Identity]int, len(base)+len(overlay))
	states := make([]model.LiveIdentityState, 0, len(base)+len(overlay))
	active := make([]bool, 0, len(base)+len(overlay))
	for _, state := range base {
		if !state.Value.Present {
			continue
		}
		positions[state.Identity] = len(states)
		states = append(states, model.LiveIdentityState{Identity: state.Identity, Value: model.CloneLiveValue(state.Value)})
		active = append(active, true)
	}
	tombstones := make([]model.Identity, 0, len(overlay))
	for _, entry := range overlay {
		position, exists := positions[entry.Identity]
		if entry.Tombstone {
			tombstones = append(tombstones, entry.Identity)
			if exists {
				active[position] = false
			}
			continue
		}
		state := model.LiveIdentityState{Identity: entry.Identity, Value: model.CloneLiveValue(entry.Value)}
		if exists {
			states[position] = state
			active[position] = true
			continue
		}
		positions[entry.Identity] = len(states)
		states = append(states, state)
		active = append(active, true)
	}
	projected := make([]model.LiveIdentityState, 0, len(states))
	for index, state := range states {
		if active[index] {
			projected = append(projected, state)
		}
	}
	return model.LiveProjection{
		Schema:     model.WorktreeSchemaV1,
		States:     projected,
		Tombstones: tombstones,
	}
}

func liveStatesFromManifest(source model.Profile, manifest model.Manifest) []model.LiveIdentityState {
	values := make(map[model.Identity]model.LiveValue)
	for _, scalar := range manifest.Env {
		values[model.Identity{Kind: model.LiveEnv, Name: scalar.Name}] = model.ScalarLiveValue(scalar.Applied)
	}
	for name, body := range manifest.Aliases.Added {
		values[model.Identity{Kind: model.LiveAlias, Name: name}] = model.ScalarLiveValue(body)
	}
	for _, name := range manifest.Functions.Added {
		body, ok := manifest.Functions.Bodies[name]
		if ok {
			values[model.Identity{Kind: model.LiveFunction, Name: name}] = model.ScalarLiveValue(body)
		}
	}
	for _, list := range manifest.Lists {
		kind := model.LivePath
		if list.Name == "FPATH" {
			kind = model.LiveFPath
		}
		values[model.Identity{Kind: kind, Name: list.Name}] = model.ListLiveValue(list.Additions)
	}
	for _, option := range manifest.Options {
		values[model.Identity{Kind: model.LiveOption, Name: option.Name}] = model.OptionLiveValue(option.Enabled)
	}

	order := make([]model.Identity, 0, len(values))
	indexes := make(map[model.Identity]int, len(values))
	for _, entry := range source.Entries {
		for _, identity := range liveEntryIdentities(entry) {
			if _, ok := values[identity]; !ok {
				continue
			}
			if previous, ok := indexes[identity]; ok {
				order = append(order[:previous], order[previous+1:]...)
				for existing, index := range indexes {
					if index > previous {
						indexes[existing] = index - 1
					}
				}
			}
			indexes[identity] = len(order)
			order = append(order, identity)
		}
	}

	states := make([]model.LiveIdentityState, 0, len(values))
	seen := make(map[model.Identity]bool, len(values))
	appendState := func(identity model.Identity) {
		value, ok := values[identity]
		if !ok || seen[identity] {
			return
		}
		states = append(states, model.LiveIdentityState{Identity: identity, Value: model.CloneLiveValue(value)})
		seen[identity] = true
	}
	for _, identity := range order {
		appendState(identity)
	}
	// Build currently admits only source-derived identities. Keep this fallback
	// deterministic for legacy hand-built profiles whose provenance is absent.
	for _, scalar := range manifest.Env {
		appendState(model.Identity{Kind: model.LiveEnv, Name: scalar.Name})
	}
	for _, list := range manifest.Lists {
		kind := model.LivePath
		if list.Name == "FPATH" {
			kind = model.LiveFPath
		}
		appendState(model.Identity{Kind: kind, Name: list.Name})
	}
	for _, name := range manifest.Functions.Added {
		appendState(model.Identity{Kind: model.LiveFunction, Name: name})
	}
	for _, option := range manifest.Options {
		appendState(model.Identity{Kind: model.LiveOption, Name: option.Name})
	}
	return states
}

func liveEntryIdentities(entry model.Entry) []model.Identity {
	if !entry.EffectiveManaged() || !entry.Representable() || entry.Secret != nil {
		return nil
	}
	switch entry.Kind {
	case model.KindAssignment:
		if len(entry.Names) != 1 {
			return nil
		}
		if entry.Category == model.CatPath {
			name := canonicalList(entry.Names[0])
			if name == "PATH" {
				return []model.Identity{{Kind: model.LivePath, Name: name}}
			}
			if name == "FPATH" {
				return []model.Identity{{Kind: model.LiveFPath, Name: name}}
			}
			return nil
		}
		if entry.Category == model.CatEnvironment || entry.Category == model.CatSecrets {
			return []model.Identity{{Kind: model.LiveEnv, Name: entry.Names[0]}}
		}
	case model.KindAlias:
		if len(entry.Names) == 1 {
			return []model.Identity{{Kind: model.LiveAlias, Name: entry.Names[0]}}
		}
	case model.KindFuncDecl:
		identities := make([]model.Identity, len(entry.Names))
		for index, name := range entry.Names {
			identities[index] = model.Identity{Kind: model.LiveFunction, Name: name}
		}
		return identities
	case model.KindCommand:
		if entry.Category == model.CatOptions && (entry.CmdName == "setopt" || entry.CmdName == unsetOptCommand) {
			identities := make([]model.Identity, len(entry.Names))
			for index, name := range entry.Names {
				identities[index] = model.Identity{Kind: model.LiveOption, Name: name}
			}
			return identities
		}
	}
	return nil
}

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
	lists := map[string][]listToken{}
	listOrder := []string{}
	for _, e := range p.Entries {
		if !e.EffectiveManaged() || !e.Representable() {
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
				name, next, ok := composeList(e.Names[0], e.ListValue, lists)
				if !ok && e.ListValue == nil && e.ValueMode == model.ValueModeLegacy {
					name, next, ok = composeLegacyList(e.Names[0], e.Value, lists)
				}
				if ok {
					if _, seen := lists[name]; seen {
						for i, candidate := range listOrder {
							if candidate == name {
								listOrder = append(listOrder[:i], listOrder[i+1:]...)
								break
							}
						}
					}
					lists[name] = next
					listOrder = append(listOrder, name)
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
			if e.FunctionBody == nil || len(e.Names) == 0 {
				continue
			}
			valid := true
			for _, name := range e.Names {
				if !symbolNameRE.MatchString(name) {
					valid = false
					break
				}
			}
			if !valid {
				continue
			}
			for _, name := range e.Names {
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
	for _, name := range listOrder {
		tokens := lists[name]
		base := 0
		adds := make([]string, 0, len(tokens)-1)
		dynamic := make([]bool, 0, len(tokens)-1)
		for _, token := range tokens {
			if token.base {
				base = len(adds)
				continue
			}
			adds = append(adds, token.value)
			dynamic = append(dynamic, token.dynamic)
		}
		m.Lists = append(m.Lists, model.ListDelta{Name: name, Additions: adds, Deletions: []string{}, BaseIndex: &base, AdditionDynamic: dynamic})
	}
	return m
}

type listToken struct {
	value         string
	dynamic, base bool
}

func composeList(raw string, value *model.ListValue, prior map[string][]listToken) (string, []listToken, bool) {
	name := canonicalList(raw)
	if name == "" || value == nil || !value.Valid() {
		return "", nil, false
	}
	current := prior[name]
	if current == nil {
		current = []listToken{{base: true}}
	}
	next := make([]listToken, 0, len(value.Segments)+len(current))
	for _, segment := range value.Segments {
		if segment.Self {
			next = append(next, current...)
			continue
		}
		if segment.Dynamic {
			next = append(next, listToken{value: segment.Source, dynamic: true})
			continue
		}
		next = append(next, listToken{value: segment.Value})
	}
	return name, next, true
}

// composeLegacyList accepts only the historical same-list representation, then
// translates its additions and base placement into the same token stream used
// by parser-proven semantic list values. Keeping this compatibility boundary
// here prevents stored legacy entries from bypassing source-order reduction.
func composeLegacyList(raw, value string, prior map[string][]listToken) (string, []listToken, bool) {
	delta, ok := pathDelta(raw, value)
	if !ok || delta.BaseIndex == nil {
		return "", nil, false
	}
	current := prior[delta.Name]
	if current == nil {
		current = []listToken{{base: true}}
	}
	next := make([]listToken, 0, len(current)+len(delta.Additions))
	for i := 0; i <= len(delta.Additions); i++ {
		if i == *delta.BaseIndex {
			next = append(next, current...)
		}
		if i < len(delta.Additions) {
			next = append(next, listToken{value: delta.Additions[i], dynamic: delta.AdditionDynamic[i]})
		}
	}
	return delta.Name, next, true
}

func canonicalList(name string) string {
	if name == "PATH" || name == "path" {
		return "PATH"
	}
	if name == "FPATH" || name == "fpath" {
		return "FPATH"
	}
	return ""
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
	canonical := canonicalList(name)
	if canonical == "" {
		return model.ListDelta{}, false
	}
	var base map[string]bool
	if canonical == "PATH" {
		base = map[string]bool{"$PATH": true, "$path": true, "${PATH}": true}
	} else {
		base = map[string]bool{"$FPATH": true, "$fpath": true, "${FPATH}": true}
	}
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
	dynamic := make([]bool, len(adds))
	for i, a := range adds {
		isDynamic, ok := legacyDynamicPath(a)
		if isDynamic && legacyListReference(a) {
			ok = false
		}
		if !ok {
			return model.ListDelta{}, false
		}
		dynamic[i] = isDynamic
	}
	baseIndex := idx
	if idx == len(parts)-1 {
		baseIndex = len(adds)
	}
	return model.ListDelta{
		Name:            canonical,
		Additions:       adds,
		Deletions:       []string{},
		BaseIndex:       &baseIndex,
		AdditionDynamic: dynamic,
	}, true
}

// legacyDynamicPath recognizes only the no-evaluation parameter form that can
// remain late-bound in a zsh list. It is intentionally narrower than shell
// expansion: a single simple parameter followed by path-safe literal segments.
func legacyDynamicPath(value string) (dynamic, ok bool) {
	if legacyDynamicPathRE.MatchString(value) {
		return true, true
	}
	if strings.Contains(value, "$") || unsafeStatic(value) {
		return false, false
	}
	return false, true
}

// legacyListReference rejects PATH/FPATH aliases in an addition. The one
// supported same-list reference is consumed as the explicit base marker above;
// admitting another list reference would invent cross-list semantics.
func legacyListReference(value string) bool {
	name := strings.TrimPrefix(value, "$")
	if strings.HasPrefix(name, "{") {
		end := strings.IndexByte(name, '}')
		if end < 0 {
			return true
		}
		name = name[1:end]
	} else if slash := strings.IndexByte(name, '/'); slash >= 0 {
		name = name[:slash]
	}
	return name == "PATH" || name == "path" || name == "FPATH" || name == "fpath"
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
