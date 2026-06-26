package zsh

import (
	"bytes"
	"strings"

	"mvdan.cc/sh/v3/syntax"

	"zsh-pro/core/model"
)

// Parse turns zsh source into ordered, structurally-described blocks. A block
// that the parser cannot understand (or a whole file that fails to parse) is
// returned as an opaque block rather than causing an error.
func (p Provider) Parse(src []byte) ([]model.Block, error) {
	parser := syntax.NewParser(
		syntax.Variant(syntax.LangZsh), // confirmed in Step 1
		syntax.KeepComments(true),
	)
	file, err := parser.Parse(bytes.NewReader(src), "")
	if err != nil {
		// Opaque blocks leave Dynamic=false by construction; they are emitted
		// verbatim regardless, so Dynamic must not be read as a standalone filter.
		return []model.Block{{
			Text:      string(src),
			StartLine: 1,
			Kind:      model.KindOther,
			Opaque:    true,
		}}, nil
	}

	var blocks []model.Block
	for _, stmt := range file.Stmts {
		start := stmt.Pos().Offset()
		startLine := stmt.Pos().Line()
		// Pull in leading comments that sit directly above the statement so the
		// block's Text carries the documentation above it for context. StartLine
		// stays the statement's own line (not the comment's): an issue points at
		// the construct, while Text begins at the pulled-up comment.
		for _, c := range stmt.Comments {
			if c.End().Offset() <= stmt.Pos().Offset() && c.Pos().Offset() < start {
				start = c.Pos().Offset()
			}
		}
		end := stmt.End().Offset()
		if int(end) > len(src) {
			end = uint(len(src))
		}

		b := model.Block{
			Text:      strings.TrimRight(string(src[start:end]), "\n"),
			StartLine: int(startLine),
		}
		p.describe(stmt, &b, src)
		blocks = append(blocks, b)
	}
	return blocks, nil
}

// describe fills in the agnostic structural fields (Kind, CmdName, Names,
// Exported) from the mvdan/sh AST node, plus the additive Value (verbatim value
// text) and Dynamic flag (an AST-derived static/dynamic verdict — no execution).
// src is the original source, used to offset-slice verbatim value spans.
func (p Provider) describe(stmt *syntax.Stmt, b *model.Block, src []byte) {
	switch c := stmt.Cmd.(type) {
	case *syntax.CallExpr:
		// Pure assignment: leading assignments and no command words.
		if len(c.Args) == 0 {
			b.Kind = model.KindAssignment
			for _, a := range c.Assigns {
				if a.Name != nil {
					b.Names = append(b.Names, a.Name.Value)
				}
				if a.Value != nil {
					b.Value = sliceSrc(src, a.Value.Pos().Offset(), a.Value.End().Offset())
					b.Dynamic = b.Dynamic || wordIsDynamic(a.Value)
				}
			}
			return
		}
		name := c.Args[0].Lit()
		b.CmdName = name
		switch name {
		case "alias":
			b.Kind = model.KindAlias
			for _, w := range c.Args[1:] {
				// alias args are name=value words; the value half is often
				// quoted, so Word.Lit() is empty. Read the literal name prefix
				// (the leading *Lit part) up to the '='.
				lit := p.wordLitPrefix(w)
				if i := strings.IndexByte(lit, '='); i > 0 {
					b.Names = append(b.Names, lit[:i])
				}
				// Value is the text AFTER the first '=' in the word's verbatim
				// source span, quotes preserved (no re-quoting). Alias args are
				// flat *Word, not *Assign, so there is no clean Assign.Value path.
				span := sliceSrc(src, w.Pos().Offset(), w.End().Offset())
				if i := strings.IndexByte(span, '='); i >= 0 {
					b.Value = span[i+1:]
				}
				b.Dynamic = b.Dynamic || wordIsDynamic(w)
			}
		case "export", "typeset", "declare", "local", "readonly":
			b.Kind = model.KindAssignment
			b.Exported = name == "export"
			for _, a := range c.Assigns {
				if a.Name != nil {
					b.Names = append(b.Names, a.Name.Value)
				}
				if a.Value != nil {
					b.Value = sliceSrc(src, a.Value.Pos().Offset(), a.Value.End().Offset())
					b.Dynamic = b.Dynamic || wordIsDynamic(a.Value)
				}
			}
			for _, w := range c.Args[1:] {
				lit := p.wordLitPrefix(w)
				if lit == "" || strings.HasPrefix(lit, "-") {
					continue
				}
				if i := strings.IndexByte(lit, '='); i > 0 {
					b.Names = append(b.Names, lit[:i])
				} else {
					b.Names = append(b.Names, lit)
				}
			}
		case "setopt", "unsetopt":
			// setopt/unsetopt carry their reversible state in the option-name
			// args (e.g. `setopt EXTENDED_GLOB`). Capture each option name into
			// Names so the routing gate can admit it (ING-02) and the templater
			// can rebuild it from structured fields (D-10). Flag args (-o NAME)
			// are skipped — a bare/flag-only setopt has no name to template.
			b.Kind = model.KindCommand
			for _, w := range c.Args[1:] {
				lit := p.wordLitPrefix(w)
				if lit == "" || strings.HasPrefix(lit, "-") {
					continue
				}
				b.Names = append(b.Names, lit)
			}
		default:
			b.Kind = model.KindCommand
		}
	case *syntax.DeclClause:
		// zsh parses export/typeset/declare/local/readonly as a DeclClause
		// rather than a CallExpr. Names live in its Args ([]*Assign).
		b.Kind = model.KindAssignment
		if c.Variant != nil {
			b.CmdName = c.Variant.Value
			b.Exported = c.Variant.Value == "export"
		}
		for _, a := range c.Args {
			if a.Name != nil {
				b.Names = append(b.Names, a.Name.Value)
			}
			if a.Value != nil {
				b.Value = sliceSrc(src, a.Value.Pos().Offset(), a.Value.End().Offset())
				b.Dynamic = b.Dynamic || wordIsDynamic(a.Value)
			}
		}
	case *syntax.FuncDecl:
		b.Kind = model.KindFuncDecl
		if c.Name != nil {
			b.Names = append(b.Names, c.Name.Value)
		}
		// zsh allows `function a b {}` declaring several names at once.
		for _, n := range c.Names {
			if n != nil {
				b.Names = append(b.Names, n.Value)
			}
		}
		// Capture the FULL `name() { ... }` span verbatim (NOT just the
		// brace-body) — the pinned convention 02-02's templater emits unchanged.
		b.Value = sliceSrc(src, stmt.Pos().Offset(), stmt.End().Offset())
	case *syntax.IfClause, *syntax.ForClause, *syntax.WhileClause,
		*syntax.CaseClause, *syntax.Block, *syntax.Subshell:
		b.Kind = model.KindCompound
		// Compound blocks (if/for/while/case) are conditionals: dynamic by
		// construction.
		b.Dynamic = true
	default:
		b.Kind = model.KindOther
	}
}

// sliceSrc returns the verbatim source text spanning [start, end), clamped to
// src bounds. It mirrors the offset-slicing used for Block.Text in Parse.
func sliceSrc(src []byte, start, end uint) string {
	if int(end) > len(src) {
		end = uint(len(src))
	}
	if start > end {
		return ""
	}
	return string(src[start:end])
}

// wordIsDynamic reports whether a value word contains a non-literal AST part:
// a parameter expansion ($X / ${...}), a command substitution ($(...) or
// backtick `...`, both *CmdSubst), an arithmetic expansion ($((...))), or a
// process substitution (<(...)). It only walks the AST — it performs NO
// execution and never resolves a value (EVAL-01).
func wordIsDynamic(w *syntax.Word) bool {
	if w == nil {
		return false
	}
	dynamic := false
	syntax.Walk(w, func(n syntax.Node) bool {
		switch n.(type) {
		case *syntax.ParamExp, *syntax.CmdSubst, *syntax.ArithmExp, *syntax.ProcSubst:
			dynamic = true
			return false // stop walking once a dynamic part is found
		}
		return true
	})
	return dynamic
}

// wordLitPrefix returns the leading literal text of a word. For a word like
// `gs='git status'` (a *Lit followed by a quoted part) Word.Lit() is empty,
// but the name we care about lives in the first *Lit part ("gs=").
func (Provider) wordLitPrefix(w *syntax.Word) string {
	if lit := w.Lit(); lit != "" {
		return lit
	}
	if len(w.Parts) > 0 {
		if l, ok := w.Parts[0].(*syntax.Lit); ok {
			return l.Value
		}
	}
	return ""
}
