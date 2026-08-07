package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"zsh-pro/core/buildinfo"
	"zsh-pro/core/model"
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

// TestBuiltBinaryInstallRejectsTrailingArgumentsWithoutMutation exercises the
// real composition root. A dispatcher regression here would otherwise create a
// profile store, cached loader, and managed .zshrc before the installer sees
// that an accidental trailing argument was supplied.
func TestBuiltBinaryInstallRejectsTrailingArgumentsWithoutMutation(t *testing.T) {
	binary := buildZshProBinary(t)

	for _, fixture := range []struct {
		name        string
		preexisting bool
	}{
		{name: "fresh home"},
		{name: "preexisting home", preexisting: true},
	} {
		for _, trailing := range []string{"--help", "unexpected-argument"} {
			t.Run(fixture.name+"/"+trailing, func(t *testing.T) {
				home := installArgumentFixture(t, fixture.preexisting)
				before := snapshotInstallTree(t, home)

				cmd := exec.Command(binary, "install", trailing)
				cmd.Env = installBinaryEnv(home)
				out, err := cmd.CombinedOutput()
				if err == nil {
					t.Fatalf("install %q unexpectedly succeeded: %s", trailing, out)
				}
				var exitErr *exec.ExitError
				if !errors.As(err, &exitErr) || exitErr.ExitCode() != int(model.ExitUsageErr) {
					t.Fatalf("install %q exit = %v, want %d; output: %s", trailing, err, model.ExitUsageErr, out)
				}
				if got, want := string(out), "usage: zsh-pro install\n"; got != want {
					t.Fatalf("install %q output = %q, want %q", trailing, got, want)
				}

				after := snapshotInstallTree(t, home)
				if !bytes.Equal(after, before) {
					t.Fatalf("install %q mutated home despite usage rejection:\n before: %q\n  after: %q", trailing, before, after)
				}
				if !fixture.preexisting {
					for _, path := range []string{
						filepath.Join(home, ".zshrc"),
						filepath.Join(home, ".zsh-pro"),
						filepath.Join(home, ".local", "share", "zsh-pro"),
					} {
						if _, statErr := os.Lstat(path); !errors.Is(statErr, os.ErrNotExist) {
							t.Fatalf("install %q created %s: %v", trailing, path, statErr)
						}
					}
				}
			})
		}
	}
}

func buildZshProBinary(t *testing.T) string {
	t.Helper()
	root := repositoryRoot(t)
	binary := filepath.Join(t.TempDir(), "zsh-pro")
	cmd := exec.Command("go", "build", "-o", binary, "./core/cmd/zsh-pro")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build zsh-pro binary: %v\n%s", err, out)
	}
	return binary
}

func repositoryRoot(t *testing.T) string {
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
			t.Fatal("could not find repository go.mod")
		}
		dir = parent
	}
}

func installArgumentFixture(t *testing.T, preexisting bool) string {
	t.Helper()
	home := t.TempDir()
	if !preexisting {
		return home
	}
	for _, fixture := range []struct {
		path string
		data string
		mode os.FileMode
	}{
		{path: ".zshrc", data: "export KEEP_ZSHRC=1\n", mode: 0o600},
		{path: ".zsh-pro/loader.zsh", data: "# preserve cached loader\n", mode: 0o600},
		{path: ".local/share/zsh-pro/profiles/sentinel", data: "preserve profile store bytes\n", mode: 0o640},
	} {
		path := filepath.Join(home, fixture.path)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(fixture.data), fixture.mode); err != nil {
			t.Fatal(err)
		}
	}
	return home
}

func installBinaryEnv(home string) []string {
	removed := map[string]bool{
		"HOME":          true,
		"XDG_DATA_HOME": true,
		"ZDOTDIR":       true,
		"ZSHPRO_HOME":   true,
	}
	env := make([]string, 0, len(os.Environ())+1)
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if !removed[key] {
			env = append(env, entry)
		}
	}
	return append(env, "HOME="+home)
}

func snapshotInstallTree(t *testing.T, root string) []byte {
	t.Helper()
	var snapshot bytes.Buffer
	var walk func(string)
	walk = func(dir string) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			path := filepath.Join(dir, entry.Name())
			rel, err := filepath.Rel(root, path)
			if err != nil {
				t.Fatal(err)
			}
			info, err := os.Lstat(path)
			if err != nil {
				t.Fatal(err)
			}
			_, _ = fmt.Fprintf(&snapshot, "%q mode=%#o\n", rel, info.Mode())
			switch {
			case info.Mode().IsDir():
				walk(path)
			case info.Mode()&os.ModeSymlink != 0:
				target, err := os.Readlink(path)
				if err != nil {
					t.Fatal(err)
				}
				_, _ = fmt.Fprintf(&snapshot, "target=%q\n", target)
			case info.Mode().IsRegular():
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				_, _ = fmt.Fprintf(&snapshot, "bytes=%d\n", len(data))
				_, _ = snapshot.Write(data)
				_ = snapshot.WriteByte('\n')
			}
		}
	}
	walk(root)
	return snapshot.Bytes()
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
	err := runInstallWithStoreInitialization(zsh.Provider{}, func(context.Context) (StoreInitialization, error) {
		return StoreInitialization{}, initErr
	})
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

// TestInstallRejectsSymlinkedCachePathsWithoutMutation keeps the user-selected
// .zshrc compatibility policy separate from generated-cache policy. The cache
// must reject a symlink before it chmods, writes, or stages under its target.
func TestInstallRejectsSymlinkedCachePathsWithoutMutation(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("descriptor-authenticated cached-loader install is unsupported on this platform")
	}

	for _, root := range []struct {
		name     string
		cacheDir func(string) string
	}{
		{
			name: "default HOME cache",
			cacheDir: func(home string) string {
				return filepath.Join(home, ".zsh-pro")
			},
		},
		{
			name: "explicit cache root",
			cacheDir: func(home string) string {
				return filepath.Join(home, "explicit-cache")
			},
		},
	} {
		root := root
		t.Run("symlinked root "+root.name, func(t *testing.T) {
			home := t.TempDir()
			setInstallHome(t, home)
			cacheDir := root.cacheDir(home)
			if cacheDir != filepath.Join(home, ".zsh-pro") {
				t.Setenv("ZSHPRO_HOME", cacheDir)
			}
			beforeRC := []byte("export KEEP_CACHE_ROOT=1\n")
			rc := filepath.Join(home, ".zshrc")
			if err := os.WriteFile(rc, beforeRC, 0o600); err != nil {
				t.Fatal(err)
			}
			victim := t.TempDir()
			if err := os.Chmod(victim, 0o755); err != nil {
				t.Fatal(err)
			}
			sentinel := filepath.Join(victim, "unrelated.txt")
			beforeSentinel := []byte("do not modify this directory\n")
			if err := os.WriteFile(sentinel, beforeSentinel, 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(victim, cacheDir); err != nil {
				t.Fatal(err)
			}

			if err := runInstall(zsh.Provider{}); err == nil {
				t.Fatal("install accepted a symlinked cached-loader directory")
			}
			assertInstallSymlink(t, cacheDir, victim)
			assertInstallBytesAndMode(t, sentinel, beforeSentinel, 0o644)
			info, err := os.Stat(victim)
			if err != nil {
				t.Fatal(err)
			}
			if info.Mode().Perm() != 0o755 {
				t.Fatalf("unrelated directory mode = %v, want 0755", info.Mode().Perm())
			}
			if _, err := os.Lstat(filepath.Join(victim, cacheLoaderName)); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("install created a cached loader in the symlink target: %v", err)
			}
			assertInstallBytesAndMode(t, rc, beforeRC, 0o600)
		})

		t.Run("symlinked loader "+root.name, func(t *testing.T) {
			home := t.TempDir()
			setInstallHome(t, home)
			cacheDir := root.cacheDir(home)
			if cacheDir != filepath.Join(home, ".zsh-pro") {
				t.Setenv("ZSHPRO_HOME", cacheDir)
			}
			if err := os.Mkdir(cacheDir, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(cacheDir, 0o755); err != nil {
				t.Fatal(err)
			}
			beforeRC := []byte("export KEEP_CACHE_FILE=1\n")
			rc := filepath.Join(home, ".zshrc")
			if err := os.WriteFile(rc, beforeRC, 0o600); err != nil {
				t.Fatal(err)
			}
			victim := filepath.Join(t.TempDir(), "unrelated-loader.zsh")
			beforeVictim := []byte("DO NOT OVERWRITE\n")
			if err := os.WriteFile(victim, beforeVictim, 0o644); err != nil {
				t.Fatal(err)
			}
			loader := filepath.Join(cacheDir, cacheLoaderName)
			if err := os.Symlink(victim, loader); err != nil {
				t.Fatal(err)
			}

			if err := runInstall(zsh.Provider{}); err == nil {
				t.Fatal("install accepted a symlinked cached loader")
			}
			assertInstallSymlink(t, loader, victim)
			assertInstallBytesAndMode(t, victim, beforeVictim, 0o644)
			assertInstallBytesAndMode(t, rc, beforeRC, 0o600)
			if info, err := os.Stat(cacheDir); err != nil || info.Mode().Perm() != 0o755 {
				t.Fatalf("loader-symlink rejection changed cache directory mode: %v, err=%v", info, err)
			}
			entries, err := os.ReadDir(cacheDir)
			if err != nil {
				t.Fatal(err)
			}
			if len(entries) != 1 || entries[0].Name() != cacheLoaderName {
				t.Fatalf("symlink rejection left cache staging behind: %v", entries)
			}
		})
	}

	t.Run("symlinked cache ancestor", func(t *testing.T) {
		home := t.TempDir()
		setInstallHome(t, home)
		victim := t.TempDir()
		if err := os.Chmod(victim, 0o755); err != nil {
			t.Fatal(err)
		}
		ancestor := filepath.Join(home, "cache-ancestor")
		if err := os.Symlink(victim, ancestor); err != nil {
			t.Fatal(err)
		}
		cacheDir := filepath.Join(ancestor, "generated-cache")
		t.Setenv("ZSHPRO_HOME", cacheDir)
		rc := filepath.Join(home, ".zshrc")
		beforeRC := []byte("export KEEP_CACHE_ANCESTOR=1\n")
		if err := os.WriteFile(rc, beforeRC, 0o600); err != nil {
			t.Fatal(err)
		}

		if err := runInstall(zsh.Provider{}); err == nil {
			t.Fatal("install accepted a symlinked cached-loader ancestor")
		}
		assertInstallSymlink(t, ancestor, victim)
		if _, err := os.Lstat(filepath.Join(victim, "generated-cache")); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("install created a cache through the symlinked ancestor: %v", err)
		}
		info, err := os.Stat(victim)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o755 {
			t.Fatalf("symlinked-ancestor victim mode = %v, want 0755", info.Mode().Perm())
		}
		assertInstallBytesAndMode(t, rc, beforeRC, 0o600)
	})

	t.Run("non-directory cache ancestor", func(t *testing.T) {
		home := t.TempDir()
		setInstallHome(t, home)
		ancestor := filepath.Join(home, "cache-ancestor-file")
		beforeAncestor := []byte("not a cache directory\n")
		if err := os.WriteFile(ancestor, beforeAncestor, 0o644); err != nil {
			t.Fatal(err)
		}
		t.Setenv("ZSHPRO_HOME", filepath.Join(ancestor, "generated-cache"))
		rc := filepath.Join(home, ".zshrc")
		beforeRC := []byte("export KEEP_CACHE_FILE_ANCESTOR=1\n")
		if err := os.WriteFile(rc, beforeRC, 0o600); err != nil {
			t.Fatal(err)
		}

		if err := runInstall(zsh.Provider{}); err == nil {
			t.Fatal("install accepted a non-directory cached-loader ancestor")
		}
		assertInstallBytesAndMode(t, ancestor, beforeAncestor, 0o644)
		assertInstallBytesAndMode(t, rc, beforeRC, 0o600)
	})
}

func TestCacheRollbackRefusesToRemoveAReplacedDirectory(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("descriptor-authenticated cached-loader install is unsupported on this platform")
	}
	home := t.TempDir()
	cacheDir := filepath.Join(home, "cache")
	state, err := secureCacheDirectory(cacheDir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = state.close() }()

	moved := filepath.Join(home, "original-cache")
	if err := os.Rename(cacheDir, moved); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := state.rollback(); err == nil {
		t.Fatal("rollback removed or accepted a replacement cache directory")
	}
	if info, err := os.Stat(cacheDir); err != nil || !info.IsDir() || info.Mode().Perm() != 0o755 {
		t.Fatalf("replacement cache directory changed: %v, err=%v", info, err)
	}
	if info, err := os.Stat(moved); err != nil || !info.IsDir() {
		t.Fatalf("original cache directory changed: %v, err=%v", info, err)
	}
}

func assertInstallSymlink(t *testing.T, path, wantTarget string) {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("%s is no longer a symlink: %v, err=%v", path, info, err)
	}
	if got, err := os.Readlink(path); err != nil || got != wantTarget {
		t.Fatalf("symlink %s target = %q, err=%v; want %q", path, got, err, wantTarget)
	}
}

func assertInstallBytesAndMode(t *testing.T, path string, want []byte, mode os.FileMode) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("%s bytes = %q, err=%v; want %q", path, got, err, want)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != mode {
		t.Fatalf("%s mode = %v, want %v", path, info.Mode().Perm(), mode)
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

func TestInstallInitializesStoreBeforeCache(t *testing.T) {
	home := t.TempDir()
	setInstallHome(t, home)
	runtimeDir := filepath.Join(home, ".zsh-pro")
	var events []string

	initializer := func(context.Context) (StoreInitialization, error) {
		if _, err := os.Lstat(runtimeDir); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("runtime cache existed before Store initialization: %v", err)
		}
		events = append(events, "initializer")
		return StoreInitialization{
			CreatedPath: filepath.Join(home, ".local", "share", "zsh-pro", "profiles"),
		}, nil
	}
	hooker := callbackHooker{script: "typeset -g ZP_INSTALL_ORDER=1\n", before: func() {
		if _, err := os.Stat(runtimeDir); err != nil {
			t.Fatalf("runtime cache was not ready before loader preparation: %v", err)
		}
		events = append(events, "cache")
	}}

	if err := runInstallWithStoreInitialization(hooker, initializer); err != nil {
		t.Fatalf("install: %v", err)
	}
	if got, want := strings.Join(events, ","), "initializer,cache"; got != want {
		t.Fatalf("install order = %q, want %q", got, want)
	}
}

func TestInstallExplicitSharedRootOrdersStoreBeforeCache(t *testing.T) {
	home := t.TempDir()
	setInstallHome(t, home)
	shared := filepath.Join(home, "shared-zsh-pro")
	t.Setenv("ZSHPRO_HOME", shared)
	before := []byte("export KEEP_SHARED_ROOT=1\n")
	if err := os.WriteFile(filepath.Join(home, ".zshrc"), before, 0o600); err != nil {
		t.Fatal(err)
	}
	var events []string
	initializer := func(context.Context) (StoreInitialization, error) {
		events = append(events, "initializer")
		if err := os.Mkdir(shared, 0o700); err != nil {
			return StoreInitialization{}, err
		}
		return StoreInitialization{
			CreatedPath: shared,
			Rollback: func() error {
				events = append(events, "initializer-rollback")
				entries, err := os.ReadDir(shared)
				if err == nil && len(entries) != 0 {
					return fmt.Errorf("runtime cache was not rolled back before initializer: %v", entries)
				}
				return os.Remove(shared)
			},
		}, nil
	}

	err := runInstallWithStoreInitialization(staticHooker("if then\n"), initializer)
	if err == nil || !strings.Contains(err.Error(), "install cached loader") {
		t.Fatalf("install error = %v, want injected loader failure", err)
	}
	if got, want := strings.Join(events, ","), "initializer,initializer-rollback"; got != want {
		t.Fatalf("shared-root compensation order = %q, want %q", got, want)
	}
	if got, readErr := os.ReadFile(filepath.Join(home, ".zshrc")); readErr != nil || !bytes.Equal(got, before) {
		t.Fatalf("shared-root failure changed target: %q, err=%v", got, readErr)
	}
	if _, err := os.Lstat(shared); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("shared-root compensation retained runtime/store state: %v", err)
	}
}

func TestInstallRejectsAncestorStoreCacheOverlap(t *testing.T) {
	for _, tc := range []struct {
		name      string
		storeRoot func(string) string
	}{
		{name: "store contains cache", storeRoot: func(home string) string { return filepath.Join(home, "overlap") }},
		{name: "cache contains store", storeRoot: func(home string) string { return filepath.Join(home, "overlap", "store") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			setInstallHome(t, home)
			cacheRoot := filepath.Join(home, "overlap", "cache")
			if tc.name == "cache contains store" {
				cacheRoot = filepath.Join(home, "overlap")
			}
			t.Setenv("ZSHPRO_HOME", cacheRoot)
			before := []byte("export KEEP_OVERLAP=1\n")
			if err := os.WriteFile(filepath.Join(home, ".zshrc"), before, 0o600); err != nil {
				t.Fatal(err)
			}
			rollbackCalls := 0
			err := runInstallWithStoreInitialization(staticHooker("typeset -g ZP_OVERLAP=1\n"), func(context.Context) (StoreInitialization, error) {
				return StoreInitialization{
					CreatedPath: tc.storeRoot(home),
					Rollback: func() error {
						rollbackCalls++
						return nil
					},
				}, nil
			})
			if err == nil || !strings.Contains(err.Error(), "overlap") {
				t.Fatalf("install error = %v, want ancestor-overlap refusal", err)
			}
			if rollbackCalls != 1 {
				t.Fatalf("initializer rollback calls = %d, want 1", rollbackCalls)
			}
			if _, err := os.Lstat(cacheRoot); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("ancestor-overlap refusal created cache: %v", err)
			}
			if got, readErr := os.ReadFile(filepath.Join(home, ".zshrc")); readErr != nil || !bytes.Equal(got, before) {
				t.Fatalf("ancestor-overlap refusal changed target: %q, err=%v", got, readErr)
			}
		})
	}
}

func TestInstallSnapshotMatchesBoundedFields(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "zshrc")
	if err := os.WriteFile(target, []byte("export SNAPSHOT=1\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	requested := filepath.Join(dir, ".zshrc")
	if err := os.Symlink(target, requested); err != nil {
		t.Fatal(err)
	}
	prepared, err := prepareIngestInstallAt(requested, renderInstallBlock())
	if err != nil {
		t.Fatal(err)
	}
	snapshot := prepared.originalSnapshot
	if matches, err := snapshot.matchesCurrent(); err != nil || !matches {
		t.Fatalf("fresh snapshot match = (%v, %v), want true", matches, err)
	}

	changedTime := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(target, changedTime, changedTime); err != nil {
		t.Fatal(err)
	}
	if matches, err := snapshot.matchesCurrent(); err != nil || !matches {
		t.Fatalf("timestamp-only change rejected bounded snapshot: (%v, %v)", matches, err)
	}
	if err := os.Chmod(target, 0o600); err != nil {
		t.Fatal(err)
	}
	if matches, err := snapshot.matchesCurrent(); err != nil || matches {
		t.Fatalf("permission change match = (%v, %v), want false", matches, err)
	}
	if err := os.Chmod(target, 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("export SNAPSHOT=2\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	if matches, err := snapshot.matchesCurrent(); err != nil || matches {
		t.Fatalf("content change match = (%v, %v), want false", matches, err)
	}
}

func TestScanZshrcMarkerTopologyTable(t *testing.T) {
	for _, tc := range []struct {
		name    string
		input   string
		want    markerTopology
		regions int
		bad     bool
	}{
		{name: "none", input: "export A=1\n", want: markerTopologyNone},
		{name: "one balanced", input: installBegin + "\nold\n" + installEnd + "\n", want: markerTopologySingle, regions: 1},
		{name: "multiple balanced", input: installBegin + "\na\n" + installEnd + "\nkeep\n" + installBegin + "\nb\n" + installEnd + "\n", want: markerTopologyMultiple, regions: 2},
		{name: "orphan end", input: installEnd + "\n", bad: true},
		{name: "unterminated start", input: installBegin + "\n", bad: true},
		{name: "nested", input: installBegin + "\n" + installBegin + "\n" + installEnd + "\n", bad: true},
		{name: "interleaved", input: installBegin + "\n" + installEnd + "\n" + installEnd + "\n", bad: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			layout, err := scanZshrcMarkerTopology([]byte(tc.input))
			if tc.bad {
				if err == nil {
					t.Fatalf("malformed topology accepted: %#v", layout)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if layout.topology != tc.want || len(layout.regions) != tc.regions {
				t.Fatalf("layout = (%v, %d regions), want (%v, %d)", layout.topology, len(layout.regions), tc.want, tc.regions)
			}
		})
	}
}

func TestPrepareIngestPreservesEveryOutsideMarkerByte(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".zshrc")
	input := []byte("export MANAGED=1\r\n" +
		"if true; then print imperative; fi\r\n" +
		"opaque syntax ???\r\n" +
		"# gap follows\r\n\r\n" +
		installBegin + "\r\nold managed bytes\r\n" + installEnd + "\r\n" +
		"export POST_END=1")
	if err := os.WriteFile(path, input, 0o600); err != nil {
		t.Fatal(err)
	}
	block := []byte(installBegin + "\ncanonical\n" + installEnd)
	prepared, err := prepareIngestInstallAt(path, block)
	if err != nil {
		t.Fatal(err)
	}
	wantSource := []byte("export MANAGED=1\r\nif true; then print imperative; fi\r\nopaque syntax ???\r\n# gap follows\r\n\r\n\r\nexport POST_END=1")
	if !bytes.Equal(prepared.eligibleSource, wantSource) {
		t.Fatalf("eligible source = %q, want %q", prepared.eligibleSource, wantSource)
	}
	wantCandidate := []byte("export MANAGED=1\r\nif true; then print imperative; fi\r\nopaque syntax ???\r\n# gap follows\r\n\r\n" + string(block) + "\r\nexport POST_END=1")
	if !bytes.Equal(prepared.candidate, wantCandidate) {
		t.Fatalf("candidate = %q, want %q", prepared.candidate, wantCandidate)
	}
	if prepared.appendWarning == "" {
		t.Fatal("post-END ordinary content produced no warning")
	}
}

func TestPrepareIngestTopologyModes(t *testing.T) {
	block := []byte(installBegin + "\nnew\n" + installEnd)
	for _, tc := range []struct {
		name  string
		input string
		want  string
	}{
		{name: "empty", input: "", want: string(block) + "\n"},
		{name: "no marker without final newline", input: "export A=1", want: "export A=1\n\n" + string(block)},
		{name: "one balanced", input: "before\n" + installBegin + "\nold\n" + installEnd + "\nafter\n", want: "before\n" + string(block) + "\nafter\n"},
		{name: "balanced duplicates", input: installBegin + "\na\n" + installEnd + "\n" + installBegin + "\nb\n" + installEnd + "\n", want: string(block) + "\n\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), ".zshrc")
			if err := os.WriteFile(path, []byte(tc.input), 0o600); err != nil {
				t.Fatal(err)
			}
			prepared, err := prepareIngestInstallAt(path, block)
			if err != nil {
				t.Fatal(err)
			}
			if got := string(prepared.candidate); got != tc.want {
				t.Fatalf("candidate = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestPrepareIngestAppendWarningRetainsEligibleSource(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".zshrc")
	post := "alias after_end='kept'\n"
	input := "before\n" + installBegin + "\nold\n" + installEnd + "\n# comment only\n\n" + post
	if err := os.WriteFile(path, []byte(input), 0o600); err != nil {
		t.Fatal(err)
	}
	prepared, err := prepareIngestInstallAt(path, renderInstallBlock())
	if err != nil {
		t.Fatal(err)
	}
	if prepared.appendWarning != ingestPostEndWarning {
		t.Fatalf("warning = %q, want %q", prepared.appendWarning, ingestPostEndWarning)
	}
	if !bytes.Contains(prepared.eligibleSource, []byte(post)) || !bytes.Contains(prepared.candidate, []byte(post)) {
		t.Fatal("post-END source was not retained in both logical and physical products")
	}
}

func TestPrepareIngestRepairsBalancedDuplicates(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".zshrc")
	input := installBegin + "\nold-a\n" + installEnd + "\n" + installBegin + "\nold-b\n" + installEnd + "\n"
	if err := os.WriteFile(path, []byte(input), 0o600); err != nil {
		t.Fatal(err)
	}
	prepared, err := prepareIngestInstallAt(path, renderInstallBlock())
	if err != nil {
		t.Fatal(err)
	}
	if prepared.layout.topology != markerTopologyMultiple {
		t.Fatalf("topology = %v, want duplicate repair", prepared.layout.topology)
	}
	if got := strings.Count(string(prepared.candidate), installBegin); got != 1 {
		t.Fatalf("candidate BEGIN markers = %d, want 1", got)
	}
	if bytes.Contains(prepared.eligibleSource, []byte("old-a")) || bytes.Contains(prepared.eligibleSource, []byte("old-b")) {
		t.Fatal("tool-owned duplicate-region bytes entered eligible source")
	}
	if prepared.appendWarning != "" {
		t.Fatalf("duplicate region was misclassified as post-END ordinary content: %q", prepared.appendWarning)
	}
}

func requireTask3InstallContract(t *testing.T, contract string) {
	t.Helper()
	switch contract {
	case "pure check and filesystem probe ordering":
		testTask3Ordering(t)
	case "unsupported isolation":
		testTask3UnsupportedIsolation(t)
	case "probe cleanup uncertainty":
		testTask3ProbeCleanupUncertainty(t)
	case "candidate fsync barrier":
		testTask3PreparationBarrier(t, "candidate", false)
	case "artifact directory fsync barrier":
		testTask3PreparationBarrier(t, "artifact-directory", false)
	case "prepared journal directory fsync barrier":
		testTask3PreparationBarrier(t, "journal-directory:prepared", true)
	case "journal transition directory fsync":
		testTask3PromotionSyncFailure(t, "journal-directory:exchanged")
	case "late substitution reverse":
		testTask3LateSubstitution(t, false)
	case "unsafe reverse refusal":
		testTask3LateSubstitution(t, true)
	case "late absent-target creation":
		testTask3LateAbsentTarget(t)
	case "post-promotion fsync uncertainty":
		testTask3PromotionSyncFailure(t, "target-parent:forward-exchange")
	case "existing recovery before exchange":
		testTask3RecoveryRow(t, true, "before")
	case "existing recovery after exchange":
		testTask3RecoveryRow(t, true, "after-namespace")
	case "existing recovery after journal advance":
		testTask3RecoveryRow(t, true, "after-journal")
	case "existing recovery after parent sync":
		testTask3RecoveryRow(t, true, "after-parent")
	case "absent recovery before create":
		testTask3RecoveryRow(t, false, "before")
	case "absent recovery after no-replace":
		testTask3RecoveryRow(t, false, "after-namespace")
	case "absent recovery after journal advance":
		testTask3RecoveryRow(t, false, "after-journal")
	case "absent recovery after parent sync":
		testTask3RecoveryRow(t, false, "after-parent")
	case "in-place target mutation":
		testTask3ChangedAxis(t, true)
	case "adapter and filesystem unsupported":
		testTask3UnsupportedRows(t)
	case "cross-process root lock":
		testTask3RootLock(t)
	case "unsafe root lock entry":
		testTask3UnsafeLock(t)
	case "unauthenticated cleanup refusal":
		testTask3UnauthenticatedCleanup(t)
	case "changed parent identity":
		testTask3ChangedParent(t)
	case "journal identity and digest matrix":
		testTask3ChangedJournal(t)
	case "requested link topology":
		testTask3ChangedLink(t)
	case "one exchange peer":
		testTask3OneExchangePeer(t)
	case "secret peer finalize cleanup":
		testTask3SecretPeer(t, false)
	case "secret peer recovery retention":
		testTask3SecretPeer(t, true)
	case "absent rollback refusal":
		testTask3AbsentRollback(t)
	case "changed target rollback refusal":
		testTask3ChangedRollback(t)
	case "mutation surface AST audit":
		testTask3MutationSurface(t)
	case "filesystem-first stale compensation":
		testTask3StaleCompensation(t)
	case "expected target axis":
		testTask3ChangedAxis(t, true)
	case "expected candidate axis":
		testTask3ChangedAxis(t, false)
	case "post-swap independent axes":
		testTask3PostSwapAxes(t)
	default:
		t.Fatalf("unknown Task 3 contract %q", contract)
	}
}

type task3InstallFixture struct {
	home      string
	target    string
	original  []byte
	prepared  preparedIngestInstall
	candidate []byte
}

func newTask3InstallFixture(t *testing.T, existing bool, block []byte) task3InstallFixture {
	t.Helper()
	home := t.TempDir()
	if err := os.Chmod(home, 0o700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(home, ".zshrc")
	original := []byte("export ORIGINAL=1\n")
	if existing {
		if err := os.WriteFile(target, original, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if block == nil {
		block = renderInstallBlock()
	}
	prepared, err := prepareIngestInstallAt(target, block)
	if err != nil {
		t.Fatal(err)
	}
	return task3InstallFixture{
		home: home, target: target, original: original, prepared: prepared,
		candidate: append([]byte(nil), prepared.candidate...),
	}
}

func prepareTask3Transaction(
	t *testing.T,
	existing bool,
	seams *installTransactionSeams,
	block []byte,
) (task3InstallFixture, *guardedInstallTransaction) {
	t.Helper()
	fixture := newTask3InstallFixture(t, existing, block)
	if err := preflightAtomicRenameTarget(fixture.target, seams); err != nil {
		t.Fatalf("atomic preflight: %v", err)
	}
	transaction, err := prepareGuardedInstallTransaction(fixture.prepared, seams)
	if err != nil {
		t.Fatalf("prepare guarded transaction: %v", err)
	}
	t.Cleanup(transaction.closeHandles)
	return fixture, transaction
}

func targetAtomicCountingSeams(counter *atomic.Int32) *installTransactionSeams {
	return &installTransactionSeams{
		atomicRename: func(fromFD int, from string, toFD int, to string, mode atomicRenameMode) error {
			if to == ".zshrc" {
				counter.Add(1)
			}
			return atomicRenameBetweenAt(fromFD, from, toFD, to, mode)
		},
	}
}

func readTask3Target(t *testing.T, path string) []byte {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return content
}

func testTask3Ordering(t *testing.T) {
	home := t.TempDir()
	setInstallHome(t, home)
	if err := os.WriteFile(filepath.Join(home, ".zshrc"), []byte("export A=1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var events []string
	seams := &installTransactionSeams{event: func(event string) { events = append(events, event) }}
	initializer := func(context.Context) (StoreInitialization, error) {
		return StoreInitialization{Finalize: func() error { return nil }}, nil
	}
	if err := runInstallWithStoreInitializationAndSeams(staticHooker("typeset -g TASK3_ORDER=1\n"), initializer, seams); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"capability:exchange", "capability:no-replace", "probe:locked", "probe:exchange",
		"probe:no-replace", "probe:cleanup", "initializer", "transaction:prepared", "cache", "loader", "target:namespace",
	}
	if strings.Join(events, ",") != strings.Join(want, ",") {
		t.Fatalf("transaction order = %v, want %v", events, want)
	}
}

func testTask3UnsupportedIsolation(t *testing.T) {
	home := t.TempDir()
	setInstallHome(t, home)
	target := filepath.Join(home, ".zshrc")
	original := []byte("export SAFE=1\n")
	if err := os.WriteFile(target, original, 0o600); err != nil {
		t.Fatal(err)
	}
	initializerCalls := 0
	seams := &installTransactionSeams{capabilityCheck: func(atomicRenameMode) error { return ErrAtomicRenameUnsupported }}
	err := runInstallWithStoreInitializationAndSeams(staticHooker("typeset -g NEVER=1\n"), func(context.Context) (StoreInitialization, error) {
		initializerCalls++
		return StoreInitialization{}, nil
	}, seams)
	if !errors.Is(err, ErrAtomicRenameUnsupported) {
		t.Fatalf("unsupported install error = %v", err)
	}
	if initializerCalls != 0 {
		t.Fatalf("initializer calls = %d, want 0", initializerCalls)
	}
	if got := readTask3Target(t, target); !bytes.Equal(got, original) {
		t.Fatalf("unsupported preflight changed target: %q", got)
	}
	if _, err := os.Lstat(filepath.Join(home, ".zsh-pro")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unsupported preflight changed cache: %v", err)
	}
}

func testTask3ProbeCleanupUncertainty(t *testing.T) {
	fixture := newTask3InstallFixture(t, true, nil)
	seams := &installTransactionSeams{syncDirectory: func(file *os.File, stage string) error {
		if stage == "probe-entries-cleaned" {
			return errors.New("injected probe cleanup uncertainty")
		}
		return file.Sync()
	}}
	err := preflightAtomicRenameTarget(fixture.target, seams)
	if !errors.Is(err, ErrInstallRecoveryRequired) {
		t.Fatalf("probe cleanup error = %v, want recovery required", err)
	}
	entries, err := os.ReadDir(filepath.Join(fixture.home, installTransactionNamespaceName))
	if err != nil {
		t.Fatal(err)
	}
	probeCount := 0
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "probe-") {
			probeCount++
		}
	}
	if probeCount != 1 {
		t.Fatalf("retained private probe directories = %d, want 1", probeCount)
	}
	if got := readTask3Target(t, fixture.target); !bytes.Equal(got, fixture.original) {
		t.Fatalf("probe uncertainty changed target: %q", got)
	}
}

func testTask3PreparationBarrier(t *testing.T, failStage string, wantJournal bool) {
	fixture := newTask3InstallFixture(t, true, nil)
	var targetCalls atomic.Int32
	seams := targetAtomicCountingSeams(&targetCalls)
	seams.syncFile = func(file *os.File, stage string) error {
		if stage == failStage {
			return errors.New("injected file sync failure")
		}
		return file.Sync()
	}
	seams.syncDirectory = func(file *os.File, stage string) error {
		if stage == failStage {
			return errors.New("injected directory sync failure")
		}
		return file.Sync()
	}
	if err := preflightAtomicRenameTarget(fixture.target, seams); err != nil {
		t.Fatal(err)
	}
	targetCalls.Store(0)
	transaction, err := prepareGuardedInstallTransaction(fixture.prepared, seams)
	if !errors.Is(err, ErrInstallRecoveryRequired) || transaction == nil {
		t.Fatalf("preparation barrier error = %v transaction=%v", err, transaction)
	}
	if targetCalls.Load() != 0 {
		t.Fatalf("target namespace calls = %d, want 0", targetCalls.Load())
	}
	if got := readTask3Target(t, fixture.target); !bytes.Equal(got, fixture.original) {
		t.Fatalf("preparation barrier changed target: %q", got)
	}
	_, journalErr := os.Lstat(filepath.Join(transaction.retainedTransactionPath(), journalBasename))
	if wantJournal && journalErr != nil {
		t.Fatalf("prepared journal was not retained: %v", journalErr)
	}
	if !wantJournal && !errors.Is(journalErr, os.ErrNotExist) {
		t.Fatalf("journal exists before its durability gate: %v", journalErr)
	}
}

func testTask3PromotionSyncFailure(t *testing.T, failStage string) {
	var targetCalls atomic.Int32
	seams := targetAtomicCountingSeams(&targetCalls)
	fixture, transaction := prepareTask3Transaction(t, true, seams, nil)
	targetCalls.Store(0)
	seams.syncDirectory = func(file *os.File, stage string) error {
		if stage == failStage {
			return errors.New("injected promotion sync failure")
		}
		return file.Sync()
	}
	transaction.seams = normalizedInstallTransactionSeams(seams)
	outcome, err := transaction.promote()
	if !errors.Is(err, ErrInstallRecoveryRequired) || !outcome.RecoveryRequired {
		t.Fatalf("promotion sync error = %v outcome=%+v", err, outcome)
	}
	if targetCalls.Load() != 1 {
		t.Fatalf("target namespace calls = %d, want 1", targetCalls.Load())
	}
	if _, err := os.Stat(transaction.retainedTransactionPath()); err != nil {
		t.Fatalf("recovery evidence was not retained: %v", err)
	}
	if bytes.Equal(readTask3Target(t, fixture.target), fixture.original) && failStage != "journal-directory:exchanged" {
		t.Fatal("post-exchange failure unexpectedly reported untouched bytes")
	}
}

func testTask3LateSubstitution(t *testing.T, unsafeReverse bool) {
	var targetCalls atomic.Int32
	seams := targetAtomicCountingSeams(&targetCalls)
	fixture, transaction := prepareTask3Transaction(t, true, seams, nil)
	targetCalls.Store(0)
	late := []byte("export LATE=1\n")
	seams.beforeNamespace = func(*guardedInstallTransaction, atomicRenameMode) error {
		return os.WriteFile(fixture.target, late, 0o600)
	}
	if unsafeReverse {
		seams.beforeReverse = func(*guardedInstallTransaction) error {
			return os.WriteFile(fixture.target, []byte("candidate changed after swap"), 0o600)
		}
	}
	transaction.seams = normalizedInstallTransactionSeams(seams)
	outcome, err := transaction.promote()
	if unsafeReverse {
		if !errors.Is(err, ErrInstallRecoveryRequired) || !outcome.RecoveryRequired {
			t.Fatalf("unsafe reverse error = %v outcome=%+v", err, outcome)
		}
		if targetCalls.Load() != 1 {
			t.Fatalf("unsafe reverse calls = %d, want only forward exchange", targetCalls.Load())
		}
		return
	}
	if !errors.Is(err, ErrInstallTargetChanged) || !outcome.Restored {
		t.Fatalf("late substitution error = %v outcome=%+v", err, outcome)
	}
	if targetCalls.Load() != 2 {
		t.Fatalf("exchange calls = %d, want forward plus guarded reverse", targetCalls.Load())
	}
	if got := readTask3Target(t, fixture.target); !bytes.Equal(got, late) {
		t.Fatalf("guarded reverse lost late occupant: %q", got)
	}
	if err := outcome.Finalize(); err != nil {
		t.Fatal(err)
	}
}

func testTask3LateAbsentTarget(t *testing.T) {
	var targetCalls atomic.Int32
	seams := targetAtomicCountingSeams(&targetCalls)
	fixture, transaction := prepareTask3Transaction(t, false, seams, nil)
	targetCalls.Store(0)
	late := []byte("late target")
	seams.beforeNamespace = func(*guardedInstallTransaction, atomicRenameMode) error {
		return os.WriteFile(fixture.target, late, 0o600)
	}
	transaction.seams = normalizedInstallTransactionSeams(seams)
	outcome, err := transaction.promote()
	if !errors.Is(err, ErrInstallTargetChanged) || outcome.RecoveryRequired {
		t.Fatalf("late no-replace error = %v outcome=%+v", err, outcome)
	}
	if targetCalls.Load() != 1 {
		t.Fatalf("no-replace calls = %d, want 1", targetCalls.Load())
	}
	if got := readTask3Target(t, fixture.target); !bytes.Equal(got, late) {
		t.Fatalf("late target changed: %q", got)
	}
	if _, err := os.Stat(filepath.Join(transaction.retainedTransactionPath(), exchangePeerBasename)); err != nil {
		t.Fatalf("candidate peer not preserved: %v", err)
	}
}

func testTask3RecoveryRow(t *testing.T, existing bool, stage string) {
	seams := &installTransactionSeams{}
	fixture, transaction := prepareTask3Transaction(t, existing, seams, nil)
	locator := transaction.recoveryLocator()
	if stage != "before" {
		switch stage {
		case "after-namespace":
			seams.afterNamespace = func(*guardedInstallTransaction, atomicRenameMode) error {
				return errors.New("injected crash after namespace syscall")
			}
		case "after-journal":
			seams.syncDirectory = func(file *os.File, syncStage string) error {
				if syncStage == "target-parent:forward-exchange" || syncStage == "target-parent:forward-no-replace" {
					return errors.New("injected crash before parent sync")
				}
				return file.Sync()
			}
		case "after-parent":
			seams.syncDirectory = func(file *os.File, syncStage string) error {
				if syncStage == "journal-directory:parent_synced" {
					return errors.New("injected crash after parent sync")
				}
				return file.Sync()
			}
		default:
			t.Fatalf("unknown recovery stage %q", stage)
		}
		transaction.seams = normalizedInstallTransactionSeams(seams)
		outcome, err := transaction.promote()
		if !errors.Is(err, ErrInstallRecoveryRequired) || !outcome.RecoveryRequired {
			t.Fatalf("crash seam %s error=%v outcome=%+v", stage, err, outcome)
		}
	}
	transaction.closeHandles()
	recovered, err := recoverGuardedInstallTransaction(locator, nil)
	if existing {
		if err != nil || !recovered.Restored {
			t.Fatalf("existing recovery %s = outcome=%+v err=%v", stage, recovered, err)
		}
		if got := readTask3Target(t, fixture.target); !bytes.Equal(got, fixture.original) {
			t.Fatalf("existing recovery %s target=%q", stage, got)
		}
		return
	}
	if stage == "before" {
		if err != nil || recovered.RecoveryRequired {
			t.Fatalf("absent pre-create recovery = outcome=%+v err=%v", recovered, err)
		}
		if _, statErr := os.Lstat(fixture.target); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("absent pre-create recovery created target: %v", statErr)
		}
		return
	}
	if !errors.Is(err, ErrInstallRecoveryRequired) || !recovered.RecoveryRequired {
		t.Fatalf("absent post-create recovery %s = outcome=%+v err=%v", stage, recovered, err)
	}
	if got := readTask3Target(t, fixture.target); !bytes.Equal(got, fixture.candidate) {
		t.Fatalf("absent recovery %s changed installed candidate: %q", stage, got)
	}
}

func testTask3ChangedAxis(t *testing.T, targetAxis bool) {
	var targetCalls atomic.Int32
	seams := targetAtomicCountingSeams(&targetCalls)
	fixture, transaction := prepareTask3Transaction(t, true, seams, nil)
	targetCalls.Store(0)
	changed := []byte("changed axis")
	if targetAxis {
		if err := os.WriteFile(fixture.target, changed, 0o600); err != nil {
			t.Fatal(err)
		}
	} else if err := os.WriteFile(filepath.Join(transaction.retainedTransactionPath(), exchangePeerBasename), changed, 0o600); err != nil {
		t.Fatal(err)
	}
	outcome, err := transaction.promote()
	if !errors.Is(err, ErrInstallTargetChanged) || outcome.Promoted || outcome.RecoveryRequired {
		t.Fatalf("changed axis error=%v outcome=%+v", err, outcome)
	}
	if targetCalls.Load() != 0 {
		t.Fatalf("changed axis target calls=%d, want 0", targetCalls.Load())
	}
	if targetAxis && !bytes.Equal(readTask3Target(t, fixture.target), changed) {
		t.Fatal("changed target axis was not preserved")
	}
}

func testTask3UnsupportedRows(t *testing.T) {
	for _, row := range []struct {
		name  string
		seams *installTransactionSeams
	}{
		{name: "adapter", seams: &installTransactionSeams{capabilityCheck: func(atomicRenameMode) error { return ErrAtomicRenameUnsupported }}},
		{name: "filesystem", seams: &installTransactionSeams{atomicRename: func(int, string, int, string, atomicRenameMode) error { return ErrAtomicRenameUnsupported }}},
	} {
		t.Run(row.name, func(t *testing.T) {
			fixture := newTask3InstallFixture(t, true, nil)
			err := preflightAtomicRenameTarget(fixture.target, row.seams)
			if !errors.Is(err, ErrAtomicRenameUnsupported) {
				t.Fatalf("unsupported row error=%v", err)
			}
			if got := readTask3Target(t, fixture.target); !bytes.Equal(got, fixture.original) {
				t.Fatalf("unsupported row changed target: %q", got)
			}
		})
	}
}

func testTask3RootLock(t *testing.T) {
	firstFixture := newTask3InstallFixture(t, true, nil)
	seams := normalizedInstallTransactionSeams(nil)
	first, err := openTargetTransactionGuard(firstFixture.target, seams)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(first.close)
	type guardResult struct {
		guard *targetTransactionGuard
		err   error
	}
	sameRoot := make(chan guardResult, 1)
	go func() {
		guard, err := openTargetTransactionGuard(firstFixture.target, seams)
		sameRoot <- guardResult{guard: guard, err: err}
	}()
	select {
	case result := <-sameRoot:
		if result.guard != nil {
			result.guard.close()
		}
		t.Fatalf("same-root lock did not block: %v", result.err)
	case <-time.After(100 * time.Millisecond):
	}
	secondFixture := newTask3InstallFixture(t, true, nil)
	differentRoot := make(chan guardResult, 1)
	go func() {
		guard, err := openTargetTransactionGuard(secondFixture.target, seams)
		differentRoot <- guardResult{guard: guard, err: err}
	}()
	select {
	case result := <-differentRoot:
		if result.err != nil {
			t.Fatal(result.err)
		}
		result.guard.close()
	case <-time.After(2 * time.Second):
		t.Fatal("different-root lock was serialized")
	}
	first.close()
	select {
	case result := <-sameRoot:
		if result.err != nil {
			t.Fatal(result.err)
		}
		result.guard.close()
	case <-time.After(2 * time.Second):
		t.Fatal("same-root waiter did not acquire after holder release")
	}
}

func testTask3UnsafeLock(t *testing.T) {
	for _, row := range []string{"symlink", "wrong-mode", "nonregular"} {
		t.Run(row, func(t *testing.T) {
			fixture := newTask3InstallFixture(t, true, nil)
			namespace := filepath.Join(fixture.home, installTransactionNamespaceName)
			if err := os.Mkdir(namespace, 0o700); err != nil {
				t.Fatal(err)
			}
			lock := filepath.Join(namespace, installTransactionLockName)
			switch row {
			case "symlink":
				if err := os.Symlink(fixture.target, lock); err != nil {
					t.Fatal(err)
				}
			case "wrong-mode":
				if err := os.WriteFile(lock, nil, 0o644); err != nil {
					t.Fatal(err)
				}
			case "nonregular":
				if err := os.Mkdir(lock, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if err := preflightAtomicRenameTarget(fixture.target, nil); err == nil {
				t.Fatal("unsafe lock entry was accepted")
			}
			if got := readTask3Target(t, fixture.target); !bytes.Equal(got, fixture.original) {
				t.Fatalf("unsafe lock changed target: %q", got)
			}
		})
	}
}

func testTask3UnauthenticatedCleanup(t *testing.T) {
	_, transaction := prepareTask3Transaction(t, true, nil, nil)
	evidencePath := filepath.Join(transaction.retainedTransactionPath(), candidateEvidenceBasename)
	if err := os.Remove(evidencePath); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(evidencePath, []byte("attacker replacement"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := transaction.finalize(); !errors.Is(err, ErrInstallRecoveryRequired) {
		t.Fatalf("unauthenticated cleanup error=%v", err)
	}
	if got, err := os.ReadFile(evidencePath); err != nil || string(got) != "attacker replacement" {
		t.Fatalf("unauthenticated replacement was removed: bytes=%q err=%v", got, err)
	}
}

func testTask3ChangedParent(t *testing.T) {
	_, transaction := prepareTask3Transaction(t, true, nil, nil)
	originalParent := transaction.guard.parentPath
	movedParent := originalParent + "-moved"
	if err := os.Rename(originalParent, movedParent); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(originalParent, 0o700); err != nil {
		t.Fatal(err)
	}
	outcome, err := transaction.promote()
	if !errors.Is(err, ErrInstallRecoveryRequired) || !outcome.RecoveryRequired {
		t.Fatalf("changed parent error=%v outcome=%+v", err, outcome)
	}
}

func testTask3ChangedJournal(t *testing.T) {
	rows := []struct {
		name   string
		mutate func(*testing.T, *guardedInstallTransaction, string)
	}{
		{name: "descriptor identity", mutate: func(t *testing.T, _ *guardedInstallTransaction, path string) {
			content, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, content, 0o600); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "mode", mutate: func(t *testing.T, _ *guardedInstallTransaction, path string) {
			if err := os.Chmod(path, 0o640); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "uid evidence", mutate: func(_ *testing.T, transaction *guardedInstallTransaction, _ string) { transaction.journalFile.UID++ }},
		{name: "gid evidence", mutate: func(_ *testing.T, transaction *guardedInstallTransaction, _ string) { transaction.journalFile.GID++ }},
		{name: "digest", mutate: func(t *testing.T, _ *guardedInstallTransaction, path string) {
			if err := os.WriteFile(path, []byte("{}"), 0o600); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "schema", mutate: func(t *testing.T, _ *guardedInstallTransaction, path string) {
			mutateTask3Journal(t, path, func(journal *installPromotionJournal) { journal.Schema++ })
		}},
		{name: "transaction id", mutate: func(t *testing.T, _ *guardedInstallTransaction, path string) {
			mutateTask3Journal(t, path, func(journal *installPromotionJournal) { journal.TransactionID = "changed" })
		}},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			_, transaction := prepareTask3Transaction(t, true, nil, nil)
			journalPath := filepath.Join(transaction.retainedTransactionPath(), journalBasename)
			row.mutate(t, transaction, journalPath)
			outcome, err := transaction.promote()
			if !errors.Is(err, ErrInstallRecoveryRequired) || !outcome.RecoveryRequired {
				t.Fatalf("changed journal row error=%v outcome=%+v", err, outcome)
			}
		})
	}
}

func mutateTask3Journal(t *testing.T, path string, mutate func(*installPromotionJournal)) {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	record, err := decodeInstallPromotionJournalRecord(content)
	if err != nil {
		t.Fatal(err)
	}
	mutate(&record.journal)
	slot, err := encodeInstallPromotionJournalSlot(record.journal, record.generation)
	if err != nil {
		t.Fatal(err)
	}
	content = make([]byte, installJournalFileSize)
	slotIndex := int((record.generation - 1) % installJournalSlotCount)
	copy(content[slotIndex*installJournalSlotSize:], slot)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
}

func testTask3ChangedLink(t *testing.T) {
	fixture, transaction := prepareTask3Transaction(t, true, nil, nil)
	backing := filepath.Join(fixture.home, "backing")
	if err := os.Rename(fixture.target, backing); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Base(backing), fixture.target); err != nil {
		t.Fatal(err)
	}
	outcome, err := transaction.promote()
	if !errors.Is(err, ErrInstallRecoveryRequired) || !outcome.RecoveryRequired {
		t.Fatalf("changed link error=%v outcome=%+v", err, outcome)
	}
}

func testTask3OneExchangePeer(t *testing.T) {
	seams := &installTransactionSeams{}
	fixture, transaction := prepareTask3Transaction(t, true, seams, nil)
	assertTask3ArtifactNames(t, transaction.retainedTransactionPath())
	late := []byte("late occupant")
	seams.beforeNamespace = func(*guardedInstallTransaction, atomicRenameMode) error {
		return os.WriteFile(fixture.target, late, 0o600)
	}
	transaction.seams = normalizedInstallTransactionSeams(seams)
	outcome, err := transaction.promote()
	if !errors.Is(err, ErrInstallTargetChanged) || !outcome.Restored {
		t.Fatalf("one-peer reverse error=%v outcome=%+v", err, outcome)
	}
	assertTask3ArtifactNames(t, transaction.retainedTransactionPath())
	peer := readTask3Target(t, filepath.Join(transaction.retainedTransactionPath(), exchangePeerBasename))
	if !bytes.Equal(peer, fixture.candidate) {
		t.Fatalf("same peer does not hold candidate after reverse: %q", peer)
	}
	if err := outcome.Finalize(); err != nil {
		t.Fatal(err)
	}
}

func assertTask3ArtifactNames(t *testing.T, path string) {
	t.Helper()
	entries, err := os.ReadDir(path)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	want := []string{candidateEvidenceBasename, exchangePeerBasename, journalBasename}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Fatalf("transaction artifacts=%v, want %v", names, want)
	}
}

func testTask3SecretPeer(t *testing.T, recovery bool) {
	canary := "TASK3_LITERAL_CANARY_65c7b6"
	fixture := newTask3InstallFixture(t, true, nil)
	fixture.original = []byte("export TOKEN='" + canary + "'\n")
	if err := os.WriteFile(fixture.target, fixture.original, 0o600); err != nil {
		t.Fatal(err)
	}
	prepared, err := prepareIngestInstallAt(fixture.target, renderInstallBlock())
	if err != nil {
		t.Fatal(err)
	}
	fixture.prepared = prepared
	fixture.candidate = append([]byte(nil), prepared.candidate...)
	seams := &installTransactionSeams{}
	if err := preflightAtomicRenameTarget(fixture.target, seams); err != nil {
		t.Fatal(err)
	}
	transaction, err := prepareGuardedInstallTransaction(prepared, seams)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(transaction.closeHandles)
	if got := task3CanaryArtifactMatches(t, transaction.retainedTransactionPath(), canary); strings.Join(got, ",") != exchangePeerBasename {
		t.Fatalf("pre-promotion canary artifacts=%v, want only peer", got)
	}
	if recovery {
		seams.syncDirectory = func(file *os.File, stage string) error {
			if stage == "target-parent:forward-exchange" {
				return errors.New("injected secret recovery")
			}
			return file.Sync()
		}
		transaction.seams = normalizedInstallTransactionSeams(seams)
		outcome, err := transaction.promote()
		if !errors.Is(err, ErrInstallRecoveryRequired) || !outcome.RecoveryRequired {
			t.Fatalf("secret recovery error=%v outcome=%+v", err, outcome)
		}
		if strings.Contains(err.Error(), canary) {
			t.Fatal("recovery error disclosed literal")
		}
		if got := task3CanaryArtifactMatches(t, transaction.retainedTransactionPath(), canary); strings.Join(got, ",") != exchangePeerBasename {
			t.Fatalf("recovery canary artifacts=%v, want only peer", got)
		}
		return
	}
	outcome, err := transaction.promote()
	if err != nil {
		t.Fatal(err)
	}
	transactionPath := transaction.retainedTransactionPath()
	if err := outcome.Finalize(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(transactionPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("finalize retained transaction artifacts: %v", err)
	}
}

func task3CanaryArtifactMatches(t *testing.T, transactionPath, canary string) []string {
	t.Helper()
	entries, err := os.ReadDir(transactionPath)
	if err != nil {
		t.Fatal(err)
	}
	var matches []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		content, err := os.ReadFile(filepath.Join(transactionPath, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(content, []byte(canary)) {
			matches = append(matches, entry.Name())
		}
	}
	return matches
}

func testTask3AbsentRollback(t *testing.T) {
	fixture, transaction := prepareTask3Transaction(t, false, nil, nil)
	outcome, err := transaction.promote()
	if err != nil || !outcome.Promoted {
		t.Fatalf("absent promotion error=%v outcome=%+v", err, outcome)
	}
	if err := outcome.Rollback(); !errors.Is(err, ErrInstallRecoveryRequired) {
		t.Fatalf("absent rollback error=%v, want recovery required", err)
	}
	if got := readTask3Target(t, fixture.target); !bytes.Equal(got, fixture.candidate) {
		t.Fatalf("absent rollback removed candidate: %q", got)
	}
	if _, err := os.Stat(filepath.Join(transaction.retainedTransactionPath(), journalBasename)); err != nil {
		t.Fatalf("absent rollback did not retain journal: %v", err)
	}
}

func testTask3ChangedRollback(t *testing.T) {
	fixture, transaction := prepareTask3Transaction(t, true, nil, nil)
	outcome, err := transaction.promote()
	if err != nil {
		t.Fatal(err)
	}
	changed := []byte("target changed after promotion")
	if err := os.WriteFile(fixture.target, changed, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := outcome.Rollback(); !errors.Is(err, ErrInstallRecoveryRequired) {
		t.Fatalf("changed rollback error=%v", err)
	}
	if got := readTask3Target(t, fixture.target); !bytes.Equal(got, changed) {
		t.Fatalf("changed rollback destroyed external bytes: %q", got)
	}
}

func testTask3MutationSurface(t *testing.T) {
	root := repositoryRoot(t)
	for _, relative := range []string{"core/cli/install.go", "core/cli/install_transaction.go"} {
		path := filepath.Join(root, relative)
		set := token.NewFileSet()
		file, err := parser.ParseFile(set, path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Body == nil || function.Name.Name == "mutateInstallNamespace" {
				continue
			}
			ast.Inspect(function.Body, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}
				forbidden := map[string]bool{
					"Rename": true, "Remove": true, "RemoveAll": true, "Link": true, "Linkat": true,
					"Symlink": true, "Mkdir": true, "MkdirAll": true, "CreateTemp": true,
					"atomicRenameAt": true, "atomicRenameBetweenAt": true,
				}
				name := ""
				switch called := call.Fun.(type) {
				case *ast.Ident:
					name = called.Name
				case *ast.SelectorExpr:
					name = called.Sel.Name
				}
				if forbidden[name] {
					t.Errorf("%s contains direct namespace mutation %s at %s", relative, name, set.Position(call.Pos()))
				}
				return true
			})
		}
	}
}

func testTask3StaleCompensation(t *testing.T) {
	home := t.TempDir()
	setInstallHome(t, home)
	target := filepath.Join(home, ".zshrc")
	original := []byte("export ORIGINAL=1\n")
	if err := os.WriteFile(target, original, 0o600); err != nil {
		t.Fatal(err)
	}
	cache := filepath.Join(home, ".zsh-pro")
	if err := os.Mkdir(cache, 0o700); err != nil {
		t.Fatal(err)
	}
	oldLoader := []byte("typeset -g OLD_LOADER=1\n")
	if err := os.WriteFile(filepath.Join(cache, cacheLoaderName), oldLoader, 0o600); err != nil {
		t.Fatal(err)
	}
	late := []byte("export EXTERNAL=1\n")
	rollbackCalls := 0
	finalizeCalls := 0
	err := runInstallWithStoreInitialization(callbackHooker{
		script: "typeset -g NEW_LOADER=1\n",
		before: func() {
			if writeErr := os.WriteFile(target, late, 0o600); writeErr != nil {
				t.Error(writeErr)
			}
		},
	}, func(context.Context) (StoreInitialization, error) {
		return StoreInitialization{
			Rollback: func() error { rollbackCalls++; return nil },
			Finalize: func() error { finalizeCalls++; return nil },
		}, nil
	})
	if !errors.Is(err, ErrInstallTargetChanged) {
		t.Fatalf("stale install error=%v", err)
	}
	if got := readTask3Target(t, target); !bytes.Equal(got, late) {
		t.Fatalf("stale compensation changed external target: %q", got)
	}
	if got := readTask3Target(t, filepath.Join(cache, cacheLoaderName)); !bytes.Equal(got, oldLoader) {
		t.Fatalf("stale compensation did not restore loader: %q", got)
	}
	if rollbackCalls != 1 || finalizeCalls != 0 {
		t.Fatalf("initializer rollback/finalize=%d/%d, want 1/0", rollbackCalls, finalizeCalls)
	}
}

func testTask3PostSwapAxes(t *testing.T) {
	fixture, transaction := prepareTask3Transaction(t, true, nil, nil)
	outcome, err := transaction.promote()
	if err != nil || !outcome.Promoted || transaction.journal.DisplacedObserved == nil {
		t.Fatalf("post-swap outcome=%+v err=%v", outcome, err)
	}
	targetEvidence, exists, err := captureInstallTargetEvidence(transaction.guard.parentRoot, transaction.guard.targetName)
	if err != nil || !exists || !transaction.expectedCandidate.authenticatedEqual(targetEvidence) {
		t.Fatalf("target is not expectedCandidate: evidence=%+v err=%v", targetEvidence, err)
	}
	peerEvidence, _, err := captureInstallFileEvidence(transaction.transactionRoot, exchangePeerBasename)
	if err != nil || !transaction.journal.DisplacedObserved.authenticatedEqual(peerEvidence) ||
		!evidenceMatchesSnapshot(peerEvidence, transaction.expectedTarget) {
		t.Fatalf("peer is not displacedObserved/expectedTarget: evidence=%+v err=%v", peerEvidence, err)
	}
	if !bytes.Equal(readTask3Target(t, fixture.target), fixture.candidate) {
		t.Fatal("post-swap target bytes are not the candidate")
	}
	if err := outcome.Rollback(); err != nil {
		t.Fatal(err)
	}
}

func testTask3TornJournalTransition(t *testing.T, afterExchanged bool) {
	fixture, transaction := prepareTask3Transaction(t, true, nil, nil)
	locator := transaction.recoveryLocator()
	journalPath := filepath.Join(transaction.retainedTransactionPath(), journalBasename)
	journalInode := transaction.journalFile.Inode
	writes := 0
	transaction.seams.writeJournal = func(file *os.File, slot []byte, offset int64) (int, error) {
		writes++
		if !afterExchanged || writes == 2 {
			prefixLength := installJournalSlotHeaderSize + 1
			if afterExchanged {
				// Tear after the new generation reaches the previously valid
				// slot but before its length, digest, or payload are replaced.
				prefixLength = len(installJournalSlotMagic) + 4 + 8
			}
			n, err := file.WriteAt(slot[:prefixLength], offset)
			if err != nil {
				return n, err
			}
			return n, errors.New("injected torn journal slot")
		}
		return file.WriteAt(slot, offset)
	}

	outcome, err := transaction.promote()
	if !errors.Is(err, ErrInstallRecoveryRequired) || !outcome.RecoveryRequired || !outcome.Promoted {
		t.Fatalf("torn transition error=%v outcome=%+v", err, outcome)
	}
	content, err := os.ReadFile(journalPath)
	if err != nil {
		t.Fatal(err)
	}
	record, err := decodeInstallPromotionJournalRecord(content)
	if err != nil {
		t.Fatalf("decode surviving journal record: %v", err)
	}
	wantState := promotionStatePrepared
	if afterExchanged {
		wantState = promotionStateExchanged
	}
	if record.journal.State != wantState {
		t.Fatalf("surviving journal state = %q, want %q", record.journal.State, wantState)
	}
	currentJournal, _, err := captureInstallFileEvidence(transaction.transactionRoot, journalBasename)
	if err != nil || currentJournal.Inode != journalInode {
		t.Fatalf("journal inode changed across torn transition: before=%d after=%d err=%v", journalInode, currentJournal.Inode, err)
	}

	transaction.closeHandles()
	recovered, err := recoverGuardedInstallTransaction(locator, nil)
	if err != nil || !recovered.Restored || recovered.RecoveryRequired {
		t.Fatalf("recover torn %s transition = outcome=%+v err=%v", wantState, recovered, err)
	}
	if got := readTask3Target(t, fixture.target); !bytes.Equal(got, fixture.original) {
		t.Fatalf("recover torn %s transition target=%q", wantState, got)
	}
}

func testTask3InvalidJournalSlots(t *testing.T, torn bool) {
	fixture, transaction := prepareTask3Transaction(t, true, nil, nil)
	locator := transaction.recoveryLocator()
	journalPath := filepath.Join(transaction.retainedTransactionPath(), journalBasename)
	transaction.closeHandles()
	content := make([]byte, installJournalFileSize)
	if torn {
		copy(content, installJournalSlotMagic[:len(installJournalSlotMagic)/2])
	}
	if err := os.WriteFile(journalPath, content, 0o600); err != nil {
		t.Fatal(err)
	}
	outcome, err := recoverGuardedInstallTransaction(locator, nil)
	if !errors.Is(err, ErrInstallRecoveryRequired) || !outcome.RecoveryRequired {
		t.Fatalf("invalid journal recovery = outcome=%+v err=%v", outcome, err)
	}
	if got := readTask3Target(t, fixture.target); !bytes.Equal(got, fixture.original) {
		t.Fatalf("invalid journal recovery changed target: %q", got)
	}
}

func TestAtomicRenameFilesystemProbePrecedesInitializerAndEffects(t *testing.T) {
	requireTask3InstallContract(t, "pure check and filesystem probe ordering")
}

func TestAtomicRenameUnsupportedLeavesStoreCacheLoaderTargetUnchanged(t *testing.T) {
	requireTask3InstallContract(t, "unsupported isolation")
}

func TestAtomicRenameProbeCleanupUncertaintyRetainsPrivateEvidence(t *testing.T) {
	requireTask3InstallContract(t, "probe cleanup uncertainty")
}

func TestPromoteGuardedCandidateFsyncFailureBeforeJournalOrNamespace(t *testing.T) {
	requireTask3InstallContract(t, "candidate fsync barrier")
}

func TestPromoteGuardedArtifactDirectoryFsyncFailureBeforePreparedJournal(t *testing.T) {
	requireTask3InstallContract(t, "artifact directory fsync barrier")
}

func TestPromoteGuardedPreparedJournalDirectoryFsyncFailureBeforeNamespace(t *testing.T) {
	requireTask3InstallContract(t, "prepared journal directory fsync barrier")
}

func TestPromotionJournalTransitionDirectoryFsyncFailureRetainsRecovery(t *testing.T) {
	requireTask3InstallContract(t, "journal transition directory fsync")
}

func TestPromotionJournalTornFirstTransitionRecoversPreparedRecord(t *testing.T) {
	testTask3TornJournalTransition(t, false)
}

func TestPromotionJournalTornLaterTransitionRecoversExchangedRecord(t *testing.T) {
	testTask3TornJournalTransition(t, true)
}

func TestPromotionJournalRejectsEmptyOrWhollyTornSlots(t *testing.T) {
	t.Run("empty", func(t *testing.T) { testTask3InvalidJournalSlots(t, false) })
	t.Run("torn", func(t *testing.T) { testTask3InvalidJournalSlots(t, true) })
}

func TestPromoteGuardedDetectsAndReversesSubstitution(t *testing.T) {
	requireTask3InstallContract(t, "late substitution reverse")
}

func TestPromoteGuardedRefusesUnsafeReverseExchange(t *testing.T) {
	requireTask3InstallContract(t, "unsafe reverse refusal")
}

func TestPromoteGuardedNoReplacePreservesLateTarget(t *testing.T) {
	requireTask3InstallContract(t, "late absent-target creation")
}

func TestPromoteGuardedFsyncFailureRequiresRecovery(t *testing.T) {
	requireTask3InstallContract(t, "post-promotion fsync uncertainty")
}

func TestRecoverGuardedExistingBeforeNamespaceSyscall(t *testing.T) {
	requireTask3InstallContract(t, "existing recovery before exchange")
}

func TestRecoverGuardedExistingAfterExchangeBeforeJournalAdvance(t *testing.T) {
	requireTask3InstallContract(t, "existing recovery after exchange")
}

func TestRecoverGuardedExistingAfterJournalAdvanceBeforeParentSync(t *testing.T) {
	requireTask3InstallContract(t, "existing recovery after journal advance")
}

func TestRecoverGuardedExistingAfterParentSync(t *testing.T) {
	requireTask3InstallContract(t, "existing recovery after parent sync")
}

func TestRecoverGuardedAbsentBeforeNamespaceSyscall(t *testing.T) {
	requireTask3InstallContract(t, "absent recovery before create")
}

func TestRecoverGuardedAbsentAfterNoReplaceBeforeJournalAdvance(t *testing.T) {
	requireTask3InstallContract(t, "absent recovery after no-replace")
}

func TestRecoverGuardedAbsentAfterJournalAdvanceBeforeParentSync(t *testing.T) {
	requireTask3InstallContract(t, "absent recovery after journal advance")
}

func TestRecoverGuardedAbsentAfterParentSync(t *testing.T) {
	requireTask3InstallContract(t, "absent recovery after parent sync")
}

func TestPromoteGuardedDetectsInPlaceMutation(t *testing.T) {
	requireTask3InstallContract(t, "in-place target mutation")
}

func TestPromoteGuardedUnsupportedBeforeEffects(t *testing.T) {
	requireTask3InstallContract(t, "adapter and filesystem unsupported")
}

func TestTargetTransactionCrossProcessRootLock(t *testing.T) {
	requireTask3InstallContract(t, "cross-process root lock")
}

func TestTargetTransactionRejectsUnsafeRootLockEntry(t *testing.T) {
	requireTask3InstallContract(t, "unsafe root lock entry")
}

func TestPromotionCleanupRejectsUnauthenticatedArtifact(t *testing.T) {
	requireTask3InstallContract(t, "unauthenticated cleanup refusal")
}

func TestPromotionJournalRejectsChangedParentIdentity(t *testing.T) {
	requireTask3InstallContract(t, "changed parent identity")
}

func TestPromotionJournalRejectsChangedJournalIdentityOrDigest(t *testing.T) {
	requireTask3InstallContract(t, "journal identity and digest matrix")
}

func TestPromotionJournalRejectsChangedRequestedLinkTopology(t *testing.T) {
	requireTask3InstallContract(t, "requested link topology")
}

func TestPromotionJournalUsesOneExchangePeerBasename(t *testing.T) {
	requireTask3InstallContract(t, "one exchange peer")
}

func TestInstallSecretBearingPeerConfinedAndRemovedOnFinalize(t *testing.T) {
	requireTask3InstallContract(t, "secret peer finalize cleanup")
}

func TestInstallSecretBearingPeerRetainedOnlyForRecovery(t *testing.T) {
	requireTask3InstallContract(t, "secret peer recovery retention")
}

func TestRollbackGuardedAbsentAfterNoReplaceRequiresRecovery(t *testing.T) {
	requireTask3InstallContract(t, "absent rollback refusal")
}

func TestRollbackGuardedRefusesChangedTarget(t *testing.T) {
	requireTask3InstallContract(t, "changed target rollback refusal")
}

func TestInstallTransactionMutationSurfaceAudit(t *testing.T) {
	requireTask3InstallContract(t, "mutation surface AST audit")
}

func TestIngestStaleCompensation(t *testing.T) {
	requireTask3InstallContract(t, "filesystem-first stale compensation")
}

func TestInstallPromotionRejectsChangedTargetBeforeExchange(t *testing.T) {
	requireTask3InstallContract(t, "expected target axis")
}

func TestInstallPromotionRejectsChangedCandidateBeforeExchange(t *testing.T) {
	requireTask3InstallContract(t, "expected candidate axis")
}

func TestInstallPromotionPostSwapSeparatesCandidateAndDisplacedIdentity(t *testing.T) {
	requireTask3InstallContract(t, "post-swap independent axes")
}
