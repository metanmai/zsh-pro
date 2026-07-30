// Command zsh-pro is the SOLE composition root: the only non-test package that may
// import a concrete shell implementation (core/shell/zsh) and the concrete store
// drivers. Every other package depends on interfaces (shell.Provider,
// shell.Regenerator, store.KeychainDriver), which are wired together here.
package main

import (
	"os"

	"zsh-pro/core/cli"
	"zsh-pro/core/shell/zsh"
	"zsh-pro/core/store"
)

func main() {
	os.Exit(newCLI().Run(os.Args[1:], os.Stdout, os.Stderr))
}

func newCLI() *cli.CLI {
	// The concrete zsh provider is the engine's Parser/Classifier/Introspector AND the
	// shell.Regenerator the store uses to derive profile.zsh (D-03).
	// It also supplies the shell.Emitter seam for Phase 5's runtime loader.
	provider := zsh.Provider{}

	// Construct + wire the git-backed store: zsh.Provider{} as the Regenerator and a
	// runtime-selected keychain driver (security on macOS / secret-tool on Linux /
	// git-ignored 0600 vault fallback — never nil) as the secret backend, over the D-04
	// store dir. The store package itself never imports core/shell/zsh; the concrete
	// drivers are injected only here.
	// The CLI verbs that consume the store (checkout/create/list/status) arrive in
	// Phase 5; the store is constructed + injectable now but not yet driven by a verb.
	// A store-root or init error (e.g. an invalid environment or absent git) simply
	// means profile storage is unavailable — it must NOT crash the existing read-only
	// `analyze` path, so it is intentionally non-fatal here and surfaces when a
	// store-backed verb is wired in Phase 5.
	var cliStore cli.Store
	var kc store.KeychainDriver
	if dir, err := cli.StoreRoot(); err == nil {
		kc = store.NewOSKeychainDriver(dir)
		if s, err := store.New(dir, provider, kc); err == nil {
			cliStore = s
		}
	}
	// Runtime secret dereference remains behind the narrow CLI resolver seam;
	// the existing concrete driver is created once here and never exposes a
	// resolved value to CLI logging or profile persistence.
	emitter := cli.NewRuntimeEmitterWithRuntimeStore(cliStore, provider, kc, func(root *cli.RuntimeRoot) (cli.Store, cli.SecretResolver, error) {
		repository, vaultParent := root.Files()
		boundStore, err := store.NewRuntime(repository, vaultParent, provider)
		if err != nil {
			return nil, nil, err
		}
		return boundStore, boundStore.RuntimeSecretResolver(), nil
	})

	return cli.New(provider, cliStore, emitter)
}
