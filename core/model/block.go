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
}
