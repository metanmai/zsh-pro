// Package zsh is the zsh implementation of shell.Provider.
package zsh

import "zsh-pro/core/shell"

// Provider implements shell.Provider for zsh.
type Provider struct{}

var _ shell.Emitter = Provider{}
