package store

// errStore is the base type for every store error (D-11). All store errors are
// zsh-pro-phrased typed sentinels — raw git/keychain stderr is never surfaced to
// the user. The pattern mirrors core/model/exitcode.go's typed-constant style; the
// Err* naming prefix matches the project's Cat*/Kind*/Exit* convention.
type errStore string

// Error implements the error interface, returning the zsh-pro-phrased message.
func (e errStore) Error() string { return string(e) }

const (
	// ErrGitAbsent is returned when the git binary is not on $PATH. The store
	// requires git; absence degrades to this clear error, never a crash (D-06).
	ErrGitAbsent errStore = "zsh-pro: git is not installed; profile storage requires git"
	// ErrNotInitialized is returned when the store repo has not been initialized.
	ErrNotInitialized errStore = "zsh-pro: profile store not initialized"
	// ErrProfileNotFound is returned when a requested profile (branch) does not exist.
	ErrProfileNotFound errStore = "zsh-pro: profile not found"
	// ErrProfileExists is returned when creating a profile whose name already exists.
	ErrProfileExists errStore = "zsh-pro: profile already exists"
	// ErrInvalidProfileName is returned when a profile/branch name is empty, starts
	// with '-' (argument-injection guard), contains a rune outside [A-Za-z0-9._/-],
	// or has a '..'/'.' path component (path-traversal guard) — threat T-03-02.
	ErrInvalidProfileName errStore = "zsh-pro: invalid profile name"
	// ErrSecretBackendUnavailable is returned when no OS keychain backend is present;
	// the store falls back to the git-ignored vault file.
	ErrSecretBackendUnavailable errStore = "zsh-pro: no secret backend available; using vault file"
	// ErrSecretNotFound distinguishes an absent key from an operational backend
	// failure, including when a present key stores the empty string.
	ErrSecretNotFound errStore = "zsh-pro: secret not found"
	// ErrKeychainTransport distinguishes an unreadable legacy or unknown stored
	// value from a keychain backend that could not be reached.
	ErrKeychainTransport errStore = "zsh-pro: unrecognized keychain secret transport"
	ErrSecretRollback    errStore = "zsh-pro: secret rollback failed"
	ErrSecretRefConflict errStore = "zsh-pro: profile ref changed concurrently"
	// ErrUnsafeSecretShape is returned when a CatSecrets assignment cannot be safely
	// excluded because its shape does not faithfully model a single secret segment.
	// The parser collapses a multi-name assignment (`export A=$HOME B=secret`) into
	// ONE entry whose Value is only the last segment and whose Dynamic flag reflects
	// ANY segment, so the literal can hide in Text under the wrong name; an array
	// secret (`export ARR=(sk-one sk-two)`) likewise carries its literal only in Text
	// with an empty Value. Neither shape can be excluded at Entry granularity without
	// risking a leak, so Commit FAILS CLOSED here (T-03-03) rather than committing the
	// literal verbatim — the user must split the secret into its own NAME=value
	// statement. This is reserved for genuinely-unsafe shapes: a single-name scalar
	// literal is still excluded, and an already-dynamic single-name secret still
	// commits verbatim (D-08).
	ErrUnsafeSecretShape errStore = "zsh-pro: cannot safely store a secret in a multi-name or non-scalar assignment; split it into its own statement"
	// ErrGitCommand is the generic mapped failure for any git plumbing error. The
	// raw git stderr is intentionally NOT embedded (D-11; Pitfall 4).
	ErrGitCommand errStore = "zsh-pro: git operation failed"
)
