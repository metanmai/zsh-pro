package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	maxSnapshotBytes  = 2 << 20
	maxSnapshotItems  = 10_000
	maxRevisionEvents = 128
)

type entry struct {
	Kind    string   `json:"kind"`
	Name    string   `json:"name"`
	Present bool     `json:"present"`
	Value   string   `json:"value,omitempty"`
	Values  []string `json:"values,omitempty"`
}

func (e entry) key() string { return e.Kind + "\x1f" + e.Name }

type exclusion struct {
	Kind   string `json:"kind"`
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

type change struct {
	Key   string `json:"key"`
	Entry entry  `json:"entry"`
}

type revisionEvent struct {
	Revision uint64   `json:"revision"`
	Shell    string   `json:"shell"`
	At       string   `json:"at"`
	Changes  []change `json:"changes"`
}

type sharedState struct {
	Revision         uint64           `json:"revision"`
	CompactedThrough uint64           `json:"compacted_through,omitempty"`
	Entries          map[string]entry `json:"entries"`
	Events           []revisionEvent  `json:"events"`
}

type conflict struct {
	BaseRevision uint64   `json:"base_revision"`
	HeadRevision uint64   `json:"head_revision"`
	Keys         []string `json:"keys"`
	Reason       string   `json:"reason,omitempty"`
	At           string   `json:"at"`
}

type shellState struct {
	Shell           string            `json:"shell"`
	AppliedRevision uint64            `json:"applied_revision"`
	PendingRevision uint64            `json:"pending_revision,omitempty"`
	Baseline        map[string]entry  `json:"baseline"`
	Exclusions      map[string]string `json:"exclusions,omitempty"`
	Behind          bool              `json:"behind"`
	AutoApply       bool              `json:"auto_apply"`
	Conflict        *conflict         `json:"conflict,omitempty"`
	LastCycleMicros int64             `json:"last_cycle_micros"`
	LastError       string            `json:"last_error,omitempty"`
	LastUpdatedAt   string            `json:"last_updated_at"`
}

type logRecord struct {
	At             string `json:"at"`
	Event          string `json:"event"`
	Shell          string `json:"shell,omitempty"`
	Revision       uint64 `json:"revision,omitempty"`
	BaseRevision   uint64 `json:"base_revision,omitempty"`
	Entries        int    `json:"entries,omitempty"`
	Changes        int    `json:"changes,omitempty"`
	PatchChanges   int    `json:"patch_changes,omitempty"`
	Excluded       int    `json:"excluded,omitempty"`
	DurationMicros int64  `json:"duration_micros,omitempty"`
	Kind           string `json:"kind,omitempty"`
	Name           string `json:"name,omitempty"`
	Reason         string `json:"reason,omitempty"`
	Error          string `json:"error,omitempty"`
}

type shellStatus struct {
	Shell           string    `json:"shell"`
	AppliedRevision uint64    `json:"applied_revision"`
	PendingRevision uint64    `json:"pending_revision,omitempty"`
	Behind          bool      `json:"behind"`
	AutoApply       bool      `json:"auto_apply"`
	Conflict        *conflict `json:"conflict,omitempty"`
	LastCycleMicros int64     `json:"last_cycle_micros"`
	LastError       string    `json:"last_error,omitempty"`
}

func main() {
	if len(os.Args) < 2 {
		fatalf("usage: spike-sync <init|attach|cycle|ack|resolve-shared|status|summary|hold-lock|inject-partial> ...")
	}
	var err error
	switch os.Args[1] {
	case "init":
		err = runInit(os.Args[2:])
	case "attach":
		err = runAttach(os.Args[2:], os.Stdin)
	case "cycle":
		err = runCycle(os.Args[2:], os.Stdin)
	case "ack":
		err = runAck(os.Args[2:], os.Stdin)
	case "resolve-shared":
		err = runResolveShared(os.Args[2:], os.Stdin)
	case "status":
		err = runStatus(os.Args[2:], os.Stdout)
	case "summary":
		err = runSummary(os.Args[2:], os.Stdout)
	case "hold-lock":
		err = runHoldLock(os.Args[2:])
	case "inject-partial":
		err = runInjectPartial(os.Args[2:])
	default:
		err = fmt.Errorf("unknown command %q", os.Args[1])
	}
	if err != nil {
		fatalf("%v", err)
	}
}

func fatalf(format string, args ...any) {
	_, _ = fmt.Fprintf(os.Stderr, "spike-sync: "+format+"\n", args...)
	os.Exit(1)
}

func commandFlags(name string, args []string, needShell bool) (string, string, error) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	root := fs.String("root", "", "shared experiment root")
	shell := fs.String("shell", "", "shell identity")
	if err := fs.Parse(args); err != nil {
		return "", "", err
	}
	if *root == "" {
		return "", "", errors.New("--root is required")
	}
	if needShell && *shell == "" {
		return "", "", errors.New("--shell is required")
	}
	if needShell && !validShellID(*shell) {
		return "", "", errors.New("--shell must contain only letters, digits, underscore, or hyphen")
	}
	return *root, *shell, nil
}

func validShellID(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			continue
		}
		return false
	}
	return true
}

func runInit(args []string) error {
	root, _, err := commandFlags("init", args, false)
	if err != nil {
		return err
	}
	if err := ensureRoot(root); err != nil {
		return err
	}
	return withLock(root, func() error {
		path := filepath.Join(root, "state.json")
		if _, err := os.Stat(path); err == nil {
			return nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		state := sharedState{Entries: map[string]entry{}, Events: []revisionEvent{}}
		if err := writeJSONAtomic(path, state); err != nil {
			return err
		}
		return appendLog(root, logRecord{At: now(), Event: "initialized"})
	})
}

func runAttach(args []string, in io.Reader) error {
	root, shell, err := commandFlags("attach", args, true)
	if err != nil {
		return err
	}
	snapshot, excluded, err := parseSnapshot(in)
	if err != nil {
		return err
	}
	if err := ensureInitialized(root); err != nil {
		return err
	}
	return withLock(root, func() error {
		shared, err := loadShared(root)
		if err != nil {
			return err
		}
		local := shellState{
			Shell: shell, Baseline: snapshot, Exclusions: exclusionMap(excluded), AutoApply: true,
			Behind: shared.Revision > 0, LastUpdatedAt: now(),
		}
		if err := saveShell(root, local); err != nil {
			return err
		}
		_ = os.Remove(patchPath(root, shell))
		if err := appendLog(root, logRecord{At: now(), Event: "attached", Shell: shell, Revision: shared.Revision, Entries: len(snapshot), Excluded: len(excluded)}); err != nil {
			return err
		}
		return logExclusions(root, shell, excluded)
	})
}

func runCycle(args []string, in io.Reader) error {
	fs := flag.NewFlagSet("cycle", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	root := fs.String("root", "", "shared experiment root")
	shell := fs.String("shell", "", "shell identity")
	apply := fs.Bool("apply", false, "prepare remote changes for apply")
	previousError := fs.String("previous-error", "", "hook failure recovered since the prior successful cycle")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *root == "" || *shell == "" || !validShellID(*shell) {
		return errors.New("valid --root and --shell are required")
	}
	started := time.Now()
	snapshot, excluded, err := parseSnapshot(in)
	if err != nil {
		return err
	}
	if err := ensureInitialized(*root); err != nil {
		return err
	}
	return withLock(*root, func() error {
		shared, err := loadShared(*root)
		if err != nil {
			return err
		}
		local, err := loadShell(*root, *shell)
		if err != nil {
			return err
		}
		_ = os.Remove(patchPath(*root, *shell))
		if *previousError != "" {
			if err := appendLog(*root, logRecord{At: now(), Event: "recovered-failure", Shell: *shell, Revision: shared.Revision, Error: *previousError}); err != nil {
				return err
			}
		}
		baseRevision := local.AppliedRevision
		delta := diffEntries(local.Baseline, snapshot)
		published := 0
		patchChanges := 0
		if local.Conflict == nil && len(delta) > 0 {
			var conflicts []string
			filtered := make(map[string]entry, len(delta))
			conflictReason := "same identity changed after the shell's applied revision"
			if local.AppliedRevision < shared.CompactedThrough {
				conflictReason = "shell revision predates compacted history; explicit shared resolution is required"
				for key := range delta {
					conflicts = append(conflicts, displayKey(key))
				}
			} else {
				remoteKeys := changedKeysSince(shared, local.AppliedRevision)
				for key, wanted := range delta {
					current, present := shared.Entries[key]
					if !present {
						current = entry{Kind: wanted.Kind, Name: wanted.Name, Present: false}
					}
					if entriesEqual(wanted, current) {
						continue
					}
					if remoteKeys[key] {
						conflicts = append(conflicts, displayKey(key))
						continue
					}
					filtered[key] = wanted
				}
			}
			if len(conflicts) > 0 {
				sort.Strings(conflicts)
				local.Conflict = &conflict{BaseRevision: local.AppliedRevision, HeadRevision: shared.Revision, Keys: conflicts, Reason: conflictReason, At: now()}
				local.Behind = true
				if err := appendLog(*root, logRecord{At: now(), Event: "conflict", Shell: *shell, Revision: shared.Revision, BaseRevision: local.AppliedRevision, Changes: len(conflicts)}); err != nil {
					return err
				}
			} else {
				local.Baseline = snapshot
				if len(filtered) > 0 {
					changes := sortedChanges(filtered)
					for _, c := range changes {
						if c.Entry.Present {
							shared.Entries[c.Key] = c.Entry
						} else {
							delete(shared.Entries, c.Key)
						}
					}
					shared.Revision++
					shared.Events = append(shared.Events, revisionEvent{Revision: shared.Revision, Shell: *shell, At: now(), Changes: changes})
					compactRevisionHistory(&shared)
					published = len(changes)
					if err := saveShared(*root, shared); err != nil {
						return err
					}
					if err := appendLog(*root, logRecord{At: now(), Event: "published", Shell: *shell, Revision: shared.Revision, BaseRevision: baseRevision, Changes: published}); err != nil {
						return err
					}
				}
			}
		}

		local.AutoApply = *apply
		if err := logNewExclusions(*root, *shell, local.Exclusions, excluded); err != nil {
			return err
		}
		local.Exclusions = exclusionMap(excluded)
		if local.Conflict == nil && *apply && shared.Revision > local.AppliedRevision {
			patchDelta := finalChangesForShell(shared, local.AppliedRevision, snapshot)
			patch, err := renderPatch(patchDelta)
			if err != nil {
				return err
			}
			if patch != "" {
				if err := writeFileAtomic(patchPath(*root, *shell), []byte(patch), 0o600); err != nil {
					return err
				}
				local.PendingRevision = shared.Revision
				patchChanges = len(patchDelta)
			}
		}
		local.Behind = shared.Revision > local.AppliedRevision
		local.LastCycleMicros = time.Since(started).Microseconds()
		local.LastUpdatedAt = now()
		local.LastError = ""
		if err := saveShell(*root, local); err != nil {
			return err
		}
		return appendLog(*root, logRecord{At: now(), Event: "cycle", Shell: *shell, Revision: shared.Revision, BaseRevision: baseRevision, Entries: len(snapshot), Changes: published, PatchChanges: patchChanges, Excluded: len(excluded), DurationMicros: local.LastCycleMicros})
	})
}

func runAck(args []string, in io.Reader) error {
	root, shell, err := commandFlags("ack", args, true)
	if err != nil {
		return err
	}
	snapshot, excluded, err := parseSnapshot(in)
	if err != nil {
		return err
	}
	return withLock(root, func() error {
		shared, err := loadShared(root)
		if err != nil {
			return err
		}
		local, err := loadShell(root, shell)
		if err != nil {
			return err
		}
		if local.PendingRevision == 0 {
			return errors.New("no prepared patch to acknowledge")
		}
		local.AppliedRevision = local.PendingRevision
		local.PendingRevision = 0
		local.Baseline = snapshot
		local.Exclusions = exclusionMap(excluded)
		local.Behind = shared.Revision > local.AppliedRevision
		local.LastUpdatedAt = now()
		local.LastError = ""
		if err := saveShell(root, local); err != nil {
			return err
		}
		_ = os.Remove(patchPath(root, shell))
		return appendLog(root, logRecord{At: now(), Event: "applied", Shell: shell, Revision: local.AppliedRevision, Entries: len(snapshot), Excluded: len(excluded)})
	})
}

func runResolveShared(args []string, in io.Reader) error {
	root, shell, err := commandFlags("resolve-shared", args, true)
	if err != nil {
		return err
	}
	snapshot, excluded, err := parseSnapshot(in)
	if err != nil {
		return err
	}
	return withLock(root, func() error {
		shared, err := loadShared(root)
		if err != nil {
			return err
		}
		local, err := loadShell(root, shell)
		if err != nil {
			return err
		}
		local.Baseline = snapshot
		local.Exclusions = exclusionMap(excluded)
		local.Conflict = nil
		patchDelta := finalChangesForShell(shared, local.AppliedRevision, snapshot)
		patch, err := renderPatch(patchDelta)
		if err != nil {
			return err
		}
		if patch != "" {
			if err := writeFileAtomic(patchPath(root, shell), []byte(patch), 0o600); err != nil {
				return err
			}
			local.PendingRevision = shared.Revision
		}
		local.Behind = shared.Revision > local.AppliedRevision
		local.LastUpdatedAt = now()
		if err := saveShell(root, local); err != nil {
			return err
		}
		return appendLog(root, logRecord{At: now(), Event: "conflict-resolved-shared", Shell: shell, Revision: shared.Revision, PatchChanges: len(patchDelta), Excluded: len(excluded)})
	})
}

func runStatus(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	root := fs.String("root", "", "shared experiment root")
	jsonOutput := fs.Bool("json", false, "emit JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *root == "" {
		return errors.New("--root is required")
	}
	return withLock(*root, func() error {
		shared, err := loadShared(*root)
		if err != nil {
			return err
		}
		shells, err := loadAllShells(*root)
		if err != nil {
			return err
		}
		if *jsonOutput {
			statuses := make([]shellStatus, 0, len(shells))
			for _, local := range shells {
				statuses = append(statuses, shellStatus{
					Shell: local.Shell, AppliedRevision: local.AppliedRevision,
					PendingRevision: local.PendingRevision, Behind: local.Behind,
					AutoApply: local.AutoApply, Conflict: local.Conflict,
					LastCycleMicros: local.LastCycleMicros, LastError: local.LastError,
				})
			}
			return json.NewEncoder(out).Encode(struct {
				Revision         uint64        `json:"revision"`
				CompactedThrough uint64        `json:"compacted_through"`
				Entries          int           `json:"entries"`
				Shells           []shellStatus `json:"shells"`
			}{shared.Revision, shared.CompactedThrough, len(shared.Entries), statuses})
		}
		_, _ = fmt.Fprintf(out, "shared revision=%d compacted_through=%d entries=%d\n", shared.Revision, shared.CompactedThrough, len(shared.Entries))
		for _, local := range shells {
			conflictText := "none"
			if local.Conflict != nil {
				conflictText = strings.Join(local.Conflict.Keys, ",")
			}
			_, _ = fmt.Fprintf(out, "%s applied=%d pending=%d behind=%t auto_apply=%t conflict=%s last_cycle=%dus\n", local.Shell, local.AppliedRevision, local.PendingRevision, local.Behind, local.AutoApply, conflictText, local.LastCycleMicros)
		}
		return nil
	})
}

func runSummary(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("summary", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	root := fs.String("root", "", "shared experiment root")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *root == "" {
		return errors.New("--root is required")
	}
	f, err := os.Open(filepath.Join(*root, "events.jsonl"))
	if err != nil {
		return err
	}
	defer f.Close()
	counts := map[string]int{}
	var durations []int64
	var errorsSeen int
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 2<<20)
	for scanner.Scan() {
		var record logRecord
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			return fmt.Errorf("parse event log: %w", err)
		}
		counts[record.Event]++
		if record.DurationMicros > 0 {
			durations = append(durations, record.DurationMicros)
		}
		if record.Error != "" {
			errorsSeen++
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
	percentile := func(p float64) int64 {
		if len(durations) == 0 {
			return 0
		}
		idx := int(float64(len(durations)-1) * p)
		return durations[idx]
	}
	return json.NewEncoder(out).Encode(struct {
		EventCounts map[string]int `json:"event_counts"`
		Samples     int            `json:"duration_samples"`
		P50Micros   int64          `json:"p50_micros"`
		P95Micros   int64          `json:"p95_micros"`
		MaxMicros   int64          `json:"max_micros"`
		Errors      int            `json:"errors"`
		LogPath     string         `json:"log_path"`
	}{counts, len(durations), percentile(.50), percentile(.95), percentile(1), errorsSeen, filepath.Join(*root, "events.jsonl")})
}

func runHoldLock(args []string) error {
	fs := flag.NewFlagSet("hold-lock", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	root := fs.String("root", "", "shared experiment root")
	duration := fs.Duration("duration", time.Second, "lock hold duration")
	ready := fs.String("ready", "", "touch this file after lock acquisition")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *root == "" {
		return errors.New("--root is required")
	}
	return withLock(*root, func() error {
		if *ready != "" {
			if err := os.WriteFile(*ready, []byte("ready\n"), 0o600); err != nil {
				return err
			}
		}
		time.Sleep(*duration)
		return nil
	})
}

func runInjectPartial(args []string) error {
	root, _, err := commandFlags("inject-partial", args, false)
	if err != nil {
		return err
	}
	path := filepath.Join(root, ".state.json.interrupted-write")
	return os.WriteFile(path, []byte(`{"revision":`), 0o600)
}

func ensureInitialized(root string) error {
	if err := ensureRoot(root); err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(root, "state.json")); errors.Is(err, os.ErrNotExist) {
		return runInit([]string{"--root", root})
	} else {
		return err
	}
}

func ensureRoot(root string) error {
	if err := os.MkdirAll(filepath.Join(root, "shells"), 0o700); err != nil {
		return err
	}
	if err := os.Chmod(root, 0o700); err != nil {
		return err
	}
	lock, err := os.OpenFile(filepath.Join(root, "lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	return lock.Close()
}

func withLock(root string, fn func() error) error {
	lock, err := os.OpenFile(filepath.Join(root, "lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	defer func() { _ = syscall.Flock(int(lock.Fd()), syscall.LOCK_UN) }()
	return fn()
}

func loadShared(root string) (sharedState, error) {
	var state sharedState
	if err := readJSON(filepath.Join(root, "state.json"), &state); err != nil {
		return state, fmt.Errorf("load shared state: %w", err)
	}
	if state.Entries == nil {
		state.Entries = map[string]entry{}
	}
	return state, nil
}

func saveShared(root string, state sharedState) error {
	return writeJSONAtomic(filepath.Join(root, "state.json"), state)
}

func shellPath(root, shell string) string {
	return filepath.Join(root, "shells", shell+".json")
}

func patchPath(root, shell string) string {
	return filepath.Join(root, "shells", shell+".patch.zsh")
}

func loadShell(root, shell string) (shellState, error) {
	var state shellState
	if err := readJSON(shellPath(root, shell), &state); err != nil {
		return state, fmt.Errorf("load shell %s: %w", shell, err)
	}
	if state.Baseline == nil {
		state.Baseline = map[string]entry{}
	}
	return state, nil
}

func saveShell(root string, state shellState) error {
	return writeJSONAtomic(shellPath(root, state.Shell), state)
}

func loadAllShells(root string) ([]shellState, error) {
	paths, err := filepath.Glob(filepath.Join(root, "shells", "*.json"))
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	states := make([]shellState, 0, len(paths))
	for _, path := range paths {
		var state shellState
		if err := readJSON(path, &state); err != nil {
			return nil, err
		}
		states = append(states, state)
	}
	return states, nil
}

func readJSON(path string, dst any) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	decoder := json.NewDecoder(f)
	if err := decoder.Decode(dst); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple JSON values")
		}
		return err
	}
	return nil
}

func writeJSONAtomic(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return writeFileAtomic(path, data, 0o600)
}

func writeFileAtomic(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	f, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-")
	if err != nil {
		return err
	}
	tmp := f.Name()
	cleanup := func() {
		_ = f.Close()
		_ = os.Remove(tmp)
	}
	if err := f.Chmod(mode); err != nil {
		cleanup()
		return err
	}
	if _, err := f.Write(data); err != nil {
		cleanup()
		return err
	}
	if err := f.Sync(); err != nil {
		cleanup()
		return err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}

func appendLog(root string, record logRecord) error {
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(root, "events.jsonl"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Write(append(data, '\n')); err != nil {
		return err
	}
	return f.Sync()
}

func logExclusions(root, shell string, exclusions []exclusion) error {
	for _, excluded := range exclusions {
		if err := appendLog(root, logRecord{At: now(), Event: "excluded", Shell: shell, Kind: excluded.Kind, Name: excluded.Name, Reason: excluded.Reason}); err != nil {
			return err
		}
	}
	return nil
}

func exclusionMap(exclusions []exclusion) map[string]string {
	result := make(map[string]string, len(exclusions))
	for _, excluded := range exclusions {
		result[excluded.Kind+"\x1f"+excluded.Name] = excluded.Reason
	}
	return result
}

func logNewExclusions(root, shell string, previous map[string]string, exclusions []exclusion) error {
	for _, excluded := range exclusions {
		key := excluded.Kind + "\x1f" + excluded.Name
		if previous[key] == excluded.Reason {
			continue
		}
		if err := appendLog(root, logRecord{At: now(), Event: "excluded", Shell: shell, Kind: excluded.Kind, Name: excluded.Name, Reason: excluded.Reason}); err != nil {
			return err
		}
	}
	return nil
}

func now() string { return time.Now().UTC().Format(time.RFC3339Nano) }

func parseSnapshot(in io.Reader) (map[string]entry, []exclusion, error) {
	data, err := io.ReadAll(io.LimitReader(in, maxSnapshotBytes+1))
	if err != nil {
		return nil, nil, err
	}
	if len(data) > maxSnapshotBytes {
		return nil, nil, fmt.Errorf("snapshot exceeds %d bytes", maxSnapshotBytes)
	}
	if len(data) == 0 {
		return nil, nil, errors.New("empty snapshot")
	}
	if data[len(data)-1] != 0 {
		return nil, nil, errors.New("snapshot is not NUL terminated")
	}
	fields := strings.Split(string(data[:len(data)-1]), "\x00")
	entries := map[string]entry{}
	var exclusions []exclusion
	for i := 0; i < len(fields); {
		if len(entries)+len(exclusions) >= maxSnapshotItems {
			return nil, nil, fmt.Errorf("snapshot exceeds %d records", maxSnapshotItems)
		}
		switch fields[i] {
		case "entry":
			if i+3 >= len(fields) {
				return nil, nil, errors.New("truncated entry record")
			}
			e := entry{Kind: fields[i+1], Name: fields[i+2], Present: true, Value: fields[i+3]}
			if err := validateEntry(e); err != nil {
				return nil, nil, err
			}
			entries[e.key()] = e
			i += 4
		case "array":
			if i+2 >= len(fields) {
				return nil, nil, errors.New("truncated array record")
			}
			count, err := strconv.Atoi(fields[i+2])
			if err != nil || count < 0 || count > maxSnapshotItems || i+3+count > len(fields) {
				return nil, nil, errors.New("invalid array record length")
			}
			e := entry{Kind: "array", Name: fields[i+1], Present: true, Values: append([]string(nil), fields[i+3:i+3+count]...)}
			if err := validateEntry(e); err != nil {
				return nil, nil, err
			}
			entries[e.key()] = e
			i += 3 + count
		case "exclude":
			if i+3 >= len(fields) {
				return nil, nil, errors.New("truncated exclusion record")
			}
			exclusions = append(exclusions, exclusion{Kind: fields[i+1], Name: fields[i+2], Reason: fields[i+3]})
			i += 4
		default:
			return nil, nil, fmt.Errorf("unknown snapshot record %q", fields[i])
		}
	}
	return entries, exclusions, nil
}

func validateEntry(e entry) error {
	switch e.Kind {
	case "env":
		if !validParameterName(e.Name) {
			return fmt.Errorf("invalid environment name %q", e.Name)
		}
	case "alias", "function":
		if e.Name == "" || strings.ContainsRune(e.Name, 0) {
			return fmt.Errorf("invalid %s name", e.Kind)
		}
	case "option":
		if !validParameterName(e.Name) || (e.Value != "on" && e.Value != "off") {
			return fmt.Errorf("invalid option %q", e.Name)
		}
	case "array":
		if e.Name != "PATH" && e.Name != "FPATH" {
			return fmt.Errorf("unsupported array %q", e.Name)
		}
	default:
		return fmt.Errorf("unsupported entry kind %q", e.Kind)
	}
	return nil
}

func validParameterName(s string) bool {
	if s == "" || !((s[0] >= 'a' && s[0] <= 'z') || (s[0] >= 'A' && s[0] <= 'Z') || s[0] == '_') {
		return false
	}
	for i := 1; i < len(s); i++ {
		c := s[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' {
			continue
		}
		return false
	}
	return true
}

func diffEntries(before, after map[string]entry) map[string]entry {
	delta := map[string]entry{}
	for key, current := range after {
		if old, ok := before[key]; !ok || !entriesEqual(old, current) {
			delta[key] = current
		}
	}
	for key, old := range before {
		if _, ok := after[key]; !ok {
			old.Present = false
			old.Value = ""
			old.Values = nil
			delta[key] = old
		}
	}
	return delta
}

func entriesEqual(a, b entry) bool {
	if a.Kind != b.Kind || a.Name != b.Name || a.Present != b.Present || a.Value != b.Value || len(a.Values) != len(b.Values) {
		return false
	}
	for i := range a.Values {
		if a.Values[i] != b.Values[i] {
			return false
		}
	}
	return true
}

func sortedChanges(delta map[string]entry) []change {
	keys := make([]string, 0, len(delta))
	for key := range delta {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	changes := make([]change, 0, len(keys))
	for _, key := range keys {
		changes = append(changes, change{Key: key, Entry: delta[key]})
	}
	return changes
}

func changedKeysSince(state sharedState, revision uint64) map[string]bool {
	keys := map[string]bool{}
	start := sort.Search(len(state.Events), func(i int) bool {
		return state.Events[i].Revision > revision
	})
	for _, event := range state.Events[start:] {
		for _, c := range event.Changes {
			keys[c.Key] = true
		}
	}
	return keys
}

func finalChangesSince(state sharedState, revision uint64) map[string]entry {
	start := sort.Search(len(state.Events), func(i int) bool {
		return state.Events[i].Revision > revision
	})
	result := map[string]entry{}
	for _, event := range state.Events[start:] {
		for _, c := range event.Changes {
			result[c.Key] = c.Entry
		}
	}
	for key, latest := range result {
		if current, ok := state.Entries[key]; ok {
			result[key] = current
		} else {
			latest.Present = false
			result[key] = latest
		}
	}
	return result
}

func finalChangesForShell(state sharedState, revision uint64, snapshot map[string]entry) map[string]entry {
	if revision >= state.CompactedThrough {
		return finalChangesSince(state, revision)
	}
	result := make(map[string]entry, len(state.Entries)+len(snapshot))
	for key, current := range state.Entries {
		result[key] = current
	}
	for key, current := range snapshot {
		if _, ok := state.Entries[key]; ok {
			continue
		}
		current.Present = false
		current.Value = ""
		current.Values = nil
		result[key] = current
	}
	return result
}

func compactRevisionHistory(state *sharedState) {
	if len(state.Events) <= maxRevisionEvents {
		return
	}
	remove := len(state.Events) - maxRevisionEvents
	state.CompactedThrough = state.Events[remove-1].Revision
	state.Events = append([]revisionEvent(nil), state.Events[remove:]...)
}

func displayKey(key string) string { return strings.Replace(key, "\x1f", ":", 1) }

func renderPatch(delta map[string]entry) (string, error) {
	if len(delta) == 0 {
		return "", nil
	}
	keys := make([]string, 0, len(delta))
	for key := range delta {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		a, b := delta[keys[i]], delta[keys[j]]
		order := map[string]int{"env": 0, "alias": 1, "function": 2, "array": 3, "option": 4}
		if order[a.Kind] != order[b.Kind] {
			return order[a.Kind] < order[b.Kind]
		}
		return a.Name < b.Name
	})
	var b strings.Builder
	b.WriteString("# generated by spike 001; validated before sourcing\n")
	for _, key := range keys {
		e := delta[key]
		if err := validateEntry(e); err != nil {
			return "", err
		}
		switch e.Kind {
		case "env":
			if e.Present {
				fmt.Fprintf(&b, "builtin typeset -gx -- %s=%s\n", e.Name, zshQuote(e.Value))
			} else {
				fmt.Fprintf(&b, "builtin unset -- %s\n", e.Name)
			}
		case "alias":
			if e.Present {
				fmt.Fprintf(&b, "builtin alias -- %s\n", zshQuote(e.Name+"="+e.Value))
			} else {
				fmt.Fprintf(&b, "builtin unalias -- %s 2>/dev/null || true\n", zshQuote(e.Name))
			}
		case "function":
			if e.Present {
				fmt.Fprintf(&b, "_ZP_SPIKE_PATCH_NAME=%s\nfunctions[$_ZP_SPIKE_PATCH_NAME]=%s\n", zshQuote(e.Name), zshQuote(e.Value))
			} else {
				fmt.Fprintf(&b, "builtin unfunction -- %s 2>/dev/null || true\n", zshQuote(e.Name))
			}
		case "array":
			arrayName := strings.ToLower(e.Name)
			fmt.Fprintf(&b, "builtin typeset -ga %s\n%s=(", arrayName, arrayName)
			for _, value := range e.Values {
				fmt.Fprintf(&b, " %s", zshQuote(value))
			}
			b.WriteString(" )\n")
		case "option":
			if e.Value == "on" && e.Present {
				fmt.Fprintf(&b, "builtin setopt -- %s\n", e.Name)
			} else {
				fmt.Fprintf(&b, "builtin unsetopt -- %s\n", e.Name)
			}
		}
	}
	return b.String(), nil
}

func zshQuote(s string) string {
	var b strings.Builder
	b.WriteString("$'")
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '\\':
			b.WriteString("\\\\")
		case '\'':
			b.WriteString("\\'")
		case '\n':
			b.WriteString("\\n")
		case '\r':
			b.WriteString("\\r")
		case '\t':
			b.WriteString("\\t")
		default:
			if c < 0x20 || c == 0x7f {
				fmt.Fprintf(&b, "\\x%02x", c)
			} else {
				b.WriteByte(c)
			}
		}
	}
	b.WriteByte('\'')
	return b.String()
}
