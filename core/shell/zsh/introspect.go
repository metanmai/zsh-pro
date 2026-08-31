package zsh

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"zsh-pro/core/buildinfo"
	"zsh-pro/core/model"
	"zsh-pro/core/shell"
)

var _ shell.Provider = Provider{}

var errLiveSnapshotFrame = errors.New("live snapshot frame is invalid")

const (
	liveSnapshotMarker  = "ZP_LIVE_SNAPSHOT"
	liveSnapshotVersion = "1"
	liveRecordMarker    = "R"
	liveEndMarker       = "E"
)

// liveCaptureProgram defines one composable function that records supported
// state from the zsh process which sources it. It deliberately does not use
// emulate/local-options because the caller's option state is part of the
// snapshot, and it never launches a child shell.
const liveCaptureProgram = `
_zp_live_capture() {
  zmodload zsh/parameter 2>/dev/null || return 1
  local name descriptor value option_state
  local -a elements

  builtin printf '%s\0' ZP_LIVE_SNAPSHOT 1
  for name in "${(@ok)parameters}"; do
    [[ "$name" == PATH || "$name" == FPATH ]] && continue
    descriptor="${parameters[$name]}"
    [[ "$descriptor" == *scalar* && "$descriptor" == *export* ]] || continue
    value="${(P)name}"
    builtin printf '%s\0' R env "$name" exported 1 "$value"
  done
  for name in "${(@ok)aliases}"; do
	[[ "${name:l}" != _zp_* && "${name:l}" != __zp_* ]] || continue
	[[ "$name" == [0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz_]* && "$name" != *[^0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz_.-]* ]] || continue
    builtin printf '%s\0' R alias "$name" body 1 "${aliases[$name]}"
  done
  for name in "${(@ok)functions}"; do
	[[ "${name:l}" != _zp_* && "${name:l}" != __zp_* ]] || continue
	[[ "$name" == [0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz_]* && "$name" != *[^0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz_.-]* ]] || continue
    builtin printf '%s\0' R function "$name" body 1 "${functions[$name]}"
  done

  elements=("${path[@]}")
  builtin printf '%s\0' R path PATH ordered "${#elements}"
  for value in "${elements[@]}"; do builtin printf '%s\0' "$value"; done
  elements=("${fpath[@]}")
  builtin printf '%s\0' R fpath FPATH ordered "${#elements}"
  for value in "${elements[@]}"; do builtin printf '%s\0' "$value"; done

  for name in "${(@ok)options}"; do
    option_state="${options[$name]}"
    builtin printf '%s\0' R option "$name" boolean 1 "$option_state"
  done
  builtin printf '%s\0' E
}
`

// LiveCaptureSource returns source which must be evaluated by the current zsh;
// executing it only defines the capture function and performs no subprocess or
// filesystem work.
func (Provider) LiveCaptureSource() string { return liveCaptureProgram }

// DecodeLiveSnapshot validates one complete versioned NUL frame. Any malformed
// input returns the zero snapshot so callers can never observe a trusted prefix.
func (Provider) DecodeLiveSnapshot(frame []byte) (model.LiveSnapshot, error) {
	if len(frame) == 0 || len(frame) > model.MaxSnapshotBytes || frame[len(frame)-1] != 0 {
		return model.LiveSnapshot{}, errLiveSnapshotFrame
	}
	reader := liveFrameReader{frame: frame}
	if !reader.expect(liveSnapshotMarker) || !reader.expect(liveSnapshotVersion) {
		return model.LiveSnapshot{}, errLiveSnapshotFrame
	}
	capacity := len(frame) / 32
	if capacity > model.MaxSnapshotRecords {
		capacity = model.MaxSnapshotRecords
	}
	states := make([]model.LiveIdentityState, 0, capacity)
	for {
		marker, ok := reader.next()
		if !ok {
			return model.LiveSnapshot{}, errLiveSnapshotFrame
		}
		if marker == liveEndMarker {
			if !reader.done() {
				return model.LiveSnapshot{}, errLiveSnapshotFrame
			}
			snapshot := model.LiveSnapshot{ByteSize: uint64(len(frame)), States: states}
			if err := model.ValidateLiveSnapshot(snapshot); err != nil {
				return model.LiveSnapshot{}, errLiveSnapshotFrame
			}
			return snapshot, nil
		}
		if marker != liveRecordMarker || len(states) == model.MaxSnapshotRecords {
			return model.LiveSnapshot{}, errLiveSnapshotFrame
		}
		state, ok := decodeLiveRecord(&reader)
		if !ok || model.ValidateLiveIdentityState(state) != nil {
			return model.LiveSnapshot{}, errLiveSnapshotFrame
		}
		states = append(states, state)
	}
}

type liveFrameReader struct {
	frame  []byte
	offset int
}

func (reader *liveFrameReader) next() (string, bool) {
	if reader.offset >= len(reader.frame) {
		return "", false
	}
	end := bytes.IndexByte(reader.frame[reader.offset:], 0)
	if end < 0 {
		return "", false
	}
	field := string(reader.frame[reader.offset : reader.offset+end])
	reader.offset += end + 1
	return field, true
}

func (reader *liveFrameReader) expect(want string) bool {
	got, ok := reader.next()
	return ok && got == want
}

func (reader *liveFrameReader) done() bool { return reader.offset == len(reader.frame) }

func decodeLiveRecord(reader *liveFrameReader) (model.LiveIdentityState, bool) {
	kindField, kindOK := reader.next()
	name, nameOK := reader.next()
	attribute, attributeOK := reader.next()
	countField, countOK := reader.next()
	if !kindOK || !nameOK || !attributeOK || !countOK {
		return model.LiveIdentityState{}, false
	}
	count, err := strconv.ParseUint(countField, 10, 64)
	if err != nil || strconv.FormatUint(count, 10) != countField || count > model.MaxSnapshotRecords {
		return model.LiveIdentityState{}, false
	}
	kind := model.LiveKind(kindField)
	identity := model.Identity{Kind: kind, Name: name}
	if model.ValidateIdentity(identity) != nil {
		return model.LiveIdentityState{}, false
	}

	var value model.LiveValue
	switch kind {
	case model.LiveEnv:
		if attribute != "exported" || count != 1 {
			return model.LiveIdentityState{}, false
		}
		value, countOK = decodeLiveScalar(reader)
	case model.LiveAlias, model.LiveFunction:
		if attribute != "body" || count != 1 {
			return model.LiveIdentityState{}, false
		}
		value, countOK = decodeLiveScalar(reader)
	case model.LivePath, model.LiveFPath:
		if attribute != "ordered" {
			return model.LiveIdentityState{}, false
		}
		value, countOK = decodeLiveList(reader, int(count))
	case model.LiveOption:
		if attribute != "boolean" || count != 1 {
			return model.LiveIdentityState{}, false
		}
		value, countOK = decodeLiveOption(reader)
	default:
		return model.LiveIdentityState{}, false
	}
	if !countOK {
		return model.LiveIdentityState{}, false
	}
	return model.LiveIdentityState{Identity: identity, Value: value}, true
}

func decodeLiveScalar(reader *liveFrameReader) (model.LiveValue, bool) {
	value, ok := reader.next()
	if !ok {
		return model.LiveValue{}, false
	}
	return model.ScalarLiveValue(value), true
}

func decodeLiveList(reader *liveFrameReader, count int) (model.LiveValue, bool) {
	values := make([]string, count)
	for index := range values {
		value, ok := reader.next()
		if !ok {
			return model.LiveValue{}, false
		}
		values[index] = value
	}
	return model.ListLiveValue(values), true
}

func decodeLiveOption(reader *liveFrameReader) (model.LiveValue, bool) {
	value, ok := reader.next()
	if !ok {
		return model.LiveValue{}, false
	}
	switch value {
	case "on":
		return model.OptionLiveValue(true), true
	case "off":
		return model.OptionLiveValue(false), true
	default:
		return model.LiveValue{}, false
	}
}

// Categories returns the taxonomy in load order.
func (Provider) Categories() []model.Category { return model.Categories() }

// introspectScript runs under `zsh -f` (no rc files). It sources the target
// ($1) with output suppressed, then dumps the resolved identity tables in a
// section-delimited format.
const introspectScript = `
emulate -L zsh
zmodload zsh/parameter 2>/dev/null
source "$1" >/dev/null 2>&1
source_status=$?
(( source_status == 0 )) || exit "$source_status"
print -r -- '##ALIASES##'
for k in "${(@k)aliases}"; do print -r -- "$k"; done
print -r -- '##FUNCTIONS##'
for k in "${(@k)functions}"; do print -r -- "$k"; done
print -r -- '##ENV##'
for k v in "${(@kv)parameters}"; do [[ "$v" == *export* ]] && print -r -- "$k"; done
print -r -- '##PATH##'
for p in $path; do print -r -- "$p"; done
print -r -- '##OPTIONS##'
for k in "${(@k)options}"; do [[ "${options[$k]}" == on ]] && print -r -- "$k"; done
print -rN -- ''
print -r -- '##ALIASBODIES##'
for k in "${(@ok)aliases}"; do print -rN -- "$k" "${aliases[$k]}"; done
print -rN -- ''
print -r -- '##FUNCTIONBODIES##'
for k in "${(@ok)functions}"; do print -rN -- "$k" "${functions[$k]}"; done
print -rN -- ''
print -r -- '##END##'
`

// Introspect runs the config in a sandboxed zsh and returns the resolved
// identity set. Any failure (zsh missing, timeout) returns Available:false.
func (p Provider) Introspect(path string) (model.IdentitySet, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "zsh", "-f", "-c", introspectScript, buildinfo.Name, path)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return model.IdentitySet{Available: false}, err
	}
	return p.parseIntrospect(out.String()), nil
}

func (Provider) parseIntrospect(s string) model.IdentitySet {
	ids := model.IdentitySet{
		Aliases:     map[string]bool{},
		Functions:   map[string]bool{},
		Env:         map[string]bool{},
		Options:     map[string]bool{},
		AliasBodies: map[string]string{}, FunctionBodies: map[string]string{},
		Available: true,
	}
	bodyAt := strings.Index(s, "\n\x00##ALIASBODIES##\n")
	prefix := s
	if bodyAt >= 0 {
		prefix = s[:bodyAt]
	}
	parseBodies(s, "##ALIASBODIES##", "##FUNCTIONBODIES##", ids.AliasBodies)
	parseBodies(s, "##FUNCTIONBODIES##", "##END##", ids.FunctionBodies)
	section := ""
	for _, line := range strings.Split(prefix, "\n") {
		switch line {
		case "##ALIASES##", "##FUNCTIONS##", "##ENV##", "##PATH##", "##OPTIONS##", "##ALIASBODIES##", "##FUNCTIONBODIES##", "##END##":
			section = line
			continue
		}
		if line == "" {
			continue
		}
		switch section {
		case "##ALIASES##":
			ids.Aliases[line] = true
		case "##FUNCTIONS##":
			ids.Functions[line] = true
		case "##ENV##":
			ids.Env[line] = true
		case "##PATH##":
			ids.Path = append(ids.Path, line)
		case "##OPTIONS##":
			ids.Options[line] = true
		case "##ALIASBODIES##", "##FUNCTIONBODIES##":
			// Body sections are parsed separately with NUL framing.
		}
	}
	// v1 scoping: the engine consumes only ids.Available (see analyze.Analyzer.Analyze).
	// The resolved tables (aliases/functions/env/path/options) are captured here
	// for a backlogged enrichment — env-isolated introspection plus opaque-init
	// identity detection — and are intentionally not yet wired into the report.
	return ids
}

// parseBodies reads NUL-framed name/body pairs. A dedicated NUL delimiter
// precedes each following sentinel, so body text that begins with ## is safe.
func parseBodies(s, start, end string, dst map[string]string) {
	startMarker := "\x00\x00" + start + "\n"
	if start == "##ALIASBODIES##" {
		startMarker = "\n\x00" + start + "\n"
	}
	startAt := strings.Index(s, startMarker)
	if startAt < 0 {
		return
	}
	data := s[startAt+len(startMarker):]
	marker := "\x00\x00" + end
	if i := strings.Index(data, marker); i >= 0 {
		data = data[:i]
	}
	parts := strings.Split(data, "\x00")
	for i := 0; i+1 < len(parts); i += 2 {
		if parts[i] == "" {
			continue
		}
		dst[parts[i]] = parts[i+1]
	}
}
