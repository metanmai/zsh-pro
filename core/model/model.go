// Package model holds the shell-agnostic data types shared across the engine.
package model

// Category is one bucket in the (load-order) taxonomy.
type Category string

const (
	CatEnvironment Category = "environment"
	CatPath        Category = "path"
	CatSecrets     Category = "secrets"
	CatPlugins     Category = "plugins"
	CatOptions     Category = "options"
	CatKeybindings Category = "keybindings"
	CatFunctions   Category = "functions"
	CatAliases     Category = "aliases"
	CatLocal       Category = "local"
	CatMisc        Category = "misc"
)

// Categories returns the categories in taxonomy (load) order.
func Categories() []Category {
	return []Category{
		CatEnvironment, CatPath, CatSecrets, CatPlugins, CatOptions,
		CatKeybindings, CatFunctions, CatAliases, CatLocal, CatMisc,
	}
}

// CategoryDescription is a human-readable label for a category.
func CategoryDescription(c Category) string {
	switch c {
	case CatEnvironment:
		return "Environment variables / exports"
	case CatPath:
		return "PATH / fpath manipulation"
	case CatSecrets:
		return "Secrets & tokens (do not sync)"
	case CatPlugins:
		return "Plugin managers, framework & tool init"
	case CatOptions:
		return "Shell options (setopt/zstyle/autoload)"
	case CatKeybindings:
		return "Key bindings (bindkey)"
	case CatFunctions:
		return "Function definitions"
	case CatAliases:
		return "Aliases"
	case CatLocal:
		return "Machine / OS-specific overrides"
	default:
		return "Uncategorized — review by hand"
	}
}

// BlockKind is the agnostic structural shape of a parsed block, extracted by
// the shell Provider's parser so the (shell-specific) classifier and the
// (agnostic) reconciler can work without re-parsing.
type BlockKind string

const (
	KindAssignment BlockKind = "assignment" // FOO=bar / export FOO=bar
	KindAlias      BlockKind = "alias"       // alias gs=...
	KindFuncDecl   BlockKind = "func"        // foo() { ... }
	KindCommand    BlockKind = "command"     // a simple command call (setopt, bindkey, source, eval, ...)
	KindCompound   BlockKind = "compound"    // if/for/while/case/{...}/subshell
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
}

// IdentitySet is the resolved end-state captured by dynamic introspection.
type IdentitySet struct {
	Aliases   map[string]bool // alias names present after sourcing
	Functions map[string]bool // function names present after sourcing
	Env       map[string]bool // exported variable names present after sourcing
	Path      []string        // resolved $path entries, in order
	Options   map[string]bool // shell options that are "on"
	Available bool            // false when introspection failed (static-only)
}

// IssueKind enumerates the problems the engine reports.
type IssueKind string

const (
	IssueDuplicateAlias IssueKind = "duplicate_alias"
	IssueReassignedEnv  IssueKind = "reassigned_env"
	IssueDuplicatePath  IssueKind = "duplicate_path"
	IssueShadowed       IssueKind = "shadowed"
)

// Issue is one reported problem.
type Issue struct {
	Kind  IssueKind `json:"kind"`
	Name  string    `json:"name"`
	Lines []int     `json:"lines,omitempty"`
	Note  string    `json:"note,omitempty"`
}

// CategorySummary is the per-category rollup for the report.
type CategorySummary struct {
	Category Category `json:"category"`
	Count    int      `json:"count"`
	Items    []string `json:"items"`
}

// Analysis is the complete read-only result the renderers consume.
type Analysis struct {
	Path         string            `json:"path"`
	Lines        int               `json:"lines"`
	BlockCount   int               `json:"blocks"`
	OpaqueBlocks int               `json:"opaque_blocks"`
	Categories   []CategorySummary `json:"categories"`
	Issues       []Issue           `json:"issues"`
	HasSecrets   bool              `json:"has_secrets"`
	Introspected bool              `json:"introspected"`
	Notes        []string          `json:"notes,omitempty"`
}

// ExitCode is 3 when actionable issues exist, else 0. Runtime (1) and usage
// (2) errors are handled by the CLI, not derived from a successful Analysis.
func (a Analysis) ExitCode() int {
	if len(a.Issues) > 0 {
		return 3
	}
	return 0
}
