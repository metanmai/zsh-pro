// Package cli wires flags and I/O to the engine. A CLI is bound to a shell
// Provider; Run returns an exit code and writes to the provided streams, so it
// is testable without os.Exit.
//
// cli depends on the core/shell interface, not the concrete implementation:
// the composition root (cmd/zsh-pro) is the only package that imports
// core/shell/zsh and injects it here via New.
package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"zsh-pro/core/analyze"
	"zsh-pro/core/buildinfo"
	"zsh-pro/core/model"
	"zsh-pro/core/render"
	"zsh-pro/core/shell"
	"zsh-pro/core/util"
)

// CLI wires flags and I/O to the engine for a given shell Provider.
type CLI struct {
	provider shell.Provider
	store    Store
	emitter  Emitter
}

// New returns a CLI bound to a Provider.
func New(p shell.Provider, s Store, e Emitter) *CLI {
	return &CLI{provider: p, store: s, emitter: e}
}

// Run executes a command. Exit codes: 0 clean, 1 runtime error, 2 usage,
// 3 actionable.
func (c *CLI) Run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		_, _ = fmt.Fprintln(stderr, "usage: zsh-pro analyze [path] [--json]")
		return int(model.ExitUsageErr)
	}
	switch args[0] {
	case "--version", "-v":
		_, _ = fmt.Fprintf(stdout, "zsh-pro %s\n", buildinfo.Version)
		return int(model.ExitClean)
	case buildinfo.Command:
		return c.runAnalyze(args[1:], stdout, stderr)
	case "hook":
		_, _ = fmt.Fprint(stdout, c.provider.HookScript())
		return int(model.ExitClean)
	case "install":
		return c.runInstall(stdout, stderr)
	case "list":
		return c.runList(stdout, stderr)
	case "status":
		return c.runStatus(stdout, stderr)
	case "emit":
		return c.runEmit(args[1:], stdout, stderr)
	default:
		_, _ = fmt.Fprintf(stderr, "zsh-pro: unknown command %q\n", args[0])
		return int(model.ExitUsageErr)
	}
}

func (c *CLI) runList(stdout, stderr io.Writer) int {
	if c.store == nil {
		return c.fail(stdout, stderr, false, "profile store unavailable")
	}
	branches, err := c.store.Branches(context.Background())
	if err != nil {
		return c.fail(stdout, stderr, false, fmt.Sprintf("list profiles: %v", err))
	}
	for _, branch := range branches {
		_, _ = fmt.Fprintln(stdout, branch)
	}
	return int(model.ExitClean)
}

func (c *CLI) runStatus(stdout, stderr io.Writer) int {
	if c.store == nil {
		return c.fail(stdout, stderr, false, "profile store unavailable")
	}
	current := c.store.Current()
	if current == "" {
		current = "main"
	}
	_, _ = fmt.Fprintln(stdout, current)
	return int(model.ExitClean)
}

func (c *CLI) runEmit(args []string, stdout, stderr io.Writer) int {
	if len(args) != 2 || (args[0] != "apply" && args[0] != "deactivate") || args[1] == "" {
		_, _ = fmt.Fprintln(stderr, "usage: zsh-pro emit <apply|deactivate> <profile>")
		return int(model.ExitUsageErr)
	}
	if c.emitter == nil {
		return c.fail(stdout, stderr, false, "emit path not yet available")
	}
	source, err := c.emitter.Emit(context.Background(), args[0], args[1])
	if err != nil {
		return c.fail(stdout, stderr, false, err.Error())
	}
	if source == "" {
		return c.fail(stdout, stderr, false, "emit produced empty output")
	}
	_, _ = fmt.Fprintln(stdout, source)
	return int(model.ExitClean)
}

func (c *CLI) runAnalyze(args []string, stdout, stderr io.Writer) int {
	path := "~/.zshrc"
	asJSON := false
	for _, a := range args {
		switch {
		case a == "--json":
			asJSON = true
		case len(a) > 0 && a[0] == '-':
			_, _ = fmt.Fprintf(stderr, "zsh-pro: unknown flag %q\n", a)
			return int(model.ExitUsageErr)
		default:
			path = a
		}
	}
	path = util.ExpandHome(path)

	src, err := os.ReadFile(path)
	if err != nil {
		return c.fail(stdout, stderr, asJSON, fmt.Sprintf("cannot read %s: %v", path, err))
	}

	a := analyze.New(c.provider).Analyze(src, path)

	var r render.Renderer = render.HumanRenderer{}
	if asJSON {
		r = render.JSONRenderer{}
	}
	b, err := r.Render(a)
	if err != nil {
		return c.fail(stdout, stderr, asJSON, fmt.Sprintf("render: %v", err))
	}
	_, _ = fmt.Fprintln(stdout, string(b))
	return int(a.ExitCode())
}

// fail emits a runtime error (exit 1) in either mode. The agent contract
// requires --json to emit exactly one JSON object on STDOUT on success OR
// failure, so the structured error envelope goes to stdout; human mode keeps
// the readable error line on stderr.
func (c *CLI) fail(stdout, stderr io.Writer, asJSON bool, msg string) int {
	if asJSON {
		obj := map[string]any{
			"tool": buildinfo.Name, "version": buildinfo.Version, "command": buildinfo.Command,
			"ok": false, "error": msg, "exit_code": int(model.ExitRuntimeErr),
		}
		b, _ := json.MarshalIndent(obj, "", "  ")
		_, _ = fmt.Fprintln(stdout, string(b))
	} else {
		_, _ = fmt.Fprintf(stderr, "zsh-pro: %s\n", msg)
	}
	return int(model.ExitRuntimeErr)
}
