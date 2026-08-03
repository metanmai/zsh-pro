// Command zsh-pro is the SOLE composition root: the only non-test package that may
// import a concrete shell implementation (core/shell/zsh) and the concrete store
// drivers. Every other package depends on interfaces (shell.Provider,
// shell.Regenerator, store.KeychainDriver), which are wired together here.
package main

import (
	"context"
	"errors"
	"os"

	"zsh-pro/core/cli"
	"zsh-pro/core/shell/zsh"
	"zsh-pro/core/store"
)

var errCompositionStoreUnavailable = errors.New("profile store unavailable")

// storeInitializerFor binds the initializer authority and every follow-on
// ingest operation to one concrete Store. A construction failure is captured
// once and replayed verbatim; the initializer never attempts replacement
// construction after CLI dispatch.
func storeInitializerFor(cliStore *store.Store, canonicalRoot string, constructionErr error) cli.StoreInitializer {
	return func(ctx context.Context) (cli.StoreInitialization, error) {
		if constructionErr != nil {
			return cli.StoreInitialization{}, constructionErr
		}
		if cliStore == nil {
			return cli.StoreInitialization{}, errCompositionStoreUnavailable
		}
		transaction, err := cliStore.InitForInstall(ctx)
		if err != nil {
			return cli.StoreInitialization{}, err
		}
		return cli.StoreInitialization{
			InitializationID: transaction.ID(),
			Transactions:     cliStore,
			CanonicalRoot:    canonicalRoot,
			Rollback:         transaction.Rollback,
			Finalize:         transaction.Finalize,
			CreatedPath:      transaction.CreatedPath(),
		}, nil
	}
}

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
	var cliStore *store.Store
	var storeRoot string
	var storeErr error
	var kc store.KeychainDriver
	storeRoot, storeErr = cli.StoreRoot()
	if storeErr == nil {
		kc = store.NewOSKeychainDriver(storeRoot)
		cliStore, storeErr = store.New(storeRoot, provider, kc)
	}
	var commandStore cli.Store
	if storeErr == nil {
		commandStore = cliStore
	}
	// Runtime secret dereference remains behind the narrow CLI resolver seam;
	// the existing concrete driver is created once here and never exposes a
	// resolved value to CLI logging or profile persistence.
	emitter := cli.NewRuntimeEmitterWithRuntimeStore(commandStore, provider, kc, func(root *cli.RuntimeRoot) (cli.Store, cli.SecretResolver, error) {
		repository, vaultParent := root.Files()
		boundStore, err := store.NewRuntime(repository, vaultParent, provider)
		if err != nil {
			return nil, nil, err
		}
		return boundStore, boundStore.RuntimeSecretResolver(), nil
	})

	return cli.NewWithStoreInitializer(provider, commandStore, emitter, storeInitializerFor(cliStore, storeRoot, storeErr))
}
