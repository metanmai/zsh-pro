// Package testgen builds random zsh configs from a typed dependency graph,
// together with the known-correct analysis they should produce. It is test
// infrastructure: it imports only core/model (a leaf) and never the engine.
package testgen

import "zsh-pro/core/model"

// NodeKind is the shell-entity type of a graph node.
type NodeKind int

const (
	NodeEnvVar NodeKind = iota
	NodeAlias
	NodeFunction
	NodePathEntry
	NodeCommand
	NodeSecret
)

// Node is one shell entity. Identity for duplicate/shadow detection is Name
// (for PathEntry, Name is the rooted directory; for Command, the leading word).
type Node struct {
	Kind      NodeKind
	Name      string
	Value     string         // RHS / body / args, Kind-dependent
	Comment   string         // optional leading comment (no leading '#')
	Cat       model.Category // intended classification — the oracle's source of truth
	DependsOn []int          // indices of nodes that must be emitted before this one
	Line      int            // 1-based statement line; filled by RenderZsh
}

// ConfigGraph is a generated config. Insertion order is a valid topological
// order: generators only ever record DependsOn edges to lower-index nodes, so
// no separate topological sort is needed when rendering.
type ConfigGraph struct {
	Nodes []*Node
}

// Add appends a node and returns its index.
func (g *ConfigGraph) Add(n *Node) int {
	g.Nodes = append(g.Nodes, n)
	return len(g.Nodes) - 1
}

// DependOn records that the child node must be emitted after the parent.
// Callers pass parent < child to preserve the topological-order invariant.
func (g *ConfigGraph) DependOn(child, parent int) {
	g.Nodes[child].DependsOn = append(g.Nodes[child].DependsOn, parent)
}
