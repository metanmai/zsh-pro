package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"zsh-pro/core/dto"
	"zsh-pro/core/model"
	"zsh-pro/core/shell/zsh"
)

type ingestRecordingStore struct{}

func (*ingestRecordingStore) Branches(context.Context) ([]string, error) { return nil, nil }
func (*ingestRecordingStore) Current() string                            { return "main" }
func (*ingestRecordingStore) Checkout(context.Context, string) error     { return nil }
func (*ingestRecordingStore) Read(context.Context, string) (model.Profile, error) {
	return model.Profile{}, nil
}
func (*ingestRecordingStore) BeginIngest(context.Context, model.InstallInitializationID) (model.IngestBeginOutcome, error) {
	return model.IngestBeginOutcome{}, errors.New("unused")
}
func (*ingestRecordingStore) CommitIngest(context.Context, model.InstallInitializationID, model.IngestTransactionID, model.Profile, string) (model.IngestCommitOutcome, error) {
	return model.IngestCommitOutcome{}, errors.New("unused")
}
func (*ingestRecordingStore) AbortIngest(context.Context, model.InstallInitializationID, model.IngestTransactionID) (model.IngestAbortOutcome, error) {
	return model.IngestAbortOutcome{}, errors.New("unused")
}

func TestRunIngestArguments(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	valid := []struct {
		name     string
		args     []string
		wantPath string
		wantJSON bool
	}{
		{name: "default", wantPath: filepath.Join(home, ".zshrc")},
		{name: "path", args: []string{"/tmp/input.zsh"}, wantPath: "/tmp/input.zsh"},
		{name: "json then path", args: []string{"--json", "~/input.zsh"}, wantPath: filepath.Join(home, "input.zsh"), wantJSON: true},
		{name: "path then json", args: []string{"~/input.zsh", "--json"}, wantPath: filepath.Join(home, "input.zsh"), wantJSON: true},
		{name: "repeated json", args: []string{"--json", "--json"}, wantPath: filepath.Join(home, ".zshrc"), wantJSON: true},
	}
	for _, tc := range valid {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := parseIngestArguments(tc.args)
			if !ok {
				t.Fatalf("parseIngestArguments(%q) rejected a valid invocation", tc.args)
			}
			if got.path != tc.wantPath || got.asJSON != tc.wantJSON {
				t.Fatalf("parseIngestArguments(%q) = %#v, want path=%q json=%v", tc.args, got, tc.wantPath, tc.wantJSON)
			}
		})
	}

	for _, args := range [][]string{
		{"--unknown"},
		{"one", "two"},
		{"one", "--unknown", "--json"},
		{"-x", "--json"},
	} {
		if _, ok := parseIngestArguments(args); ok {
			t.Errorf("parseIngestArguments(%q) accepted invalid arguments", args)
		}
	}
}

func TestRunIngestUsageHumanAndJSON(t *testing.T) {
	canary := "CANARY_PATH_OR_FLAG_MUST_NOT_ECHO"
	for _, tc := range []struct {
		name string
		args []string
		json bool
	}{
		{name: "human unknown", args: []string{"ingest", "--" + canary}},
		{name: "human second path", args: []string{"ingest", "first", canary}},
		{name: "json before error", args: []string{"ingest", "--json", "--" + canary}, json: true},
		{name: "json after error", args: []string{"ingest", "--" + canary, "--json"}, json: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := New(nil, nil, nil).Run(tc.args, &stdout, &stderr)
			if code != int(model.ExitUsageErr) {
				t.Fatalf("code = %d, want %d; stdout=%q stderr=%q", code, model.ExitUsageErr, stdout.String(), stderr.String())
			}
			if strings.Contains(stdout.String(), canary) || strings.Contains(stderr.String(), canary) {
				t.Fatalf("usage output disclosed rejected input: stdout=%q stderr=%q", stdout.String(), stderr.String())
			}
			if !tc.json {
				if stdout.Len() != 0 || stderr.String() != ingestUsageLine+"\n" {
					t.Fatalf("human usage = stdout %q stderr %q", stdout.String(), stderr.String())
				}
				return
			}
			if stderr.Len() != 0 {
				t.Fatalf("JSON usage wrote stderr: %q", stderr.String())
			}
			got := decodeOneIngestResult(t, stdout.Bytes())
			want := newIngestResult(int(model.ExitUsageErr))
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("JSON usage = %#v, want %#v", got, want)
			}
		})
	}
}

func TestRunIngestManagedUnmanagedAccountingHumanAndJSON(t *testing.T) {
	lines := 5
	result := newIngestResult(int(model.ExitClean))
	result.OK = true
	result.ProfileCommitted = true
	result.StartupInstalled = true
	result.ManagedEntries = 2
	result.UnmanagedStatements = 1
	result.UnmanagedSourceLines = &lines
	result.SourceStatements = 3
	result.AccountedStatements = 3
	result.Withheld = []dto.IngestWithheld{{Name: "TOKEN", StartLine: 7}}
	result.Warnings = []string{ingestPostEndWarning}

	var humanOut, humanErr bytes.Buffer
	if code := renderIngestResult(result, ingestFailureNone, false, &humanOut, &humanErr); code != int(model.ExitClean) {
		t.Fatalf("human code = %d", code)
	}
	if humanErr.Len() != 0 {
		t.Fatalf("human success wrote stderr: %q", humanErr.String())
	}
	wantHuman := "zsh-pro: ingest complete\n" +
		"profile committed: yes\n" +
		"startup installed: yes\n" +
		"recovery required: no\n" +
		"managed entries: 2\n" +
		"unmanaged statements: 1\n" +
		"unmanaged source lines: 5\n" +
		"source statements: 3\n" +
		"accounted statements: 3\n" +
		"withheld: TOKEN (line 7)\n" +
		"warning: " + ingestPostEndWarning + "\n"
	if humanOut.String() != wantHuman {
		t.Fatalf("human result =\n%s\nwant:\n%s", humanOut.String(), wantHuman)
	}

	var jsonOut, jsonErr bytes.Buffer
	if code := renderIngestResult(result, ingestFailureNone, true, &jsonOut, &jsonErr); code != int(model.ExitClean) {
		t.Fatalf("JSON code = %d", code)
	}
	if jsonErr.Len() != 0 {
		t.Fatalf("JSON success wrote stderr: %q", jsonErr.String())
	}
	if got := decodeOneIngestResult(t, jsonOut.Bytes()); !reflect.DeepEqual(got, result) {
		t.Fatalf("JSON result = %#v, want %#v", got, result)
	}
	if result.ManagedEntries+result.UnmanagedStatements != result.AccountedStatements || result.AccountedStatements != result.SourceStatements {
		t.Fatal("fixture does not prove statement accounting invariant")
	}
}

func TestRunIngestSafeFailureMatrix(t *testing.T) {
	reasons := []ingestFailureReason{
		ingestFailureProviderUnavailable,
		ingestFailureCapabilityUnavailable,
		ingestFailureStoreUnavailable,
		ingestFailureSourceUnavailable,
		ingestFailureSourceInvalid,
		ingestFailureTransactionUnavailable,
		ingestFailureStartupConflict,
		ingestFailureStoreConflict,
		ingestFailureNotCommitted,
		ingestFailureRecoveryRequired,
		ingestFailureFinalizeFailed,
	}
	canary := "CANARY_RAW_ERROR_PATH_REF_OBJECT_BACKEND_VALUE"
	for _, reason := range reasons {
		t.Run(reason.String(), func(t *testing.T) {
			result := newIngestResult(int(model.ExitRuntimeErr))
			result.ManagedEntries = 1
			result.UnmanagedStatements = 2
			result.SourceStatements = 3
			result.AccountedStatements = 3

			for _, asJSON := range []bool{false, true} {
				var stdout, stderr bytes.Buffer
				code := renderIngestResult(result, reason, asJSON, &stdout, &stderr)
				if code != int(model.ExitRuntimeErr) {
					t.Fatalf("reason %q json=%v code=%d", reason, asJSON, code)
				}
				combined := stdout.String() + stderr.String()
				if strings.Contains(combined, canary) {
					t.Fatalf("reason %q disclosed canary: %q", reason, combined)
				}
				if strings.Contains(combined, "/") || strings.Contains(combined, "refs/") || strings.Contains(combined, "backend") {
					t.Fatalf("reason %q emitted forbidden internal detail: %q", reason, combined)
				}
				if asJSON {
					if stderr.Len() != 0 {
						t.Fatalf("reason %q JSON wrote stderr: %q", reason, stderr.String())
					}
					if got := decodeOneIngestResult(t, stdout.Bytes()); !reflect.DeepEqual(got, result) {
						t.Fatalf("reason %q JSON = %#v, want %#v", reason, got, result)
					}
				} else if stdout.Len() != 0 || !strings.Contains(stderr.String(), reason.String()) {
					t.Fatalf("reason %q human streams = stdout %q stderr %q", reason, stdout.String(), stderr.String())
				}
			}
		})
	}

	// The CLI usage path receives hostile argument text, but its closed renderer
	// has no raw-error parameter and therefore cannot echo it.
	var stdout, stderr bytes.Buffer
	if code := New(nil, nil, nil).Run([]string{"ingest", "--" + canary, "--json"}, &stdout, &stderr); code != int(model.ExitUsageErr) {
		t.Fatalf("usage code = %d", code)
	}
	if strings.Contains(stdout.String()+stderr.String(), canary) {
		t.Fatalf("usage renderer disclosed canary: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestCLIUsesOneInjectedStoreInstance(t *testing.T) {
	store := &ingestRecordingStore{}
	cli := newCLI(zsh.Provider{}, store, NotReadyEmitter(), func(context.Context) (StoreInitialization, error) {
		return StoreInitialization{Transactions: store}, nil
	})
	if cli.store != store {
		t.Fatal("CLI did not retain the exact injected Store instance")
	}
	initialized, err := cli.storeInitializer(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if initialized.Transactions != store {
		t.Fatal("initializer did not retain the exact injected transaction Store instance")
	}
}

type controllerStore struct {
	initializationID  model.InstallInitializationID
	commitStatus      model.IngestCommitStatus
	commitError       error
	autoWithhold      bool
	commits           []model.Profile
	messages          []string
	beginCalls        int
	commitCalls       int
	abortCalls        int
	events            *[]string
	onCommit          func(model.Profile)
	publishedRevision string
	materializeCalls  int
	materializeBranch string
	materializeOID    string
	materializeError  error
}

func newControllerStore(t *testing.T, events *[]string) *controllerStore {
	t.Helper()
	initializationID, err := model.NewInstallInitializationID()
	if err != nil {
		t.Fatal(err)
	}
	return &controllerStore{
		initializationID:  initializationID,
		commitStatus:      model.IngestCommitCommitted,
		publishedRevision: strings.Repeat("a", 40),
		events:            events,
	}
}

func (*controllerStore) Branches(context.Context) ([]string, error) { return []string{"main"}, nil }
func (*controllerStore) Current() string                            { return "main" }
func (*controllerStore) Checkout(context.Context, string) error     { return nil }
func (*controllerStore) Read(context.Context, string) (model.Profile, error) {
	return model.Profile{}, nil
}

func (store *controllerStore) BeginIngest(context.Context, model.InstallInitializationID) (model.IngestBeginOutcome, error) {
	store.beginCalls++
	store.record("store:begin-call")
	transactionID, err := model.NewIngestTransactionID()
	if err != nil {
		return model.IngestBeginOutcome{}, err
	}
	baseline, err := model.NewIngestBaseline(store.initializationID, transactionID, false, nil, model.Profile{}, false)
	if err != nil {
		return model.IngestBeginOutcome{}, err
	}
	return model.IngestBeginOutcome{
		InitializationID:        store.initializationID,
		TransactionID:           transactionID,
		Baseline:                baseline,
		Lifecycle:               model.IngestLifecycleActive,
		Cleanup:                 model.QuarantineCleanupRetained,
		FailureCode:             model.IngestFailureNone,
		InitializerRollbackSafe: false,
	}, nil
}

func (store *controllerStore) CommitIngest(
	_ context.Context,
	initializationID model.InstallInitializationID,
	transactionID model.IngestTransactionID,
	profile model.Profile,
	message string,
) (model.IngestCommitOutcome, error) {
	store.commitCalls++
	store.record("store:commit-call")
	store.commits = append(store.commits, cloneControllerProfile(profile))
	store.messages = append(store.messages, message)
	if store.onCommit != nil {
		store.onCommit(profile)
	}
	outcome := model.IngestCommitOutcome{
		InitializationID:        initializationID,
		TransactionID:           transactionID,
		Status:                  store.commitStatus,
		RefState:                model.IngestRefCandidate,
		Backend:                 model.IngestBackendUnchanged,
		Objects:                 model.IngestObjectsPublished,
		Cleanup:                 model.QuarantineCleanupRemoved,
		FailureCode:             model.IngestFailureNone,
		InitializerRollbackSafe: true,
	}
	switch store.commitStatus {
	case model.IngestCommitCommitted:
		revision := store.publishedRevision
		outcome.PublishedRevision = &revision
		if store.autoWithhold {
			for _, entry := range profile.Entries {
				if entry.Category == model.CatSecrets && entry.Secret == nil && !entry.Dynamic {
					outcome.Withheld = append(outcome.Withheld, model.WithheldSecret{Name: entry.Names[0], StartLine: entry.StartLine})
				}
			}
		}
	case model.IngestCommitConflict:
		outcome.RefState = model.IngestRefOther
		outcome.Objects = model.IngestObjectsRemoved
		outcome.FailureCode = model.IngestFailureRefPrepare
	case model.IngestCommitRecoveryRequired:
		outcome.RefState = model.IngestRefUnknown
		outcome.Objects = model.IngestObjectsRetained
		outcome.Cleanup = model.QuarantineCleanupRetained
		outcome.FailureCode = model.IngestFailureRefCommit
		outcome.RecoveryRequired = true
		outcome.InitializerRollbackSafe = false
	default:
		outcome.RefState = model.IngestRefExpected
		outcome.Objects = model.IngestObjectsRemoved
		outcome.FailureCode = model.IngestFailureCandidate
	}
	return outcome, store.commitError
}

func (store *controllerStore) MaterializeCommittedWorktree(_ context.Context, branch, oid string) error {
	store.materializeCalls++
	store.materializeBranch = branch
	store.materializeOID = oid
	return store.materializeError
}

func (store *controllerStore) AbortIngest(
	_ context.Context,
	initializationID model.InstallInitializationID,
	transactionID model.IngestTransactionID,
) (model.IngestAbortOutcome, error) {
	store.abortCalls++
	store.record("store:abort-call")
	return model.IngestAbortOutcome{
		InitializationID:        initializationID,
		TransactionID:           transactionID,
		Lifecycle:               model.IngestLifecycleTerminal,
		Cleanup:                 model.QuarantineCleanupRemoved,
		FailureCode:             model.IngestFailureNone,
		InitializerRollbackSafe: true,
	}, nil
}

func (store *controllerStore) record(event string) {
	if store.events != nil {
		*store.events = append(*store.events, event)
	}
}

func cloneControllerProfile(profile model.Profile) model.Profile {
	return model.Profile{Entries: append([]model.Entry(nil), profile.Entries...)}
}

type ingestControllerFixture struct {
	t          *testing.T
	home       string
	target     string
	runtimeDir string
	storeRoot  string
	program    *CLI
	store      *controllerStore
	events     []string
	seams      ingestControllerSeams
	rollbacks  int
	finalizes  int
}

func newIngestControllerFixture(t *testing.T, source string) *ingestControllerFixture {
	t.Helper()
	if runtimeUnsupportedForIngestTest() {
		t.Skip("guarded startup transactions require Linux or Darwin with zsh")
	}
	home := t.TempDir()
	setInstallHome(t, home)
	target := filepath.Join(home, ".zshrc")
	if err := os.WriteFile(target, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	fixture := &ingestControllerFixture{
		t:          t,
		home:       home,
		target:     target,
		runtimeDir: filepath.Join(home, ".zsh-pro"),
		storeRoot:  filepath.Join(home, ".local", "share", "zsh-pro"),
	}
	fixture.store = newControllerStore(t, &fixture.events)
	fixture.seams = ingestControllerSeams{
		event: func(event string) { fixture.events = append(fixture.events, event) },
		install: &installTransactionSeams{
			event: func(event string) { fixture.events = append(fixture.events, event) },
		},
	}
	initializer := func(context.Context) (StoreInitialization, error) {
		fixture.events = append(fixture.events, "initializer:call")
		return StoreInitialization{
			InitializationID: fixture.store.initializationID,
			Transactions:     fixture.store,
			CanonicalRoot:    fixture.storeRoot,
			Rollback: func() error {
				fixture.rollbacks++
				fixture.events = append(fixture.events, "initializer:rollback-call")
				return nil
			},
			Finalize: func() error {
				fixture.finalizes++
				fixture.events = append(fixture.events, "initializer:finalize-call")
				return nil
			},
		}, nil
	}
	fixture.program = newCLI(zsh.Provider{}, fixture.store, NotReadyEmitter(), initializer)
	return fixture
}

func runtimeUnsupportedForIngestTest() bool {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		return true
	}
	if _, err := exec.LookPath("zsh"); err != nil {
		return true
	}
	return false
}

func (fixture *ingestControllerFixture) runJSON() (int, dto.IngestResult, string) {
	fixture.t.Helper()
	var stdout, stderr bytes.Buffer
	code := fixture.program.runIngestWithSeams(
		context.Background(),
		ingestArguments{path: fixture.target, asJSON: true},
		&stdout,
		&stderr,
		&fixture.seams,
	)
	if stderr.Len() != 0 {
		fixture.t.Fatalf("JSON ingest wrote stderr: %q", stderr.String())
	}
	return code, decodeOneIngestResult(fixture.t, stdout.Bytes()), stdout.String()
}

func (fixture *ingestControllerFixture) requireCommitted() dto.IngestResult {
	fixture.t.Helper()
	code, result, _ := fixture.runJSON()
	if code != int(model.ExitClean) || !result.OK || !result.ProfileCommitted || !result.StartupInstalled || result.RecoveryRequired {
		fixture.t.Fatalf("committed ingest = code %d result %#v events=%v", code, result, fixture.events)
	}
	return result
}

func eventIndex(events []string, want string) int {
	for index, event := range events {
		if event == want {
			return index
		}
	}
	return -1
}

func eventCount(events []string, want string) int {
	count := 0
	for _, event := range events {
		if event == want {
			count++
		}
	}
	return count
}

func TestRunIngestTransactionOutcomeMatrix(t *testing.T) {
	for _, tc := range []struct {
		name      string
		status    model.IngestCommitStatus
		wantCode  int
		committed bool
		recovery  bool
	}{
		{name: "committed", status: model.IngestCommitCommitted, wantCode: int(model.ExitClean), committed: true},
		{name: "conflict", status: model.IngestCommitConflict, wantCode: int(model.ExitRuntimeErr)},
		{name: "not committed", status: model.IngestCommitNotCommitted, wantCode: int(model.ExitRuntimeErr)},
		{name: "recovery", status: model.IngestCommitRecoveryRequired, wantCode: int(model.ExitRuntimeErr), recovery: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newIngestControllerFixture(t, "export EDITOR=nvim\n")
			fixture.store.commitStatus = tc.status
			if tc.status != model.IngestCommitCommitted {
				fixture.store.commitError = errors.New("CANARY_RAW_COMMIT_ERROR")
			}
			code, result, encoded := fixture.runJSON()
			if code != tc.wantCode || result.ProfileCommitted != tc.committed || result.RecoveryRequired != tc.recovery {
				t.Fatalf("outcome = code %d result %#v events=%v", code, result, fixture.events)
			}
			if strings.Contains(encoded, "CANARY") {
				t.Fatalf("safe output disclosed Store error: %s", encoded)
			}
		})
	}

	t.Run("committed empty baseline and deterministic re-ingest", func(t *testing.T) {
		fixture := newIngestControllerFixture(t, "")
		fixture.requireCommitted()
		fixture.requireCommitted()
		if len(fixture.store.commits) != 2 || len(fixture.store.commits[0].Entries) != 0 || len(fixture.store.commits[1].Entries) != 0 {
			t.Fatalf("empty re-ingest commits = %#v", fixture.store.commits)
		}
		installed, err := os.ReadFile(fixture.target)
		if err != nil || bytes.Count(installed, []byte(installBegin)) != 1 || bytes.Count(installed, []byte(installEnd)) != 1 {
			t.Fatalf("empty re-ingest startup = %q, err=%v", installed, err)
		}
	})
}

func TestPublishedRevisionEvidenceValidation(t *testing.T) {
	store := newControllerStore(t, nil)
	transactionID, err := model.NewIngestTransactionID()
	if err != nil {
		t.Fatal(err)
	}
	revision := strings.Repeat("a", 40)
	base := model.IngestCommitOutcome{
		InitializationID:  store.initializationID,
		TransactionID:     transactionID,
		Status:            model.IngestCommitCommitted,
		PublishedRevision: &revision,
		RefState:          model.IngestRefCandidate,
		Objects:           model.IngestObjectsPublished,
	}
	if !validIngestCommit(store.initializationID, transactionID, base) {
		t.Fatal("authoritative published revision was rejected")
	}

	missing := base
	missing.PublishedRevision = nil
	if validIngestCommit(store.initializationID, transactionID, missing) {
		t.Fatal("committed evidence without an exact published revision was accepted")
	}

	invalid := base
	badRevision := "not-an-object-id"
	invalid.PublishedRevision = &badRevision
	if validIngestCommit(store.initializationID, transactionID, invalid) {
		t.Fatal("malformed published revision was accepted")
	}

	conflict := base
	conflict.Status = model.IngestCommitConflict
	conflict.RefState = model.IngestRefOther
	if validIngestCommit(store.initializationID, transactionID, conflict) {
		t.Fatal("non-authoritative conflict carried published evidence")
	}
}

func TestRunIngestMaterializesExactPublishedRevisionAfterFinalize(t *testing.T) {
	fixture := newIngestControllerFixture(t, "export EDITOR=nvim\n")
	fixture.requireCommitted()
	if fixture.store.materializeCalls != 1 || fixture.store.materializeBranch != "main" ||
		fixture.store.materializeOID != fixture.store.publishedRevision {
		t.Fatalf("materialization = calls %d branch %q oid %q", fixture.store.materializeCalls,
			fixture.store.materializeBranch, fixture.store.materializeOID)
	}
	if eventIndex(fixture.events, "worktree:materialize") <= eventIndex(fixture.events, "initializer:finalize-call") {
		t.Fatalf("materialization preceded finalize: %v", fixture.events)
	}
}

func TestRunIngestMaterializationFailurePreservesPublicationTruth(t *testing.T) {
	const canary = "RAW_MATERIALIZER_CANARY"
	fixture := newIngestControllerFixture(t, "export EDITOR=nvim\n")
	fixture.store.materializeError = errors.New(canary)
	code, result, encoded := fixture.runJSON()
	if code != int(model.ExitRuntimeErr) || !result.ProfileCommitted || !result.StartupInstalled ||
		!result.RecoveryRequired || fixture.store.materializeCalls != 1 {
		t.Fatalf("materialization split = code %d result %#v calls %d", code, result, fixture.store.materializeCalls)
	}
	if strings.Contains(encoded, canary) || strings.Contains(encoded, fixture.store.publishedRevision) {
		t.Fatalf("materialization split disclosed internal evidence: %s", encoded)
	}
}

func TestRunIngestNoncommitNeverMaterializes(t *testing.T) {
	fixture := newIngestControllerFixture(t, "export EDITOR=nvim\n")
	fixture.store.commitStatus = model.IngestCommitConflict
	fixture.store.commitError = errors.New("conflict")
	code, result, _ := fixture.runJSON()
	if code != int(model.ExitRuntimeErr) || result.ProfileCommitted || fixture.store.materializeCalls != 0 {
		t.Fatalf("noncommit materialization = code %d result %#v calls %d", code, result, fixture.store.materializeCalls)
	}
}

func TestRunIngestCommitsCompleteOrderedProfile(t *testing.T) {
	fixture := newIngestControllerFixture(t, "export EDITOR=nvim\necho imperative\nalias ll='ls -l'\n")
	fixture.requireCommitted()
	if len(fixture.store.commits) != 1 || len(fixture.store.commits[0].Entries) != 3 {
		t.Fatalf("committed profile = %#v", fixture.store.commits)
	}
	got := fixture.store.commits[0].Entries
	want := []string{"export EDITOR=nvim", "echo imperative", "alias ll='ls -l'"}
	for index := range want {
		if got[index].Text != want[index] {
			t.Fatalf("entry %d text = %q, want %q", index, got[index].Text, want[index])
		}
	}
	if fixture.store.messages[0] != "zsh-pro: ingest baseline" {
		t.Fatalf("commit message = %q", fixture.store.messages[0])
	}
}

func TestRunIngestEffectiveManagedIsProjectionOnly(t *testing.T) {
	fixture := newIngestControllerFixture(t, "export EDITOR=nvim\necho imperative\n")
	fixture.seams.afterBuild = func(profile *model.Profile) {
		profile.Entries[0].Override = model.OverrideUnmanaged
		profile.Entries[1].Override = model.OverrideManaged
	}
	result := fixture.requireCommitted()
	if result.ManagedEntries != 1 || result.UnmanagedStatements != 1 || len(fixture.store.commits[0].Entries) != 2 {
		t.Fatalf("projection/filter result = %#v profile=%#v", result, fixture.store.commits[0])
	}
	if fixture.store.commits[0].Entries[0].Override != model.OverrideUnmanaged || fixture.store.commits[0].Entries[1].Override != model.OverrideManaged {
		t.Fatal("complete profile lost projection overrides")
	}
}

func TestRunIngestPromotesStartupExactlyOnce(t *testing.T) {
	fixture := newIngestControllerFixture(t, "export EDITOR=nvim\n")
	fixture.requireCommitted()
	if got := eventCount(fixture.events, "target:namespace"); got != 1 {
		t.Fatalf("target namespace promotions = %d, want 1; events=%v", got, fixture.events)
	}
	if eventIndex(fixture.events, "loader:promote") > eventIndex(fixture.events, "target:promote") {
		t.Fatalf("loader did not precede target: %v", fixture.events)
	}
}

func TestRunIngestCommitFinalizesWithoutTargetRewrite(t *testing.T) {
	fixture := newIngestControllerFixture(t, "export EDITOR=nvim\n")
	var atCommit os.FileInfo
	fixture.store.onCommit = func(model.Profile) {
		atCommit, _ = os.Stat(fixture.target)
	}
	fixture.requireCommitted()
	after, err := os.Stat(fixture.target)
	if err != nil || atCommit == nil || !os.SameFile(atCommit, after) {
		t.Fatalf("post-commit finalize rewrote target: before=%v after=%v err=%v", atCommit, after, err)
	}
	if eventCount(fixture.events, "target:namespace") != 1 || eventIndex(fixture.events, "store:commit-call") > eventIndex(fixture.events, "target:finalize") {
		t.Fatalf("commit/finalize order = %v", fixture.events)
	}
}

func TestRunIngestStoreNoncommitCompensatesFilesystemFirst(t *testing.T) {
	source := "export EDITOR=nvim\n"
	fixture := newIngestControllerFixture(t, source)
	fixture.store.commitStatus = model.IngestCommitConflict
	fixture.store.commitError = errors.New("conflict")
	code, result, _ := fixture.runJSON()
	if code != int(model.ExitRuntimeErr) || result.StartupInstalled || result.ProfileCommitted {
		t.Fatalf("noncommit result = %#v", result)
	}
	if fixture.store.commitCalls != 1 || eventIndex(fixture.events, "filesystem:rollback") < 0 ||
		eventIndex(fixture.events, "filesystem:rollback") < eventIndex(fixture.events, "store:commit-call") ||
		eventIndex(fixture.events, "filesystem:rollback") > eventIndex(fixture.events, "loader:rollback") || fixture.store.abortCalls != 0 {
		t.Fatalf("noncommit compensation order = %v aborts=%d", fixture.events, fixture.store.abortCalls)
	}
	if got, err := os.ReadFile(fixture.target); err != nil || string(got) != source {
		t.Fatalf("noncommit target = %q, err=%v", got, err)
	}
}

func TestRunIngestPreCommitCompensationFilesystemFirst(t *testing.T) {
	source := "export EDITOR=nvim\n"
	fixture := newIngestControllerFixture(t, source)
	fixture.seams.beforeCommit = func(*guardedInstallTransaction) error { return errors.New("precommit failure") }
	code, result, _ := fixture.runJSON()
	if code != int(model.ExitRuntimeErr) || result.RecoveryRequired || fixture.store.commitCalls != 0 || fixture.store.abortCalls != 1 {
		t.Fatalf("precommit result = %#v commits=%d aborts=%d", result, fixture.store.commitCalls, fixture.store.abortCalls)
	}
	order := []string{"filesystem:rollback", "store:abort", "loader:rollback", "cache:rollback", "initializer:rollback"}
	previous := -1
	for _, event := range order {
		index := eventIndex(fixture.events, event)
		if index <= previous {
			t.Fatalf("compensation order %q index=%d previous=%d events=%v", event, index, previous, fixture.events)
		}
		previous = index
	}
}

func TestRunIngestUnsafeFilesystemDisarmsLaterCompensation(t *testing.T) {
	fixture := newIngestControllerFixture(t, "export EDITOR=nvim\n")
	fixture.seams.install.syncDirectory = func(file *os.File, stage string) error {
		if stage == "journal-directory:parent_synced" {
			return errors.New("sync uncertainty")
		}
		return file.Sync()
	}
	code, result, _ := fixture.runJSON()
	if code != int(model.ExitRuntimeErr) || !result.RecoveryRequired {
		t.Fatalf("unsafe filesystem result = %#v events=%v", result, fixture.events)
	}
	for _, forbidden := range []string{"store:abort", "loader:rollback", "cache:rollback", "initializer:rollback"} {
		if eventIndex(fixture.events, forbidden) >= 0 {
			t.Fatalf("unsafe filesystem allowed %q: %v", forbidden, fixture.events)
		}
	}
}

func TestRunIngestSeparatesOriginalAndCandidateEvidence(t *testing.T) {
	fixture := newIngestControllerFixture(t, "export EDITOR=nvim\n")
	fixture.seams.beforeCommit = func(transaction *guardedInstallTransaction) error {
		if transaction == nil || transaction.expectedTarget.digest == ([32]byte{}) || transaction.expectedCandidate.Digest == "" {
			return errors.New("missing evidence axis")
		}
		if fmt.Sprintf("%x", transaction.expectedTarget.digest) == transaction.expectedCandidate.Digest {
			return errors.New("original and candidate evidence collapsed")
		}
		return nil
	}
	fixture.requireCommitted()
}

func TestRunIngestPostEndWarningRetainsProfileAndBytes(t *testing.T) {
	source := "export BEFORE=1\n" + string(renderInstallBlock()) + "\nexport AFTER=2\n"
	fixture := newIngestControllerFixture(t, source)
	result := fixture.requireCommitted()
	if len(result.Warnings) != 1 || result.Warnings[0] != ingestPostEndWarning || len(fixture.store.commits[0].Entries) != 2 {
		t.Fatalf("post-END result = %#v profile=%#v", result, fixture.store.commits[0])
	}
	installed, err := os.ReadFile(fixture.target)
	if err != nil || !bytes.HasSuffix(installed, []byte("\nexport AFTER=2\n")) {
		t.Fatalf("post-END bytes moved or changed: %q, err=%v", installed, err)
	}
}

func TestRunIngestSourceLiteralMayRecapture(t *testing.T) {
	secret := "literal-value-must-not-render"
	fixture := newIngestControllerFixture(t, "export API_TOKEN='"+secret+"'\n")
	fixture.store.autoWithhold = true
	code, result, encoded := fixture.runJSON()
	if code != int(model.ExitClean) || len(result.Withheld) != 1 || result.Withheld[0].Name != "API_TOKEN" {
		t.Fatalf("literal result = %#v profile=%#v", result, fixture.store.commits)
	}
	entry := fixture.store.commits[0].Entries[0]
	if entry.Secret != nil || entry.Dynamic || !strings.Contains(entry.Text, secret) {
		t.Fatalf("source parser fabricated persisted reference: %#v", entry)
	}
	if strings.Contains(encoded, secret) {
		t.Fatalf("literal leaked to output: %s", encoded)
	}
}

func TestRunIngestProgrammaticSecretRefPassThroughIsSeparate(t *testing.T) {
	fixture := newIngestControllerFixture(t, "export API_TOKEN=source-literal\n")
	fixture.seams.afterBuild = func(profile *model.Profile) {
		ref := &model.SecretRef{Kind: model.SecretRefFile, Key: "API_TOKEN"}
		profile.Entries[0].Secret = ref
		profile.Entries[0].Text = "export API_TOKEN='zsh-pro-secret:file:API_TOKEN'"
		profile.Entries[0].Value = "'zsh-pro-secret:file:API_TOKEN'"
		profile.Entries[0].RuntimeValue = nil
		profile.Entries[0].Dynamic = false
		profile.Entries[0].ValueMode = model.ValueModeUnsupported
	}
	result := fixture.requireCommitted()
	entry := fixture.store.commits[0].Entries[0]
	if entry.Secret == nil || entry.Secret.Kind != model.SecretRefFile || len(result.Withheld) != 0 {
		t.Fatalf("programmatic pass-through = result %#v entry %#v", result, entry)
	}
}

func TestRunIngestDynamicEntryRerunPreservesFieldsAndReporting(t *testing.T) {
	source := "export API_TOKEN=$HOME/token\n"
	fixture := newIngestControllerFixture(t, source)
	result := fixture.requireCommitted()
	entry := fixture.store.commits[0].Entries[0]
	if entry.Text != strings.TrimSuffix(source, "\n") || !entry.Dynamic || entry.ValueMode != model.ValueModeDynamic || entry.Secret != nil || len(result.Withheld) != 0 {
		t.Fatalf("dynamic rerun = result %#v entry %#v", result, entry)
	}
}

func TestRunIngestManagedUnmanagedMultilineAccounting(t *testing.T) {
	fixture := newIngestControllerFixture(t, "export EDITOR=nvim\nif true; then\n  print -r -- hi\nfi\n")
	result := fixture.requireCommitted()
	if result.ManagedEntries != 1 || result.UnmanagedStatements != 1 || result.SourceStatements != 2 ||
		result.AccountedStatements != 2 || result.UnmanagedSourceLines == nil || *result.UnmanagedSourceLines != 3 {
		t.Fatalf("multiline accounting = %#v profile=%#v", result, fixture.store.commits[0])
	}
}

func TestRunIngestDifferentTargetsPrepareConflict(t *testing.T) {
	fixture := newIngestControllerFixture(t, "export FIRST=1\n")
	other := filepath.Join(fixture.home, "other.zshrc")
	if err := os.WriteFile(other, []byte("export SECOND=2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	fixture.target = other
	fixture.store.commitStatus = model.IngestCommitConflict
	fixture.store.commitError = errors.New("different prepared target lost main CAS")
	code, result, _ := fixture.runJSON()
	if code != int(model.ExitRuntimeErr) || result.ProfileCommitted || result.StartupInstalled ||
		fixture.store.commitCalls != 1 || eventIndex(fixture.events, "filesystem:rollback") < 0 {
		t.Fatalf("different-target conflict = %#v", result)
	}
	if got, err := os.ReadFile(other); err != nil || string(got) != "export SECOND=2\n" {
		t.Fatalf("losing target not restored: %q err=%v", got, err)
	}
}

func TestRunIngestExplicitSharedRoot(t *testing.T) {
	fixture := newIngestControllerFixture(t, "export EDITOR=nvim\n")
	shared := fixture.runtimeDir
	t.Setenv("ZSHPRO_HOME", shared)
	fixture.storeRoot = shared
	fixture.requireCommitted()
	if fixture.rollbacks != 0 || fixture.finalizes != 1 {
		t.Fatalf("shared-root terminal evidence = rollbacks %d finalizes %d", fixture.rollbacks, fixture.finalizes)
	}
}

func TestRunIngestNeverExecutesSource(t *testing.T) {
	fixture := newIngestControllerFixture(t, "")
	sentinel := filepath.Join(fixture.home, "must-not-exist")
	source := "export SAFE=$(touch " + sentinel + ")\n"
	if err := os.WriteFile(fixture.target, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	fixture.requireCommitted()
	if _, err := os.Lstat(sentinel); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("ingest executed command substitution: %v", err)
	}
	if len(fixture.store.commits) != 1 || fixture.store.commits[0].Entries[0].Text != strings.TrimSuffix(source, "\n") {
		t.Fatalf("static source was not committed verbatim: %#v", fixture.store.commits)
	}
}

func TestRunIngestTargetLockCancellationHasNoLaterEffects(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("target-root locking requires Linux or Darwin")
	}
	home := t.TempDir()
	setInstallHome(t, home)
	target := filepath.Join(home, ".zshrc")
	original := []byte("export SAFE=1\n")
	if err := os.WriteFile(target, original, 0o600); err != nil {
		t.Fatal(err)
	}
	holder := startTargetLockHelperProcess(t, target)
	defer holder.release(t)

	store := newControllerStore(t, nil)
	initializerCalls := 0
	program := newCLI(zsh.Provider{}, store, NotReadyEmitter(), func(context.Context) (StoreInitialization, error) {
		initializerCalls++
		return StoreInitialization{}, nil
	})
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Millisecond)
	defer cancel()
	var stdout, stderr bytes.Buffer
	started := time.Now()
	code := program.runIngestWithSeams(
		ctx,
		ingestArguments{path: target, asJSON: true},
		&stdout,
		&stderr,
		nil,
	)
	if code != int(model.ExitRuntimeErr) || stderr.Len() != 0 {
		t.Fatalf("canceled ingest = code %d stdout %q stderr %q", code, stdout.String(), stderr.String())
	}
	if elapsed := time.Since(started); elapsed > 500*time.Millisecond {
		t.Fatalf("canceled ingest returned after %s, want prompt return", elapsed)
	}
	if initializerCalls != 0 || store.beginCalls != 0 || store.commitCalls != 0 || store.abortCalls != 0 {
		t.Fatalf(
			"canceled ingest effects = initializer %d begin %d commit %d abort %d",
			initializerCalls, store.beginCalls, store.commitCalls, store.abortCalls,
		)
	}
	if got, err := os.ReadFile(target); err != nil || !bytes.Equal(got, original) {
		t.Fatalf("canceled ingest changed target: %q err=%v", got, err)
	}
	if _, err := os.Lstat(filepath.Join(home, ".zsh-pro")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("canceled ingest created cache or loader: %v", err)
	}
	assertTargetLockNamespaceHasNoTransactionEffects(t, target)
}

func decodeOneIngestResult(t *testing.T, encoded []byte) dto.IngestResult {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	var result dto.IngestResult
	if err := decoder.Decode(&result); err != nil {
		t.Fatalf("decode ingest result: %v\n%s", err, encoded)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		t.Fatalf("ingest JSON contained a second value: %s", encoded)
	}
	if !bytes.HasSuffix(encoded, []byte("\n")) {
		t.Fatalf("ingest JSON lacks exactly one trailing LF: %q", encoded)
	}
	return result
}
