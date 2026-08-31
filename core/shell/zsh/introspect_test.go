package zsh

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"zsh-pro/core/model"
)

type liveFrameTestRecord struct {
	kind      model.LiveKind
	name      string
	attribute string
	values    []string
}

func encodeLiveTestFrame(records ...liveFrameTestRecord) []byte {
	var frame bytes.Buffer
	writeLiveTestFields(&frame, "ZP_LIVE_SNAPSHOT", "1")
	for _, record := range records {
		writeLiveTestFields(&frame, "R", string(record.kind), record.name, record.attribute, strconv.Itoa(len(record.values)))
		writeLiveTestFields(&frame, record.values...)
	}
	writeLiveTestFields(&frame, "E")
	return frame.Bytes()
}

func writeLiveTestFields(frame *bytes.Buffer, fields ...string) {
	for _, field := range fields {
		frame.WriteString(field)
		frame.WriteByte(0)
	}
}

func findLiveTestState(t *testing.T, snapshot model.LiveSnapshot, identity model.Identity) model.LiveValue {
	t.Helper()
	for _, state := range snapshot.States {
		if state.Identity == identity {
			return state.Value
		}
	}
	t.Fatalf("missing live identity %#v", identity)
	return model.LiveValue{}
}

func TestLiveCaptureSourceRoundTripsCurrentShellState(t *testing.T) {
	if _, err := exec.LookPath("zsh"); err != nil {
		t.Skip("zsh not installed")
	}
	functionBody := "\tprint -r -- one\n\tprint -r -- \"##END##\""
	script := (Provider{}).LiveCaptureSource() + `
export ZP_LIVE_EMPTY=''
export ZP_LIVE_MULTI=$'line one\nline two'
aliases[ZP_LIVE_ALIAS]=$'print -r -- "quoted\nbody"'
functions[ZP_LIVE_FUNCTION]=$'print -r -- one\nprint -r -- "##END##"'
path=('/one:colon' '' '/three space')
fpath=('' '/functions:colon')
setopt AUTO_CD
unsetopt NOMATCH
_zp_live_capture
`
	cmd := exec.Command("zsh", "-f")
	cmd.Stdin = strings.NewReader(script)
	frame, err := cmd.Output()
	if err != nil {
		t.Fatalf("live capture failed: %v", err)
	}

	snapshot, err := (Provider{}).DecodeLiveSnapshot(frame)
	if err != nil {
		t.Fatalf("DecodeLiveSnapshot: %v", err)
	}
	if snapshot.ByteSize != uint64(len(frame)) {
		t.Fatalf("ByteSize=%d want %d", snapshot.ByteSize, len(frame))
	}
	checks := []struct {
		identity model.Identity
		want     model.LiveValue
	}{
		{model.Identity{Kind: model.LiveEnv, Name: "ZP_LIVE_EMPTY"}, model.ScalarLiveValue("")},
		{model.Identity{Kind: model.LiveEnv, Name: "ZP_LIVE_MULTI"}, model.ScalarLiveValue("line one\nline two")},
		{model.Identity{Kind: model.LiveAlias, Name: "ZP_LIVE_ALIAS"}, model.ScalarLiveValue("print -r -- \"quoted\nbody\"")},
		{model.Identity{Kind: model.LiveFunction, Name: "ZP_LIVE_FUNCTION"}, model.ScalarLiveValue(functionBody)},
		{model.Identity{Kind: model.LivePath, Name: "PATH"}, model.ListLiveValue([]string{"/one:colon", "", "/three space"})},
		{model.Identity{Kind: model.LiveFPath, Name: "FPATH"}, model.ListLiveValue([]string{"", "/functions:colon"})},
		{model.Identity{Kind: model.LiveOption, Name: "autocd"}, model.OptionLiveValue(true)},
		{model.Identity{Kind: model.LiveOption, Name: "nomatch"}, model.OptionLiveValue(false)},
	}
	for _, check := range checks {
		got := findLiveTestState(t, snapshot, check.identity)
		if !model.EqualLiveValue(check.identity.Kind, got, check.want) {
			if got.Scalar != nil && check.want.Scalar != nil {
				t.Errorf("%#v scalar=%q want %q", check.identity, *got.Scalar, *check.want.Scalar)
				continue
			}
			t.Errorf("%#v=%#v want %#v", check.identity, got, check.want)
		}
	}
}

func TestLiveCaptureSourceSkipsUnsupportedSymbolNamesBeforeBodies(t *testing.T) {
	if _, err := exec.LookPath("zsh"); err != nil {
		t.Skip("zsh not installed")
	}
	const unsupportedCanary = "unsupported-symbol-body-canary"
	const reservedCanary = "reserved-symbol-body-canary"
	const validAliasBody = "print -r -- valid-alias"
	const validFunctionBody = "\tprint -r -- valid-function\n\tprint -r -- second-line"
	script := (Provider{}).LiveCaptureSource() + `
aliases[valid.alias-1]='` + validAliasBody + `'
aliases[_ordinary_user_alias]='print -r -- ordinary-underscore'
aliases[azhw:zle-alias]='` + unsupportedCanary + `-alias'
aliases[-leading]='` + unsupportedCanary + `-leading'
aliases[_zp_private_alias]='` + reservedCanary + `-alias'
functions[valid.function-1]=$'print -r -- valid-function\nprint -r -- second-line'
functions[_ordinary_user_function]='print -r -- ordinary-underscore'
functions[azhw:zle-history-line-set]='print -r -- ` + unsupportedCanary + `-function'
functions[.leading]='print -r -- ` + unsupportedCanary + `-dot'
functions[__zp_worktree_reverse_1_1]='print -r -- ` + reservedCanary + `-reverse'
functions[_zp_private_function]='print -r -- ` + reservedCanary + `-function'
_zp_live_capture
`
	cmd := exec.Command("zsh", "-f")
	cmd.Stdin = strings.NewReader(script)
	frame, err := cmd.Output()
	if err != nil {
		t.Fatalf("live capture failed: %v", err)
	}
	if bytes.Contains(frame, []byte(unsupportedCanary)) || bytes.Contains(frame, []byte(reservedCanary)) ||
		bytes.Contains(frame, []byte("azhw:zle-")) || bytes.Contains(frame, []byte("__zp_worktree_reverse_")) {
		t.Fatal("unsupported or loader-owned symbol name/body entered the live frame")
	}

	snapshot, err := (Provider{}).DecodeLiveSnapshot(frame)
	if err != nil {
		t.Fatalf("DecodeLiveSnapshot: %v", err)
	}
	checks := []struct {
		identity model.Identity
		want     string
	}{
		{model.Identity{Kind: model.LiveAlias, Name: "valid.alias-1"}, validAliasBody},
		{model.Identity{Kind: model.LiveAlias, Name: "_ordinary_user_alias"}, "print -r -- ordinary-underscore"},
		{model.Identity{Kind: model.LiveFunction, Name: "valid.function-1"}, validFunctionBody},
		{model.Identity{Kind: model.LiveFunction, Name: "_ordinary_user_function"}, "\tprint -r -- ordinary-underscore"},
	}
	for _, check := range checks {
		got := findLiveTestState(t, snapshot, check.identity)
		if got.Scalar == nil || *got.Scalar != check.want {
			t.Errorf("%#v=%#v want scalar %q", check.identity, got, check.want)
		}
	}
}

func TestLiveCaptureSourceHasNoChildOrProcessLocalRecords(t *testing.T) {
	source := (Provider{}).LiveCaptureSource()
	if source == "" {
		t.Fatal("live capture source is empty")
	}
	for _, forbidden := range []string{"zsh -f", "exec ", "command zsh", "\x00pwd\x00", "\x00jobs\x00", "\x00history\x00", "\x00buffer\x00"} {
		if strings.Contains(strings.ToLower(source), forbidden) {
			t.Fatalf("live capture source contains forbidden %q", forbidden)
		}
	}
}

func TestDecodeLiveSnapshotExactBounds(t *testing.T) {
	provider := Provider{}
	for name, frame := range map[string][]byte{
		"zero": encodeLiveTestFrame(),
		"one":  encodeLiveTestFrame(liveFrameTestRecord{kind: model.LiveAlias, name: "a", attribute: "body", values: []string{""}}),
	} {
		t.Run(name, func(t *testing.T) {
			snapshot, err := provider.DecodeLiveSnapshot(frame)
			if err != nil {
				t.Fatal(err)
			}
			if snapshot.ByteSize != uint64(len(frame)) {
				t.Fatalf("ByteSize=%d want %d", snapshot.ByteSize, len(frame))
			}
		})
	}

	atCap := make([]liveFrameTestRecord, model.MaxSnapshotRecords)
	for index := range atCap {
		atCap[index] = liveFrameTestRecord{
			kind: model.LiveEnv, name: fmt.Sprintf("ZP_CAP_%05d", index), attribute: "exported", values: []string{"v"},
		}
	}
	if snapshot, err := provider.DecodeLiveSnapshot(encodeLiveTestFrame(atCap...)); err != nil || len(snapshot.States) != model.MaxSnapshotRecords {
		t.Fatalf("exact record cap = (%d, %v)", len(snapshot.States), err)
	}
	overCap := append(atCap, liveFrameTestRecord{kind: model.LiveEnv, name: "ZP_CAP_OVER", attribute: "exported", values: []string{"v"}})
	if snapshot, err := provider.DecodeLiveSnapshot(encodeLiveTestFrame(overCap...)); err == nil || !reflect.DeepEqual(snapshot, model.LiveSnapshot{}) {
		t.Fatalf("cap+1 = (%#v, %v), want zero snapshot error", snapshot, err)
	}

	prefix := encodeLiveTestFrame(liveFrameTestRecord{kind: model.LiveEnv, name: "ZP_BYTES", attribute: "exported", values: []string{""}})
	exactValueBytes := model.MaxSnapshotBytes - len(prefix)
	exact := encodeLiveTestFrame(liveFrameTestRecord{kind: model.LiveEnv, name: "ZP_BYTES", attribute: "exported", values: []string{strings.Repeat("x", exactValueBytes)}})
	if len(exact) != model.MaxSnapshotBytes {
		t.Fatalf("exact frame size=%d", len(exact))
	}
	if _, err := provider.DecodeLiveSnapshot(exact); err != nil {
		t.Fatalf("exact byte cap: %v", err)
	}
	overBytes := encodeLiveTestFrame(liveFrameTestRecord{kind: model.LiveEnv, name: "ZP_BYTES", attribute: "exported", values: []string{strings.Repeat("x", exactValueBytes+1)}})
	if snapshot, err := provider.DecodeLiveSnapshot(overBytes); err == nil || !reflect.DeepEqual(snapshot, model.LiveSnapshot{}) {
		t.Fatalf("byte cap+1 = (%#v, %v), want zero snapshot error", snapshot, err)
	}
}

func TestDecodeLiveSnapshotRejectsMalformedWithoutPartialState(t *testing.T) {
	valid := encodeLiveTestFrame(liveFrameTestRecord{kind: model.LiveEnv, name: "GOOD", attribute: "exported", values: []string{"safe"}})
	malformed := map[string][]byte{
		"missing terminal NUL": valid[:len(valid)-1],
		"wrong marker":         bytes.Replace(valid, []byte("ZP_LIVE_SNAPSHOT"), []byte("BAD_LIVE_SNAPSHOT"), 1),
		"wrong schema":         bytes.Replace(valid, []byte("\x001\x00"), []byte("\x002\x00"), 1),
		"wrong record marker":  bytes.Replace(valid, []byte("\x00R\x00"), []byte("\x00X\x00"), 1),
		"wrong kind":           bytes.Replace(valid, []byte("\x00env\x00"), []byte("\x00pwd\x00"), 1),
		"wrong name":           bytes.Replace(valid, []byte("\x00GOOD\x00"), []byte("\x00BAD-NAME\x00"), 1),
		"wrong attribute":      bytes.Replace(valid, []byte("\x00exported\x00"), []byte("\x00plain\x00"), 1),
		"wrong arity":          bytes.Replace(valid, []byte("\x001\x00safe\x00"), []byte("\x002\x00safe\x00"), 1),
		"trailing record":      append(append([]byte(nil), valid...), []byte("extra\x00")...),
	}
	for name, frame := range malformed {
		t.Run(name, func(t *testing.T) {
			snapshot, err := (Provider{}).DecodeLiveSnapshot(frame)
			if err == nil || !reflect.DeepEqual(snapshot, model.LiveSnapshot{}) {
				t.Fatalf("DecodeLiveSnapshot = (%#v, %v), want zero snapshot error", snapshot, err)
			}
		})
	}
}

func TestIntrospectReadsAliasesAndFunctions(t *testing.T) {
	if _, err := exec.LookPath("zsh"); err != nil {
		t.Skip("zsh not installed; skipping dynamic introspection test")
	}
	dir := t.TempDir()
	cfg := filepath.Join(dir, "rc.zsh")
	content := "alias gs='git status'\ngreet() { echo hi }\nexport MYVAR=1\npath+=(\"$HOME/bin\")\nsetopt AUTO_CD\n"
	if err := os.WriteFile(cfg, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	ids, err := (Provider{}).Introspect(cfg)
	if err != nil {
		t.Fatalf("Introspect error: %v", err)
	}
	if !ids.Available {
		t.Fatal("expected Available=true")
	}
	if !ids.Aliases["gs"] {
		t.Errorf("alias gs missing from %v", ids.Aliases)
	}
	if !ids.Functions["greet"] {
		t.Errorf("function greet missing from %v", ids.Functions)
	}
	if !ids.Env["MYVAR"] {
		t.Errorf("env MYVAR missing from %v", ids.Env)
	}
}

func TestIntrospectMissingFileDegrades(t *testing.T) {
	if _, err := exec.LookPath("zsh"); err != nil {
		t.Skip("zsh not installed")
	}
	ids, err := (Provider{}).Introspect("/nonexistent/path/rc.zsh")
	if err == nil || ids.Available {
		t.Fatalf("missing source = (%#v, %v), want unavailable error", ids, err)
	}
}

func TestIntrospectFailureAndEmpty(t *testing.T) {
	if _, err := exec.LookPath("zsh"); err != nil {
		t.Skip("zsh not installed")
	}
	dir := t.TempDir()
	for _, tc := range []struct {
		name, body string
		fail       bool
	}{{"syntax", "if then\n", true}, {"explicit", "return 9\n", true}, {"empty", "", false}} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(dir, tc.name+".zsh")
			if err := os.WriteFile(path, []byte(tc.body), 0o644); err != nil {
				t.Fatal(err)
			}
			ids, err := (Provider{}).Introspect(path)
			if tc.fail && (err == nil || ids.Available) {
				t.Fatalf("got %#v %v", ids, err)
			}
			if !tc.fail && (err != nil || !ids.Available) {
				t.Fatalf("got %#v %v", ids, err)
			}
		})
	}
}

func TestIntrospectCapturesBodies(t *testing.T) {
	if _, err := exec.LookPath("zsh"); err != nil {
		t.Skip("zsh not installed")
	}
	dir := t.TempDir()
	cfg := filepath.Join(dir, "rc.zsh")
	body := "\tprint -r -- 'one'\n\tprint -r -- \"$HOME\""
	if err := os.WriteFile(cfg, []byte("alias gs='git status'\nfoo() {"+body+";}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ids, err := (Provider{}).Introspect(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if ids.AliasBodies["gs"] != "git status" {
		t.Fatalf("alias body=%q", ids.AliasBodies["gs"])
	}
	if ids.FunctionBodies["foo"] != body {
		t.Fatalf("function body=%q want %q", ids.FunctionBodies["foo"], body)
	}
}

func TestParseIntrospectBodyBoundary(t *testing.T) {
	s := "##OPTIONS##\n\x00##ALIASBODIES##\ngs\x00git status\x00\x00##FUNCTIONBODIES##\nfoo\x00\tprint hi\n## not a header\x00\x00##END##\n"
	ids := (Provider{}).parseIntrospect(s)
	if ids.FunctionBodies["foo"] != "\tprint hi\n## not a header" {
		t.Fatalf("body=%q", ids.FunctionBodies["foo"])
	}
}

func TestParseIntrospectSentinelBodyDoesNotPolluteIdentityPrefix(t *testing.T) {
	body := "##ALIASES##\n##FUNCTIONS##\n##ENV##\n##PATH##\n##OPTIONS##\n##ALIASBODIES##\n##FUNCTIONBODIES##\n##END##"
	s := "##ALIASES##\na\n##FUNCTIONS##\nf\n##OPTIONS##\n\x00##ALIASBODIES##\na\x00\x00\x00##FUNCTIONBODIES##\nf\x00" + body + "\x00\x00##END##\n"
	ids := (Provider{}).parseIntrospect(s)
	if ids.FunctionBodies["f"] != body || !ids.Aliases["a"] || !ids.Functions["f"] || len(ids.Env) != 0 || len(ids.Path) != 0 || len(ids.Options) != 0 {
		t.Fatalf("%#v", ids)
	}
}

func TestParseIntrospectIgnoresBodyTextThatLooksLikeSectionHeaders(t *testing.T) {
	s := "##ALIASES##\na\n##FUNCTIONS##\nf\n##OPTIONS##\n\x00##ALIASBODIES##\na\x00##FUNCTIONBODIES##\ninside alias\x00\x00##FUNCTIONBODIES##\nf\x00##ALIASBODIES##\ninside function\x00\x00##END##\n"
	ids := (Provider{}).parseIntrospect(s)
	if ids.AliasBodies["a"] != "##FUNCTIONBODIES##\ninside alias" {
		t.Fatalf("alias body=%q", ids.AliasBodies["a"])
	}
	if ids.FunctionBodies["f"] != "##ALIASBODIES##\ninside function" {
		t.Fatalf("function body=%q", ids.FunctionBodies["f"])
	}
}
