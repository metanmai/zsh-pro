package worktree

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"zsh-pro/core/model"
)

func TestMaterializeIsExactIdempotentAndMismatchRequiresRecovery(t *testing.T) {
	harness := newServiceHarness(t, fakeLiveSecretPolicy{})
	committed := serviceCommitted("shared")
	if err := harness.service.Materialize(context.Background(), "main", strings.Repeat("a", 40), committed); err != nil {
		t.Fatal(err)
	}
	first := harness.state(t)
	if !first.Materialized || first.HeadRevision != 1 || !first.AutoApplyDefault || len(first.Events) != 1 {
		t.Fatalf("materialized state = %#v", first)
	}
	if err := harness.service.Materialize(context.Background(), "main", strings.Repeat("a", 40), committed); err != nil {
		t.Fatalf("exact repeat failed: %v", err)
	}
	if got := harness.state(t); got.HeadRevision != 1 || len(got.Events) != 1 {
		t.Fatalf("exact repeat changed generation: %#v", got)
	}
	if err := harness.service.Materialize(context.Background(), "main", strings.Repeat("b", 40), committed); !errors.Is(err, ErrRecoveryRequired) {
		t.Fatalf("mismatched repeat = %v", err)
	}
	if got := harness.state(t); got.BaseOID != strings.Repeat("a", 40) || got.HeadRevision != 1 {
		t.Fatalf("mismatch overwrote materialized truth: %#v", got)
	}
}

func TestAttachRequiredBeforePublishAndExactReplayPublishesNoDelta(t *testing.T) {
	harness := materializedServiceHarness(t, fakeLiveSecretPolicy{})
	credential := serviceCredential(t, "shell-a", 'A')
	request := model.PublishRequest{
		OperationID: "publish-before-attach", Credential: credential, AcknowledgedRevision: 1,
		Delta: []model.LiveChange{serviceUpdate(serviceIdentity("EDITOR"), "illegal")},
	}
	before := harness.bytes(t)
	if _, err := harness.service.Publish(context.Background(), request); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("unattached publish = %v", err)
	}
	if after := harness.bytes(t); !bytes.Equal(before, after) {
		t.Fatal("unattached publish changed canonical state")
	}

	attach := model.AttachRequest{OperationID: "attach-a", Credential: credential, Initial: serviceSnapshot(serviceState("EDITOR", "shared"))}
	result, err := harness.service.Attach(context.Background(), attach)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Attached || result.Revision != 1 || len(result.Exclusions) != 0 {
		t.Fatalf("attach result = %#v", result)
	}
	state := harness.state(t)
	shell := state.Shells["shell-a"]
	if len(state.Events) != 1 || state.HeadRevision != 1 || shell.AppliedRevision != 1 || shell.Behind || len(shell.UnpublishedDelta) != 0 {
		t.Fatalf("attach published or lost baseline: state=%#v shell=%#v", state, shell)
	}
	replayed, err := harness.service.Attach(context.Background(), attach)
	if err != nil || !reflect.DeepEqual(replayed, result) {
		t.Fatalf("attach replay = %#v, %v", replayed, err)
	}
	if got := harness.state(t); len(got.OperationReceipts) != 1 || len(got.Events) != 1 {
		t.Fatalf("attach replay duplicated state: %#v", got)
	}
}

func TestAttachFiltersInheritedAndSecretValuesWhilePostAttachAdmissionWorks(t *testing.T) {
	secret := serviceIdentity("CREATED_API_TOKEN")
	harness := materializedServiceHarness(t, fakeLiveSecretPolicy{secret: true})
	credential := serviceCredential(t, "shell-a", 'A')
	const inheritedCanary = "inherited-value-canary"
	const secretCanary = "secret-value-canary"
	_, err := harness.service.Attach(context.Background(), model.AttachRequest{
		OperationID: "attach-filter", Credential: credential,
		Initial: serviceSnapshot(
			serviceState("EDITOR", "shared"),
			serviceState("INHERITED", inheritedCanary),
			serviceState("CREATED_API_TOKEN", secretCanary),
		),
	})
	if err != nil {
		t.Fatal(err)
	}
	state := harness.state(t)
	shell := state.Shells["shell-a"]
	if len(shell.CaptureBaseline) != 1 || len(shell.Exclusions) != 2 || len(shell.PresentAtAttach) != 3 {
		t.Fatalf("filtered attachment = %#v", shell)
	}
	result, err := harness.service.Publish(context.Background(), model.PublishRequest{
		OperationID: "publish-admission", Credential: credential, AcknowledgedRevision: 1,
		Delta: []model.LiveChange{
			serviceUpdate(serviceIdentity("INHERITED"), "changed-inherited-canary"),
			serviceUpdate(secret, "changed-secret-canary"),
			serviceUpdate(serviceIdentity("CREATED_AFTER_ATTACH"), "admitted"),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := result.Accepted, []model.Identity{serviceIdentity("CREATED_AFTER_ATTACH")}; !reflect.DeepEqual(got, want) {
		t.Fatalf("accepted = %#v, want %#v", got, want)
	}
	raw := harness.bytes(t)
	for _, canary := range []string{inheritedCanary, secretCanary, "changed-inherited-canary", "changed-secret-canary"} {
		if bytes.Contains(raw, []byte(canary)) {
			t.Fatalf("excluded value %q reached canonical state", canary)
		}
	}
}

func TestPrivateCredentialSpoofCrossShellReplayAndReceiptGuessAreValueFree(t *testing.T) {
	harness := materializedServiceHarness(t, fakeLiveSecretPolicy{})
	credentialA := serviceCredential(t, "shell-a", 'A')
	credentialB := serviceCredential(t, "shell-b", 'B')
	harness.attach(t, credentialA, "attach-a")
	harness.attach(t, credentialB, "attach-b")
	publish := model.PublishRequest{
		OperationID: "publish-a", Credential: credentialA, AcknowledgedRevision: 1,
		Delta: []model.LiveChange{serviceUpdate(serviceIdentity("A_ONLY"), "a")},
	}
	if _, err := harness.service.Publish(context.Background(), publish); err != nil {
		t.Fatal(err)
	}
	before := harness.bytes(t)
	attacks := []model.PublishRequest{
		{OperationID: publish.OperationID, Credential: model.ShellCredential{ShellID: credentialA.ShellID, Capability: credentialB.Capability}, AcknowledgedRevision: 1, Delta: publish.Delta},
		{OperationID: publish.OperationID, Credential: model.ShellCredential{ShellID: credentialB.ShellID, Capability: credentialA.Capability}, AcknowledgedRevision: 1, Delta: publish.Delta},
		{OperationID: "guessed-receipt", Credential: model.ShellCredential{ShellID: credentialA.ShellID, Capability: credentialB.Capability}, AcknowledgedRevision: 1},
	}
	for _, attack := range attacks {
		if _, err := harness.service.Publish(context.Background(), attack); !errors.Is(err, ErrUnauthorized) {
			t.Fatalf("spoof outcome = %v", err)
		}
		if after := harness.bytes(t); !bytes.Equal(before, after) {
			t.Fatal("unauthorized request mutated canonical state")
		}
	}
}

func TestPublishDisjointStaleChangesCompose(t *testing.T) {
	harness := materializedServiceHarness(t, fakeLiveSecretPolicy{})
	a := serviceCredential(t, "shell-a", 'A')
	b := serviceCredential(t, "shell-b", 'B')
	harness.attach(t, a, "attach-a")
	harness.attach(t, b, "attach-b")
	first, err := harness.service.Publish(context.Background(), model.PublishRequest{
		OperationID: "publish-a", Credential: a, AcknowledgedRevision: 1,
		Delta: []model.LiveChange{serviceUpdate(serviceIdentity("A_ONLY"), "a")},
	})
	if err != nil || first.SharedRevision != 2 || len(first.Accepted) != 1 {
		t.Fatalf("first publish = %#v, %v", first, err)
	}
	second, err := harness.service.Publish(context.Background(), model.PublishRequest{
		OperationID: "publish-b", Credential: b, AcknowledgedRevision: 1,
		Delta: []model.LiveChange{serviceUpdate(serviceIdentity("B_ONLY"), "b")},
	})
	if err != nil || second.SharedRevision != 3 || len(second.Accepted) != 1 || len(second.Conflicts) != 0 {
		t.Fatalf("stale disjoint publish = %#v, %v", second, err)
	}
	state := harness.state(t)
	if !serviceStateHas(state.Shared, serviceIdentity("A_ONLY"), "a") || !serviceStateHas(state.Shared, serviceIdentity("B_ONLY"), "b") {
		t.Fatalf("disjoint changes did not compose: %#v", state.Shared)
	}
}

func TestConcurrentOverlapFirstPublicationWinsAndLoserRemainsUnpublished(t *testing.T) {
	harness := materializedServiceHarness(t, fakeLiveSecretPolicy{})
	a := serviceCredential(t, "shell-a", 'A')
	b := serviceCredential(t, "shell-b", 'B')
	harness.attach(t, a, "attach-a")
	harness.attach(t, b, "attach-b")
	if _, err := harness.service.Publish(context.Background(), model.PublishRequest{
		OperationID: "winner", Credential: a, AcknowledgedRevision: 1,
		Delta: []model.LiveChange{serviceUpdate(serviceIdentity("EDITOR"), "winner")},
	}); err != nil {
		t.Fatal(err)
	}
	result, err := harness.service.Publish(context.Background(), model.PublishRequest{
		OperationID: "loser", Credential: b, AcknowledgedRevision: 1,
		Delta: []model.LiveChange{
			serviceUpdate(serviceIdentity("EDITOR"), "loser"),
			serviceUpdate(serviceIdentity("B_ONLY"), "accepted"),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Accepted) != 1 || result.Accepted[0] != serviceIdentity("B_ONLY") || len(result.Conflicts) != 1 {
		t.Fatalf("partial publish result = %#v", result)
	}
	state := harness.state(t)
	if !serviceStateHas(state.Shared, serviceIdentity("EDITOR"), "winner") || !serviceStateHas(state.Shared, serviceIdentity("B_ONLY"), "accepted") {
		t.Fatalf("winner/accepted state = %#v", state.Shared)
	}
	shell := state.Shells["shell-b"]
	if shell.Conflict == nil || len(shell.UnpublishedDelta) != 1 || !serviceStateHas(shell.CaptureBaseline, serviceIdentity("EDITOR"), "loser") {
		t.Fatalf("losing value was not durable and distinct: %#v", shell)
	}
}

func TestPreparePullAndAcknowledgeRequireExactFreshTarget(t *testing.T) {
	harness := materializedServiceHarness(t, fakeLiveSecretPolicy{})
	a := serviceCredential(t, "shell-a", 'A')
	b := serviceCredential(t, "shell-b", 'B')
	harness.attach(t, a, "attach-a")
	harness.attach(t, b, "attach-b")
	if _, err := harness.service.Publish(context.Background(), model.PublishRequest{
		OperationID: "publish-a", Credential: a, AcknowledgedRevision: 1,
		Delta: []model.LiveChange{serviceUpdate(serviceIdentity("A_ONLY"), "a")},
	}); err != nil {
		t.Fatal(err)
	}
	pending, err := harness.service.PreparePull(context.Background(), model.PreparePullRequest{
		OperationID: "prepare-b", Credential: b, AppliedRevision: 1,
	})
	if err != nil || pending.PendingRevision != 2 || pending.Token == "" || len(pending.Changes) != 1 {
		t.Fatalf("prepare = %#v, %v", pending, err)
	}
	badAck := model.AcknowledgeRequest{
		OperationID: "ack-b-bad", Credential: b, Revision: pending.PendingRevision, Token: pending.Token,
		Snapshot: serviceSnapshot(serviceState("EDITOR", "shared"), serviceState("A_ONLY", "wrong")),
	}
	before := harness.bytes(t)
	if _, err := harness.service.Acknowledge(context.Background(), badAck); !errors.Is(err, ErrAcknowledgeMismatch) {
		t.Fatalf("bad acknowledge = %v", err)
	}
	if after := harness.bytes(t); !bytes.Equal(before, after) {
		t.Fatal("failed acknowledgement changed pending/baseline state")
	}
	goodAck := badAck
	goodAck.OperationID = "ack-b-good"
	goodAck.Snapshot = serviceSnapshot(serviceState("EDITOR", "shared"), serviceState("A_ONLY", "a"))
	result, err := harness.service.Acknowledge(context.Background(), goodAck)
	if err != nil || !result.Acknowledged || result.AppliedRevision != 2 {
		t.Fatalf("acknowledge = %#v, %v", result, err)
	}
	shell := harness.state(t).Shells["shell-b"]
	if shell.Pending.Kind != PendingNone || shell.AppliedRevision != 2 || shell.Behind {
		t.Fatalf("acknowledged shell = %#v", shell)
	}
}

func TestAcknowledgePreparedRevisionPreservesConcurrentLaterHead(t *testing.T) {
	harness := materializedServiceHarness(t, fakeLiveSecretPolicy{})
	a := serviceCredential(t, "shell-a", 'A')
	b := serviceCredential(t, "shell-b", 'B')
	harness.attach(t, a, "attach-a")
	harness.attach(t, b, "attach-b")
	harness.publish(t, a, "publish-2", 1, serviceUpdate(serviceIdentity("REMOTE_2"), "two"))
	pending, err := harness.service.PreparePull(context.Background(), model.PreparePullRequest{OperationID: "prepare-2", Credential: b, AppliedRevision: 1})
	if err != nil {
		t.Fatal(err)
	}
	harness.publish(t, a, "publish-3", 1, serviceUpdate(serviceIdentity("REMOTE_3"), "three"))
	target := serviceSnapshot(serviceState("EDITOR", "shared"), serviceState("REMOTE_2", "two"))
	if _, err := harness.service.Acknowledge(context.Background(), model.AcknowledgeRequest{
		OperationID: "ack-2", Credential: b, Revision: pending.PendingRevision, Token: pending.Token, Snapshot: target,
	}); err != nil {
		t.Fatal(err)
	}
	shell := harness.state(t).Shells["shell-b"]
	if shell.AppliedRevision != 2 || !shell.Behind {
		t.Fatalf("later head was collapsed into prepared ack: %#v", shell)
	}
}

func TestResolveSharedRetainsConflictUntilExactAcknowledgement(t *testing.T) {
	harness, b, conflict := overlapHarness(t)
	loserSnapshot := serviceSnapshot(serviceState("EDITOR", "loser"))
	resolved, err := harness.service.ResolveShared(context.Background(), model.ResolveSharedRequest{
		OperationID: "resolve-b", Credential: b, Conflict: conflict.Identity, Token: conflict.Token, Snapshot: loserSnapshot,
	})
	if err != nil || resolved.PendingRevision != conflict.SharedRevision || resolved.Token == "" || len(resolved.Changes) != 1 {
		t.Fatalf("resolve shared = %#v, %v", resolved, err)
	}
	state := harness.state(t)
	shell := state.Shells["shell-b"]
	if shell.Conflict == nil || len(shell.UnpublishedDelta) != 1 || shell.Pending.Kind != PendingResolution || !serviceStateHas(shell.CaptureBaseline, conflict.Identity, "loser") {
		t.Fatalf("resolve cleared evidence before apply: %#v", shell)
	}
	badBefore := harness.bytes(t)
	if _, err := harness.service.Acknowledge(context.Background(), model.AcknowledgeRequest{
		OperationID: "ack-resolve-bad", Credential: b, Revision: resolved.PendingRevision, Token: resolved.Token, Snapshot: loserSnapshot,
	}); !errors.Is(err, ErrAcknowledgeMismatch) {
		t.Fatalf("losing snapshot acknowledged: %v", err)
	}
	if after := harness.bytes(t); !bytes.Equal(badBefore, after) {
		t.Fatal("failed resolution acknowledgement cleared evidence")
	}
	winnerSnapshot := serviceSnapshot(serviceState("EDITOR", "winner"))
	if _, err := harness.service.Acknowledge(context.Background(), model.AcknowledgeRequest{
		OperationID: "ack-resolve", Credential: b, Revision: resolved.PendingRevision, Token: resolved.Token, Snapshot: winnerSnapshot,
	}); err != nil {
		t.Fatal(err)
	}
	shell = harness.state(t).Shells["shell-b"]
	if shell.Conflict != nil || len(shell.UnpublishedDelta) != 0 || shell.Pending.Kind != PendingNone || !serviceStateHas(shell.CaptureBaseline, conflict.Identity, "winner") {
		t.Fatalf("successful resolution did not converge: %#v", shell)
	}
}

func TestHistoryGapDirtyPublishRequiresExplicitSharedResolution(t *testing.T) {
	harness := materializedServiceHarness(t, fakeLiveSecretPolicy{})
	b := serviceCredential(t, "shell-b", 'B')
	harness.attach(t, b, "attach-b")
	if err := harness.store.WithTransaction(context.Background(), func(state *State) error {
		for index := 0; index < MaxStateEvents+1; index++ {
			identity := serviceIdentity(fmt.Sprintf("REMOTE_%03d", index))
			change := serviceUpdate(identity, "remote")
			state.HeadRevision++
			state.Shared, _ = applyStateChanges(state.Shared, []model.LiveChange{change})
			state.Events = append(state.Events, StateEvent{Revision: state.HeadRevision, OperationID: fmt.Sprintf("remote-%03d", index), ShellID: "shell-b", Changes: []model.LiveChange{change}})
		}
		return CompactStateHistory(state)
	}); err != nil {
		t.Fatal(err)
	}
	result, err := harness.service.Publish(context.Background(), model.PublishRequest{
		OperationID: "dirty-gap", Credential: b, AcknowledgedRevision: 1,
		Delta: []model.LiveChange{serviceUpdate(serviceIdentity("LOCAL_DIRTY"), "local")},
	})
	if err != nil || len(result.Accepted) != 0 || len(result.Conflicts) != 1 || result.Conflicts[0].Kind != model.ConflictHistoryGap {
		t.Fatalf("dirty history gap = %#v, %v", result, err)
	}
	state := harness.state(t)
	shell := state.Shells["shell-b"]
	if state.CompactionFloor == 0 || shell.Conflict == nil || shell.Conflict.Kind != model.ConflictHistoryGap || len(shell.UnpublishedDelta) != 1 || serviceStateHas(state.Shared, serviceIdentity("LOCAL_DIRTY"), "local") {
		t.Fatalf("history-gap durability = floor=%d shell=%#v", state.CompactionFloor, shell)
	}
}

func TestCrashReplayPublishResolveAndAcknowledgeIsIdempotent(t *testing.T) {
	t.Run("publish", func(t *testing.T) {
		harness := materializedServiceHarness(t, fakeLiveSecretPolicy{})
		a := serviceCredential(t, "shell-a", 'A')
		harness.attach(t, a, "attach-a")
		request := model.PublishRequest{OperationID: "publish-lost", Credential: a, AcknowledgedRevision: 1, Delta: []model.LiveChange{serviceUpdate(serviceIdentity("LOST"), "response")}}
		harness.store.faults.afterRename = func() error { return errors.New("lost response") }
		if _, err := harness.service.Publish(context.Background(), request); err == nil {
			t.Fatal("lost response fault not returned")
		}
		harness.store.faults.afterRename = nil
		result, err := harness.service.Publish(context.Background(), request)
		if err != nil || result.SharedRevision != 2 {
			t.Fatalf("publish replay = %#v, %v", result, err)
		}
		state := harness.state(t)
		if state.HeadRevision != 2 || serviceReceiptCount(state, "publish-lost") != 1 {
			t.Fatalf("publish replay duplicated transition: %#v", state)
		}
		wrong := request
		wrong.Credential = serviceCredential(t, "shell-a", 'Z')
		if _, err := harness.service.Publish(context.Background(), wrong); !errors.Is(err, ErrUnauthorized) {
			t.Fatalf("wrong credential replay = %v", err)
		}
	})

	t.Run("resolve-and-acknowledge", func(t *testing.T) {
		harness, b, conflict := overlapHarness(t)
		resolve := model.ResolveSharedRequest{OperationID: "resolve-lost", Credential: b, Conflict: conflict.Identity, Token: conflict.Token, Snapshot: serviceSnapshot(serviceState("EDITOR", "loser"))}
		harness.store.faults.afterRename = func() error { return errors.New("lost resolve response") }
		if _, err := harness.service.ResolveShared(context.Background(), resolve); err == nil {
			t.Fatal("lost resolve response not returned")
		}
		harness.store.faults.afterRename = nil
		pending, err := harness.service.ResolveShared(context.Background(), resolve)
		if err != nil {
			t.Fatal(err)
		}
		ack := model.AcknowledgeRequest{OperationID: "ack-lost", Credential: b, Revision: pending.PendingRevision, Token: pending.Token, Snapshot: serviceSnapshot(serviceState("EDITOR", "winner"))}
		harness.store.faults.afterDirSync = func() error { return errors.New("lost ack response") }
		if _, err := harness.service.Acknowledge(context.Background(), ack); err == nil {
			t.Fatal("lost acknowledgement response not returned")
		}
		harness.store.faults.afterDirSync = nil
		result, err := harness.service.Acknowledge(context.Background(), ack)
		if err != nil || !result.Acknowledged {
			t.Fatalf("ack replay = %#v, %v", result, err)
		}
		state := harness.state(t)
		if serviceReceiptCount(state, "resolve-lost") != 1 || serviceReceiptCount(state, "ack-lost") != 1 || state.Shells["shell-b"].Conflict != nil {
			t.Fatalf("resolve/ack replay duplicated or lost state: %#v", state)
		}
	})
}

func TestStatusAndDiffRemainValueFreeAndUseDurableAuthority(t *testing.T) {
	harness := materializedServiceHarness(t, fakeLiveSecretPolicy{})
	a := serviceCredential(t, "shell-a", 'A')
	harness.attach(t, a, "attach-a")
	harness.publish(t, a, "publish-dirty", 1, serviceUpdate(serviceIdentity("NEW_VALUE"), "sensitive-canary"))
	status, err := harness.service.Status(context.Background(), "shell-a")
	if err != nil {
		t.Fatal(err)
	}
	if status.Branch != "main" || status.BaseOID != strings.Repeat("a", 40) || status.Revision != 2 || status.DirtyCount != 1 || status.Shell == nil || !status.Shell.Behind {
		t.Fatalf("status = %#v", status)
	}
	diff, err := harness.service.Diff(context.Background())
	if err != nil || len(diff.Environment) != 1 || diff.Environment[0].Identity != serviceIdentity("NEW_VALUE") {
		t.Fatalf("diff = %#v, %v", diff, err)
	}
	if strings.Contains(fmt.Sprintf("%#v %#v", status, diff), "sensitive-canary") {
		t.Fatal("public status/diff exposed a captured value")
	}
}

type serviceHarness struct {
	root    string
	store   *StateStore
	service *Service
}

func newServiceHarness(t *testing.T, policy fakeLiveSecretPolicy) serviceHarness {
	t.Helper()
	root := privateStateRoot(t)
	store, err := OpenStateStore(root)
	if err != nil {
		t.Fatal(err)
	}
	service, err := NewService(store, NewRegistry(policy))
	if err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	harness := serviceHarness{root: root, store: store, service: service}
	t.Cleanup(func() { closeStateStore(t, store) })
	return harness
}

func materializedServiceHarness(t *testing.T, policy fakeLiveSecretPolicy) serviceHarness {
	t.Helper()
	harness := newServiceHarness(t, policy)
	if err := harness.service.Materialize(context.Background(), "main", strings.Repeat("a", 40), serviceCommitted("shared")); err != nil {
		t.Fatal(err)
	}
	return harness
}

func serviceCommitted(editor string) model.CommittedWorktree {
	states := []model.LiveIdentityState{serviceState("EDITOR", editor)}
	return model.NewCommittedWorktree(
		model.Profile{Entries: []model.Entry{materializedAssignment("EDITOR", model.CatEnvironment)}},
		model.LiveProjection{States: states},
	)
}

func serviceCredential(t *testing.T, shellID string, fill byte) model.ShellCredential {
	t.Helper()
	capability, err := model.NewShellCapability(bytes.Repeat([]byte{fill}, model.ShellCapabilityBytes))
	if err != nil {
		t.Fatal(err)
	}
	return model.ShellCredential{ShellID: shellID, Capability: capability}
}

func (h serviceHarness) attach(t *testing.T, credential model.ShellCredential, operationID string) model.AttachResult {
	t.Helper()
	result, err := h.service.Attach(context.Background(), model.AttachRequest{OperationID: operationID, Credential: credential, Initial: serviceSnapshot(serviceState("EDITOR", "shared"))})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func (h serviceHarness) publish(t *testing.T, credential model.ShellCredential, operationID string, revision uint64, changes ...model.LiveChange) model.PublishResult {
	t.Helper()
	result, err := h.service.Publish(context.Background(), model.PublishRequest{OperationID: operationID, Credential: credential, AcknowledgedRevision: revision, Delta: changes})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func (h serviceHarness) state(t *testing.T) State {
	t.Helper()
	state, err := h.store.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return state
}

func (h serviceHarness) bytes(t *testing.T) []byte {
	t.Helper()
	payload, err := os.ReadFile(filepath.Join(h.root, stateFileName))
	if err != nil {
		t.Fatal(err)
	}
	return payload
}

func overlapHarness(t *testing.T) (serviceHarness, model.ShellCredential, model.Conflict) {
	t.Helper()
	harness := materializedServiceHarness(t, fakeLiveSecretPolicy{})
	a := serviceCredential(t, "shell-a", 'A')
	b := serviceCredential(t, "shell-b", 'B')
	harness.attach(t, a, "attach-a")
	harness.attach(t, b, "attach-b")
	harness.publish(t, a, "winner", 1, serviceUpdate(serviceIdentity("EDITOR"), "winner"))
	loser := harness.publish(t, b, "loser", 1, serviceUpdate(serviceIdentity("EDITOR"), "loser"))
	if len(loser.Conflicts) != 1 {
		t.Fatalf("overlap did not conflict: %#v", loser)
	}
	return harness, b, loser.Conflicts[0]
}

func serviceSnapshot(states ...model.LiveIdentityState) model.LiveSnapshot {
	return model.LiveSnapshot{States: states}
}

func serviceIdentity(name string) model.Identity {
	return model.Identity{Kind: model.LiveEnv, Name: name}
}

func serviceState(name, value string) model.LiveIdentityState {
	return model.LiveIdentityState{Identity: serviceIdentity(name), Value: model.ScalarLiveValue(value)}
}

func serviceUpdate(identity model.Identity, value string) model.LiveChange {
	return model.LiveChange{Kind: model.LiveUpdate, Identity: identity, Value: model.ScalarLiveValue(value)}
}

func serviceStateHas(states []model.LiveIdentityState, identity model.Identity, value string) bool {
	for _, state := range states {
		if state.Identity == identity && state.Value.Scalar != nil && *state.Value.Scalar == value {
			return true
		}
	}
	return false
}

func serviceReceiptCount(state State, operationID string) int {
	count := 0
	for _, receipt := range state.OperationReceipts {
		if receipt.OperationID == operationID {
			count++
		}
	}
	return count
}
