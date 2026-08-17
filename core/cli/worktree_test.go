package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"zsh-pro/core/model"
	"zsh-pro/core/worktree"
)

type recordingWorktree struct {
	status worktree.WorkflowStatus
	diff   model.CategorizedDiff
	result model.WorktreeCommitResult
	err    error
	calls  []string
}

func (service *recordingWorktree) WorkflowStatus(_ context.Context, shellID string) (worktree.WorkflowStatus, error) {
	service.calls = append(service.calls, "status:"+shellID)
	return service.status, service.err
}

func (service *recordingWorktree) Diff(context.Context) (model.CategorizedDiff, error) {
	service.calls = append(service.calls, "diff")
	return service.diff, service.err
}

func (service *recordingWorktree) Branches(context.Context) ([]string, error) {
	service.calls = append(service.calls, "branches")
	return []string{"main", "feature"}, service.err
}

func (service *recordingWorktree) Commit(_ context.Context, message string) (model.WorktreeCommitResult, error) {
	service.calls = append(service.calls, "commit:"+message)
	return service.result, service.err
}

func (service *recordingWorktree) Branch(_ context.Context, branch string) error {
	service.calls = append(service.calls, "branch:"+branch)
	return service.err
}

func (service *recordingWorktree) Checkout(_ context.Context, branch string, create bool) error {
	service.calls = append(service.calls, fmt.Sprintf("checkout:%s:%t", branch, create))
	return service.err
}

func (service *recordingWorktree) ResetHard(context.Context) error {
	service.calls = append(service.calls, "reset:hard")
	return service.err
}

func (service *recordingWorktree) SetAutoApplyDefault(_ context.Context, enabled bool) error {
	service.calls = append(service.calls, fmt.Sprintf("config:auto-apply:%t", enabled))
	return service.err
}

func worktreeCLI(service *recordingWorktree) *CLI {
	return NewWithWorktree(nil, nil, nil, service, service)
}

func runWorktreeCLI(t *testing.T, cli *CLI, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := cli.Run(args, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func TestWorktreeStatusRendersSharedAndSourcedShellTruth(t *testing.T) {
	base := worktree.WorkflowStatus{
		Worktree:         model.WorktreeStatus{Branch: "main", BaseOID: strings.Repeat("a", 40), Revision: 9, DirtyCount: 2, ConflictCount: 1},
		PersistedDefault: false, Effective: false, Source: worktree.AutoApplyDefaultSource,
	}
	withoutShell := &recordingWorktree{status: base}
	unsetenvForTest(t, "ZSHPRO_SHELL_ID")
	code, stdout, stderr := runWorktreeCLI(t, worktreeCLI(withoutShell), "status")
	want := "branch: main\nrevision: 9\ndirty identities: 2\nconflicts: 1\nauto-apply default: false\ncurrent shell: unattached\nauto-apply effective: false\nauto-apply source: default\n"
	if code != int(model.ExitClean) || stdout != want || stderr != "" || !equalStrings(withoutShell.calls, []string{"status:"}) {
		t.Fatalf("shared status = code %d stdout %q stderr %q calls %v", code, stdout, stderr, withoutShell.calls)
	}
	if strings.Contains(stdout, base.Worktree.BaseOID) {
		t.Fatal("status exposed raw object ID")
	}

	attachedStatus := base
	attachedStatus.Worktree.Shell = &model.ShellWorktreeStatus{Attached: true, AppliedRevision: 7, Behind: true, AutoApply: true, ConflictCount: 1}
	attachedStatus.Effective = true
	attachedStatus.Source = worktree.AutoApplyShellSource
	attached := &recordingWorktree{status: attachedStatus}
	t.Setenv("ZSHPRO_SHELL_ID", "shell-a")
	code, stdout, stderr = runWorktreeCLI(t, worktreeCLI(attached), "status")
	want = "branch: main\nrevision: 9\ndirty identities: 2\nconflicts: 1\nauto-apply default: false\ncurrent shell: attached\napplied revision: 7\nbehind: true\nshell conflicts: 1\nauto-apply effective: true\nauto-apply source: shell\n"
	if code != int(model.ExitClean) || stdout != want || stderr != "" || !equalStrings(attached.calls, []string{"status:shell-a"}) {
		t.Fatalf("shell status = code %d stdout %q stderr %q calls %v", code, stdout, stderr, attached.calls)
	}
}

func TestSourcedShellIDRejectsMalformedBeforeWorktreeAccess(t *testing.T) {
	for _, shellID := range []string{"", "-leading", "has space", strings.Repeat("a", 129), "line\nbreak"} {
		t.Run(fmt.Sprintf("length-%d", len(shellID)), func(t *testing.T) {
			service := &recordingWorktree{}
			t.Setenv("ZSHPRO_SHELL_ID", shellID)
			code, stdout, stderr := runWorktreeCLI(t, worktreeCLI(service), "status")
			if code != int(model.ExitUsageErr) || stdout != "" || len(service.calls) != 0 {
				t.Fatalf("malformed shell ID = code %d stdout %q stderr %q calls %v", code, stdout, stderr, service.calls)
			}
			if (shellID != "" && strings.Contains(stderr, shellID)) || !strings.Contains(stderr, "usage: zsh-pro status") {
				t.Fatalf("unsafe shell-ID diagnostic = %q", stderr)
			}
		})
	}
}

func TestWorktreeDiffUsesStableValueFreeCategoryOrder(t *testing.T) {
	service := &recordingWorktree{diff: model.CategorizedDiff{
		Environment: []model.DiffEntry{{Kind: model.LiveUpdate, Identity: model.Identity{Kind: model.LiveEnv, Name: "EDITOR"}}},
		Aliases:     []model.DiffEntry{{Kind: model.LiveAdd, Identity: model.Identity{Kind: model.LiveAlias, Name: "gs"}}},
		Functions:   []model.DiffEntry{{Kind: model.LiveRemove, Identity: model.Identity{Kind: model.LiveFunction, Name: "prompt"}}},
		Path:        []model.DiffEntry{{Kind: model.LiveUpdate, Identity: model.Identity{Kind: model.LivePath, Name: "PATH"}}},
		FPath:       []model.DiffEntry{{Kind: model.LiveAdd, Identity: model.Identity{Kind: model.LiveFPath, Name: "FPATH"}}},
		Options:     []model.DiffEntry{{Kind: model.LiveUpdate, Identity: model.Identity{Kind: model.LiveOption, Name: "AUTO_CD"}}},
	}}
	code, stdout, stderr := runWorktreeCLI(t, worktreeCLI(service), "diff")
	want := "environment:\n  update EDITOR\naliases:\n  add gs\nfunctions:\n  remove prompt\npath:\n  update PATH\nfpath:\n  add FPATH\noptions:\n  update AUTO_CD\n"
	if code != int(model.ExitClean) || stdout != want || stderr != "" || !equalStrings(service.calls, []string{"diff"}) {
		t.Fatalf("diff = code %d stdout %q stderr %q calls %v", code, stdout, stderr, service.calls)
	}
	if strings.Contains(stdout, "captured-secret-canary") {
		t.Fatal("diff exposed captured value")
	}
}

func TestCommitBranchCheckoutResetAndAutoApplyDelegateExactRequests(t *testing.T) {
	tests := []struct {
		name string
		args []string
		call string
		out  string
	}{
		{name: "commit", args: []string{"commit", "-m", "message"}, call: "commit:message", out: "committed\n"},
		{name: "branch list", args: []string{"branch"}, call: "branches", out: "feature\nmain\n"},
		{name: "branch", args: []string{"branch", "feature"}, call: "branch:feature", out: "branch created\n"},
		{name: "checkout", args: []string{"checkout", "feature"}, call: "checkout:feature:false", out: "checked out\n"},
		{name: "checkout create", args: []string{"checkout", "-b", "new"}, call: "checkout:new:true", out: "checked out\n"},
		{name: "reset", args: []string{"reset", "--hard"}, call: "reset:hard", out: "worktree reset\n"},
		{name: "config false", args: []string{"config", "set", "auto-apply", "false"}, call: "config:auto-apply:false", out: "auto-apply default: false\n"},
		{name: "config true", args: []string{"config", "set", "auto-apply", "true"}, call: "config:auto-apply:true", out: "auto-apply default: true\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			service := &recordingWorktree{result: model.WorktreeCommitResult{Committed: true, OID: strings.Repeat("b", 40)}}
			code, stdout, stderr := runWorktreeCLI(t, worktreeCLI(service), tc.args...)
			if code != int(model.ExitClean) || stdout != tc.out || stderr != "" || !equalStrings(service.calls, []string{tc.call}) {
				t.Fatalf("result = code %d stdout %q stderr %q calls %v", code, stdout, stderr, service.calls)
			}
			if strings.Contains(stdout, service.result.OID) {
				t.Fatal("workflow output exposed raw object ID")
			}
		})
	}
}

func TestCommitOutcomesRemainTruthfulAndValueSafe(t *testing.T) {
	tests := []struct {
		name   string
		result model.WorktreeCommitResult
		err    error
		code   int
		stdout string
		stderr string
	}{
		{name: "clean", code: int(model.ExitClean), stdout: "nothing to commit\n"},
		{name: "conflict", result: model.WorktreeCommitResult{Conflict: true}, err: errors.New("raw-conflict-canary"), code: int(model.ExitActionable), stderr: "zsh-pro: worktree commit conflict\n"},
		{name: "published recovery", result: model.WorktreeCommitResult{Committed: true, OID: strings.Repeat("c", 40), RecoveryRequired: true}, err: errors.New("raw-recovery-canary"), code: int(model.ExitRuntimeErr), stderr: "zsh-pro: commit published; recovery required\n"},
		{name: "unpublished recovery", result: model.WorktreeCommitResult{RecoveryRequired: true}, err: errors.New("raw-recovery-canary"), code: int(model.ExitRuntimeErr), stderr: "zsh-pro: worktree recovery required\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			service := &recordingWorktree{result: tc.result, err: tc.err}
			code, stdout, stderr := runWorktreeCLI(t, worktreeCLI(service), "commit", "-m", "message")
			if code != tc.code || stdout != tc.stdout || stderr != tc.stderr || !equalStrings(service.calls, []string{"commit:message"}) {
				t.Fatalf("outcome = code %d stdout %q stderr %q calls %v", code, stdout, stderr, service.calls)
			}
			if strings.Contains(stdout+stderr, tc.result.OID) && tc.result.OID != "" {
				t.Fatal("commit outcome exposed raw object ID")
			}
			if strings.Contains(stdout+stderr, "raw-") {
				t.Fatal("commit outcome exposed raw service error")
			}
		})
	}
}

func TestUnsupportedGitAndMalformedWorktreeCommandsNeverReachDependencies(t *testing.T) {
	invalid := [][]string{
		{"status", "extra"}, {"diff", "--raw"}, {"commit"}, {"commit", "message"}, {"commit", "-m"}, {"commit", "-m", ""}, {"commit", "-m", "message", "extra"},
		{"branch", "one", "two"}, {"checkout"}, {"checkout", "-b"}, {"checkout", "--force", "main"}, {"reset"}, {"reset", "--soft"},
		{"config"}, {"config", "set", "auto-apply", "TRUE"}, {"config", "set", "auto-apply", "false", "extra"},
		{"sync"}, {"sync", "--resolve", "shared"}, {"add", "."}, {"stage", "."}, {"merge", "main"}, {"rebase", "main"},
		{"cherry-pick", "deadbeef"}, {"remote", "-v"}, {"push"}, {"pull"}, {"worktree", "add", "elsewhere"},
	}
	for _, args := range invalid {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			service := &recordingWorktree{}
			code, stdout, stderr := runWorktreeCLI(t, worktreeCLI(service), args...)
			if code != int(model.ExitUsageErr) || stdout != "" || stderr == "" || len(service.calls) != 0 {
				t.Fatalf("invalid %q = code %d stdout %q stderr %q calls %v", args, code, stdout, stderr, service.calls)
			}
		})
	}
}

func TestWorktreeErrorsAreValueSafe(t *testing.T) {
	service := &recordingWorktree{err: errors.New("captured-secret-canary " + strings.Repeat("c", 40))}
	code, stdout, stderr := runWorktreeCLI(t, worktreeCLI(service), "diff")
	if code != int(model.ExitRuntimeErr) || stdout != "" || stderr == "" || strings.Contains(stderr, "captured-secret-canary") || strings.Contains(stderr, strings.Repeat("c", 40)) {
		t.Fatalf("unsafe error = code %d stdout %q stderr %q", code, stdout, stderr)
	}
}

type panicLegacyStore struct{}

func (panicLegacyStore) Branches(context.Context) ([]string, error) {
	panic("legacy Branches called")
}
func (panicLegacyStore) Current() string { panic("legacy Current called") }
func (panicLegacyStore) Checkout(context.Context, string) error {
	panic("legacy Checkout called")
}
func (panicLegacyStore) Read(context.Context, string) (model.Profile, error) {
	panic("legacy Read called")
}

func TestWorktreeCommandsNeverUseLegacyStoreAuthority(t *testing.T) {
	unsetenvForTest(t, "ZSHPRO_SHELL_ID")
	service := &recordingWorktree{
		status: worktree.WorkflowStatus{
			Worktree: model.WorktreeStatus{Branch: "main", Revision: 1}, PersistedDefault: true, Effective: true, Source: worktree.AutoApplyDefaultSource,
		},
		result: model.WorktreeCommitResult{Committed: true, OID: strings.Repeat("d", 40)},
	}
	program := NewWithWorktree(nil, panicLegacyStore{}, nil, service, service)
	for _, args := range [][]string{
		{"status"}, {"diff"}, {"commit", "-m", "message"}, {"branch"}, {"branch", "feature"},
		{"checkout", "feature"}, {"reset", "--hard"}, {"config", "set", "auto-apply", "true"},
	} {
		if code, _, stderr := runWorktreeCLI(t, program, args...); code != int(model.ExitClean) {
			t.Fatalf("worktree command %q touched legacy authority or failed: code %d stderr %q", args, code, stderr)
		}
	}
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func unsetenvForTest(t *testing.T, name string) {
	t.Helper()
	value, present := os.LookupEnv(name)
	if err := os.Unsetenv(name); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if present {
			_ = os.Setenv(name, value)
			return
		}
		_ = os.Unsetenv(name)
	})
}
