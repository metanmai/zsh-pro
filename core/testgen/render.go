package testgen

import (
	"fmt"
	"strings"
)

// RenderZsh emits the graph as .zsh source in node (topological) order,
// emitting each node's optional leading comment, then its statement, then a
// blank separator line. It sets each Node.Line to the 1-based line of that
// node's statement. Call RenderZsh before Expected (Expected reads Node.Line).
func (g *ConfigGraph) RenderZsh() []byte {
	var b strings.Builder
	line := 1
	for _, n := range g.Nodes {
		if n.Comment != "" {
			fmt.Fprintf(&b, "# %s\n", n.Comment)
			line++
		}
		n.Line = line
		stmt := n.render()
		b.WriteString(stmt)
		b.WriteByte('\n')
		line += strings.Count(stmt, "\n") + 1
		b.WriteByte('\n') // blank separator
		line++
	}
	return []byte(b.String())
}

// render returns the zsh statement text for a node (no trailing newline).
func (n *Node) render() string {
	switch n.Kind {
	case NodeEnvVar, NodeSecret:
		return fmt.Sprintf("export %s=%q", n.Name, n.Value)
	case NodeAlias:
		return fmt.Sprintf("alias %s='%s'", n.Name, n.Value)
	case NodeFunction:
		return fmt.Sprintf("%s() {\n  %s\n}", n.Name, n.Value)
	case NodePathEntry:
		return fmt.Sprintf("export PATH=%q", n.Name+":$PATH")
	case NodeCommand:
		return strings.TrimSpace(n.Name + " " + n.Value)
	default:
		return "# unknown node"
	}
}
