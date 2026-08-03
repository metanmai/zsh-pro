package dto

// IngestWithheld identifies one source declaration whose literal value was
// excluded from persistence. It deliberately carries only name/line metadata.
type IngestWithheld struct {
	Name      string `json:"name"`
	StartLine int    `json:"start_line"`
}

// IngestResult is the value-free wire contract for ingest. It contains only
// outcome flags, projection/accounting counts, withheld metadata, and stable
// warnings; raw paths, source, refs, object IDs, backend IDs, and errors have no
// representable field.
type IngestResult struct {
	OK                   bool             `json:"ok"`
	ExitCode             int              `json:"exit_code"`
	ProfileCommitted     bool             `json:"profile_committed"`
	StartupInstalled     bool             `json:"startup_installed"`
	RecoveryRequired     bool             `json:"recovery_required"`
	ManagedEntries       int              `json:"managed_entries"`
	UnmanagedStatements  int              `json:"unmanaged_statements"`
	UnmanagedSourceLines *int             `json:"unmanaged_source_lines,omitempty"`
	SourceStatements     int              `json:"source_statements"`
	AccountedStatements  int              `json:"accounted_statements"`
	Withheld             []IngestWithheld `json:"withheld"`
	Warnings             []string         `json:"warnings"`
}
