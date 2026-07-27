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
				captureAssignmentShape(b, a)
				if a.Name != nil {
					b.Names = append(b.Names, a.Name.Value)
				}
				// `+=` append must be recorded so the router keeps it out of the
				// templated path (WR-01): the templater only emits `=`, which would
				// silently turn an append into an overwrite across the round-trip.
				if a.Append {
					b.Append = true
				}
				if a.Value != nil {
					b.Value = sliceSrc(src, a.Value.Pos().Offset(), a.Value.End().Offset())
					captureWordSemantics(b, a.Value, "")
				}
				// Array-valued (`name=(...)`): mvdan/sh populates a.Array and leaves
				// a.Value nil, so the scalar branch above is skipped and b.Value stays
				// empty. Detect it so the router keeps it out of the templated path —
				// the full `(...)` span (multi-line included) round-trips via verbatim
				// Text instead of being dropped to a bare `name=` (UAT array gap).
				if a.Array != nil {
					b.Array = true
				}
			}
			ensureExplicitValueMode(b)
			captureListValue(b, c.Assigns, src)
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
				// A type flag (`alias -g`/`-s`/...) changes alias semantics
				// (global/suffix vs regular). The flag is not captured in the
				// structured fields, so record its presence (WR-02) to keep the
				// statement out of the templated path; it then round-trips as
				// verbatim Text with the flag intact.
				if strings.HasPrefix(lit, "-") {
					b.Flagged = true
					continue
				}
				prefix := ""
				if i := strings.IndexByte(lit, '='); i > 0 {
					b.Names = append(b.Names, lit[:i])
					prefix = lit[:i+1]
				}
				// Value is the text AFTER the first '=' in the word's verbatim
				// source span, quotes preserved (no re-quoting). Alias args are
				// flat *Word, not *Assign, so there is no clean Assign.Value path.
				span := sliceSrc(src, w.Pos().Offset(), w.End().Offset())
				if i := strings.IndexByte(span, '='); i >= 0 {
					b.Value = span[i+1:]
				}
				if prefix != "" {
					captureWordSemantics(b, w, prefix)
				}
			}
			ensureExplicitValueMode(b)
		case "export", "typeset", "declare", "local", "readonly":
			b.Kind = model.KindAssignment
			b.Exported = name == "export"
			p.captureDeclarationFlags(b, c.Args[1:])
			for _, a := range c.Assigns {
				captureAssignmentShape(b, a)
				if a.Name != nil {
					b.Names = append(b.Names, a.Name.Value)
				}
				if a.Append {
					b.Append = true // WR-01: keep `export FOO+=x` out of the templated path
				}
				if a.Value != nil {
					b.Value = sliceSrc(src, a.Value.Pos().Offset(), a.Value.End().Offset())
					captureWordSemantics(b, a.Value, "")
				}
				if a.Array != nil {
					b.Array = true // `export ARR=(x y)`: keep out of the templated path (UAT array gap)
				}
			}
			ensureExplicitValueMode(b)
			captureListValue(b, c.Assigns, src)
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
			captureAssignmentShape(b, a)
			if a.Name != nil {
				b.Names = append(b.Names, a.Name.Value)
			}
			if a.Append {
				b.Append = true // WR-01: `export PATH+=:/x` parses as a DeclClause
			}
			if a.Value != nil {
				b.Value = sliceSrc(src, a.Value.Pos().Offset(), a.Value.End().Offset())
				captureWordSemantics(b, a.Value, "")
			}
			if a.Array != nil {
				b.Array = true // `typeset -a arr=(p q)` parses as a DeclClause (UAT array gap)
			}
			if a.Naked && a.Name == nil && a.Value != nil {
				p.captureDeclarationFlags(b, []*syntax.Word{a.Value})
			}
		}
		ensureExplicitValueMode(b)
		captureListValue(b, c.Args, src)
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
		b.FunctionBody = captureFunctionBody(c.Body, src)
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

// captureAssignmentShape records every parser-visible assignment source shape
// that would otherwise be silently lowered to a plain scalar. A nil Index is
// an explicit parser-known unindexed shape; a non-nil Index covers numeric and
// associative subscripts without evaluating either form.
func captureAssignmentShape(b *model.Block, a *syntax.Assign) {
	if a != nil && a.Index != nil {
		b.Indexed = true
	}
}

// captureDeclarationFlags records only real literal option words. `--` ends
// option parsing and is not itself an attribute; words after it are ordinary
// declaration arguments. This deliberately leaves alias Flagged independent.
func (p Provider) captureDeclarationFlags(b *model.Block, words []*syntax.Word) {
	options := true
	for _, w := range words {
		lit := p.wordLitPrefix(w)
		if lit == "" {
			// A dynamically produced argument can become a declaration option at
			// execution time. The AST cannot expose its semantic attributes, so
			// leave the whole source shape opaque instead of claiming no flags.
			b.Opaque = true
			continue
		}
		if options && lit == "--" {
			options = false
			continue
		}
		if options && strings.HasPrefix(lit, "-") && lit != "-" {
			b.DeclarationFlags = append(b.DeclarationFlags, lit)
		}
	}
}

// ensureExplicitValueMode prevents newly parsed scalar and alias blocks from
// sharing the zero-value legacy fallback. Shapes without one safely modeled
// word (bare declarations, arrays, flags, or multiple values) fail closed.
func ensureExplicitValueMode(b *model.Block) {
	if b.ValueMode == model.ValueModeLegacy {
		b.ValueMode = model.ValueModeUnsupported
		b.RuntimeValue = nil
	}
}

// captureWordSemantics assigns one explicit semantic mode while preserving the
// caller-owned verbatim Value. prefix is empty for assignments and the decoded
// "name=" prefix for aliases, where only that exact prefix is removed.
func captureWordSemantics(b *model.Block, w *syntax.Word, prefix string) {
	if b.ValueMode != model.ValueModeLegacy {
		// A Block has only one Value/RuntimeValue slot. Multiple assignment or
		// alias words are routed imperative; mark their aggregate unsupported
		// rather than attach one word's semantics to another word's source.
		b.ValueMode = model.ValueModeUnsupported
		b.RuntimeValue = nil
		b.Dynamic = b.Dynamic || wordIsDynamic(w)
		return
	}

	if wordIsDynamic(w) {
		b.Dynamic = true
		b.ValueMode = model.ValueModeDynamic
		b.RuntimeValue = nil
		return
	}

	decoded, ok := decodeLiteralWord(w, prefix)
	if !ok {
		b.ValueMode = model.ValueModeUnsupported
		b.RuntimeValue = nil
		return
	}
	if prefix != "" {
		if !strings.HasPrefix(decoded, prefix) {
			b.ValueMode = model.ValueModeUnsupported
			return
		}
		decoded = strings.TrimPrefix(decoded, prefix)
	}
	b.ValueMode = model.ValueModeLiteral
	b.RuntimeValue = &decoded
}

// captureListValue records the PATH/FPATH semantic list only for a single
// scalar assignment. The normal Value/RuntimeValue contract remains untouched.
func captureListValue(b *model.Block, assigns []*syntax.Assign, src []byte) {
	if len(assigns) != 1 || b.Append || b.Array {
		return
	}
	a := assigns[0]
	if a.Name == nil || a.Value == nil || canonicalListName(a.Name.Value) == "" {
		return
	}
	b.ListValue = decodeListValue(a.Name.Value, a.Value, src)
}

func canonicalListName(name string) string {
	switch name {
	case "PATH", "path":
		return "PATH"
	case "FPATH", "fpath":
		return "FPATH"
	default:
		return ""
	}
}

type listPiece struct {
	literal string
	param   string
	source  string
}

// decodeListValue tokenizes a scalar PATH/FPATH value using only the parsed
// AST. Dynamic source remains a scalar expression: it is deliberately not
// split into presumed list elements before zsh evaluates it in tied context.
func decodeListValue(name string, w *syntax.Word, src []byte) *model.ListValue {
	canonical := canonicalListName(name)
	if canonical == "" || w == nil {
		return nil
	}
	pieces, ok := listPieces(w.Parts, src, literalUnquoted)
	if !ok {
		return nil
	}

	var segments []model.ListSegment
	var literal, source strings.Builder
	var params []string
	hasLiteral := false
	flush := func() bool {
		if len(params) == 0 {
			segments = append(segments, model.ListSegment{Value: literal.String()})
		} else {
			listParam := ""
			for _, param := range params {
				if got := canonicalListName(param); got != "" {
					if got != canonical || listParam != "" {
						return false
					}
					listParam = got
				}
			}
			if listParam != "" {
				if len(params) != 1 || hasLiteral {
					return false
				}
				segments = append(segments, model.ListSegment{Self: true})
			} else {
				segments = append(segments, model.ListSegment{Dynamic: true, Source: source.String()})
			}
		}
		literal.Reset()
		source.Reset()
		params = nil
		hasLiteral = false
		return true
	}
	for _, piece := range pieces {
		if piece.param != "" {
			params = append(params, piece.param)
			source.WriteString(piece.source)
			continue
		}
		parts := strings.Split(piece.literal, ":")
		for i, part := range parts {
			literal.WriteString(part)
			if part != "" {
				source.WriteString(listLiteralSource(part))
				hasLiteral = true
			}
			if i < len(parts)-1 && !flush() {
				return nil
			}
		}
	}
	if !flush() {
		return nil
	}
	value := &model.ListValue{Segments: segments}
	if !value.Valid() {
		return nil
	}
	return value
}

func listPieces(parts []syntax.WordPart, src []byte, context literalQuoteContext) ([]listPiece, bool) {
	var pieces []listPiece
	for _, part := range parts {
		switch p := part.(type) {
		case *syntax.Lit:
			var literal strings.Builder
			if !decodeLiteralLit(&literal, p.Value, context) {
				return nil, false
			}
			pieces = append(pieces, listPiece{literal: literal.String()})
		case *syntax.SglQuoted:
			if p.Dollar {
				return nil, false
			}
			pieces = append(pieces, listPiece{literal: p.Value})
		case *syntax.DblQuoted:
			if p.Dollar {
				return nil, false
			}
			nested, ok := listPieces(p.Parts, src, literalDoubleQuoted)
			if !ok {
				return nil, false
			}
			pieces = append(pieces, nested...)
		case *syntax.ParamExp:
			param, ok := simpleParamName(p)
			if !ok {
				return nil, false
			}
			pieces = append(pieces, listPiece{
				param:  param,
				source: sliceSrc(src, p.Pos().Offset(), p.End().Offset()),
			})
		default:
			return nil, false
		}
	}
	return pieces, true
}

func simpleParamName(p *syntax.ParamExp) (string, bool) {
	if p == nil || p.Param == nil || p.Flags != nil || p.Excl || p.Length || p.Width || p.IsSet ||
		p.NestedParam != nil || p.Index != nil || len(p.Modifiers) != 0 || p.Slice != nil ||
		p.Repl != nil || p.Names != 0 || p.Exp != nil {
		return "", false
	}
	return p.Param.Value, true
}

func listLiteralSource(value string) string {
	if value == "" {
		return "''"
	}
	for i := 0; i < len(value); i++ {
		switch value[i] {
		case ' ', '\t', '\n', '\\', '\'', '"', '$', '`', '*', '?', '[', ']', '{', '}', '(', ')', '|', '&', ';', '<', '>', '!', '~':
			var quoted strings.Builder
			quoted.WriteByte('"')
			for j := 0; j < len(value); j++ {
				if strings.ContainsRune("\\\"$`", rune(value[j])) {
					quoted.WriteByte('\\')
				}
				quoted.WriteByte(value[j])
			}
			quoted.WriteByte('"')
			return quoted.String()
		}
	}
	return value
}

// decodeLiteralWord decodes only AST forms whose runtime data semantics are
// explicitly modeled. It does no expansion, environment lookup, filesystem
// access, or execution. prefix identifies the unquoted alias "name=" portion
// so a tilde immediately after it is still recognized as a leading expansion.
func decodeLiteralWord(w *syntax.Word, prefix string) (string, bool) {
	if w == nil {
		return "", false
	}

	word := *w
	word.Parts = append([]syntax.WordPart(nil), w.Parts...)
	// Brace expansion begins life inside Lit nodes. SplitBraces upgrades valid
	// forms to BraceExp nodes so the allowlist rejects them instead of treating
	// their expansion syntax as inert data.
	syntax.SplitBraces(&word)

	if len(word.Parts) > 0 {
		if lit, ok := word.Parts[0].(*syntax.Lit); ok {
			leading := lit.Value
			if prefix != "" {
				if !strings.HasPrefix(leading, prefix) {
					return "", false
				}
				leading = strings.TrimPrefix(leading, prefix)
			}
			if strings.HasPrefix(leading, "~") {
				return "", false
			}
		}
	}
	if containsUnquotedExtglob(word.Parts) {
		return "", false
	}

	var out strings.Builder
	if !decodeLiteralParts(&out, word.Parts, literalUnquoted, true) {
		return "", false
	}
	return out.String(), true
}

type literalQuoteContext uint8

const (
	literalUnquoted literalQuoteContext = iota
	literalDoubleQuoted
)

func decodeLiteralParts(out *strings.Builder, parts []syntax.WordPart, context literalQuoteContext, rejectExtglob bool) bool {
	for _, part := range parts {
		switch p := part.(type) {
		case *syntax.Lit:
			if rejectExtglob && containsExtglobSyntax(p.Value) {
				return false
			}
			if !decodeLiteralLit(out, p.Value, context) {
				return false
			}
		case *syntax.SglQuoted:
			if p.Dollar {
				return false
			}
			out.WriteString(p.Value)
		case *syntax.DblQuoted:
			if p.Dollar || !decodeLiteralParts(out, p.Parts, literalDoubleQuoted, false) {
				return false
			}
		default:
			return false
		}
	}
	return true
}

// decodeLiteralLit models the zsh backslash rules that the AST preserves in
// literal nodes. It intentionally decodes no expansions: every unmodeled part
// remains Unsupported at the caller.
func decodeLiteralLit(out *strings.Builder, value string, context literalQuoteContext) bool {
	for i := 0; i < len(value); i++ {
		if value[i] != '\\' {
			out.WriteByte(value[i])
			continue
		}
		if i+1 == len(value) {
			return false
		}
		next := value[i+1]
		if context == literalDoubleQuoted && next != '$' && next != '`' && next != '"' && next != '\\' && next != '\n' {
			out.WriteByte('\\')
			out.WriteByte(next)
			i++
			continue
		}
		if next != '\n' {
			out.WriteByte(next)
		}
		i++
	}
	return true
}

// mvdan/sh's zsh variant retains some extended-glob spellings inside Lit
// nodes. They are still expansion syntax at runtime, so fail closed instead of
// treating them as inert data. Quoted Lit nodes bypass this check.
func containsExtglobSyntax(s string) bool {
	for i := 0; i+1 < len(s); i++ {
		if s[i+1] != '(' {
			continue
		}
		switch s[i] {
		case '@', '+', '*', '?', '!':
			return true
		}
	}
	return false
}

func containsUnquotedExtglob(parts []syntax.WordPart) bool {
	var run strings.Builder
	flush := func() bool {
		unsupported := containsExtglobSyntax(run.String())
		run.Reset()
		return unsupported
	}
	for _, part := range parts {
		if lit, ok := part.(*syntax.Lit); ok {
			run.WriteString(lit.Value)
			continue
		}
		if flush() {
			return true
		}
	}
	return flush()
}

// captureFunctionBody returns assignment-ready body text. Only a plain,
// unmodified brace block loses its outer braces; all other statement forms keep
// their complete source span so redirects, parentheses, and keywords survive.
func captureFunctionBody(body *syntax.Stmt, src []byte) *string {
	if body == nil || body.Cmd == nil {
		return nil
	}
	if block, ok := body.Cmd.(*syntax.Block); ok && plainFunctionBlock(body) {
		value := sliceSrc(src, block.Lbrace.Offset()+1, block.Rbrace.Offset())
		return &value
	}
	value := sliceSrc(src, body.Pos().Offset(), body.End().Offset())
	return &value
}

func plainFunctionBlock(body *syntax.Stmt) bool {
	return len(body.Redirs) == 0 &&
		!body.Negated && !body.Background && !body.Coprocess && !body.Disown
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
