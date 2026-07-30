package zsh

import (
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"

	"zsh-pro/core/activate"
)

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
zp_restore_scalar() {
  local var="$1" applied="$2" originalSlot="$3" presenceSlot="$4" exportSlot="$5" appliedSlot="$6"
  if [[ "${(P)+appliedSlot}" == 1 ]]; then applied="${(P)appliedSlot}"; fi
  if [[ "${(P)+var}" == 1 && "${(P)var}" == "$applied" ]]; then
    if [[ "${(P)presenceSlot}" == 1 ]]; then
      typeset -g "$var=${(P)originalSlot}"
      if [[ "${(P)exportSlot}" == 1 ]]; then export "$var"; else typeset +x "$var"; fi
    else unset "$var"; fi
  fi
  unset "$originalSlot"
  unset "$presenceSlot"
  unset "$exportSlot"
  unset "$appliedSlot"
}
`

type emitNames struct {
	apply, deactivate            string
	captureScalar, restoreScalar string
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
		apply:          "zp_apply",
		deactivate:     "zp_deactivate",
		captureScalar:  "zp_capture_scalar",
		restoreScalar:  "zp_restore_scalar",
		captureApplied: true,
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
		apply:          applyName,
		deactivate:     deactivateName,
		captureScalar:  "_zp_capture_scalar",
		restoreScalar:  "_zp_restore_scalar",
		captureApplied: false,
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
	for _, op := range p.Deactivate {
		if err := emitDeactivate(&d, op, names); err != nil {
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

func emitDeactivate(b *strings.Builder, op activate.Op, names emitNames) error {
	switch x := op.(type) {
	case activate.RestoreScalar:
		if !safeEnvName(x.Name) {
			return nil
		}
		emitRestoreScalar(b, x.Name, x.Applied, x.Dynamic, names)
	case activate.UnsetScalar:
		if !safeEnvName(x.Name) {
			return nil
		}
		emitRestoreScalar(b, x.Name, x.Applied, x.Dynamic, names)
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
		fmt.Fprintf(b, "  unalias %s 2>/dev/null\n", x.Name)
	case activate.RestoreShadowedAlias:
		if !safeAliasFuncName(x.Name) {
			return nil
		}
		original := encodeSlot("ORIGINAL_ALIAS", x.Name)
		presence := encodeSlot("PRESENT_ALIAS", x.Name)
		fmt.Fprintf(b, "  if (( ${+%s} )); then if (( %s )); then alias %s=\"${%s}\"; else unalias %s 2>/dev/null; fi; unset %s; unset %s; fi\n", presence, presence, x.Name, original, x.Name, original, presence)
	case activate.UnsetFunc:
		if !safeAliasFuncName(x.Name) {
			return nil
		}
		fmt.Fprintf(b, "  unset -f %s 2>/dev/null\n", x.Name)
	case activate.RestoreShadowedFunc:
		if !safeAliasFuncName(x.Name) {
			return nil
		}
		original := encodeSlot("ORIGINAL_FUNCTION", x.Name)
		presence := encodeSlot("PRESENT_FUNCTION", x.Name)
		fmt.Fprintf(b, "  if (( ${+%s} )); then if (( %s )); then functions[%s]=\"${%s}\"; else unset -f %s 2>/dev/null; fi; unset %s; unset %s; fi\n", presence, presence, x.Name, original, x.Name, original, presence)
	case activate.RestoreOption:
		if !safeOptionName(x.Name) {
			return nil
		}
		slot := encodeSlot("WAS_ON_OPTION", x.Name)
		fmt.Fprintf(b, "  if (( ${+%s} )); then if (( %s )); then setopt %s; else unsetopt %s; fi; unset %s; fi\n", slot, slot, x.Name, x.Name, slot)
	default:
		return fmt.Errorf("deactivate: unsupported operation %T", op)
	}
	return nil
}

func emitRestoreScalar(b *strings.Builder, name, applied string, dynamic bool, names emitNames) {
	args := []string{
		names.restoreScalar,
		name,
		renderValue(applied, false),
		encodeSlot("ORIGINAL_SCALAR", name),
		encodeSlot("PRESENT_SCALAR", name),
		encodeSlot("EXPORTED_SCALAR", name),
	}
	if names.captureApplied || dynamic {
		args = append(args, encodeSlot("APPLIED_SCALAR", name))
	}
	fmt.Fprintf(b, "  %s\n", strings.Join(args, " "))
}

// Reserved future element-removal form: when ListDelta.Deletions is activated,
// use [[ $e == "$target" ]] for literal equality. The current v2.0 deactivate
// path is a full rebuild-to-base and intentionally emits no removal loop.
// (Never use ${path:#pattern} or an unquoted == $target comparison.)
