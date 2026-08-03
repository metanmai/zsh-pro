package cli

import (
	"io"

	"zsh-pro/core/dto"
)

const ingestUsageLine = "usage: zsh-pro ingest [path] [--json]"

type ingestArguments struct {
	path   string
	asJSON bool
}

// parseIngestArguments is introduced with its RED contract; Task 1 GREEN
// supplies the strict parser and one-time home expansion.
func parseIngestArguments([]string) (ingestArguments, bool) {
	return ingestArguments{}, false
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

// renderIngestResult is intentionally incomplete in the RED contract commit.
func renderIngestResult(dto.IngestResult, ingestFailureReason, bool, io.Writer, io.Writer) int {
	return -1
}
