// Package render turns an Analysis into human or JSON output. Both are pure
// functions of the model; neither mutates it.
package render

import (
	"encoding/json"
	"fmt"
	"strings"

	"zsh-pro/core/buildinfo"
	"zsh-pro/core/model"
)

// Human returns the terminal-friendly report.
func Human(a model.Analysis) string {
	var b strings.Builder
	fmt.Fprintf(&b, "\n  zsh-pro — analysis of %s\n", a.Path)
	fmt.Fprintf(&b, "  %d lines, %d blocks", a.Lines, a.BlockCount)
	if a.OpaqueBlocks > 0 {
		fmt.Fprintf(&b, " (%d not structurally understood)", a.OpaqueBlocks)
	}
	b.WriteString("\n\n  CATEGORIES\n")
	for _, c := range a.Categories {
		fmt.Fprintf(&b, "   - %-12s %3d  %s\n", c.Category, c.Count, model.CategoryDescription(c.Category))
	}
	if a.HasSecrets {
		b.WriteString("\n  !  Secrets detected — keep these in a .gitignore'd file; never sync them.\n")
	}
	if !a.Introspected {
		for _, n := range a.Notes {
			fmt.Fprintf(&b, "\n  !  %s\n", n)
		}
	}

	b.WriteString("\n  ISSUES\n")
	if len(a.Issues) == 0 {
		b.WriteString("   (none found — nice and clean)\n")
		return b.String()
	}
	for _, is := range a.Issues {
		line := fmt.Sprintf("   ! %-16s %s", is.Kind, is.Name)
		if len(is.Lines) > 0 {
			line += fmt.Sprintf("  at lines %v", is.Lines)
		}
		if is.Note != "" {
			line += "  (" + is.Note + ")"
		}
		b.WriteString(line + "\n")
	}
	return b.String()
}

// envelope mirrors the prototype's machine-readable contract.
type envelope struct {
	Tool        string         `json:"tool"`
	Version     string         `json:"version"`
	Command     string         `json:"command"`
	OK          bool           `json:"ok"`
	IssuesFound bool           `json:"issues_found"`
	ExitCode    int            `json:"exit_code"`
	Analysis    model.Analysis `json:"analysis"`
}

// JSON returns exactly one JSON object (the agent contract).
func JSON(a model.Analysis) ([]byte, error) {
	env := envelope{
		Tool:        "zsh-pro",
		Version:     buildinfo.Version,
		Command:     "analyze",
		OK:          true,
		IssuesFound: len(a.Issues) > 0,
		ExitCode:    a.ExitCode(),
		Analysis:    a,
	}
	return json.MarshalIndent(env, "", "  ")
}
