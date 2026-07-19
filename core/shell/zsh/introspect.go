package zsh

import (
	"bytes"
	"context"
	"os/exec"
	"strings"
	"time"

	"zsh-pro/core/buildinfo"
	"zsh-pro/core/model"
	"zsh-pro/core/shell"
)

var _ shell.Provider = Provider{}

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
print -r -- '##ALIASBODIES##'
for k in "${(@ok)aliases}"; do print -rN -- "$k" "${aliases[$k]}"; done
print -r -- '##FUNCTIONBODIES##'
for k in "${(@ok)functions}"; do print -rN -- "$k" "${functions[$k]}"; done
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
	bodyAt := strings.Index(s, "##ALIASBODIES##\n")
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

// parseBodies reads NUL-framed name/body pairs. The section boundary is a NUL
// followed by the next newline-framed sentinel, so body lines beginning ## are safe.
func parseBodies(s, start, end string, dst map[string]string) {
	startAt := strings.Index(s, start)
	if startAt < 0 {
		return
	}
	startAt = strings.IndexByte(s[startAt:], '\n')
	if startAt < 0 {
		return
	}
	startAt += strings.Index(s, start)
	data := s[startAt+1:]
	marker := "\x00" + end
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
