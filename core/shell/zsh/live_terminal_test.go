package zsh

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLiveTerminalLoaderSwitchesCurrentShellWithoutResidue(t *testing.T) {
	if _, err := exec.LookPath("zsh"); err != nil {
		t.Skip("zsh not installed")
	}
	dir := t.TempDir()
	loader := filepath.Join(dir, "loader.zsh")
	if err := os.WriteFile(loader, []byte((Provider{}).HookScript()), 0o600); err != nil {
		t.Fatal(err)
	}
	shim := filepath.Join(dir, "zsh-pro")
	const shimSource = `#!/bin/sh
case "$1:$2:$3" in
  emit:apply:A) printf '%s\n' "zp_apply() { export ZP_TEST_ENV=A; alias zp_test_alias='print A'; path=(/zp-a/bin \$path); }" ;;
  emit:apply:B) printf '%s\n' "zp_apply() { export ZP_TEST_ENV=B; alias zp_test_alias='print B'; path=(/zp-b/bin \$path); }" ;;
  emit:deactivate:*) printf '%s\n' "zp_deactivate() { unalias zp_test_alias 2>/dev/null; unset ZP_TEST_ENV; path=(\${(@s/:/)ZP_BASE_PATH}); }" ;;
  list) printf '%s\n' main A B ;;
  status) printf '%s\n' main ;;
  *) exit 64 ;;
esac
`
	if err := os.WriteFile(shim, []byte(shimSource), 0o700); err != nil {
		t.Fatal(err)
	}
	const body = `
source "$1"
before_path_count=$#path
checkout A || exit 10
[[ "$ZSHPRO_PROFILE" == A ]] || exit 11
[[ "$ZP_TEST_ENV" == A ]] || exit 12
alias zp_test_alias >/dev/null || exit 13
activate B || exit 14
[[ "$ZSHPRO_PROFILE" == B ]] || exit 15
deactivate || exit 16
[[ -z "${ZSHPRO_PROFILE+x}" ]] || exit 17
[[ -z "${ZP_TEST_ENV+x}" ]] || exit 18
alias zp_test_alias >/dev/null 2>&1 && exit 19
[[ "$(status)" == main ]] || exit 20
[[ $#path -eq $before_path_count ]] || exit 21

# An inherited profile without loader state is a fresh terminal, not a request
# to consume missing undo slots.
unset ZP_BASE_PATH
export ZSHPRO_PROFILE=A
activate A || exit 22
[[ -n "${ZP_BASE_PATH+x}" ]] || exit 23
`
	cmd := exec.Command("zsh", "-f", "-c", body, "zsh-pro-test", loader)
	cmd.Env = append(os.Environ(), "PATH="+dir+":"+os.Getenv("PATH"))
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("loader did not mutate the current shell correctly: %v\n%s", err, strings.TrimSpace(string(out)))
	}
}

func TestLiveTerminalLoaderRejectsInvalidEmitWithoutChangingLastGood(t *testing.T) {
	realZsh, err := exec.LookPath("zsh")
	if err != nil {
		t.Skip("zsh not installed")
	}
	dir := t.TempDir()
	loader := filepath.Join(dir, "loader.zsh")
	if err := os.WriteFile(loader, []byte((Provider{}).HookScript()), 0o600); err != nil {
		t.Fatal(err)
	}
	shim := filepath.Join(dir, "zsh-pro")
	if err := os.WriteFile(shim, []byte("#!/bin/sh\nprintf '%s\\n' 'this is ( invalid zsh'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	validationMarker := filepath.Join(dir, "validated")
	validator := filepath.Join(dir, "zsh")
	validatorSource := "#!/bin/sh\ntouch \"$ZP_VALIDATION_MARKER\"\nexec \"$ZP_REAL_ZSH\" \"$@\"\n"
	if err := os.WriteFile(validator, []byte(validatorSource), 0o700); err != nil {
		t.Fatal(err)
	}
	body := `
source "$1"
export ZSHPRO_PROFILE=good ZP_LAST_GOOD_PROFILE=good
checkout bad
[[ "$ZP_LAST_RUNTIME_STATUS" -ne 0 ]] || exit 10
[[ "$ZSHPRO_PROFILE" == good ]] || exit 11
[[ "$ZP_LAST_GOOD_PROFILE" == good ]] || exit 12
[[ -z "${ZP_TEST_ENV+x}" ]] || exit 13
`
	cmd := exec.Command(realZsh, "-f", "-c", body, "zsh-pro-test", loader)
	cmd.Env = append(os.Environ(), "PATH="+dir+":"+os.Getenv("PATH"), "ZP_VALIDATION_MARKER="+validationMarker, "ZP_REAL_ZSH="+realZsh)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("invalid emit changed live state or escaped the validation gate: %v\n%s", err, strings.TrimSpace(string(out)))
	}
	if _, err := os.Stat(validationMarker); err != nil {
		t.Fatal("invalid emit never ran zsh -n validation")
	}
}

func TestLiveTerminalLoaderGatesEmptyEmitAndReportsRuntimeFailure(t *testing.T) {
	realZsh, err := exec.LookPath("zsh")
	if err != nil {
		t.Skip("zsh not installed")
	}
	for _, tc := range []struct {
		name, emit, assertion string
	}{
		{"empty emit", "#!/bin/sh\nexit 0\n", `[[ "$ZSHPRO_PROFILE" == good ]] || exit 11; [[ "$ZP_LAST_GOOD_PROFILE" == good ]] || exit 12`},
		{"runtime failure", "#!/bin/sh\nprintf '%s\\n' 'zp_apply() { export ZP_PARTIAL=1; return 9; }'\n", `[[ "$ZP_LAST_GOOD_PROFILE" == good ]] || exit 21; [[ "$ZP_PARTIAL" == 1 ]] || exit 22`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			loader := filepath.Join(dir, "loader.zsh")
			if err := os.WriteFile(loader, []byte((Provider{}).HookScript()), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "zsh-pro"), []byte(tc.emit), 0o700); err != nil {
				t.Fatal(err)
			}
			marker := filepath.Join(dir, "validated")
			validator := "#!/bin/sh\ntouch \"$ZP_VALIDATION_MARKER\"\nexec \"$ZP_REAL_ZSH\" \"$@\"\n"
			if err := os.WriteFile(filepath.Join(dir, "zsh"), []byte(validator), 0o700); err != nil {
				t.Fatal(err)
			}
			body := "source \"$1\"; export ZSHPRO_PROFILE=good ZP_LAST_GOOD_PROFILE=good; activate bad; [[ \"$ZP_LAST_RUNTIME_STATUS\" -ne 0 ]] || exit 10; " + tc.assertion
			cmd := exec.Command(realZsh, "-f", "-c", body, "zsh-pro-test", loader)
			cmd.Env = append(os.Environ(), "PATH="+dir+":"+os.Getenv("PATH"), "ZP_VALIDATION_MARKER="+marker, "ZP_REAL_ZSH="+realZsh)
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("loader gate behavior failed: %v\n%s", err, out)
			}
			if tc.name == "empty emit" {
				if _, err := os.Stat(marker); !os.IsNotExist(err) {
					t.Fatal("empty emit reached zsh -n instead of aborting before validation")
				}
			} else if !strings.Contains(string(out), "switch failed at runtime") {
				t.Fatalf("runtime failure had no recovery/report path: %s", out)
			}
		})
	}
}

func TestLiveTerminalPublicVerbsFailOpenUnderErrExitAndErrReturn(t *testing.T) {
	realZsh, err := exec.LookPath("zsh")
	if err != nil {
		t.Skip("zsh not installed")
	}

	for _, verb := range []string{"activate bad", "checkout bad", "deactivate"} {
		for _, option := range []string{"ERR_EXIT", "ERR_RETURN"} {
			t.Run(verb+"/"+option, func(t *testing.T) {
				dir := t.TempDir()
				loader := writeLiveLoader(t, dir)
				writeLiveExecutable(t, filepath.Join(dir, "zsh-pro"), "#!/bin/sh\nexit 9\n")

				body := `
source "$1"
export ZSHPRO_PROFILE=good ZP_LAST_GOOD_PROFILE=good
setopt ` + option + `
` + verb + `
print -r -- SURVIVED
[[ "$ZP_LAST_RUNTIME_STATUS" -ne 0 ]] || exit 20
[[ "$ZSHPRO_PROFILE" == good ]] || exit 21
[[ "$ZP_LAST_GOOD_PROFILE" == good ]] || exit 22
`
				cmd := exec.Command(realZsh, "-f", "-c", body, "zsh-pro-test", loader)
				cmd.Env = append(os.Environ(), "PATH="+dir+":"+os.Getenv("PATH"))
				out, err := cmd.CombinedOutput()
				if err != nil {
					t.Fatalf("%s under %s escaped its fail-open boundary: %v\n%s", verb, option, err, out)
				}
				if !strings.Contains(string(out), "SURVIVED") {
					t.Fatalf("%s under %s did not reach the next command:\n%s", verb, option, out)
				}
			})
		}
	}
}

func TestLiveTerminalConsumesAllExpectedRuntimeFailures(t *testing.T) {
	realZsh, err := exec.LookPath("zsh")
	if err != nil {
		t.Skip("zsh not installed")
	}

	for _, tc := range []struct {
		name       string
		emitter    string
		validator  string
		badTempDir bool
	}{
		{name: "emitter failure", emitter: "#!/bin/sh\nexit 9\n"},
		{name: "empty emitter output", emitter: "#!/bin/sh\nexit 0\n"},
		{name: "staging failure", emitter: "#!/bin/sh\nprintf '%s\\n' 'zp_apply() { :; }'\n", badTempDir: true},
		{name: "validation failure", emitter: "#!/bin/sh\nprintf '%s\\n' 'zp_apply() { :; }'\n", validator: "#!/bin/sh\nexit 9\n"},
		{name: "evaluation failure", emitter: "#!/bin/sh\nprintf '%s\\n' 'zp_apply() { return 9; }'\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			loader := writeLiveLoader(t, dir)
			writeLiveExecutable(t, filepath.Join(dir, "zsh-pro"), tc.emitter)
			if tc.validator != "" {
				writeLiveExecutable(t, filepath.Join(dir, "zsh"), tc.validator)
			}
			tempDir := dir
			if tc.badTempDir {
				tempDir = filepath.Join(dir, "not-a-directory")
				if err := os.WriteFile(tempDir, []byte("not a directory"), 0o600); err != nil {
					t.Fatal(err)
				}
			}

			body := `
source "$1"
export ZSHPRO_PROFILE=good ZP_LAST_GOOD_PROFILE=good
setopt ERR_EXIT
setopt XTRACE
activate bad
print -r -- SURVIVED
[[ "$ZP_LAST_RUNTIME_STATUS" -ne 0 ]] || exit 30
[[ "$ZSHPRO_PROFILE" == good ]] || exit 31
[[ "$ZP_LAST_GOOD_PROFILE" == good ]] || exit 32
[[ -o xtrace ]] || exit 33
`
			cmd := exec.Command(realZsh, "-f", "-c", body, "zsh-pro-test", loader)
			cmd.Env = append(os.Environ(), "PATH="+dir+":"+os.Getenv("PATH"), "TMPDIR="+tempDir)
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("%s escaped its runtime failure boundary: %v\n%s", tc.name, err, out)
			}
			if !strings.Contains(string(out), "SURVIVED") {
				t.Fatalf("%s did not reach the next command:\n%s", tc.name, out)
			}
			assertNoStagedSource(t, dir)
		})
	}
}

func TestLiveTerminalTimeoutsAreBoundedAndCleanedUp(t *testing.T) {
	realZsh, err := exec.LookPath("zsh")
	if err != nil {
		t.Skip("zsh not installed")
	}

	for _, tc := range []struct {
		name      string
		emitter   string
		validator string
	}{
		{name: "emitter", emitter: "#!/bin/sh\nexec sleep 5\n"},
		{name: "validator", emitter: "#!/bin/sh\nprintf '%s\\n' 'zp_apply() { :; }'\n", validator: "#!/bin/sh\nexec sleep 5\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			loader := writeLiveLoader(t, dir)
			writeLiveExecutable(t, filepath.Join(dir, "zsh-pro"), tc.emitter)
			if tc.validator != "" {
				writeLiveExecutable(t, filepath.Join(dir, "zsh"), tc.validator)
			}

			body := `
source "$1"
export ZSHPRO_PROFILE=good ZP_LAST_GOOD_PROFILE=good
setopt ERR_EXIT
activate bad
print -r -- SURVIVED
[[ "$ZP_LAST_RUNTIME_STATUS" -ne 0 ]] || exit 40
[[ "$ZSHPRO_PROFILE" == good ]] || exit 41
[[ "$ZP_LAST_GOOD_PROFILE" == good ]] || exit 42
`
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			started := time.Now()
			cmd := exec.CommandContext(ctx, realZsh, "-f", "-c", body, "zsh-pro-test", loader)
			cmd.Env = append(os.Environ(), "PATH="+dir+":"+os.Getenv("PATH"), "TMPDIR="+dir, "ZP_RUNTIME_TIMEOUT_SECONDS=1")
			out, err := cmd.CombinedOutput()
			elapsed := time.Since(started)
			if ctx.Err() == context.DeadlineExceeded {
				t.Fatalf("%s timeout needed the external Go watchdog after %s:\n%s", tc.name, elapsed, out)
			}
			if err != nil {
				t.Fatalf("%s timeout escaped its runtime failure boundary: %v\n%s", tc.name, err, out)
			}
			if elapsed > 2500*time.Millisecond {
				t.Fatalf("%s timeout took %s, want less than 2.5s", tc.name, elapsed)
			}
			if !strings.Contains(string(out), "SURVIVED") {
				t.Fatalf("%s timeout did not reach the next command:\n%s", tc.name, out)
			}
			assertNoStagedSource(t, dir)
		})
	}
}

func TestLiveTerminalStagingUsesExclusiveCreateAndCleansOnlyItsFile(t *testing.T) {
	realZsh, err := exec.LookPath("zsh")
	if err != nil {
		t.Skip("zsh not installed")
	}
	dir := t.TempDir()
	loader := writeLiveLoader(t, dir)
	body := `
source "$1"
RANDOM=1
collision="$TMPDIR/zsh-pro-eval-17767-$$"
print -r -- sentinel > "$collision"
_zp_eval_block $'zp_apply() { :; }\nzp_apply' good || exit 50
[[ "$(<"$collision")" == sentinel ]] || exit 51
rm -f -- "$collision"
`
	cmd := exec.Command(realZsh, "-f", "-c", body, "zsh-pro-test", loader)
	cmd.Env = append(os.Environ(), "TMPDIR="+dir, "PATH="+os.Getenv("PATH"))
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("staging collision changed an existing file or left temp state: %v\n%s", err, out)
	}
	assertNoStagedSource(t, dir)
}

func writeLiveLoader(t *testing.T, dir string) string {
	t.Helper()
	loader := filepath.Join(dir, "loader.zsh")
	if err := os.WriteFile(loader, []byte((Provider{}).HookScript()), 0o600); err != nil {
		t.Fatal(err)
	}
	return loader
}

func writeLiveExecutable(t *testing.T, path, source string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(source), 0o700); err != nil {
		t.Fatal(err)
	}
}

func assertNoStagedSource(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		switch entry.Name() {
		case "loader.zsh", "zsh-pro", "zsh", "not-a-directory":
			continue
		default:
			t.Fatalf("unexpected staged runtime file %q remains in %s", entry.Name(), dir)
		}
	}
}
