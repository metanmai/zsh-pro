package ir_test

// This is the Phase 2 correctness gate: the byte-identical round-trip oracle.
// It lives in the EXTERNAL test package (ir_test) on purpose — it is the only
// place that wires the concrete zsh.Provider to the agnostic core/ir, so the
// production ir package stays shell-free. It executes the ORIGINAL and the
// REGENERATED .zsh each under sandboxed `zsh -f` (no rc files, 5s timeout, via
// the reused Provider.Introspect instrument), snapshots the resolved
// model.IdentitySet, and asserts the two are equal across all 5 declarative
// classes (D-08). Skip-guarded for zsh-absent environments.

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"

	"zsh-pro/core/ir"
	"zsh-pro/core/shell/zsh"
)

// TestRegenRoundTrip proves the IR regenerates behavior-equivalent zsh: the
// runtime state after sourcing the regenerated config is byte-identical to the
// runtime state after sourcing the original, with dynamic values late-bound
// verbatim (never resolved against this machine).
func TestRegenRoundTrip(t *testing.T) {
	if _, err := exec.LookPath("zsh"); err != nil {
		t.Skip("zsh not installed; skipping round-trip oracle")
	}

	// Representative fixture covering ALL 5 declarative classes:
	//   static env, dynamic env, dynamic PATH, alias, option, function.
	// A real PATH assignment exercises the $path snapshot (#1). Deliberately NO
	// compinit/zstyle/autoload — keep the oracle deterministic (Pitfall 4).
	//
	// The trailing block is the ADVERSARIAL set (WR-04): the exact shapes that
	// previously panicked (BL-01) or silently corrupted config (BL-02/WR-01/WR-02)
	// when mis-templated. They now route IMPERATIVE (verbatim Text), so the oracle
	// proves they round-trip byte/behavior-faithfully rather than being downgraded.
	src := []byte("export EDITOR=nvim\n" +
		"export GOPATH=$HOME/go\n" +
		"export PATH=$HOME/bin:$PATH\n" +
		"alias gs='git status'\n" +
		"setopt EXTENDED_GLOB\n" +
		"greet() { echo hi }\n" +
		// Adversarial: multi-name assignment (BL-02) — must keep BOTH names+values.
		"export MULTA=one MULTB=two\n" +
		// Adversarial: multi-name alias (BL-02).
		"alias ma=1 mb=2\n" +
		// Adversarial: `+=` append (WR-01) — must stay an append, not an overwrite.
		"FPATH=/seed\n" +
		"FPATH+=:/extra\n" +
		// Adversarial: global alias flag (WR-02) — must stay global, not regular.
		"alias -g GG='| grep'\n")

	p := zsh.Provider{}
	blocks, err := p.Parse(src)
	if err != nil {
		// Parse is documented never to error today; assert it explicitly so a
		// future regression surfaces here instead of as a confusing downstream
		// introspection mismatch (WR-04).
		t.Fatalf("Parse returned an error (regression): %v", err)
	}
	profile := ir.Build(blocks, p)
	regen := ir.Regenerate(profile, p)

	dir := t.TempDir()
	origPath := filepath.Join(dir, "orig.zsh")
	regenPath := filepath.Join(dir, "regen.zsh")
	if err := os.WriteFile(origPath, src, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(regenPath, regen, 0o644); err != nil {
		t.Fatal(err)
	}

	// Separate error variables (#5): a second-call failure must never surface as
	// a confusing nil panic or shared-err ambiguity.
	origIDS, origErr := p.Introspect(origPath)
	regenIDS, regenErr := p.Introspect(regenPath)

	if origErr != nil || !origIDS.Available {
		t.Fatalf("introspect(original) failed: err=%v available=%v", origErr, origIDS.Available)
	}
	if regenErr != nil || !regenIDS.Available {
		t.Fatalf("introspect(regenerated) failed: err=%v available=%v\nregen=%q", regenErr, regenIDS.Available, regen)
	}

	// Byte-identical state tables across all 5 declarative classes (D-08).
	if !reflect.DeepEqual(origIDS.Aliases, regenIDS.Aliases) {
		t.Errorf("Aliases differ:\n orig=%v\n regen=%v\nsrc=%q\nregen=%q", origIDS.Aliases, regenIDS.Aliases, src, regen)
	}
	if !reflect.DeepEqual(origIDS.Functions, regenIDS.Functions) {
		t.Errorf("Functions differ:\n orig=%v\n regen=%v\nsrc=%q\nregen=%q", origIDS.Functions, regenIDS.Functions, src, regen)
	}
	if !reflect.DeepEqual(origIDS.Env, regenIDS.Env) {
		t.Errorf("Env differ:\n orig=%v\n regen=%v\nsrc=%q\nregen=%q", origIDS.Env, regenIDS.Env, src, regen)
	}
	if !reflect.DeepEqual(origIDS.Options, regenIDS.Options) {
		t.Errorf("Options differ:\n orig=%v\n regen=%v\nsrc=%q\nregen=%q", origIDS.Options, regenIDS.Options, src, regen)
	}
	if !reflect.DeepEqual(origIDS.Path, regenIDS.Path) {
		t.Errorf("Path differ:\n orig=%v\n regen=%v\nsrc=%q\nregen=%q", origIDS.Path, regenIDS.Path, src, regen)
	}

	// Class-presence sanity: the function and the option must actually be present
	// in BOTH snapshots (proves templated-function + templated-setopt regeneration,
	// not two empty-but-equal tables).
	if !origIDS.Functions["greet"] || !regenIDS.Functions["greet"] {
		t.Errorf("greet absent from a snapshot (templated-function regen not proven): orig=%v regen=%v", origIDS.Functions, regenIDS.Functions)
	}
	if !origIDS.Options["extendedglob"] || !regenIDS.Options["extendedglob"] {
		t.Errorf("extendedglob option absent from a snapshot (templated-setopt regen not proven): orig=%v regen=%v", origIDS.Options, regenIDS.Options)
	}

	// EVAL-01 SC3: GOPATH (driven by $HOME/go) present in both Env snapshots
	// (late-bound identically), and the regenerated bytes still carry the LITERAL
	// dynamic values — never the resolved absolute path.
	if !origIDS.Env["GOPATH"] || !regenIDS.Env["GOPATH"] {
		t.Errorf("GOPATH absent from a snapshot (dynamic env round-trip not proven): orig=%v regen=%v", origIDS.Env, regenIDS.Env)
	}
	if !bytes.Contains(regen, []byte("$HOME/go")) {
		t.Errorf("dynamic value resolved: regenerated bytes do not contain literal $HOME/go: %q", regen)
	}
	if !bytes.Contains(regen, []byte("$HOME/bin:$PATH")) {
		t.Errorf("dynamic PATH resolved: regenerated bytes do not contain literal $HOME/bin:$PATH: %q", regen)
	}

	// Adversarial-shape sanity (WR-04): the previously-broken shapes must survive
	// the round-trip intact. The DeepEqual checks above already enforce identical
	// resolved state; these assertions pin WHY (no corruption/downgrade) and prove
	// the shapes route imperative (verbatim Text), not mis-templated.
	for _, want := range []string{
		"MULTA", "MULTB", // BL-02: multi-name export keeps both names...
	} {
		if !origIDS.Env[want] || !regenIDS.Env[want] {
			t.Errorf("multi-name env %q absent from a snapshot (BL-02 corruption): orig=%v regen=%v", want, origIDS.Env, regenIDS.Env)
		}
	}
	// BL-02: multi-name alias keeps both names.
	if !origIDS.Aliases["ma"] || !regenIDS.Aliases["ma"] || !origIDS.Aliases["mb"] || !regenIDS.Aliases["mb"] {
		t.Errorf("multi-name alias dropped a name (BL-02): orig=%v regen=%v", origIDS.Aliases, regenIDS.Aliases)
	}
	// BL-02 + WR-01: the multi-name export and the += append must be regenerated
	// verbatim (not collapsed to a single name=last-value or rewritten to `=`).
	if !bytes.Contains(regen, []byte("export MULTA=one MULTB=two")) {
		t.Errorf("multi-name export not regenerated verbatim (BL-02): %q", regen)
	}
	if !bytes.Contains(regen, []byte("FPATH+=:/extra")) {
		t.Errorf("+= append rewritten to = (WR-01); regenerated bytes: %q", regen)
	}
	// WR-02: the global alias must stay global (verbatim `alias -g`), not downgraded.
	if !bytes.Contains(regen, []byte("alias -g GG=")) {
		t.Errorf("global alias flag dropped (WR-02); regenerated bytes: %q", regen)
	}
}
