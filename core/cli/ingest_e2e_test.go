package cli

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"zsh-pro/core/activate"
	"zsh-pro/core/ir"
	"zsh-pro/core/model"
	"zsh-pro/core/shell/zsh"
	storepkg "zsh-pro/core/store"
)

const (
	ingestE2EPlaceholder = "__PHASE6_LITERAL_SECRET__"
	ingestE2ESecretName  = "PHASE6_API_TOKEN"
	ingestE2EResolved    = "phase6-runtime-resolution-canary"
)

type ingestE2EBackend struct {
	values                     map[string]string
	kindCalls, storeCalls      int
	retrieveCalls, deleteCalls int
}

func newIngestE2EBackend() *ingestE2EBackend {
	return &ingestE2EBackend{values: map[string]string{}}
}

func (b *ingestE2EBackend) Kind() model.SecretRefKind {
	b.kindCalls++
	return model.SecretRefFile
}
func (b *ingestE2EBackend) Store(key, value string) error {
	b.storeCalls++
	b.values[key] = value
	return nil
}
func (b *ingestE2EBackend) Retrieve(key string) (string, error) {
	b.retrieveCalls++
	value, ok := b.values[key]
	if !ok {
		return "", storepkg.ErrSecretNotFound
	}
	return value, nil
}
func (b *ingestE2EBackend) Delete(key string) error {
	b.deleteCalls++
	delete(b.values, key)
	return nil
}
func (b *ingestE2EBackend) resetCalls() {
	b.kindCalls, b.storeCalls, b.retrieveCalls, b.deleteCalls = 0, 0, 0, 0
}

type ingestE2EProvenance struct {
	label                  string
	startLine, endLine     int
	category               model.Category
	kind                   model.BlockKind
	name                   string
	managed, representable bool
	secret                 bool
}

// This table is deliberately authored from real.zshrc line spans. It is used
// to validate the source before Parse/Build/Store are invoked, and is never
// populated from production output.
var ingestE2EProvenanceTable = []ingestE2EProvenance{
	{"order-ledger-definition", 1, 3, model.CatEnvironment, model.KindAssignment, "PHASE6_ORDER_LEDGER", false, false, false},
	{"first-export", 4, 4, model.CatEnvironment, model.KindAssignment, "PHASE6_FIRST", true, true, false},
	{"order-ledger-append", 5, 5, model.CatEnvironment, model.KindAssignment, "PHASE6_ORDER_LEDGER", false, false, false},
	{"alias", 7, 7, model.CatAliases, model.KindAlias, "phase6_alias", true, true, false},
	{"function-definition", 9, 11, model.CatFunctions, model.KindFuncDecl, "phase6_function", true, true, false},
	{"function-call", 12, 12, model.CatMisc, model.KindCommand, "", false, false, false},
	{"path", 14, 14, model.CatPath, model.KindAssignment, "PATH", true, true, false},
	{"option", 15, 15, model.CatOptions, model.KindCommand, "HIST_IGNORE_DUPS", true, true, false},
	{"opaque-array", 17, 17, model.CatEnvironment, model.KindAssignment, "PHASE6_OPAQUE", true, false, false},
	{"forced-unmanaged", 18, 18, model.CatEnvironment, model.KindAssignment, "PHASE6_OPAQUE_RESULT", false, false, false},
	{"definition-before-use", 20, 23, model.CatMisc, model.KindCompound, "", false, true, false},
	{"literal-secret", 25, 25, model.CatSecrets, model.KindAssignment, ingestE2ESecretName, true, true, true},
	{"dynamic-secret", 26, 26, model.CatSecrets, model.KindAssignment, "PHASE6_DYNAMIC_TOKEN", true, true, false},
	{"source-execution-canary", 28, 31, model.CatMisc, model.KindCompound, "", false, true, false},
	{"subprocess-canary", 33, 36, model.CatMisc, model.KindCompound, "", false, true, false},
}

type ingestE2EFixture struct {
	t                 *testing.T
	home, dataHome    string
	target, storeRoot string
	sourceTemplate    []byte
	expectedTemplate  []byte
	source            []byte
	secret            string
	canary            string
	provider          zsh.Provider
	backend           *ingestE2EBackend
	store             *storepkg.Store
	program           *CLI
	stdout, stderr    bytes.Buffer
	events            []string
	resultCode        int
	profile           model.Profile
}

func ingestE2ERepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("repository root not found")
		}
		dir = parent
	}
}

func newIngestE2EFixture(t *testing.T) *ingestE2EFixture {
	t.Helper()
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("guarded ingest transactions require Linux or Darwin")
	}
	for _, command := range []string{"git", "zsh"} {
		if _, err := exec.LookPath(command); err != nil {
			t.Skipf("%s is required", command)
		}
	}
	root := ingestE2ERepoRoot(t)
	read := func(name string) []byte {
		b, err := os.ReadFile(filepath.Join(root, "core", "cli", "testdata", "ingest", name))
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Count(b, []byte(ingestE2EPlaceholder)) != 1 {
			t.Fatalf("%s must contain exactly one secret placeholder", name)
		}
		return b
	}
	token := make([]byte, 24)
	if _, err := rand.Read(token); err != nil {
		t.Fatal(err)
	}
	secret := "phase6-token-" + hex.EncodeToString(token)
	sourceTemplate := read("real.zshrc")
	expectedTemplate := read("expected-installed.zshrc")
	validateIngestE2EAuthoredSource(t, sourceTemplate)
	home := t.TempDir()
	dataHome := filepath.Join(home, ".local", "share")
	setInstallHome(t, home)
	t.Setenv("XDG_DATA_HOME", dataHome)
	canary := filepath.Join(home, "source-executed")
	t.Setenv("PHASE6_SOURCE_EXECUTION_CANARY", canary)
	t.Setenv("PHASE6_RUN_SUBPROCESS_CANARY", "0")
	target := filepath.Join(home, ".zshrc")
	source := bytes.Replace(sourceTemplate, []byte(ingestE2EPlaceholder), []byte(secret), 1)
	if err := os.WriteFile(target, source, 0o600); err != nil {
		t.Fatal(err)
	}
	backend := newIngestE2EBackend()
	provider := zsh.Provider{}
	storeRoot := filepath.Join(dataHome, "zsh-pro")
	s, err := storepkg.New(storeRoot, provider, backend)
	if err != nil {
		t.Fatal(err)
	}
	initializer := func(ctx context.Context) (StoreInitialization, error) {
		initialization, err := s.InitForInstall(ctx)
		if err != nil {
			return StoreInitialization{}, err
		}
		return StoreInitialization{
			InitializationID: initialization.ID(),
			Transactions:     s,
			CanonicalRoot:    storeRoot,
			CreatedPath:      initialization.CreatedPath(),
			Rollback:         initialization.Rollback,
			Finalize:         initialization.Finalize,
		}, nil
	}
	f := &ingestE2EFixture{
		t: t, home: home, dataHome: dataHome, target: target, storeRoot: storeRoot,
		sourceTemplate: sourceTemplate, expectedTemplate: expectedTemplate,
		source: source, secret: secret, canary: canary, provider: provider,
		backend: backend, store: s,
	}
	f.program = newCLI(provider, s, NewRuntimeEmitter(s, provider, backend), initializer)
	return f
}

func validateIngestE2EAuthoredSource(t *testing.T, source []byte) {
	t.Helper()
	lines := strings.Split(strings.TrimSuffix(string(source), "\n"), "\n")
	if len(lines) != 36 || len(ingestE2EProvenanceTable) != 15 {
		t.Fatalf("authored fixture shape changed: lines=%d entries=%d", len(lines), len(ingestE2EProvenanceTable))
	}
	checks := map[string]string{
		"order-ledger-definition": "typeset -ga PHASE6_ORDER_LEDGER=()",
		"first-export":            "export PHASE6_FIRST='ready'",
		"order-ledger-append":     "PHASE6_ORDER_LEDGER+=('definition')",
		"alias":                   "alias phase6_alias='print -r -- alias-ok'",
		"function-definition":     "phase6_function() {",
		"function-call":           "phase6_function",
		"path":                    "export PATH=\"${PHASE6_STUB_BIN}:/phase6/first:/phase6/second\"",
		"option":                  "setopt HIST_IGNORE_DUPS",
		"opaque-array":            "typeset -gA PHASE6_OPAQUE=(alpha one beta two)",
		"forced-unmanaged":        "typeset -g PHASE6_OPAQUE_RESULT='one-two'",
		"definition-before-use":   "if [[ \"$PHASE6_FIRST\" == 'ready' ]]; then",
		"literal-secret":          "export PHASE6_API_TOKEN='__PHASE6_LITERAL_SECRET__'",
		"dynamic-secret":          "export PHASE6_DYNAMIC_TOKEN=\"${PHASE6_DYNAMIC_SOURCE:-dynamic-fallback}\"",
		"source-execution-canary": "if [[ -n \"${PHASE6_SOURCE_EXECUTION_CANARY-}\" ]]; then",
		"subprocess-canary":       "if [[ \"${PHASE6_RUN_SUBPROCESS_CANARY-0}\" == 1 ]]; then",
	}
	for _, row := range ingestE2EProvenanceTable {
		span := strings.Join(lines[row.startLine-1:row.endLine], "\n")
		if !strings.Contains(span, checks[row.label]) {
			t.Fatalf("authored provenance row %q no longer matches lines %d-%d", row.label, row.startLine, row.endLine)
		}
	}
}

func applyIngestE2EOverrides(profile *model.Profile) {
	for i := range profile.Entries {
		switch firstName(profile.Entries[i]) {
		case "PHASE6_OPAQUE":
			profile.Entries[i].Override = model.OverrideManaged
		case "PHASE6_OPAQUE_RESULT":
			profile.Entries[i].Override = model.OverrideUnmanaged
		}
	}
}

func firstName(entry model.Entry) string {
	if len(entry.Names) == 0 {
		return ""
	}
	return entry.Names[0]
}

func (f *ingestE2EFixture) run(seams *ingestControllerSeams) {
	f.t.Helper()
	if seams == nil {
		seams = &ingestControllerSeams{}
	}
	priorEvent := seams.event
	seams.event = func(event string) {
		f.events = append(f.events, event)
		if priorEvent != nil {
			priorEvent(event)
		}
	}
	priorAfterBuild := seams.afterBuild
	seams.afterBuild = func(profile *model.Profile) {
		applyIngestE2EOverrides(profile)
		if priorAfterBuild != nil {
			priorAfterBuild(profile)
		}
	}
	f.stdout.Reset()
	f.stderr.Reset()
	f.resultCode = f.program.runIngestWithSeams(
		context.Background(),
		ingestArguments{path: f.target, asJSON: true}, &f.stdout, &f.stderr, seams,
	)
}

func (f *ingestE2EFixture) requireSuccess() {
	f.t.Helper()
	f.run(nil)
	if f.resultCode != int(model.ExitClean) || f.stderr.Len() != 0 {
		f.t.Fatalf("ingest failed: code=%d stdout=%q stderr=%q events=%v backend=[kind:%d store:%d retrieve:%d delete:%d]", f.resultCode, redactForFailure(f.stdout.String(), f.secret), redactForFailure(f.stderr.String(), f.secret), f.events, f.backend.kindCalls, f.backend.storeCalls, f.backend.retrieveCalls, f.backend.deleteCalls)
	}
	if bytes.Contains(f.stdout.Bytes(), []byte(f.secret)) {
		f.t.Fatal("ingest output disclosed the runtime secret")
	}
	if _, err := os.Lstat(f.canary); !errors.Is(err, fs.ErrNotExist) {
		f.t.Fatalf("ingest executed source canary: %v", err)
	}
	profile, err := f.store.Read(context.Background(), "main")
	if err != nil {
		f.t.Fatal(err)
	}
	f.profile = profile
	installed, err := os.ReadFile(f.target)
	if err != nil {
		f.t.Fatal(err)
	}
	wantInstalled := bytes.Replace(f.expectedTemplate, []byte(ingestE2EPlaceholder), []byte(f.secret), 1)
	if !bytes.Equal(installed, wantInstalled) {
		f.t.Fatal("installed target differs from independently authored expected bytes")
	}
}

func redactForFailure(value, secret string) string {
	return strings.ReplaceAll(value, secret, "<redacted>")
}

func expectedIngestE2EText(row ingestE2EProvenance, sourceTemplate []byte) string {
	lines := strings.Split(strings.TrimSuffix(string(sourceTemplate), "\n"), "\n")
	text := strings.Join(lines[row.startLine-1:row.endLine], "\n")
	if row.secret {
		return "export " + ingestE2ESecretName + "='<zsh-pro secret file:" + ingestE2ESecretName + ">'"
	}
	return text
}

func expectedIngestE2EStatementLine(row ingestE2EProvenance) int {
	switch row.label {
	case "order-ledger-definition":
		return 3
	case "source-execution-canary":
		return 29
	case "subprocess-canary":
		return 34
	default:
		return row.startLine
	}
}

func requireIngestE2EProfile(t *testing.T, f *ingestE2EFixture) {
	t.Helper()
	if len(f.profile.Entries) != len(ingestE2EProvenanceTable) {
		t.Fatalf("Store.Read entry count=%d want=%d", len(f.profile.Entries), len(ingestE2EProvenanceTable))
	}
	for i, row := range ingestE2EProvenanceTable {
		entry := f.profile.Entries[i]
		if entry.StartLine != expectedIngestE2EStatementLine(row) || entry.Category != row.category || entry.Kind != row.kind ||
			firstName(entry) != row.name || entry.EffectiveManaged() != row.managed || entry.Representable() != row.representable {
			t.Fatalf("entry %d (%s) provenance mismatch: line=%d category=%s kind=%s name=%q effective=%v representable=%v", i, row.label, entry.StartLine, entry.Category, entry.Kind, firstName(entry), entry.EffectiveManaged(), entry.Representable())
		}
		if entry.Text != expectedIngestE2EText(row, f.sourceTemplate) {
			t.Fatalf("entry %d (%s) text differs from authored span", i, row.label)
		}
		if row.secret {
			if entry.Secret == nil || entry.Secret.Kind != model.SecretRefFile || entry.Secret.Key != ingestE2ESecretName ||
				entry.Value != "'<zsh-pro secret file:PHASE6_API_TOKEN>'" || entry.RuntimeValue != nil || entry.ValueMode != model.ValueModeUnsupported {
				t.Fatalf("entry %d secret redaction contract mismatch", i)
			}
		} else if entry.Secret != nil {
			t.Fatalf("entry %d (%s) unexpectedly gained SecretRef authority", i, row.label)
		}
	}
}

func TestIngestE2EStoreReadReturnsCompleteOrderedProfile(t *testing.T) {
	f := newIngestE2EFixture(t)
	f.requireSuccess()
	requireIngestE2EProfile(t, f)
}

func TestIngestE2ERegeneratePreservesNonSecretOrderTextAndSemantics(t *testing.T) {
	f := newIngestE2EFixture(t)
	f.requireSuccess()
	requireIngestE2EProfile(t, f)
	regenerated := ir.Regenerate(f.profile, f.provider)
	var want []byte
	for _, row := range ingestE2EProvenanceTable {
		want = append(want, expectedIngestE2EText(row, f.sourceTemplate)...)
		want = append(want, '\n')
	}
	if !bytes.Equal(regenerated, want) {
		t.Fatalf("regeneration differs from independently authored redacted source\ngot:  %q\nwant: %q", regenerated, want)
	}
	if bytes.Contains(regenerated, []byte(f.secret)) {
		t.Fatal("regeneration disclosed runtime secret")
	}
}

type ingestE2EAuthority struct {
	profile *model.Profile
	index   int
}

func (a ingestE2EAuthority) validate(candidate *model.Entry, source model.Entry) bool {
	if a.profile == nil || a.index < 0 || a.index >= len(a.profile.Entries) || candidate != &a.profile.Entries[a.index] {
		return false
	}
	authoritative := &a.profile.Entries[a.index]
	if authoritative.Secret == nil || authoritative.Secret.Kind != model.SecretRefFile || authoritative.Secret.Key != ingestE2ESecretName ||
		authoritative.Text != "export PHASE6_API_TOKEN='<zsh-pro secret file:PHASE6_API_TOKEN>'" ||
		authoritative.Value != "'<zsh-pro secret file:PHASE6_API_TOKEN>'" {
		return false
	}
	// Only source-reconstructible fields participate. Source Secret, ValueMode,
	// RuntimeValue, and StartLine deliberately confer no authority.
	return source.Category == authoritative.Category && source.Kind == authoritative.Kind &&
		source.CmdName == authoritative.CmdName && reflect.DeepEqual(source.Names, authoritative.Names) &&
		source.Exported == authoritative.Exported && source.Managed == authoritative.Managed &&
		source.Override == authoritative.Override && source.Dynamic == false &&
		source.StructuralFidelityKnown == authoritative.StructuralFidelityKnown &&
		source.Append == authoritative.Append && source.Array == authoritative.Array &&
		source.Flagged == authoritative.Flagged && source.Indexed == authoritative.Indexed &&
		source.AliasAssignment == authoritative.AliasAssignment &&
		reflect.DeepEqual(source.DeclarationFlags, authoritative.DeclarationFlags) &&
		reflect.DeepEqual(source.OptionFlags, authoritative.OptionFlags)
}

func TestIngestE2ESecretRefComparatorDoesNotForgeAuthority(t *testing.T) {
	f := newIngestE2EFixture(t)
	f.requireSuccess()
	index := -1
	for i := range f.profile.Entries {
		if firstName(f.profile.Entries[i]) == ingestE2ESecretName {
			index = i
			break
		}
	}
	if index < 0 {
		t.Fatal("authoritative Store.Read secret entry absent")
	}
	blocks, err := f.provider.Parse(f.source)
	if err != nil {
		t.Fatal(err)
	}
	sourceProfile := ir.Build(blocks, f.provider)
	applyIngestE2EOverrides(&sourceProfile)
	if index >= len(sourceProfile.Entries) {
		t.Fatalf("reparsed source profile has %d entries; authoritative index %d is unavailable", len(sourceProfile.Entries), index)
	}
	source := sourceProfile.Entries[index]
	source.StartLine = -100
	source.ValueMode = model.ValueModeUnsupported
	source.RuntimeValue = nil
	source.Secret = &model.SecretRef{Kind: model.SecretRefKeychain, Key: "FORGED"}
	authority := ingestE2EAuthority{profile: &f.profile, index: index}
	if !authority.validate(&f.profile.Entries[index], source) {
		t.Fatal("bounded comparator rejected authoritative Store.Read entry")
	}
	forged := f.profile.Entries[index]
	if authority.validate(&forged, source) {
		t.Fatal("bounded comparator granted authority to a forged clone")
	}
}

func TestIngestE2EActivationUsesOnlyEffectiveManagedEntries(t *testing.T) {
	f := newIngestE2EFixture(t)
	f.requireSuccess()
	resolver := newIngestE2EBackend()
	resolver.values[ingestE2ESecretName] = ingestE2EResolved
	resolved, err := resolveSecretRefs(f.profile, resolver)
	if err != nil {
		t.Fatal(err)
	}
	manifest := activate.Build(resolved)
	gotEnv := map[string]string{}
	for _, scalar := range manifest.Env {
		gotEnv[scalar.Name] = scalar.Applied
	}
	for name, want := range map[string]string{
		"PHASE6_FIRST": "ready", ingestE2ESecretName: ingestE2EResolved,
		"PHASE6_DYNAMIC_TOKEN": "\"${PHASE6_DYNAMIC_SOURCE:-dynamic-fallback}\"",
	} {
		if gotEnv[name] != want {
			t.Fatalf("managed environment %s=%q want %q", name, gotEnv[name], want)
		}
	}
	for _, forbidden := range []string{"PHASE6_ORDER_LEDGER", "PHASE6_OPAQUE", "PHASE6_OPAQUE_RESULT", "PHASE6_IMPERATIVE_RESULT"} {
		if _, ok := gotEnv[forbidden]; ok {
			t.Fatalf("unmanaged/unrepresentable %s reached activation", forbidden)
		}
	}
	if len(manifest.Lists) != 0 || manifest.Aliases.Added["phase6_alias"] != "print -r -- alias-ok" ||
		!reflect.DeepEqual(manifest.Functions.Added, []string{"phase6_function"}) || len(manifest.Options) != 1 ||
		manifest.Options[0].Name != "HIST_IGNORE_DUPS" || !manifest.Options[0].Enabled {
		t.Fatalf("managed activation projection mismatch: %#v", manifest)
	}
}

func TestIngestE2EActivationResolvesSecretRefs(t *testing.T) {
	f := newIngestE2EFixture(t)
	f.requireSuccess()
	resolver := newIngestE2EBackend()
	resolver.values[ingestE2ESecretName] = ingestE2EResolved
	resolver.resetCalls()
	emitted, err := NewRuntimeEmitter(f.store, f.provider, resolver).Emit(context.Background(), "apply", "main")
	if err != nil {
		t.Fatal(err)
	}
	if resolver.kindCalls != 1 || resolver.retrieveCalls != 1 || resolver.storeCalls != 0 || resolver.deleteCalls != 0 {
		t.Fatalf("resolver calls kind=%d retrieve=%d store=%d delete=%d", resolver.kindCalls, resolver.retrieveCalls, resolver.storeCalls, resolver.deleteCalls)
	}
	for _, want := range []string{"PHASE6_FIRST", ingestE2ESecretName, ingestE2EResolved, "phase6_alias", "phase6_function", "HIST_IGNORE_DUPS"} {
		if !strings.Contains(emitted, want) {
			t.Fatalf("runtime emitter omitted managed symbol %q", want)
		}
	}
	for _, forbidden := range []string{f.secret, "PHASE6_OPAQUE", "PHASE6_OPAQUE_RESULT", "PHASE6_SOURCE_EXECUTION_CANARY", "phase6-subprocess-canary"} {
		if strings.Contains(emitted, forbidden) {
			t.Fatalf("runtime emitter included forbidden symbol/value %q", forbidden)
		}
	}
}

func TestIngestE2EUnmanagedExecutionCanariesAreInert(t *testing.T) {
	f := newIngestE2EFixture(t)
	f.requireSuccess()
	resolver := newIngestE2EBackend()
	resolver.values[ingestE2ESecretName] = ingestE2EResolved
	resolved, err := resolveSecretRefs(f.profile, resolver)
	if err != nil {
		t.Fatal(err)
	}
	manifest := activate.Build(resolved)
	plan, err := activate.Diff(nil, &manifest)
	if err != nil {
		t.Fatal(err)
	}
	apply, _, err := f.provider.Emit(plan)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"PHASE6_SOURCE_EXECUTION_CANARY", "PHASE6_RUN_SUBPROCESS_CANARY", "phase6-subprocess-canary", "PHASE6_OPAQUE", "PHASE6_IMPERATIVE_RESULT"} {
		if strings.Contains(apply, forbidden) {
			t.Fatalf("unmanaged execution canary reached generated apply source: %s", forbidden)
		}
	}
}

func runGit(t *testing.T, env []string, input []byte, args ...string) []byte {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Env = append(os.Environ(), env...)
	cmd.Stdin = bytes.NewReader(input)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git command failed: %v", err)
	}
	return out
}

func allGitObjects(t *testing.T, gitDir string, extraEnv ...string) []byte {
	t.Helper()
	return runGit(t, extraEnv, nil, "--git-dir", gitDir, "cat-file", "--batch-all-objects", "--batch")
}

func TestIngestE2ESecretAllObjectDatabases(t *testing.T) {
	f := newIngestE2EFixture(t)
	f.requireSuccess()
	unreachable := []byte("phase6-unreachable-object-canary")
	objectID := strings.TrimSpace(string(runGit(t, nil, unreachable, "--git-dir", f.storeRoot, "hash-object", "-w", "--stdin")))
	runGit(t, nil, nil, "--git-dir", f.storeRoot, "gc", "--prune=never")
	objects := allGitObjects(t, f.storeRoot)
	if !bytes.Contains(objects, unreachable) || !bytes.Contains(objects, []byte(objectID)) {
		t.Fatal("all-object scan did not cover authored unreachable object")
	}
	if bytes.Contains(objects, []byte(f.secret)) {
		t.Fatal("runtime literal reached a reachable, unreachable, or packed Git object")
	}
	requireLiteralOnlyAt(t, f.secret, f.target, f.home)
	if bytes.Contains(f.stdout.Bytes(), []byte(f.secret)) || bytes.Contains(f.stderr.Bytes(), []byte(f.secret)) {
		t.Fatal("captured ingest output disclosed runtime literal")
	}
}

func requireLiteralOnlyAt(t *testing.T, literal, allowed string, roots ...string) {
	t.Helper()
	allowed, _ = filepath.Abs(allowed)
	var matches []string
	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if !entry.Type().IsRegular() {
				return nil
			}
			b, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if bytes.Contains(b, []byte(literal)) {
				absolute, _ := filepath.Abs(path)
				matches = append(matches, absolute)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if !reflect.DeepEqual(matches, []string{allowed}) {
		t.Fatalf("runtime literal paths=%v want only installed target", matches)
	}
}

// Remaining selectors are implemented below; keeping them in this file makes
// the full acceptance matrix independently runnable with one exact regex.
func TestIngestE2ESecretAbsentAfterUpdateRefObservationAmbiguity(t *testing.T) {
	f := newIngestE2EFixture(t)
	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git is required")
	}
	realGit, err = filepath.Abs(realGit)
	if err != nil {
		t.Fatal(err)
	}
	proxyDir := t.TempDir()
	marker := filepath.Join(proxyDir, "commit-response-lost")
	proxy := filepath.Join(proxyDir, "git")
	testBinary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(testBinary, proxy); err != nil {
		t.Fatal(err)
	}
	originalPath := os.Getenv("PATH")
	t.Setenv(ingestE2EGitProxyModeEnv, "1")
	t.Setenv("PHASE6_REAL_GIT", realGit)
	t.Setenv("PHASE6_AMBIGUITY_MARKER", marker)
	t.Setenv("PATH", proxyDir+string(os.PathListSeparator)+originalPath)
	f.run(nil)
	if f.resultCode != int(model.ExitRuntimeErr) || f.stderr.Len() != 0 {
		t.Fatalf("ambiguity result code=%d stdout=%q stderr=%q", f.resultCode, redactForFailure(f.stdout.String(), f.secret), redactForFailure(f.stderr.String(), f.secret))
	}
	result := decodeOneIngestResult(t, f.stdout.Bytes())
	if !result.RecoveryRequired || !result.StartupInstalled || result.ProfileCommitted {
		t.Fatalf("ambiguity result axes=%#v", result)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("proxy did not reach lost commit response boundary: %v", err)
	}
	if eventIndex(f.events, "store:commit") < 0 || eventIndex(f.events, "filesystem:rollback") < eventIndex(f.events, "store:commit") || eventIndex(f.events, "store:abort") >= 0 {
		t.Fatalf("ambiguity event order=%v", f.events)
	}
	if bytes.Contains(f.stdout.Bytes(), []byte(f.secret)) || bytes.Contains(f.stderr.Bytes(), []byte(f.secret)) {
		t.Fatal("value-free recovery output disclosed runtime literal")
	}
	// Stop proxying before inspecting the durable/retained object databases.
	if err := os.Setenv("PATH", originalPath); err != nil {
		t.Fatal(err)
	}
	mainObjects := allGitObjects(t, f.storeRoot)
	if bytes.Contains(mainObjects, []byte(f.secret)) {
		t.Fatal("ambiguous publication leaked the literal into the final object database")
	}
	storeNamespace := ingestE2EStoreTransactionNamespace(t, f.storeRoot)
	requirePrivateCurrentUserDirectory(t, storeNamespace)
	entries, err := os.ReadDir(storeNamespace)
	if err != nil {
		t.Fatal(err)
	}
	var quarantines []string
	for _, entry := range entries {
		if entry.IsDir() {
			quarantine := filepath.Join(storeNamespace, entry.Name())
			requirePrivateCurrentUserDirectory(t, quarantine)
			quarantines = append(quarantines, quarantine)
		}
	}
	if len(quarantines) == 0 {
		t.Fatal("ambiguity did not retain authenticated Store quarantine")
	}
	scannedQuarantines := 0
	for _, quarantine := range quarantines {
		quarantineObjects := filepath.Join(quarantine, "objects")
		retainedObjects := allGitObjects(t, f.storeRoot,
			"GIT_OBJECT_DIRECTORY="+quarantineObjects,
			"GIT_ALTERNATE_OBJECT_DIRECTORIES="+filepath.Join(f.storeRoot, "objects"))
		scannedQuarantines++
		if !bytes.Contains(retainedObjects, []byte("PHASE6_FIRST")) {
			t.Fatal("retained-quarantine scan did not enumerate candidate objects")
		}
		if bytes.Contains(retainedObjects, []byte(f.secret)) {
			t.Fatal("retained quarantine object database disclosed runtime literal")
		}
	}
	if scannedQuarantines != len(quarantines) {
		t.Fatalf("retained quarantine scans=%d discovered=%d", scannedQuarantines, len(quarantines))
	}
	installNamespace := filepath.Join(f.home, installTransactionNamespaceName)
	requirePrivateCurrentUserDirectory(t, installNamespace)
	matches := literalBearingPaths(t, f.secret, f.home)
	if len(matches) != 2 {
		t.Fatalf("recovery literal confinement paths=%v want target plus one exchange peer", matches)
	}
	targetAbs, _ := filepath.Abs(f.target)
	peer := ""
	for _, match := range matches {
		if match != targetAbs {
			peer = match
		}
	}
	if peer == "" || filepath.Base(peer) != exchangePeerBasename || filepath.Dir(filepath.Dir(peer)) != installNamespace {
		t.Fatalf("recovery literal peer is not the authenticated exchange peer: %v", matches)
	}
	requirePrivateCurrentUserDirectory(t, filepath.Dir(peer))
}

func ingestE2EStoreTransactionNamespace(t *testing.T, storeRoot string) string {
	t.Helper()
	canonical, err := filepath.EvalSymlinks(storeRoot)
	if err != nil {
		t.Fatal(err)
	}
	canonical, err = filepath.Abs(canonical)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte(filepath.Clean(canonical)))
	return filepath.Join(filepath.Dir(canonical), ".zsh-pro-transactions-"+hex.EncodeToString(digest[:16]))
}

func literalBearingPaths(t *testing.T, literal string, roots ...string) []string {
	t.Helper()
	var matches []string
	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if !entry.Type().IsRegular() {
				return nil
			}
			contents, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if bytes.Contains(contents, []byte(literal)) {
				absolute, _ := filepath.Abs(path)
				matches = append(matches, absolute)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	return matches
}

func TestIngestE2EProgrammaticSecretRefPassThroughKindOnly(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("guarded ingest transactions require Linux or Darwin")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is required")
	}
	backend := newIngestE2EBackend()
	provider := zsh.Provider{}
	storeRoot := filepath.Join(t.TempDir(), "store")
	s, err := storepkg.New(storeRoot, provider, backend)
	if err != nil {
		t.Fatal(err)
	}
	initialization, err := s.InitForInstall(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	begin, err := s.BeginIngest(context.Background(), initialization.ID())
	if err != nil {
		t.Fatal(err)
	}
	placeholder := "'<zsh-pro secret file:PHASE6_API_TOKEN>'"
	profile := model.Profile{Entries: []model.Entry{{
		Text: "export PHASE6_API_TOKEN=" + placeholder, StartLine: 1,
		Category: model.CatSecrets, Kind: model.KindAssignment, CmdName: "export",
		Names: []string{ingestE2ESecretName}, Value: placeholder, Exported: true,
		Managed: true, Override: model.OverrideAuto, StructuralFidelityKnown: true,
		DeclarationFlags: []string{}, Secret: &model.SecretRef{Kind: model.SecretRefFile, Key: ingestE2ESecretName},
		ValueMode: model.ValueModeUnsupported,
	}}}
	backend.resetCalls()
	outcome, err := s.CommitIngest(context.Background(), initialization.ID(), begin.TransactionID, profile, "zsh-pro: e2e persisted reference")
	if err != nil || outcome.Status != model.IngestCommitCommitted || len(outcome.Withheld) != 0 {
		t.Fatalf("persisted-reference commit status=%s withheld=%d err=%v", outcome.Status, len(outcome.Withheld), err)
	}
	if backend.kindCalls != 1 || backend.retrieveCalls != 0 || backend.storeCalls != 0 || backend.deleteCalls != 0 {
		t.Fatalf("persisted reference calls kind=%d retrieve=%d store=%d delete=%d", backend.kindCalls, backend.retrieveCalls, backend.storeCalls, backend.deleteCalls)
	}
	read, err := s.Read(context.Background(), "main")
	if err != nil || !reflect.DeepEqual(read, profile) {
		t.Fatalf("persisted reference changed across Store: err=%v", err)
	}
	if err := initialization.Finalize(); err != nil {
		t.Fatal(err)
	}
}

func TestIngestE2ESourceLiteralRerunMayRecapture(t *testing.T) {
	f := newIngestE2EFixture(t)
	f.requireSuccess()
	firstStores := f.backend.storeCalls
	parserSawReference := false
	seams := &ingestControllerSeams{afterBuild: func(profile *model.Profile) {
		for i := range profile.Entries {
			if firstName(profile.Entries[i]) == ingestE2ESecretName && profile.Entries[i].Secret != nil {
				parserSawReference = true
			}
		}
	}}
	f.run(seams)
	if f.resultCode != int(model.ExitClean) || f.stderr.Len() != 0 {
		t.Fatalf("literal rerun failed: code=%d stdout=%q", f.resultCode, redactForFailure(f.stdout.String(), f.secret))
	}
	result := decodeOneIngestResult(t, f.stdout.Bytes())
	if parserSawReference || len(result.Withheld) != 1 || result.Withheld[0].Name != ingestE2ESecretName || f.backend.storeCalls <= firstStores {
		t.Fatalf("literal rerun distinction failed: parserRef=%v withheld=%v stores=%d before=%d", parserSawReference, result.Withheld, f.backend.storeCalls, firstStores)
	}
	read, err := f.store.Read(context.Background(), "main")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range read.Entries {
		if firstName(entry) == ingestE2ESecretName {
			if entry.Secret == nil || entry.ValueMode != model.ValueModeUnsupported || strings.Contains(entry.Text, f.secret) {
				t.Fatal("literal rerun did not finish as a redacted authoritative reference")
			}
			return
		}
	}
	t.Fatal("literal rerun omitted secret entry")
}

func TestIngestE2ENeverExecutesSource(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		f := newIngestE2EFixture(t)
		f.requireSuccess()
		if _, err := os.Lstat(f.canary); !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("successful ingest executed source: %v", err)
		}
	})
	t.Run("injected failure", func(t *testing.T) {
		f := newIngestE2EFixture(t)
		f.run(&ingestControllerSeams{beforeCommit: func(*guardedInstallTransaction) error {
			return errors.New("injected pre-commit failure")
		}})
		if f.resultCode != int(model.ExitRuntimeErr) || f.stderr.Len() != 0 {
			t.Fatalf("injected failure result code=%d stderr=%q", f.resultCode, f.stderr.String())
		}
		if _, err := os.Lstat(f.canary); !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("failing ingest executed source: %v", err)
		}
		if bytes.Contains(f.stdout.Bytes(), []byte(f.secret)) {
			t.Fatal("failing ingest output disclosed runtime literal")
		}
		profile, err := f.store.Read(context.Background(), "main")
		if err != nil && !errors.Is(err, storepkg.ErrProfileNotFound) {
			t.Fatalf("injected failure profile read error: %v", err)
		}
		if len(profile.Entries) != 0 {
			t.Fatalf("injected failure committed profile: entries=%d err=%v", len(profile.Entries), err)
		}
	})
}
