//go:build unix

package worktree

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"zsh-pro/core/model"
)

func TestStateStorePathAndAuthenticatedDescriptorUseSameCanonicalGeneration(t *testing.T) {
	root := privateStateRoot(t)
	pathStore, err := OpenStateStore(root)
	if err != nil {
		t.Fatal(err)
	}
	state := validStateFixture(t)
	if err := pathStore.WithTransaction(context.Background(), func(current *State) error {
		*current = state
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	pathBytes, err := os.ReadFile(filepath.Join(root, stateFileName))
	if err != nil {
		t.Fatal(err)
	}
	if err := pathStore.Close(); err != nil {
		t.Fatal(err)
	}

	rootFD, err := os.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	descriptorStore, err := NewStateStoreFromAuthenticatedRoot(rootFD)
	if err != nil {
		t.Fatal(err)
	}
	if err := rootFD.Close(); err != nil {
		t.Fatal(err)
	}
	got, err := descriptorStore.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := MarshalState(got)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(pathBytes, encoded) {
		t.Fatalf("constructors observed different canonical bytes")
	}
	if err := descriptorStore.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestStateStoreFromAuthenticatedRootOwnsOnlyDuplicate(t *testing.T) {
	root := privateStateRoot(t)
	input, err := os.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	store, err := NewStateStoreFromAuthenticatedRoot(input)
	if err != nil {
		t.Fatal(err)
	}
	if err := input.Close(); err != nil {
		t.Fatal(err)
	}
	if err := store.WithTransaction(context.Background(), func(state *State) error {
		*state = validStateFixture(t)
		return nil
	}); err != nil {
		t.Fatalf("store depended on caller descriptor: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	input, err = os.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	store, err = NewStateStoreFromAuthenticatedRoot(input)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := input.Stat(); err != nil {
		t.Fatalf("store close closed caller descriptor: %v", err)
	}
	_ = input.Close()
}

func TestStateStoreCloseIsIdempotentAndOperationsFail(t *testing.T) {
	store, err := OpenStateStore(privateStateRoot(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("second close: %v", err)
	}
	if _, err := store.Read(context.Background()); !errors.Is(err, ErrStateStoreClosed) {
		t.Fatalf("read after close = %v", err)
	}
	if err := store.WithTransaction(context.Background(), func(*State) error { return nil }); !errors.Is(err, ErrStateStoreClosed) {
		t.Fatalf("transaction after close = %v", err)
	}
}

func TestStateStoreConstructorFailureDoesNotLeakOrCloseInput(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0o755); err != nil {
		t.Fatal(err)
	}
	input, err := os.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	before := openFDCount(t)
	for range 32 {
		if _, err := NewStateStoreFromAuthenticatedRoot(input); err == nil {
			t.Fatal("wrong-mode descriptor accepted")
		}
	}
	runtime.GC()
	after := openFDCount(t)
	if after > before {
		t.Fatalf("constructor leaked descriptor: before=%d after=%d", before, after)
	}
	if _, err := input.Stat(); err != nil {
		t.Fatalf("constructor closed caller input: %v", err)
	}
	_ = input.Close()
}

func TestSymlinkAndOwnershipBoundaries(t *testing.T) {
	realRoot := privateStateRoot(t)
	rootLink := filepath.Join(t.TempDir(), "root-link")
	if err := os.Symlink(realRoot, rootLink); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenStateStore(rootLink); err == nil {
		t.Fatal("symlink root accepted")
	}
	wrongMode := t.TempDir()
	if err := os.Chmod(wrongMode, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenStateStore(wrongMode); err == nil {
		t.Fatal("non-private root accepted")
	}
	if err := validateStateRootMetadata(stateRootMetadata{isDir: true, mode: 0o700, uid: uint32(os.Geteuid() + 1)}); err == nil {
		t.Fatal("foreign-owned root accepted")
	}

	target := filepath.Join(t.TempDir(), "target")
	if err := os.WriteFile(target, []byte("target"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(realRoot, stateLockFileName)); err != nil {
		t.Fatal(err)
	}
	store, err := OpenStateStore(realRoot)
	if err != nil {
		t.Fatal(err)
	}
	defer closeStateStore(t, store)
	if err := store.WithTransaction(context.Background(), func(*State) error { return nil }); err == nil {
		t.Fatal("symlink lock accepted")
	}
}

func TestStateStoreRemainsBoundToAuthenticatedDescriptorAfterPathReplacement(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "runtime")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	store, err := OpenStateStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer closeStateStore(t, store)
	moved := filepath.Join(parent, "authenticated")
	if err := os.Rename(root, moved); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := store.WithTransaction(context.Background(), func(state *State) error {
		*state = validStateFixture(t)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(moved, stateFileName)); err != nil {
		t.Fatalf("authenticated directory not updated: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, stateFileName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("replacement path was used: %v", err)
	}
}

func TestAtomicFaultsLeaveOldOrCompleteNewGeneration(t *testing.T) {
	tests := []struct {
		name    string
		install func(*StateStore)
		wantNew bool
	}{
		{"short-write", func(store *StateStore) { store.faults.write = func(int, []byte) (int, error) { return 0, nil } }, false},
		{"file-sync", func(store *StateStore) {
			store.faults.fileSync = func(int) error { return errors.New("file sync fault") }
		}, false},
		{"before-rename", func(store *StateStore) {
			store.faults.beforeRename = func() error { return errors.New("before rename fault") }
		}, false},
		{"rename", func(store *StateStore) {
			store.faults.rename = func(int, string, int, string) error { return errors.New("rename fault") }
		}, false},
		{"after-rename", func(store *StateStore) {
			store.faults.afterRename = func() error { return errors.New("response lost after rename") }
		}, true},
		{"dir-sync", func(store *StateStore) {
			store.faults.dirSync = func(int) error { return errors.New("dir sync fault") }
		}, true},
		{"after-dir-sync", func(store *StateStore) {
			store.faults.afterDirSync = func() error { return errors.New("response lost after dir sync") }
		}, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := privateStateRoot(t)
			store := seedStateStore(t, root)
			test.install(store)
			err := store.WithTransaction(context.Background(), func(state *State) error {
				advanceStateForAtomicTest(t, state, "fault-op")
				return nil
			})
			if err == nil {
				t.Fatal("injected failure was not returned")
			}
			_ = store.Close()
			reopened, err := OpenStateStore(root)
			if err != nil {
				t.Fatal(err)
			}
			defer closeStateStore(t, reopened)
			got, err := reopened.Read(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			want := uint64(1)
			if test.wantNew {
				want = 2
			}
			if got.HeadRevision != want {
				t.Fatalf("revision after fault = %d, want %d", got.HeadRevision, want)
			}
		})
	}
}

func TestAtomicFullWriteHandlesShortWrites(t *testing.T) {
	store, err := OpenStateStore(privateStateRoot(t))
	if err != nil {
		t.Fatal(err)
	}
	defer closeStateStore(t, store)
	store.faults.write = func(fd int, payload []byte) (int, error) {
		if len(payload) > 7 {
			payload = payload[:7]
		}
		return syscall.Write(fd, payload)
	}
	if err := store.WithTransaction(context.Background(), func(state *State) error {
		*state = validStateFixture(t)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Read(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestLockContentionHonorsCancellationAndDeadline(t *testing.T) {
	root := privateStateRoot(t)
	store := seedStateStore(t, root)
	defer closeStateStore(t, store)
	lock, err := os.OpenFile(filepath.Join(root, stateLockFileName), os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer closeTestFile(t, lock)
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_UN); err != nil {
			t.Errorf("unlock test lock: %v", err)
		}
	}()

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := store.WithTransaction(cancelled, func(*State) error { return nil }); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled lock = %v", err)
	}
	deadline, stop := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer stop()
	if err := store.WithTransaction(deadline, func(*State) error { return nil }); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("deadline lock = %v", err)
	}
}

func TestStateStoreTwoProcessCounterRaceSerializes(t *testing.T) {
	if os.Getenv("ZSH_PRO_STATESTORE_HELPER") == "1" {
		root := os.Args[len(os.Args)-1]
		store, err := OpenStateStore(root)
		if err == nil {
			err = store.WithTransaction(context.Background(), func(state *State) error {
				advanceStateForAtomicTest(t, state, os.Getenv("ZSH_PRO_STATESTORE_OPERATION"))
				return nil
			})
			_ = store.Close()
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		os.Exit(0)
	}
	root := privateStateRoot(t)
	store := seedStateStore(t, root)
	_ = store.Close()
	commands := make([]*exec.Cmd, 2)
	for i := range commands {
		commands[i] = exec.Command(os.Args[0], "-test.run=TestStateStoreTwoProcessCounterRaceSerializes", "--", root)
		commands[i].Env = append(os.Environ(), "ZSH_PRO_STATESTORE_HELPER=1", fmt.Sprintf("ZSH_PRO_STATESTORE_OPERATION=process-%d", i))
	}
	var wg sync.WaitGroup
	errs := make(chan error, len(commands))
	for _, command := range commands {
		wg.Add(1)
		go func(command *exec.Cmd) {
			defer wg.Done()
			errs <- command.Run()
		}(command)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	store, err := OpenStateStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer closeStateStore(t, store)
	state, err := store.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if state.HeadRevision != 3 {
		t.Fatalf("serialized revision = %d, want 3", state.HeadRevision)
	}
}

func TestInterruptedResponseReopenSurfacesStoredReceipt(t *testing.T) {
	root := privateStateRoot(t)
	store := seedStateStore(t, root)
	store.faults.afterRename = func() error { return errors.New("response lost") }
	err := store.WithTransaction(context.Background(), func(state *State) error {
		fingerprint := SnapshotFingerprint{1}
		state.OperationReceipts = append(state.OperationReceipts, OperationReceipt{
			ShellID: "shell-1", OperationID: "attach-replay", Kind: ReceiptAttach,
			RequestFingerprint: fingerprint,
			Attach:             &model.AttachResult{Revision: state.HeadRevision, Attached: true},
		})
		return nil
	})
	if err == nil {
		t.Fatal("lost response fault not returned")
	}
	_ = store.Close()
	reopened, err := OpenStateStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer closeStateStore(t, reopened)
	state, err := reopened.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, receipt := range state.OperationReceipts {
		found = found || receipt.OperationID == "attach-replay"
	}
	if !found {
		t.Fatal("stored receipt missing after uncertain response")
	}
}

func TestFaultForensicAppendDoesNotReverseCanonicalAndRecordsRecovery(t *testing.T) {
	root := privateStateRoot(t)
	store := seedStateStore(t, root)
	defer closeStateStore(t, store)
	store.faults.forensicAppend = func(int, []byte) (int, error) {
		return 0, errors.New("forensic append fault")
	}
	if err := store.WithTransaction(context.Background(), func(state *State) error {
		advanceStateForAtomicTest(t, state, "forensic-fault")
		return nil
	}); err != nil {
		t.Fatalf("derived append reversed canonical success: %v", err)
	}
	store.faults.forensicAppend = nil
	state, err := store.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if state.HeadRevision != 2 || state.Shells["shell-1"].LastRecoveredError != RecoveredPersistence {
		t.Fatalf("recovered diagnostic missing from canonical state: %#v", state.Shells["shell-1"])
	}
	forensic, err := os.ReadFile(filepath.Join(root, stateForensicFileName))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(forensic, []byte("shared")) {
		t.Fatalf("forensic projection exposed captured value: %q", forensic)
	}
}

func TestAtomicReadersSeeOldOrCompleteNewAndIgnoreTemps(t *testing.T) {
	root := privateStateRoot(t)
	store := seedStateStore(t, root)
	defer closeStateStore(t, store)
	if err := os.WriteFile(filepath.Join(root, ".worktree-state-abandoned"), []byte("partial secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	entered := make(chan struct{})
	release := make(chan struct{})
	store.faults.beforeRename = func() error {
		close(entered)
		<-release
		return nil
	}
	done := make(chan error, 1)
	go func() {
		done <- store.WithTransaction(context.Background(), func(state *State) error {
			advanceStateForAtomicTest(t, state, "atomic-reader")
			return nil
		})
	}()
	<-entered
	payload, err := os.ReadFile(filepath.Join(root, stateFileName))
	if err != nil {
		t.Fatal(err)
	}
	old, err := UnmarshalState(payload)
	if err != nil || old.HeadRevision != 1 {
		t.Fatalf("reader saw staged generation: revision=%d err=%v", old.HeadRevision, err)
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	newState, err := store.Read(context.Background())
	if err != nil || newState.HeadRevision != 2 {
		t.Fatalf("reader did not see complete new generation: revision=%d err=%v", newState.HeadRevision, err)
	}
}

func privateStateRoot(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "runtime")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	return root
}

func seedStateStore(t *testing.T, root string) *StateStore {
	t.Helper()
	store, err := OpenStateStore(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.WithTransaction(context.Background(), func(state *State) error {
		*state = validStateFixture(t)
		return nil
	}); err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	return store
}

func advanceStateForAtomicTest(t *testing.T, state *State, operationID string) {
	t.Helper()
	if !state.Materialized {
		t.Fatal("fixture was not materialized")
	}
	state.HeadRevision++
	state.Events = append(state.Events, StateEvent{Revision: state.HeadRevision, OperationID: operationID})
}

func openFDCount(t *testing.T) int {
	t.Helper()
	entries, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		t.Skip("/proc/self/fd unavailable")
	}
	return len(entries)
}

func closeStateStore(t *testing.T, store *StateStore) {
	t.Helper()
	if err := store.Close(); err != nil {
		t.Errorf("close state store: %v", err)
	}
}

func closeTestFile(t *testing.T, file *os.File) {
	t.Helper()
	if err := file.Close(); err != nil {
		t.Errorf("close test file: %v", err)
	}
}

func TestStateRootMetadataRejectsNonDirectoryAndUnexpectedMode(t *testing.T) {
	tests := []stateRootMetadata{
		{isDir: false, mode: 0o700, uid: uint32(os.Geteuid())},
		{isDir: true, mode: 0o755, uid: uint32(os.Geteuid())},
	}
	for _, metadata := range tests {
		if err := validateStateRootMetadata(metadata); err == nil {
			t.Fatalf("metadata accepted: %#v", metadata)
		}
	}
}

func TestStateStoreTransactionCallbackErrorDoesNotWrite(t *testing.T) {
	root := privateStateRoot(t)
	store := seedStateStore(t, root)
	defer closeStateStore(t, store)
	want := errors.New("callback failed")
	if err := store.WithTransaction(context.Background(), func(state *State) error {
		advanceStateForAtomicTest(t, state, "never-written")
		return want
	}); !errors.Is(err, want) {
		t.Fatalf("callback error = %v", err)
	}
	state, err := store.Read(context.Background())
	if err != nil || state.HeadRevision != 1 {
		t.Fatalf("callback failure changed state: revision=%d err=%v", state.HeadRevision, err)
	}
}

func TestStateStoreRejectsMalformedCanonicalAndStateSymlink(t *testing.T) {
	for _, content := range []string{"malformed", strings.Repeat("x", MaxStateBytes+1)} {
		root := privateStateRoot(t)
		if err := os.WriteFile(filepath.Join(root, stateFileName), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		store, err := OpenStateStore(root)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := store.Read(context.Background()); err == nil {
			t.Fatal("invalid canonical state accepted")
		}
		_ = store.Close()
	}
	root := privateStateRoot(t)
	target := filepath.Join(t.TempDir(), "outside")
	if err := os.WriteFile(target, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(root, stateFileName)); err != nil {
		t.Fatal(err)
	}
	store, err := OpenStateStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer closeStateStore(t, store)
	if _, err := store.Read(context.Background()); err == nil {
		t.Fatal("symlink canonical state accepted")
	}
}

func TestStateStoreNoopTransactionPreservesCanonicalBytes(t *testing.T) {
	root := privateStateRoot(t)
	store := seedStateStore(t, root)
	defer closeStateStore(t, store)
	before, err := os.ReadFile(filepath.Join(root, stateFileName))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.WithTransaction(context.Background(), func(*State) error { return nil }); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(filepath.Join(root, stateFileName))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatal("no-op transaction rewrote canonical state")
	}
}

func TestStateStoreDescriptorOwnership(t *testing.T) {
	t.Run("path and descriptor share generation", TestStateStorePathAndAuthenticatedDescriptorUseSameCanonicalGeneration)
	t.Run("constructor owns only duplicate", TestStateStoreFromAuthenticatedRootOwnsOnlyDuplicate)
	t.Run("constructor failure preserves input", TestStateStoreConstructorFailureDoesNotLeakOrCloseInput)
	t.Run("path replacement cannot redirect descriptor", TestStateStoreRemainsBoundToAuthenticatedDescriptorAfterPathReplacement)
}

func TestWorktreeSecurityFaultMatrix(t *testing.T) {
	t.Run("owner mode and symlink boundary", TestSymlinkAndOwnershipBoundaries)
	t.Run("canonical old or complete new", TestAtomicFaultsLeaveOldOrCompleteNewGeneration)
	t.Run("short writes complete", TestAtomicFullWriteHandlesShortWrites)
	t.Run("contention observes deadline", TestLockContentionHonorsCancellationAndDeadline)
	t.Run("cross-process generation serialization", TestStateStoreTwoProcessCounterRaceSerializes)
	t.Run("lost response receipt recovery", TestInterruptedResponseReopenSurfacesStoredReceipt)
	t.Run("forensic failure cannot reverse truth", TestFaultForensicAppendDoesNotReverseCanonicalAndRecordsRecovery)
	t.Run("readers ignore abandoned temporary state", TestAtomicReadersSeeOldOrCompleteNewAndIgnoreTemps)
	t.Run("malformed canonical and state symlink", TestStateStoreRejectsMalformedCanonicalAndStateSymlink)
	t.Run("callback failure writes nothing", TestStateStoreTransactionCallbackErrorDoesNotWrite)
}
