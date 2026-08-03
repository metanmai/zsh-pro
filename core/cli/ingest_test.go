package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

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

func decodeOneIngestResult(t *testing.T, encoded []byte) dto.IngestResult {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	var result dto.IngestResult
	if err := decoder.Decode(&result); err != nil {
		t.Fatalf("decode ingest result: %v\n%s", err, encoded)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, os.ErrClosed) && err == nil {
		t.Fatalf("ingest JSON contained a second value: %s", encoded)
	}
	if !bytes.HasSuffix(encoded, []byte("\n")) {
		t.Fatalf("ingest JSON lacks exactly one trailing LF: %q", encoded)
	}
	return result
}
