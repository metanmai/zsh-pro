// Command zsh-pro is the SOLE composition root: the only non-test package that may
// import a concrete shell implementation (core/shell/zsh) and the concrete store
// drivers. Every other package depends on interfaces (shell.Provider,
// shell.Regenerator, store.KeychainDriver), which are wired together here.
package main

import (
	"os"
	"path/filepath"

	"zsh-pro/core/cli"
	"zsh-pro/core/shell/zsh"
	"zsh-pro/core/store"
)

func main() {
	// The concrete zsh provider is the engine's Parser/Classifier/Introspector AND the
	// shell.Regenerator the store uses to derive profile.zsh (D-03).
	// It also supplies the shell.Emitter seam for Phase 5's runtime loader.
	provider := zsh.Provider{}

	// Construct + wire the git-backed store: zsh.Provider{} as the Regenerator and a
	// runtime-selected keychain driver (security on macOS / secret-tool on Linux /
	// git-ignored 0600 vault fallback — never nil) as the secret backend, over the D-04
	// store dir. The store package itself never imports core/shell/zsh; the concrete
	// drivers are injected only here.
	dir := storeDir()
	kc := store.NewOSKeychainDriver(dir)
	s, err := store.New(dir, provider, kc)
	// The CLI verbs that consume the store (checkout/create/list/status) arrive in
	// Phase 5; the store is constructed + injectable now but not yet driven by a verb.
	// An init error (e.g. git absent) simply means profile storage is unavailable — it
	// must NOT crash the existing read-only `analyze` path, so it is intentionally
	// non-fatal here and surfaces when a store-backed verb is wired in Phase 5.
	var cliStore cli.Store
	if err == nil {
		cliStore = s
	}
	// Runtime secret dereference remains behind the narrow CLI resolver seam;
	// the existing concrete driver is created once here and never exposes a
	// resolved value to CLI logging or profile persistence.
	emitter := cli.NewRuntimeEmitter(cliStore, provider, kc)

	os.Exit(cli.New(provider, cliStore, emitter).Run(os.Args[1:], os.Stdout, os.Stderr))
}

// storeDir resolves the bare-repo location per D-04: $ZSHPRO_HOME if set, else
// $XDG_DATA_HOME/zsh-pro, else ~/.local/share/zsh-pro. It is a pure env read — no
// directory is created here (Init owns creation, idempotently).
func storeDir() string {
	if home := os.Getenv("ZSHPRO_HOME"); home != "" {
		return home
	}
	if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
		return filepath.Join(xdg, "zsh-pro")
	}
	return filepath.Join(os.Getenv("HOME"), ".local", "share", "zsh-pro")
}
