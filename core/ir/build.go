// Package ir builds the intermediate representation of a parsed config: it turns
// the parser's []model.Block into an ordered model.Profile, classifying each
// statement declarative-vs-imperative (the ING-02 switchability gate) and
// carrying the parse-time static/dynamic verdict (EVAL-01). It performs NO
// execution of user config. Like core/analyze it depends only on core/model and
// the core/shell interfaces — never the concrete core/shell/zsh provider.
package ir

import (
	"zsh-pro/core/model"
	"zsh-pro/core/shell"
)

// Build turns parsed blocks into an ordered Profile. It classifies each block
// via the injected Classifier (reusing the existing classifier — never
// re-parsing), and appends one Entry per block in SOURCE ORDER (D-02). Each
// Entry copies the block's verbatim Text and derived fields, defaults its
// Override to OverrideAuto (D-07), and sets Managed from the routeManaged
// declarative/imperative verdict (ING-02). The Dynamic flag is the parse-time
// value (D-05) — an orthogonal axis to Managed.
func Build(blocks []model.Block, c shell.Classifier) model.Profile {
	p := model.Profile{Entries: make([]model.Entry, 0, len(blocks))}
	for i := range blocks {
		b := blocks[i]
		cat, _ := c.Classify(b) // confidence is never a routing gate (D-06)
		p.Entries = append(p.Entries, model.Entry{
			Text:      b.Text,
			StartLine: b.StartLine,
			Category:  cat,
			Kind:      b.Kind,
			CmdName:   b.CmdName,
			// Defensive-copy the Names slice (WR-03): copying the slice header
			// straight through would make the Entry and the source Block share a
			// backing array, so a later mutation of either silently corrupts the
			// other. A fresh slice severs that aliasing at the IR boundary.
			Names:    append([]string(nil), b.Names...),
			Value:    b.Value,
			Exported: b.Exported,
			Managed:  routeManaged(b, cat),
			Override: model.OverrideAuto,
			Dynamic:  b.Dynamic,
			// Parser Blocks carry every source-shape marker unless an unavailable
			// declaration shape made the statement opaque. Keep that state unknown
			// so a later override cannot promote it into scalar lowering.
			StructuralFidelityKnown: !b.Opaque,
			Append:                  b.Append,
			Array:                   b.Array,
			Flagged:                 b.Flagged,
			Indexed:                 b.Indexed,
			AliasAssignment:         b.AliasAssignment,
			DeclarationFlags:        completeStringSlice(b.DeclarationFlags),
			OptionFlags:             cloneStrings(b.OptionFlags),
			ValueMode:               b.ValueMode,
			// The Block remains independently reusable after Build. Copy pointed-to
			// values instead of sharing mutable storage across the IR boundary.
			RuntimeValue: cloneString(b.RuntimeValue),
			FunctionBody: cloneString(b.FunctionBody),
			ListValue:    cloneListValue(b.ListValue),
		})
	}
	return p
}

func cloneString(s *string) *string {
	if s == nil {
		return nil
	}
	v := *s
	return &v
}

func cloneStrings(values []string) []string {
	if values == nil {
		return nil
	}
	cloned := make([]string, len(values))
	copy(cloned, values)
	return cloned
}

func completeStringSlice(values []string) []string {
	if values == nil {
		return []string{}
	}
	return cloneStrings(values)
}

func cloneListValue(v *model.ListValue) *model.ListValue {
	if v == nil || !v.Valid() {
		return nil
	}
	return &model.ListValue{Segments: append([]model.ListSegment(nil), v.Segments...)}
}
