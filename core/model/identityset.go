package model

// IdentitySet is the resolved end-state captured by dynamic introspection.
type IdentitySet struct {
	Aliases   map[string]bool // alias names present after sourcing
	Functions map[string]bool // function names present after sourcing
	Env       map[string]bool // exported variable names present after sourcing
	Path      []string        // resolved $path entries, in order
	Options   map[string]bool // shell options that are "on"
	Available bool            // false when introspection failed (static-only)
}
