package cli

import (
	"errors"
	"os"
)

// RuntimeRoot is the authenticated directory pair used only while the runtime
// helper emits a profile. RepositoryFile is the final $ZSHPRO_HOME directory;
// VaultParentFile is its parent, which anchors the file-backed vault sibling.
// Callers may pass the files to a trusted composition-root factory but must not
// close them: the RuntimeRoot owns their lifetime.
type RuntimeRoot struct {
	repository  *os.File
	vaultParent *os.File
}

// Files exposes the authenticated repository and vault-parent descriptors to
// the composition root. They remain valid until Close is called.
func (r *RuntimeRoot) Files() (repository, vaultParent *os.File) {
	if r == nil {
		return nil, nil
	}
	return r.repository, r.vaultParent
}

// Close releases both authenticated descriptors. It is safe to call more than
// once, and it reports the first close error while attempting both closes.
func (r *RuntimeRoot) Close() error {
	if r == nil {
		return nil
	}
	var first error
	if r.repository != nil {
		if err := r.repository.Close(); err != nil && !errors.Is(err, os.ErrClosed) {
			first = err
		}
		r.repository = nil
	}
	if r.vaultParent != nil {
		if err := r.vaultParent.Close(); err != nil && !errors.Is(err, os.ErrClosed) && first == nil {
			first = err
		}
		r.vaultParent = nil
	}
	return first
}
