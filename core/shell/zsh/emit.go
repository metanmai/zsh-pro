package zsh

import (
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"

	"zsh-pro/core/activate"
	"zsh-pro/core/model"
)

func emitLiveIdentityState(_ model.LiveIdentityState, _ bool) ([]byte, error) {
	return nil, fmt.Errorf("live identity emission is not implemented")
}

func emitLiveIdentityTombstone(_ model.Identity) ([]byte, error) {
	return nil, fmt.Errorf("live identity removal is not implemented")
}

func emitLiveOperation(_ activate.Op) ([]byte, error) {
	return nil, fmt.Errorf("live operation emission is not implemented")
}

// renderValue and renderList are package-level seams deliberately kept
// injectable. Besides making the escaping boundary easy to audit, this lets
// residue tests substitute mutants without changing the zero-value Provider.
var renderValue = func(applied string, dynamic bool) string {
	if dynamic {
		return applied
	}
	return zquote(applied)
}

var renderList = func(name string, additions []string, _ bool) string {
	base := encodeSlot("BASE", name)
	array := "path"
	if strings.EqualFold(name, "PATH") {
		base = "ZP_BASE_PATH"
	} else if strings.EqualFold(name, "FPATH") {
		base = "ZP_BASE_FPATH"
		array = "fpath"
	}
	parts := make([]string, 0, len(additions))
	for _, addition := range additions {
		// A clean expansion such as $HOME/bin or $path is intentionally left
		// dynamic. Everything else, including command substitutions mixed with
		// punctuation or whitespace, is an inert literal.
		parts = append(parts, renderValue(addition, dynamicSegment(addition)))
	}
	return fmt.Sprintf("%s=\"$%s\"; %s=(%s $%s)", name, base, array, strings.Join(parts, " "), array)
}

func renderListDelta(name string, additions []string, baseIndex *int, dynamic []bool) string {
	if baseIndex == nil || len(dynamic) != len(additions) {
		return renderList(name, additions, false)
	}
	base := "ZP_BASE_PATH"
	if strings.EqualFold(name, "FPATH") {
		base = "ZP_BASE_FPATH"
	}
	parts := make([]string, 0, len(additions)+1)
	for i := 0; i <= len(additions); i++ {
		if i == *baseIndex {
			parts = append(parts, "$"+base)
		}
		if i < len(additions) {
			parts = append(parts, renderValue(additions[i], dynamic[i]))
		}
	}
	return fmt.Sprintf("%s=%s", name, strings.Join(parts, ":"))
}

var optionNameRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
var aliasFuncNameRe = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]*$`)
var envNameRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// zquote emits one shell literal. Single quotes are escaped using the
// standard close-quote/backslash-quote/reopen sequence; newlines and all
// metacharacters consequently remain data when the loader is eval'd.
func zquote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// quoteFunctionBody quotes the string assigned to zsh's functions map without
// passing it through zquote (function bodies are executable code once invoked,
// unlike scalar/alias data). Expansion is suppressed while assigning, then
// zsh evaluates the stored body when the function is called.
func quoteFunctionBody(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, "$", `\$`)
	s = strings.ReplaceAll(s, "`", "\\`")
	return `"` + s + `"`
}

func dynamicSegment(s string) bool {
	if !strings.ContainsAny(s, "$`") || strings.ContainsAny(s, " ;|&<>\n\r\t") {
		return false
	}
	// Only simple variable/parameter expansions are considered portable
	// self-references. Command substitutions with punctuation are static data
	// at this boundary and are therefore quoted.
	if strings.HasPrefix(s, "$HOME/") || strings.HasPrefix(s, "$PATH") || strings.HasPrefix(s, "$path") || strings.HasPrefix(s, "${") {
		return !strings.Contains(s, "$(") && !strings.Contains(s, "`")
	}
	return regexp.MustCompile(`^\$[A-Za-z_][A-Za-z0-9_]*(/[^;[:space:]]*)?$`).MatchString(s)
}

// encodeSlot is injective for every UTF-8 name and emits only identifier-safe
// bytes. The state-class component keeps independently captured identities in
// separate namespaces.
func encodeSlot(class, name string) string {
	return "ZP_" + class + "_" + hex.EncodeToString([]byte(name))
}

func safeEnvName(s string) bool       { return envNameRe.MatchString(s) }
func safeOptionName(s string) bool    { return optionNameRe.MatchString(s) }
func safeAliasFuncName(s string) bool { return aliasFuncNameRe.MatchString(s) }

const runtimeHelpers = `
# These helpers intentionally remain plain functions: option changes made by
# the apply loader must persist after the function returns.
zp_capture_scalar() {
  local var="$1" originalSlot="$2" presenceSlot="$3" exportSlot="$4"
  if [[ "${(P)+presenceSlot}" == 1 ]]; then return 0; fi
  if [[ "${(P)+var}" == 1 ]]; then
    typeset -g "$presenceSlot=1"
    typeset -g "$originalSlot=${(P)var}"
    if [[ "${parameters[$var]}" == *export* ]]; then typeset -g "$exportSlot=1"
    else typeset -g "$exportSlot=0"; fi
  else
    typeset -g "$presenceSlot=0"
    typeset -g "$exportSlot=0"
  fi
}
zp_preflight_undo_slots() {
  local slot
  for slot in "$@"; do
    if [[ -n "$slot" && "${(P)+slot}" == 1 && "${parameters[$slot]}" == *readonly* ]]; then
      return 1
    fi
  done
  return 0
}
zp_preflight_restore_scalar() {
  local var="$1" applied="$2" originalSlot="$3" presenceSlot="$4" exportSlot="$5" appliedSlot="${6-}"
  if [[ -n "$appliedSlot" && "${(P)+appliedSlot}" == 1 ]]; then applied="${(P)appliedSlot}"; fi
  # Commit clears these slots even when a user has changed the target since
  # activation, so validate them before returning for drift.
  zp_preflight_undo_slots "$originalSlot" "$presenceSlot" "$exportSlot" "$appliedSlot" || return $?
  if [[ "${(P)+var}" != 1 || "${(P)var}" != "$applied" ]]; then return 0; fi
  if [[ "${parameters[$var]}" == *readonly* ]]; then return 1; fi
  if [[ "${(P)+presenceSlot}" != 1 ]]; then return 1; fi
  if [[ "${(P)presenceSlot}" == 1 && ( "${(P)+originalSlot}" != 1 || "${(P)+exportSlot}" != 1 ) ]]; then
    return 1
  fi
  return 0
}
zp_restore_scalar() {
  local var="$1" applied="$2" originalSlot="$3" presenceSlot="$4" exportSlot="$5" appliedSlot="${6-}" rc=0
  if [[ "${(P)+appliedSlot}" == 1 ]]; then applied="${(P)appliedSlot}"; fi
  if [[ "${(P)+var}" != 1 || "${(P)var}" != "$applied" ]]; then return 0; fi
  if [[ "${(P)presenceSlot}" == 1 ]]; then
    if typeset -g "$var=${(P)originalSlot}"; then :; else return $?; fi
    if [[ "${(P)exportSlot}" == 1 ]]; then export "$var"; else typeset +x "$var"; fi
    rc=$?
    if (( rc != 0 )); then return "$rc"; fi
  elif unset "$var"; then :; else return $?; fi
  return 0
}
zp_commit_restore_scalar() {
  local originalSlot="$1" presenceSlot="$2" exportSlot="$3" appliedSlot="${4-}"
  if [[ -n "$appliedSlot" ]]; then
    if unset "$originalSlot" "$presenceSlot" "$exportSlot" "$appliedSlot"; then return 0; else return $?; fi
  fi
  if unset "$originalSlot" "$presenceSlot" "$exportSlot"; then return 0; else return $?; fi
}
zp_commit_undo_slots() {
  if unset "$@"; then return 0; else return $?; fi
}
`

type emitNames struct {
	apply, deactivate                             string
	captureScalar, preflightScalar, restoreScalar string
	preflightUndoSlots, commitScalar              string
	commitUndoSlots                               string
	// captureApplied records every applied scalar in a global slot for the
	// standalone emitter. Runtime payloads omit that duplicate for static
	// values: their retained reverse already carries the expected literal, so a
	// second global copy would unnecessarily extend a resolved secret's
	// lifetime. Dynamic values still require a slot for their evaluated value.
	captureApplied bool
}

// Emit renders one plan into two directly sourceable blocks. This public
// emitter keeps the historical helper names for standalone plan tests; the
// runtime transport below uses loader-owned helpers and unique function names.
func (Provider) Emit(p activate.Plan) (apply, deactivate string, err error) {
	return emitPlan(p, emitNames{
		apply:              "zp_apply",
		deactivate:         "zp_deactivate",
		captureScalar:      "zp_capture_scalar",
		preflightScalar:    "zp_preflight_restore_scalar",
		preflightUndoSlots: "zp_preflight_undo_slots",
		restoreScalar:      "zp_restore_scalar",
		commitScalar:       "zp_commit_restore_scalar",
		commitUndoSlots:    "zp_commit_undo_slots",
		captureApplied:     true,
	}, true)
}

// EmitRuntime renders a loader-only payload. The caller supplies freshly
// generated function names, while scalar helpers stay in the sourced loader
// so the emitted source retains only the active reverse that can carry
// runtime-resolved values.
func (Provider) EmitRuntime(p activate.Plan, applyName, deactivateName string) (apply, deactivate string, err error) {
	if !safeAliasFuncName(applyName) || !safeAliasFuncName(deactivateName) {
		return "", "", fmt.Errorf("runtime emission requires safe function names")
	}
	return emitPlan(p, emitNames{
		apply:              applyName,
		deactivate:         deactivateName,
		captureScalar:      "_zp_capture_scalar",
		preflightScalar:    "_zp_preflight_restore_scalar",
		preflightUndoSlots: "_zp_preflight_undo_slots",
		restoreScalar:      "_zp_restore_scalar",
		commitScalar:       "_zp_commit_restore_scalar",
		commitUndoSlots:    "_zp_commit_undo_slots",
		captureApplied:     false,
	}, false)
}

func emitPlan(p activate.Plan, names emitNames, includeHelpers bool) (apply, deactivate string, err error) {
	var a, d strings.Builder
	if includeHelpers {
		a.WriteString(runtimeHelpers)
	}
	a.WriteString("\n" + names.apply + "() {\n")
	for _, op := range p.Activate {
		if err := emitActivate(&a, op, names); err != nil {
			return "", "", err
		}
	}
	a.WriteString("}\n")

	if includeHelpers {
		d.WriteString(runtimeHelpers)
	}
	d.WriteString("\n" + names.deactivate + "() {\n")
	// Reversal is deliberately split into three phases. The preflight catches
	// known non-restorable state before any operation changes the terminal; the
	// commit phase consumes undo slots only after every reverse operation has
	// succeeded. A failed retained reverse can therefore be retried safely.
	for _, op := range p.Deactivate {
		if err := emitPreflightDeactivate(&d, op, names); err != nil {
			return "", "", err
		}
	}
	for _, op := range p.Deactivate {
		if err := emitDeactivate(&d, op, names); err != nil {
			return "", "", err
		}
	}
	for _, op := range p.Deactivate {
		if err := emitCommitDeactivate(&d, op, names); err != nil {
			return "", "", err
		}
	}
	d.WriteString("}\n")
	return a.String(), d.String(), nil
}

func emitActivate(b *strings.Builder, op activate.Op, names emitNames) error {
	switch x := op.(type) {
	case activate.SetScalar:
		if !safeEnvName(x.Name) {
			return nil
		}
		original := encodeSlot("ORIGINAL_SCALAR", x.Name)
		presence := encodeSlot("PRESENT_SCALAR", x.Name)
		exported := encodeSlot("EXPORTED_SCALAR", x.Name)
		applied := encodeSlot("APPLIED_SCALAR", x.Name)
		fmt.Fprintf(b, "  %s %s %s %s %s\n", names.captureScalar, x.Name, original, presence, exported)
		if x.Exported {
			fmt.Fprintf(b, "  export %s=%s\n", x.Name, renderValue(x.Applied, x.Dynamic))
		} else {
			fmt.Fprintf(b, "  typeset -g %s=%s\n", x.Name, renderValue(x.Applied, x.Dynamic))
		}
		if names.captureApplied || x.Dynamic {
			fmt.Fprintf(b, "  typeset -g %s=\"$%s\"\n", applied, x.Name)
		}
	case activate.ApplyListDelta:
		if !safeEnvName(x.Name) {
			return nil
		}
		base, present := "ZP_BASE_PATH", "ZP_BASE_PATH_PRESENT"
		if strings.EqualFold(x.Name, "FPATH") {
			base, present = "ZP_BASE_FPATH", "ZP_BASE_FPATH_PRESENT"
		}
		fmt.Fprintf(b, "  if (( ! ${+%s} )); then if (( ${+%s} )); then typeset -g %s=1; else typeset -g %s=0; fi; typeset -g %s=\"$%s\"; fi\n", base, x.Name, present, present, base, x.Name)
		fmt.Fprintf(b, "  %s\n", renderListDelta(x.Name, x.Additions, x.BaseIndex, x.AdditionDynamic))
	case activate.AddAlias:
		if !safeAliasFuncName(x.Name) {
			return nil
		}
		original := encodeSlot("ORIGINAL_ALIAS", x.Name)
		presence := encodeSlot("PRESENT_ALIAS", x.Name)
		fmt.Fprintf(b, "  if (( ! ${+%s} )); then if (( ${+aliases[%s]} )); then typeset -g %s=1; typeset -g \"%s=${aliases[%s]}\"; else typeset -g %s=0; fi; fi\n", presence, x.Name, presence, original, x.Name, presence)
		fmt.Fprintf(b, "  alias %s=%s\n", x.Name, renderValue(x.Body, x.Dynamic))
	case activate.AddFunc:
		if !safeAliasFuncName(x.Name) {
			return nil
		}
		original := encodeSlot("ORIGINAL_FUNCTION", x.Name)
		presence := encodeSlot("PRESENT_FUNCTION", x.Name)
		fmt.Fprintf(b, "  if (( ! ${+%s} )); then if (( ${+functions[%s]} )); then typeset -g %s=1; typeset -g \"%s=${functions[%s]}\"; else typeset -g %s=0; fi; fi\n", presence, x.Name, presence, original, x.Name, presence)
		fmt.Fprintf(b, "  functions[%s]=%s\n", x.Name, quoteFunctionBody(x.Body))
	case activate.SetOption:
		if !safeOptionName(x.Name) {
			return nil
		}
		slot := encodeSlot("WAS_ON_OPTION", x.Name)
		fmt.Fprintf(b, "  if (( ! ${+%s} )); then if [[ -o %s ]]; then typeset -g %s=1; else typeset -g %s=0; fi; fi\n", slot, x.Name, slot, slot)
		if x.Enabled {
			fmt.Fprintf(b, "  setopt %s\n", x.Name)
		} else {
			fmt.Fprintf(b, "  unsetopt %s\n", x.Name)
		}
	default:
		return fmt.Errorf("activate: unsupported operation %T", op)
	}
	return nil
}

func emitPreflightDeactivate(b *strings.Builder, op activate.Op, names emitNames) error {
	switch x := op.(type) {
	case activate.RestoreScalar:
		if !safeEnvName(x.Name) {
			return nil
		}
		emitScalarCall(b, names.preflightScalar, x.Name, x.Applied, x.Dynamic, names)
	case activate.UnsetScalar:
		if !safeEnvName(x.Name) {
			return nil
		}
		emitScalarCall(b, names.preflightScalar, x.Name, x.Applied, x.Dynamic, names)
	case activate.RebuildListFromBase:
		if !safeEnvName(x.Name) {
			return nil
		}
		fmt.Fprintf(b, "  if [[ \"${parameters[%s]}\" == *readonly* || \"${parameters[%s]}\" == *readonly* ]]; then return 1; fi\n", x.Name, strings.ToLower(x.Name))
	case activate.RestoreShadowedAlias:
		if !safeAliasFuncName(x.Name) {
			return nil
		}
		original := encodeSlot("ORIGINAL_ALIAS", x.Name)
		presence := encodeSlot("PRESENT_ALIAS", x.Name)
		fmt.Fprintf(b, "  if (( ${+%s} )); then %s %s %s || return $?; fi\n", presence, names.preflightUndoSlots, original, presence)
	case activate.RestoreShadowedFunc:
		if !safeAliasFuncName(x.Name) {
			return nil
		}
		original := encodeSlot("ORIGINAL_FUNCTION", x.Name)
		presence := encodeSlot("PRESENT_FUNCTION", x.Name)
		fmt.Fprintf(b, "  if (( ${+%s} )); then %s %s %s || return $?; fi\n", presence, names.preflightUndoSlots, original, presence)
	case activate.RestoreOption:
		if !safeOptionName(x.Name) {
			return nil
		}
		slot := encodeSlot("WAS_ON_OPTION", x.Name)
		fmt.Fprintf(b, "  if (( ${+%s} )); then %s %s || return $?; fi\n", slot, names.preflightUndoSlots, slot)
	case activate.Unalias, activate.UnsetFunc:
		return nil
	default:
		return fmt.Errorf("deactivate: unsupported operation %T", op)
	}
	return nil
}

func emitDeactivate(b *strings.Builder, op activate.Op, names emitNames) error {
	switch x := op.(type) {
	case activate.RestoreScalar:
		if !safeEnvName(x.Name) {
			return nil
		}
		emitScalarCall(b, names.restoreScalar, x.Name, x.Applied, x.Dynamic, names)
	case activate.UnsetScalar:
		if !safeEnvName(x.Name) {
			return nil
		}
		emitScalarCall(b, names.restoreScalar, x.Name, x.Applied, x.Dynamic, names)
	case activate.RebuildListFromBase:
		if !safeEnvName(x.Name) {
			return nil
		}
		base := encodeSlot("BASE", x.Name)
		if strings.EqualFold(x.Name, "PATH") {
			base = "ZP_BASE_PATH"
		} else if strings.EqualFold(x.Name, "FPATH") {
			base = "ZP_BASE_FPATH"
		}
		present := "ZP_BASE_PATH_PRESENT"
		if strings.EqualFold(x.Name, "FPATH") {
			present = "ZP_BASE_FPATH_PRESENT"
		}
		fmt.Fprintf(b, "  if (( ${+%s} )) && [[ \"$%s\" != 1 ]]; then unset %s %s; else %s=\"$%s\"; fi\n", present, present, x.Name, strings.ToLower(x.Name), x.Name, base)
	case activate.Unalias:
		if !safeAliasFuncName(x.Name) {
			return nil
		}
		fmt.Fprintf(b, "  unalias %s 2>/dev/null || :\n", x.Name)
	case activate.RestoreShadowedAlias:
		if !safeAliasFuncName(x.Name) {
			return nil
		}
		original := encodeSlot("ORIGINAL_ALIAS", x.Name)
		presence := encodeSlot("PRESENT_ALIAS", x.Name)
		fmt.Fprintf(b, "  if (( ${+%s} )); then if (( %s )); then if alias %s=\"${%s}\"; then :; else return $?; fi; else unalias %s 2>/dev/null || :; fi; fi\n", presence, presence, x.Name, original, x.Name)
	case activate.UnsetFunc:
		if !safeAliasFuncName(x.Name) {
			return nil
		}
		fmt.Fprintf(b, "  unset -f %s 2>/dev/null || :\n", x.Name)
	case activate.RestoreShadowedFunc:
		if !safeAliasFuncName(x.Name) {
			return nil
		}
		original := encodeSlot("ORIGINAL_FUNCTION", x.Name)
		presence := encodeSlot("PRESENT_FUNCTION", x.Name)
		fmt.Fprintf(b, "  if (( ${+%s} )); then if (( %s )); then if functions[%s]=\"${%s}\"; then :; else return $?; fi; else unset -f %s 2>/dev/null || :; fi; fi\n", presence, presence, x.Name, original, x.Name)
	case activate.RestoreOption:
		if !safeOptionName(x.Name) {
			return nil
		}
		slot := encodeSlot("WAS_ON_OPTION", x.Name)
		fmt.Fprintf(b, "  if (( ${+%s} )); then if (( %s )); then setopt %s || return $?; else unsetopt %s || return $?; fi; fi\n", slot, slot, x.Name, x.Name)
	default:
		return fmt.Errorf("deactivate: unsupported operation %T", op)
	}
	return nil
}

func emitCommitDeactivate(b *strings.Builder, op activate.Op, names emitNames) error {
	switch x := op.(type) {
	case activate.RestoreScalar:
		if !safeEnvName(x.Name) {
			return nil
		}
		emitScalarCommit(b, x.Name, x.Dynamic, names)
	case activate.UnsetScalar:
		if !safeEnvName(x.Name) {
			return nil
		}
		emitScalarCommit(b, x.Name, x.Dynamic, names)
	case activate.RestoreShadowedAlias:
		if !safeAliasFuncName(x.Name) {
			return nil
		}
		original := encodeSlot("ORIGINAL_ALIAS", x.Name)
		presence := encodeSlot("PRESENT_ALIAS", x.Name)
		fmt.Fprintf(b, "  if (( ${+%s} )); then %s %s %s || return $?; fi\n", presence, names.commitUndoSlots, original, presence)
	case activate.RestoreShadowedFunc:
		if !safeAliasFuncName(x.Name) {
			return nil
		}
		original := encodeSlot("ORIGINAL_FUNCTION", x.Name)
		presence := encodeSlot("PRESENT_FUNCTION", x.Name)
		fmt.Fprintf(b, "  if (( ${+%s} )); then %s %s %s || return $?; fi\n", presence, names.commitUndoSlots, original, presence)
	case activate.RestoreOption:
		if !safeOptionName(x.Name) {
			return nil
		}
		slot := encodeSlot("WAS_ON_OPTION", x.Name)
		fmt.Fprintf(b, "  if (( ${+%s} )); then %s %s || return $?; fi\n", slot, names.commitUndoSlots, slot)
	case activate.RebuildListFromBase, activate.Unalias, activate.UnsetFunc:
		return nil
	default:
		return fmt.Errorf("deactivate: unsupported operation %T", op)
	}
	return nil
}

func emitScalarCall(b *strings.Builder, helper, name, applied string, dynamic bool, names emitNames) {
	args := []string{
		helper,
		name,
		renderValue(applied, false),
		encodeSlot("ORIGINAL_SCALAR", name),
		encodeSlot("PRESENT_SCALAR", name),
		encodeSlot("EXPORTED_SCALAR", name),
	}
	if names.captureApplied || dynamic {
		args = append(args, encodeSlot("APPLIED_SCALAR", name))
	}
	fmt.Fprintf(b, "  %s || return $?\n", strings.Join(args, " "))
}

func emitScalarCommit(b *strings.Builder, name string, dynamic bool, names emitNames) {
	args := []string{
		names.commitScalar,
		encodeSlot("ORIGINAL_SCALAR", name),
		encodeSlot("PRESENT_SCALAR", name),
		encodeSlot("EXPORTED_SCALAR", name),
	}
	if names.captureApplied || dynamic {
		args = append(args, encodeSlot("APPLIED_SCALAR", name))
	}
	fmt.Fprintf(b, "  %s || return $?\n", strings.Join(args, " "))
}

// Reserved future element-removal form: when ListDelta.Deletions is activated,
// use [[ $e == "$target" ]] for literal equality. The current v2.0 deactivate
// path is a full rebuild-to-base and intentionally emits no removal loop.
// (Never use ${path:#pattern} or an unquoted == $target comparison.)
