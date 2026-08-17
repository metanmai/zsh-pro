package worktree

import (
	"reflect"
	"testing"
	"zsh-pro/core/model"
)

func liveState(kind model.LiveKind, name string, value model.LiveValue) model.LiveIdentityState {
	return model.LiveIdentityState{Identity: model.Identity{Kind: kind, Name: name}, Value: value}
}

func TestValidateSnapshotUsesModelBoundsAndRejectsMalformedValues(t *testing.T) {
	if err := ValidateSnapshot(model.LiveSnapshot{ByteSize: model.MaxSnapshotBytes}); err != nil {
		t.Fatalf("exact byte cap rejected: %v", err)
	}
	if err := ValidateSnapshot(model.LiveSnapshot{ByteSize: model.MaxSnapshotBytes + 1}); err == nil {
		t.Fatal("byte cap plus one accepted")
	}
	bad := model.LiveSnapshot{States: []model.LiveIdentityState{
		liveState(model.LiveEnv, "GOOD", model.ScalarLiveValue("ok")),
		liveState(model.LivePath, "PATH", model.ScalarLiveValue("/not-a-list")),
	}}
	if err := ValidateSnapshot(bad); err == nil {
		t.Fatal("kind/value mismatch accepted")
	}
}

func TestDiffSnapshotFinalOccurrenceSemanticEqualityAndOrdering(t *testing.T) {
	before := model.LiveSnapshot{States: []model.LiveIdentityState{
		liveState(model.LiveEnv, "A", model.ScalarLiveValue("shadowed")),
		liveState(model.LiveEnv, "B", model.ScalarLiveValue("removed")),
		liveState(model.LiveAlias, "zz", model.ScalarLiveValue("ls")),
		liveState(model.LiveFunction, "fn", model.ScalarLiveValue("print old\n")),
		liveState(model.LivePath, "PATH", model.ListLiveValue([]string{"/a", "", "/a"})),
		liveState(model.LiveFPath, "FPATH", model.ListLiveValue([]string{})),
		liveState(model.LiveOption, "NO_BEEP", model.OptionLiveValue(false)),
		liveState(model.LiveEnv, "A", model.ScalarLiveValue("same")),
	}}
	after := model.LiveSnapshot{States: []model.LiveIdentityState{
		liveState(model.LiveOption, "NO_BEEP", model.OptionLiveValue(true)),
		liveState(model.LivePath, "PATH", model.ListLiveValue([]string{"/a", "/a", ""})),
		liveState(model.LiveEnv, "C", model.ScalarLiveValue("added")),
		liveState(model.LiveFunction, "fn", model.ScalarLiveValue("print new\n")),
		liveState(model.LiveFPath, "FPATH", model.ListLiveValue(nil)),
		liveState(model.LiveEnv, "A", model.ScalarLiveValue("same")),
	}}

	// Pin the shared authority directly before exercising its consumer.
	if !model.EqualLiveValue(model.LiveFPath, before.States[5].Value, after.States[4].Value) {
		t.Fatal("model semantic authority does not equate empty list representations")
	}
	normalized, err := model.NormalizeLiveStates(before.States)
	if err != nil || len(normalized) != 7 || normalized[6].Identity.Name != "A" {
		t.Fatalf("model final-occurrence authority failed: %#v, %v", normalized, err)
	}

	changes, err := DiffSnapshot(before, after)
	if err != nil {
		t.Fatal(err)
	}
	want := []model.LiveChange{
		{Kind: model.LiveRemove, Identity: model.Identity{Kind: model.LiveEnv, Name: "B"}, Value: model.RemovedLiveValue()},
		{Kind: model.LiveAdd, Identity: model.Identity{Kind: model.LiveEnv, Name: "C"}, Value: model.ScalarLiveValue("added")},
		{Kind: model.LiveRemove, Identity: model.Identity{Kind: model.LiveAlias, Name: "zz"}, Value: model.RemovedLiveValue()},
		{Kind: model.LiveUpdate, Identity: model.Identity{Kind: model.LiveFunction, Name: "fn"}, Value: model.ScalarLiveValue("print new\n")},
		{Kind: model.LiveUpdate, Identity: model.Identity{Kind: model.LivePath, Name: "PATH"}, Value: model.ListLiveValue([]string{"/a", "/a", ""})},
		{Kind: model.LiveUpdate, Identity: model.Identity{Kind: model.LiveOption, Name: "NO_BEEP"}, Value: model.OptionLiveValue(true)},
	}
	if !reflect.DeepEqual(changes, want) {
		t.Fatalf("DiffSnapshot() = %#v, want %#v", changes, want)
	}
}

func TestDiffInvalidPairReturnsNoPartialChanges(t *testing.T) {
	before := model.LiveSnapshot{States: []model.LiveIdentityState{
		liveState(model.LiveEnv, "A", model.ScalarLiveValue("old")),
	}}
	after := model.LiveSnapshot{States: []model.LiveIdentityState{
		liveState(model.LiveEnv, "A", model.ScalarLiveValue("new")),
		liveState(model.LiveKind("history"), "HISTFILE", model.ScalarLiveValue("secret")),
	}}
	changes, err := DiffSnapshot(before, after)
	if err == nil || changes != nil {
		t.Fatalf("invalid pair returned partial output: %#v, %v", changes, err)
	}
}

func TestOverlayPreservesSourceOrderAndTombstonesShadowedOccurrences(t *testing.T) {
	base := []model.LiveIdentityState{
		liveState(model.LiveEnv, "A", model.ScalarLiveValue("old")),
		liveState(model.LiveEnv, "B", model.ScalarLiveValue("keep-position")),
		liveState(model.LiveAlias, "ll", model.ScalarLiveValue("ls")),
		liveState(model.LiveEnv, "A", model.ScalarLiveValue("final-source")),
	}
	overlay := []model.OverlayEntry{
		{Identity: model.Identity{Kind: model.LiveEnv, Name: "A"}, Value: model.ScalarLiveValue("ignored")},
		{Identity: model.Identity{Kind: model.LiveEnv, Name: "B"}, Value: model.ScalarLiveValue("updated")},
		{Identity: model.Identity{Kind: model.LiveEnv, Name: "C"}, Value: model.ScalarLiveValue("new")},
		{Identity: model.Identity{Kind: model.LiveEnv, Name: "A"}, Tombstone: true},
	}

	got, err := ApplyOverlay(base, overlay)
	if err != nil {
		t.Fatal(err)
	}
	want := []model.LiveIdentityState{
		liveState(model.LiveEnv, "B", model.ScalarLiveValue("updated")),
		liveState(model.LiveAlias, "ll", model.ScalarLiveValue("ls")),
		liveState(model.LiveEnv, "C", model.ScalarLiveValue("new")),
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ApplyOverlay() = %#v, want %#v", got, want)
	}

	*overlay[1].Value.Scalar = "caller-mutated"
	if *got[0].Value.Scalar != "updated" {
		t.Fatal("overlay output aliases caller value")
	}
	for _, state := range got {
		if state.Identity.Name == "A" {
			t.Fatal("tombstone exposed an earlier repeated source assignment")
		}
	}
}

func TestProjectionRetainsFinalTombstonesAndRejectsPartialOutput(t *testing.T) {
	base := []model.LiveIdentityState{
		liveState(model.LiveEnv, "A", model.ScalarLiveValue("old")),
		liveState(model.LiveAlias, "ll", model.ScalarLiveValue("ls")),
	}
	overlay := []model.OverlayEntry{
		{Identity: model.Identity{Kind: model.LiveEnv, Name: "A"}, Tombstone: true},
		{Identity: model.Identity{Kind: model.LiveFunction, Name: "f"}, Value: model.ScalarLiveValue("")},
	}
	projection, err := BuildProjection(base, overlay)
	if err != nil {
		t.Fatal(err)
	}
	if projection.Schema != model.WorktreeSchemaV1 || len(projection.States) != 2 || len(projection.Tombstones) != 1 || projection.Tombstones[0].Name != "A" {
		t.Fatalf("projection lost state/tombstone distinction: %#v", projection)
	}
	*overlay[1].Value.Scalar = "mutated"
	if *projection.States[1].Value.Scalar != "" {
		t.Fatal("projection aliases overlay value")
	}

	invalid := append(append([]model.OverlayEntry(nil), overlay...), model.OverlayEntry{
		Identity: model.Identity{Kind: model.LiveOption, Name: "BAD-OPTION"},
		Value:    model.OptionLiveValue(true),
	})
	failed, err := BuildProjection(base, invalid)
	if err == nil || !reflect.DeepEqual(failed, model.LiveProjection{}) {
		t.Fatalf("invalid projection returned partial output: %#v, %v", failed, err)
	}
}

func TestCategorizeDiffStableKindAndExactNameOrdering(t *testing.T) {
	changes := []model.LiveChange{
		{Kind: model.LiveUpdate, Identity: model.Identity{Kind: model.LiveOption, Name: "Z_OPT"}, Value: model.OptionLiveValue(true)},
		{Kind: model.LiveAdd, Identity: model.Identity{Kind: model.LiveEnv, Name: "ZED"}, Value: model.ScalarLiveValue("z")},
		{Kind: model.LiveRemove, Identity: model.Identity{Kind: model.LiveFPath, Name: "FPATH"}, Value: model.RemovedLiveValue()},
		{Kind: model.LiveAdd, Identity: model.Identity{Kind: model.LiveEnv, Name: "ALPHA"}, Value: model.ScalarLiveValue("a")},
		{Kind: model.LiveUpdate, Identity: model.Identity{Kind: model.LivePath, Name: "PATH"}, Value: model.ListLiveValue([]string{"/bin"})},
		{Kind: model.LiveAdd, Identity: model.Identity{Kind: model.LiveAlias, Name: "g"}, Value: model.ScalarLiveValue("git")},
		{Kind: model.LiveAdd, Identity: model.Identity{Kind: model.LiveFunction, Name: "f"}, Value: model.ScalarLiveValue("print f")},
	}

	got, err := CategorizeDiff(changes)
	if err != nil {
		t.Fatal(err)
	}
	if names := diffNames(got.Environment); !reflect.DeepEqual(names, []string{"ALPHA", "ZED"}) {
		t.Fatalf("environment order = %#v", names)
	}
	if len(got.Aliases) != 1 || len(got.Functions) != 1 || len(got.Path) != 1 || len(got.FPath) != 1 || len(got.Options) != 1 {
		t.Fatalf("categorized diff lost a category: %#v", got)
	}
	if reflect.TypeOf(got.Environment[0]).NumField() != 2 {
		t.Fatalf("public diff entry unexpectedly widened: %#v", got.Environment[0])
	}
}

func TestOverlayInvalidInputReturnsNoPartialState(t *testing.T) {
	base := []model.LiveIdentityState{liveState(model.LiveEnv, "A", model.ScalarLiveValue("old"))}
	overlay := []model.OverlayEntry{
		{Identity: model.Identity{Kind: model.LiveEnv, Name: "A"}, Value: model.ScalarLiveValue("new")},
		{Identity: model.Identity{Kind: model.LivePath, Name: "PATH"}, Value: model.ScalarLiveValue("wrong")},
	}
	got, err := ApplyOverlay(base, overlay)
	if err == nil || got != nil {
		t.Fatalf("invalid overlay returned partial state: %#v, %v", got, err)
	}
}

func diffNames(entries []model.DiffEntry) []string {
	names := make([]string, len(entries))
	for index, entry := range entries {
		names[index] = entry.Identity.Name
	}
	return names
}
