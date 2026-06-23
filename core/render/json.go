package render

import (
	"encoding/json"

	"zsh-pro/core/buildinfo"
	"zsh-pro/core/dto"
	"zsh-pro/core/model"
)

// JSONRenderer produces exactly one JSON object (the agent contract).
type JSONRenderer struct{}

// Render maps the model onto the wire DTO and marshals the envelope.
func (r JSONRenderer) Render(a model.Analysis) ([]byte, error) {
	return json.MarshalIndent(r.toDTO(a), "", "  ")
}

// toDTO maps the domain model onto the wire-format envelope field-by-field.
// Nil slices are preserved as nil (Categories/Issues serialize as null;
// the omitempty fields Items/Lines/Notes are omitted) to keep the output
// byte-identical to the model that previously carried the json tags.
func (JSONRenderer) toDTO(a model.Analysis) dto.Envelope {
	out := dto.Analysis{
		Path:         a.Path,
		Lines:        a.Lines,
		BlockCount:   a.BlockCount,
		OpaqueBlocks: a.OpaqueBlocks,
		HasSecrets:   a.HasSecrets,
		Introspected: a.Introspected,
		Notes:        a.Notes,
	}
	if a.Categories != nil {
		out.Categories = make([]dto.CategorySummary, len(a.Categories))
		for i, c := range a.Categories {
			out.Categories[i] = dto.CategorySummary{
				Category: string(c.Category),
				Count:    c.Count,
				Items:    c.Items,
			}
		}
	}
	if a.Issues != nil {
		out.Issues = make([]dto.Issue, len(a.Issues))
		for i, is := range a.Issues {
			out.Issues[i] = dto.Issue{
				Kind:  string(is.Kind),
				Name:  is.Name,
				Lines: is.Lines,
				Note:  is.Note,
			}
		}
	}
	return dto.Envelope{
		Tool:        buildinfo.Name,
		Version:     buildinfo.Version,
		Command:     buildinfo.Command,
		OK:          true,
		IssuesFound: len(a.Issues) > 0,
		ExitCode:    int(a.ExitCode()),
		Analysis:    out,
	}
}
