package testgen

import (
	"fmt"
	"math/rand"

	"zsh-pro/core/model"
)

// Name pools — disjoint per kind so classification is deterministic and the
// oracle never mis-predicts a category. Env names contain neither "PATH" nor a
// secret-regex token; path dirs are rooted so the analyzer's pathSegRe extracts
// each whole. Pools must be large enough for base + planted-defect draws.
var (
	envNames    = []string{"EDITOR", "PAGER", "LANG", "LESS", "TERM", "COLORTERM", "TMPDIR", "LSCOLORS", "MANWIDTH", "GREP_OPTIONS"}
	aliasNames  = []string{"ll", "la", "gs", "gd", "gco", "kc", "tf", "dco", "vi", "rl"}
	funcNames   = []string{"mkcd", "extract", "backup", "up", "ports", "serve", "gclone", "tre"}
	pathDirs    = []string{"/usr/local/bin", "/opt/bin", "/opt/homebrew/bin", "$HOME/.local/bin", "$HOME/go/bin", "/snap/bin", "$HOME/.cargo/bin"}
	secretNames = []string{"GITHUB_TOKEN", "AWS_SECRET_ACCESS_KEY", "API_KEY", "NPM_TOKEN", "CLIENT_SECRET", "AUTH_TOKEN"}
)

// commandTmpl is one renderable command with its known category.
type commandTmpl struct {
	word string
	args string
	cat  model.Category
}

var commandTmpls = []commandTmpl{
	{"bindkey", `"^X^E" edit-command-line`, model.CatKeybindings},
	{"setopt", "EXTENDED_GLOB", model.CatOptions},
	{"source", "/etc/zsh/zshrc.local", model.CatPlugins},
}

// Generator builds random config graphs from an injected PRNG.
type Generator struct{ rng *rand.Rand }

// New returns a Generator driven by rng.
func New(rng *rand.Rand) *Generator { return &Generator{rng: rng} }

// GenParams says how many of each node type to create and how many defects to
// plant. Defect counts must fit within the relevant pool (caller's responsibility).
type GenParams struct {
	EnvVars, Aliases, Functions, PathEntries, Commands    int
	DupAliases, ReassignedEnv, DupPaths, Shadows, Secrets int
}

// pick shuffles a copy of pool and returns its first n names (deterministic).
func (gen *Generator) pick(pool []string, n int) []string {
	cp := append([]string(nil), pool...)
	gen.rng.Shuffle(len(cp), func(i, j int) { cp[i], cp[j] = cp[j], cp[i] })
	return cp[:n]
}

// Build constructs a graph: clean base nodes from disjoint pools, then planted
// defects on fresh, non-overlapping names.
func (gen *Generator) Build(p GenParams) *ConfigGraph {
	g := &ConfigGraph{}

	// Partition each pool into clean-base names and defect names up front so the
	// two never overlap.
	aliasN := gen.pick(aliasNames, p.Aliases+p.DupAliases+p.Shadows)
	baseAlias, dupAlias, shadowName := aliasN[:p.Aliases], aliasN[p.Aliases:p.Aliases+p.DupAliases], aliasN[p.Aliases+p.DupAliases:]
	envN := gen.pick(envNames, p.EnvVars+p.ReassignedEnv)
	baseEnv, dupEnv := envN[:p.EnvVars], envN[p.EnvVars:]
	pathN := gen.pick(pathDirs, p.PathEntries+p.DupPaths)
	basePath, dupPath := pathN[:p.PathEntries], pathN[p.PathEntries:]
	funcN := gen.pick(funcNames, p.Functions)
	secretN := gen.pick(secretNames, p.Secrets)

	for _, name := range baseEnv {
		g.Add(&Node{Kind: NodeEnvVar, Name: name, Value: "1", Cat: model.CatEnvironment})
	}
	for _, name := range baseAlias {
		g.Add(&Node{Kind: NodeAlias, Name: name, Value: "echo " + name, Cat: model.CatAliases})
	}
	for _, name := range funcN {
		g.Add(&Node{Kind: NodeFunction, Name: name, Value: "echo " + name, Cat: model.CatFunctions})
	}
	for _, dir := range basePath {
		g.Add(&Node{Kind: NodePathEntry, Name: dir, Cat: model.CatPath})
	}
	for i := 0; i < p.Commands; i++ {
		t := commandTmpls[i%len(commandTmpls)]
		g.Add(&Node{Kind: NodeCommand, Name: t.word, Value: t.args, Cat: t.cat})
	}
	for _, name := range secretN {
		g.Add(&Node{Kind: NodeSecret, Name: name, Value: "xxxxxxxx", Cat: model.CatSecrets})
	}

	// Planted defects (fresh names, distinct from base). The first node of each
	// dup pair carries a leading comment so the rendered source places a
	// `# comment` line directly above the first occurrence — this exercises the
	// LINE-02 statement-vs-comment line fix for duplicate_alias and
	// reassigned_env, not only shadowed.
	for _, name := range dupAlias {
		g.Add(&Node{Kind: NodeAlias, Name: name, Value: "echo first", Cat: model.CatAliases, Comment: fmt.Sprintf("first %s", name)})
		g.Add(&Node{Kind: NodeAlias, Name: name, Value: "echo second", Cat: model.CatAliases})
	}
	for _, name := range dupEnv {
		g.Add(&Node{Kind: NodeEnvVar, Name: name, Value: "first", Cat: model.CatEnvironment, Comment: fmt.Sprintf("first %s", name)})
		g.Add(&Node{Kind: NodeEnvVar, Name: name, Value: "second", Cat: model.CatEnvironment})
	}
	for _, dir := range dupPath {
		g.Add(&Node{Kind: NodePathEntry, Name: dir, Cat: model.CatPath})
		g.Add(&Node{Kind: NodePathEntry, Name: dir, Cat: model.CatPath})
	}
	for _, name := range shadowName {
		g.Add(&Node{Kind: NodeAlias, Name: name, Value: "echo alias", Cat: model.CatAliases, Comment: fmt.Sprintf("%s alias", name)})
		g.Add(&Node{Kind: NodeFunction, Name: name, Value: "echo func", Cat: model.CatFunctions})
	}
	return g
}
