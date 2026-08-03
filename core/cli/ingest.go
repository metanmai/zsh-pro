package cli

import (
	"encoding/json"
	"fmt"
	"io"

	"zsh-pro/core/dto"
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

	// Task 3 replaces this closed staged failure with the transaction
	// controller. Keeping the staged path value-free ensures Task 1 can land the
	// public contract without inventing a partial filesystem transaction.
	return renderIngestResult(result, ingestFailureTransactionUnavailable, parsed.asJSON, stdout, stderr)
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
