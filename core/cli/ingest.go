package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"zsh-pro/core/buildinfo"
	"zsh-pro/core/dto"
	"zsh-pro/core/ir"
	"zsh-pro/core/model"
	"zsh-pro/core/util"
)

const ingestUsageLine = "usage: zsh-pro ingest [path] [--json]"

type ingestArguments struct {
	path   string
	asJSON bool
}

func parseIngestArguments(args []string) (ingestArguments, bool) {
	parsed := ingestArguments{path: "~/.zshrc"}
	for _, arg := range args {
		if arg == "--json" {
			parsed.asJSON = true
		}
	}

	pathSeen := false
	for _, arg := range args {
		switch {
		case arg == "--json":
			// Idempotent by contract.
		case arg == "" || arg[0] == '-':
			return ingestArguments{asJSON: parsed.asJSON}, false
		case pathSeen:
			return ingestArguments{asJSON: parsed.asJSON}, false
		default:
			parsed.path = arg
			pathSeen = true
		}
	}

	// Home expansion is owned exclusively by this argv boundary. The prepared
	// filesystem transaction and Store receive the resulting path verbatim.
	parsed.path = util.ExpandHome(parsed.path)
	return parsed, true
}

type ingestFailureReason uint8

const (
	ingestFailureNone ingestFailureReason = iota
	ingestFailureProviderUnavailable
	ingestFailureCapabilityUnavailable
	ingestFailureStoreUnavailable
	ingestFailureSourceUnavailable
	ingestFailureSourceInvalid
	ingestFailureTransactionUnavailable
	ingestFailureStartupConflict
	ingestFailureStoreConflict
	ingestFailureNotCommitted
	ingestFailureRecoveryRequired
	ingestFailureFinalizeFailed
)

// ingestControllerSeams are copied per invocation. They expose only
// deterministic observation/failure points used to prove transaction ordering;
// production always passes nil.
type ingestControllerSeams struct {
	install      *installTransactionSeams
	event        func(string)
	afterBuild   func(*model.Profile)
	beforeCommit func(*guardedInstallTransaction) error
}

// WorktreeMaterializer consumes only exact authoritative publication evidence.
// The production adapter reads the committed DTO at revision before delegating
// to the durable worktree Service; it never resolves a process-local current
// profile or accepts caller-supplied source bytes.
type WorktreeMaterializer interface {
	MaterializeCommittedWorktree(context.Context, string, string) error
}

func (reason ingestFailureReason) String() string {
	switch reason {
	case ingestFailureProviderUnavailable:
		return "shell provider unavailable"
	case ingestFailureCapabilityUnavailable:
		return "startup transaction unavailable"
	case ingestFailureStoreUnavailable:
		return "profile store unavailable"
	case ingestFailureSourceUnavailable:
		return "startup source unavailable"
	case ingestFailureSourceInvalid:
		return "startup source invalid"
	case ingestFailureTransactionUnavailable:
		return "profile transaction unavailable"
	case ingestFailureStartupConflict:
		return "startup changed during ingest"
	case ingestFailureStoreConflict:
		return "profile changed during ingest"
	case ingestFailureNotCommitted:
		return "profile was not committed"
	case ingestFailureRecoveryRequired:
		return "manual recovery required"
	case ingestFailureFinalizeFailed:
		return "ingest cleanup incomplete"
	default:
		return ""
	}
}

func newIngestResult(exitCode int) dto.IngestResult {
	return dto.IngestResult{ExitCode: exitCode, Withheld: []dto.IngestWithheld{}, Warnings: []string{}}
}

func (c *CLI) runIngestCommand(args []string, stdout, stderr io.Writer) int {
	parsed, valid := parseIngestArguments(args)
	if !valid {
		if parsed.asJSON {
			return renderIngestResult(newIngestResult(int(model.ExitUsageErr)), ingestFailureNone, true, stdout, stderr)
		}
		_, _ = fmt.Fprintln(stderr, ingestUsageLine)
		return int(model.ExitUsageErr)
	}

	result := newIngestResult(int(model.ExitRuntimeErr))
	if isNilLike(c.provider) {
		return renderIngestResult(result, ingestFailureProviderUnavailable, parsed.asJSON, stdout, stderr)
	}
	if c.storeInitializer == nil {
		return renderIngestResult(result, ingestFailureStoreUnavailable, parsed.asJSON, stdout, stderr)
	}

	return c.runIngestWithSeams(context.Background(), parsed, stdout, stderr, nil)
}

func (c *CLI) runIngestWithSeams(
	ctx context.Context,
	arguments ingestArguments,
	stdout, stderr io.Writer,
	input *ingestControllerSeams,
) int {
	result := newIngestResult(int(model.ExitRuntimeErr))
	fail := func(reason ingestFailureReason) int {
		return renderIngestResult(result, reason, arguments.asJSON, stdout, stderr)
	}
	if isNilLike(c.provider) {
		return fail(ingestFailureProviderUnavailable)
	}
	if c.storeInitializer == nil {
		return fail(ingestFailureStoreUnavailable)
	}

	seams := normalizedIngestControllerSeams(input)
	targetPath, err := filepath.Abs(arguments.path)
	if err != nil {
		return fail(ingestFailureSourceUnavailable)
	}
	preflightTarget, err := resolveIngestPreflightTarget(targetPath)
	if err != nil {
		return fail(ingestFailureSourceUnavailable)
	}
	seams.event("preflight")
	if err := preflightAtomicRenameTarget(ctx, preflightTarget, seams.install); err != nil {
		if errors.Is(err, ErrInstallRecoveryRequired) {
			result.RecoveryRequired = true
			return fail(ingestFailureRecoveryRequired)
		}
		return fail(ingestFailureCapabilityUnavailable)
	}

	paths, err := resolveInstallPaths()
	if err != nil {
		return fail(ingestFailureSourceUnavailable)
	}
	seams.event("initializer")
	initialization, err := c.storeInitializer(ctx)
	if err != nil {
		return fail(ingestFailureStoreUnavailable)
	}
	if initialization.InitializationID.IsZero() || isNilLike(initialization.Transactions) {
		result.RecoveryRequired = true
		return fail(ingestFailureRecoveryRequired)
	}
	if err := validateInstallRootRelationship(paths.runtimeDir, initialization); err != nil {
		if rollbackStoreInitialization(initialization) != nil {
			result.RecoveryRequired = true
			return fail(ingestFailureRecoveryRequired)
		}
		return fail(ingestFailureStoreUnavailable)
	}

	seams.event("store:begin")
	begin, err := initialization.Transactions.BeginIngest(ctx, initialization.InitializationID)
	if err != nil || !validIngestBegin(initialization.InitializationID, begin) {
		if begin.RecoveryRequired || !begin.InitializerRollbackSafe {
			result.RecoveryRequired = begin.RecoveryRequired
			return fail(ingestFailureTransactionUnavailable)
		}
		if rollbackStoreInitialization(initialization) != nil {
			result.RecoveryRequired = true
			return fail(ingestFailureRecoveryRequired)
		}
		return fail(ingestFailureTransactionUnavailable)
	}

	state := ingestCompensationState{
		initialization: initialization,
		begin:          begin,
		transaction:    initialization.Transactions,
		tokenActive:    true,
		seams:          seams,
	}
	abortBeforeVisibleEffect := func(reason ingestFailureReason) int {
		if state.compensatePreCommitFilesystemFirst() {
			result.RecoveryRequired = true
			return fail(ingestFailureRecoveryRequired)
		}
		return fail(reason)
	}

	seams.event("source:prepare")
	prepared, err := prepareIngestInstallAt(targetPath, renderInstallBlock())
	if err != nil {
		return abortBeforeVisibleEffect(ingestFailureSourceInvalid)
	}
	seams.event("source:parse")
	blocks, err := c.provider.Parse(prepared.eligibleSource)
	if err != nil {
		return abortBeforeVisibleEffect(ingestFailureSourceInvalid)
	}
	profile := ir.Build(blocks, c.provider)
	if !profileMatchesParsedBlocks(profile, blocks) {
		return abortBeforeVisibleEffect(ingestFailureSourceInvalid)
	}
	if seams.afterBuild != nil {
		seams.afterBuild(&profile)
	}
	managed, unmanaged, unmanagedLines, valid := ingestProjection(profile)
	if !valid {
		return abortBeforeVisibleEffect(ingestFailureSourceInvalid)
	}
	result.ManagedEntries = managed
	result.UnmanagedStatements = unmanaged
	result.UnmanagedSourceLines = &unmanagedLines
	result.SourceStatements = len(profile.Entries)
	result.AccountedStatements = managed + unmanaged
	if prepared.appendWarning != "" {
		result.Warnings = append(result.Warnings, prepared.appendWarning)
	}

	seams.event("target:prepare")
	targetTransaction, err := prepareGuardedInstallTransaction(ctx, prepared, seams.install)
	if err != nil {
		if errors.Is(err, ErrInstallRecoveryRequired) {
			result.RecoveryRequired = true
			return fail(ingestFailureRecoveryRequired)
		}
		return abortBeforeVisibleEffect(ingestFailureStartupConflict)
	}
	state.targetTransaction = targetTransaction
	defer targetTransaction.closeHandles()

	seams.event("cache:prepare")
	cacheState, err := secureCacheDirectory(paths.runtimeDir)
	if err != nil {
		return abortBeforeVisibleEffect(ingestFailureStartupConflict)
	}
	state.cache = cacheState
	defer func() { _ = cacheState.close() }()
	loaderRollback, err := cacheState.prepareLoaderRollback()
	if err != nil {
		return abortBeforeVisibleEffect(ingestFailureStartupConflict)
	}
	state.loaderRollback = &loaderRollback
	loaderScript := "# zsh-pro cached loader version " + buildinfo.Version + "\n" + c.provider.HookScript()
	loaderWrite, err := cacheState.prepareValidatedLoader([]byte(loaderScript))
	if err != nil {
		loaderRollback.discard()
		return abortBeforeVisibleEffect(ingestFailureStartupConflict)
	}
	state.loaderWrite = &loaderWrite
	seams.event("loader:promote")
	if err := loaderWrite.promote(); err != nil {
		return abortBeforeVisibleEffect(ingestFailureStartupConflict)
	}

	seams.event("target:promote")
	targetOutcome, err := targetTransaction.promote()
	state.targetOutcome = targetOutcome
	if err != nil {
		if targetOutcome.RecoveryRequired || errors.Is(err, ErrInstallRecoveryRequired) {
			result.RecoveryRequired = true
			result.StartupInstalled = targetOutcome.Promoted
			return fail(ingestFailureRecoveryRequired)
		}
		return abortBeforeVisibleEffect(ingestFailureStartupConflict)
	}
	result.StartupInstalled = targetOutcome.Promoted

	if seams.beforeCommit != nil {
		if err := seams.beforeCommit(targetTransaction); err != nil {
			if state.compensatePreCommitFilesystemFirst() {
				result.RecoveryRequired = true
				result.StartupInstalled = state.targetRemainsInstalled
				return fail(ingestFailureRecoveryRequired)
			}
			result.StartupInstalled = false
			return fail(ingestFailureNotCommitted)
		}
	}
	seams.event("target:validate")
	targetCurrent, targetErr := targetTransaction.targetMatches(targetTransaction.expectedCandidate)
	if targetErr != nil || !targetCurrent {
		if state.compensatePreCommitFilesystemFirst() {
			result.RecoveryRequired = true
			result.StartupInstalled = state.targetRemainsInstalled
			return fail(ingestFailureRecoveryRequired)
		}
		result.StartupInstalled = false
		return fail(ingestFailureStartupConflict)
	}

	seams.event("store:commit")
	commit, commitErr := initialization.Transactions.CommitIngest(
		ctx, initialization.InitializationID, begin.TransactionID,
		profile, "zsh-pro: ingest baseline",
	)
	state.tokenActive = false
	if !validIngestCommit(initialization.InitializationID, begin.TransactionID, commit) {
		state.retainTarget()
		if state.compensatePreCommitFilesystemFirst() {
			result.RecoveryRequired = true
			result.StartupInstalled = state.targetRemainsInstalled
			return fail(ingestFailureRecoveryRequired)
		}
		return fail(ingestFailureTransactionUnavailable)
	}

	if commit.Status != model.IngestCommitCommitted {
		if commit.Status == model.IngestCommitRecoveryRequired || commit.RecoveryRequired {
			state.retainTarget()
		}
		if state.compensatePreCommitFilesystemFirst() {
			result.RecoveryRequired = true
			result.StartupInstalled = state.targetRemainsInstalled
			return fail(ingestFailureRecoveryRequired)
		}
		result.StartupInstalled = false
		switch commit.Status {
		case model.IngestCommitConflict:
			return fail(ingestFailureStoreConflict)
		default:
			_ = commitErr
			return fail(ingestFailureNotCommitted)
		}
	}

	// Publication truth is authoritative even if Commit returned an observation
	// or cleanup error. Finalization removes only the already-promoted
	// transaction artifacts; it never writes the startup target again.
	result.ProfileCommitted = true
	result.StartupInstalled = true
	result.Withheld = ingestWithheldDTO(commit.Withheld)
	seams.event("target:finalize")
	targetFinalizeErr := targetOutcome.Finalize()
	loaderRollback.discard()
	loaderWrite.discard()
	seams.event("initializer:finalize")
	initializationFinalizeErr := finalizeStoreInitialization(initialization)
	if targetFinalizeErr != nil || initializationFinalizeErr != nil || commit.RecoveryRequired || commitErr != nil {
		result.RecoveryRequired = true
		return fail(ingestFailureFinalizeFailed)
	}
	materializer, ok := initialization.Transactions.(WorktreeMaterializer)
	if !ok || isNilLike(materializer) {
		result.RecoveryRequired = true
		return fail(ingestFailureRecoveryRequired)
	}
	seams.event("worktree:materialize")
	if err := materializer.MaterializeCommittedWorktree(ctx, "main", *commit.PublishedRevision); err != nil {
		result.RecoveryRequired = true
		return fail(ingestFailureRecoveryRequired)
	}
	result.OK = true
	result.ExitCode = int(model.ExitClean)
	return renderIngestResult(result, ingestFailureNone, arguments.asJSON, stdout, stderr)
}

func normalizedIngestControllerSeams(input *ingestControllerSeams) ingestControllerSeams {
	var seams ingestControllerSeams
	if input != nil {
		seams = *input
	}
	if seams.event == nil {
		seams.event = func(string) {}
	}
	return seams
}

func resolveIngestPreflightTarget(path string) (string, error) {
	path = filepath.Clean(path)
	info, err := os.Lstat(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return path, nil
	case err != nil:
		return "", err
	case info.Mode()&os.ModeSymlink != 0:
		return filepath.EvalSymlinks(path)
	default:
		return path, nil
	}
}

func validIngestBegin(initializationID model.InstallInitializationID, outcome model.IngestBeginOutcome) bool {
	return !initializationID.IsZero() && outcome.InitializationID == initializationID &&
		!outcome.TransactionID.IsZero() && outcome.Lifecycle == model.IngestLifecycleActive &&
		outcome.Baseline.InitializationID == initializationID &&
		outcome.Baseline.TransactionID == outcome.TransactionID && outcome.Baseline.Validate() == nil &&
		!outcome.RecoveryRequired
}

func validIngestCommit(
	initializationID model.InstallInitializationID,
	transactionID model.IngestTransactionID,
	outcome model.IngestCommitOutcome,
) bool {
	if outcome.InitializationID != initializationID || outcome.TransactionID != transactionID {
		return false
	}
	if outcome.ValidatePublishedRevision() != nil {
		return false
	}
	switch outcome.Status {
	case model.IngestCommitCommitted, model.IngestCommitConflict,
		model.IngestCommitNotCommitted, model.IngestCommitRecoveryRequired:
		return true
	default:
		return false
	}
}

func profileMatchesParsedBlocks(profile model.Profile, blocks []model.Block) bool {
	if len(profile.Entries) != len(blocks) {
		return false
	}
	for index := range blocks {
		if profile.Entries[index].Text != blocks[index].Text ||
			profile.Entries[index].StartLine != blocks[index].StartLine || blocks[index].StartLine < 1 {
			return false
		}
	}
	return true
}

func ingestProjection(profile model.Profile) (managed, unmanaged, unmanagedLines int, valid bool) {
	for _, entry := range profile.Entries {
		if entry.EffectiveManaged() {
			managed++
			continue
		}
		unmanaged++
		unmanagedLines += physicalStatementLines(entry.Text)
	}
	return managed, unmanaged, unmanagedLines, managed+unmanaged == len(profile.Entries)
}

func physicalStatementLines(text string) int {
	if text == "" {
		return 0
	}
	lines := 1
	for _, character := range text {
		if character == '\n' {
			lines++
		}
	}
	if text[len(text)-1] == '\n' {
		lines--
	}
	return lines
}

func ingestWithheldDTO(report model.WithheldReport) []dto.IngestWithheld {
	result := make([]dto.IngestWithheld, 0, len(report))
	for _, withheld := range report {
		result = append(result, dto.IngestWithheld{Name: withheld.Name, StartLine: withheld.StartLine})
	}
	return result
}

type ingestCompensationState struct {
	initialization         StoreInitialization
	begin                  model.IngestBeginOutcome
	transaction            IngestTransactionStore
	tokenActive            bool
	targetTransaction      *guardedInstallTransaction
	targetOutcome          promotionOutcome
	cache                  *cacheDirectoryState
	loaderWrite            *preparedCacheWrite
	loaderRollback         *cacheWriteRollback
	seams                  ingestControllerSeams
	targetRemainsInstalled bool
}

func (state *ingestCompensationState) retainTarget() {
	if state.targetTransaction != nil {
		state.targetTransaction.disposition = promotionRecoveryRequired
	}
	state.targetRemainsInstalled = state.targetOutcome.Promoted
}

// compensatePreCommitFilesystemFirst is the only noncommit compensation path.
// It classifies/restores-or-retains the startup target before touching Store,
// loader/cache, or initializer state. Any uncertainty stops the sequence.
func (state *ingestCompensationState) compensatePreCommitFilesystemFirst() bool {
	state.seams.event("filesystem:rollback")
	if state.targetTransaction != nil {
		var err error
		if state.targetOutcome.transaction != nil {
			err = state.targetOutcome.Rollback()
		} else {
			err = state.targetTransaction.rollback()
		}
		if err != nil {
			state.targetRemainsInstalled = state.targetOutcome.Promoted
			return true
		}
	}
	state.targetRemainsInstalled = false

	if state.tokenActive {
		state.seams.event("store:abort")
		abort, err := state.transaction.AbortIngest(
			context.Background(), state.initialization.InitializationID, state.begin.TransactionID,
		)
		state.tokenActive = false
		if err != nil || abort.RecoveryRequired || abort.Lifecycle != model.IngestLifecycleTerminal ||
			abort.InitializationID != state.initialization.InitializationID || abort.TransactionID != state.begin.TransactionID {
			return true
		}
	}
	if state.loaderWrite != nil && state.loaderWrite.promoted {
		state.seams.event("loader:rollback")
		if state.loaderRollback == nil || rollbackPromotedCache(state.loaderWrite, state.loaderRollback) != nil {
			return true
		}
	}
	if state.loaderWrite != nil {
		state.loaderWrite.discard()
	}
	if state.loaderRollback != nil {
		state.loaderRollback.discard()
	}
	if state.cache != nil {
		state.seams.event("cache:rollback")
		if state.cache.rollback() != nil {
			return true
		}
	}
	state.seams.event("initializer:rollback")
	return rollbackStoreInitialization(state.initialization) != nil
}

func renderIngestResult(result dto.IngestResult, reason ingestFailureReason, asJSON bool, stdout, stderr io.Writer) int {
	if result.Withheld == nil {
		result.Withheld = []dto.IngestWithheld{}
	}
	if result.Warnings == nil {
		result.Warnings = []string{}
	}
	if asJSON {
		_ = json.NewEncoder(stdout).Encode(result)
		return result.ExitCode
	}

	destination := stdout
	if result.ExitCode != int(model.ExitClean) {
		destination = stderr
	}
	if result.OK {
		_, _ = fmt.Fprintln(destination, "zsh-pro: ingest complete")
	} else if reason != ingestFailureNone {
		_, _ = fmt.Fprintf(destination, "zsh-pro: ingest failed: %s\n", reason.String())
	}
	_, _ = fmt.Fprintf(destination, "profile committed: %s\n", yesNo(result.ProfileCommitted))
	_, _ = fmt.Fprintf(destination, "startup installed: %s\n", yesNo(result.StartupInstalled))
	_, _ = fmt.Fprintf(destination, "recovery required: %s\n", yesNo(result.RecoveryRequired))
	_, _ = fmt.Fprintf(destination, "managed entries: %d\n", result.ManagedEntries)
	_, _ = fmt.Fprintf(destination, "unmanaged statements: %d\n", result.UnmanagedStatements)
	if result.UnmanagedSourceLines != nil {
		_, _ = fmt.Fprintf(destination, "unmanaged source lines: %d\n", *result.UnmanagedSourceLines)
	}
	_, _ = fmt.Fprintf(destination, "source statements: %d\n", result.SourceStatements)
	_, _ = fmt.Fprintf(destination, "accounted statements: %d\n", result.AccountedStatements)
	for _, withheld := range result.Withheld {
		_, _ = fmt.Fprintf(destination, "withheld: %s (line %d)\n", withheld.Name, withheld.StartLine)
	}
	for _, warning := range result.Warnings {
		_, _ = fmt.Fprintf(destination, "warning: %s\n", warning)
	}
	return result.ExitCode
}

func yesNo(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}
