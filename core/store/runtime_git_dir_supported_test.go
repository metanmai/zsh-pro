//go:build linux || darwin

package store

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestRuntimeGitRunnerFollowsAuthenticatedDirectoryRename(t *testing.T) {
	pathRunner := newBareGitRunnerForTest(t, "runtime-store")
	originalPath := pathRunner.repoDir
	root, err := os.Open(originalPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()
	runner, err := newRuntimeGitRunner(root)
	if err != nil {
		t.Fatal(err)
	}

	renamedPath := filepath.Join(filepath.Dir(originalPath), "runtime-store-renamed")
	starts := 0
	runner.beforeStart = func([]string) {
		starts++
		if err := os.Rename(originalPath, renamedPath); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(originalPath, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	out, err := runner.run(context.Background(), "rev-parse", "--is-bare-repository")
	if err != nil || !bytes.Equal(bytes.TrimSpace(out), []byte("true")) {
		t.Fatalf("descriptor-root Git after rename = %q, err=%v", out, err)
	}
	if starts != 1 {
		t.Fatalf("Git starts = %d, want 1", starts)
	}

	resolved, err := runtimeGitWorkingDirectory(root)
	if err != nil {
		t.Fatal(err)
	}
	descriptorInfo, err := root.Stat()
	if err != nil {
		t.Fatal(err)
	}
	resolvedInfo, err := os.Stat(resolved)
	if err != nil || !os.SameFile(descriptorInfo, resolvedInfo) {
		t.Fatalf("resolved runtime directory lost descriptor identity: %q, err=%v", resolved, err)
	}
}
