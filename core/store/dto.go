// Package store is the git-backed persistence layer for the Phase 2 model.Profile
// IR, where each git branch is an environment profile. It is shell-agnostic: it
// must never import the concrete zsh provider package (that import is reserved for
// the composition root); shell-specific regeneration is injected via a seam. This
// file owns the profile.json serialization contract — the payload Read returns and
// Commit writes.
package store

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"

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
	Text               string                 `json:"text"`
	StartLine          int                    `json:"startLine"`
	Category           model.Category         `json:"category"`
	Kind               model.BlockKind        `json:"kind"`
	CmdName            string                 `json:"cmdName"`
	Names              []string               `json:"names"`
	Value              string                 `json:"value"`
	Exported           bool                   `json:"exported"`
	Managed            bool                   `json:"managed"`
	Override           model.ManagedOverride  `json:"override"`
	Dynamic            bool                   `json:"dynamic"`
	StructuralFidelity *structuralFidelityDTO `json:"structuralFidelity,omitempty"`
	Secret             *model.SecretRef       `json:"secret,omitempty"`
	ValueMode          model.ValueMode        `json:"valueMode,omitempty"`
	// Pointer presence preserves decoded/parsed empty strings through JSON.
	RuntimeValue *string       `json:"runtimeValue,omitempty"`
	FunctionBody *string       `json:"functionBody,omitempty"`
	ListValue    *listValueDTO `json:"listValue,omitempty"`
}

const structuralFidelityVersion = 3

// structuralFidelityDTO is presence-aware on purpose: absent bool fields are
// historical unknowns, never inferred as false.
type structuralFidelityDTO struct {
	Version          int             `json:"version"`
	Append           *bool           `json:"append"`
	Array            *bool           `json:"array"`
	Flagged          *bool           `json:"flagged"`
	Indexed          *bool           `json:"indexed"`
	AliasAssignment  *bool           `json:"aliasAssignment"`
	DeclarationFlags *[]string       `json:"declarationFlags"`
	OptionFlags      *stringSliceDTO `json:"optionFlags"`
}

// stringSliceDTO preserves nil, explicit-empty, and populated marker slices.
// A pointer to this wrapper records field presence separately from Values, so
// a current fidelity record cannot confuse a missing optionFlags marker with a
// deliberately nil source-control slice.
type stringSliceDTO struct {
	Values []string `json:"values"`
}

// listValueDTO is the optional persistence form of parser-verified PATH/FPATH
// semantics. The flags intentionally do not omit false so each segment shape is
// explicit and stable in profile JSON.
type listValueDTO struct {
	Segments []listSegmentDTO `json:"segments"`
}

type listSegmentDTO struct {
	Value   string `json:"value"`
	Source  string `json:"source,omitempty"`
	Dynamic bool   `json:"dynamic"`
	Self    bool   `json:"self"`
}

// profileDTO is the top-level wire shape of a serialized profile.
type profileDTO struct {
	Entries  []entryDTO            `json:"entries"`
	Worktree *committedWorktreeDTO `json:"worktree,omitempty"`
}

type committedWorktreeDTO struct {
	Schema     string             `json:"schema"`
	Projection *liveProjectionDTO `json:"projection"`
}

type liveProjectionDTO struct {
	Schema     string                 `json:"schema"`
	States     *liveIdentityStatesDTO `json:"states"`
	Tombstones *liveIdentitiesDTO     `json:"tombstones"`
}

// The wrappers distinguish an absent mandatory field from present-null,
// present-empty, and populated ordered slices.
type liveIdentityStatesDTO struct {
	Values []liveIdentityStateDTO `json:"values"`
}

type liveIdentitiesDTO struct {
	Values []liveIdentityDTO `json:"values"`
}

type liveIdentityStateDTO struct {
	Identity liveIdentityDTO `json:"identity"`
	Value    liveValueDTO    `json:"value"`
}

type liveIdentityDTO struct {
	Kind model.LiveKind `json:"kind"`
	Name string         `json:"name"`
}

type liveValueDTO struct {
	Present *bool           `json:"present"`
	Scalar  *string         `json:"scalar,omitempty"`
	List    *stringSliceDTO `json:"list,omitempty"`
	Option  *bool           `json:"option,omitempty"`
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

// MarshalCommittedWorktree stores the complete historical source under the
// legacy-compatible entries field and adds one versioned final projection.
// Shared branch/revision/shell coordination deliberately stays out of Git.
func MarshalCommittedWorktree(document model.CommittedWorktree) ([]byte, error) {
	if err := validateCommittedWorktreeDTO(document); err != nil {
		return nil, err
	}
	dto := profileDTO{
		Entries: make([]entryDTO, len(document.Source.Entries)),
		Worktree: &committedWorktreeDTO{
			Schema: document.Schema,
			Projection: &liveProjectionDTO{
				Schema:     document.Projection.Schema,
				States:     &liveIdentityStatesDTO{Values: toLiveIdentityStatesDTO(document.Projection.States)},
				Tombstones: &liveIdentitiesDTO{Values: toLiveIdentitiesDTO(document.Projection.Tombstones)},
			},
		},
	}
	if document.Source.Entries == nil {
		dto.Entries = nil
	}
	for i := range document.Source.Entries {
		dto.Entries[i] = toEntryDTO(document.Source.Entries[i])
	}
	payload, err := json.MarshalIndent(dto, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(payload, '\n'), nil
}

// UnmarshalCommittedWorktree accepts only the complete current committed
// schema. Source-only legacy objects remain available through UnmarshalProfile;
// this method never invents final projection state for them.
func UnmarshalCommittedWorktree(payload []byte) (model.CommittedWorktree, error) {
	var dto profileDTO
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&dto); err != nil {
		return model.CommittedWorktree{}, err
	}
	if err := requireJSONEOF(decoder); err != nil {
		return model.CommittedWorktree{}, err
	}
	if dto.Worktree == nil || dto.Worktree.Projection == nil || dto.Worktree.Projection.States == nil || dto.Worktree.Projection.Tombstones == nil {
		return model.CommittedWorktree{}, errors.New("committed worktree projection is incomplete")
	}
	document := model.CommittedWorktree{
		Schema: dto.Worktree.Schema,
		Source: profileFromDTOExact(dto.Entries),
		Projection: model.LiveProjection{
			Schema:     dto.Worktree.Projection.Schema,
			States:     fromLiveIdentityStatesDTO(dto.Worktree.Projection.States.Values),
			Tombstones: fromLiveIdentitiesDTO(dto.Worktree.Projection.Tombstones.Values),
		},
	}
	if err := validateCommittedWorktreeDTO(document); err != nil {
		return model.CommittedWorktree{}, err
	}
	return document, nil
}

func requireJSONEOF(decoder *json.Decoder) error {
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return errors.New("committed worktree has trailing JSON data")
		}
		return err
	}
	return nil
}

func profileFromDTOExact(entries []entryDTO) model.Profile {
	profile := model.Profile{Entries: make([]model.Entry, len(entries))}
	if entries == nil {
		profile.Entries = nil
	}
	for index := range entries {
		profile.Entries[index] = fromEntryDTO(entries[index])
	}
	return profile
}

func toLiveIdentityStatesDTO(states []model.LiveIdentityState) []liveIdentityStateDTO {
	if states == nil {
		return nil
	}
	out := make([]liveIdentityStateDTO, len(states))
	for index, state := range states {
		out[index] = liveIdentityStateDTO{
			Identity: toLiveIdentityDTO(state.Identity),
			Value:    toLiveValueDTO(state.Value),
		}
	}
	return out
}

func fromLiveIdentityStatesDTO(states []liveIdentityStateDTO) []model.LiveIdentityState {
	if states == nil {
		return nil
	}
	out := make([]model.LiveIdentityState, len(states))
	for index, state := range states {
		out[index] = model.LiveIdentityState{
			Identity: fromLiveIdentityDTO(state.Identity),
			Value:    fromLiveValueDTO(state.Value),
		}
	}
	return out
}

func toLiveIdentitiesDTO(identities []model.Identity) []liveIdentityDTO {
	if identities == nil {
		return nil
	}
	out := make([]liveIdentityDTO, len(identities))
	for index, identity := range identities {
		out[index] = toLiveIdentityDTO(identity)
	}
	return out
}

func fromLiveIdentitiesDTO(identities []liveIdentityDTO) []model.Identity {
	if identities == nil {
		return nil
	}
	out := make([]model.Identity, len(identities))
	for index, identity := range identities {
		out[index] = fromLiveIdentityDTO(identity)
	}
	return out
}

func toLiveIdentityDTO(identity model.Identity) liveIdentityDTO {
	return liveIdentityDTO{Kind: identity.Kind, Name: identity.Name}
}

func fromLiveIdentityDTO(identity liveIdentityDTO) model.Identity {
	return model.Identity{Kind: identity.Kind, Name: identity.Name}
}

func toLiveValueDTO(value model.LiveValue) liveValueDTO {
	present := value.Present
	out := liveValueDTO{
		Present: &present,
		Scalar:  cloneStringPointer(value.Scalar),
		Option:  cloneBoolPointer(value.Option),
	}
	if value.List != nil {
		out.List = &stringSliceDTO{Values: cloneStrings(value.List)}
	}
	return out
}

func fromLiveValueDTO(value liveValueDTO) model.LiveValue {
	out := model.LiveValue{
		Scalar: cloneStringPointer(value.Scalar),
		Option: cloneBoolPointer(value.Option),
	}
	if value.Present != nil {
		out.Present = *value.Present
	}
	if value.List != nil {
		out.List = cloneStrings(value.List.Values)
	}
	return out
}

func cloneBoolPointer(in *bool) *bool {
	if in == nil {
		return nil
	}
	out := *in
	return &out
}

func validateCommittedWorktreeDTO(document model.CommittedWorktree) error {
	if document.Schema != model.WorktreeSchemaV1 || document.Projection.Schema != model.WorktreeSchemaV1 {
		return errors.New("committed worktree schema is unsupported")
	}
	if len(document.Projection.States)+len(document.Projection.Tombstones) > model.MaxSnapshotRecords {
		return errors.New("committed worktree projection exceeds the record limit")
	}
	if err := validateCommittedSourceSecrets(document.Source); err != nil {
		return err
	}
	pinned := committedSourceSecretIdentities(document.Source)
	normalized, err := model.NormalizeLiveStates(document.Projection.States)
	if err != nil {
		return fmt.Errorf("validate committed projection: %w", err)
	}
	if len(normalized) != len(document.Projection.States) {
		return errors.New("committed worktree projection has duplicate states")
	}
	seen := make(map[model.Identity]bool, len(normalized)+len(document.Projection.Tombstones))
	for _, state := range normalized {
		if !state.Value.Present {
			return errors.New("committed worktree projection contains an absent state")
		}
		if pinned[state.Identity] {
			return errors.New("committed worktree projection contains a pinned identity")
		}
		seen[state.Identity] = true
	}
	for _, identity := range document.Projection.Tombstones {
		if err := model.ValidateIdentity(identity); err != nil {
			return fmt.Errorf("validate committed tombstone: %w", err)
		}
		if seen[identity] {
			return errors.New("committed worktree projection has a duplicate or overlapping tombstone")
		}
		if pinned[identity] {
			return errors.New("committed worktree projection contains a pinned identity")
		}
		seen[identity] = true
	}
	return nil
}

func validateCommittedSourceSecrets(profile model.Profile) error {
	for _, entry := range profile.Entries {
		if entry.Secret == nil {
			if entry.Category != model.CatSecrets {
				continue
			}
			if !isExcludableSecretShape(entry) || entry.ValueMode != model.ValueModeDynamic || !entry.Dynamic || entry.RuntimeValue != nil {
				return errors.New("committed worktree source contains an unredacted secret")
			}
			continue
		}
		ref := entry.Secret
		if entry.Category != model.CatSecrets || entry.Kind != model.KindAssignment || len(entry.Names) != 1 || entry.Names[0] == "" || ref.Key == "" || ref.Key != entry.Names[0] ||
			(ref.Kind != model.SecretRefKeychain && ref.Kind != model.SecretRefFile) || entry.Append || entry.Array || entry.Indexed || entry.Dynamic || entry.RuntimeValue != nil || entry.ValueMode != model.ValueModeUnsupported {
			return errors.New("committed worktree source contains an invalid SecretRef")
		}
		placeholder := secretRefValue(*ref)
		wantText := ref.Key + "=" + placeholder
		if entry.Exported {
			wantText = "export " + wantText
		}
		if entry.Text != wantText || entry.Value != placeholder {
			return errors.New("committed worktree source SecretRef is not redacted")
		}
	}
	return nil
}

func committedSourceSecretIdentities(profile model.Profile) map[model.Identity]bool {
	pinned := make(map[model.Identity]bool)
	for _, entry := range profile.Entries {
		if entry.Category == model.CatSecrets && entry.Kind == model.KindAssignment && len(entry.Names) == 1 {
			pinned[model.Identity{Kind: model.LiveEnv, Name: entry.Names[0]}] = true
		}
	}
	return pinned
}

// toEntryDTO maps a domain Entry to its wire DTO, defensively copying Names so the
// DTO never aliases the caller's backing array.
func toEntryDTO(e model.Entry) entryDTO {
	return entryDTO{
		Text:               e.Text,
		StartLine:          e.StartLine,
		Category:           e.Category,
		Kind:               e.Kind,
		CmdName:            e.CmdName,
		Names:              cloneNames(e.Names),
		Value:              e.Value,
		Exported:           e.Exported,
		Managed:            e.Managed,
		Override:           e.Override,
		Dynamic:            e.Dynamic,
		StructuralFidelity: toStructuralFidelityDTO(e),
		Secret:             e.Secret,
		ValueMode:          e.ValueMode,
		RuntimeValue:       cloneStringPointer(e.RuntimeValue),
		FunctionBody:       cloneStringPointer(e.FunctionBody),
		ListValue:          toListValueDTO(e.ListValue),
	}
}

// fromEntryDTO maps a wire DTO back to a domain Entry, defensively copying Names so
// the decoded Profile never shares a backing array with the DTO.
func fromEntryDTO(d entryDTO) model.Entry {
	return model.Entry{
		Text:                    d.Text,
		StartLine:               d.StartLine,
		Category:                d.Category,
		Kind:                    d.Kind,
		CmdName:                 d.CmdName,
		Names:                   cloneNames(d.Names),
		Value:                   d.Value,
		Exported:                d.Exported,
		Managed:                 d.Managed,
		Override:                d.Override,
		Dynamic:                 d.Dynamic,
		StructuralFidelityKnown: structuralFidelityKnown(d.StructuralFidelity),
		Secret:                  d.Secret,
		ValueMode:               d.ValueMode,
		RuntimeValue:            cloneStringPointer(d.RuntimeValue),
		FunctionBody:            cloneStringPointer(d.FunctionBody),
		ListValue:               fromListValueDTO(d.ListValue),
		Append:                  structuralFidelityMarker(d.StructuralFidelity, func(f *structuralFidelityDTO) *bool { return f.Append }),
		Array:                   structuralFidelityMarker(d.StructuralFidelity, func(f *structuralFidelityDTO) *bool { return f.Array }),
		Flagged:                 structuralFidelityMarker(d.StructuralFidelity, func(f *structuralFidelityDTO) *bool { return f.Flagged }),
		Indexed:                 structuralFidelityMarker(d.StructuralFidelity, func(f *structuralFidelityDTO) *bool { return f.Indexed }),
		AliasAssignment:         structuralFidelityMarker(d.StructuralFidelity, func(f *structuralFidelityDTO) *bool { return f.AliasAssignment }),
		DeclarationFlags:        structuralFidelityFlags(d.StructuralFidelity),
		OptionFlags:             structuralFidelityOptionFlags(d.StructuralFidelity),
	}
}

func toStructuralFidelityDTO(e model.Entry) *structuralFidelityDTO {
	if !e.StructuralFidelityKnown {
		return nil
	}
	flags := cloneStrings(e.DeclarationFlags)
	if flags == nil {
		flags = []string{}
	}
	return &structuralFidelityDTO{
		Version:          structuralFidelityVersion,
		Append:           boolPointer(e.Append),
		Array:            boolPointer(e.Array),
		Flagged:          boolPointer(e.Flagged),
		Indexed:          boolPointer(e.Indexed),
		AliasAssignment:  boolPointer(e.AliasAssignment),
		DeclarationFlags: &flags,
		OptionFlags:      &stringSliceDTO{Values: cloneStrings(e.OptionFlags)},
	}
}

func structuralFidelityKnown(in *structuralFidelityDTO) bool {
	return in != nil && in.Version == structuralFidelityVersion && in.Append != nil && in.Array != nil && in.Flagged != nil && in.Indexed != nil && in.AliasAssignment != nil && in.DeclarationFlags != nil && in.OptionFlags != nil
}

func structuralFidelityMarker(in *structuralFidelityDTO, marker func(*structuralFidelityDTO) *bool) bool {
	if !structuralFidelityKnown(in) {
		return false
	}
	return *marker(in)
}

func structuralFidelityFlags(in *structuralFidelityDTO) []string {
	if !structuralFidelityKnown(in) {
		return nil
	}
	return cloneStrings(*in.DeclarationFlags)
}

func structuralFidelityOptionFlags(in *structuralFidelityDTO) []string {
	if !structuralFidelityKnown(in) {
		return nil
	}
	return cloneStrings(in.OptionFlags.Values)
}

func boolPointer(value bool) *bool { return &value }

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

func cloneStrings(in []string) []string {
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

func toListValueDTO(in *model.ListValue) *listValueDTO {
	if in == nil || !in.Valid() {
		return nil
	}
	out := &listValueDTO{Segments: make([]listSegmentDTO, len(in.Segments))}
	for i, segment := range in.Segments {
		out.Segments[i] = listSegmentDTO{
			Value:   segment.Value,
			Source:  segment.Source,
			Dynamic: segment.Dynamic,
			Self:    segment.Self,
		}
	}
	return out
}

func fromListValueDTO(in *listValueDTO) *model.ListValue {
	if in == nil {
		return nil
	}
	out := &model.ListValue{Segments: make([]model.ListSegment, len(in.Segments))}
	for i, segment := range in.Segments {
		out.Segments[i] = model.ListSegment{
			Value:   segment.Value,
			Source:  segment.Source,
			Dynamic: segment.Dynamic,
			Self:    segment.Self,
		}
	}
	if !out.Valid() {
		return nil
	}
	return out
}
