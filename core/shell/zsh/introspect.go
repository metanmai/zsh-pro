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
		Aliases:   map[string]bool{},
		Functions: map[string]bool{},
		Env:       map[string]bool{},
		Options:   map[string]bool{},
		Available: true,
	}
	section := ""
	for _, line := range strings.Split(s, "\n") {
		switch line {
		case "##ALIASES##", "##FUNCTIONS##", "##ENV##", "##PATH##", "##OPTIONS##", "##END##":
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
		}
	}
	// v1 scoping: the engine consumes only ids.Available (see analyze.Analyzer.Analyze).
	// The resolved tables (aliases/functions/env/path/options) are captured here
	// for a backlogged enrichment — env-isolated introspection plus opaque-init
	// identity detection — and are intentionally not yet wired into the report.
	return ids
}
