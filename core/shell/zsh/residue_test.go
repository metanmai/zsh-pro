package zsh

import (
	"os/exec"
	"strings"
	"testing"

	"zsh-pro/core/activate"
)

func TestEmitBalancedSequencesLeaveNoResidue(t *testing.T) {
	if _, err := exec.LookPath("zsh"); err != nil {
		t.Skip("zsh not installed")
	}
	p := activate.Plan{Activate: []activate.Op{
		activate.SetScalar{Name: "ZP_RESIDUE", Applied: "profile", Dynamic: false},
		activate.ApplyListDelta{Name: "PATH", Additions: []string{"/usr/local/bin"}},
		activate.AddAlias{Name: "ll", Body: "ls -l", Dynamic: false},
		activate.AddFunc{Name: "ff", Body: "print profile"},
		activate.SetOption{Name: "extendedglob", Enabled: true},
	}, Deactivate: []activate.Op{
		activate.RestoreScalar{Name: "ZP_RESIDUE", Applied: "profile"},
		activate.RebuildListFromBase{Name: "PATH"},
		activate.Unalias{Name: "ll"}, activate.RestoreShadowedAlias{Name: "ll"},
		activate.UnsetFunc{Name: "ff"}, activate.RestoreShadowedFunc{Name: "ff"},
		activate.RestoreOption{Name: "extendedglob"},
	}}
	a, d, err := (Provider{}).Emit(p)
	if err != nil {
		t.Fatal(err)
	}
	// Define the loader blocks once, snapshot after setup, and run 20 balanced
	// apply/deactivate cycles in one zsh process so PATH and non-exported state
	// are compared in the same shell.
	var seq strings.Builder
	seq.WriteString("PATH=/usr/local/bin:/usr/bin; ZP_BASE_PATH=$PATH; ZP_RESIDUE=base; alias ll='old'; ff() { print old; }; setopt extendedglob\n")
	seq.WriteString(a + "\n" + d + "\n")
	seq.WriteString("before=\"$ZP_RESIDUE|$PATH|${aliases[ll]}|${functions[ff]}|$options[extendedglob]\"\n")
	for i := 0; i < 20; i++ {
		seq.WriteString("zp_apply; zp_deactivate;\n")
	}
	seq.WriteString("after=\"$ZP_RESIDUE|$PATH|${aliases[ll]}|${functions[ff]}|$options[extendedglob]\"; [[ $before == $after ]] || { print -u2 -- \"$before -> $after\"; exit 1; }; print PASS\n")
	out, err := exec.Command("zsh", "-f", "-c", seq.String()).CombinedOutput()
	if err != nil || !strings.Contains(string(out), "PASS") {
		t.Fatalf("residue: %v\n%s", err, out)
	}
}
