package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"zsh-pro/core/buildinfo"
	"zsh-pro/core/shell"
)

const (
	installBegin = "# >>> zsh-pro >>>"
	installEnd   = "# <<< zsh-pro <<<"

	validationTimeout = 5 * time.Second
)

type installPaths struct {
	runtimeDir string
	zshrcPath  string
}

// runInstall prepares the complete .zshrc replacement before it changes cache
// state. The cache path is deliberately resolved by the same environment
// expression rendered in the stub, so sourcing never depends on a binary
// invocation during shell startup.
func runInstall(provider shell.Hooker) error {
	paths, err := resolveInstallPaths()
	if err != nil {
		return err
	}
	current, err := os.ReadFile(paths.zshrcPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("read %s: %w", paths.zshrcPath, err)
	}
	next, err := replaceManagedBlock(current, renderInstallBlock())
	if err != nil {
		return err
	}

	cacheDir := paths.runtimeDir
	if err := os.MkdirAll(cacheDir, 0o700); err != nil {
		return fmt.Errorf("create runtime directory: %w", err)
	}
	if err := os.Chmod(cacheDir, 0o700); err != nil {
		return fmt.Errorf("secure runtime directory: %w", err)
	}
	loader := filepath.Join(cacheDir, "loader.zsh")
	loaderScript := "# zsh-pro cached loader version " + buildinfo.Version + "\n" + provider.HookScript()
	if err := writeValidatedLoader(loader, []byte(loaderScript)); err != nil {
		return fmt.Errorf("install cached loader: %w", err)
	}
	if err := atomicWrite(paths.zshrcPath, next, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", paths.zshrcPath, err)
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

func resolveInstallPaths() (installPaths, error) {
	home, present := os.LookupEnv("HOME")
	if !present || home == "" || !filepath.IsAbs(home) {
		return installPaths{}, errors.New("install requires a non-empty absolute HOME")
	}

	runtime := filepath.Join(home, ".zsh-pro")
	if configured, present := os.LookupEnv("ZSHPRO_HOME"); present {
		if configured == "" || !filepath.IsAbs(configured) {
			return installPaths{}, errors.New("install requires ZSHPRO_HOME to be a non-empty absolute path when set")
		}
		runtime = configured
	}

	zdotdir := home
	if configured, present := os.LookupEnv("ZDOTDIR"); present {
		if configured == "" || !filepath.IsAbs(configured) {
			return installPaths{}, errors.New("install requires ZDOTDIR to be a non-empty absolute path when set")
		}
		zdotdir = configured
	}

	return installPaths{
		runtimeDir: runtime,
		zshrcPath:  filepath.Join(zdotdir, ".zshrc"),
	}, nil
}

func renderInstallBlock() []byte {
	return []byte(installBegin + "\n" +
		"if [[ -z \"${ZSHPRO_DISABLE-}\" ]]; then\n" +
		"  if [[ -f \"${ZSHPRO_HOME:-$HOME/.zsh-pro}/loader.zsh\" && -r \"${ZSHPRO_HOME:-$HOME/.zsh-pro}/loader.zsh\" ]]; then\n" +
		"    if source \"${ZSHPRO_HOME:-$HOME/.zsh-pro}/loader.zsh\"; then\n" +
		"      :\n" +
		"    else\n" +
		"      :\n" +
		"    fi\n" +
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
	target, err := resolveWriteTarget(path)
	if err != nil {
		return err
	}
	mode := defaultMode
	info, statErr := os.Stat(target)
	if statErr == nil {
		mode = info.Mode().Perm()
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return statErr
	}
	temp, err := writeTemp(target, content, mode)
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(temp) }()
	if err = os.Rename(temp, target); err != nil {
		return err
	}
	return syncDirectory(filepath.Dir(target))
}

func writeValidatedLoader(path string, content []byte) error {
	target, err := resolveWriteTarget(path)
	if err != nil {
		return err
	}
	temp, err := writeTemp(target, content, 0o600)
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(temp) }()
	if err := validateZsh(temp); err != nil {
		return err
	}
	if err := os.Rename(temp, target); err != nil {
		return err
	}
	return syncDirectory(filepath.Dir(target))
}

func resolveWriteTarget(path string) (string, error) {
	target := path
	info, err := os.Lstat(path)
	if err == nil && info.Mode()&os.ModeSymlink != 0 {
		target, err = filepath.EvalSymlinks(path)
		if err != nil {
			return "", err
		}
		return target, nil
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	return target, nil
}

func writeTemp(target string, content []byte, mode os.FileMode) (string, error) {
	f, err := os.CreateTemp(filepath.Dir(target), ".zsh-pro-loader-*")
	if err != nil {
		return "", err
	}
	temp := f.Name()
	cleanup := func() {
		_ = f.Close()
		_ = os.Remove(temp)
	}
	if err := f.Chmod(mode); err != nil {
		cleanup()
		return "", err
	}
	if n, err := f.Write(content); err != nil {
		cleanup()
		return "", err
	} else if n != len(content) {
		cleanup()
		return "", io.ErrShortWrite
	}
	if err := f.Sync(); err != nil {
		cleanup()
		return "", err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(temp)
		return "", err
	}
	return temp, nil
}

func syncDirectory(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer func() { _ = d.Close() }()
	return d.Sync()
}

func validateZsh(path string) error {
	zsh, err := exec.LookPath("zsh")
	if err != nil {
		return fmt.Errorf("find zsh for loader validation: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), validationTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, zsh, "-n", path)
	if out, err := cmd.CombinedOutput(); err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return errors.New("zsh -n validation timed out")
		}
		return fmt.Errorf("zsh -n: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
