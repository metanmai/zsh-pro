package cli

import (
	"bufio"
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
)

const ingestE2EGitProxyModeEnv = "PHASE6_GIT_PROXY_MODE"

// TestMain lets the ambiguity E2E symlink the already-built test binary as
// `git`. This keeps the real update-ref transaction while avoiding Bash's
// post-3.2 coproc feature on native macOS hosts.
func TestMain(m *testing.M) {
	if os.Getenv(ingestE2EGitProxyModeEnv) == "1" {
		os.Exit(runIngestE2EGitProxy(os.Args[1:]))
	}
	os.Exit(m.Run())
}

func runIngestE2EGitProxy(args []string) int {
	realGit := os.Getenv("PHASE6_REAL_GIT")
	marker := os.Getenv("PHASE6_AMBIGUITY_MARKER")
	if realGit == "" || marker == "" {
		return 125
	}
	if len(args) > 0 && args[0] == "for-each-ref" {
		if _, err := os.Stat(marker); err == nil {
			return 97
		} else if !errors.Is(err, os.ErrNotExist) {
			return 125
		}
	}
	if len(args) == 3 && args[0] == "update-ref" && args[1] == "--no-deref" && args[2] == "--stdin" {
		return proxyAmbiguousUpdateRef(realGit, marker, args)
	}
	return runRealGitProxyCommand(realGit, args)
}

func runRealGitProxyCommand(realGit string, args []string) int {
	command := exec.Command(realGit, args...)
	command.Stdin = os.Stdin
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	return proxyCommandExitCode(command.Run())
}

func proxyAmbiguousUpdateRef(realGit, marker string, args []string) int {
	command := exec.Command(realGit, args...)
	command.Stderr = os.Stderr
	childInput, err := command.StdinPipe()
	if err != nil {
		return 125
	}
	childOutput, err := command.StdoutPipe()
	if err != nil {
		_ = childInput.Close()
		return 125
	}
	if err := command.Start(); err != nil {
		_ = childInput.Close()
		return 125
	}

	parentReader := bufio.NewReader(os.Stdin)
	parentWriter := bufio.NewWriter(os.Stdout)
	childReader := bufio.NewReader(childOutput)
	childWriter := bufio.NewWriter(childInput)
	for {
		frame, readErr := parentReader.ReadString('\n')
		if frame != "" {
			if _, err := childWriter.WriteString(frame); err != nil || childWriter.Flush() != nil {
				_ = childInput.Close()
				_ = command.Wait()
				return 125
			}
			switch strings.TrimSuffix(frame, "\n") {
			case "start", "prepare", "abort":
				reply, err := childReader.ReadString('\n')
				if err != nil {
					_ = childInput.Close()
					_ = command.Wait()
					return 125
				}
				if _, err := parentWriter.WriteString(reply); err != nil || parentWriter.Flush() != nil {
					_ = childInput.Close()
					_ = command.Wait()
					return 125
				}
			case "commit":
				reply, err := childReader.ReadString('\n')
				if err != nil || reply != "commit: ok\n" || os.WriteFile(marker, nil, 0o600) != nil {
					_ = childInput.Close()
					_ = command.Wait()
					return 125
				}
				_ = childInput.Close()
				_ = command.Wait()
				return 98
			}
		}
		if readErr != nil {
			_ = childInput.Close()
			if !errors.Is(readErr, io.EOF) {
				_ = command.Wait()
				return 125
			}
			return proxyCommandExitCode(command.Wait())
		}
	}
}

func proxyCommandExitCode(err error) int {
	if err == nil {
		return 0
	}
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		return exitError.ExitCode()
	}
	return 125
}
