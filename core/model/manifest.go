package model

// SchemaV1 is the manifest schema accepted by the activation differ.
const SchemaV1 = "v1"

// Manifest is the reversible declarative record for one profile.
type Manifest struct {
	Profile   string      `json:"profile"`   // profile/branch name
	Schema    string      `json:"schema"`    // forward-compatibility schema
	Env       []Scalar    `json:"env"`       // scalar environment changes
	Lists     []ListDelta `json:"lists"`     // PATH/FPATH deltas
	Aliases   AliasSet    `json:"aliases"`   // alias changes
	Functions FuncSet     `json:"functions"` // function changes
	Options   []OptionSet `json:"options"`   // option changes
}

// Scalar records an applied scalar and its optional prior value. Original is
// tri-state: nil means unset, while pointers preserve empty and non-empty values.
type Scalar struct {
	Name     string  `json:"name"`               // variable name
	Applied  string  `json:"applied"`            // profile value
	Original *string `json:"original,omitempty"` // runtime-reconciled prior value
}

// ListDelta records additions and deletions relative to the runtime base. Deletions
// are validated-present but unexercised in this phase; later runtime work authors them.
type ListDelta struct {
	Name      string   `json:"name"`      // PATH or FPATH
	Additions []string `json:"additions"` // profile entries
	Deletions []string `json:"deletions"` // base entries removed (Phase 5+)
}

// AliasSet separates profile-added alias bodies from runtime-captured shadows.
// This distinct shape supersedes the tentative uniform-map design because the
// validated wire fixture uses a map for aliases.added.
type AliasSet struct {
	Added    map[string]string `json:"added"`    // profile aliases
	Shadowed map[string]string `json:"shadowed"` // runtime prior bodies
}

// FuncSet separates function names (an array in the wire contract) from runtime
// captured shadow bodies; a uniform map cannot unmarshal functions.added arrays.
type FuncSet struct {
	Added    []string          `json:"added"`    // profile function names
	Shadowed map[string]string `json:"shadowed"` // runtime prior bodies
}

// OptionSet records the desired option state. WasOn is a runtime-captured fact;
// the builder is forbidden to author it and leaves it at its zero value.
type OptionSet struct {
	Name    string `json:"name"`    // option name
	Enabled bool   `json:"enabled"` // desired state
	WasOn   bool   `json:"was_on"`  // runtime prior state, never builder-authored
}
