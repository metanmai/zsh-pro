package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"zsh-pro/core/buildinfo"
	"zsh-pro/core/shell/zsh"
)

func TestInstallIdempotentPreservesUserContent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("ZSHPRO_HOME", "")
	rc := filepath.Join(home, ".zshrc")
	if err := os.WriteFile(rc, []byte("export EDITOR=nvim\nalias ll='ls -l'\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := runInstall(zsh.Provider{}); err != nil {
		t.Fatalf("first install: %v", err)
	}
	first, err := os.ReadFile(rc)
	if err != nil {
		t.Fatal(err)
	}
	if err := runInstall(zsh.Provider{}); err != nil {
		t.Fatalf("second install: %v", err)
	}
	second, err := os.ReadFile(rc)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("managed block changed on an unchanged second install")
	}
	if got := strings.Count(string(second), installBegin); got != 1 {
		t.Fatalf("BEGIN markers = %d, want 1", got)
	}
	if !strings.Contains(string(second), "export EDITOR=nvim") || !strings.Contains(string(second), "alias ll='ls -l'") {
		t.Fatal("installer did not preserve user content")
	}
}

func TestInstallCollapsesBalancedDuplicatesAndPreservesInterveningContent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("ZSHPRO_HOME", "")
	rc := filepath.Join(home, ".zshrc")
	before := "before\n" + installBegin + "\nold\n" + installEnd + "\nkeep-me\n" + installBegin + "\nold-again\n" + installEnd + "\nafter\n"
	if err := os.WriteFile(rc, []byte(before), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := runInstall(zsh.Provider{}); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(rc)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(got), installBegin) != 1 || !strings.Contains(string(got), "keep-me") || !strings.Contains(string(got), "before\n") || !strings.Contains(string(got), "after\n") {
		t.Fatalf("duplicate repair did not preserve unmanaged bytes:\n%s", got)
	}
}

func TestInstallCreatesAndRefusesUnbalancedMarkersWithoutWriting(t *testing.T) {
	t.Run("missing", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		t.Setenv("ZSHPRO_HOME", "")
		if err := runInstall(zsh.Provider{}); err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(filepath.Join(home, ".zshrc"))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Count(string(got), installBegin) != 1 {
			t.Fatalf("markers = %d", strings.Count(string(got), installBegin))
		}
	})
	for _, tc := range []struct{ name, content string }{
		{"begin only", "user\n" + installBegin + "\n"},
		{"end only", "user\n" + installEnd + "\n"},
		{"end before begin", installEnd + "\n" + installBegin + "\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("ZSHPRO_HOME", "")
			rc := filepath.Join(home, ".zshrc")
			if err := os.WriteFile(rc, []byte(tc.content), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := runInstall(zsh.Provider{}); err == nil {
				t.Fatal("install accepted malformed markers")
			}
			got, err := os.ReadFile(rc)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, []byte(tc.content)) {
				t.Fatalf("corrupt file changed: %q", got)
			}
		})
	}
}

func TestInstallPreservesSymlinkAndModesAndWritesSecureCacheFirst(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("ZSHPRO_HOME", "")
	realDir := t.TempDir()
	realRC := filepath.Join(realDir, "zshrc")
	if err := os.WriteFile(realRC, []byte("export X=1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(home, ".zshrc")
	if err := os.Symlink(realRC, link); err != nil {
		t.Fatal(err)
	}
	if err := runInstall(zsh.Provider{}); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Lstat(link); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("zshrc symlink replaced: %v %v", info, err)
	}
	if info, err := os.Stat(realRC); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("target mode = %v, err=%v", info.Mode().Perm(), err)
	}
	if got, _ := os.ReadFile(realRC); !bytes.Contains(got, []byte(installBegin)) {
		t.Fatal("real symlink target was not updated")
	}
	if info, err := os.Stat(filepath.Join(home, ".zsh-pro")); err != nil || info.Mode().Perm() != 0o700 {
		t.Fatalf("cache dir mode = %v, err=%v", info.Mode().Perm(), err)
	}
	loader := filepath.Join(home, ".zsh-pro", "loader.zsh")
	if info, err := os.Stat(loader); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("loader mode = %v, err=%v", info.Mode().Perm(), err)
	}
	if got, _ := os.ReadFile(loader); !bytes.Contains(got, []byte("cached loader version "+buildinfo.Version)) {
		t.Fatal("cached loader lacks version marker")
	}
}

func TestInstallCacheFailureDoesNotTouchZshrc(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	bad := filepath.Join(home, "not-a-dir")
	if err := os.WriteFile(bad, []byte("file"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ZSHPRO_HOME", bad)
	rc := filepath.Join(home, ".zshrc")
	before := []byte("export KEEP=1\n")
	if err := os.WriteFile(rc, before, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := runInstall(zsh.Provider{}); err == nil {
		t.Fatal("install succeeded with an invalid cache directory")
	}
	after, err := os.ReadFile(rc)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("zshrc changed after cached-loader failure")
	}
}

func TestRenderedStubIsFailOpenAndParseable(t *testing.T) {
	stub := string(renderInstallBlock())
	for _, want := range []string{"if [[ -z", "command -v zsh-pro", "[[ -r", "source", "true"} {
		if !strings.Contains(stub, want) {
			t.Fatalf("stub missing %q", want)
		}
	}
	if strings.Contains(stub, "\nreturn") {
		t.Fatal("stub contains a top-level return")
	}
	if _, err := exec.LookPath("zsh"); err == nil {
		cmd := exec.Command("zsh", "-n")
		cmd.Stdin = strings.NewReader(stub)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("stub does not parse: %v\n%s", err, out)
		}
		cmd = exec.Command("zsh", "-f", "-c", "setopt NO_UNSET WARN_CREATE_GLOBAL; source \"$1\"; [[ $? -eq 0 ]]", "zsh-pro-test", "/dev/stdin")
		cmd.Stdin = strings.NewReader(stub)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("stub is not fail-open under hostile options: %v\n%s", err, out)
		}
	}
}

func TestInstalledStubFailsOpenForDisabledMissingAndCorruptLoaders(t *testing.T) {
	if _, err := exec.LookPath("zsh"); err != nil {
		t.Skip("zsh not installed")
	}
	run := func(t *testing.T, env []string, body string) string {
		t.Helper()
		cmd := exec.Command("zsh", "-f", "-c", "source \"$1\"; "+body, "zsh-pro-test", filepath.Join(envValue(env, "HOME"), ".zshrc"))
		cmd.Env = append(os.Environ(), env...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("stub did not fail open: %v\n%s", err, out)
		}
		return string(out)
	}
	t.Run("missing binary", func(t *testing.T) {
		home := t.TempDir()
		if err := os.WriteFile(filepath.Join(home, ".zshrc"), renderInstallBlock(), 0o600); err != nil {
			t.Fatal(err)
		}
		out := run(t, []string{"HOME=" + home, "PATH=/usr/bin:/bin"}, "print -r -- after")
		if !strings.Contains(out, "after") {
			t.Fatal("line after stub did not run")
		}
	})
	t.Run("disabled", func(t *testing.T) {
		home, bin := t.TempDir(), t.TempDir()
		if err := os.WriteFile(filepath.Join(home, ".zshrc"), renderInstallBlock(), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(filepath.Join(home, ".zsh-pro"), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(home, ".zsh-pro", "loader.zsh"), []byte("typeset -g ZP_LOADED=1\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(bin, "zsh-pro"), []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
			t.Fatal(err)
		}
		out := run(t, []string{"HOME=" + home, "PATH=" + bin + ":/usr/bin:/bin", "ZSHPRO_DISABLE=1"}, "[[ -z \"${ZP_LOADED+x}\" ]] || exit 20; print -r -- after")
		if !strings.Contains(out, "after") {
			t.Fatal("line after disabled stub did not run")
		}
	})
	t.Run("corrupt cache", func(t *testing.T) {
		home, bin := t.TempDir(), t.TempDir()
		if err := os.WriteFile(filepath.Join(home, ".zshrc"), renderInstallBlock(), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(filepath.Join(home, ".zsh-pro"), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(home, ".zsh-pro", "loader.zsh"), []byte("this is ( corrupt\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(bin, "zsh-pro"), []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
			t.Fatal(err)
		}
		out := run(t, []string{"HOME=" + home, "PATH=" + bin + ":/usr/bin:/bin"}, "print -r -- after")
		if !strings.Contains(out, "after") {
			t.Fatal("corrupt loader blocked startup")
		}
	})
}

func envValue(env []string, name string) string {
	prefix := name + "="
	for _, value := range env {
		if strings.HasPrefix(value, prefix) {
			return strings.TrimPrefix(value, prefix)
		}
	}
	return ""
}
