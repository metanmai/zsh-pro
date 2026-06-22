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
	"path/filepath"

	"zsh-pro/core/analyze"
	"zsh-pro/core/buildinfo"
	"zsh-pro/core/render"
	"zsh-pro/core/shell/zsh"
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
	path = expandHome(path)

	src, err := os.ReadFile(path)
	if err != nil {
		return fail(stderr, asJSON, fmt.Sprintf("cannot read %s: %v", path, err))
	}

	a := analyze.Analyze(zsh.Provider{}, src, path)

	if asJSON {
		b, err := render.JSON(a)
		if err != nil {
			return fail(stderr, true, fmt.Sprintf("render: %v", err))
		}
		fmt.Fprintln(stdout, string(b))
	} else {
		fmt.Fprintln(stdout, render.Human(a))
	}
	return a.ExitCode()
}

func expandHome(p string) string {
	if p == "~" || (len(p) >= 2 && p[:2] == "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, p[1:])
		}
	}
	return p
}

// fail emits a runtime error (exit 1) in either mode.
func fail(stderr io.Writer, asJSON bool, msg string) int {
	if asJSON {
		obj := map[string]any{
			"tool": "zsh-pro", "version": buildinfo.Version, "command": "analyze",
			"ok": false, "error": msg, "exit_code": 1,
		}
		b, _ := json.MarshalIndent(obj, "", "  ")
		fmt.Fprintln(stderr, string(b))
	} else {
		fmt.Fprintf(stderr, "zsh-pro: %s\n", msg)
	}
	return 1
}
