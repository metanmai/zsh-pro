package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"zsh-pro/core/buildinfo"
	"zsh-pro/core/dto"
	"zsh-pro/core/model"
	"zsh-pro/core/shell/zsh"
)

type fakeStore struct {
	branches []string
	current  string
	err      error
}

func (s fakeStore) Branches(context.Context) ([]string, error) { return s.branches, s.err }
func (s fakeStore) Current() string                            { return s.current }
func (s fakeStore) Checkout(context.Context, string) error     { return s.err }
func (s fakeStore) Read(context.Context, string) (model.Profile, error) {
	return model.Profile{}, s.err
}

type fakeEmitter struct {
	text string
	err  error
}

func (e fakeEmitter) Emit(context.Context, string, string) (string, error) { return e.text, e.err }

func writeRC(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "rc.zsh")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func newTestCLI() *CLI { return New(zsh.Provider{}, nil, NotReadyEmitter()) }

func TestRunCleanConfigExitsZero(t *testing.T) {
	p := writeRC(t, "export EDITOR=nvim\nalias ll='ls -l'\n")
	var out, errBuf bytes.Buffer
	code := newTestCLI().Run([]string{"analyze", p}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0\nstderr: %s", code, errBuf.String())
	}
	if !strings.Contains(out.String(), "CATEGORIES") {
		t.Errorf("missing report body:\n%s", out.String())
	}
}

func TestRunDuplicateExitsThree(t *testing.T) {
	p := writeRC(t, "alias gs='git status'\nalias gs='git switch'\n")
	var out, errBuf bytes.Buffer
	code := newTestCLI().Run([]string{"analyze", p}, &out, &errBuf)
	if code != 3 {
		t.Fatalf("exit code = %d, want 3", code)
	}
}

// TestRunJSONEmitsOneObject pins the agent contract on the SUCCESS path: in
// --json mode, stdout MUST be exactly one parseable JSON envelope and nothing
// else, stderr stays clean, and the envelope reports ok:true with the matching
// exit code even when actionable issues are found. This is the guard a stray
// log line or fmt.Print to stdout would trip.
func TestRunJSONEmitsOneObject(t *testing.T) {
	p := writeRC(t, "alias gs='git status'\nalias gs='git switch'\n")
	var out, errBuf bytes.Buffer
	code := newTestCLI().Run([]string{"analyze", p, "--json"}, &out, &errBuf)
	if code != 3 {
		t.Fatalf("exit code = %d, want 3\nstdout: %s\nstderr: %s", code, out.String(), errBuf.String())
	}

	// Nothing may leak to stderr on a successful --json run.
	if errBuf.Len() != 0 {
		t.Errorf("expected empty stderr on --json success, got:\n%s", errBuf.String())
	}

	// stdout must be exactly one parseable JSON envelope — no leading or trailing
	// noise. A stray log/print would make Decode fail or leave trailing data.
	dec := json.NewDecoder(bytes.NewReader(out.Bytes()))
	var env dto.Envelope
	if err := dec.Decode(&env); err != nil {
		t.Fatalf("stdout is not a single parseable JSON envelope: %v\nstdout: %s", err, out.String())
	}
	if dec.More() {
		t.Errorf("expected exactly one JSON object on stdout, found trailing data:\n%s", out.String())
	}

	// Envelope contract: a successful analysis (even with actionable issues) is
	// ok:true, self-identifying, and carries the exit code the process returned.
	if !env.OK {
		t.Errorf("expected ok:true on a successful analysis, got ok:false\nstdout: %s", out.String())
	}
	if env.ExitCode != code {
		t.Errorf("envelope exit_code = %d, want %d (must match the process exit code)", env.ExitCode, code)
	}
	if !env.IssuesFound {
		t.Error("expected issues_found:true for a duplicate-alias config")
	}
	if env.Tool != buildinfo.Name || env.Command != buildinfo.Command {
		t.Errorf("envelope identity mismatch: tool=%q command=%q", env.Tool, env.Command)
	}
}

func TestRunMissingFileExitsOne(t *testing.T) {
	var out, errBuf bytes.Buffer
	code := newTestCLI().Run([]string{"analyze", "/no/such/rc"}, &out, &errBuf)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
}

// TestRunMissingFileJSONErrorOnStdout pins the agent contract: in --json mode a
// failure must emit exactly one parseable JSON object on STDOUT (not stderr)
// with "ok": false, and still return exit code 1.
func TestRunMissingFileJSONErrorOnStdout(t *testing.T) {
	var outBuf, errBuf bytes.Buffer
	code := newTestCLI().Run([]string{"analyze", "/no/such/file", "--json"}, &outBuf, &errBuf)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1\nstdout: %s\nstderr: %s", code, outBuf.String(), errBuf.String())
	}

	// The structured error must land on stdout, not stderr.
	if strings.TrimSpace(outBuf.String()) == "" {
		t.Fatalf("expected JSON error on stdout, got empty stdout (stderr: %s)", errBuf.String())
	}

	// Exactly one parseable JSON object on stdout, with ok=false.
	var obj map[string]any
	dec := json.NewDecoder(bytes.NewReader(outBuf.Bytes()))
	if err := dec.Decode(&obj); err != nil {
		t.Fatalf("stdout is not a single parseable JSON object: %v\nstdout: %s", err, outBuf.String())
	}
	if dec.More() {
		t.Errorf("expected exactly one JSON object on stdout, found trailing data:\n%s", outBuf.String())
	}
	if ok, found := obj["ok"].(bool); !found || ok {
		t.Errorf("expected \"ok\": false in JSON object, got %v\nstdout: %s", obj["ok"], outBuf.String())
	}
}

func TestRunUnknownCommandExitsTwo(t *testing.T) {
	var out, errBuf bytes.Buffer
	code := newTestCLI().Run([]string{"frobnicate"}, &out, &errBuf)
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
}

func TestRuntimeVerbsUseInjectedSeams(t *testing.T) {
	var out, errBuf bytes.Buffer
	c := New(zsh.Provider{}, fakeStore{branches: []string{"main", "dev"}, current: "dev"}, fakeEmitter{text: "export EDITOR=nvim"})
	if code := c.Run([]string{"hook"}, &out, &errBuf); code != int(model.ExitClean) || out.String() != (zsh.Provider{}).HookScript() {
		t.Fatalf("hook = (%d, %q), want loader", code, out.String())
	}
	out.Reset()
	if code := c.Run([]string{"list"}, &out, &errBuf); code != int(model.ExitClean) || out.String() != "main\ndev\n" {
		t.Fatalf("list = (%d, %q)", code, out.String())
	}
	out.Reset()
	if code := c.Run([]string{"status"}, &out, &errBuf); code != int(model.ExitClean) || out.String() != "dev\n" {
		t.Fatalf("status = (%d, %q)", code, out.String())
	}
	out.Reset()
	if code := c.Run([]string{"emit", "apply", "dev"}, &out, &errBuf); code != int(model.ExitClean) || out.String() != "export EDITOR=nvim\n" {
		t.Fatalf("emit = (%d, %q)", code, out.String())
	}
}

func TestRuntimeVerbsFailClosedForEmitterAndNilStore(t *testing.T) {
	for _, tc := range []struct {
		name string
		cli  *CLI
		args []string
	}{
		{"emitter error", New(zsh.Provider{}, fakeStore{}, fakeEmitter{err: errors.New("boom")}), []string{"emit", "apply", "dev"}},
		{"not ready", New(zsh.Provider{}, fakeStore{}, NotReadyEmitter()), []string{"emit", "apply", "dev"}},
		{"nil store list", New(zsh.Provider{}, nil, fakeEmitter{}), []string{"list"}},
		{"nil store status", New(zsh.Provider{}, nil, fakeEmitter{}), []string{"status"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out, errBuf bytes.Buffer
			if code := tc.cli.Run(tc.args, &out, &errBuf); code != int(model.ExitRuntimeErr) {
				t.Fatalf("code = %d, want runtime failure", code)
			}
			if out.Len() != 0 || errBuf.Len() == 0 {
				t.Fatalf("failure output = stdout %q stderr %q", out.String(), errBuf.String())
			}
		})
	}
}
