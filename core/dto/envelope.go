package dto

// Envelope is the single top-level JSON object emitted by every command.
type Envelope struct {
	Tool        string   `json:"tool"`
	Version     string   `json:"version"`
	Command     string   `json:"command"`
	OK          bool     `json:"ok"`
	IssuesFound bool     `json:"issues_found"`
	ExitCode    int      `json:"exit_code"`
	Analysis    Analysis `json:"analysis"`
}
