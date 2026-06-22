package zsh

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestIntrospectReadsAliasesAndFunctions(t *testing.T) {
	if _, err := exec.LookPath("zsh"); err != nil {
		t.Skip("zsh not installed; skipping dynamic introspection test")
	}
	dir := t.TempDir()
	cfg := filepath.Join(dir, "rc.zsh")
	content := "alias gs='git status'\ngreet() { echo hi }\nexport MYVAR=1\npath+=(\"$HOME/bin\")\nsetopt AUTO_CD\n"
	if err := os.WriteFile(cfg, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	ids, err := (Provider{}).Introspect(cfg)
	if err != nil {
		t.Fatalf("Introspect error: %v", err)
	}
	if !ids.Available {
		t.Fatal("expected Available=true")
	}
	if !ids.Aliases["gs"] {
		t.Errorf("alias gs missing from %v", ids.Aliases)
	}
	if !ids.Functions["greet"] {
		t.Errorf("function greet missing from %v", ids.Functions)
	}
	if !ids.Env["MYVAR"] {
		t.Errorf("env MYVAR missing from %v", ids.Env)
	}
}

func TestIntrospectMissingFileDegrades(t *testing.T) {
	if _, err := exec.LookPath("zsh"); err != nil {
		t.Skip("zsh not installed")
	}
	ids, err := (Provider{}).Introspect("/nonexistent/path/rc.zsh")
	// Sourcing a missing file is suppressed; introspection still returns the
	// (empty) resolved set with Available=true. The key guarantee: no panic.
	if err == nil && !ids.Available {
		t.Fatal("inconsistent: no error but Available=false")
	}
}
