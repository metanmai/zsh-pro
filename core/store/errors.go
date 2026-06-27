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
	// ErrSecretBackendUnavailable is returned when no OS keychain backend is present;
	// the store falls back to the git-ignored vault file.
	ErrSecretBackendUnavailable errStore = "zsh-pro: no secret backend available; using vault file"
	// ErrGitCommand is the generic mapped failure for any git plumbing error. The
	// raw git stderr is intentionally NOT embedded (D-11; Pitfall 4).
	ErrGitCommand errStore = "zsh-pro: git operation failed"
)
