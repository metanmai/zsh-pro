package zsh

import (
	"math/rand"
	"os/exec"
	"testing"
)

const (
	residueSeed        int64 = 0x5eed0405
	residueActionCount       = 24
)

type residueAction struct {
	profile int
	apply   bool
}

func generateBalancedActions(_ *rand.Rand, _ int, _ int) []residueAction {
	return nil
}

func actionsAreBalanced(actions []residueAction) bool {
	active := -1
	for _, action := range actions {
		if action.apply {
			if active != -1 {
				return false
			}
			active = action.profile
			continue
		}
		if active != action.profile {
			return false
		}
		active = -1
	}
	return active == -1
}

func TestBalancedResidueActionGenerator(t *testing.T) {
	actions := generateBalancedActions(rand.New(rand.NewSource(residueSeed)), residueActionCount, 2)
	if len(actions) < residueActionCount {
		t.Fatalf("seed=%d: got %d actions, want at least %d", residueSeed, len(actions), residueActionCount)
	}
	if !actionsAreBalanced(actions) {
		t.Fatalf("seed=%d: generated an unbalanced trace: %#v", residueSeed, actions)
	}
}

func TestZeroResidueFullStateProperty(t *testing.T) {
	if _, err := exec.LookPath("zsh"); err != nil {
		t.Skip("zsh not installed")
	}
	t.Fatalf("seed=%d: full-state residue oracle not implemented", residueSeed)
}
