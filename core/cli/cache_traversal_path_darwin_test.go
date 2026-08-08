//go:build darwin

package cli

import "testing"

func TestCacheTraversalPathRecognizesTrustedMacOSCompatibilityAliases(t *testing.T) {
	for _, test := range []struct {
		name string
		path string
		want string
	}{
		{name: "var", path: "/var/zsh-pro-cache-test", want: "/private/var/zsh-pro-cache-test"},
		{name: "tmp", path: "/tmp/zsh-pro-cache-test", want: "/private/tmp/zsh-pro-cache-test"},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := cacheTraversalPath(test.path)
			if err != nil {
				t.Fatalf("cacheTraversalPath(%q): %v", test.path, err)
			}
			if got != test.want {
				t.Fatalf("cacheTraversalPath(%q) = %q, want %q", test.path, got, test.want)
			}
		})
	}
}
