package zsh

import (
	"strings"
	"testing"

	"zsh-pro/core/model"
)

func TestParseClassifiesKinds(t *testing.T) {
	src := []byte(`# my aliases
alias gs='git status'
export EDITOR=nvim
PATH="$HOME/bin:$PATH"
greet() { echo hi }
setopt AUTO_CD
if [[ "$OSTYPE" == darwin* ]]; then
  alias ls='ls -G'
fi
`)
	blocks, err := (Provider{}).Parse(src)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(blocks) != 6 {
		t.Fatalf("got %d blocks, want 6", len(blocks))
	}

	cases := []struct {
		idx     int
		kind    model.BlockKind
		cmdName string
		name    string
	}{
		{0, model.KindAlias, "alias", "gs"},
		{1, model.KindAssignment, "export", "EDITOR"},
		{2, model.KindAssignment, "", "PATH"},
		{3, model.KindFuncDecl, "", "greet"},
		{4, model.KindCommand, "setopt", ""},
		{5, model.KindCompound, "", ""},
	}
	for _, c := range cases {
		b := blocks[c.idx]
		if b.Kind != c.kind {
			t.Errorf("block %d: kind %q, want %q", c.idx, b.Kind, c.kind)
		}
		if c.cmdName != "" && b.CmdName != c.cmdName {
			t.Errorf("block %d: cmdName %q, want %q", c.idx, b.CmdName, c.cmdName)
		}
		if c.name != "" && (len(b.Names) == 0 || b.Names[0] != c.name) {
			t.Errorf("block %d: names %v, want first=%q", c.idx, b.Names, c.name)
		}
	}
	// The leading comment is pulled into the alias block's Text for context, but
	// StartLine is the statement's own line (line 2), not the comment line (1).
	if blocks[0].StartLine != 2 {
		t.Errorf("alias block StartLine = %d, want 2 (statement line, not leading-comment line)", blocks[0].StartLine)
	}
}

// TestParseCapturesAppendAndFlags pins the faithfulness signals the router uses
// to keep non-byte-reversible shapes out of the managed/templated path:
//   - `+=` append assignments set Block.Append (WR-01) so they are not rewritten
//     to a plain `=`.
//   - `alias -g`/`-s` flagged aliases set Block.Flagged (WR-02) so the flag is
//     never dropped by the regenerator.
//   - multi-name assignment/alias statements keep every name in Names (BL-02) so
//     the router can detect len(Names) != 1 and route imperative rather than
//     pairing the first name with the last value.
func TestParseCapturesAppendAndFlags(t *testing.T) {
	cases := []struct {
		name      string
		src       string
		wantKind  model.BlockKind
		append    bool
		flagged   bool
		wantNames []string
	}{
		{"plain append", "FOO+=bar\n", model.KindAssignment, true, false, []string{"FOO"}},
		{"export append", "export PATH+=:/x\n", model.KindAssignment, true, false, []string{"PATH"}},
		{"global alias flagged", "alias -g G='| grep'\n", model.KindAlias, false, true, nil},
		{"suffix alias flagged", "alias -s txt=cat\n", model.KindAlias, false, true, nil},
		{"multi-name export keeps all names", "export FOO=bar BAZ=qux\n", model.KindAssignment, false, false, []string{"FOO", "BAZ"}},
		{"multi-name assignment keeps all names", "FOO=bar BAZ=qux\n", model.KindAssignment, false, false, []string{"FOO", "BAZ"}},
		{"multi-name alias keeps all names", "alias a=1 b=2\n", model.KindAlias, false, false, []string{"a", "b"}},
		{"plain assignment not append", "FOO=bar\n", model.KindAssignment, false, false, []string{"FOO"}},
		{"plain alias not flagged", "alias gs='git status'\n", model.KindAlias, false, false, []string{"gs"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			blocks, err := (Provider{}).Parse([]byte(c.src))
			if err != nil {
				t.Fatalf("Parse error: %v", err)
			}
			if len(blocks) != 1 {
				t.Fatalf("got %d blocks, want 1", len(blocks))
			}
			b := blocks[0]
			if b.Kind != c.wantKind {
				t.Errorf("kind = %q, want %q", b.Kind, c.wantKind)
			}
			if b.Append != c.append {
				t.Errorf("Append = %v, want %v", b.Append, c.append)
			}
			if b.Flagged != c.flagged {
				t.Errorf("Flagged = %v, want %v", b.Flagged, c.flagged)
			}
			if c.wantNames != nil {
				if len(b.Names) != len(c.wantNames) {
					t.Fatalf("Names = %v, want %v", b.Names, c.wantNames)
				}
				for i, n := range c.wantNames {
					if b.Names[i] != n {
						t.Errorf("Names[%d] = %q, want %q", i, b.Names[i], n)
					}
				}
			}
		})
	}
}

// TestParseCapturesArrayAssignment pins the UAT array gap: an array assignment
// (`name=(...)`) must be DETECTED at parse time via Block.Array. mvdan/sh models
// it as a.Array (*ArrayExpr) with a.Value == nil, so the scalar Value capture is
// skipped and b.Value stays empty. Array must be set in all three Assigns loops
// (plain `=`, export/typeset CallExpr, DeclClause) so the router can route it
// imperative and the full `(...)` span survives via verbatim Text — same remedy
// class as multi-name (BL-02) / `+=` (WR-01) / flagged aliases (WR-02). Scalar
// assignments keep Array == false (additive, no scalar-path regression).
func TestParseCapturesArrayAssignment(t *testing.T) {
	cases := []struct {
		name        string
		src         string
		wantArray   bool
		wantExport  bool
		wantDynamic bool
		wantAppend  bool
		wantNames   []string
		textParts   []string // substrings that must survive in the verbatim Text
	}{
		{
			name:      "single-line array (plugins)",
			src:       "plugins=(git zsh-autosuggestions zsh-syntax-highlighting)\n",
			wantArray: true,
			wantNames: []string{"plugins"},
			textParts: []string{"(git zsh-autosuggestions zsh-syntax-highlighting)"},
		},
		{
			name:      "multi-line array",
			src:       "arr=(\n  a\n  b\n)\n",
			wantArray: true,
			wantNames: []string{"arr"},
			textParts: []string{"a", "b", ")"},
		},
		{
			name:       "exported array",
			src:        "export ARR=(x y)\n",
			wantArray:  true,
			wantExport: true,
			wantNames:  []string{"ARR"},
			textParts:  []string{"(x y)"},
		},
		{
			// typeset -a may parse as a CallExpr or a DeclClause depending on the
			// local mvdan/sh; both loops set Array, so the assertion holds either way.
			name:      "typeset array (DeclClause path)",
			src:       "typeset -a tarr=(p q)\n",
			wantArray: true,
			wantNames: []string{"tarr"},
			textParts: []string{"(p q)"},
		},
		// Regression: Array must stay false for every scalar shape.
		{
			name:      "scalar assignment not array",
			src:       "FOO=bar\n",
			wantArray: false,
			wantNames: []string{"FOO"},
		},
		{
			name:        "dynamic scalar export not array",
			src:         "export FOO=$HOME/x\n",
			wantArray:   false,
			wantExport:  true,
			wantDynamic: true,
			wantNames:   []string{"FOO"},
		},
		{
			name:       "append scalar not array",
			src:        "FOO+=:/x\n",
			wantArray:  false,
			wantAppend: true,
			wantNames:  []string{"FOO"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			blocks, err := (Provider{}).Parse([]byte(c.src))
			if err != nil {
				t.Fatalf("Parse error: %v", err)
			}
			if len(blocks) != 1 {
				t.Fatalf("got %d blocks, want 1", len(blocks))
			}
			b := blocks[0]
			if b.Kind != model.KindAssignment {
				t.Errorf("Kind = %q, want %q", b.Kind, model.KindAssignment)
			}
			if b.Array != c.wantArray {
				t.Errorf("Array = %v, want %v", b.Array, c.wantArray)
			}
			if b.Exported != c.wantExport {
				t.Errorf("Exported = %v, want %v", b.Exported, c.wantExport)
			}
			if b.Dynamic != c.wantDynamic {
				t.Errorf("Dynamic = %v, want %v", b.Dynamic, c.wantDynamic)
			}
			if b.Append != c.wantAppend {
				t.Errorf("Append = %v, want %v", b.Append, c.wantAppend)
			}
			if len(b.Names) != len(c.wantNames) {
				t.Fatalf("Names = %v, want %v", b.Names, c.wantNames)
			}
			for i, n := range c.wantNames {
				if b.Names[i] != n {
					t.Errorf("Names[%d] = %q, want %q", i, b.Names[i], n)
				}
			}
			for _, part := range c.textParts {
				if !strings.Contains(b.Text, part) {
					t.Errorf("Text %q does not contain %q (verbatim array span not preserved)", b.Text, part)
				}
			}
		})
	}
}

func TestParseOpaqueOnUnparseable(t *testing.T) {
	// A hard syntax error must not crash; it yields one opaque block.
	blocks, err := (Provider{}).Parse([]byte("if then fi fi ;;"))
	if err != nil {
		t.Fatalf("Parse should not error on bad input, got %v", err)
	}
	if len(blocks) != 1 || !blocks[0].Opaque {
		t.Fatalf("want one opaque block, got %+v", blocks)
	}
}

// TestParseCapturesFunctionBody pins assignment-ready body extraction without
// changing Value's full-declaration regeneration contract.
func TestParseCapturesFunctionBody(t *testing.T) {
	cases := []struct {
		name     string
		src      string
		wantBody string
	}{
		{name: "empty block", src: `foo(){}`, wantBody: ""},
		{
			name: "multiline plain block",
			src: `foo() {
  print one
  # braces in comments stay data: { }
  if true; then
    print '}'
  fi
}`,
			wantBody: "\n  print one\n  # braces in comments stay data: { }\n  if true; then\n    print '}'\n  fi\n",
		},
		{name: "subshell body", src: `foo() ( print hi )`, wantBody: `( print hi )`},
		{name: "redirected block body", src: `foo() { print hi; } >file`, wantBody: `{ print hi; } >file`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			blocks, err := (Provider{}).Parse([]byte(tc.src))
			if err != nil {
				t.Fatalf("Parse error: %v", err)
			}
			if len(blocks) != 1 {
				t.Fatalf("got %d blocks, want 1", len(blocks))
			}
			got := blocks[0]
			if got.Kind != model.KindFuncDecl {
				t.Fatalf("Kind = %q, want func", got.Kind)
			}
			if got.Value != tc.src {
				t.Errorf("Value = %q, want full declaration %q", got.Value, tc.src)
			}
			if got.FunctionBody == nil {
				t.Fatal("FunctionBody = nil, want present")
			}
			if *got.FunctionBody != tc.wantBody {
				t.Errorf("FunctionBody = %q, want %q", *got.FunctionBody, tc.wantBody)
			}
		})
	}
}

func TestParseListValueSegments(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want []model.ListSegment
	}{
		{
			name: "path dynamic then self",
			src:  `PATH="$HOME/bin:$PATH"`,
			want: []model.ListSegment{{Dynamic: true, Source: "$HOME/bin"}, {Self: true}},
		},
		{
			name: "fpath braced dynamic then self",
			src:  `FPATH="${HOME}/zfunc:$FPATH"`,
			want: []model.ListSegment{{Dynamic: true, Source: "${HOME}/zfunc"}, {Self: true}},
		},
		{
			name: "ordered mixed quoted additions",
			src:  `PATH='/a b':$PATH:"/c d"`,
			want: []model.ListSegment{{Value: "/a b"}, {Self: true}, {Value: "/c d"}},
		},
		{
			name: "literal then self",
			src:  `PATH=/a:$PATH`,
			want: []model.ListSegment{{Value: "/a"}, {Self: true}},
		},
		{
			name: "self then literal",
			src:  `PATH=$PATH:/b`,
			want: []model.ListSegment{{Self: true}, {Value: "/b"}},
		},
		{
			name: "quoted and escaped delimiters",
			src:  `PATH=':/quoted':/escaped\:part:$PATH`,
			want: []model.ListSegment{{Value: ""}, {Value: "/quoted"}, {Value: "/escaped"}, {Value: "part"}, {Self: true}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			blocks, err := (Provider{}).Parse([]byte(tc.src))
			if err != nil || len(blocks) != 1 {
				t.Fatalf("blocks=%#v err=%v", blocks, err)
			}
			got := blocks[0]
			if got.ListValue == nil {
				t.Fatal("ListValue = nil")
			}
			if len(got.ListValue.Segments) != len(tc.want) {
				t.Fatalf("segments=%#v want=%#v", got.ListValue.Segments, tc.want)
			}
			for i := range tc.want {
				if got.ListValue.Segments[i] != tc.want[i] {
					t.Fatalf("segment %d = %#v, want %#v", i, got.ListValue.Segments[i], tc.want[i])
				}
			}
			if got.Value != strings.TrimPrefix(tc.src, "PATH=") && got.Value != strings.TrimPrefix(tc.src, "FPATH=") {
				t.Fatalf("Value changed to %q", got.Value)
			}
		})
	}
}

func TestParseListValueRejectsAmbiguousBase(t *testing.T) {
	for _, src := range []string{
		`PATH=$FPATH:/a`, `PATH=$fpath:/a`, `FPATH=$PATH:/a`, `FPATH=$path:/a`,
		`PATH=/a`, `PATH=$PATH:$PATH`, `PATH=$(printf /a):$PATH`, `PATH=${PATH:-/a}`,
	} {
		t.Run(src, func(t *testing.T) {
			blocks, err := (Provider{}).Parse([]byte(src))
			if err != nil || len(blocks) != 1 {
				t.Fatalf("blocks=%#v err=%v", blocks, err)
			}
			if blocks[0].ListValue != nil {
				t.Fatalf("ListValue = %#v, want nil", blocks[0].ListValue)
			}
		})
	}
}
