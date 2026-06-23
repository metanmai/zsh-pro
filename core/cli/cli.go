// Package cli wires flags and I/O to the engine. Run returns an exit code and
// writes to the provided streams, so it is testable without os.Exit.
//
// This is the composition root: it is the only package permitted to import the
// concrete shell implementation (core/shell/zsh). Everything below the engine
// depends on the core/shell interfaces, not this wiring.
package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"zsh-pro/core/analyze"
	"zsh-pro/core/buildinfo"
	"zsh-pro/core/render"
	"zsh-pro/core/shell/zsh"
	"zsh-pro/core/util"
)

// Run executes a command. Exit codes: 0 clean, 1 runtime error, 2 usage,
// 3 actionable.
func Run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: zsh-pro analyze [path] [--json]")
		return 2
	}
	switch args[0] {
	case "--version", "-v":
		fmt.Fprintf(stdout, "zsh-pro %s\n", buildinfo.Version)
		return 0
	case "analyze":
		return runAnalyze(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "zsh-pro: unknown command %q\n", args[0])
		return 2
	}
}

func runAnalyze(args []string, stdout, stderr io.Writer) int {
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
		return fail(stdout, stderr, asJSON, fmt.Sprintf("cannot read %s: %v", path, err))
	}

	a := analyze.New(zsh.Provider{}).Analyze(src, path)

	if asJSON {
		b, err := (render.JSONRenderer{}).Render(a)
		if err != nil {
			return fail(stdout, stderr, true, fmt.Sprintf("render: %v", err))
		}
		fmt.Fprintln(stdout, string(b))
	} else {
		b, err := (render.HumanRenderer{}).Render(a)
		if err != nil {
			return fail(stdout, stderr, false, fmt.Sprintf("render: %v", err))
		}
		fmt.Fprintln(stdout, string(b))
	}
	return a.ExitCode()
}

// fail emits a runtime error (exit 1) in either mode. The agent contract
// requires --json to emit exactly one JSON object on STDOUT on success OR
// failure, so the structured error envelope goes to stdout; human mode keeps
// the readable error line on stderr.
func fail(stdout, stderr io.Writer, asJSON bool, msg string) int {
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
