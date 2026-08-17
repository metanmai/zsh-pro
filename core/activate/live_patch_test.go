package activate_test

import (
	"reflect"
	"testing"

	"zsh-pro/core/activate"
	"zsh-pro/core/model"
	"zsh-pro/core/worktree"
)

func stringPointer(value string) *string { return &value }

func state(kind model.LiveKind, name string, value model.LiveValue) model.LiveIdentityState {
	return model.LiveIdentityState{Identity: model.Identity{Kind: kind, Name: name}, Value: value}
}

func identity(kind model.LiveKind, name string) model.Identity {
	return model.Identity{Kind: kind, Name: name}
}

func literalEntry(category model.Category, kind model.BlockKind, name, value string) model.Entry {
	return model.Entry{
		Text:                    name + "=" + value,
		Category:                category,
		Kind:                    kind,
		Names:                   []string{name},
		Managed:                 true,
		StructuralFidelityKnown: true,
		ValueMode:               model.ValueModeLiteral,
		RuntimeValue:            stringPointer(value),
	}
}

func TestBuildEffectivePreservesCompleteSourceAndProjection(t *testing.T) {
	empty := ""
	body := "print -r -- one\nprint -r -- two"
	profile := model.Profile{Entries: []model.Entry{
		literalEntry(model.CatEnvironment, model.KindAssignment, "KEEP", "old"),
		literalEntry(model.CatEnvironment, model.KindAssignment, "DROP", "shadowed"),
		literalEntry(model.CatEnvironment, model.KindAssignment, "DROP", "final"),
		{Text: "export EMPTY=''", Category: model.CatEnvironment, Kind: model.KindAssignment, Names: []string{"EMPTY"}, Exported: true, Managed: true, StructuralFidelityKnown: true, ValueMode: model.ValueModeLiteral, RuntimeValue: &empty},
		{Text: "alias ll='ls -la'", Category: model.CatAliases, Kind: model.KindAlias, Names: []string{"ll"}, Managed: true, StructuralFidelityKnown: true, AliasAssignment: true, ValueMode: model.ValueModeLiteral, RuntimeValue: stringPointer("ls -la")},
		{Text: "demo() { ... }", Category: model.CatFunctions, Kind: model.KindFuncDecl, Names: []string{"demo"}, Managed: true, StructuralFidelityKnown: true, FunctionBody: &body},
		{Text: "path=(/bin '' /bin $path)", Category: model.CatPath, Kind: model.KindAssignment, Names: []string{"path"}, Managed: true, StructuralFidelityKnown: true, ListValue: &model.ListValue{Segments: []model.ListSegment{{Value: "/bin"}, {Value: ""}, {Value: "/bin"}, {Self: true}}}},
		{Text: "fpath=($fpath /functions)", Category: model.CatPath, Kind: model.KindAssignment, Names: []string{"fpath"}, Managed: true, StructuralFidelityKnown: true, ListValue: &model.ListValue{Segments: []model.ListSegment{{Self: true}, {Value: "/functions"}}}},
		{Text: "setopt EXTENDED_GLOB", Category: model.CatOptions, Kind: model.KindCommand, CmdName: "setopt", Names: []string{"EXTENDED_GLOB"}, Managed: true, StructuralFidelityKnown: true},
		{Text: "export TOKEN=<redacted>", Category: model.CatSecrets, Kind: model.KindAssignment, Names: []string{"TOKEN"}, Managed: true, StructuralFidelityKnown: true, ValueMode: model.ValueModeUnsupported, Secret: &model.SecretRef{Kind: model.SecretRefFile, Key: "TOKEN"}},
	}}
	overlay := []model.OverlayEntry{
		{Identity: identity(model.LiveEnv, "KEEP"), Value: model.ScalarLiveValue("ignored")},
		{Identity: identity(model.LiveAlias, "ll"), Value: model.ScalarLiveValue("")},
		{Identity: identity(model.LivePath, "PATH"), Value: model.ListLiveValue([]string{"/bin", "", "/bin", "/new"})},
		{Identity: identity(model.LiveOption, "EXTENDED_GLOB"), Value: model.OptionLiveValue(false)},
		{Identity: identity(model.LiveEnv, "ADDED"), Value: model.ScalarLiveValue("exact\nbytes")},
		{Identity: identity(model.LiveEnv, "KEEP"), Value: model.ScalarLiveValue("updated")},
		{Identity: identity(model.LiveEnv, "DROP"), Tombstone: true},
	}

	document, err := activate.BuildEffective(profile, overlay)
	if err != nil {
		t.Fatal(err)
	}
	if document.Schema != model.WorktreeSchemaV1 || document.Projection.Schema != model.WorktreeSchemaV1 {
		t.Fatalf("schemas = %q/%q", document.Schema, document.Projection.Schema)
	}
	if !reflect.DeepEqual(document.Source, profile) {
		t.Fatalf("source history changed:\n got=%#v\nwant=%#v", document.Source, profile)
	}
	wantStates := []model.LiveIdentityState{
		state(model.LiveEnv, "KEEP", model.ScalarLiveValue("updated")),
		state(model.LiveEnv, "EMPTY", model.ScalarLiveValue("")),
		state(model.LiveAlias, "ll", model.ScalarLiveValue("")),
		state(model.LiveFunction, "demo", model.ScalarLiveValue(body)),
		state(model.LivePath, "PATH", model.ListLiveValue([]string{"/bin", "", "/bin", "/new"})),
		state(model.LiveFPath, "FPATH", model.ListLiveValue([]string{"/functions"})),
		state(model.LiveOption, "EXTENDED_GLOB", model.OptionLiveValue(false)),
		state(model.LiveEnv, "ADDED", model.ScalarLiveValue("exact\nbytes")),
	}
	if !reflect.DeepEqual(document.Projection.States, wantStates) {
		t.Fatalf("projection states:\n got=%#v\nwant=%#v", document.Projection.States, wantStates)
	}
	if want := []model.Identity{identity(model.LiveEnv, "DROP")}; !reflect.DeepEqual(document.Projection.Tombstones, want) {
		t.Fatalf("tombstones = %#v, want %#v", document.Projection.Tombstones, want)
	}
	for _, projected := range document.Projection.States {
		if projected.Identity.Name == "TOKEN" {
			t.Fatal("pinned SecretRef entered live projection")
		}
	}
	profile.Entries[0].Text = "mutated"
	*overlay[1].Value.Scalar = "mutated"
	if document.Source.Entries[0].Text == "mutated" || *document.Projection.States[2].Value.Scalar != "" {
		t.Fatal("committed worktree aliases caller memory")
	}
}

func TestBuildEffectiveRejectsMalformedOverlayWithoutPartialDocument(t *testing.T) {
	profile := model.Profile{Entries: []model.Entry{literalEntry(model.CatEnvironment, model.KindAssignment, "A", "one")}}
	document, err := activate.BuildEffective(profile, []model.OverlayEntry{
		{Identity: identity(model.LiveEnv, "A"), Value: model.ScalarLiveValue("two")},
		{Identity: identity(model.LivePath, "PATH"), Value: model.ScalarLiveValue("not-a-list")},
	})
	if err == nil || !reflect.DeepEqual(document, model.CommittedWorktree{}) {
		t.Fatalf("malformed overlay returned partial document: %#v, %v", document, err)
	}
}

func TestBuildLivePatchMatchesCategorizedWorktreeDiff(t *testing.T) {
	before := []model.LiveIdentityState{
		state(model.LiveEnv, "A", model.ScalarLiveValue("shadowed")),
		state(model.LiveAlias, "gone", model.ScalarLiveValue("ls")),
		state(model.LiveFunction, "fn", model.ScalarLiveValue("print old\n")),
		state(model.LivePath, "PATH", model.ListLiveValue([]string{"/base", "", "/dup", "/dup"})),
		state(model.LiveFPath, "FPATH", model.ListLiveValue([]string{})),
		state(model.LiveOption, "NO_BEEP", model.OptionLiveValue(false)),
		state(model.LiveEnv, "A", model.ScalarLiveValue("final")),
	}
	after := []model.LiveIdentityState{
		state(model.LiveEnv, "A", model.ScalarLiveValue("")),
		state(model.LiveAlias, "new", model.ScalarLiveValue("echo 'quoted'")),
		state(model.LiveFunction, "fn", model.ScalarLiveValue("print new\nprint second")),
		state(model.LivePath, "PATH", model.ListLiveValue([]string{"/base", "/dup", "", "/dup"})),
		state(model.LiveFPath, "FPATH", model.ListLiveValue(nil)),
		state(model.LiveOption, "NO_BEEP", model.OptionLiveValue(true)),
	}

	changes, err := worktree.DiffSnapshot(model.LiveSnapshot{States: before}, model.LiveSnapshot{States: after})
	if err != nil {
		t.Fatal(err)
	}
	categorized, err := worktree.CategorizeDiff(changes)
	if err != nil {
		t.Fatal(err)
	}
	patch, err := activate.BuildLivePatch(before, after)
	if err != nil {
		t.Fatal(err)
	}
	if got := opIdentities(patch.Forward); !reflect.DeepEqual(got, changeIdentities(changes)) {
		t.Fatalf("forward identities = %#v, diff identities = %#v", got, changeIdentities(changes))
	}
	if len(categorized.Environment) != 1 || len(categorized.Aliases) != 2 || len(categorized.Functions) != 1 || len(categorized.Path) != 1 || len(categorized.FPath) != 0 || len(categorized.Options) != 1 {
		t.Fatalf("categorized golden lost a category: %#v", categorized)
	}
	if len(patch.Forward) != len(changes) || len(patch.ReplacementReverse) != len(changes) {
		t.Fatalf("patch operation counts = %d/%d, changes=%d", len(patch.Forward), len(patch.ReplacementReverse), len(changes))
	}
	list, ok := patch.Forward[4].(activate.TransitionLiveList)
	if !ok || !reflect.DeepEqual(list.Before, []string{"/base", "", "/dup", "/dup"}) || !reflect.DeepEqual(list.After, []string{"/base", "/dup", "", "/dup"}) {
		t.Fatalf("PATH transition lost occurrence order: %#v", patch.Forward[4])
	}

	forward := applyOps(t, before, patch.Forward)
	if !equalStates(forward, after) {
		t.Fatalf("forward result:\n got=%#v\nwant=%#v", forward, after)
	}
	reversed := applyOps(t, forward, patch.ReplacementReverse)
	if !equalStates(reversed, before) {
		t.Fatalf("replacement reverse result:\n got=%#v\nwant=%#v", reversed, before)
	}

	again, err := activate.BuildLivePatch(before, after)
	if err != nil || !reflect.DeepEqual(patch, again) {
		t.Fatalf("patch is not deterministic: %#v / %#v / %v", patch, again, err)
	}
}

func TestBuildLivePatchRejectsMalformedCompleteInput(t *testing.T) {
	before := []model.LiveIdentityState{state(model.LiveEnv, "A", model.ScalarLiveValue("old"))}
	after := []model.LiveIdentityState{
		state(model.LiveEnv, "A", model.ScalarLiveValue("new")),
		state(model.LivePath, "PATH", model.ScalarLiveValue("wrong")),
	}
	patch, err := activate.BuildLivePatch(before, after)
	if err == nil || len(patch.Forward) != 0 || len(patch.ReplacementReverse) != 0 {
		t.Fatalf("malformed input returned partial patch: %#v, %v", patch, err)
	}
}

func opIdentities(operations []activate.Op) []model.Identity {
	out := make([]model.Identity, 0, len(operations))
	for _, operation := range operations {
		switch operation := operation.(type) {
		case activate.SetLiveScalar:
			out = append(out, operation.Identity)
		case activate.RemoveLiveScalar:
			out = append(out, operation.Identity)
		case activate.TransitionLiveList:
			out = append(out, operation.Identity)
		case activate.SetLiveOptionState:
			out = append(out, operation.Identity)
		case activate.RemoveLiveOptionState:
			out = append(out, operation.Identity)
		default:
			panic("unexpected live operation")
		}
	}
	return out
}

func changeIdentities(changes []model.LiveChange) []model.Identity {
	out := make([]model.Identity, len(changes))
	for index, change := range changes {
		out[index] = change.Identity
	}
	return out
}

func applyOps(t *testing.T, states []model.LiveIdentityState, operations []activate.Op) []model.LiveIdentityState {
	t.Helper()
	normalized, err := model.NormalizeLiveStates(states)
	if err != nil {
		t.Fatal(err)
	}
	values := make(map[model.Identity]model.LiveValue, len(normalized))
	order := make([]model.Identity, 0, len(normalized)+len(operations))
	for _, item := range normalized {
		if item.Value.Present {
			values[item.Identity] = model.CloneLiveValue(item.Value)
			order = append(order, item.Identity)
		}
	}
	set := func(id model.Identity, value model.LiveValue) {
		if _, exists := values[id]; !exists {
			order = append(order, id)
		}
		values[id] = model.CloneLiveValue(value)
	}
	for _, operation := range operations {
		switch operation := operation.(type) {
		case activate.SetLiveScalar:
			set(operation.Identity, model.ScalarLiveValue(operation.Value))
		case activate.RemoveLiveScalar:
			delete(values, operation.Identity)
		case activate.TransitionLiveList:
			if operation.AfterPresent {
				set(operation.Identity, model.ListLiveValue(operation.After))
			} else {
				delete(values, operation.Identity)
			}
		case activate.SetLiveOptionState:
			set(operation.Identity, model.OptionLiveValue(operation.Enabled))
		case activate.RemoveLiveOptionState:
			delete(values, operation.Identity)
		default:
			t.Fatalf("unexpected operation %T", operation)
		}
	}
	out := make([]model.LiveIdentityState, 0, len(values))
	seen := map[model.Identity]bool{}
	for _, id := range order {
		if value, exists := values[id]; exists && !seen[id] {
			out = append(out, model.LiveIdentityState{Identity: id, Value: model.CloneLiveValue(value)})
			seen[id] = true
		}
	}
	return out
}

func equalStates(left, right []model.LiveIdentityState) bool {
	leftNormalized, leftErr := model.NormalizeLiveStates(left)
	rightNormalized, rightErr := model.NormalizeLiveStates(right)
	if leftErr != nil || rightErr != nil || len(leftNormalized) != len(rightNormalized) {
		return false
	}
	leftValues := map[model.Identity]model.LiveValue{}
	for _, item := range leftNormalized {
		if item.Value.Present {
			leftValues[item.Identity] = item.Value
		}
	}
	for _, item := range rightNormalized {
		value, exists := leftValues[item.Identity]
		if item.Value.Present != exists || (exists && !model.EqualLiveValue(item.Identity.Kind, value, item.Value)) {
			return false
		}
	}
	return len(leftValues) == len(rightNormalized)
}
