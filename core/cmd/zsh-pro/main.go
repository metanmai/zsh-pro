// Command zsh-pro is the SOLE composition root: the only non-test package that may
// import a concrete shell implementation (core/shell/zsh) and the concrete store
// drivers. Every other package depends on interfaces (shell.Provider,
// shell.Regenerator, store.KeychainDriver), which are wired together here.
package main

import (
	"context"
	"errors"
	"os"
	"sync"

	"zsh-pro/core/cli"
	"zsh-pro/core/model"
	"zsh-pro/core/shell/zsh"
	"zsh-pro/core/store"
	"zsh-pro/core/worktree"
)

var errCompositionStoreUnavailable = errors.New("profile store unavailable")

type materializingIngestTransactions struct {
	cli.IngestTransactionStore
	materializer cli.WorktreeMaterializer
}

func (transactions materializingIngestTransactions) MaterializeCommittedWorktree(ctx context.Context, branch, revision string) error {
	return transactions.materializer.MaterializeCommittedWorktree(ctx, branch, revision)
}

// compositionWorktree retains one path-bound public service over the canonical
// store root. Construction is lazy so analyze remains read-only and available
// when profile storage has not been initialized. The runtime factory retains
// only the configuration needed by the later authenticated-descriptor binding;
// it never receives or reopens this pathname.
type compositionWorktree struct {
	mu              sync.Mutex
	canonicalRoot   string
	repository      *store.Store
	registry        *worktree.Registry
	runtimeFactory  cli.RuntimeWorktreeFactory
	constructionErr error
	state           *worktree.StateStore
	service         *worktree.Service
}

func newCompositionWorktree(
	canonicalRoot string,
	repository *store.Store,
	provider zsh.Provider,
	runtimeFactory cli.RuntimeWorktreeFactory,
	constructionErr error,
) *compositionWorktree {
	return &compositionWorktree{
		canonicalRoot:   canonicalRoot,
		repository:      repository,
		registry:        worktree.NewRegistry(provider),
		runtimeFactory:  runtimeFactory,
		constructionErr: constructionErr,
	}
}

func (authority *compositionWorktree) bind() (*worktree.Service, *worktree.StateStore, error) {
	if authority == nil {
		return nil, nil, errCompositionStoreUnavailable
	}
	authority.mu.Lock()
	defer authority.mu.Unlock()
	if authority.constructionErr != nil {
		return nil, nil, authority.constructionErr
	}
	if authority.repository == nil || authority.registry == nil || authority.canonicalRoot == "" {
		return nil, nil, errCompositionStoreUnavailable
	}
	if authority.service != nil && authority.state != nil {
		return authority.service, authority.state, nil
	}
	state, err := worktree.OpenStateStore(authority.canonicalRoot)
	if err != nil {
		return nil, nil, err
	}
	service, err := worktree.NewService(state, authority.registry, authority.repository)
	if err != nil {
		_ = state.Close()
		return nil, nil, err
	}
	authority.state = state
	authority.service = service
	return service, state, nil
}

func (authority *compositionWorktree) MaterializeCommittedWorktree(ctx context.Context, branch, revision string) error {
	service, _, err := authority.bind()
	if err != nil {
		return err
	}
	committed, err := authority.repository.ReadWorktreeRevision(ctx, revision)
	if err != nil {
		return err
	}
	if err := service.Materialize(ctx, branch, revision, committed); err != nil {
		if !errors.Is(err, worktree.ErrRecoveryRequired) {
			return err
		}
		// An earlier exact generation may already be durable. Let Service's
		// under-lock split-recovery path inspect the current branch OID, read that
		// exact committed DTO, and advance only when its projection equals shared.
		status, repairErr := service.WorkflowStatus(ctx, "")
		if repairErr != nil || status.Worktree.Branch != branch || status.Worktree.BaseOID != revision {
			return err
		}
	}
	return nil
}

func (authority *compositionWorktree) ensureMaterialized(ctx context.Context) (*worktree.Service, error) {
	service, stateStore, err := authority.bind()
	if err != nil {
		return nil, err
	}
	state, err := stateStore.Read(ctx)
	if err != nil {
		return nil, err
	}
	if state.Materialized {
		return service, nil
	}
	revision, err := authority.repository.ResolveWorktreeRevision(ctx, "main")
	if err != nil {
		return nil, err
	}
	committed, err := authority.repository.ReadWorktreeRevision(ctx, revision)
	if err != nil {
		return nil, err
	}
	if err := service.Materialize(ctx, "main", revision, committed); err != nil {
		return nil, err
	}
	return service, nil
}

func (authority *compositionWorktree) WorkflowStatus(ctx context.Context, shellID string) (worktree.WorkflowStatus, error) {
	service, err := authority.ensureMaterialized(ctx)
	if err != nil {
		return worktree.WorkflowStatus{}, err
	}
	return service.WorkflowStatus(ctx, shellID)
}

func (authority *compositionWorktree) Diff(ctx context.Context) (model.CategorizedDiff, error) {
	service, err := authority.ensureMaterialized(ctx)
	if err != nil {
		return model.CategorizedDiff{}, err
	}
	return service.Diff(ctx)
}

func (authority *compositionWorktree) Branches(ctx context.Context) ([]string, error) {
	service, err := authority.ensureMaterialized(ctx)
	if err != nil {
		return nil, err
	}
	return service.Branches(ctx)
}

func (authority *compositionWorktree) Commit(ctx context.Context, message string) (model.WorktreeCommitResult, error) {
	service, _, err := authority.bind()
	if err != nil {
		return model.WorktreeCommitResult{}, err
	}
	return service.Commit(ctx, message)
}

func (authority *compositionWorktree) Branch(ctx context.Context, branch string) error {
	service, _, err := authority.bind()
	if err != nil {
		return err
	}
	return service.Branch(ctx, branch)
}

func (authority *compositionWorktree) Checkout(ctx context.Context, branch string, create bool) error {
	service, _, err := authority.bind()
	if err != nil {
		return err
	}
	return service.Checkout(ctx, branch, create)
}

func (authority *compositionWorktree) ResetHard(ctx context.Context) error {
	service, _, err := authority.bind()
	if err != nil {
		return err
	}
	return service.ResetHard(ctx)
}

func (authority *compositionWorktree) SetAutoApplyDefault(ctx context.Context, enabled bool) error {
	service, _, err := authority.bind()
	if err != nil {
		return err
	}
	return service.SetAutoApplyDefault(ctx, enabled)
}

var (
	_ worktree.Repository      = (*store.Store)(nil)
	_ cli.WorktreeMaterializer = (*compositionWorktree)(nil)
	_ cli.WorktreeReader       = (*compositionWorktree)(nil)
	_ cli.WorktreeWorkflow     = (*compositionWorktree)(nil)
)

// storeInitializerFor binds the initializer authority and every follow-on
// ingest operation to one concrete Store. A construction failure is captured
// once and replayed verbatim; the initializer never attempts replacement
// construction after CLI dispatch.
func storeInitializerFor(cliStore *store.Store, canonicalRoot string, constructionErr error, materializers ...cli.WorktreeMaterializer) cli.StoreInitializer {
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
		var transactions cli.IngestTransactionStore = cliStore
		if len(materializers) > 0 && materializers[0] != nil {
			transactions = materializingIngestTransactions{
				IngestTransactionStore: cliStore,
				materializer:           materializers[0],
			}
		}
		return cli.StoreInitialization{
			InitializationID: transaction.ID(),
			Transactions:     transactions,
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
	runtimeWorktreeFactory := cli.NewRuntimeWorktreeFactory(provider, provider, provider)
	worktreeAuthority := newCompositionWorktree(storeRoot, cliStore, provider, runtimeWorktreeFactory, storeErr)

	return cli.NewWithStoreInitializerAndWorktree(
		provider,
		commandStore,
		emitter,
		storeInitializerFor(cliStore, storeRoot, storeErr, worktreeAuthority),
		worktreeAuthority,
		worktreeAuthority,
	)
}
