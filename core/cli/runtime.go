package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"
)

const (
	runtimeTimeoutMinSeconds = 1
	runtimeTimeoutMaxSeconds = 99
)

// runtimeRootValidated is a package-local synchronization seam for the
// descriptor-replacement regression. Production leaves it as a no-op; the test
// pauses here, after authentication and before the descriptor-bound store is
// constructed, to prove no later path reopen can observe a swapped root.
var runtimeRootValidated = func(*RuntimeRoot) {}

type runtimeRootEmitter interface {
	emitFromRuntimeRoot(context.Context, *RuntimeRoot, string, string) (string, error)
	listFromRuntimeRoot(context.Context, *RuntimeRoot) ([]string, error)
}

// runRuntime owns the private, sourced-loader transport commands. They are
// intentionally not part of the user-facing profile surface: the loader uses
// them to keep emitted source on process-owned pipes rather than reopening a
// pathname below a configurable runtime directory.
func (c *CLI) runRuntime(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		return runtimeFail(stderr, 2, "usage: zsh-pro runtime <capture|validate> ...")
	}
	switch args[0] {
	case "capture":
		return c.runRuntimeCapture(args[1:], stdout, stderr)
	case "validate":
		return c.runRuntimeValidate(args[1:], stderr)
	default:
		return runtimeFail(stderr, 2, fmt.Sprintf("unknown runtime command %q", args[0]))
	}
}

// runRuntimeCapture executes only the loader's emit/list request. For emit it
// authenticates ZSHPRO_HOME once and constructs a descriptor-bound store in
// this process; it never launches a second composition root that can reopen the
// original pathname. Its stdout stays in memory until the request succeeds, so
// no emitted source is written beneath an attacker-replaceable path.
func (c *CLI) runRuntimeCapture(args []string, stdout, stderr io.Writer) int {
	if len(args) < 4 || args[1] != "--" {
		return runtimeFail(stderr, 2, "usage: zsh-pro runtime capture <seconds> -- zsh-pro <emit|list> ...")
	}
	timeout, err := parseRuntimeTimeout(args[0])
	if err != nil {
		return runtimeFail(stderr, 2, err.Error())
	}
	childArgs := args[2:]
	if len(childArgs) < 2 || childArgs[0] != "zsh-pro" {
		return runtimeFail(stderr, 2, "runtime capture only permits zsh-pro emit or list")
	}
	var root *RuntimeRoot
	switch childArgs[1] {
	case "emit":
		if len(childArgs) != 4 || (childArgs[2] != "apply" && childArgs[2] != "deactivate") || childArgs[3] == "" {
			return runtimeFail(stderr, 2, "runtime capture requires zsh-pro emit <apply|deactivate> <profile>")
		}
		rootPath, err := runtimeRoot()
		if err != nil {
			return runtimeFail(stderr, 1, err.Error())
		}
		// Before the emitter starts, secureRuntimeRoot walks every component
		// by descriptor with O_NOFOLLOW and leaves the terminal descriptors
		// open. The emission below must consume these exact objects rather than
		// root, whose spelling may be replaced after this check.
		root, err = secureRuntimeRoot(rootPath)
		if err != nil {
			return runtimeFail(stderr, 1, "runtime staging root is unsafe")
		}
		defer func() { _ = root.Close() }()
		runtimeRootValidated(root)
	case "list":
		if len(childArgs) != 2 {
			return runtimeFail(stderr, 2, "runtime capture requires zsh-pro list without arguments")
		}
		rootPath, err := runtimeRoot()
		if err != nil {
			return runtimeFail(stderr, 1, err.Error())
		}
		root, err = secureRuntimeRoot(rootPath)
		if err != nil {
			return runtimeFail(stderr, 1, "runtime staging root is unsafe")
		}
		defer func() { _ = root.Close() }()
		runtimeRootValidated(root)
	default:
		return runtimeFail(stderr, 2, "runtime capture only permits zsh-pro emit or list")
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	bound, ok := c.emitter.(runtimeRootEmitter)
	if !ok {
		return runtimeFail(stderr, 1, "runtime emitter is unavailable")
	}
	if childArgs[1] == "list" {
		branches, err := bound.listFromRuntimeRoot(ctx, root)
		if err != nil {
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return runtimeFail(stderr, 124, "runtime command timed out")
			}
			return runtimeFail(stderr, 1, "runtime command failed")
		}
		for _, branch := range branches {
			if _, err := fmt.Fprintln(stdout, branch); err != nil {
				return runtimeFail(stderr, 1, "write captured runtime source")
			}
		}
		return 0
	}

	source, err := bound.emitFromRuntimeRoot(ctx, root, childArgs[2], childArgs[3])
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return runtimeFail(stderr, 124, "runtime command timed out")
		}
		return runtimeFail(stderr, 1, "runtime command failed")
	}
	if source == "" {
		return runtimeFail(stderr, 1, "emit produced empty output")
	}
	if _, err := io.WriteString(stdout, source); err != nil {
		return runtimeFail(stderr, 1, "write captured runtime source")
	}
	return 0
}

// runRuntimeValidate validates exactly the pipe-fed emitted bytes. stderr is
// deliberately discarded in validateRuntimeSource: parser diagnostics can
// contain source-adjacent data, including resolved secrets.
func (c *CLI) runRuntimeValidate(args []string, stderr io.Writer) int {
	if len(args) != 1 {
		return runtimeFail(stderr, 2, "usage: zsh-pro runtime validate <seconds>")
	}
	timeout, err := parseRuntimeTimeout(args[0])
	if err != nil {
		return runtimeFail(stderr, 2, err.Error())
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if err := validateRuntimeSource(ctx, os.Stdin); err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return runtimeFail(stderr, 124, "runtime source validation timed out")
		}
		return runtimeFail(stderr, 1, "runtime source validation failed")
	}
	return 0
}

func parseRuntimeTimeout(raw string) (time.Duration, error) {
	seconds, err := strconv.Atoi(raw)
	if err != nil || seconds < runtimeTimeoutMinSeconds || seconds > runtimeTimeoutMaxSeconds {
		return 0, fmt.Errorf("runtime timeout must be an integer from %d to %d seconds", runtimeTimeoutMinSeconds, runtimeTimeoutMaxSeconds)
	}
	return time.Duration(seconds) * time.Second, nil
}

func runtimeRoot() (string, error) {
	if root, ok := os.LookupEnv("ZSHPRO_HOME"); ok {
		if root == "" || !filepath.IsAbs(root) {
			return "", errors.New("runtime staging requires an absolute non-empty ZSHPRO_HOME")
		}
		return root, nil
	}
	home, ok := os.LookupEnv("HOME")
	if !ok || home == "" || !filepath.IsAbs(home) {
		return "", errors.New("runtime staging requires an absolute non-empty HOME")
	}
	return filepath.Join(home, ".zsh-pro"), nil
}

func validateRuntimeSource(ctx context.Context, source io.Reader) error {
	zsh, err := exec.LookPath("zsh")
	if err != nil {
		return fmt.Errorf("find zsh: %w", err)
	}
	cmd := exec.CommandContext(ctx, zsh, "-n")
	cmd.Stdin = source
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return context.DeadlineExceeded
		}
		return errors.New("zsh source failed validation")
	}
	return nil
}

func runtimeFail(stderr io.Writer, code int, message string) int {
	_, _ = fmt.Fprintf(stderr, "zsh-pro: %s\n", message)
	return code
}
