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
			Names:     b.Names,
			Value:     b.Value,
			Exported:  b.Exported,
			Managed:   routeManaged(b, cat),
			Override:  model.OverrideAuto,
			Dynamic:   b.Dynamic,
		})
	}
	return p
}
