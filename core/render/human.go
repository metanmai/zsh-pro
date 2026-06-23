package render

import (
	"fmt"
	"strings"

	"zsh-pro/core/model"
)

// HumanRenderer produces the terminal-friendly report.
type HumanRenderer struct{}

// Render returns the terminal-friendly report. It never errors.
func (HumanRenderer) Render(a model.Analysis) ([]byte, error) {
	var b strings.Builder
	fmt.Fprintf(&b, "\n  zsh-pro — analysis of %s\n", a.Path)
	fmt.Fprintf(&b, "  %d lines, %d blocks", a.Lines, a.BlockCount)
	if a.OpaqueBlocks > 0 {
		fmt.Fprintf(&b, " (%d not structurally understood)", a.OpaqueBlocks)
	}
	b.WriteString("\n\n  CATEGORIES\n")
	for _, c := range a.Categories {
		fmt.Fprintf(&b, "   - %-12s %3d  %s\n", c.Category, c.Count, c.Category.Description())
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
		return []byte(b.String()), nil
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
	return []byte(b.String()), nil
}
