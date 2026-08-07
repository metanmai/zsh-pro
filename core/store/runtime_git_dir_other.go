//go:build !linux && !darwin

package store

import "os"

func runtimeGitWorkingDirectory(*os.File) (string, error) {
	return "", ErrGitCommand
}
