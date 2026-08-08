package store

// Deterministic fake-executable coverage for the OS keychain command contracts.
// The fakes receive only the versioned base64 transport, never a plaintext test
// secret, so these tests pin both byte-exact round trips and the argv/stdin privacy
// boundary without needing a live macOS or Secret Service session.

import (
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

type fakeOSKeychain struct {
	argvPath   string
	stdinPath  string
	outputPath string
}

type osKeychainCommandSpec struct {
	name              string
	binary            string
	newDriver         func() KeychainDriver
	storeArgs         []string
	retrieveArgs      []string
	missingExit       int
	storeInput        func(string) string
	missingDiagnostic bool
}

func TestOSKeychainTransportRoundTripsWithFakeExecutables(t *testing.T) {
	for _, spec := range []osKeychainCommandSpec{
		{
			name:      "macOS security",
			binary:    "security",
			newDriver: func() KeychainDriver { return macOSKeychain{} },
			storeArgs: []string{
				"add-generic-password", "-a", "API_KEY", "-s", keychainService, "-U", "-w",
			},
			retrieveArgs: []string{
				"find-generic-password", "-a", "API_KEY", "-s", keychainService, "-w",
			},
			missingExit:       44,
			missingDiagnostic: true,
			storeInput: func(transport string) string {
				return transport + "\n" + transport + "\n"
			},
		},
		{
			name:      "Linux secret-tool",
			binary:    "secret-tool",
			newDriver: func() KeychainDriver { return linuxKeychain{} },
			storeArgs: []string{
				"store", "--label=zsh-pro API_KEY", "service", keychainService, "key", "API_KEY",
			},
			retrieveArgs: []string{
				"lookup", "service", keychainService, "key", "API_KEY",
			},
			missingExit: 1,
			storeInput:  func(transport string) string { return transport },
		},
	} {
		t.Run(spec.name, func(t *testing.T) {
			fake := installFakeOSKeychain(t, spec.binary)
			driver := spec.newDriver()

			for _, test := range []struct {
				name  string
				value string
			}{
				{name: "existing", value: "plain-secret"},
				{name: "empty", value: ""},
				{name: "trailing newline", value: "tail\n"},
				{name: "multiline", value: "line-one\nline-two\n"},
			} {
				t.Run(test.name, func(t *testing.T) {
					transport := testKeychainTransport(test.value)
					fake.reset(t)

					if err := driver.Store("API_KEY", test.value); err != nil {
						t.Fatal("Store returned an error")
					}
					fake.assertArgs(t, "store", spec.storeArgs)
					fake.assertStdin(t, "store", spec.storeInput(transport))
					fake.assertNoPlaintext(t, test.value)

					if err := os.WriteFile(fake.outputPath, []byte(transport+"\n"), 0o600); err != nil {
						t.Fatal("could not configure fake lookup output")
					}
					got, err := driver.Retrieve("API_KEY")
					if err != nil {
						t.Fatal("Retrieve returned an error")
					}
					if got != test.value {
						t.Error("Retrieve did not preserve the stored bytes")
					}
					fake.assertArgs(t, "retrieve", spec.retrieveArgs)
					fake.assertStdin(t, "retrieve", "")
				})
			}

			t.Run("missing maps to typed error", func(t *testing.T) {
				fake.reset(t)
				t.Setenv("FAKE_KEYCHAIN_MISSING_COMMAND", spec.retrieveArgs[0])
				t.Setenv("FAKE_KEYCHAIN_EXIT_STATUS", strconv.Itoa(spec.missingExit))
				if spec.missingDiagnostic {
					t.Setenv("FAKE_KEYCHAIN_MISSING_DIAGNOSTIC", "raw-missing-diagnostic")
				}

				_, err := driver.Retrieve("API_KEY")
				if !errors.Is(err, ErrSecretNotFound) {
					t.Error("missing entry did not map to ErrSecretNotFound")
				}
				if err == nil || !strings.HasPrefix(err.Error(), "zsh-pro:") {
					t.Error("missing entry did not return a zsh-pro-phrased error")
				}
				if err != nil && strings.Contains(err.Error(), "raw-missing-diagnostic") {
					t.Error("missing entry exposed backend stderr")
				}
				fake.assertArgs(t, "missing retrieve", spec.retrieveArgs)
				fake.assertStdin(t, "missing retrieve", "")
			})

			t.Run("backend failure hides stderr", func(t *testing.T) {
				fake.reset(t)
				t.Setenv("FAKE_KEYCHAIN_FAIL_COMMAND", spec.retrieveArgs[0])
				t.Setenv("FAKE_KEYCHAIN_EXIT_STATUS", "1")
				t.Setenv("FAKE_KEYCHAIN_FAILURE_TEXT", "raw-backend-diagnostic")

				_, err := driver.Retrieve("API_KEY")
				if !errors.Is(err, ErrSecretBackendUnavailable) {
					t.Error("backend failure did not map to ErrSecretBackendUnavailable")
				}
				if err != nil && strings.Contains(err.Error(), "raw-backend-diagnostic") {
					t.Error("backend failure exposed raw stderr")
				}
				fake.assertArgs(t, "failed retrieve", spec.retrieveArgs)
				fake.assertStdin(t, "failed retrieve", "")
			})
		})
	}
}

func installFakeOSKeychain(t *testing.T, binary string) fakeOSKeychain {
	t.Helper()
	root := t.TempDir()
	binDir := filepath.Join(root, "bin")
	if err := os.Mkdir(binDir, 0o700); err != nil {
		t.Fatal("could not create fake executable directory")
	}
	fake := fakeOSKeychain{
		argvPath:   filepath.Join(root, "argv"),
		stdinPath:  filepath.Join(root, "stdin"),
		outputPath: filepath.Join(root, "output"),
	}
	if err := os.WriteFile(filepath.Join(binDir, binary), []byte(fakeOSKeychainProgram), 0o700); err != nil {
		t.Fatal("could not create fake keychain executable")
	}
	path := binDir + string(os.PathListSeparator) + os.Getenv("PATH")
	t.Setenv("PATH", path)
	t.Setenv("FAKE_KEYCHAIN_ARGV", fake.argvPath)
	t.Setenv("FAKE_KEYCHAIN_STDIN", fake.stdinPath)
	t.Setenv("FAKE_KEYCHAIN_OUTPUT", fake.outputPath)
	return fake
}

func (fake fakeOSKeychain) reset(t *testing.T) {
	t.Helper()
	for _, path := range []string{fake.argvPath, fake.stdinPath, fake.outputPath} {
		if err := os.WriteFile(path, nil, 0o600); err != nil {
			t.Fatal("could not reset fake keychain state")
		}
	}
	t.Setenv("FAKE_KEYCHAIN_FAIL_COMMAND", "")
	t.Setenv("FAKE_KEYCHAIN_MISSING_COMMAND", "")
	t.Setenv("FAKE_KEYCHAIN_MISSING_DIAGNOSTIC", "")
	t.Setenv("FAKE_KEYCHAIN_EXIT_STATUS", "")
	t.Setenv("FAKE_KEYCHAIN_FAILURE_TEXT", "")
}

func (fake fakeOSKeychain) assertArgs(t *testing.T, operation string, want []string) {
	t.Helper()
	raw, err := os.ReadFile(fake.argvPath)
	if err != nil {
		t.Fatalf("could not read %s argv", operation)
	}
	got := strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n")
	if !slices.Equal(got, want) {
		t.Errorf("%s argv did not match the privacy-safe command contract", operation)
	}
}

func (fake fakeOSKeychain) assertStdin(t *testing.T, operation, want string) {
	t.Helper()
	got, err := os.ReadFile(fake.stdinPath)
	if err != nil {
		t.Fatalf("could not read %s stdin", operation)
	}
	if string(got) != want {
		t.Errorf("%s stdin did not match the versioned transport contract", operation)
	}
}

func (fake fakeOSKeychain) assertNoPlaintext(t *testing.T, value string) {
	t.Helper()
	if value == "" {
		return
	}
	argv, argvErr := os.ReadFile(fake.argvPath)
	stdin, stdinErr := os.ReadFile(fake.stdinPath)
	if argvErr != nil || stdinErr != nil {
		t.Fatal("could not inspect fake keychain transport")
	}
	if strings.Contains(string(argv), value) || strings.Contains(string(stdin), value) {
		t.Error("plaintext secret was exposed to fake command argv or stdin")
	}
}

func testKeychainTransport(value string) string {
	return keychainTransportPrefix + base64.StdEncoding.EncodeToString([]byte(value))
}

const fakeOSKeychainProgram = `#!/bin/sh
set -eu
printf '%s\n' "$@" > "$FAKE_KEYCHAIN_ARGV"
cat > "$FAKE_KEYCHAIN_STDIN"
if [ "${FAKE_KEYCHAIN_FAIL_COMMAND:-}" = "$1" ]; then
	printf '%s\n' "${FAKE_KEYCHAIN_FAILURE_TEXT:-backend failure}" >&2
	exit "${FAKE_KEYCHAIN_EXIT_STATUS:-1}"
fi
if [ "${FAKE_KEYCHAIN_MISSING_COMMAND:-}" = "$1" ]; then
	if [ -n "${FAKE_KEYCHAIN_MISSING_DIAGNOSTIC:-}" ]; then
		printf '%s\n' "$FAKE_KEYCHAIN_MISSING_DIAGNOSTIC" >&2
	fi
	exit "${FAKE_KEYCHAIN_EXIT_STATUS:-1}"
fi
case "$1" in
	find-generic-password|lookup)
		cat "$FAKE_KEYCHAIN_OUTPUT"
		;;
esac
`
