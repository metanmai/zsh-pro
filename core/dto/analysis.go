// Package dto holds the wire-format (JSON) structs for zsh-pro output. These are
// data-only transfer objects, deliberately separate from the domain model in
// core/model. dto imports nothing inside core/ — it is a leaf.
package dto

// Analysis is the wire shape of an analysis result.
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

// Issue is the wire shape of one reported problem.
type Issue struct {
	Kind  string `json:"kind"`
	Name  string `json:"name"`
	Lines []int  `json:"lines,omitempty"`
	Note  string `json:"note,omitempty"`
}

// CategorySummary is the wire shape of one per-category rollup.
type CategorySummary struct {
	Category string   `json:"category"`
	Count    int      `json:"count"`
	Items    []string `json:"items,omitempty"`
}
