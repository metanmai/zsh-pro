package model

// BlockKind is the agnostic structural shape of a parsed block, extracted by
// the shell Provider's parser so the (shell-specific) classifier and the
// (agnostic) reconciler can work without re-parsing.
type BlockKind string

const (
	KindAssignment BlockKind = "assignment" // FOO=bar / export FOO=bar
	KindAlias      BlockKind = "alias"      // alias gs=...
	KindFuncDecl   BlockKind = "func"       // foo() { ... }
	KindCommand    BlockKind = "command"    // a simple command call (setopt, bindkey, source, eval, ...)
	KindCompound   BlockKind = "compound"   // if/for/while/case/{...}/subshell
	KindOther      BlockKind = "other"
)

// Confidence expresses how sure the classifier is about a block's category.
type Confidence int

const (
	ConfLow Confidence = iota
	ConfMedium
	ConfHigh
)

// Block is one logical construct from the source (plus leading comments).
type Block struct {
	Text      string     // original source text (incl. leading comments)
	StartLine int        // 1-based line in source
	Kind      BlockKind  // structural shape (set by parser)
	CmdName   string     // leading command word for KindCommand (e.g. "setopt")
	Names     []string   // assigned var names / func name / alias name(s)
	Exported  bool       // assignment used `export`
	Opaque    bool       // parser could not structurally understand this block
	Category  Category   // set by classifier
	Conf      Confidence // set by classifier
	Value     string     // verbatim assignment value / alias body / option args / full func span (set by parser)
	Dynamic   bool       // value's word contains a non-literal AST part ($HOME/$(...)/etc.) — no execution
	Append    bool       // assignment used `+=` (append, not overwrite) — kept out of the templated path so it is never rewritten to `=` (WR-01)
	Flagged   bool       // alias carried a type flag (`-g`/`-s`/...) — kept out of the templated path so the flag is never dropped (WR-02)
	Array     bool       // assignment is array-valued (`name=(...)`): mvdan/sh populates a.Array and leaves a.Value nil, so the scalar templater cannot reproduce it; kept out of the templated path so the array value is never dropped (UAT array gap, same class as WR-01/WR-02)
	Indexed   bool       // assignment carried a subscript (`name[index]=...`); never lower it as scalar/list state
	// AliasAssignment distinguishes an alias definition (`alias name=value`),
	// including an empty value, from an alias query (`alias name`).
	AliasAssignment bool
	// DeclarationFlags retains semantic declaration attribute words in source
	// order (for example `-i` in `export -i COUNT=1`). It is distinct from
	// alias Flagged, which records only that an alias type flag was present.
	DeclarationFlags []string
	// OptionFlags retains setopt/unsetopt invocation controls in source order.
	// Nil means no controls appeared; an explicit empty slice remains distinct
	// for persistence fidelity.
	OptionFlags []string
	ValueMode   ValueMode // semantic meaning of Value for activation; parser-owned, additive to the verbatim source contract
	// RuntimeValue is the AST-decoded scalar or alias value when ValueMode is
	// ValueModeLiteral. Pointer presence distinguishes a decoded empty string
	// from a value that was not decoded.
	RuntimeValue *string
	// FunctionBody is assignment-ready function body text. Pointer presence
	// distinguishes an empty function body from a body that was not captured.
	FunctionBody *string
	// ListValue is a verified semantic contract for PATH/FPATH-style scalar
	// assignments. Nil means the parser could not safely model the list.
	ListValue *ListValue
}
