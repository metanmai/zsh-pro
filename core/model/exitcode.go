package model

// ExitCode is the process exit status and agent contract: 0 clean, 1 runtime
// error, 2 usage error, 3 actionable (issues found).
type ExitCode int

const (
	ExitClean      ExitCode = 0
	ExitRuntimeErr ExitCode = 1
	ExitUsageErr   ExitCode = 2
	ExitActionable ExitCode = 3
)
