// Package store is the git-backed persistence layer for the Phase 2 model.Profile
// IR, where each git branch is an environment profile. It is shell-agnostic: it
// must never import the concrete zsh provider package (that import is reserved for
// the composition root); shell-specific regeneration is injected via a seam. This
// file owns the profile.json serialization contract — the payload Read returns and
// Commit writes.
package store

import (
	"encoding/json"

	"zsh-pro/core/model"
)

// Serialization design (resolves critical decision #1, option b): a store-local
// DTO maps model.Profile<->JSON rather than tagging the Phase 2 model.Entry
// (which carries no json tags). The DTO is chosen over tagging model.Entry so the
// Phase 2 IR struct stays a pure domain type and serialization concerns live in
// core/store; it is lossless for all derived fields (D-01) and deterministic via
// encoding/json declaration order (struct fields emit in source order; the only
// map — none here — would sort its keys). Values pass through verbatim: no $HOME
// / $(...) resolution ever happens in the store (T-03-02; no util.ExpandHome).

// entryDTO mirrors every model.Entry field one-to-one with stable lowercase json
// keys. The DTO — not model.Entry — carries the wire tags (decision #1).
type entryDTO struct {
	Text      string                `json:"text"`
	StartLine int                   `json:"startLine"`
	Category  model.Category        `json:"category"`
	Kind      model.BlockKind       `json:"kind"`
	CmdName   string                `json:"cmdName"`
	Names     []string              `json:"names"`
	Value     string                `json:"value"`
	Exported  bool                  `json:"exported"`
	Managed   bool                  `json:"managed"`
	Override  model.ManagedOverride `json:"override"`
	Dynamic   bool                  `json:"dynamic"`
	Secret    *model.SecretRef      `json:"secret,omitempty"`
	ValueMode model.ValueMode       `json:"valueMode,omitempty"`
	// Pointer presence preserves decoded/parsed empty strings through JSON.
	RuntimeValue *string `json:"runtimeValue,omitempty"`
	FunctionBody *string `json:"functionBody,omitempty"`
}

// profileDTO is the top-level wire shape of a serialized profile.
type profileDTO struct {
	Entries []entryDTO `json:"entries"`
}

// MarshalProfile serializes a model.Profile to deterministic profile.json bytes
// (D-01). Output is json.MarshalIndent two-space-indented with a single trailing
// newline for stable diffs / POSIX-friendly files. It is lossless: every derived
// field round-trips via UnmarshalProfile. Values are emitted verbatim — dynamic
// values such as $HOME/go are never resolved.
func MarshalProfile(p model.Profile) ([]byte, error) {
	dto := profileDTO{Entries: make([]entryDTO, len(p.Entries))}
	for i := range p.Entries {
		dto.Entries[i] = toEntryDTO(p.Entries[i])
	}
	b, err := json.MarshalIndent(dto, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

// UnmarshalProfile reconstructs a model.Profile from profile.json bytes. It is the
// exact inverse of MarshalProfile (no re-parse, so the store stays shell-agnostic).
// An entry-less profile decodes to the zero value model.Profile{} (nil Entries),
// not a non-nil empty slice — the same nil-preserving choice cloneNames makes — so
// the empty-profile round-trip Read(Commit(model.Profile{})) is reflect.DeepEqual
// to its input (D-01 fidelity at the no-entries edge).
func UnmarshalProfile(b []byte) (model.Profile, error) {
	var dto profileDTO
	if err := json.Unmarshal(b, &dto); err != nil {
		return model.Profile{}, err
	}
	if len(dto.Entries) == 0 {
		return model.Profile{}, nil
	}
	p := model.Profile{Entries: make([]model.Entry, len(dto.Entries))}
	for i := range dto.Entries {
		p.Entries[i] = fromEntryDTO(dto.Entries[i])
	}
	return p, nil
}

// toEntryDTO maps a domain Entry to its wire DTO, defensively copying Names so the
// DTO never aliases the caller's backing array.
func toEntryDTO(e model.Entry) entryDTO {
	return entryDTO{
		Text:         e.Text,
		StartLine:    e.StartLine,
		Category:     e.Category,
		Kind:         e.Kind,
		CmdName:      e.CmdName,
		Names:        cloneNames(e.Names),
		Value:        e.Value,
		Exported:     e.Exported,
		Managed:      e.Managed,
		Override:     e.Override,
		Dynamic:      e.Dynamic,
		Secret:       e.Secret,
		ValueMode:    e.ValueMode,
		RuntimeValue: cloneStringPointer(e.RuntimeValue),
		FunctionBody: cloneStringPointer(e.FunctionBody),
	}
}

// fromEntryDTO maps a wire DTO back to a domain Entry, defensively copying Names so
// the decoded Profile never shares a backing array with the DTO.
func fromEntryDTO(d entryDTO) model.Entry {
	return model.Entry{
		Text:         d.Text,
		StartLine:    d.StartLine,
		Category:     d.Category,
		Kind:         d.Kind,
		CmdName:      d.CmdName,
		Names:        cloneNames(d.Names),
		Value:        d.Value,
		Exported:     d.Exported,
		Managed:      d.Managed,
		Override:     d.Override,
		Dynamic:      d.Dynamic,
		Secret:       d.Secret,
		ValueMode:    d.ValueMode,
		RuntimeValue: cloneStringPointer(d.RuntimeValue),
		FunctionBody: cloneStringPointer(d.FunctionBody),
	}
}

// cloneNames returns a fresh copy of the slice (nil stays nil so a no-names entry
// round-trips as nil, not []string{}, preserving reflect.DeepEqual equality).
func cloneNames(in []string) []string {
	if in == nil {
		return nil
	}
	out := make([]string, len(in))
	copy(out, in)
	return out
}

func cloneStringPointer(in *string) *string {
	if in == nil {
		return nil
	}
	out := *in
	return &out
}
