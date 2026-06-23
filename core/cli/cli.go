// Package cli wires flags and I/O to the engine. A CLI is bound to a shell
// Provider; Run returns an exit code and writes to the provided streams, so it
// is testable without os.Exit.
//
// cli depends on the core/shell interface, not the concrete implementation:
// the composition root (cmd/zsh-pro) is the only package that imports
// core/shell/zsh and injects it here via New.
package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"zsh-pro/core/analyze"
	"zsh-pro/core/buildinfo"
	"zsh-pro/core/render"
	"zsh-pro/core/shell"
	"zsh-pro/core/util"
)

// CLI wires flags and I/O to the engine for a given shell Provider.
type CLI struct{ provider shell.Provider }

// New returns a CLI bound to a Provider.
func New(p shell.Provider) *CLI { return &CLI{provider: p} }

// Run executes a command. Exit codes: 0 clean, 1 runtime error, 2 usage,
// 3 actionable.
func (c *CLI) Run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: zsh-pro analyze [path] [--json]")
		return 2
	}
	switch args[0] {
	case "--version", "-v":
		fmt.Fprintf(stdout, "zsh-pro %s\n", buildinfo.Version)
		return 0
	case "analyze":
		return c.runAnalyze(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "zsh-pro: unknown command %q\n", args[0])
		return 2
	}
}

func (c *CLI) runAnalyze(args []string, stdout, stderr io.Writer) int {
	path := "~/.zshrc"
	asJSON := false
	for _, a := range args {
		switch {
		case a == "--json":
			asJSON = true
		case len(a) > 0 && a[0] == '-':
			fmt.Fprintf(stderr, "zsh-pro: unknown flag %q\n", a)
			return 2
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
	fmt.Fprintln(stdout, string(b))
	return a.ExitCode()
}

// fail emits a runtime error (exit 1) in either mode. The agent contract
// requires --json to emit exactly one JSON object on STDOUT on success OR
// failure, so the structured error envelope goes to stdout; human mode keeps
// the readable error line on stderr.
func (c *CLI) fail(stdout, stderr io.Writer, asJSON bool, msg string) int {
	if asJSON {
		obj := map[string]any{
			"tool": "zsh-pro", "version": buildinfo.Version, "command": "analyze",
			"ok": false, "error": msg, "exit_code": 1,
		}
		b, _ := json.MarshalIndent(obj, "", "  ")
		fmt.Fprintln(stdout, string(b))
	} else {
		fmt.Fprintf(stderr, "zsh-pro: %s\n", msg)
	}
	return 1
}
