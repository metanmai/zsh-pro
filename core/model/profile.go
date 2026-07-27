package model

// ValueMode records how a parsed scalar or alias Value may be consumed at
// activation time without changing Value's verbatim source-text contract.
type ValueMode string

const (
	// ValueModeLegacy is the zero value for entries created before the semantic
	// contract existed. Only legacy entries may use the historical
	// Value/Dynamic fallback.
	ValueModeLegacy ValueMode = ""
	// ValueModeLiteral means RuntimeValue contains AST-decoded data, including a
	// present pointer to an empty string.
	ValueModeLiteral ValueMode = "literal"
	// ValueModeDynamic means Value remains verbatim late-bound shell syntax and
	// RuntimeValue is nil.
	ValueModeDynamic ValueMode = "dynamic"
	// ValueModeUnsupported means no safe runtime value is available (for example,
	// the parser rejected a static shape or the store redacted a literal secret).
	// It must never use the legacy fallback.
	ValueModeUnsupported ValueMode = "unsupported"
)

// ListSegment is one ordered part of a semantic PATH/FPATH scalar value.
// Literal segments carry runtime Value. Dynamic segments carry parser-validated
// scalar-context Source, which zsh may expand to zero, one, or many elements.
// Self marks the one same-list reference that supplies the existing tied value.
type ListSegment struct {
	Value   string
	Source  string
	Dynamic bool
	Self    bool
}

// ListValue is the presence-aware semantic representation of a PATH/FPATH
// scalar assignment. A present value may contain only its Self segment.
type ListValue struct {
	Segments []ListSegment
}

// Valid reports whether ListValue can be consumed without inventing list
// semantics. It requires exactly one self marker and rejects combinations that
// would blur literal data, dynamic source, or the current list reference.
func (v ListValue) Valid() bool {
	selfCount := 0
	for _, s := range v.Segments {
		switch {
		case s.Self:
			if s.Dynamic || s.Value != "" || s.Source != "" {
				return false
			}
			selfCount++
		case s.Dynamic:
			if s.Value != "" || s.Source == "" {
				return false
			}
		case s.Source != "":
			return false
		}
	}
	return selfCount == 1
}

// ManagedOverride is an explicit per-entry override of the auto managed/imperative
// verdict. It is an orthogonal axis to the Dynamic flag (D-05): an entry may be
// dynamic yet managed, or static yet forced unmanaged.
type ManagedOverride string

const (
	// OverrideAuto defers to the auto Managed verdict (the default).
	OverrideAuto ManagedOverride = "auto"
	// OverrideManaged forces the entry into the managed/declarative path,
	// regardless of the auto verdict (D-07: override wins and persists).
	OverrideManaged ManagedOverride = "forced-managed"
	// OverrideUnmanaged forces the entry out of the managed path, regardless of
	// the auto verdict (D-07: override wins and persists).
	OverrideUnmanaged ManagedOverride = "forced-unmanaged"
)

// Entry is one source statement in a Profile, carrying the verbatim Text (D-01)
// plus the derived fields the templater needs to rebuild a declarative entry
// (D-10). Entries are stored in source order on Profile.Entries (D-02); the
// Category is a field, not the storage layout.
type Entry struct {
	Text      string          // verbatim source text of the statement (D-01)
	StartLine int             // 1-based line in source
	Category  Category        // classifier verdict
	Kind      BlockKind       // structural shape (from the parser)
	CmdName   string          // leading command word for KindCommand (e.g. "setopt")
	Names     []string        // assigned var names / func name / alias name(s)
	Value     string          // verbatim assignment value / alias body / option args / full func span
	Exported  bool            // assignment used `export`
	Managed   bool            // auto verdict: is this a reversible declarative class? (D-06)
	Override  ManagedOverride // explicit override of the auto verdict (D-07); defaults OverrideAuto
	Dynamic   bool            // value contains a non-literal AST part ($HOME/$(...)/...) — orthogonal axis (D-05)
	// StructuralFidelityKnown is true only when all parser-owned source-shape
	// markers below are present. Historical DTOs that omitted any marker remain
	// unknown rather than being guessed as ordinary scalar declarations.
	StructuralFidelityKnown bool
	Append                  bool       // assignment used +=; never lower it to =
	Array                   bool       // assignment used (...); never lower it to a scalar
	Flagged                 bool       // alias used a flag; never drop its attributes
	Secret                  *SecretRef // non-nil iff this entry is a secret-replaced literal (D-07); the literal Value is cleared when set so the secret never round-trips
	ValueMode               ValueMode  // parser-owned semantic mode; zero remains backward-compatible legacy
	// RuntimeValue is the AST-decoded scalar or alias value for literal mode.
	// Pointer presence distinguishes decoded empty from missing.
	RuntimeValue *string
	// FunctionBody is assignment-ready function body text. Pointer presence
	// distinguishes a parsed empty function from a missing body.
	FunctionBody *string
	// ListValue is the optional parser-verified semantic list contract. Nil
	// preserves legacy and unsupported entries without inferring semantics.
	ListValue *ListValue
}

// EffectiveManaged reports whether the entry is treated as managed (declarative,
// switchable) after applying any Override. A set Override wins over the auto
// Managed verdict and persists (D-07): OverrideManaged -> true, OverrideUnmanaged
// -> false, OverrideAuto -> the Managed bool.
func (e Entry) EffectiveManaged() bool {
	switch e.Override {
	case OverrideManaged:
		return true
	case OverrideUnmanaged:
		return false
	default:
		return e.Managed
	}
}

// DeclarationRepresentable reports whether Entry contains enough declaration
// semantics to recreate its source without weakening shell attributes or
// scope. The IR intentionally records declaration command markers, but not
// integer, readonly, tied-list, or local-scope attributes, so those forms must
// remain verbatim even when a persisted override forces them managed.
func (e Entry) DeclarationRepresentable() bool {
	if e.Kind != KindAssignment {
		return true
	}
	switch e.CmdName {
	case "typeset", "declare", "local", "readonly":
		return false
	default:
		return true
	}
}

// Representable reports whether the persisted Entry has enough source-derived
// structure to recreate its semantics. It deliberately does not answer whether
// Override selected the managed axis: a forced override cannot compensate for
// incomplete or behavior-bearing source structure.
func (e Entry) Representable() bool {
	if !e.StructuralFidelityKnown {
		return false
	}
	if !e.DeclarationRepresentable() {
		return false
	}
	switch e.Kind {
	case KindAssignment:
		return !e.Append && !e.Array
	case KindAlias:
		return !e.Flagged
	default:
		return true
	}
}

// Profile is the ordered intermediate representation of a parsed config: a single
// source-ordered list of Entry (D-02). It is the spine the v2.0 store and manifest
// builders serialize. Per-category views are deliberately NOT stored here;
// regeneration emits in source order (D-09), so a category view would be untested
// dead code this phase — Phase 4 adds one when the manifest builder needs it.
type Profile struct {
	Entries []Entry
}
