package zsh

import (
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
	base := "ZP_BASE_" + sanitizeSlot(name)
	array := "path"
	if strings.EqualFold(name, "FPATH") {
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

func sanitizeSlot(s string) string {
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	if b.Len() == 0 {
		return "x"
	}
	return b.String()
}

func safeEnvName(s string) bool       { return envNameRe.MatchString(s) }
func safeOptionName(s string) bool    { return optionNameRe.MatchString(s) }
func safeAliasFuncName(s string) bool { return aliasFuncNameRe.MatchString(s) }

const runtimeHelpers = `
# These helpers intentionally remain plain functions: option changes made by
# the apply loader must persist after the function returns.
zp_capture_env() {
  local var="$1" slot="__ZP_ORIG_$1"
  if [[ "${(P)+slot}" == 1 ]]; then return 0; fi
  if [[ "${(P)+var}" == 1 ]]; then
    typeset -g "$slot=${(P)var}"
  else
    typeset -g "$slot=__ZP_UNSET__"
  fi
}
zp_restore_env() {
  local var="$1" applied="$2" slot="__ZP_ORIG_$1" appliedSlot="__ZP_APPLIED_$1" prior
  if [[ "${(P)+appliedSlot}" == 1 ]]; then applied="${(P)appliedSlot}"; fi
  if [[ "${(P)+var}" == 1 && "${(P)var}" == "$applied" ]]; then
    if [[ "${(P)+slot}" != 1 ]]; then return 0; fi
    prior="${(P)slot}"
    if [[ "$prior" == __ZP_UNSET__ ]]; then unset "$var"
    else export "$var"="$prior"; fi
  fi
  unset "$slot"
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
		fmt.Fprintf(b, "  zp_capture_env %s\n  export %s=%s\n", x.Name, x.Name, renderValue(x.Applied, x.Dynamic))
		slot := "__ZP_APPLIED_" + sanitizeSlot(x.Name)
		fmt.Fprintf(b, "  if (( ! ${+%s} )); then typeset -g %s=\"$%s\"; fi\n", slot, slot, x.Name)
	case activate.ApplyListDelta:
		if !safeEnvName(x.Name) {
			return nil
		}
		fmt.Fprintf(b, "  %s\n", renderList(x.Name, x.Additions, false))
	case activate.AddAlias:
		if !safeAliasFuncName(x.Name) {
			return nil
		}
		slot := "ZP_PRIOR_ALIAS_" + sanitizeSlot(x.Name)
		fmt.Fprintf(b, "  if (( ! ${+%s} )); then if (( ${+aliases[%s]} )); then typeset -g \"%s=${aliases[%s]}\"; else typeset -g %s=__ZP_UNSET__; fi; fi\n", slot, x.Name, slot, x.Name, slot)
		fmt.Fprintf(b, "  alias %s=%s\n", x.Name, renderValue(x.Body, x.Dynamic))
	case activate.AddFunc:
		if !safeAliasFuncName(x.Name) {
			return nil
		}
		slot := "ZP_PRIOR_FUNC_" + sanitizeSlot(x.Name)
		fmt.Fprintf(b, "  if (( ! ${+%s} )); then if (( ${+functions[%s]} )); then typeset -g \"%s=${functions[%s]}\"; else typeset -g %s=__ZP_UNSET__; fi; fi\n", slot, x.Name, slot, x.Name, slot)
		if x.Body != "" {
			fmt.Fprintf(b, "  functions[%s]=%s\n", x.Name, quoteFunctionBody(x.Body))
		}
	case activate.SetOption:
		if !safeOptionName(x.Name) {
			return nil
		}
		slot := "ZP_WAS_ON_" + sanitizeSlot(x.Name)
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
		fmt.Fprintf(b, "  zp_restore_env %s %s\n", x.Name, renderValue(x.Applied, false))
	case activate.UnsetScalar:
		if !safeEnvName(x.Name) {
			return nil
		}
		fmt.Fprintf(b, "  zp_restore_env %s %s\n", x.Name, renderValue(x.Applied, false))
	case activate.RebuildListFromBase:
		if !safeEnvName(x.Name) {
			return nil
		}
		base := "ZP_BASE_" + sanitizeSlot(x.Name)
		if strings.EqualFold(x.Name, "PATH") {
			base = "ZP_BASE_PATH"
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
		slot := "ZP_PRIOR_ALIAS_" + sanitizeSlot(x.Name)
		fmt.Fprintf(b, "  if (( ${+%s} )); then if [[ \"${%s}\" == __ZP_UNSET__ ]]; then unalias %s 2>/dev/null; else alias %s=\"${%s}\"; fi; unset %s; fi\n", slot, slot, x.Name, x.Name, slot, slot)
	case activate.UnsetFunc:
		if !safeAliasFuncName(x.Name) {
			return nil
		}
		fmt.Fprintf(b, "  unset -f %s 2>/dev/null\n", x.Name)
	case activate.RestoreShadowedFunc:
		if !safeAliasFuncName(x.Name) {
			return nil
		}
		slot := "ZP_PRIOR_FUNC_" + sanitizeSlot(x.Name)
		fmt.Fprintf(b, "  if (( ${+%s} )); then if [[ \"${%s}\" == __ZP_UNSET__ ]]; then unset -f %s 2>/dev/null; else functions[%s]=\"${%s}\"; fi; unset %s; fi\n", slot, slot, x.Name, x.Name, slot, slot)
	case activate.RestoreOption:
		if !safeOptionName(x.Name) {
			return nil
		}
		slot := "ZP_WAS_ON_" + sanitizeSlot(x.Name)
		fmt.Fprintf(b, "  if (( ${+%s} )); then if (( %s )); then setopt %s; else unsetopt %s; fi; unset %s; fi\n", slot, slot, x.Name, x.Name, slot)
	default:
		return fmt.Errorf("deactivate: unsupported operation %T", op)
	}
	return nil
}

// Reserved future element-removal form: when ListDelta.Deletions is activated,
// use [[ $e == "$target" ]] for literal equality. The current v2.0 deactivate
// path is a full rebuild-to-base and intentionally emits no removal loop.
// (Never use ${path:#pattern} or an unquoted == $target comparison.)
