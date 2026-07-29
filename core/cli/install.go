package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"zsh-pro/core/buildinfo"
	"zsh-pro/core/shell"
)

const (
	installBegin = "# >>> zsh-pro >>>"
	installEnd   = "# <<< zsh-pro <<<"
)

// runInstall writes the cached loader before it changes .zshrc. The cache path
// is deliberately resolved by the same environment expression rendered in the
// stub, so sourcing never depends on a binary invocation during shell startup.
func runInstall(provider shell.Hooker) error {
	cacheDir := runtimeDir()
	if err := os.MkdirAll(cacheDir, 0o700); err != nil {
		return fmt.Errorf("create runtime directory: %w", err)
	}
	if err := os.Chmod(cacheDir, 0o700); err != nil {
		return fmt.Errorf("secure runtime directory: %w", err)
	}
	loader := filepath.Join(cacheDir, "loader.zsh")
	loaderScript := "# zsh-pro cached loader version " + buildinfo.Version + "\n" + provider.HookScript()
	if err := atomicWrite(loader, []byte(loaderScript), 0o600); err != nil {
		return fmt.Errorf("write cached loader: %w", err)
	}
	if err := validateZsh(loaderScript); err != nil {
		return fmt.Errorf("validate cached loader: %w", err)
	}

	rc := zshrcPath()
	current, err := os.ReadFile(rc)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("read %s: %w", rc, err)
	}
	next, err := replaceManagedBlock(current, renderInstallBlock())
	if err != nil {
		return err
	}
	if err := atomicWrite(rc, next, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", rc, err)
	}
	return nil
}

func (c *CLI) runInstall(stdout, stderr io.Writer) int {
	if err := runInstall(c.provider); err != nil {
		return c.fail(stdout, stderr, false, err.Error())
	}
	_, _ = fmt.Fprintln(stdout, "zsh-pro: installed")
	return 0
}

func runtimeDir() string {
	if dir := os.Getenv("ZSHPRO_HOME"); dir != "" {
		return dir
	}
	return filepath.Join(os.Getenv("HOME"), ".zsh-pro")
}

func zshrcPath() string {
	base := os.Getenv("ZDOTDIR")
	if base == "" {
		base = os.Getenv("HOME")
	}
	return filepath.Join(base, ".zshrc")
}

func renderInstallBlock() []byte {
	return []byte(installBegin + "\n" +
		"if [[ -z \"${ZSHPRO_DISABLE-}\" ]]; then\n" +
		"  if command -v zsh-pro >/dev/null 2>&1 && [[ -r \"${ZSHPRO_HOME:-$HOME/.zsh-pro}/loader.zsh\" ]]; then\n" +
		"    source \"${ZSHPRO_HOME:-$HOME/.zsh-pro}/loader.zsh\"\n" +
		"  fi\n" +
		"fi\n" +
		"true\n" +
		installEnd)
}

// replaceManagedBlock is deliberately byte-oriented: all text outside complete
// marker regions is copied verbatim. Any malformed marker sequence is rejected
// before a caller can write, preventing accidental truncation to EOF.
func replaceManagedBlock(current, block []byte) ([]byte, error) {
	type region struct{ start, end int }
	var regions []region
	open := -1
	for lineStart := 0; lineStart < len(current); {
		lineEnd := len(current)
		terminatorEnd := len(current)
		if newline := bytes.IndexByte(current[lineStart:], '\n'); newline >= 0 {
			lineEnd = lineStart + newline
			terminatorEnd = lineEnd + 1
		}

		comparisonEnd := lineEnd
		if terminatorEnd > lineEnd && comparisonEnd > lineStart && current[comparisonEnd-1] == '\r' {
			comparisonEnd--
		}
		line := current[lineStart:comparisonEnd]

		switch {
		case bytes.Equal(line, []byte(installBegin)):
			if open >= 0 {
				return nil, errors.New("refusing to edit .zshrc: nested or interleaved BEGIN markers")
			}
			open = lineStart
		case bytes.Equal(line, []byte(installEnd)):
			if open < 0 {
				return nil, errors.New("refusing to edit .zshrc: END marker has no preceding BEGIN marker")
			}
			// Keep the END line's terminator outside the region so the rendered
			// block remains a complete physical line without normalizing it.
			regions = append(regions, region{start: open, end: comparisonEnd})
			open = -1
		}

		lineStart = terminatorEnd
	}
	if open >= 0 {
		return nil, errors.New("refusing to edit .zshrc: BEGIN marker has no END marker")
	}
	if len(regions) == 0 {
		if len(current) == 0 {
			return append(append([]byte(nil), block...), '\n'), nil
		}
		out := append([]byte(nil), current...)
		if !bytes.HasSuffix(out, []byte("\n")) {
			out = append(out, '\n')
		}
		out = append(out, '\n')
		return append(out, block...), nil
	}

	var out bytes.Buffer
	last := 0
	for i, region := range regions {
		out.Write(current[last:region.start])
		if i == 0 {
			out.Write(block)
		}
		last = region.end
	}
	out.Write(current[last:])
	return out.Bytes(), nil
}

func atomicWrite(path string, content []byte, defaultMode os.FileMode) (err error) {
	target := path
	info, statErr := os.Lstat(path)
	if statErr == nil && info.Mode()&os.ModeSymlink != 0 {
		target, err = filepath.EvalSymlinks(path)
		if err != nil {
			return err
		}
		info, statErr = os.Stat(target)
	}
	if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
		return statErr
	}
	mode := defaultMode
	if statErr == nil {
		mode = info.Mode().Perm()
	}
	dir := filepath.Dir(target)
	f, err := os.CreateTemp(dir, ".zsh-pro-*")
	if err != nil {
		return err
	}
	temp := f.Name()
	defer func() { _ = os.Remove(temp) }()
	if err = f.Chmod(mode); err == nil {
		_, err = f.Write(content)
	}
	if err == nil {
		err = f.Sync()
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err = os.Rename(temp, target); err != nil {
		return err
	}
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer func() { _ = d.Close() }()
	return d.Sync()
}

func validateZsh(script string) error {
	zsh, err := exec.LookPath("zsh")
	if err != nil {
		return nil
	}
	cmd := exec.Command(zsh, "-n")
	cmd.Stdin = strings.NewReader(script)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("zsh -n: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
