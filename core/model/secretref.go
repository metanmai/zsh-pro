package model

// SecretRefKind identifies the resolver backend a SecretRef points into (D-09).
// It is a resolver-agnostic tag: the store never reads the literal value, it only
// records which backend holds it so a later phase (Ph4/5) can dereference by name.
type SecretRefKind string

const (
	// SecretRefKeychain resolves via the OS keychain subprocess (the default
	// backend: `security` on macOS / `secret-tool` on Linux). No plaintext on disk.
	SecretRefKeychain SecretRefKind = "keychain"
	// SecretRefFile resolves via the git-ignored vault file fallback, used when no
	// keychain backend is available. The file is never committed (.gitignore'd).
	SecretRefFile SecretRefKind = "file"
	// SecretRefCmd resolves via an arbitrary retrieval command, reserved for future
	// secret managers (e.g. `op read ...`); not produced by Phase 3.
	SecretRefCmd SecretRefKind = "cmd"
)

// SecretRef is a resolver-agnostic pointer to a captured secret value (D-07/D-09).
// It serializes as {"kind":"keychain","key":"API_KEY"} in profile.json: the
// literal value never enters the git tree — only the reference does. The Key is
// name-scoped (global-by-name, RESEARCH Open Question 2): exactly one stored entry
// per secret name, dereferenced by name in Ph4/5.
type SecretRef struct {
	Kind SecretRefKind `json:"kind"` // resolver backend that holds the value
	Key  string        `json:"key"`  // name-scoped lookup key (e.g. "API_KEY")
}
