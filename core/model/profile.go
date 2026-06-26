package model

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

// Profile is the ordered intermediate representation of a parsed config: a single
// source-ordered list of Entry (D-02). It is the spine the v2.0 store and manifest
// builders serialize. Per-category views are deliberately NOT stored here;
// regeneration emits in source order (D-09), so a category view would be untested
// dead code this phase — Phase 4 adds one when the manifest builder needs it.
type Profile struct {
	Entries []Entry
}
