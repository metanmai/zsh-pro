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
		p.describe(stmt, &b)
		blocks = append(blocks, b)
	}
	return blocks, nil
}

// describe fills in the agnostic structural fields (Kind, CmdName, Names,
// Exported) from the mvdan/sh AST node.
func (p Provider) describe(stmt *syntax.Stmt, b *model.Block) {
	switch c := stmt.Cmd.(type) {
	case *syntax.CallExpr:
		// Pure assignment: leading assignments and no command words.
		if len(c.Args) == 0 {
			b.Kind = model.KindAssignment
			for _, a := range c.Assigns {
				if a.Name != nil {
					b.Names = append(b.Names, a.Name.Value)
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
			}
		case "export", "typeset", "declare", "local", "readonly":
			b.Kind = model.KindAssignment
			b.Exported = name == "export"
			for _, a := range c.Assigns {
				if a.Name != nil {
					b.Names = append(b.Names, a.Name.Value)
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
	case *syntax.IfClause, *syntax.ForClause, *syntax.WhileClause,
		*syntax.CaseClause, *syntax.Block, *syntax.Subshell:
		b.Kind = model.KindCompound
	default:
		b.Kind = model.KindOther
	}
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
