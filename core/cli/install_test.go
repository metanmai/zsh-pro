package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"zsh-pro/core/buildinfo"
	"zsh-pro/core/shell/zsh"
)

type staticHooker string

func (h staticHooker) HookScript() string { return string(h) }

type callbackHooker struct {
	script string
	before func()
}

func (h callbackHooker) HookScript() string {
	if h.before != nil {
		h.before()
	}
	return h.script
}

func setInstallHome(t *testing.T, home string) {
	t.Helper()
	t.Setenv("HOME", home)
	unsetInstallEnv(t, "ZDOTDIR")
	unsetInstallEnv(t, "ZSHPRO_HOME")
}

func unsetInstallEnv(t *testing.T, key string) {
	t.Helper()
	previous, wasSet := os.LookupEnv(key)
	if err := os.Unsetenv(key); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if wasSet {
			_ = os.Setenv(key, previous)
			return
		}
		_ = os.Unsetenv(key)
	})
}

func setTempWorkingDir(t *testing.T) string {
	t.Helper()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	workingDir := t.TempDir()
	if err := os.Chdir(workingDir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })
	return workingDir
}

func TestInstallIdempotentPreservesUserContent(t *testing.T) {
	home := t.TempDir()
	setInstallHome(t, home)
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

func TestInstallStoreInitializationFailureDoesNotMutateBootstrap(t *testing.T) {
	home := t.TempDir()
	setInstallHome(t, home)
	rc := filepath.Join(home, ".zshrc")
	before := []byte("export KEEP_ME=1\n")
	if err := os.WriteFile(rc, before, 0o600); err != nil {
		t.Fatal(err)
	}

	initErr := errors.New("profile store fixture unavailable")
	err := runInstallWithStoreInitialization(zsh.Provider{}, func(context.Context) error { return initErr })
	if !errors.Is(err, initErr) {
		t.Fatalf("install error = %v, want wrapped initialization error", err)
	}
	after, err := os.ReadFile(rc)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, before) {
		t.Fatalf("store initialization failure changed .zshrc:\n got: %q\nwant: %q", after, before)
	}
	if _, err := os.Lstat(filepath.Join(home, ".zsh-pro")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("store initialization failure created loader cache: %v", err)
	}
}

func TestInstallCollapsesBalancedDuplicatesAndPreservesInterveningContent(t *testing.T) {
	home := t.TempDir()
	setInstallHome(t, home)
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
		setInstallHome(t, home)
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
			setInstallHome(t, home)
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

func TestReplaceManagedBlockOnlyRecognizesExactPhysicalMarkerLines(t *testing.T) {
	replacement := []byte("replacement")
	appendBlock := func(current string) []byte {
		return append([]byte(current+"\n"), replacement...)
	}

	for _, tc := range []struct {
		name  string
		input string
		want  []byte
	}{
		{
			name:  "quoted marker text remains user content",
			input: "before\nprint -r -- '# >>> zsh-pro >>>'\nordinary-content-must-survive\nprint -r -- '# <<< zsh-pro <<<'\nafter\n",
			want:  appendBlock("before\nprint -r -- '# >>> zsh-pro >>>'\nordinary-content-must-survive\nprint -r -- '# <<< zsh-pro <<<'\nafter\n"),
		},
		{
			name:  "prefixed marker text remains user content",
			input: "prefix # >>> zsh-pro >>>\nordinary-content-must-survive\nprefix # <<< zsh-pro <<<\n",
			want:  appendBlock("prefix # >>> zsh-pro >>>\nordinary-content-must-survive\nprefix # <<< zsh-pro <<<\n"),
		},
		{
			name:  "suffixed marker text remains user content",
			input: "# >>> zsh-pro >>> # ordinary comment\nordinary-content-must-survive\n# <<< zsh-pro <<< # ordinary comment\n",
			want:  appendBlock("# >>> zsh-pro >>> # ordinary comment\nordinary-content-must-survive\n# <<< zsh-pro <<< # ordinary comment\n"),
		},
		{
			name:  "CRLF marker lines preserve surrounding CRLF bytes",
			input: "before\r\n# >>> zsh-pro >>>\r\nold\r\n# <<< zsh-pro <<<\r\nafter\r\n",
			want:  []byte("before\r\nreplacement\r\nafter\r\n"),
		},
		{
			name:  "no final newline is only changed by append separator",
			input: "ordinary-content-must-survive",
			want:  []byte("ordinary-content-must-survive\n\nreplacement"),
		},
		{
			name:  "duplicate real regions collapse while intervening bytes survive",
			input: "before\n# >>> zsh-pro >>>\nold\n# <<< zsh-pro <<<\nkeep-me\n# >>> zsh-pro >>>\nold-again\n# <<< zsh-pro <<<\nafter\n",
			want:  []byte("before\nreplacement\nkeep-me\n\nafter\n"),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := replaceManagedBlock([]byte(tc.input), replacement)
			if err != nil {
				t.Fatalf("replaceManagedBlock: %v", err)
			}
			if !bytes.Equal(got, tc.want) {
				t.Fatalf("replacement changed unmanaged bytes:\n got: %q\nwant: %q", got, tc.want)
			}
		})
	}
}

func TestReplaceManagedBlockRejectsEveryMalformedExactMarkerOrdering(t *testing.T) {
	for _, tc := range []struct {
		name  string
		input string
	}{
		{"begin only", "before\n# >>> zsh-pro >>>\n"},
		{"end only", "before\n# <<< zsh-pro <<<\n"},
		{"end before begin", "# <<< zsh-pro <<<\n# >>> zsh-pro >>>\n"},
		{"nested begins", "# >>> zsh-pro >>>\n# >>> zsh-pro >>>\n# <<< zsh-pro <<<\n"},
		{"stray end after region", "# >>> zsh-pro >>>\n# <<< zsh-pro <<<\n# <<< zsh-pro <<<\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := replaceManagedBlock([]byte(tc.input), []byte("replacement")); err == nil {
				t.Fatal("malformed exact marker ordering was accepted")
			}
		})
	}
}

// TestInstallRefusesMalformedExactMarkerOrderingsWithoutWriting pins the
// D-09/D-10 marker transaction before D-11/D-13/D-14 cache work. A malformed
// user-owned region must reject before either a known-good loader is promoted
// over or a new runtime directory is created.
func TestInstallRefusesMalformedExactMarkerOrderingsWithoutWriting(t *testing.T) {
	for _, tc := range []struct {
		name  string
		input string
	}{
		{"nested begins", "before\n# >>> zsh-pro >>>\n# >>> zsh-pro >>>\n# <<< zsh-pro <<<\nafter\n"},
		{"stray end after region", "before\n# >>> zsh-pro >>>\n# <<< zsh-pro <<<\n# <<< zsh-pro <<<\nafter\n"},
		{"unterminated begin", "before\n# >>> zsh-pro >>>\nafter\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, cache := range []struct {
				name string
				seed bool
			}{
				{name: "known good cache", seed: true},
				{name: "missing cache"},
			} {
				t.Run(cache.name, func(t *testing.T) {
					home := t.TempDir()
					setInstallHome(t, home)
					rc := filepath.Join(home, ".zshrc")
					before := []byte(tc.input)
					if err := os.WriteFile(rc, before, 0o600); err != nil {
						t.Fatal(err)
					}

					cacheDir := filepath.Join(home, ".zsh-pro")
					loader := filepath.Join(cacheDir, "loader.zsh")
					knownGood := []byte("# known-good cached loader\n")
					if cache.seed {
						if err := os.Mkdir(cacheDir, 0o700); err != nil {
							t.Fatal(err)
						}
						if err := os.WriteFile(loader, knownGood, 0o600); err != nil {
							t.Fatal(err)
						}
					}

					if err := runInstall(zsh.Provider{}); err == nil {
						t.Fatal("install accepted malformed exact marker ordering")
					}
					after, err := os.ReadFile(rc)
					if err != nil {
						t.Fatal(err)
					}
					if !bytes.Equal(after, before) {
						t.Fatalf("malformed input was written:\n got: %q\nwant: %q", after, before)
					}
					if cache.seed {
						got, err := os.ReadFile(loader)
						if err != nil || !bytes.Equal(got, knownGood) {
							t.Fatalf("known-good cache changed after malformed-marker rejection: %q, err=%v", got, err)
						}
						if _, err := os.Stat(cacheDir); err != nil {
							t.Fatalf("known-good cache directory disappeared: %v", err)
						}
						return
					}
					if _, err := os.Stat(cacheDir); !os.IsNotExist(err) {
						t.Fatalf("missing cache directory changed by malformed-marker rejection: %v", err)
					}
				})
			}
		})
	}
}

func TestInstallRejectsUnstablePathInputsBeforeMutation(t *testing.T) {
	for _, tc := range []struct {
		name      string
		configure func(t *testing.T)
	}{
		{"HOME unset", func(t *testing.T) { unsetInstallEnv(t, "HOME") }},
		{"HOME empty", func(t *testing.T) { t.Setenv("HOME", "") }},
		{"HOME relative", func(t *testing.T) { t.Setenv("HOME", "relative-home") }},
		{"ZDOTDIR empty", func(t *testing.T) { t.Setenv("ZDOTDIR", "") }},
		{"ZDOTDIR relative", func(t *testing.T) { t.Setenv("ZDOTDIR", "relative-zdotdir") }},
		{"ZSHPRO_HOME empty", func(t *testing.T) { t.Setenv("ZSHPRO_HOME", "") }},
		{"ZSHPRO_HOME relative", func(t *testing.T) { t.Setenv("ZSHPRO_HOME", "relative-zshpro") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			workingDir := setTempWorkingDir(t)
			setInstallHome(t, filepath.Join(workingDir, "home"))
			tc.configure(t)

			if err := runInstall(zsh.Provider{}); err == nil {
				t.Fatal("install accepted an unstable path input")
			}
			entries, err := os.ReadDir(workingDir)
			if err != nil {
				t.Fatal(err)
			}
			if len(entries) != 0 {
				t.Fatalf("install wrote relative to the working directory: %v", entries)
			}
		})
	}
}

func TestInstallRepairsCachedLoaderModeWithoutChangingZshrcMode(t *testing.T) {
	home := t.TempDir()
	setInstallHome(t, home)
	cacheDir := filepath.Join(home, ".zsh-pro")
	if err := os.Mkdir(cacheDir, 0o700); err != nil {
		t.Fatal(err)
	}
	loader := filepath.Join(cacheDir, "loader.zsh")
	if err := os.WriteFile(loader, []byte("old loader\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(loader, 0o644); err != nil {
		t.Fatal(err)
	}
	rc := filepath.Join(home, ".zshrc")
	if err := os.WriteFile(rc, []byte("export KEEP=1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(rc, 0o640); err != nil {
		t.Fatal(err)
	}

	if err := runInstall(zsh.Provider{}); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(loader); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("cached loader mode = %v, err=%v; want 0600", info.Mode().Perm(), err)
	}
	if info, err := os.Stat(rc); err != nil || info.Mode().Perm() != 0o640 {
		t.Fatalf("zshrc mode = %v, err=%v; want 0640", info.Mode().Perm(), err)
	}
}

func TestInstallInvalidCandidatePreservesKnownGoodCacheAndCleansStaging(t *testing.T) {
	if _, err := exec.LookPath("zsh"); err != nil {
		t.Skip("zsh not installed")
	}
	home := t.TempDir()
	setInstallHome(t, home)
	cacheDir := filepath.Join(home, ".zsh-pro")
	if err := os.Mkdir(cacheDir, 0o700); err != nil {
		t.Fatal(err)
	}
	loader := filepath.Join(cacheDir, "loader.zsh")
	oldLoader := []byte("known-good loader\n")
	if err := os.WriteFile(loader, oldLoader, 0o600); err != nil {
		t.Fatal(err)
	}
	rc := filepath.Join(home, ".zshrc")
	oldRC := []byte("export KEEP=1\n")
	if err := os.WriteFile(rc, oldRC, 0o600); err != nil {
		t.Fatal(err)
	}

	if err := runInstall(staticHooker("if then\n")); err == nil {
		t.Fatal("install accepted a syntax-invalid loader candidate")
	}
	if got, err := os.ReadFile(loader); err != nil || !bytes.Equal(got, oldLoader) {
		t.Fatalf("known-good cache changed after failed validation: %q, err=%v", got, err)
	}
	if got, err := os.ReadFile(rc); err != nil || !bytes.Equal(got, oldRC) {
		t.Fatalf("zshrc changed after failed validation: %q, err=%v", got, err)
	}
	entries, err := os.ReadDir(cacheDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "loader.zsh" {
		t.Fatalf("candidate staging was not cleaned: %v", entries)
	}
}

func TestInstallValidationDeadlinePreservesKnownGoodCache(t *testing.T) {
	home := t.TempDir()
	setInstallHome(t, home)
	cacheDir := filepath.Join(home, ".zsh-pro")
	if err := os.Mkdir(cacheDir, 0o700); err != nil {
		t.Fatal(err)
	}
	loader := filepath.Join(cacheDir, "loader.zsh")
	oldLoader := []byte("known-good loader\n")
	if err := os.WriteFile(loader, oldLoader, 0o600); err != nil {
		t.Fatal(err)
	}
	rc := filepath.Join(home, ".zshrc")
	oldRC := []byte("export KEEP=1\n")
	if err := os.WriteFile(rc, oldRC, 0o600); err != nil {
		t.Fatal(err)
	}
	binDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(binDir, "zsh"), []byte("#!/bin/sh\nexec /bin/sleep 6\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir)

	started := time.Now()
	err := runInstall(zsh.Provider{})
	if elapsed := time.Since(started); elapsed >= 5500*time.Millisecond {
		t.Fatalf("validation was not bounded: took %s", elapsed)
	}
	if err == nil {
		t.Fatal("install accepted a timed-out loader validation")
	}
	if got, readErr := os.ReadFile(loader); readErr != nil || !bytes.Equal(got, oldLoader) {
		t.Fatalf("known-good cache changed after timed-out validation: %q, err=%v", got, readErr)
	}
	if got, readErr := os.ReadFile(rc); readErr != nil || !bytes.Equal(got, oldRC) {
		t.Fatalf("zshrc changed after timed-out validation: %q, err=%v", got, readErr)
	}
}

func TestInstallPreservesSymlinkAndModesAndWritesSecureCacheFirst(t *testing.T) {
	home := t.TempDir()
	setInstallHome(t, home)
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
	setInstallHome(t, home)
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

func TestInstallZshrcPreparationFailurePreservesKnownGoodCache(t *testing.T) {
	seedCache := func(t *testing.T, home string) (string, []byte) {
		t.Helper()
		cacheDir := filepath.Join(home, ".zsh-pro")
		if err := os.Mkdir(cacheDir, 0o700); err != nil {
			t.Fatal(err)
		}
		loader := filepath.Join(cacheDir, "loader.zsh")
		knownGood := []byte("# known-good cached loader\n")
		if err := os.WriteFile(loader, knownGood, 0o600); err != nil {
			t.Fatal(err)
		}
		return loader, knownGood
	}
	assertUnchanged := func(t *testing.T, loader string, want []byte) {
		t.Helper()
		got, err := os.ReadFile(loader)
		if err != nil || !bytes.Equal(got, want) {
			t.Fatalf("known-good cache changed after .zshrc preparation failure: %q, err=%v", got, err)
		}
	}

	t.Run("dangling symlink", func(t *testing.T) {
		home := t.TempDir()
		setInstallHome(t, home)
		loader, knownGood := seedCache(t, home)
		if err := os.Symlink(filepath.Join(home, "missing", ".zshrc"), filepath.Join(home, ".zshrc")); err != nil {
			t.Fatal(err)
		}
		if err := runInstall(zsh.Provider{}); err == nil {
			t.Fatal("install accepted a dangling .zshrc target")
		}
		assertUnchanged(t, loader, knownGood)
	})

	t.Run("valid marker in unwritable target directory", func(t *testing.T) {
		home := t.TempDir()
		setInstallHome(t, home)
		loader, knownGood := seedCache(t, home)
		zdotdir := filepath.Join(home, "locked-zdotdir")
		if err := os.Mkdir(zdotdir, 0o700); err != nil {
			t.Fatal(err)
		}
		rc := filepath.Join(zdotdir, ".zshrc")
		before := append([]byte("export KEEP=1\n"), renderInstallBlock()...)
		if err := os.WriteFile(rc, before, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(zdotdir, 0o500); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(zdotdir, 0o700) })
		t.Setenv("ZDOTDIR", zdotdir)
		if err := runInstall(zsh.Provider{}); err == nil {
			t.Fatal("install accepted an unwritable .zshrc target directory")
		}
		assertUnchanged(t, loader, knownGood)
		got, err := os.ReadFile(rc)
		if err != nil || !bytes.Equal(got, before) {
			t.Fatalf("valid-marker .zshrc changed after preparation failure: %q, err=%v", got, err)
		}
	})
}

func TestInstallRollsBackLoaderWhenPreparedZshrcCannotBePromoted(t *testing.T) {
	if _, err := exec.LookPath("zsh"); err != nil {
		t.Skip("zsh not installed")
	}
	home := t.TempDir()
	setInstallHome(t, home)
	zdotdir := filepath.Join(home, "late-locked-zdotdir")
	if err := os.Mkdir(zdotdir, 0o700); err != nil {
		t.Fatal(err)
	}
	rc := filepath.Join(zdotdir, ".zshrc")
	beforeRC := append([]byte("export KEEP=1\n"), renderInstallBlock()...)
	if err := os.WriteFile(rc, beforeRC, 0o600); err != nil {
		t.Fatal(err)
	}
	cacheDir := filepath.Join(home, ".zsh-pro")
	if err := os.Mkdir(cacheDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	loader := filepath.Join(cacheDir, "loader.zsh")
	beforeLoader := []byte("# known-good cached loader\n")
	if err := os.WriteFile(loader, beforeLoader, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ZDOTDIR", zdotdir)
	locked := false
	t.Cleanup(func() {
		if locked {
			_ = os.Chmod(zdotdir, 0o700)
		}
	})
	hooker := callbackHooker{script: "typeset -g ZP_INSTALL_TRANSACTION_TEST=1\n", before: func() {
		if err := os.Chmod(zdotdir, 0o500); err != nil {
			t.Fatal(err)
		}
		locked = true
	}}
	if err := runInstall(hooker); err == nil {
		t.Fatal("install succeeded after the prepared .zshrc target became unwritable")
	}
	if err := os.Chmod(zdotdir, 0o700); err != nil {
		t.Fatal(err)
	}
	locked = false
	if got, err := os.ReadFile(loader); err != nil || !bytes.Equal(got, beforeLoader) {
		t.Fatalf("loader was not rolled back after .zshrc promotion failure: %q, err=%v", got, err)
	}
	if got, err := os.ReadFile(rc); err != nil || !bytes.Equal(got, beforeRC) {
		t.Fatalf("valid-marker .zshrc changed after failed promotion: %q, err=%v", got, err)
	}
	entries, err := os.ReadDir(cacheDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "loader.zsh" {
		t.Fatalf("loader rollback left cache staging behind: %v", entries)
	}
	if info, err := os.Stat(cacheDir); err != nil || info.Mode().Perm() != 0o755 {
		t.Fatalf("cache directory mode was not rolled back: %v, err=%v", info.Mode().Perm(), err)
	}
}

func TestRenderedStubIsFailOpenAndParseable(t *testing.T) {
	stub := string(renderInstallBlock())
	for _, want := range []string{"if [[ -z", "[[ -f", "-r", "if source", "else", "true"} {
		if !strings.Contains(stub, want) {
			t.Fatalf("stub missing %q", want)
		}
	}
	if strings.Contains(stub, "command -v zsh-pro") || strings.Contains(stub, "$(") {
		t.Fatal("stub invokes a command or command substitution during shell startup")
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
	t.Run("corrupt installed cache under hostile options", func(t *testing.T) {
		zshPath, err := exec.LookPath("zsh")
		if err != nil {
			t.Fatal(err)
		}
		home := t.TempDir()
		setInstallHome(t, home)
		if err := runInstall(zsh.Provider{}); err != nil {
			t.Fatalf("install loader: %v", err)
		}
		if err := os.WriteFile(filepath.Join(home, ".zsh-pro", "loader.zsh"), []byte("this is ( corrupt\n"), 0o600); err != nil {
			t.Fatal(err)
		}

		// Keep the old command guard live during RED. The repaired stub must
		// source only the regular cached file, but this makes the pre-fix
		// implementation reach its corrupt source path too.
		bin := t.TempDir()
		if err := os.WriteFile(filepath.Join(bin, "zsh-pro"), []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
			t.Fatal(err)
		}
		t.Setenv("PATH", bin)
		for _, option := range []string{"ERR_EXIT", "ERR_RETURN"} {
			t.Run(strings.ToLower(option), func(t *testing.T) {
				cmd := exec.Command(zshPath, "-f", "-c", "setopt "+option+"; source \"$1\"; print -r -- SURVIVED", "zsh-pro-test", filepath.Join(home, ".zshrc"))
				cmd.Env = os.Environ()
				out, err := cmd.CombinedOutput()
				if err != nil {
					t.Fatalf("corrupt installed cache aborted %s startup: %v\n%s", option, err, out)
				}
				if !strings.Contains(string(out), "SURVIVED") {
					t.Fatalf("corrupt installed cache did not reach the command after source under %s: %s", option, out)
				}
			})
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
