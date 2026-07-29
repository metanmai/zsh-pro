package main

import (
	"io"
	"math"
	"strings"
	"testing"
)

// addedStartupMilliseconds is deliberately wrong during the RED step. The green
// implementation replaces this test-local placeholder with the real decoder.
func addedStartupMilliseconds(_ io.Reader, _, _ string) (float64, error) { return 0, nil }

func TestAddedStartupMillisecondsDecodesWhitespaceAndResultOrder(t *testing.T) {
	results := `{
  "results" : [
    { "command" : "without-zsh-pro", "mean" : 0.100 },
    { "mean": 0.1075, "command" : "with-zsh-pro" }
  ]
}`

	got, err := addedStartupMilliseconds(strings.NewReader(results), "with-zsh-pro", "without-zsh-pro")
	if err != nil {
		t.Fatalf("addedStartupMilliseconds: %v", err)
	}
	if want := 7.5; math.Abs(got-want) > 0.000001 {
		t.Fatalf("added startup milliseconds = %v, want %v", got, want)
	}
}

func TestAddedStartupMillisecondsRejectsMissingNamedResult(t *testing.T) {
	results := `{"results":[{"command":"with-zsh-pro","mean":0.1075}]}`
	if _, err := addedStartupMilliseconds(strings.NewReader(results), "with-zsh-pro", "without-zsh-pro"); err == nil {
		t.Fatal("missing baseline result was accepted")
	}
}
