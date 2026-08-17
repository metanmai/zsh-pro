package worktree

import (
	"bytes"
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"

	"zsh-pro/core/model"
)

func TestStateRoundTripIsStableBoundedAndDefensivelyCopied(t *testing.T) {
	state := validStateFixture(t)
	first, err := MarshalState(state)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) == 0 || first[len(first)-1] != '\n' || (len(first) > 1 && first[len(first)-2] == '\n') {
		t.Fatalf("state does not have exactly one trailing newline: %q", first)
	}
	decoded, err := UnmarshalState(first)
	if err != nil {
		t.Fatal(err)
	}
	second, err := MarshalState(decoded)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatalf("state encoding is not stable:\n%s\n%s", first, second)
	}
	*state.Shared[0].Value.Scalar = "caller-mutated"
	state.Shells["shell-1"].CaptureBaseline[0].Identity.Name = "MUTATED"
	if *decoded.Shared[0].Value.Scalar != "shared" || decoded.Shells["shell-1"].CaptureBaseline[0].Identity.Name != "EDITOR" {
		t.Fatal("decoded state aliases caller memory")
	}
	if len(first) > MaxStateBytes {
		t.Fatalf("fixture exceeds state cap: %d", len(first))
	}
}

func TestShellStateAttachResolveAndLocallyAcknowledgedLoserRemainDistinct(t *testing.T) {
	state := validStateFixture(t)
	shell := state.Shells["shell-1"]
	if shell.AttachState != AttachStateAtHead || shell.AppliedRevision != 1 || shell.AppliedBaseline[0].Identity.Name != "EDITOR" {
		t.Fatalf("attached-at-head collapsed: %#v", shell)
	}
	losing := model.LiveChange{Kind: model.LiveUpdate, Identity: shell.CaptureBaseline[0].Identity, Value: model.ScalarLiveValue("losing-local")}
	shell.CaptureBaseline[0].Value = model.ScalarLiveValue("losing-local")
	shell.UnpublishedDelta = []model.LiveChange{losing}
	shell.Conflict = &model.Conflict{Kind: model.ConflictOverlap, Identity: losing.Identity, BaseRevision: 1, SharedRevision: 2, Token: "resolve-2"}
	shell.Transition = TransitionUnpublished
	shell.Behind = true
	state.HeadRevision = 2
	state.Shared[0].Value = model.ScalarLiveValue("winner")
	state.Events = append(state.Events, StateEvent{Revision: 2, OperationID: "publish-2", ShellID: "shell-2", Changes: []model.LiveChange{{Kind: model.LiveUpdate, Identity: losing.Identity, Value: model.ScalarLiveValue("winner")}}})
	state.Shells["shell-1"] = shell
	if err := ValidateState(state); err != nil {
		t.Fatalf("locally acknowledged loser rejected: %v", err)
	}
	decodedBytes, err := MarshalState(state)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := UnmarshalState(decodedBytes)
	if err != nil {
		t.Fatal(err)
	}
	got := decoded.Shells["shell-1"]
	if !model.EqualLiveValue(model.LiveEnv, got.CaptureBaseline[0].Value, losing.Value) || len(got.UnpublishedDelta) != 1 || got.Conflict == nil {
		t.Fatalf("losing delta collapsed into capture/applied state: %#v", got)
	}

	reconcile := got
	reconcile.AttachState = AttachStateCleanReconcile
	reconcile.AppliedBaseline = nil
	reconcile.AppliedRevision = 0
	if reconcile.AttachState == got.AttachState {
		t.Fatal("clean-reconcile state collapsed into attached-at-head")
	}
}

func TestCompactStateHistoryExact128And129(t *testing.T) {
	state := validStateFixture(t)
	state.HeadRevision = MaxStateEvents
	state.Events = make([]StateEvent, MaxStateEvents)
	for i := range state.Events {
		state.Events[i] = StateEvent{Revision: uint64(i + 1), OperationID: "op-" + strings.Repeat("x", i%3+1), ShellID: "shell-1"}
	}
	if err := CompactStateHistory(&state); err != nil {
		t.Fatal(err)
	}
	if len(state.Events) != MaxStateEvents || state.CompactionFloor != 0 {
		t.Fatalf("exact event cap changed: events=%d floor=%d", len(state.Events), state.CompactionFloor)
	}
	state.HeadRevision++
	state.Events = append(state.Events, StateEvent{Revision: state.HeadRevision, OperationID: "op-last", ShellID: "shell-1"})
	if err := CompactStateHistory(&state); err != nil {
		t.Fatal(err)
	}
	if len(state.Events) != MaxStateEvents || state.CompactionFloor != 1 || state.Events[0].Revision != 2 {
		t.Fatalf("129-event compaction = events=%d floor=%d first=%d", len(state.Events), state.CompactionFloor, state.Events[0].Revision)
	}
}

func TestStateRevisionTransitionReceiptAndGarbageCollectionValidation(t *testing.T) {
	legal := [][2]ShellTransition{
		{"", TransitionAttach},
		{TransitionAttach, TransitionCapture},
		{TransitionCapture, TransitionPublish},
		{TransitionCapture, TransitionUnpublished},
		{TransitionUnpublished, TransitionResolve},
		{TransitionPublish, TransitionPrepare},
		{TransitionPrepare, TransitionApply},
		{TransitionResolve, TransitionApply},
		{TransitionApply, TransitionFreshCapture},
		{TransitionFreshCapture, TransitionAcknowledge},
	}
	for _, edge := range legal {
		if !IsLegalShellTransition(edge[0], edge[1]) {
			t.Errorf("legal transition rejected: %q -> %q", edge[0], edge[1])
		}
	}
	if IsLegalShellTransition(TransitionPrepare, TransitionPublish) {
		t.Fatal("reordered prepare -> publish transition accepted")
	}

	state := validStateFixture(t)
	if CanGarbageCollectShell(state, "shell-1") {
		t.Fatal("active shell was garbage-collectable")
	}
	shell := state.Shells["shell-1"]
	shell.LastActiveRevision = 0
	state.HeadRevision = 10
	state.Shells["shell-1"] = shell
	if !CanGarbageCollectShell(state, "shell-1") {
		t.Fatal("clean inactive shell was not garbage-collectable")
	}
	shell.UnpublishedDelta = []model.LiveChange{{Kind: model.LiveUpdate, Identity: shell.CaptureBaseline[0].Identity, Value: model.ScalarLiveValue("keep")}}
	state.Shells["shell-1"] = shell
	if CanGarbageCollectShell(state, "shell-1") {
		t.Fatal("shell with unpublished delta was garbage-collectable")
	}

	state = validStateFixture(t)
	state.OperationReceipts[0].Attach = nil
	state.OperationReceipts[0].Publish = &model.PublishResult{SharedRevision: 1}
	if err := ValidateState(state); err == nil {
		t.Fatal("contradictory operation receipt accepted")
	}
	state = validStateFixture(t)
	state.HeadRevision = math.MaxUint64
	state.Events[0].Revision = math.MaxUint64
	if err := ValidateState(state); err == nil {
		t.Fatal("nonmonotonic MaxUint64 revision state accepted")
	}
}

func TestStateVerifierAtRestAndSecretCanary(t *testing.T) {
	state := validStateFixture(t)
	encoded, err := MarshalState(state)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encoded, []byte(strings.Repeat("Z", model.ShellCapabilityBytes))) {
		t.Fatal("raw capability canary serialized")
	}
	if !bytes.Contains(encoded, []byte("capability_verifier")) {
		t.Fatal("capability verifier missing at rest")
	}

	var raw map[string]any
	if err := json.Unmarshal(encoded, &raw); err != nil {
		t.Fatal(err)
	}
	shells := raw["shells"].(map[string]any)
	shell := shells["shell-1"].(map[string]any)
	delete(shell, "capability_verifier")
	malformed, _ := json.Marshal(raw)
	if _, err := UnmarshalState(malformed); err == nil {
		t.Fatal("missing verifier accepted")
	}
	raw["unexpected"] = true
	malformed, _ = json.Marshal(raw)
	if _, err := UnmarshalState(malformed); err == nil {
		t.Fatal("unknown state field accepted")
	}
}

func validStateFixture(t *testing.T) State {
	t.Helper()
	raw := bytes.Repeat([]byte("Z"), model.ShellCapabilityBytes)
	capability, err := model.NewShellCapability(raw)
	if err != nil {
		t.Fatal(err)
	}
	verifier, err := model.DeriveShellCapabilityVerifier(capability)
	if err != nil {
		t.Fatal(err)
	}
	identity := model.Identity{Kind: model.LiveEnv, Name: "EDITOR"}
	shared := []model.LiveIdentityState{{Identity: identity, Value: model.ScalarLiveValue("shared")}}
	override := false
	return State{
		SchemaVersion:      StateSchemaVersion,
		Materialized:       true,
		Branch:             "main",
		BaseOID:            strings.Repeat("a", 40),
		Committed:          model.NewCommittedWorktree(model.Profile{}, model.LiveProjection{States: shared}),
		HeadRevision:       1,
		AutoApplyDefault:   true,
		Shared:             model.CloneLiveStates(shared),
		CompactionFloor:    0,
		CompactionBaseline: nil,
		Events: []StateEvent{{
			Revision: 1, OperationID: "materialize-1", Changes: []model.LiveChange{{Kind: model.LiveAdd, Identity: identity, Value: model.ScalarLiveValue("shared")}},
		}},
		Shells: map[string]ShellState{
			"shell-1": {
				CapabilityVerifier: verifier,
				AttachState:        AttachStateAtHead, AttachedRevision: 1,
				AppliedRevision: 1, AppliedBaseline: model.CloneLiveStates(shared),
				CaptureBaseline: model.CloneLiveStates(shared), Transition: TransitionAcknowledge,
				Pending: PendingTransition{Kind: PendingNone}, AutoApplyOverride: &override,
				LastActiveRevision: 1,
			},
		},
		OperationReceipts: []OperationReceipt{{
			ShellID: "shell-1", OperationID: "attach-1", Kind: ReceiptAttach,
			Attach: &model.AttachResult{Revision: 1, Attached: true},
		}},
	}
}

func TestAutoApplyNilAndExplicitFalseRemainDistinct(t *testing.T) {
	state := validStateFixture(t)
	shell := state.Shells["shell-1"]
	if shell.AutoApplyOverride == nil || *shell.AutoApplyOverride {
		t.Fatal("explicit false override lost")
	}
	shell.AutoApplyOverride = nil
	state.Shells["shell-1"] = shell
	encoded, err := MarshalState(state)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := UnmarshalState(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Shells["shell-1"].AutoApplyOverride != nil {
		t.Fatal("nil auto-apply override became explicit false")
	}
	if reflect.DeepEqual(decoded.Shells["shell-1"].AutoApplyOverride, &[]bool{false}[0]) {
		t.Fatal("nil and false auto-apply states collapsed")
	}
}
