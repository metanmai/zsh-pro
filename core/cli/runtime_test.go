package cli

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRuntimeCaptureUsesPrivatePipeAndRejectsUnsafeRoots(t *testing.T) {
	binDir := t.TempDir()
	emitSeen := filepath.Join(binDir, "emit-seen")
	writeRuntimeTestCommand(t, filepath.Join(binDir, "zsh-pro"), `#!/bin/sh
case "$1:$2:$3" in
  emit:apply:B) : > "$ZP_EMIT_SEEN"; printf '%s\n' 'export ZP_CAPTURED=1' ;;
  *) exit 64 ;;
esac
`)
	t.Setenv("PATH", binDir+":"+os.Getenv("PATH"))
	t.Setenv("ZP_EMIT_SEEN", emitSeen)

	run := func(t *testing.T, root string) (int, string, string) {
		t.Helper()
		t.Setenv("ZSHPRO_HOME", root)
		_ = os.Remove(emitSeen)
		var stdout, stderr bytes.Buffer
		code := New(nil, nil, nil).Run(
			[]string{"runtime", "capture", "1", "--", "zsh-pro", "emit", "apply", "B"},
			&stdout,
			&stderr,
		)
		return code, stdout.String(), stderr.String()
	}

	safeRoot := filepath.Join(t.TempDir(), "runtime")
	if err := os.Mkdir(safeRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if code, stdout, stderr := run(t, safeRoot); code != 0 || stdout != "export ZP_CAPTURED=1\n" || stderr != "" {
		t.Fatalf("safe capture = (%d, %q, %q)", code, stdout, stderr)
	}
	if _, err := os.Stat(emitSeen); err != nil {
		t.Fatalf("safe root did not permit the emitter: %v", err)
	}
	entries, err := os.ReadDir(safeRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("runtime capture staged a file under the configured root: %v", entries)
	}
	if info, err := os.Stat(os.TempDir()); err == nil && info.Mode()&os.ModeSticky != 0 {
		stickyRoot, err := os.MkdirTemp(os.TempDir(), "zsh-pro-sticky-safe-")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.RemoveAll(stickyRoot) })
		if err := os.Chmod(stickyRoot, 0o700); err != nil {
			t.Fatal(err)
		}
		if code, stdout, stderr := run(t, stickyRoot); code != 0 || stdout != "export ZP_CAPTURED=1\n" || stderr != "" {
			t.Fatalf("sticky-ancestor capture = (%d, %q, %q)", code, stdout, stderr)
		}
	}

	link := filepath.Join(t.TempDir(), "attacker-owned-runtime-link")
	if err := os.Symlink(safeRoot, link); err != nil {
		t.Fatal(err)
	}
	if code, stdout, _ := run(t, link); code == 0 || stdout != "" {
		t.Fatalf("symlink runtime root was accepted: (%d, %q)", code, stdout)
	}
	if _, err := os.Stat(emitSeen); !os.IsNotExist(err) {
		t.Fatalf("symlink root reached the emitter: %v", err)
	}

	componentTarget := filepath.Join(t.TempDir(), "component-target")
	if err := os.Mkdir(componentTarget, 0o700); err != nil {
		t.Fatal(err)
	}
	componentRoot := filepath.Join(componentTarget, "runtime")
	if err := os.Mkdir(componentRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	componentLink := filepath.Join(t.TempDir(), "attacker-owned-component-link")
	if err := os.Symlink(componentTarget, componentLink); err != nil {
		t.Fatal(err)
	}
	if code, stdout, _ := run(t, filepath.Join(componentLink, "runtime")); code == 0 || stdout != "" {
		t.Fatalf("symlink runtime component was accepted: (%d, %q)", code, stdout)
	}
	if _, err := os.Stat(emitSeen); !os.IsNotExist(err) {
		t.Fatalf("symlink component reached the emitter: %v", err)
	}

	unsafeParent := filepath.Join(t.TempDir(), "unsafe-parent")
	if err := os.Mkdir(unsafeParent, 0o777); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(unsafeParent, 0o777); err != nil {
		t.Fatal(err)
	}
	unsafeRoot := filepath.Join(unsafeParent, "runtime")
	if err := os.Mkdir(unsafeRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if code, stdout, _ := run(t, unsafeRoot); code == 0 || stdout != "" {
		t.Fatalf("non-sticky writable ancestor was accepted: (%d, %q)", code, stdout)
	}
	if _, err := os.Stat(emitSeen); !os.IsNotExist(err) {
		t.Fatalf("unsafe ancestor reached the emitter: %v", err)
	}
}

func TestRuntimeCaptureBoundsEmitter(t *testing.T) {
	binDir := t.TempDir()
	writeRuntimeTestCommand(t, filepath.Join(binDir, "zsh-pro"), "#!/bin/sh\ncase \"$1:$2:$3\" in\nemit:apply:B) exec sleep 5 ;;\n*) exit 64 ;;\nesac\n")
	t.Setenv("PATH", binDir+":"+os.Getenv("PATH"))
	root := filepath.Join(t.TempDir(), "runtime")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ZSHPRO_HOME", root)

	started := time.Now()
	var stdout, stderr bytes.Buffer
	code := New(nil, nil, nil).Run(
		[]string{"runtime", "capture", "1", "--", "zsh-pro", "emit", "apply", "B"},
		&stdout,
		&stderr,
	)
	if code != 124 {
		t.Fatalf("timeout code = %d, want 124 (stderr %q)", code, stderr.String())
	}
	if elapsed := time.Since(started); elapsed > 2500*time.Millisecond {
		t.Fatalf("runtime capture exceeded its deadline: %s", elapsed)
	}
	if stdout.Len() != 0 {
		t.Fatalf("timed-out capture returned output: %q", stdout.String())
	}
}

func TestValidateRuntimeSourceUsesPipeAndSuppressesSourceDiagnostics(t *testing.T) {
	if _, err := exec.LookPath("zsh"); err != nil {
		t.Skip("zsh not installed")
	}
	if err := validateRuntimeSource(context.Background(), strings.NewReader("export ZP_VALID=1\n")); err != nil {
		t.Fatalf("valid pipe source: %v", err)
	}
	secretLike := "phase5-runtime-fixture"
	err := validateRuntimeSource(context.Background(), strings.NewReader("if then "+secretLike+"\n"))
	if err == nil {
		t.Fatal("invalid pipe source was accepted")
	}
	if strings.Contains(err.Error(), secretLike) {
		t.Fatalf("validator error leaked source data: %v", err)
	}
}

func writeRuntimeTestCommand(t *testing.T, path, source string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(source), 0o700); err != nil {
		t.Fatal(err)
	}
}
