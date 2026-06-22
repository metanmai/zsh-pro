package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeRC(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "rc.zsh")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestRunCleanConfigExitsZero(t *testing.T) {
	p := writeRC(t, "export EDITOR=nvim\nalias ll='ls -l'\n")
	var out, errBuf bytes.Buffer
	code := Run([]string{"analyze", p}, &out, &errBuf)
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
	code := Run([]string{"analyze", p}, &out, &errBuf)
	if code != 3 {
		t.Fatalf("exit code = %d, want 3", code)
	}
}

func TestRunJSONEmitsOneObject(t *testing.T) {
	p := writeRC(t, "alias gs='git status'\nalias gs='git switch'\n")
	var out, errBuf bytes.Buffer
	code := Run([]string{"analyze", p, "--json"}, &out, &errBuf)
	if code != 3 {
		t.Fatalf("exit code = %d, want 3", code)
	}
	s := strings.TrimSpace(out.String())
	if !strings.HasPrefix(s, "{") || !strings.HasSuffix(s, "}") {
		t.Errorf("expected a single JSON object, got:\n%s", s)
	}
}

func TestRunMissingFileExitsOne(t *testing.T) {
	var out, errBuf bytes.Buffer
	code := Run([]string{"analyze", "/no/such/rc"}, &out, &errBuf)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
}

// TestRunMissingFileJSONErrorOnStdout pins the agent contract: in --json mode a
// failure must emit exactly one parseable JSON object on STDOUT (not stderr)
// with "ok": false, and still return exit code 1.
func TestRunMissingFileJSONErrorOnStdout(t *testing.T) {
	var outBuf, errBuf bytes.Buffer
	code := Run([]string{"analyze", "/no/such/file", "--json"}, &outBuf, &errBuf)
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
	code := Run([]string{"frobnicate"}, &out, &errBuf)
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
}
