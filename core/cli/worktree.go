package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strings"

	"zsh-pro/core/model"
	"zsh-pro/core/worktree"
)

const (
	maxWorktreeBranchBytes = 255
	maxCommitMessageBytes  = 64 * 1024
)

var (
	worktreeBranchRE  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]*$`)
	worktreeShellIDRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]*$`)
	worktreeOIDRE     = regexp.MustCompile(`^[0-9a-f]{40}([0-9a-f]{24})?$`)
)

type worktreeCommandKind uint8

const (
	worktreeStatus worktreeCommandKind = iota + 1
	worktreeDiff
	worktreeCommit
	worktreeBranchList
	worktreeBranch
	worktreeCheckout
	worktreeReset
	worktreeConfig
)

type worktreeCommand struct {
	kind      worktreeCommandKind
	message   string
	branch    string
	create    bool
	autoApply bool
}

// runWorktree parses the entire public invocation before reading sourced-shell
// context or touching either narrow service dependency.
func (c *CLI) runWorktree(args []string, stdout, stderr io.Writer) int {
	command, usage, ok := parseWorktreeCommand(args)
	if !ok {
		_, _ = fmt.Fprintln(stderr, usage)
		return int(model.ExitUsageErr)
	}

	shellID := ""
	if command.kind == worktreeStatus {
		if value, present := os.LookupEnv("ZSHPRO_SHELL_ID"); present {
			if !validWorktreeShellID(value) {
				_, _ = fmt.Fprintln(stderr, "usage: zsh-pro status")
				return int(model.ExitUsageErr)
			}
			shellID = value
		}
	}

	ctx := context.Background()
	switch command.kind {
	case worktreeStatus:
		if isNilLike(c.worktreeReader) {
			return c.fail(stdout, stderr, false, "worktree status unavailable")
		}
		status, err := c.worktreeReader.WorkflowStatus(ctx, shellID)
		if err != nil {
			return c.fail(stdout, stderr, false, "worktree status unavailable")
		}
		rendered, err := renderWorktreeStatus(status)
		if err != nil {
			return c.fail(stdout, stderr, false, "worktree status unavailable")
		}
		_, _ = io.WriteString(stdout, rendered)
		return int(model.ExitClean)
	case worktreeDiff:
		if isNilLike(c.worktreeReader) {
			return c.fail(stdout, stderr, false, "worktree diff unavailable")
		}
		diff, err := c.worktreeReader.Diff(ctx)
		if err != nil {
			return c.fail(stdout, stderr, false, "worktree diff unavailable")
		}
		rendered, err := renderWorktreeDiff(diff)
		if err != nil {
			return c.fail(stdout, stderr, false, "worktree diff unavailable")
		}
		_, _ = io.WriteString(stdout, rendered)
		return int(model.ExitClean)
	case worktreeCommit:
		if isNilLike(c.worktreeWorkflow) {
			return c.fail(stdout, stderr, false, "worktree commit unavailable")
		}
		result, err := c.worktreeWorkflow.Commit(ctx, command.message)
		return renderWorktreeCommitResult(result, err, stdout, stderr)
	case worktreeBranchList:
		if isNilLike(c.worktreeReader) {
			return c.fail(stdout, stderr, false, "worktree branches unavailable")
		}
		branches, err := c.worktreeReader.Branches(ctx)
		if err != nil {
			return c.fail(stdout, stderr, false, "worktree branches unavailable")
		}
		rendered, err := renderWorktreeBranches(branches)
		if err != nil {
			return c.fail(stdout, stderr, false, "worktree branches unavailable")
		}
		_, _ = io.WriteString(stdout, rendered)
		return int(model.ExitClean)
	case worktreeBranch:
		if isNilLike(c.worktreeWorkflow) {
			return c.fail(stdout, stderr, false, "worktree branch unavailable")
		}
		if err := c.worktreeWorkflow.Branch(ctx, command.branch); err != nil {
			return c.fail(stdout, stderr, false, "worktree branch failed")
		}
		_, _ = fmt.Fprintln(stdout, "branch created")
		return int(model.ExitClean)
	case worktreeCheckout:
		if isNilLike(c.worktreeWorkflow) {
			return c.fail(stdout, stderr, false, "worktree checkout unavailable")
		}
		if err := c.worktreeWorkflow.Checkout(ctx, command.branch, command.create); err != nil {
			return c.fail(stdout, stderr, false, "worktree checkout failed")
		}
		_, _ = fmt.Fprintln(stdout, "checked out")
		return int(model.ExitClean)
	case worktreeReset:
		if isNilLike(c.worktreeWorkflow) {
			return c.fail(stdout, stderr, false, "worktree reset unavailable")
		}
		if err := c.worktreeWorkflow.ResetHard(ctx); err != nil {
			return c.fail(stdout, stderr, false, "worktree reset failed")
		}
		_, _ = fmt.Fprintln(stdout, "worktree reset")
		return int(model.ExitClean)
	case worktreeConfig:
		if isNilLike(c.worktreeWorkflow) {
			return c.fail(stdout, stderr, false, "worktree config unavailable")
		}
		if err := c.worktreeWorkflow.SetAutoApplyDefault(ctx, command.autoApply); err != nil {
			return c.fail(stdout, stderr, false, "worktree config failed")
		}
		_, _ = fmt.Fprintf(stdout, "auto-apply default: %t\n", command.autoApply)
		return int(model.ExitClean)
	default:
		return c.fail(stdout, stderr, false, "worktree workflow unavailable")
	}
}

func parseWorktreeCommand(args []string) (worktreeCommand, string, bool) {
	if len(args) == 0 {
		return worktreeCommand{}, "usage: zsh-pro <command>", false
	}
	switch args[0] {
	case "status":
		return exactWorktreeCommand(args, 1, worktreeCommand{kind: worktreeStatus}, "usage: zsh-pro status")
	case "diff":
		return exactWorktreeCommand(args, 1, worktreeCommand{kind: worktreeDiff}, "usage: zsh-pro diff")
	case "commit":
		usage := "usage: zsh-pro commit -m <message>"
		if len(args) != 3 || args[1] != "-m" || !validCommitMessageArgument(args[2]) {
			return worktreeCommand{}, usage, false
		}
		return worktreeCommand{kind: worktreeCommit, message: args[2]}, usage, true
	case "branch":
		usage := "usage: zsh-pro branch [name]"
		if len(args) == 1 {
			return worktreeCommand{kind: worktreeBranchList}, usage, true
		}
		if len(args) != 2 || !validWorktreeBranch(args[1]) {
			return worktreeCommand{}, usage, false
		}
		return worktreeCommand{kind: worktreeBranch, branch: args[1]}, usage, true
	case "checkout":
		usage := "usage: zsh-pro checkout [-b] <name>"
		switch {
		case len(args) == 2 && validWorktreeBranch(args[1]):
			return worktreeCommand{kind: worktreeCheckout, branch: args[1]}, usage, true
		case len(args) == 3 && args[1] == "-b" && validWorktreeBranch(args[2]):
			return worktreeCommand{kind: worktreeCheckout, branch: args[2], create: true}, usage, true
		default:
			return worktreeCommand{}, usage, false
		}
	case "reset":
		usage := "usage: zsh-pro reset --hard"
		if len(args) != 2 || args[1] != "--hard" {
			return worktreeCommand{}, usage, false
		}
		return worktreeCommand{kind: worktreeReset}, usage, true
	case "config":
		usage := "usage: zsh-pro config set auto-apply <true|false>"
		if len(args) != 4 || args[1] != "set" || args[2] != "auto-apply" || (args[3] != "true" && args[3] != "false") {
			return worktreeCommand{}, usage, false
		}
		return worktreeCommand{kind: worktreeConfig, autoApply: args[3] == "true"}, usage, true
	default:
		return worktreeCommand{}, "usage: zsh-pro <command>", false
	}
}

func exactWorktreeCommand(args []string, length int, command worktreeCommand, usage string) (worktreeCommand, string, bool) {
	if len(args) != length {
		return worktreeCommand{}, usage, false
	}
	return command, usage, true
}

func validCommitMessageArgument(message string) bool {
	return message != "" && len(message) <= maxCommitMessageBytes && strings.IndexByte(message, 0) < 0
}

func validWorktreeBranch(branch string) bool {
	if len(branch) == 0 || len(branch) > maxWorktreeBranchBytes || !worktreeBranchRE.MatchString(branch) {
		return false
	}
	return branch != "." && branch != "@" && !strings.Contains(branch, "..") && !strings.Contains(branch, "@{") &&
		!strings.Contains(branch, "//") && !strings.Contains(branch, "/.") && !strings.HasSuffix(branch, "/") &&
		!strings.HasSuffix(branch, ".") && !strings.HasSuffix(branch, ".lock")
}

func validWorktreeShellID(shellID string) bool {
	return len(shellID) > 0 && len(shellID) <= model.MaxResolutionTokenBytes && worktreeShellIDRE.MatchString(shellID)
}

func renderWorktreeStatus(status worktree.WorkflowStatus) (string, error) {
	if !validWorktreeBranch(status.Worktree.Branch) || status.Worktree.Revision == 0 || status.Worktree.DirtyCount < 0 || status.Worktree.ConflictCount < 0 {
		return "", errors.New("invalid worktree status")
	}
	if status.Source != worktree.AutoApplyDefaultSource && status.Source != worktree.AutoApplyShellSource {
		return "", errors.New("invalid auto-apply source")
	}
	if status.Source == worktree.AutoApplyShellSource && status.Worktree.Shell == nil {
		return "", errors.New("shell auto-apply source has no shell")
	}
	var rendered bytes.Buffer
	_, _ = fmt.Fprintf(&rendered, "branch: %s\n", status.Worktree.Branch)
	_, _ = fmt.Fprintf(&rendered, "revision: %d\n", status.Worktree.Revision)
	_, _ = fmt.Fprintf(&rendered, "dirty identities: %d\n", status.Worktree.DirtyCount)
	_, _ = fmt.Fprintf(&rendered, "conflicts: %d\n", status.Worktree.ConflictCount)
	_, _ = fmt.Fprintf(&rendered, "auto-apply default: %t\n", status.PersistedDefault)
	if status.Worktree.Shell == nil {
		_, _ = fmt.Fprintln(&rendered, "current shell: unattached")
	} else {
		shell := status.Worktree.Shell
		if !shell.Attached || shell.AppliedRevision > status.Worktree.Revision || shell.ConflictCount < 0 || shell.AutoApply != status.Effective {
			return "", errors.New("invalid shell worktree status")
		}
		_, _ = fmt.Fprintln(&rendered, "current shell: attached")
		_, _ = fmt.Fprintf(&rendered, "applied revision: %d\n", shell.AppliedRevision)
		_, _ = fmt.Fprintf(&rendered, "behind: %t\n", shell.Behind)
		_, _ = fmt.Fprintf(&rendered, "shell conflicts: %d\n", shell.ConflictCount)
	}
	_, _ = fmt.Fprintf(&rendered, "auto-apply effective: %t\n", status.Effective)
	_, _ = fmt.Fprintf(&rendered, "auto-apply source: %s\n", status.Source)
	return rendered.String(), nil
}

func renderWorktreeDiff(diff model.CategorizedDiff) (string, error) {
	categories := []struct {
		name    string
		kind    model.LiveKind
		entries []model.DiffEntry
	}{
		{name: "environment", kind: model.LiveEnv, entries: diff.Environment},
		{name: "aliases", kind: model.LiveAlias, entries: diff.Aliases},
		{name: "functions", kind: model.LiveFunction, entries: diff.Functions},
		{name: "path", kind: model.LivePath, entries: diff.Path},
		{name: "fpath", kind: model.LiveFPath, entries: diff.FPath},
		{name: "options", kind: model.LiveOption, entries: diff.Options},
	}
	var rendered bytes.Buffer
	for _, category := range categories {
		entries := append([]model.DiffEntry(nil), category.entries...)
		for _, entry := range entries {
			if entry.Identity.Kind != category.kind || model.ValidateIdentity(entry.Identity) != nil || !validDiffKind(entry.Kind) {
				return "", errors.New("invalid categorized worktree diff")
			}
		}
		sort.Slice(entries, func(left, right int) bool {
			if entries[left].Identity.Name != entries[right].Identity.Name {
				return entries[left].Identity.Name < entries[right].Identity.Name
			}
			return entries[left].Kind < entries[right].Kind
		})
		_, _ = fmt.Fprintf(&rendered, "%s:\n", category.name)
		for _, entry := range entries {
			_, _ = fmt.Fprintf(&rendered, "  %s %s\n", entry.Kind, entry.Identity.Name)
		}
	}
	return rendered.String(), nil
}

func renderWorktreeBranches(branches []string) (string, error) {
	names := append([]string(nil), branches...)
	for _, branch := range names {
		if !validWorktreeBranch(branch) {
			return "", errors.New("invalid worktree branch list")
		}
	}
	sort.Strings(names)
	var rendered bytes.Buffer
	for _, branch := range names {
		_, _ = fmt.Fprintln(&rendered, branch)
	}
	return rendered.String(), nil
}

func validDiffKind(kind model.LiveChangeKind) bool {
	return kind == model.LiveAdd || kind == model.LiveUpdate || kind == model.LiveRemove
}

func renderWorktreeCommitResult(result model.WorktreeCommitResult, err error, stdout, stderr io.Writer) int {
	if result.Conflict {
		_, _ = fmt.Fprintln(stderr, "zsh-pro: worktree commit conflict")
		return int(model.ExitActionable)
	}
	if result.RecoveryRequired {
		if result.Committed {
			_, _ = fmt.Fprintln(stderr, "zsh-pro: commit published; recovery required")
		} else {
			_, _ = fmt.Fprintln(stderr, "zsh-pro: worktree recovery required")
		}
		return int(model.ExitRuntimeErr)
	}
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "zsh-pro: worktree commit failed")
		return int(model.ExitRuntimeErr)
	}
	if result.Committed {
		if !worktreeOIDRE.MatchString(result.OID) {
			_, _ = fmt.Fprintln(stderr, "zsh-pro: worktree commit outcome invalid")
			return int(model.ExitRuntimeErr)
		}
		_, _ = fmt.Fprintln(stdout, "committed")
		return int(model.ExitClean)
	}
	if result.OID != "" {
		_, _ = fmt.Fprintln(stderr, "zsh-pro: worktree commit outcome invalid")
		return int(model.ExitRuntimeErr)
	}
	_, _ = fmt.Fprintln(stdout, "nothing to commit")
	return int(model.ExitClean)
}
