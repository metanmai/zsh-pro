package worktree

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"zsh-pro/core/model"
	"zsh-pro/core/shell/zsh"
	storepkg "zsh-pro/core/store"
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

func TestAttachResultTruthfullyReportsCleanReconcile(t *testing.T) {
	reconcileRequired := func(t *testing.T, result model.AttachResult) bool {
		t.Helper()
		field := reflect.ValueOf(result).FieldByName("ReconcileRequired")
		if !field.IsValid() || field.Kind() != reflect.Bool {
			t.Fatal("AttachResult does not carry the reconcile-required decision")
		}
		return field.Bool()
	}

	t.Run("exact initial snapshot is applied at head", func(t *testing.T) {
		harness := materializedServiceHarness(t, fakeLiveSecretPolicy{})
		credential := serviceCredential(t, "shell-exact", 'E')
		result, err := harness.service.Attach(context.Background(), model.AttachRequest{
			OperationID: "attach-exact-truth", Credential: credential,
			Initial: serviceSnapshot(serviceState("EDITOR", "shared")),
		})
		if err != nil {
			t.Fatal(err)
		}
		if !result.Attached || result.Revision != 1 || reconcileRequired(t, result) {
			t.Fatalf("exact attach result = %#v", result)
		}
		shell := harness.state(t).Shells[credential.ShellID]
		if shell.AttachState != AttachStateAtHead || shell.AppliedRevision != 1 || shell.Behind ||
			!equalLiveStates(shell.AppliedBaseline, []model.LiveIdentityState{serviceState("EDITOR", "shared")}) {
			t.Fatalf("exact attach shell truth = %#v", shell)
		}
	})

	t.Run("mismatched initial snapshot remains behind and replays identically", func(t *testing.T) {
		harness := materializedServiceHarness(t, fakeLiveSecretPolicy{})
		credential := serviceCredential(t, "shell-mismatch", 'M')
		request := model.AttachRequest{
			OperationID: "attach-mismatch-truth", Credential: credential,
			Initial: serviceSnapshot(serviceState("EDITOR", "stale-local")),
		}
		harness.store.faults.afterRename = func() error { return errors.New("lost attach response") }
		result, err := harness.service.Attach(context.Background(), request)
		if err == nil || !result.Attached || result.Revision != 1 || !reconcileRequired(t, result) {
			t.Fatalf("lost-response mismatch attach = %#v, %v", result, err)
		}
		harness.store.faults.afterRename = nil
		replayed, err := reopenService(t, harness.root, fakeLiveSecretPolicy{}).Attach(context.Background(), request)
		if err != nil || !reflect.DeepEqual(replayed, result) || !reconcileRequired(t, replayed) {
			t.Fatalf("mismatch replay = %#v, %v; want %#v", replayed, err, result)
		}
		shell := harness.state(t).Shells[credential.ShellID]
		if shell.AttachState != AttachStateCleanReconcile || shell.AppliedRevision != 0 || !shell.Behind || len(shell.AppliedBaseline) != 0 {
			t.Fatalf("mismatch attach shell truth = %#v", shell)
		}
	})

	t.Run("concurrent later head is never claimed by attach or stale acknowledgement", func(t *testing.T) {
		harness := materializedServiceHarness(t, fakeLiveSecretPolicy{})
		a := serviceCredential(t, "shell-a", 'A')
		c := serviceCredential(t, "shell-c", 'C')
		harness.attach(t, a, "attach-a-before-c")
		harness.publish(t, a, "publish-head-2", 1, serviceUpdate(serviceIdentity("REMOTE_2"), "two"))

		attached, err := harness.service.Attach(context.Background(), model.AttachRequest{
			OperationID: "attach-c-at-head-2", Credential: c,
			Initial: serviceSnapshot(serviceState("EDITOR", "shared")),
		})
		if err != nil || attached.Revision != 2 || !reconcileRequired(t, attached) {
			t.Fatalf("concurrent mismatch attach = %#v, %v", attached, err)
		}
		if shell := harness.state(t).Shells[c.ShellID]; shell.AppliedRevision != 0 || !shell.Behind {
			t.Fatalf("attach falsely applied concurrent head: %#v", shell)
		}
		pending, err := harness.service.PreparePull(context.Background(), model.PreparePullRequest{
			OperationID: "prepare-c-head-2", Credential: c, AppliedRevision: 0,
		})
		if err != nil || pending.PendingRevision != 2 {
			t.Fatalf("prepare head 2 = %#v, %v", pending, err)
		}
		harness.publish(t, a, "publish-head-3", 1, serviceUpdate(serviceIdentity("REMOTE_3"), "three"))
		acknowledged, err := harness.service.Acknowledge(context.Background(), model.AcknowledgeRequest{
			OperationID: "ack-c-head-2", Credential: c, Revision: pending.PendingRevision, Token: pending.Token,
			Snapshot: serviceSnapshot(serviceState("EDITOR", "shared"), serviceState("REMOTE_2", "two")),
		})
		if err != nil || !acknowledged.Acknowledged || acknowledged.AppliedRevision != 2 {
			t.Fatalf("stale target acknowledgement = %#v, %v", acknowledged, err)
		}
		if shell := harness.state(t).Shells[c.ShellID]; shell.AppliedRevision != 2 || !shell.Behind {
			t.Fatalf("later head was falsely acknowledged: %#v", shell)
		}
	})
}

func TestSemanticEqualityDrivesAttachRepairAndBehind(t *testing.T) {
	ctx := context.Background()
	baseOID := strings.Repeat("a", 40)
	repairedOID := strings.Repeat("b", 40)
	environment := serviceState("EDITOR", "value")
	alias := model.LiveIdentityState{
		Identity: model.Identity{Kind: model.LiveAlias, Name: "demo.live"},
		Value:    model.ScalarLiveValue("print ok"),
	}
	canonical := model.NewCommittedWorktree(
		model.Profile{Entries: []model.Entry{materializedAssignment("EDITOR", model.CatEnvironment), materializedAlias("demo.live")}},
		model.LiveProjection{States: []model.LiveIdentityState{environment, alias}},
	)
	reordered := model.NewCommittedWorktree(canonical.Source, model.LiveProjection{States: []model.LiveIdentityState{alias, environment}})

	harness := newServiceHarness(t, fakeLiveSecretPolicy{})
	repository := &workflowRepository{
		branches: map[string]string{"main": baseOID},
		documents: map[string]model.CommittedWorktree{
			baseOID:     canonical,
			repairedOID: reordered,
		},
	}
	service, err := NewService(harness.store, NewRegistry(fakeLiveSecretPolicy{}), repository)
	if err != nil {
		t.Fatal(err)
	}
	harness.service = service
	if err := service.Materialize(ctx, "main", baseOID, canonical); err != nil {
		t.Fatal(err)
	}

	credential := serviceCredential(t, "semantic-shell", 'S')
	attached, err := service.Attach(ctx, model.AttachRequest{
		OperationID: "semantic-attach", Credential: credential,
		Initial: serviceSnapshot(alias, environment),
	})
	if err != nil || !attached.Attached || attached.ReconcileRequired || attached.Revision != 1 {
		t.Fatalf("order-only attach = %#v, %v", attached, err)
	}
	if shell := harness.state(t).Shells[credential.ShellID]; shell.Behind || shell.AppliedRevision != 1 {
		t.Fatalf("order-only attach was marked behind: %#v", shell)
	}

	repository.branches["main"] = repairedOID
	status, err := service.WorkflowStatus(ctx, credential.ShellID)
	if err != nil || status.Worktree.BaseOID != repairedOID {
		t.Fatalf("order-only exact-OID repair = %#v, %v", status, err)
	}
	repaired := harness.state(t)
	if repaired.BaseOID != repairedOID || !reflect.DeepEqual(repaired.Committed.Projection.States, reordered.Projection.States) {
		t.Fatalf("order-only repair did not retain exact repository document: %#v", repaired)
	}

	if err := service.ResetHard(ctx); err != nil {
		t.Fatalf("order-only generation replacement: %v", err)
	}
	after := harness.state(t)
	shell := after.Shells[credential.ShellID]
	if after.HeadRevision != 1 || shell.Behind || shell.AppliedRevision != 1 {
		t.Fatalf("order-only generation replacement invented change or behind state: %#v", shell)
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

func TestFreshServiceLateShellUsesDurableAdmission(t *testing.T) {
	ctx := context.Background()
	root := privateStateRoot(t)
	secret := serviceIdentity("CREATED_API_TOKEN")
	policy := fakeLiveSecretPolicy{secret: true}
	storeA, err := OpenStateStore(root)
	if err != nil {
		t.Fatal(err)
	}
	serviceA, err := NewService(storeA, NewRegistry(policy))
	if err != nil {
		_ = storeA.Close()
		t.Fatal(err)
	}
	if err := serviceA.Materialize(ctx, "main", strings.Repeat("a", 40), serviceCommitted("shared")); err != nil {
		_ = storeA.Close()
		t.Fatal(err)
	}
	a := serviceCredential(t, "shell-a", 'A')
	if _, err := serviceA.Attach(ctx, model.AttachRequest{
		OperationID: "attach-a-durable", Credential: a,
		Initial: serviceSnapshot(serviceState("EDITOR", "shared")),
	}); err != nil {
		_ = storeA.Close()
		t.Fatal(err)
	}
	alias := model.Identity{Kind: model.LiveAlias, Name: "late-c-alias"}
	const aliasValue = "print -- durable-late-c"
	const secretCanary = "secret-value-must-not-persist"
	publish := model.PublishRequest{
		OperationID: "publish-durable-alias", Credential: a, AcknowledgedRevision: 1,
		Delta: []model.LiveChange{
			{Kind: model.LiveAdd, Identity: alias, Value: model.ScalarLiveValue(aliasValue)},
			{Kind: model.LiveAdd, Identity: secret, Value: model.ScalarLiveValue(secretCanary)},
		},
	}
	published, err := serviceA.Publish(ctx, publish)
	if err != nil {
		_ = storeA.Close()
		t.Fatal(err)
	}
	if got, want := published.Accepted, []model.Identity{alias}; !reflect.DeepEqual(got, want) || published.SharedRevision != 2 {
		_ = storeA.Close()
		t.Fatalf("published = %#v, want revision 2 and %#v", published, want)
	}
	if err := storeA.Close(); err != nil {
		t.Fatal(err)
	}

	serviceReplay := reopenService(t, root, policy)
	replayed, err := serviceReplay.Publish(ctx, publish)
	if err != nil || !reflect.DeepEqual(replayed, published) {
		t.Fatalf("fresh-service publish replay = %#v, %v; want %#v", replayed, err, published)
	}

	serviceC := reopenService(t, root, policy)
	c := serviceCredential(t, "shell-c", 'C')
	attached, err := serviceC.Attach(ctx, model.AttachRequest{
		OperationID: "attach-late-c", Credential: c,
		Initial: serviceSnapshot(
			serviceState("EDITOR", "shared"),
			model.LiveIdentityState{Identity: alias, Value: model.ScalarLiveValue("stale-local")},
			serviceState("AMBIENT_ONLY", "ambient-value-must-not-persist"),
			serviceState(secret.Name, "late-secret-must-not-persist"),
		),
	})
	if err != nil || !attached.Attached || attached.Revision != published.SharedRevision {
		t.Fatalf("late C attach = %#v, %v", attached, err)
	}
	pending, err := serviceC.PreparePull(ctx, model.PreparePullRequest{
		OperationID: "prepare-late-c", Credential: c, AppliedRevision: 0,
	})
	if err != nil || pending.PendingRevision != published.SharedRevision || pending.Token == "" || len(pending.Changes) != 1 {
		t.Fatalf("late C prepare = %#v, %v", pending, err)
	}
	target := serviceSnapshot(
		serviceState("EDITOR", "shared"),
		model.LiveIdentityState{Identity: alias, Value: model.ScalarLiveValue(aliasValue)},
	)
	acknowledged, err := serviceC.Acknowledge(ctx, model.AcknowledgeRequest{
		OperationID: "ack-late-c", Credential: c, Revision: pending.PendingRevision,
		Token: pending.Token, Snapshot: target,
	})
	if err != nil || !acknowledged.Acknowledged || acknowledged.AppliedRevision != published.SharedRevision {
		t.Fatalf("late C acknowledge = %#v, %v", acknowledged, err)
	}

	state, err := serviceC.store.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if state.HeadRevision != 2 || !reflect.DeepEqual(state.AdmittedIdentities, []model.Identity{alias}) || state.Shells[c.ShellID].Behind {
		t.Fatalf("durable late-C state = %#v", state)
	}
	payload, err := os.ReadFile(filepath.Join(root, stateFileName))
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(payload, &raw); err != nil {
		t.Fatal(err)
	}
	admitted := raw["admitted_identities"]
	for _, forbidden := range [][]byte{[]byte(aliasValue), []byte(secret.Name), []byte(secretCanary)} {
		if bytes.Contains(admitted, forbidden) {
			t.Fatalf("forbidden data reached durable admission metadata: %q", forbidden)
		}
	}
	for _, forbidden := range [][]byte{
		[]byte(secretCanary), []byte("late-secret-must-not-persist"), []byte("ambient-value-must-not-persist"),
		bytes.Repeat([]byte{'A'}, model.ShellCapabilityBytes), bytes.Repeat([]byte{'C'}, model.ShellCapabilityBytes),
	} {
		if bytes.Contains(payload, forbidden) {
			t.Fatalf("forbidden value or credential reached canonical state: %q", forbidden)
		}
	}
	if !bytes.Contains(admitted, []byte(alias.Name)) {
		t.Fatalf("durable admission metadata missing alias: %s", admitted)
	}
}

func TestLegacyAdmissionMigrationUsesCanonicalSharedOnly(t *testing.T) {
	harness := materializedServiceHarness(t, fakeLiveSecretPolicy{})
	a := serviceCredential(t, "shell-a", 'A')
	harness.attach(t, a, "attach-a-legacy-migration")
	alias := model.Identity{Kind: model.LiveAlias, Name: "legacy-admitted"}
	harness.publish(t, a, "publish-legacy-admission", 1, model.LiveChange{
		Kind: model.LiveAdd, Identity: alias, Value: model.ScalarLiveValue("print -- legacy"),
	})
	if err := harness.store.WithTransaction(context.Background(), func(state *State) error {
		state.AdmittedIdentities = nil
		state.admittedIdentitiesMissing = true
		shell := state.Shells[a.ShellID]
		shell.PresentAtAttach = append(shell.PresentAtAttach, serviceIdentity("AMBIENT_NOT_SHARED"))
		state.Shells[a.ShellID] = shell
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if raw := harness.bytes(t); bytes.Contains(raw, []byte("admitted_identities")) {
		t.Fatalf("legacy fixture retained admitted identity field: %s", raw)
	}

	fresh := reopenService(t, harness.root, fakeLiveSecretPolicy{})
	c := serviceCredential(t, "shell-c", 'C')
	if _, err := fresh.Attach(context.Background(), model.AttachRequest{
		OperationID: "attach-c-legacy-migration", Credential: c,
		Initial: serviceSnapshot(
			serviceState("EDITOR", "shared"),
			model.LiveIdentityState{Identity: alias, Value: model.ScalarLiveValue("stale")},
			serviceState("AMBIENT_NOT_SHARED", "ambient"),
		),
	}); err != nil {
		t.Fatal(err)
	}
	state, err := fresh.store.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if state.admittedIdentitiesMissing || !reflect.DeepEqual(state.AdmittedIdentities, []model.Identity{alias}) {
		t.Fatalf("legacy migration = missing %v admissions %#v", state.admittedIdentitiesMissing, state.AdmittedIdentities)
	}
	for _, identity := range state.AdmittedIdentities {
		if identity == serviceIdentity("AMBIENT_NOT_SHARED") {
			t.Fatal("shell-local present-at-attach identity migrated into canonical ownership")
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

func TestConcurrentDurableAdmissionsAreDeterministic(t *testing.T) {
	t.Run("disjoint", func(t *testing.T) {
		harness := materializedServiceHarness(t, fakeLiveSecretPolicy{})
		a := serviceCredential(t, "shell-a", 'A')
		b := serviceCredential(t, "shell-b", 'B')
		harness.attach(t, a, "attach-a-durable-disjoint")
		harness.attach(t, b, "attach-b-durable-disjoint")

		alias := model.Identity{Kind: model.LiveAlias, Name: "a-disjoint"}
		environment := serviceIdentity("B_DISJOINT")
		requests := []model.PublishRequest{
			{OperationID: "publish-a-durable-disjoint", Credential: a, AcknowledgedRevision: 1, Delta: []model.LiveChange{{Kind: model.LiveAdd, Identity: alias, Value: model.ScalarLiveValue("print -- a")}}},
			{OperationID: "publish-b-durable-disjoint", Credential: b, AcknowledgedRevision: 1, Delta: []model.LiveChange{{Kind: model.LiveAdd, Identity: environment, Value: model.ScalarLiveValue("b")}}},
		}
		services := []*Service{
			reopenService(t, harness.root, fakeLiveSecretPolicy{}),
			reopenService(t, harness.root, fakeLiveSecretPolicy{}),
		}
		results := make([]model.PublishResult, len(requests))
		errs := make([]error, len(requests))
		start := make(chan struct{})
		var group sync.WaitGroup
		for index := range requests {
			group.Add(1)
			go func(index int) {
				defer group.Done()
				<-start
				results[index], errs[index] = services[index].Publish(context.Background(), requests[index])
			}(index)
		}
		close(start)
		group.Wait()
		for index, err := range errs {
			if err != nil || len(results[index].Accepted) != 1 || len(results[index].Conflicts) != 0 {
				t.Fatalf("disjoint result %d = %#v, %v", index, results[index], err)
			}
		}
		state := harness.state(t)
		want := []model.Identity{alias, environment}
		if state.HeadRevision != 3 || !reflect.DeepEqual(state.AdmittedIdentities, want) ||
			!serviceStateHas(state.Shared, alias, "print -- a") || !serviceStateHas(state.Shared, environment, "b") {
			t.Fatalf("concurrent disjoint durable state = %#v, want admissions %#v", state, want)
		}

		replayed, err := reopenService(t, harness.root, fakeLiveSecretPolicy{}).Publish(context.Background(), requests[0])
		if err != nil || !reflect.DeepEqual(replayed, results[0]) {
			t.Fatalf("disjoint replay = %#v, %v; want %#v", replayed, err, results[0])
		}
		if after := harness.state(t); after.HeadRevision != 3 || !reflect.DeepEqual(after.AdmittedIdentities, want) {
			t.Fatalf("replay duplicated durable admission: %#v", after)
		}
	})

	t.Run("same identity", func(t *testing.T) {
		harness := materializedServiceHarness(t, fakeLiveSecretPolicy{})
		a := serviceCredential(t, "shell-a", 'A')
		b := serviceCredential(t, "shell-b", 'B')
		harness.attach(t, a, "attach-a-durable-overlap")
		harness.attach(t, b, "attach-b-durable-overlap")
		identity := model.Identity{Kind: model.LiveAlias, Name: "same-admission"}
		requests := []model.PublishRequest{
			{OperationID: "publish-a-durable-overlap", Credential: a, AcknowledgedRevision: 1, Delta: []model.LiveChange{{Kind: model.LiveAdd, Identity: identity, Value: model.ScalarLiveValue("print -- a")}}},
			{OperationID: "publish-b-durable-overlap", Credential: b, AcknowledgedRevision: 1, Delta: []model.LiveChange{{Kind: model.LiveAdd, Identity: identity, Value: model.ScalarLiveValue("print -- b")}}},
		}
		services := []*Service{
			reopenService(t, harness.root, fakeLiveSecretPolicy{}),
			reopenService(t, harness.root, fakeLiveSecretPolicy{}),
		}
		results := make([]model.PublishResult, len(requests))
		errs := make([]error, len(requests))
		start := make(chan struct{})
		var group sync.WaitGroup
		for index := range requests {
			group.Add(1)
			go func(index int) {
				defer group.Done()
				<-start
				results[index], errs[index] = services[index].Publish(context.Background(), requests[index])
			}(index)
		}
		close(start)
		group.Wait()
		accepted := 0
		conflicted := 0
		for index, err := range errs {
			if err != nil {
				t.Fatalf("same-identity result %d error = %v", index, err)
			}
			accepted += len(results[index].Accepted)
			conflicted += len(results[index].Conflicts)
		}
		state := harness.state(t)
		if accepted != 1 || conflicted != 1 || state.HeadRevision != 2 ||
			!reflect.DeepEqual(state.AdmittedIdentities, []model.Identity{identity}) {
			t.Fatalf("same-identity arbitration results=%#v state=%#v", results, state)
		}
	})
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

func TestPreparePullUsesCurrentCaptureForPublishedThenResetTarget(t *testing.T) {
	harness, _ := workflowServiceHarness(t)
	credential := serviceCredential(t, "shell-a", 'A')
	harness.attach(t, credential, "attach-a")
	alias := model.Identity{Kind: model.LiveAlias, Name: "reset.me"}
	harness.publish(t, credential, "publish-alias", 1, model.LiveChange{
		Kind: model.LiveAdd, Identity: alias, Value: model.ScalarLiveValue("print -- dirty"),
	})
	if err := harness.service.ResetHard(context.Background()); err != nil {
		t.Fatal(err)
	}

	pending, err := harness.service.PreparePull(context.Background(), model.PreparePullRequest{
		OperationID: "prepare-reset", Credential: credential, AppliedRevision: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if pending.PendingRevision != 3 || len(pending.Changes) != 1 || pending.Changes[0].Kind != model.LiveRemove || pending.Changes[0].Identity != alias {
		t.Fatalf("reset transition = %#v", pending)
	}
	result, err := harness.service.Acknowledge(context.Background(), model.AcknowledgeRequest{
		OperationID: "ack-reset", Credential: credential, Revision: pending.PendingRevision,
		Token: pending.Token, Snapshot: serviceSnapshot(serviceState("EDITOR", "shared")),
	})
	if err != nil || !result.Acknowledged || result.AppliedRevision != 3 {
		t.Fatalf("reset acknowledge = %#v, %v", result, err)
	}
	shell := harness.state(t).Shells[credential.ShellID]
	if shell.Behind || shell.Pending.Kind != PendingNone || shell.AppliedRevision != 3 || !equalLiveStates(shell.CaptureBaseline, []model.LiveIdentityState{serviceState("EDITOR", "shared")}) {
		t.Fatalf("reset-converged shell = %#v", shell)
	}
}

func TestAcknowledgeCanonicalizesFreshIdentityRecordOrder(t *testing.T) {
	harness := materializedServiceHarness(t, fakeLiveSecretPolicy{})
	a := serviceCredential(t, "shell-a", 'A')
	b := serviceCredential(t, "shell-b", 'B')
	harness.attach(t, a, "attach-a")
	harness.attach(t, b, "attach-b")
	aliasIdentity := model.Identity{Kind: model.LiveAlias, Name: "demo.live"}
	aliasState := model.LiveIdentityState{Identity: aliasIdentity, Value: model.ScalarLiveValue("print -- live")}
	harness.publish(t, a, "publish-alias", 1, model.LiveChange{
		Kind: model.LiveAdd, Identity: aliasIdentity, Value: model.CloneLiveValue(aliasState.Value),
	})

	pending, err := harness.service.PreparePull(context.Background(), model.PreparePullRequest{
		OperationID: "prepare-b", Credential: b, AppliedRevision: 1,
	})
	if err != nil || pending.PendingRevision != 2 {
		t.Fatalf("prepare = %#v, %v", pending, err)
	}
	result, err := harness.service.Acknowledge(context.Background(), model.AcknowledgeRequest{
		OperationID: "ack-b", Credential: b, Revision: pending.PendingRevision, Token: pending.Token,
		Snapshot: serviceSnapshot(aliasState, serviceState("EDITOR", "shared")),
	})
	if err != nil || !result.Acknowledged || result.AppliedRevision != 2 {
		t.Fatalf("reordered acknowledge = %#v, %v", result, err)
	}
	shell := harness.state(t).Shells[b.ShellID]
	if shell.AppliedRevision != 2 || shell.Behind || shell.Pending.Kind != PendingNone {
		t.Fatalf("acknowledged shell = %#v", shell)
	}
}

func TestResolveSharedCanonicalizesFreshIdentityRecordOrderOnly(t *testing.T) {
	t.Run("reordered identity records", func(t *testing.T) {
		harness, credential, conflict, snapshot := canonicalResolveHarness(t)
		result, err := harness.service.ResolveShared(context.Background(), model.ResolveSharedRequest{
			OperationID: "resolve-reordered", Credential: credential, Conflict: conflict.Identity,
			Token: conflict.Token, Snapshot: serviceSnapshot(snapshot...),
		})
		if err != nil || result.PendingRevision != 3 || result.Token != conflict.Token {
			t.Fatalf("reordered resolve = %#v, %v", result, err)
		}
	})

	tests := []struct {
		name   string
		mutate func([]model.LiveIdentityState) []model.LiveIdentityState
	}{
		{"scalar value", func(states []model.LiveIdentityState) []model.LiveIdentityState {
			states[0].Value = model.ScalarLiveValue("changed-loser")
			return states
		}},
		{"presence", func(states []model.LiveIdentityState) []model.LiveIdentityState {
			states[0].Value = model.RemovedLiveValue()
			return states
		}},
		{"identity kind", func(states []model.LiveIdentityState) []model.LiveIdentityState {
			states[0].Identity.Kind = model.LiveAlias
			return states
		}},
		{"identity name", func(states []model.LiveIdentityState) []model.LiveIdentityState {
			states[0].Identity.Name = "OTHER_EDITOR"
			return states
		}},
		{"PATH element order", func(states []model.LiveIdentityState) []model.LiveIdentityState {
			states[1].Value = model.ListLiveValue([]string{"/two", "", "/one", "/one"})
			return states
		}},
		{"FPATH element order", func(states []model.LiveIdentityState) []model.LiveIdentityState {
			states[2].Value = model.ListLiveValue([]string{"/functions/two", "/functions/one", ""})
			return states
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			harness, credential, conflict, snapshot := canonicalResolveHarness(t)
			snapshot = test.mutate(model.CloneLiveStates(snapshot))
			before := harness.bytes(t)
			_, err := harness.service.ResolveShared(context.Background(), model.ResolveSharedRequest{
				OperationID: "resolve-reject", Credential: credential, Conflict: conflict.Identity,
				Token: conflict.Token, Snapshot: serviceSnapshot(snapshot...),
			})
			if !errors.Is(err, ErrConflictRequiresResolution) {
				t.Fatalf("changed freshness resolve = %v", err)
			}
			if after := harness.bytes(t); !bytes.Equal(before, after) {
				t.Fatal("rejected freshness snapshot changed durable state")
			}
		})
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
	t.Run("before-publish-replace", func(t *testing.T) {
		harness := materializedServiceHarness(t, fakeLiveSecretPolicy{})
		a := serviceCredential(t, "shell-a", 'A')
		harness.attach(t, a, "attach-a")
		request := model.PublishRequest{OperationID: "publish-before", Credential: a, AcknowledgedRevision: 1, Delta: []model.LiveChange{serviceUpdate(serviceIdentity("BEFORE"), "retry")}}
		harness.store.faults.beforeRename = func() error { return errors.New("crash before replace") }
		if _, err := harness.service.Publish(context.Background(), request); err == nil {
			t.Fatal("before-replace fault not returned")
		}
		if state := harness.state(t); state.HeadRevision != 1 || serviceReceiptCount(state, "publish-before") != 0 {
			t.Fatalf("pre-replace crash exposed partial transition: %#v", state)
		}
		harness.store.faults.beforeRename = nil
		reopened := reopenService(t, harness.root, fakeLiveSecretPolicy{})
		result, err := reopened.Publish(context.Background(), request)
		if err != nil || result.SharedRevision != 2 {
			t.Fatalf("pre-replace retry = %#v, %v", result, err)
		}
	})

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
		reopened := reopenService(t, harness.root, fakeLiveSecretPolicy{})
		result, err := reopened.Publish(context.Background(), request)
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
		reopened := reopenService(t, harness.root, fakeLiveSecretPolicy{})
		pending, err := reopened.ResolveShared(context.Background(), resolve)
		if err != nil {
			t.Fatal(err)
		}
		ack := model.AcknowledgeRequest{OperationID: "ack-lost", Credential: b, Revision: pending.PendingRevision, Token: pending.Token, Snapshot: serviceSnapshot(serviceState("EDITOR", "winner"))}
		harness.store.faults.afterDirSync = func() error { return errors.New("lost ack response") }
		if _, err := harness.service.Acknowledge(context.Background(), ack); err == nil {
			t.Fatal("lost acknowledgement response not returned")
		}
		harness.store.faults.afterDirSync = nil
		reopenedAgain := reopenService(t, harness.root, fakeLiveSecretPolicy{})
		result, err := reopenedAgain.Acknowledge(context.Background(), ack)
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

func TestStatusDerivesPassiveShellBehindFromCanonicalDurableState(t *testing.T) {
	harness := materializedServiceHarness(t, fakeLiveSecretPolicy{})
	a := serviceCredential(t, "shell-a", 'A')
	b := serviceCredential(t, "shell-b", 'B')
	harness.attach(t, a, "attach-a")
	harness.attach(t, b, "attach-b")

	converged, err := harness.service.WorkflowStatus(context.Background(), b.ShellID)
	if err != nil || converged.Worktree.Shell == nil || converged.Worktree.Shell.Behind {
		t.Fatalf("converged status = %#v, %v", converged, err)
	}
	aliasIdentity := model.Identity{Kind: model.LiveAlias, Name: "passive.receiver"}
	harness.publish(t, a, "publish-passive", 1, model.LiveChange{
		Kind: model.LiveAdd, Identity: aliasIdentity, Value: model.ScalarLiveValue("print -- passive"),
	})
	passive, err := harness.service.WorkflowStatus(context.Background(), b.ShellID)
	if err != nil || passive.Worktree.Shell == nil || !passive.Worktree.Shell.Behind || passive.Worktree.Shell.AppliedRevision != 1 {
		t.Fatalf("passive receiver status = %#v, %v", passive, err)
	}
	if passive.Worktree.Shell.AutoApply != passive.PersistedDefault {
		t.Fatalf("passive status changed auto-apply authority: %#v", passive)
	}
	absent, err := harness.service.WorkflowStatus(context.Background(), "absent-shell")
	if err != nil || absent.Worktree.Shell != nil {
		t.Fatalf("absent shell status = %#v, %v", absent, err)
	}
	if _, err := harness.service.WorkflowStatus(context.Background(), "invalid shell id"); err == nil {
		t.Fatal("invalid shell ID was accepted")
	}

	state := harness.state(t)
	shell := state.Shells[b.ShellID]
	shell.AppliedRevision = state.HeadRevision
	shell.Behind = false
	shell.AppliedBaseline = []model.LiveIdentityState{
		{Identity: aliasIdentity, Value: model.ScalarLiveValue("print -- passive")},
		serviceState("EDITOR", "shared"),
	}
	state.Shells[b.ShellID] = shell
	reordered, err := workflowStatusFromState(state, b.ShellID)
	if err != nil || reordered.Shell == nil || reordered.Shell.Behind {
		t.Fatalf("canonical reordered baseline status = %#v, %v", reordered, err)
	}

	shell.AppliedBaseline[0].Value = model.ScalarLiveValue("print -- mismatch")
	state.Shells[b.ShellID] = shell
	mismatch, err := workflowStatusFromState(state, b.ShellID)
	if err != nil || mismatch.Shell == nil || !mismatch.Shell.Behind {
		t.Fatalf("at-head baseline mismatch status = %#v, %v", mismatch, err)
	}

	shell.AppliedBaseline = model.CloneLiveStates(state.Shared)
	shell.Conflict = &model.Conflict{Kind: model.ConflictOverlap, Identity: aliasIdentity, BaseRevision: 1, SharedRevision: state.HeadRevision, Token: "conflict-token"}
	state.Shells[b.ShellID] = shell
	conflicted, err := workflowStatusFromState(state, b.ShellID)
	if err != nil || conflicted.Shell == nil || !conflicted.Shell.Behind {
		t.Fatalf("conflict truth status = %#v, %v", conflicted, err)
	}

	shell.Conflict = nil
	shell.LastRecoveredError = RecoveredApply
	state.Shells[b.ShellID] = shell
	recovered, err := workflowStatusFromState(state, b.ShellID)
	if err != nil || recovered.Shell == nil || !recovered.Shell.Behind {
		t.Fatalf("recovery truth status = %#v, %v", recovered, err)
	}
}

func TestWorktreeCommitIncludesEveryDirtyIdentityAndClearsOnlyCommittedOverlay(t *testing.T) {
	harness, repository := workflowServiceHarness(t)
	harness.makeDirty(t,
		serviceUpdate(serviceIdentity("EDITOR"), "changed"),
		serviceUpdate(serviceIdentity("NEW_VALUE"), "new"),
	)

	result, err := harness.service.Commit(context.Background(), "commit every dirty identity")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Committed || result.Conflict || result.RecoveryRequired || result.OID != strings.Repeat("b", 40) {
		t.Fatalf("commit result = %#v", result)
	}
	if repository.commitCalls != 1 || repository.lastCommit.branch != "main" || repository.lastCommit.baseOID != strings.Repeat("a", 40) {
		t.Fatalf("repository commit call = %#v", repository.lastCommit)
	}
	wantStates := []model.LiveIdentityState{
		serviceState("EDITOR", "changed"),
		serviceState("NEW_VALUE", "new"),
	}
	if got := repository.lastCommit.document.Projection.States; !reflect.DeepEqual(got, wantStates) {
		t.Fatalf("committed projection = %#v, want %#v", got, wantStates)
	}
	state := harness.state(t)
	if state.BaseOID != result.OID || !reflect.DeepEqual(state.Committed, repository.lastCommit.document) || !equalLiveStates(state.Shared, wantStates) {
		t.Fatalf("committed canonical state = %#v", state)
	}
	if diff, diffErr := harness.service.Diff(context.Background()); diffErr != nil || categorizedDiffCount(diff) != 0 {
		t.Fatalf("post-commit diff = %#v, %v", diff, diffErr)
	}
	clean, err := harness.service.Commit(context.Background(), "must not create an empty commit")
	if err != nil || clean.Committed || repository.commitCalls != 1 {
		t.Fatalf("clean commit = %#v, %v; calls=%d", clean, err, repository.commitCalls)
	}
}

func TestWorktreeCommitConflictPreservesExactDirtyGeneration(t *testing.T) {
	harness, repository := workflowServiceHarness(t)
	harness.makeDirty(t, serviceUpdate(serviceIdentity("EDITOR"), "local"))
	before := harness.state(t)
	repository.commitResult = model.WorktreeCommitResult{Conflict: true}
	repository.commitErr = errors.New("expected base no longer matches")

	result, err := harness.service.Commit(context.Background(), "conflicting commit")
	if err == nil || !result.Conflict || result.Committed || result.RecoveryRequired {
		t.Fatalf("conflict result = %#v, %v", result, err)
	}
	after := harness.state(t)
	if after.BaseOID != before.BaseOID || !reflect.DeepEqual(after.Committed, before.Committed) || !equalLiveStates(after.Shared, before.Shared) {
		t.Fatalf("conflict changed durable generation: before=%#v after=%#v", before, after)
	}
}

func TestBranchUsesCurrentBaseAndCheckoutRejectsDirtyBeforeRepositoryAccess(t *testing.T) {
	harness, repository := workflowServiceHarness(t)
	t.Setenv("ZSHPRO_PROFILE", "stale-profile-marker")
	t.Setenv("ZP_ACTIVE_PROFILE", "stale-active-marker")

	if err := harness.service.Branch(context.Background(), "feature"); err != nil {
		t.Fatal(err)
	}
	if repository.createCalls != 1 || repository.lastCreate.branch != "feature" || repository.lastCreate.baseOID != strings.Repeat("a", 40) {
		t.Fatalf("create from = %#v", repository.lastCreate)
	}
	if state := harness.state(t); state.Branch != "main" || state.BaseOID != strings.Repeat("a", 40) {
		t.Fatalf("branch creation switched durable authority: %#v", state)
	}

	harness.makeDirty(t, serviceUpdate(serviceIdentity("EDITOR"), "dirty"))
	resolveBefore := repository.resolveCalls
	readBefore := repository.readCalls
	if err := harness.service.Checkout(context.Background(), "feature", false); !errors.Is(err, ErrWorktreeDirty) {
		t.Fatalf("dirty checkout = %v", err)
	}
	if repository.resolveCalls != resolveBefore || repository.readCalls != readBefore {
		t.Fatal("dirty checkout accessed repository before rejecting")
	}
	if state := harness.state(t); state.Branch != "main" || !serviceStateHas(state.Shared, serviceIdentity("EDITOR"), "dirty") {
		t.Fatalf("dirty checkout discarded or switched state: %#v", state)
	}
}

func TestCheckoutAndResetHardPublishExactRevisionEvents(t *testing.T) {
	harness, repository := workflowServiceHarness(t)
	targetOID := strings.Repeat("c", 40)
	target := serviceCommitted("feature")
	repository.branches["feature"] = targetOID
	repository.documents[targetOID] = target

	if err := harness.service.Checkout(context.Background(), "feature", false); err != nil {
		t.Fatal(err)
	}
	state := harness.state(t)
	if state.Branch != "feature" || state.BaseOID != targetOID || state.HeadRevision != 2 || len(state.Events) != 2 ||
		!serviceStateHas(state.Shared, serviceIdentity("EDITOR"), "feature") {
		t.Fatalf("checkout state = %#v", state)
	}
	if repository.lastResolvedBranch != "feature" || repository.lastReadRevision != targetOID {
		t.Fatalf("checkout exact reads = branch %q revision %q", repository.lastResolvedBranch, repository.lastReadRevision)
	}

	harness.makeDirty(t, serviceUpdate(serviceIdentity("EDITOR"), "discard-me"), serviceUpdate(serviceIdentity("TEMP"), "discard-me"))
	if err := harness.service.ResetHard(context.Background()); err != nil {
		t.Fatal(err)
	}
	state = harness.state(t)
	if state.Branch != "feature" || state.BaseOID != targetOID || !serviceStateHas(state.Shared, serviceIdentity("EDITOR"), "feature") ||
		serviceStateHas(state.Shared, serviceIdentity("TEMP"), "discard-me") || state.HeadRevision != 4 {
		t.Fatalf("hard reset state = %#v", state)
	}
	if repository.lastReadRevision != targetOID {
		t.Fatalf("reset read revision = %q", repository.lastReadRevision)
	}
}

func TestAutoApplyDefaultPreservesExplicitFalseOverrideAndDurableIdentity(t *testing.T) {
	harness, repository := workflowServiceHarness(t)
	credential := serviceCredential(t, "shell-a", 'A')
	harness.attach(t, credential, "attach-auto")
	override := true
	if err := harness.store.WithTransaction(context.Background(), func(state *State) error {
		shell := state.Shells[credential.ShellID]
		shell.AutoApplyOverride = &override
		state.Shells[credential.ShellID] = shell
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	before := harness.state(t)
	if err := harness.service.SetAutoApplyDefault(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	after := harness.state(t)
	if after.AutoApplyDefault || after.Branch != before.Branch || after.BaseOID != before.BaseOID ||
		after.HeadRevision != before.HeadRevision || !equalLiveStates(after.Shared, before.Shared) ||
		after.Shells[credential.ShellID].AutoApplyOverride == nil || !*after.Shells[credential.ShellID].AutoApplyOverride {
		t.Fatalf("default update changed unrelated authority: before=%#v after=%#v", before, after)
	}
	attached, err := harness.service.WorkflowStatus(context.Background(), credential.ShellID)
	if err != nil || attached.PersistedDefault || !attached.Effective || attached.Source != AutoApplyShellSource {
		t.Fatalf("attached auto status = %#v, %v", attached, err)
	}
	sharedOnly, err := harness.service.WorkflowStatus(context.Background(), "")
	if err != nil || sharedOnly.PersistedDefault || sharedOnly.Effective || sharedOnly.Source != AutoApplyDefaultSource || sharedOnly.Worktree.Shell != nil {
		t.Fatalf("shared-only auto status = %#v, %v", sharedOnly, err)
	}

	second, err := NewService(harness.store, NewRegistry(fakeLiveSecretPolicy{}), repository)
	if err != nil {
		t.Fatal(err)
	}
	other, err := second.WorkflowStatus(context.Background(), credential.ShellID)
	if err != nil || !reflect.DeepEqual(other, attached) {
		t.Fatalf("distinct service status = %#v, %v; want %#v", other, err, attached)
	}
}

func TestRecoveryAfterCommittedStateSaveFailureUsesExactPublishedRevision(t *testing.T) {
	harness, repository := workflowServiceHarness(t)
	harness.makeDirty(t, serviceUpdate(serviceIdentity("EDITOR"), "published"))
	injected := errors.New("state replace failed before rename")
	fired := false
	harness.store.faults.beforeRename = func() error {
		if !fired {
			fired = true
			return injected
		}
		return nil
	}

	result, err := harness.service.Commit(context.Background(), "published before state save")
	if !errors.Is(err, ErrRecoveryRequired) || !result.Committed || !result.RecoveryRequired || result.OID != strings.Repeat("b", 40) {
		t.Fatalf("split result = %#v, %v", result, err)
	}
	harness.store.faults.beforeRename = nil
	old := harness.state(t)
	if old.BaseOID != strings.Repeat("a", 40) {
		t.Fatalf("failed state save claimed clean candidate: %#v", old)
	}

	status, err := harness.service.WorkflowStatus(context.Background(), "")
	if err != nil || status.Worktree.BaseOID != result.OID || status.Worktree.DirtyCount != 0 {
		t.Fatalf("repaired status = %#v, %v", status, err)
	}
	if repository.lastReadRevision != result.OID {
		t.Fatalf("recovery read %q, want exact candidate %q", repository.lastReadRevision, result.OID)
	}
	repaired := harness.state(t)
	if repaired.BaseOID != result.OID || !reflect.DeepEqual(repaired.Committed, repository.documents[result.OID]) {
		t.Fatalf("repaired state = %#v", repaired)
	}
}

func TestWorktreeWorkflowRealGitBoundary(t *testing.T) {
	t.Run("commit branch checkout and reset", func(t *testing.T) {
		harness, repository := realGitWorkflowHarness(t)
		harness.makeDirty(t,
			serviceUpdate(serviceIdentity("EDITOR"), "real-git"),
			serviceUpdate(serviceIdentity("REAL_GIT_NEW"), "present"),
		)
		result, err := harness.service.Commit(context.Background(), "real git complete worktree")
		if err != nil || !result.Committed || result.Conflict || result.RecoveryRequired {
			t.Fatalf("real Git commit = %#v, %v", result, err)
		}
		exact, err := repository.ReadWorktreeRevision(context.Background(), result.OID)
		if err != nil || !equalLiveStates(exact.Projection.States, harness.state(t).Shared) {
			t.Fatalf("real Git exact read = %#v, %v", exact, err)
		}
		if err := harness.service.Branch(context.Background(), "feature"); err != nil {
			t.Fatal(err)
		}
		featureOID, err := repository.ResolveWorktreeRevision(context.Background(), "feature")
		if err != nil || featureOID != result.OID {
			t.Fatalf("real Git branch base = %q, %v; want %q", featureOID, err, result.OID)
		}
		harness.makeDirty(t, serviceUpdate(serviceIdentity("EDITOR"), "discard"))
		if err := harness.service.Checkout(context.Background(), "feature", false); !errors.Is(err, ErrWorktreeDirty) {
			t.Fatalf("real Git dirty checkout = %v", err)
		}
		if err := harness.service.ResetHard(context.Background()); err != nil {
			t.Fatal(err)
		}
		if err := harness.service.Checkout(context.Background(), "feature", false); err != nil {
			t.Fatal(err)
		}
		if state := harness.state(t); state.Branch != "feature" || state.BaseOID != result.OID || !equalLiveStates(state.Shared, exact.Projection.States) {
			t.Fatalf("real Git checkout/reset state = %#v", state)
		}
	})

	t.Run("ref race remains conflict", func(t *testing.T) {
		harness, repository := realGitWorkflowHarness(t)
		state := harness.state(t)
		external := serviceCommitted("external")
		moved, err := repository.CommitWorktree(context.Background(), state.Branch, state.BaseOID, external, "external ref movement")
		if err != nil || !moved.Committed {
			t.Fatalf("external move = %#v, %v", moved, err)
		}
		harness.makeDirty(t, serviceUpdate(serviceIdentity("EDITOR"), "local"))
		before := harness.state(t)
		result, err := harness.service.Commit(context.Background(), "must lose expected-base race")
		if err == nil || !result.Conflict || result.Committed || result.RecoveryRequired {
			t.Fatalf("real Git race = %#v, %v", result, err)
		}
		after := harness.state(t)
		if after.BaseOID != before.BaseOID || !equalLiveStates(after.Shared, before.Shared) {
			t.Fatalf("real Git race changed state: before=%#v after=%#v", before, after)
		}
	})

	t.Run("published ref repairs exact state split", func(t *testing.T) {
		harness, _ := realGitWorkflowHarness(t)
		harness.makeDirty(t, serviceUpdate(serviceIdentity("EDITOR"), "published"))
		fired := false
		harness.store.faults.beforeRename = func() error {
			if !fired {
				fired = true
				return errors.New("injected state replace failure")
			}
			return nil
		}
		result, err := harness.service.Commit(context.Background(), "publish before state split")
		if !errors.Is(err, ErrRecoveryRequired) || !result.Committed || !result.RecoveryRequired {
			t.Fatalf("real Git split = %#v, %v", result, err)
		}
		harness.store.faults.beforeRename = nil
		status, err := harness.service.WorkflowStatus(context.Background(), "")
		if err != nil || status.Worktree.BaseOID != result.OID || status.Worktree.DirtyCount != 0 {
			t.Fatalf("real Git split repair = %#v, %v", status, err)
		}
	})
}

func TestWorktreeEdgeContract(t *testing.T) {
	probes := []struct {
		requirement string
		category    string
		assert      func(*testing.T)
	}{
		{"WORK-01", "idempotency", TestMaterializeIsExactIdempotentAndMismatchRequiresRecovery},
		{"WORK-01", "concurrency", TestAtomicFaultsLeaveOldOrCompleteNewGeneration},
		{"WORK-02", "boundary", TestValidateSnapshotUsesModelBoundsAndRejectsMalformedValues},
		{"WORK-02", "adjacency", TestDiffSnapshotFinalOccurrenceSemanticEqualityAndOrdering},
		{"WORK-02", "empty", TestAttachRequiredBeforePublishAndExactReplayPublishesNoDelta},
		{"WORK-02", "encoding", TestStateRoundTripIsStableBoundedAndDefensivelyCopied},
		{"WORK-02", "ordering", TestFingerprintSnapshotCanonicalIdentityOrder},
		{"WORK-02", "precision", TestStateRevisionTransitionReceiptAndGarbageCollectionValidation},
		{"WORK-03", "adjacency", TestBranchUsesCurrentBaseAndCheckoutRejectsDirtyBeforeRepositoryAccess},
		{"WORK-03", "empty", TestCheckoutAndResetHardPublishExactRevisionEvents},
		{"WORK-03", "ordering", TestStatusAndDiffRemainValueFreeAndUseDurableAuthority},
		{"WORK-03", "idempotency", TestStateStoreNoopTransactionPreservesCanonicalBytes},
		{"WORK-03", "concurrency", TestWorktreeCommitConflictPreservesExactDirtyGeneration},
		{"SYNC-01", "boundary", TestAutoApplyDefaultPreservesExplicitFalseOverrideAndDurableIdentity},
		{"SYNC-01", "adjacency", TestPreparePullAndAcknowledgeRequireExactFreshTarget},
		{"SYNC-01", "empty", TestPreparePullUsesCurrentCaptureForPublishedThenResetTarget},
		{"SYNC-01", "ordering", TestResolveSharedCanonicalizesFreshIdentityRecordOrderOnly},
		{"SYNC-01", "precision", TestResolveSharedRetainsConflictUntilExactAcknowledgement},
		{"SYNC-01", "idempotency", TestCrashReplayPublishResolveAndAcknowledgeIsIdempotent},
		{"SYNC-01", "concurrency", TestAcknowledgePreparedRevisionPreservesConcurrentLaterHead},
		{"SYNC-02", "unclassified-reviewed", TestPrivateCredentialSpoofCrossShellReplayAndReceiptGuessAreValueFree},
	}
	if len(probes) != 21 {
		t.Fatalf("edge probe count = %d, want exactly 21", len(probes))
	}
	seen := make(map[string]bool, len(probes))
	for _, probe := range probes {
		name := probe.requirement + "/" + probe.category
		if seen[name] {
			t.Fatalf("duplicate edge probe %q", name)
		}
		seen[name] = true
		t.Run(name, probe.assert)
	}
}

type serviceHarness struct {
	root    string
	store   *StateStore
	service *Service
}

type workflowRepository struct {
	branches  map[string]string
	documents map[string]model.CommittedWorktree

	commitCalls  int
	createCalls  int
	resolveCalls int
	readCalls    int

	lastCommit struct {
		branch   string
		baseOID  string
		document model.CommittedWorktree
		message  string
	}
	lastCreate struct {
		branch  string
		baseOID string
	}
	lastResolvedBranch string
	lastReadRevision   string
	commitResult       model.WorktreeCommitResult
	commitErr          error
}

type realGitWorkflowRepository struct {
	root  string
	store *storepkg.Store
}

func (repository realGitWorkflowRepository) Branches(ctx context.Context) ([]string, error) {
	return repository.store.Branches(ctx)
}

func (repository realGitWorkflowRepository) ResolveWorktreeRevision(ctx context.Context, branch string) (string, error) {
	command := exec.CommandContext(ctx, "git", "--git-dir", repository.root, "rev-parse", "--verify", "refs/heads/"+branch)
	output, err := command.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func (repository realGitWorkflowRepository) ReadWorktreeRevision(ctx context.Context, revision string) (model.CommittedWorktree, error) {
	return repository.store.ReadWorktreeRevision(ctx, revision)
}

func (repository realGitWorkflowRepository) CreateFrom(ctx context.Context, branch, baseOID string) error {
	return repository.store.CreateFrom(ctx, branch, baseOID)
}

func (repository realGitWorkflowRepository) CommitWorktree(ctx context.Context, branch, baseOID string, document model.CommittedWorktree, message string) (model.WorktreeCommitResult, error) {
	return repository.store.CommitWorktree(ctx, branch, baseOID, document, message)
}

func realGitWorkflowHarness(t *testing.T) (serviceHarness, realGitWorkflowRepository) {
	t.Helper()
	root := privateStateRoot(t)
	repositoryStore, err := storepkg.New(root, zsh.Provider{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := repositoryStore.Init(context.Background()); err != nil {
		t.Fatal(err)
	}
	repository := realGitWorkflowRepository{root: root, store: repositoryStore}
	baseOID, err := repository.ResolveWorktreeRevision(context.Background(), "main")
	if err != nil {
		t.Fatal(err)
	}
	committed := serviceCommitted("shared")
	seed, err := repository.CommitWorktree(context.Background(), "main", baseOID, committed, "seed shared worktree")
	if err != nil || !seed.Committed {
		t.Fatalf("seed real Git worktree = %#v, %v", seed, err)
	}
	stateStore, err := OpenStateStore(root)
	if err != nil {
		t.Fatal(err)
	}
	service, err := NewService(stateStore, NewRegistry(fakeLiveSecretPolicy{}), repository)
	if err != nil {
		_ = stateStore.Close()
		t.Fatal(err)
	}
	if err := service.Materialize(context.Background(), "main", seed.OID, committed); err != nil {
		_ = stateStore.Close()
		t.Fatal(err)
	}
	harness := serviceHarness{root: root, store: stateStore, service: service}
	t.Cleanup(func() { closeStateStore(t, stateStore) })
	return harness, repository
}

func (repository *workflowRepository) Branches(context.Context) ([]string, error) {
	names := make([]string, 0, len(repository.branches))
	for name := range repository.branches {
		names = append(names, name)
	}
	for i := 1; i < len(names); i++ {
		for j := i; j > 0 && names[j-1] > names[j]; j-- {
			names[j-1], names[j] = names[j], names[j-1]
		}
	}
	return names, nil
}

func (repository *workflowRepository) ResolveWorktreeRevision(_ context.Context, branch string) (string, error) {
	repository.resolveCalls++
	repository.lastResolvedBranch = branch
	revision, ok := repository.branches[branch]
	if !ok {
		return "", errors.New("worktree branch is unavailable")
	}
	return revision, nil
}

func (repository *workflowRepository) ReadWorktreeRevision(_ context.Context, revision string) (model.CommittedWorktree, error) {
	repository.readCalls++
	repository.lastReadRevision = revision
	document, ok := repository.documents[revision]
	if !ok {
		return model.CommittedWorktree{}, errors.New("worktree revision is unavailable")
	}
	return model.NewCommittedWorktree(document.Source, document.Projection), nil
}

func (repository *workflowRepository) CreateFrom(_ context.Context, branch, baseOID string) error {
	repository.createCalls++
	repository.lastCreate.branch = branch
	repository.lastCreate.baseOID = baseOID
	if _, exists := repository.branches[branch]; exists {
		return errors.New("worktree branch already exists")
	}
	repository.branches[branch] = baseOID
	return nil
}

func (repository *workflowRepository) CommitWorktree(_ context.Context, branch, baseOID string, document model.CommittedWorktree, message string) (model.WorktreeCommitResult, error) {
	repository.commitCalls++
	repository.lastCommit.branch = branch
	repository.lastCommit.baseOID = baseOID
	repository.lastCommit.document = model.NewCommittedWorktree(document.Source, document.Projection)
	repository.lastCommit.message = message
	result := repository.commitResult
	if result == (model.WorktreeCommitResult{}) && repository.commitErr == nil {
		result = model.WorktreeCommitResult{Committed: true, OID: strings.Repeat("b", 40)}
	}
	if result.Committed {
		repository.branches[branch] = result.OID
		repository.documents[result.OID] = model.NewCommittedWorktree(document.Source, document.Projection)
	}
	return result, repository.commitErr
}

func workflowServiceHarness(t *testing.T) (serviceHarness, *workflowRepository) {
	t.Helper()
	harness := newServiceHarness(t, fakeLiveSecretPolicy{})
	baseOID := strings.Repeat("a", 40)
	committed := serviceCommitted("shared")
	repository := &workflowRepository{
		branches:  map[string]string{"main": baseOID},
		documents: map[string]model.CommittedWorktree{baseOID: committed},
	}
	service, err := NewService(harness.store, NewRegistry(fakeLiveSecretPolicy{}), repository)
	if err != nil {
		t.Fatal(err)
	}
	harness.service = service
	if err := harness.service.Materialize(context.Background(), "main", baseOID, committed); err != nil {
		t.Fatal(err)
	}
	return harness, repository
}

func (h serviceHarness) makeDirty(t *testing.T, changes ...model.LiveChange) {
	t.Helper()
	if err := h.store.WithTransaction(context.Background(), func(state *State) error {
		next, err := model.NextRevision(state.HeadRevision)
		if err != nil {
			return err
		}
		shared, err := applyStateChanges(state.Shared, changes)
		if err != nil {
			return err
		}
		state.Shared = shared
		state.HeadRevision = next
		state.Events = append(state.Events, StateEvent{Revision: next, OperationID: fmt.Sprintf("test-dirty-%d", next), Changes: model.CloneLiveChanges(changes)})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func categorizedDiffCount(diff model.CategorizedDiff) int {
	return len(diff.Environment) + len(diff.Aliases) + len(diff.Functions) + len(diff.Path) + len(diff.FPath) + len(diff.Options)
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

func reopenService(t *testing.T, root string, policy fakeLiveSecretPolicy) *Service {
	t.Helper()
	store, err := OpenStateStore(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { closeStateStore(t, store) })
	service, err := NewService(store, NewRegistry(policy))
	if err != nil {
		t.Fatal(err)
	}
	return service
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

func canonicalResolveHarness(t *testing.T) (serviceHarness, model.ShellCredential, model.Conflict, []model.LiveIdentityState) {
	t.Helper()
	harness := materializedServiceHarness(t, fakeLiveSecretPolicy{})
	a := serviceCredential(t, "shell-a", 'A')
	b := serviceCredential(t, "shell-b", 'B')
	harness.attach(t, a, "attach-a")
	harness.attach(t, b, "attach-b")
	path := model.LiveIdentityState{
		Identity: model.Identity{Kind: model.LivePath, Name: "PATH"},
		Value:    model.ListLiveValue([]string{"", "/one", "/one", "/two"}),
	}
	fpath := model.LiveIdentityState{
		Identity: model.Identity{Kind: model.LiveFPath, Name: "FPATH"},
		Value:    model.ListLiveValue([]string{"/functions/one", "", "/functions/two"}),
	}
	harness.publish(t, a, "publish-lists", 1,
		model.LiveChange{Kind: model.LiveAdd, Identity: path.Identity, Value: model.CloneLiveValue(path.Value)},
		model.LiveChange{Kind: model.LiveAdd, Identity: fpath.Identity, Value: model.CloneLiveValue(fpath.Value)},
	)
	target := []model.LiveIdentityState{fpath, serviceState("EDITOR", "shared"), path}
	for index, credential := range []model.ShellCredential{a, b} {
		pending, err := harness.service.PreparePull(context.Background(), model.PreparePullRequest{
			OperationID: fmt.Sprintf("prepare-lists-%d", index), Credential: credential, AppliedRevision: 1,
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := harness.service.Acknowledge(context.Background(), model.AcknowledgeRequest{
			OperationID: fmt.Sprintf("ack-lists-%d", index), Credential: credential,
			Revision: pending.PendingRevision, Token: pending.Token, Snapshot: serviceSnapshot(target...),
		}); err != nil {
			t.Fatal(err)
		}
	}
	harness.publish(t, a, "winner", 2, serviceUpdate(serviceIdentity("EDITOR"), "winner"))
	loser := harness.publish(t, b, "loser", 2, serviceUpdate(serviceIdentity("EDITOR"), "loser"))
	if len(loser.Conflicts) != 1 {
		t.Fatalf("overlap did not conflict: %#v", loser)
	}
	fresh := []model.LiveIdentityState{serviceState("EDITOR", "loser"), path, fpath}
	return harness, b, loser.Conflicts[0], fresh
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
