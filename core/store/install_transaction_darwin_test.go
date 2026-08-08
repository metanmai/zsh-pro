//go:build darwin

package store

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"zsh-pro/core/model"
)

func assertDarwinQuarantineCleanupRemoved(t *testing.T, populate func(string) error) {
	t.Helper()
	store, initialization, _ := newPhase6IngestStore(t)
	begin := beginPhase6Ingest(t, store, initialization)
	path := store.transactionQuarantinePath(begin.TransactionID)
	if populate != nil {
		if err := populate(path); err != nil {
			t.Fatal(err)
		}
	}
	outcome, err := store.AbortIngest(context.Background(), initialization.ID(), begin.TransactionID)
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Cleanup != model.QuarantineCleanupRemoved || outcome.RecoveryRequired {
		t.Fatalf("native Darwin cleanup = %#v, want removed and synced", outcome)
	}
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("native Darwin cleanup retained %s: %v", path, err)
	}
}

func TestDarwinQuarantineCleanupCapabilitySupported(t *testing.T) {
	assertDarwinQuarantineCleanupRemoved(t, nil)
}

func TestDarwinQuarantineCleanupRealTopLevel(t *testing.T) {
	assertDarwinQuarantineCleanupRemoved(t, func(path string) error {
		return os.WriteFile(filepath.Join(path, "top-level"), []byte("owned"), 0o600)
	})
}

func TestDarwinQuarantineCleanupRealNestedFile(t *testing.T) {
	assertDarwinQuarantineCleanupRemoved(t, func(path string) error {
		if err := os.Mkdir(filepath.Join(path, "nested"), 0o700); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(path, "nested", "file"), []byte("owned"), 0o600)
	})
}

func TestDarwinQuarantineCleanupRealNestedDirectory(t *testing.T) {
	assertDarwinQuarantineCleanupRemoved(t, func(path string) error {
		if err := os.MkdirAll(filepath.Join(path, "one", "two", "three"), 0o700); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(path, "one", "two", "three", "file"), []byte("owned"), 0o600)
	})
}

func TestDarwinQuarantineCleanupRealSymlink(t *testing.T) {
	assertDarwinQuarantineCleanupRemoved(t, func(path string) error {
		return os.Symlink("owned-target", filepath.Join(path, "link"))
	})
}

func TestDarwinQuarantineSymlinkDescriptorProbe(t *testing.T) {
	store, initialization, _ := newPhase6IngestStore(t)
	begin := beginPhase6Ingest(t, store, initialization)
	store.transactionMu.Lock()
	record := store.ingestTransactions[begin.TransactionID]
	quarantine := record.quarantine
	store.transactionMu.Unlock()
	if quarantine == nil || quarantine.root == nil {
		t.Fatal("missing test quarantine root")
	}
	if err := os.Symlink("owned-target", filepath.Join(quarantine.path, "link")); err != nil {
		t.Fatal(err)
	}
	original, err := quarantine.root.Lstat("link")
	if err != nil {
		t.Fatal(err)
	}
	link, err := openAuthenticatedQuarantineSymlink(quarantine.root, "link")
	if err != nil {
		t.Fatalf("open link: %v", err)
	}
	opened, err := link.Stat()
	if err != nil {
		_ = link.Close()
		t.Fatalf("stat link: %v", err)
	}
	t.Logf("lstat mode=%#o stat mode=%#o same=%t", original.Mode(), opened.Mode(), sameQuarantineEntry(original, opened))
	if err := link.Close(); err != nil {
		t.Fatalf("close link: %v", err)
	}
	if err := quarantine.root.Remove("link"); err != nil {
		t.Fatalf("remove link after close: %v", err)
	}
}

func TestDarwinQuarantineCleanupRealReplacementBeforeFinalCheckMatrix(t *testing.T) {
	for _, kind := range []string{"file", "symlink", "directory"} {
		t.Run(kind, func(t *testing.T) {
			testAbortIngestRetainsReplacedChild(t, kind, false)
		})
	}
	t.Run("top", func(t *testing.T) {
		testAbortIngestRetainsTopReplacement(t, false)
	})
}

func TestDarwinQuarantineCleanupRealReplacementAfterFinalCheckMatrix(t *testing.T) {
	for _, kind := range []string{"file", "symlink", "directory"} {
		t.Run(kind, func(t *testing.T) {
			testAbortIngestRetainsReplacedChild(t, kind, true)
		})
	}
	t.Run("top", func(t *testing.T) {
		testAbortIngestRetainsTopReplacement(t, true)
	})
}
