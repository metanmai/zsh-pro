package zsh

import (
	"fmt"
	"os/exec"
	"reflect"
	"strings"
	"testing"

	"zsh-pro/core/activate"
	"zsh-pro/core/model"
)

func TestCommittedProjectionEmitCoversEveryLiveCategory(t *testing.T) {
	states := []model.LiveIdentityState{
		{Identity: model.Identity{Kind: model.LiveEnv, Name: "EXPORTED"}, Value: model.ScalarLiveValue("it's\nexact")},
		{Identity: model.Identity{Kind: model.LiveAlias, Name: "empty_alias"}, Value: model.ScalarLiveValue("")},
		{Identity: model.Identity{Kind: model.LiveFunction, Name: "multi_fn"}, Value: model.ScalarLiveValue("print -r -- one\nprint -r -- two")},
		{Identity: model.Identity{Kind: model.LivePath, Name: "PATH"}, Value: model.ListLiveValue([]string{"/base", "", "/dup", "/dup"})},
		{Identity: model.Identity{Kind: model.LiveFPath, Name: "FPATH"}, Value: model.ListLiveValue([]string{})},
		{Identity: model.Identity{Kind: model.LiveOption, Name: "AUTO_CD"}, Value: model.OptionLiveValue(false)},
	}
	var source []byte
	for _, item := range states {
		line, err := emitLiveIdentityState(item, true)
		if err != nil {
			t.Fatal(err)
		}
		source = append(source, line...)
	}
	for _, id := range []model.Identity{
		{Kind: model.LiveEnv, Name: "REMOVED"},
		{Kind: model.LiveAlias, Name: "old_alias"},
		{Kind: model.LiveFunction, Name: "old_fn"},
		{Kind: model.LiveOption, Name: "BEEP"},
	} {
		line, err := emitLiveIdentityTombstone(id)
		if err != nil {
			t.Fatal(err)
		}
		source = append(source, line...)
	}
	check := exec.Command("zsh", "-n")
	check.Stdin = strings.NewReader(string(source))
	if output, err := check.CombinedOutput(); err != nil {
		t.Fatalf("generated source is invalid: %v\n%s\n%s", err, output, source)
	}
	if strings.Contains(string(source), "not implemented") {
		t.Fatalf("placeholder source escaped: %s", source)
	}
}

func TestLivePatchListOccurrenceRemovalAndReplacementReverse(t *testing.T) {
	before := []string{"/unmanaged", "", "/dup", "/owned", "/dup"}
	after := []string{"/unmanaged", "", "/dup", "/dup"}
	wantBefore := model.ListLiveValue(before)
	wantAfter := model.ListLiveValue(after)
	if reflect.DeepEqual(wantBefore, wantAfter) {
		t.Fatal("invalid list fixture")
	}
	tests := []struct {
		name     string
		identity model.Identity
		array    string
		scalar   string
	}{
		{name: "PATH", identity: model.Identity{Kind: model.LivePath, Name: "PATH"}, array: "path", scalar: "PATH"},
		{name: "FPATH", identity: model.Identity{Kind: model.LiveFPath, Name: "FPATH"}, array: "fpath", scalar: "FPATH"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			forward := activate.TransitionLiveList{
				Identity:      test.identity,
				BeforePresent: true,
				AfterPresent:  true,
				Before:        before,
				After:         after,
			}
			reverse := activate.TransitionLiveList{
				Identity:      forward.Identity,
				BeforePresent: true,
				AfterPresent:  true,
				Before:        after,
				After:         before,
			}
			forwardSource, err := emitLiveOperation(forward)
			if err != nil {
				t.Fatal(err)
			}
			reverseSource, err := emitLiveOperation(reverse)
			if err != nil {
				t.Fatal(err)
			}
			script := strings.Join([]string{
				fmt.Sprintf("%s=('/unmanaged' '' '/dup' '/owned' '/dup')", test.array),
				fmt.Sprintf("typeset snapshot_array=${(qqqq)%s}", test.array),
				fmt.Sprintf("typeset snapshot_scalar=${(qqqq)%s}", test.scalar),
				string(forwardSource),
				fmt.Sprintf("[[ ${#%s} == 4 && $%s[1] == /unmanaged && $%s[2] == '' && $%s[3] == /dup && $%s[4] == /dup ]] || exit 21", test.array, test.array, test.array, test.array, test.array),
				fmt.Sprintf("[[ $%s == '/unmanaged::/dup:/dup' ]] || exit 22", test.scalar),
				string(reverseSource),
				fmt.Sprintf("[[ ${(qqqq)%s} == $snapshot_array && ${(qqqq)%s} == $snapshot_scalar ]] || exit 23", test.array, test.scalar),
			}, "\n")
			if output, err := exec.Command("zsh", "-f", "-c", script).CombinedOutput(); err != nil {
				t.Fatalf("list transition did not preserve exact occurrence/tied reverse: %v\n%s\n%s", err, output, script)
			}
		})
	}
}

func TestEmitLivePatchOwnsExactApplyAndReplacementReverse(t *testing.T) {
	before := []model.LiveIdentityState{
		{Identity: model.Identity{Kind: model.LiveEnv, Name: "ZP08_VALUE"}, Value: model.ScalarLiveValue("before")},
		{Identity: model.Identity{Kind: model.LiveAlias, Name: "zp08_alias"}, Value: model.ScalarLiveValue("print -r -- before")},
		{Identity: model.Identity{Kind: model.LiveFunction, Name: "zp08_fn"}, Value: model.ScalarLiveValue("print -r -- before")},
		{Identity: model.Identity{Kind: model.LivePath, Name: "PATH"}, Value: model.ListLiveValue([]string{"/before", "", "/dup", "/dup"})},
		{Identity: model.Identity{Kind: model.LiveFPath, Name: "FPATH"}, Value: model.ListLiveValue([]string{"/functions-before"})},
		{Identity: model.Identity{Kind: model.LiveOption, Name: "AUTO_CD"}, Value: model.OptionLiveValue(false)},
	}
	after := []model.LiveIdentityState{
		{Identity: model.Identity{Kind: model.LiveEnv, Name: "ZP08_VALUE"}, Value: model.ScalarLiveValue("after\nexact")},
		{Identity: model.Identity{Kind: model.LiveAlias, Name: "zp08_alias"}, Value: model.ScalarLiveValue("print -r -- after")},
		{Identity: model.Identity{Kind: model.LiveFunction, Name: "zp08_fn"}, Value: model.ScalarLiveValue("print -r -- after")},
		{Identity: model.Identity{Kind: model.LivePath, Name: "PATH"}, Value: model.ListLiveValue([]string{"/after", "", "/dup"})},
		{Identity: model.Identity{Kind: model.LiveFPath, Name: "FPATH"}, Value: model.ListLiveValue([]string{})},
		{Identity: model.Identity{Kind: model.LiveOption, Name: "AUTO_CD"}, Value: model.OptionLiveValue(true)},
	}
	patch, err := activate.BuildLivePatch(before, after)
	if err != nil {
		t.Fatal(err)
	}
	source, err := (Provider{}).EmitLivePatch(patch.Forward, patch.ReplacementReverse, "__zp08_apply", "__zp08_reverse")
	if err != nil {
		t.Fatal(err)
	}
	check := exec.Command("zsh", "-n")
	check.Stdin = strings.NewReader(string(source))
	if output, err := check.CombinedOutput(); err != nil {
		t.Fatalf("live patch is not valid zsh: %v\n%s\n%s", err, output, source)
	}
	script := strings.Join([]string{
		"ZP08_VALUE=before",
		"alias zp08_alias='print -r -- before'",
		"zp08_fn() { print -r -- before; }",
		"path=('/before' '' '/dup' '/dup')",
		"fpath=('/functions-before')",
		"unsetopt autocd",
		string(source),
		"__zp08_apply",
		"[[ $ZP08_VALUE == $'after\\nexact' ]] || exit 31",
		"[[ ${aliases[zp08_alias]} == 'print -r -- after' ]] || exit 32",
		"[[ ${functions[zp08_fn]} == *'print -r -- after'* ]] || exit 33",
		"[[ ${(j:|:)path} == '/after||/dup' ]] || exit 34",
		"[[ ${#fpath} == 0 ]] || exit 35",
		"[[ -o autocd ]] || exit 36",
		"__zp08_reverse",
		"[[ $ZP08_VALUE == before ]] || exit 41",
		"[[ ${aliases[zp08_alias]} == 'print -r -- before' ]] || exit 42",
		"[[ ${functions[zp08_fn]} == *'print -r -- before'* ]] || exit 43",
		"[[ ${(j:|:)path} == '/before||/dup|/dup' ]] || exit 44",
		"[[ ${(j:|:)fpath} == '/functions-before' ]] || exit 45",
		"[[ ! -o autocd ]] || exit 46",
	}, "\n")
	if output, err := exec.Command("zsh", "-f", "-c", script).CombinedOutput(); err != nil {
		t.Fatalf("live patch did not apply and reverse exactly: %v\n%s\n%s", err, output, script)
	}
}

func TestEmitLivePatchFailsFastPerOperation(t *testing.T) {
	if _, err := exec.LookPath("zsh"); err != nil {
		t.Skip("zsh not available")
	}

	identity := func(name string) model.Identity {
		return model.Identity{Kind: model.LiveAlias, Name: name}
	}
	operations := []activate.Op{
		activate.SetLiveScalar{Identity: identity("zp14_first"), Value: "print -r -- first-secret-value"},
		activate.SetLiveScalar{Identity: identity("zp14_middle"), Value: "print -r -- middle-secret-value"},
		activate.SetLiveScalar{Identity: identity("zp14_final"), Value: "print -r -- final-secret-value"},
	}

	for index, name := range []string{"first", "middle", "final"} {
		t.Run("forward_"+name, func(t *testing.T) {
			source, err := (Provider{}).EmitLivePatch(operations, nil, "__zp14_apply", "__zp14_reverse")
			if err != nil {
				t.Fatal(err)
			}
			failed := []string{"zp14_first", "zp14_middle", "zp14_final"}[index]
			failureStatus := 41 + index
			checks := []string{
				"alias() { if [[ $1 == " + failed + "=* ]]; then return " + fmt.Sprint(failureStatus) + "; fi; builtin alias \"$@\"; }",
				string(source),
				"__zp14_apply",
				"typeset -i operation_rc=$?",
				"(( operation_rc == " + fmt.Sprint(failureStatus) + " )) || exit 71",
			}
			for later := index + 1; later < len(operations); later++ {
				name := []string{"zp14_first", "zp14_middle", "zp14_final"}[later]
				checks = append(checks, "(( ! ${+aliases["+name+"]} )) || exit 72")
			}
			output, runErr := exec.Command("zsh", "-f", "-c", strings.Join(checks, "\n")).CombinedOutput()
			if strings.Contains(string(output), "secret-value") {
				t.Fatalf("failure diagnostics leaked a live value: %q", output)
			}
			if runErr != nil {
				t.Fatalf("%s forward operation did not fail fast: %v\n%s", name, runErr, output)
			}
		})
	}

	t.Run("replacement_reverse", func(t *testing.T) {
		reverse := []activate.Op{
			activate.SetLiveScalar{Identity: identity("zp14_reverse_first"), Value: "print -r -- reverse-first-secret-value"},
			activate.SetLiveScalar{Identity: identity("zp14_reverse_middle"), Value: "print -r -- reverse-middle-secret-value"},
			activate.SetLiveScalar{Identity: identity("zp14_reverse_final"), Value: "print -r -- reverse-final-secret-value"},
		}
		source, err := (Provider{}).EmitLivePatch(nil, reverse, "__zp14_apply", "__zp14_reverse")
		if err != nil {
			t.Fatal(err)
		}
		script := strings.Join([]string{
			"alias() { if [[ $1 == zp14_reverse_middle=* ]]; then return 57; fi; builtin alias \"$@\"; }",
			string(source),
			"__zp14_reverse",
			"typeset -i operation_rc=$?",
			"(( operation_rc == 57 )) || exit 81",
			"(( ! ${+aliases[zp14_reverse_final]} )) || exit 82",
		}, "\n")
		output, runErr := exec.Command("zsh", "-f", "-c", script).CombinedOutput()
		if strings.Contains(string(output), "secret-value") {
			t.Fatalf("failure diagnostics leaked a replacement reverse value: %q", output)
		}
		if runErr != nil {
			t.Fatalf("replacement reverse did not fail fast: %v\n%s", runErr, output)
		}
	})
}

func TestEmitRuntimeTransitionOwnsFinalAcknowledgementEnvelope(t *testing.T) {
	fingerprint := strings.Repeat("ab", 32)
	source, err := (Provider{}).EmitRuntimeTransition(nil, nil, "__zp08_apply", "__zp08_reverse", 17, 29, fingerprint)
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Join([]string{
		"typeset -g ZP_WORKTREE_REPLY_PROTOCOL='1'",
		"typeset -g ZP_WORKTREE_REPLY_REVISION='17'",
		"typeset -g ZP_WORKTREE_REPLY_TOKEN='29'",
		"typeset -g ZP_WORKTREE_REPLY_FINGERPRINT='" + fingerprint + "'",
		"typeset -g ZP_WORKTREE_REPLY_COMPLETE='1'",
		"",
	}, "\n")
	if string(source) != want {
		t.Fatalf("metadata-only transition mismatch\nwant:\n%s\ngot:\n%s", want, source)
	}
	if strings.Contains(string(source), "__zp08_apply") || strings.Contains(string(source), "__zp08_reverse") {
		t.Fatalf("zero-mutation transition emitted patch functions: %s", source)
	}
	if output, err := exec.Command("zsh", "-f", "-c", string(source)+"\n[[ $ZP_WORKTREE_REPLY_PROTOCOL == 1 && $ZP_WORKTREE_REPLY_REVISION == 17 && $ZP_WORKTREE_REPLY_TOKEN == 29 && $ZP_WORKTREE_REPLY_FINGERPRINT == '"+fingerprint+"' && $ZP_WORKTREE_REPLY_COMPLETE == 1 ]]").CombinedOutput(); err != nil {
		t.Fatalf("acknowledgement envelope is not exact executable zsh: %v\n%s", err, output)
	}
}

func TestEmitLivePatchRejectsMalformedOrReservedInputWithoutSource(t *testing.T) {
	reserved := activate.SetLiveScalar{Identity: model.Identity{Kind: model.LiveEnv, Name: "ZP_WORKTREE_REPLY_TOKEN"}, Value: "attacker"}
	malformed := activate.TransitionLiveList{Identity: model.Identity{Kind: model.LivePath, Name: "PATH"}, BeforePresent: false, Before: []string{"carried"}}
	tests := []struct {
		name        string
		forward     []activate.Op
		apply       string
		reverse     string
		revision    uint64
		token       uint64
		fingerprint string
	}{
		{name: "reserved identity", forward: []activate.Op{reserved}, apply: "__zp08_apply", reverse: "__zp08_reverse", revision: 1, token: 1, fingerprint: strings.Repeat("a", 64)},
		{name: "malformed list", forward: []activate.Op{malformed}, apply: "__zp08_apply", reverse: "__zp08_reverse", revision: 1, token: 1, fingerprint: strings.Repeat("a", 64)},
		{name: "unsafe apply", apply: "bad; print owned", reverse: "__zp08_reverse", revision: 1, token: 1, fingerprint: strings.Repeat("a", 64)},
		{name: "duplicate names", apply: "__zp08_same", reverse: "__zp08_same", revision: 1, token: 1, fingerprint: strings.Repeat("a", 64)},
		{name: "zero revision", apply: "__zp08_apply", reverse: "__zp08_reverse", token: 1, fingerprint: strings.Repeat("a", 64)},
		{name: "zero token", apply: "__zp08_apply", reverse: "__zp08_reverse", revision: 1, fingerprint: strings.Repeat("a", 64)},
		{name: "short fingerprint", apply: "__zp08_apply", reverse: "__zp08_reverse", revision: 1, token: 1, fingerprint: strings.Repeat("a", 63)},
		{name: "uppercase fingerprint", apply: "__zp08_apply", reverse: "__zp08_reverse", revision: 1, token: 1, fingerprint: strings.Repeat("A", 64)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			source, err := (Provider{}).EmitRuntimeTransition(test.forward, nil, test.apply, test.reverse, test.revision, test.token, test.fingerprint)
			if err == nil || len(source) != 0 {
				t.Fatalf("malformed transition escaped: source=%q err=%v", source, err)
			}
		})
	}
}

func TestEmitRuntimeTransitionRejectsMalformedInput(t *testing.T) {
	malformed := activate.TransitionLiveList{
		Identity:      model.Identity{Kind: model.LivePath, Name: "PATH"},
		BeforePresent: false,
		Before:        []string{"must-not-escape"},
	}
	source, err := (Provider{}).EmitRuntimeTransition(
		[]activate.Op{malformed}, nil,
		"__zp14_apply", "__zp14_reverse",
		1, 1, strings.Repeat("a", 64),
	)
	if err == nil || len(source) != 0 {
		t.Fatalf("malformed runtime transition escaped: source=%q err=%v", source, err)
	}
}

func TestEmitStaticDynamicAndSyntax(t *testing.T) {
	p := activate.Plan{Activate: []activate.Op{
		activate.SetScalar{Name: "EDITOR", Applied: "it's; echo bad", Dynamic: false},
		activate.SetScalar{Name: "GOPATH", Applied: "$HOME/go", Dynamic: true, Exported: true},
		activate.ApplyListDelta{Name: "PATH", Additions: []string{"/opt/tool*", "$HOME/bin"}},
		activate.AddAlias{Name: "gs", Body: "git status", Dynamic: false},
		activate.SetOption{Name: "extendedglob", Enabled: true},
	}, Deactivate: []activate.Op{
		activate.RestoreScalar{Name: "EDITOR", Applied: "it's; echo bad"},
		activate.RebuildListFromBase{Name: "PATH"},
		activate.Unalias{Name: "gs"}, activate.RestoreShadowedAlias{Name: "gs"},
		activate.RestoreOption{Name: "extendedglob"},
	}}
	a, d, err := (Provider{}).Emit(p)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(a, "'it'\\''s; echo bad'") {
		t.Errorf("static value was not quoted: %s", a)
	}
	if !strings.Contains(a, "export GOPATH=$HOME/go") {
		t.Errorf("dynamic value was quoted: %s", a)
	}
	if !strings.Contains(a, "'/opt/tool*'") || !strings.Contains(a, "$HOME/bin") {
		t.Errorf("list rendering lost static/dynamic split: %s", a)
	}
	for name, src := range map[string]string{"apply": a, "deactivate": d} {
		cmd := exec.Command("zsh", "-n")
		cmd.Stdin = strings.NewReader(src)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%s syntax: %v\n%s\n%s", name, err, out, src)
		}
	}
}

func TestEmitRuntimeAvoidsStaticAppliedSecretCopiesButKeepsDynamicReversal(t *testing.T) {
	p := activate.Plan{Activate: []activate.Op{
		activate.SetScalar{Name: "ZP_RUNTIME_STATIC", Applied: "phase5-static-secret", Exported: true},
		activate.SetScalar{Name: "ZP_RUNTIME_DYNAMIC", Applied: "$HOME/runtime", Dynamic: true, Exported: true},
	}, Deactivate: []activate.Op{
		activate.RestoreScalar{Name: "ZP_RUNTIME_STATIC", Applied: "phase5-static-secret"},
		activate.RestoreScalar{Name: "ZP_RUNTIME_DYNAMIC", Applied: "$HOME/runtime", Dynamic: true},
	}}
	apply, deactivate, err := (Provider{}).EmitRuntime(p, "zp_runtime_apply", "zp_runtime_reverse")
	if err != nil {
		t.Fatal(err)
	}
	staticSlot := encodeSlot("APPLIED_SCALAR", "ZP_RUNTIME_STATIC")
	dynamicSlot := encodeSlot("APPLIED_SCALAR", "ZP_RUNTIME_DYNAMIC")
	for name, source := range map[string]string{"apply": apply, "deactivate": deactivate} {
		if strings.Contains(source, staticSlot) {
			t.Fatalf("%s retained avoidable static applied slot %q:\n%s", name, staticSlot, source)
		}
		if !strings.Contains(source, dynamicSlot) {
			t.Fatalf("%s omitted dynamic applied slot %q:\n%s", name, dynamicSlot, source)
		}
	}

	script := strings.Join([]string{
		loaderScript,
		"export ZP_RUNTIME_STATIC=before-static ZP_RUNTIME_DYNAMIC=before-dynamic",
		"export HOME=/runtime-home",
		apply,
		"zp_runtime_apply",
		"[[ $ZP_RUNTIME_STATIC == phase5-static-secret ]] || exit 10",
		"[[ $ZP_RUNTIME_DYNAMIC == /runtime-home/runtime ]] || exit 11",
		"[[ ${+" + dynamicSlot + "} == 1 ]] || exit 12",
		deactivate,
		"zp_runtime_reverse",
		"[[ $ZP_RUNTIME_STATIC == before-static && $ZP_RUNTIME_DYNAMIC == before-dynamic ]] || exit 13",
		"[[ ${+" + dynamicSlot + "} == 0 ]] || exit 14",
	}, "\n")
	if out, err := exec.Command("zsh", "-f", "-c", script, "zsh-pro-runtime-dynamic-test").CombinedOutput(); err != nil {
		t.Fatalf("runtime scalar reversal lost dynamic correctness: %v\n%s\n%s", err, out, script)
	}
}

func TestEmitRejectsHostileNamesWithoutOutput(t *testing.T) {
	a, d, err := (Provider{}).Emit(activate.Plan{Activate: []activate.Op{
		activate.SetOption{Name: "foo;touch", Enabled: true},
		activate.AddAlias{Name: "x$(touch CANARY)", Body: "bad"},
		activate.AddFunc{Name: "a|b", Body: "bad"},
	}, Deactivate: []activate.Op{
		activate.RestoreOption{Name: "foo;touch"},
		activate.Unalias{Name: "x$(touch CANARY)"},
		activate.UnsetFunc{Name: "a|b"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(a, "foo;touch") || strings.Contains(a, "CANARY") || strings.Contains(d, "CANARY") {
		t.Fatalf("hostile names reached output: apply=%s deactivate=%s", a, d)
	}
}

func TestEmitAppliesAndRestoresShadowAndOption(t *testing.T) {
	base := activate.Plan{Activate: []activate.Op{
		activate.SetScalar{Name: "ZP_DEMO_VALUE", Applied: "changed"},
		activate.AddAlias{Name: "ll", Body: "new alias"},
		activate.AddFunc{Name: "ff", Body: "{ print new-func; }"},
		activate.SetOption{Name: "extendedglob", Enabled: false},
	}, Deactivate: []activate.Op{
		activate.RestoreScalar{Name: "ZP_DEMO_VALUE", Applied: "changed"},
		activate.Unalias{Name: "ll"}, activate.RestoreShadowedAlias{Name: "ll"},
		activate.UnsetFunc{Name: "ff"}, activate.RestoreShadowedFunc{Name: "ff"},
		activate.RestoreOption{Name: "extendedglob"},
	}}
	a, d, err := (Provider{}).Emit(base)
	if err != nil {
		t.Fatal(err)
	}
	script := "ZP_DEMO_VALUE=before\nalias ll='old alias'\nff() { print old-func; }\nsetopt extendedglob\n" + a + "\nzp_apply\n" + d + "\nzp_deactivate\nprintf '%s\\n' \"$ZP_DEMO_VALUE\" \"${aliases[ll]}\" \"${functions[ff]}\" \"$options[extendedglob]\"\n"
	out, err := exec.Command("zsh", "-f", "-c", script).CombinedOutput()
	if err != nil {
		t.Fatalf("zsh: %v\n%s\n%s", err, out, script)
	}
	if !strings.Contains(string(out), "before") || !strings.Contains(string(out), "old alias") || !strings.Contains(string(out), "old-func") || !strings.Contains(string(out), "on") {
		t.Fatalf("restore output=%q", out)
	}
}

func TestEmitDefinesEmptyAndMultilineFunctions(t *testing.T) {
	if _, err := exec.LookPath("zsh"); err != nil {
		t.Skip("zsh not available")
	}
	p := activate.Plan{Activate: []activate.Op{
		activate.AddFunc{Name: "zp04_empty", Body: ""},
		activate.AddFunc{Name: "zp04_multiline", Body: "print -r -- line-one\nprint -r -- line-two"},
	}}
	apply, _, err := (Provider{}).Emit(p)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(apply, `functions[zp04_empty]=""`) {
		t.Fatalf("empty function assignment missing:\n%s", apply)
	}

	check := exec.Command("zsh", "-n")
	check.Stdin = strings.NewReader(apply)
	if out, err := check.CombinedOutput(); err != nil {
		t.Fatalf("syntax: %v\n%s\n%s", err, out, apply)
	}

	script := apply + `
zp_apply
(( ${+functions[zp04_empty]} )) || exit 11
zp04_empty || exit 12
[[ "$(zp04_multiline)" == $'line-one\nline-two' ]] || exit 13
`
	if out, err := exec.Command("zsh", "-f", "-c", script).CombinedOutput(); err != nil {
		t.Fatalf("zsh: %v\n%s\n%s", err, out, script)
	}
}

func TestEncodeSlotIsInjectiveAndIdentifierSafe(t *testing.T) {
	names := []string{"foo-bar", "foo.bar", "foo_bar", "", "€"}
	seen := map[string]bool{}
	for _, name := range names {
		slot := encodeSlot("SCALAR", name)
		if !envNameRe.MatchString(slot) {
			t.Fatalf("slot %q for %q is not a valid identifier", slot, name)
		}
		if seen[slot] {
			t.Fatalf("slot collision for %q: %q", name, slot)
		}
		seen[slot] = true
	}
}

func TestEmitRestoresPresenceSentinelAndExportState(t *testing.T) {
	p := activate.Plan{Activate: []activate.Op{
		activate.SetScalar{Name: "ZP04_UNSET", Applied: "plain", Exported: false},
		activate.SetScalar{Name: "ZP04_EMPTY_PLAIN", Applied: "exported", Exported: true},
		activate.SetScalar{Name: "ZP04_SENTINEL", Applied: "changed", Exported: false},
		activate.AddAlias{Name: "zp04_alias", Body: "changed"},
		activate.AddFunc{Name: "zp04_func", Body: "print changed"},
	}, Deactivate: []activate.Op{
		activate.UnsetScalar{Name: "ZP04_UNSET", Applied: "plain"},
		activate.RestoreScalar{Name: "ZP04_EMPTY_PLAIN", Applied: "exported"},
		activate.RestoreScalar{Name: "ZP04_SENTINEL", Applied: "changed"},
		activate.Unalias{Name: "zp04_alias"}, activate.RestoreShadowedAlias{Name: "zp04_alias"},
		activate.UnsetFunc{Name: "zp04_func"}, activate.RestoreShadowedFunc{Name: "zp04_func"},
	}}
	apply, deactivate, err := (Provider{}).Emit(p)
	if err != nil {
		t.Fatal(err)
	}
	script := strings.Join([]string{
		"typeset ZP04_EMPTY_PLAIN=''",
		"typeset -gx ZP04_SENTINEL='__ZP_UNSET__'",
		"alias zp04_alias='__ZP_UNSET__'",
		"zp04_func() { print __ZP_UNSET__; }",
		apply,
		"zp_apply",
		deactivate,
		"zp_deactivate",
		"[[ ${+ZP04_UNSET} == 0 ]] || exit 11",
		"[[ ${+ZP04_EMPTY_PLAIN} == 1 && $ZP04_EMPTY_PLAIN == '' ]] || exit 12",
		"[[ ${parameters[ZP04_EMPTY_PLAIN]} != *export* ]] || exit 13",
		"[[ $ZP04_SENTINEL == __ZP_UNSET__ && ${parameters[ZP04_SENTINEL]} == *export* ]] || exit 14",
		"[[ ${aliases[zp04_alias]} == __ZP_UNSET__ ]] || exit 15",
		"[[ ${functions[zp04_func]} == *'__ZP_UNSET__'* ]] || exit 16",
	}, "\n")
	if out, err := exec.Command("zsh", "-f", "-c", script).CombinedOutput(); err != nil {
		t.Fatalf("zsh: %v\n%s\n%s", err, out, script)
	}
}

func TestEmitRepeatedScalarTracksFinalAppliedValue(t *testing.T) {
	p := activate.Plan{Activate: []activate.Op{
		activate.SetScalar{Name: "ZP04_REPEATED", Applied: "one"},
		activate.SetScalar{Name: "ZP04_REPEATED", Applied: "two"},
	}, Deactivate: []activate.Op{
		activate.RestoreScalar{Name: "ZP04_REPEATED", Applied: "two"},
	}}
	apply, deactivate, err := (Provider{}).Emit(p)
	if err != nil {
		t.Fatal(err)
	}
	script := "ZP04_REPEATED=before\n" + apply + "\nzp_apply\n" + deactivate + "\nzp_deactivate\n[[ $ZP04_REPEATED == before ]] || exit 21\n"
	if out, err := exec.Command("zsh", "-f", "-c", script).CombinedOutput(); err != nil {
		t.Fatalf("zsh: %v\n%s\n%s", err, out, script)
	}
}
