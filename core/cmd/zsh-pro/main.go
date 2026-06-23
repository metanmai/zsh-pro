package main

import (
	"os"

	"zsh-pro/core/cli"
	"zsh-pro/core/shell/zsh"
)

func main() {
	os.Exit(cli.New(zsh.Provider{}).Run(os.Args[1:], os.Stdout, os.Stderr))
}
