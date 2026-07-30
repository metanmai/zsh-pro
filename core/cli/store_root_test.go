package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStoreRootUsesDocumentedPrecedenceAndRejectsUnsafeInputs(t *testing.T) {
	home := t.TempDir()
	xdg := t.TempDir()
	explicit := filepath.Join(t.TempDir(), "profiles")

	tests := []struct {
		name    string
		env     map[string]string
		want    string
		errPart string
	}{
		{
			name: "HOME fallback",
			env:  map[string]string{"HOME": home},
			want: filepath.Join(home, ".local", "share", "zsh-pro"),
		},
		{
			name: "absolute XDG data home",
			env: map[string]string{
				"HOME":          home,
				"XDG_DATA_HOME": xdg,
			},
			want: filepath.Join(xdg, "zsh-pro"),
		},
		{
			name: "explicit profile root",
			env: map[string]string{
				"HOME":          home,
				"XDG_DATA_HOME": xdg,
				"ZSHPRO_HOME":   explicit,
			},
			want: explicit,
		},
		{
			name:    "empty explicit profile root",
			env:     map[string]string{"HOME": home, "ZSHPRO_HOME": ""},
			errPart: "ZSHPRO_HOME",
		},
		{
			name:    "relative explicit profile root",
			env:     map[string]string{"HOME": home, "ZSHPRO_HOME": "profiles"},
			errPart: "ZSHPRO_HOME",
		},
		{
			name:    "empty XDG data home",
			env:     map[string]string{"HOME": home, "XDG_DATA_HOME": ""},
			errPart: "XDG_DATA_HOME",
		},
		{
			name:    "relative XDG data home",
			env:     map[string]string{"HOME": home, "XDG_DATA_HOME": "profiles"},
			errPart: "XDG_DATA_HOME",
		},
		{
			name:    "missing HOME",
			errPart: "HOME",
		},
		{
			name:    "empty HOME",
			env:     map[string]string{"HOME": ""},
			errPart: "HOME",
		},
		{
			name:    "relative HOME",
			env:     map[string]string{"HOME": "profiles"},
			errPart: "HOME",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setStoreRootEnvironment(t, tt.env)
			got, err := StoreRoot()
			if tt.errPart != "" {
				if err == nil || !strings.Contains(err.Error(), tt.errPart) {
					t.Fatalf("StoreRoot() error = %v, want %q", err, tt.errPart)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Fatalf("StoreRoot() = %q, want %q", got, tt.want)
			}
		})
	}
}

func setStoreRootEnvironment(t *testing.T, values map[string]string) {
	t.Helper()
	for _, name := range []string{"HOME", "XDG_DATA_HOME", "ZSHPRO_HOME"} {
		name := name
		old, present := os.LookupEnv(name)
		t.Cleanup(func() {
			if present {
				_ = os.Setenv(name, old)
				return
			}
			_ = os.Unsetenv(name)
		})
		if err := os.Unsetenv(name); err != nil {
			t.Fatal(err)
		}
	}
	for name, value := range values {
		if err := os.Setenv(name, value); err != nil {
			t.Fatal(err)
		}
	}
}
