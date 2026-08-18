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

func TestStateAdmittedIdentitiesStrictValueFreeRoundTrip(t *testing.T) {
	state := validStateFixture(t)
	want := []model.Identity{
		{Kind: model.LiveAlias, Name: "late-alias"},
		{Kind: model.LiveEnv, Name: "LATE_EDITOR"},
	}

	baseEncoded, err := MarshalState(state)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(baseEncoded, &raw); err != nil {
		t.Fatal(err)
	}
	identitiesJSON, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	raw["admitted_identities"] = identitiesJSON
	encoded, err := json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := UnmarshalState(encoded)
	if err != nil {
		t.Fatalf("strict admitted identity metadata rejected: %v", err)
	}
	if !bytes.Contains(encoded, []byte(`"admitted_identities"`)) ||
		!bytes.Contains(encoded, []byte("late-alias")) ||
		!bytes.Contains(encoded, []byte("LATE_EDITOR")) {
		t.Fatalf("admitted identity metadata missing from canonical state: %s", encoded)
	}
	field := reflect.ValueOf(decoded).FieldByName("AdmittedIdentities")
	if !field.IsValid() {
		t.Fatal("State.AdmittedIdentities is missing")
	}
	got, ok := field.Interface().([]model.Identity)
	if !ok || !reflect.DeepEqual(got, want) {
		t.Fatalf("AdmittedIdentities = %#v, want %#v", got, want)
	}

	delete(raw, "admitted_identities")
	legacy, err := json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := UnmarshalState(legacy); err != nil {
		t.Fatalf("legacy state without admitted identities rejected: %v", err)
	}

	var emptyRaw map[string]json.RawMessage
	if err := json.Unmarshal(baseEncoded, &emptyRaw); err != nil {
		t.Fatal(err)
	}
	if got, ok := emptyRaw["admitted_identities"]; !ok || string(got) != "[]" {
		t.Fatalf("new materialized state did not emit a present empty admitted list: %s", baseEncoded)
	}

	for name, identities := range map[string][]model.Identity{
		"duplicate":          {{Kind: model.LiveEnv, Name: "DUP"}, {Kind: model.LiveEnv, Name: "DUP"}},
		"invalid":            {{Kind: model.LiveEnv, Name: "BAD-NAME"}},
		"noncanonical order": {{Kind: model.LiveEnv, Name: "SECOND"}, {Kind: model.LiveAlias, Name: "first"}},
	} {
		t.Run(name, func(t *testing.T) {
			var candidate map[string]json.RawMessage
			if err := json.Unmarshal(baseEncoded, &candidate); err != nil {
				t.Fatal(err)
			}
			candidate["admitted_identities"], err = json.Marshal(identities)
			if err != nil {
				t.Fatal(err)
			}
			malformed, err := json.Marshal(candidate)
			if err != nil {
				t.Fatal(err)
			}
			if _, decodeErr := UnmarshalState(malformed); decodeErr == nil {
				t.Fatal("invalid admitted identity metadata accepted")
			}
		})
	}

	var strict map[string]any
	if err := json.Unmarshal(encoded, &strict); err != nil {
		t.Fatal(err)
	}
	identities := strict["admitted_identities"].([]any)
	identities[0].(map[string]any)["Value"] = "value-canary-must-not-cross"
	malformed, err := json.Marshal(strict)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := UnmarshalState(malformed); err == nil || strings.Contains(err.Error(), "value-canary-must-not-cross") {
		t.Fatalf("value-bearing admitted identity did not fail closed without disclosure: %v", err)
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

func TestFingerprintSnapshotCanonicalIdentityOrder(t *testing.T) {
	path := model.LiveIdentityState{
		Identity: model.Identity{Kind: model.LivePath, Name: "PATH"},
		Value:    model.ListLiveValue([]string{"", "/one", "/one", "/two"}),
	}
	fpath := model.LiveIdentityState{
		Identity: model.Identity{Kind: model.LiveFPath, Name: "FPATH"},
		Value:    model.ListLiveValue([]string{"/functions/two", "", "/functions/one"}),
	}
	environment := model.LiveIdentityState{
		Identity: model.Identity{Kind: model.LiveEnv, Name: "EDITOR"},
		Value:    model.ScalarLiveValue("shared"),
	}
	alias := model.LiveIdentityState{
		Identity: model.Identity{Kind: model.LiveAlias, Name: "demo.live"},
		Value:    model.ScalarLiveValue("print -- live"),
	}
	original := []model.LiveIdentityState{path, alias, fpath, environment}
	wantOriginal := model.CloneLiveStates(original)

	canonical, err := FingerprintSnapshot(model.LiveSnapshot{States: original})
	if err != nil {
		t.Fatal(err)
	}
	reordered, err := FingerprintSnapshot(model.LiveSnapshot{States: []model.LiveIdentityState{environment, fpath, alias, path}})
	if err != nil {
		t.Fatal(err)
	}
	if canonical != reordered {
		t.Fatal("identity-record order changed the snapshot fingerprint")
	}
	if !reflect.DeepEqual(original, wantOriginal) {
		t.Fatal("fingerprinting reordered or aliased the caller's snapshot")
	}

	assertDifferent := func(name string, states []model.LiveIdentityState) {
		t.Helper()
		fingerprint, fingerprintErr := FingerprintSnapshot(model.LiveSnapshot{States: states})
		if fingerprintErr != nil {
			t.Fatalf("%s: %v", name, fingerprintErr)
		}
		if fingerprint == canonical {
			t.Errorf("%s did not change the snapshot fingerprint", name)
		}
	}

	valueChanged := model.CloneLiveStates(original)
	valueChanged[1].Value = model.ScalarLiveValue("print -- changed")
	assertDifferent("scalar value", valueChanged)

	presenceChanged := model.CloneLiveStates(original)
	presenceChanged[3].Value = model.RemovedLiveValue()
	assertDifferent("present versus absent", presenceChanged)

	identityKindChanged := model.CloneLiveStates(original)
	identityKindChanged[1].Identity = model.Identity{Kind: model.LiveFunction, Name: "demo.live"}
	assertDifferent("identity kind", identityKindChanged)

	identityNameChanged := model.CloneLiveStates(original)
	identityNameChanged[1].Identity.Name = "demo.other"
	assertDifferent("identity name", identityNameChanged)

	pathOrderChanged := model.CloneLiveStates(original)
	pathOrderChanged[0].Value = model.ListLiveValue([]string{"/one", "", "/one", "/two"})
	assertDifferent("PATH element order", pathOrderChanged)

	fpathOrderChanged := model.CloneLiveStates(original)
	fpathOrderChanged[2].Value = model.ListLiveValue([]string{"", "/functions/two", "/functions/one"})
	assertDifferent("FPATH element order", fpathOrderChanged)
}
