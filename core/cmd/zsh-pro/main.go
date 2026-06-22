package main

import (
	"fmt"
	"os"

	"zsh-pro/core/buildinfo"
)

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "--version" || os.Args[1] == "-v") {
		fmt.Printf("zsh-pro %s\n", buildinfo.Version)
		return
	}
	fmt.Fprintln(os.Stderr, "usage: zsh-pro analyze [path] [--json]")
	os.Exit(2)
}
