package zsh

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
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
checkout bad && exit 10
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
