// Package render turns an Analysis into output. Renderers are role types: each
// is a pure function of the model and mutates nothing.
package render

import "zsh-pro/core/model"

// Renderer produces one output representation of an Analysis.
type Renderer interface {
	Render(a model.Analysis) ([]byte, error)
}
