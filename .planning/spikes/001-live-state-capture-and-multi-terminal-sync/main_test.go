package main

import (
	"bytes"
	"os/exec"
	"strings"
	"testing"
)

func TestParseSnapshotPreservesFramedValuesAndExclusions(t *testing.T) {
	var input bytes.Buffer
	fields := []string{
		"entry", "env", "SPIKE_MULTILINE", "alpha\nbeta",
		"entry", "function", "spike_quote", "builtin print -r -- \"a'b\"",
		"array", "PATH", "3", "/bin", "/tmp/space path", "",
		"exclude", "env", "SPIKE_API_TOKEN", "secret-name",
	}
	for _, field := range fields {
		input.WriteString(field)
		input.WriteByte(0)
	}

	entries, exclusions, err := parseSnapshot(&input)
	if err != nil {
		t.Fatal(err)
	}
	if got := entries["env\x1fSPIKE_MULTILINE"].Value; got != "alpha\nbeta" {
		t.Fatalf("multiline value = %q", got)
	}
	if got := entries["array\x1fPATH"].Values; len(got) != 3 || got[1] != "/tmp/space path" || got[2] != "" {
		t.Fatalf("PATH values = %#v", got)
	}
	if len(exclusions) != 1 || exclusions[0].Name != "SPIKE_API_TOKEN" || exclusions[0].Reason != "secret-name" {
		t.Fatalf("exclusions = %#v", exclusions)
	}
}

func TestParseSnapshotRejectsMalformedAndOversizedInput(t *testing.T) {
	for _, input := range [][]byte{
		[]byte("not-framed"),
		append([]byte("unknown\x00"), 0),
		bytes.Repeat([]byte{'x'}, maxSnapshotBytes+1),
	} {
		if _, _, err := parseSnapshot(bytes.NewReader(input)); err == nil {
			t.Fatalf("accepted malformed snapshot of %d bytes", len(input))
		}
	}
}

func TestDiffEntriesIsPerIdentityAndRepresentsRemoval(t *testing.T) {
	before := map[string]entry{
		"env\x1fSPIKE_KEEP":   {Kind: "env", Name: "SPIKE_KEEP", Present: true, Value: "same"},
		"env\x1fSPIKE_CHANGE": {Kind: "env", Name: "SPIKE_CHANGE", Present: true, Value: "old"},
		"alias\x1fspike_gone": {Kind: "alias", Name: "spike_gone", Present: true, Value: "old"},
	}
	after := map[string]entry{
		"env\x1fSPIKE_KEEP":     {Kind: "env", Name: "SPIKE_KEEP", Present: true, Value: "same"},
		"env\x1fSPIKE_CHANGE":   {Kind: "env", Name: "SPIKE_CHANGE", Present: true, Value: "new"},
		"function\x1fspike_new": {Kind: "function", Name: "spike_new", Present: true, Value: "builtin true"},
	}

	delta := diffEntries(before, after)
	if len(delta) != 3 {
		t.Fatalf("delta keys = %#v", delta)
	}
	if delta["alias\x1fspike_gone"].Present {
		t.Fatal("removal was not represented explicitly")
	}
	if _, ok := delta["env\x1fSPIKE_KEEP"]; ok {
		t.Fatal("unchanged identity appeared in delta")
	}
}

func TestRenderedPatchIsParseableAndPreservesSemanticValues(t *testing.T) {
	delta := map[string]entry{}
	for _, e := range []entry{
		{Kind: "env", Name: "SPIKE_MULTILINE", Present: true, Value: "alpha\nbeta'\\tail"},
		{Kind: "alias", Name: "spike_hi", Present: true, Value: "builtin print -r -- alias-ok"},
		{Kind: "function", Name: "spike_fn", Present: true, Value: "builtin print -r -- function-ok"},
		{Kind: "array", Name: "PATH", Present: true, Values: []string{"/tmp/space path", "/bin", ""}},
		{Kind: "option", Name: "autocd", Present: true, Value: "on"},
	} {
		delta[e.key()] = e
	}
	patch, err := renderPatch(delta)
	if err != nil {
		t.Fatal(err)
	}
	script := "zmodload zsh/parameter\n" + patch + `
[[ $SPIKE_MULTILINE == $'alpha\nbeta\'\\tail' ]] || exit 10
[[ ${aliases[spike_hi]} == 'builtin print -r -- alias-ok' ]] || exit 11
[[ ${functions[spike_fn]} == *function-ok* ]] || exit 12
[[ $path[1] == '/tmp/space path' && $path[2] == /bin && $path[3] == '' ]] || exit 13
[[ ${options[autocd]} == on ]] || exit 14
spike_fn
`
	cmd := exec.Command("zsh", "-f", "-c", script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("rendered patch failed: %v\n%s\npatch:\n%s", err, out, patch)
	}
	if strings.TrimSpace(string(out)) != "function-ok" {
		t.Fatalf("function output = %q", out)
	}
}

func TestRevisionHistoryIsCappedAndHistoryGapUsesAuthoritativeState(t *testing.T) {
	state := sharedState{Entries: map[string]entry{
		"env\x1fSPIKE_SHARED": {Kind: "env", Name: "SPIKE_SHARED", Present: true, Value: "head"},
	}}
	for revision := uint64(1); revision <= maxRevisionEvents+3; revision++ {
		state.Events = append(state.Events, revisionEvent{Revision: revision})
		state.Revision = revision
	}
	compactRevisionHistory(&state)
	if len(state.Events) != maxRevisionEvents || state.CompactedThrough != 3 || state.Events[0].Revision != 4 {
		t.Fatalf("compaction = floor %d, len %d, first %d", state.CompactedThrough, len(state.Events), state.Events[0].Revision)
	}

	snapshot := map[string]entry{
		"env\x1fSPIKE_LOCAL_ONLY": {Kind: "env", Name: "SPIKE_LOCAL_ONLY", Present: true, Value: "stale"},
	}
	patch := finalChangesForShell(state, 2, snapshot)
	if patch["env\x1fSPIKE_SHARED"].Value != "head" {
		t.Fatal("history-gap reconciliation omitted authoritative shared value")
	}
	if patch["env\x1fSPIKE_LOCAL_ONLY"].Present {
		t.Fatal("history-gap reconciliation did not explicitly remove stale local-only value")
	}
}
