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

// Emit renders one plan into two source blocks. The blocks define zp_apply and
// zp_deactivate and can be sourced by a future loader in the current shell.
func (Provider) Emit(p activate.Plan) (apply, deactivate string, err error) {
	var a, d strings.Builder
	a.WriteString(runtimeHelpers)
	a.WriteString("\nzp_apply() {\n")
	for _, op := range p.Activate {
		if err := emitActivate(&a, op); err != nil {
			return "", "", err
		}
	}
	a.WriteString("}\n")

	d.WriteString(runtimeHelpers)
	d.WriteString("\nzp_deactivate() {\n")
	for _, op := range p.Deactivate {
		if err := emitDeactivate(&d, op); err != nil {
			return "", "", err
		}
	}
	d.WriteString("}\n")
	return a.String(), d.String(), nil
}

func emitActivate(b *strings.Builder, op activate.Op) error {
	switch x := op.(type) {
	case activate.SetScalar:
		if !safeEnvName(x.Name) {
			return nil
		}
		original := encodeSlot("ORIGINAL_SCALAR", x.Name)
		presence := encodeSlot("PRESENT_SCALAR", x.Name)
		exported := encodeSlot("EXPORTED_SCALAR", x.Name)
		applied := encodeSlot("APPLIED_SCALAR", x.Name)
		fmt.Fprintf(b, "  zp_capture_scalar %s %s %s %s\n", x.Name, original, presence, exported)
		if x.Exported {
			fmt.Fprintf(b, "  export %s=%s\n", x.Name, renderValue(x.Applied, x.Dynamic))
		} else {
			fmt.Fprintf(b, "  typeset -g %s=%s\n", x.Name, renderValue(x.Applied, x.Dynamic))
		}
		fmt.Fprintf(b, "  typeset -g %s=\"$%s\"\n", applied, x.Name)
	case activate.ApplyListDelta:
		if !safeEnvName(x.Name) {
			return nil
		}
		fmt.Fprintf(b, "  %s\n", renderList(x.Name, x.Additions, false))
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

func emitDeactivate(b *strings.Builder, op activate.Op) error {
	switch x := op.(type) {
	case activate.RestoreScalar:
		if !safeEnvName(x.Name) {
			return nil
		}
		emitRestoreScalar(b, x.Name, x.Applied)
	case activate.UnsetScalar:
		if !safeEnvName(x.Name) {
			return nil
		}
		emitRestoreScalar(b, x.Name, x.Applied)
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
		fmt.Fprintf(b, "  %s=\"$%s\"\n", x.Name, base)
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

func emitRestoreScalar(b *strings.Builder, name, applied string) {
	fmt.Fprintf(b, "  zp_restore_scalar %s %s %s %s %s %s\n", name, renderValue(applied, false), encodeSlot("ORIGINAL_SCALAR", name), encodeSlot("PRESENT_SCALAR", name), encodeSlot("EXPORTED_SCALAR", name), encodeSlot("APPLIED_SCALAR", name))
}

// Reserved future element-removal form: when ListDelta.Deletions is activated,
// use [[ $e == "$target" ]] for literal equality. The current v2.0 deactivate
// path is a full rebuild-to-base and intentionally emits no removal loop.
// (Never use ${path:#pattern} or an unquoted == $target comparison.)
