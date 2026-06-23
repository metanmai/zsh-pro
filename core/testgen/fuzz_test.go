package testgen_test

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"testing"

	"zsh-pro/core/analyze"
	"zsh-pro/core/render"
	"zsh-pro/core/shell/zsh"
	"zsh-pro/core/testgen"
)

// fuzzIters is a deliberate, logged bound — each iteration runs the full engine
// (which spawns `zsh -f`), so we keep it modest rather than silently capping a
// huge run.
const fuzzIters = 40

// TestFuzzSurvival corrupts a generated config and asserts the engine survives:
// it never panics and the JSON renderer always emits valid JSON. No oracle —
// corrupted output is unpredictable, so we assert only robustness.
func TestFuzzSurvival(t *testing.T) {
	t.Logf("fuzzing %d corrupted configs (bounded: each runs the full engine / zsh -f)", fuzzIters)
	for i := range fuzzIters {
		seed := int64(1000 + i)
		rng := rand.New(rand.NewSource(seed))
		g := testgen.New(rng).Build(propParams())
		corrupt := testgen.NewMutator(rng).Corrupt(g.RenderZsh())

		out, ok := analyzeNoPanic(corrupt, fmt.Sprintf("fuzz-%d.zsh", seed))
		if !ok {
			t.Fatalf("analyze panicked on seed %d\n%s", seed, corrupt)
		}
		if !json.Valid(out) {
			t.Fatalf("JSON renderer emitted invalid JSON on seed %d\n%s\n--- json ---\n%s", seed, corrupt, out)
		}
	}
}

// analyzeNoPanic runs the full engine + JSON render, recovering any panic so the
// test can report it as a robustness failure rather than crashing the run.
func analyzeNoPanic(src []byte, path string) (out []byte, ok bool) {
	defer func() {
		if r := recover(); r != nil {
			ok = false
		}
	}()
	a := analyze.New(zsh.Provider{}).Analyze(src, path)
	b, err := render.JSONRenderer{}.Render(a)
	if err != nil {
		return nil, false
	}
	return b, true
}
