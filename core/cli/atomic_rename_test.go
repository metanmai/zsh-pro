package cli

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
)

// Task 3 wires these production entry points into the guarded install
// transaction. Keep this Task 2 boundary lint-clean without introducing a
// mutable production seam or weakening the direct per-call injection tests.
var (
	_ = atomicRenameAt
	_ = acquireTargetRootTransactionLock
)

func TestAtomicRenameCapabilityCheckIsSideEffectFree(t *testing.T) {
	dir := t.TempDir()
	before, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []atomicRenameMode{atomicRenameExchange, atomicRenameNoReplace} {
		err := atomicRenameCapabilityCheck(mode)
		if runtime.GOOS == "linux" || runtime.GOOS == "darwin" {
			if err != nil {
				t.Fatalf("capability check for mode %d = %v, want supported local adapter", mode, err)
			}
		} else if !errors.Is(err, ErrAtomicRenameUnsupported) {
			t.Fatalf("capability check for mode %d = %v, want typed unsupported", mode, err)
		}
	}
	if err := atomicRenameCapabilityCheck(atomicRenameMode(255)); err == nil {
		t.Fatal("capability check accepted an invalid mode")
	}
	after, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("pure capability check changed filesystem: before=%v after=%v", before, after)
	}
}

func TestAtomicRenameAtRejectsUnsafeBasenames(t *testing.T) {
	for _, tc := range []struct {
		name string
		from string
		to   string
		mode atomicRenameMode
	}{
		{name: "empty from", from: "", to: "to", mode: atomicRenameExchange},
		{name: "empty to", from: "from", to: "", mode: atomicRenameExchange},
		{name: "dot", from: ".", to: "to", mode: atomicRenameExchange},
		{name: "dot dot", from: "..", to: "to", mode: atomicRenameExchange},
		{name: "slash", from: "a/b", to: "to", mode: atomicRenameExchange},
		{name: "backslash", from: `a\b`, to: "to", mode: atomicRenameExchange},
		{name: "nul", from: "a\x00b", to: "to", mode: atomicRenameExchange},
		{name: "same", from: "same", to: "same", mode: atomicRenameExchange},
		{name: "invalid mode", from: "from", to: "to", mode: atomicRenameMode(255)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			call := func(uintptr, uintptr, uintptr, uintptr, uintptr, uintptr, uintptr) (uintptr, uintptr, syscall.Errno) {
				calls++
				return 0, 0, 0
			}
			if err := atomicRenameAtWithSyscall(call, 3, tc.from, tc.to, tc.mode); err == nil {
				t.Fatal("unsafe atomic rename input was accepted")
			}
			if calls != 0 {
				t.Fatalf("raw syscall calls = %d, want 0", calls)
			}
		})
	}
}

func TestAtomicRenameBetweenAtAllowsEqualBasenamesInDifferentDirectories(t *testing.T) {
	var calls int
	call := func(_ uintptr, fromDirFD uintptr, _ uintptr, toDirFD uintptr, _ uintptr, _ uintptr, _ uintptr) (uintptr, uintptr, syscall.Errno) {
		calls++
		if fromDirFD != 3 || toDirFD != 4 {
			t.Fatalf("raw directory descriptors = %d/%d, want 3/4", fromDirFD, toDirFD)
		}
		return 0, 0, 0
	}
	if err := atomicRenameBetweenAtWithSyscall(call, 3, "same", 4, "same", atomicRenameExchange); err != nil {
		t.Fatalf("cross-directory equal-basename rename: %v", err)
	}
	if calls != 1 {
		t.Fatalf("raw syscall calls = %d, want 1", calls)
	}
}

func TestAtomicRenamePerCallInjectionIsParallelSafe(t *testing.T) {
	if err := atomicRenameCapabilityCheck(atomicRenameExchange); err != nil {
		if errors.Is(err, ErrAtomicRenameUnsupported) && runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
			t.Skip("native atomic rename adapter is intentionally unsupported")
		}
		t.Fatalf("local adapter capability: %v", err)
	}

	const workers = 48
	counts := make([]atomic.Int32, workers)
	var wait sync.WaitGroup
	errorsByWorker := make(chan error, workers)
	for index := range workers {
		index := index
		wait.Add(1)
		go func() {
			defer wait.Done()
			mode := atomicRenameExchange
			if index%2 != 0 {
				mode = atomicRenameNoReplace
			}
			call := func(_ uintptr, _ uintptr, _ uintptr, _ uintptr, _ uintptr, _ uintptr, _ uintptr) (uintptr, uintptr, syscall.Errno) {
				counts[index].Add(1)
				return 0, 0, 0
			}
			if err := atomicRenameAtWithSyscall(call, 9, fmt.Sprintf("from-%d", index), fmt.Sprintf("to-%d", index), mode); err != nil {
				errorsByWorker <- err
			}
		}()
	}
	wait.Wait()
	close(errorsByWorker)
	for err := range errorsByWorker {
		t.Errorf("parallel atomic rename: %v", err)
	}
	for index := range workers {
		if got := counts[index].Load(); got != 1 {
			t.Fatalf("worker %d raw calls = %d, want exactly 1", index, got)
		}
	}
}

func TestAtomicRenameLinuxGo125ABIAndFallbackSurface(t *testing.T) {
	version := strings.TrimSpace(runLocalGo(t, "env", "GOVERSION"))
	if matched, _ := regexp.MatchString(`^go1\.25\.[0-9]+$`, version); !matched {
		t.Fatalf("GOVERSION = %q, want local Go 1.25.x", version)
	}
	if toolchain := strings.TrimSpace(runLocalGo(t, "env", "GOTOOLCHAIN")); toolchain != "local" {
		t.Fatalf("GOTOOLCHAIN = %q, want local", toolchain)
	}

	root := repositoryRoot(t)
	linuxPath := filepath.Join(root, "core", "cli", "atomic_rename_linux.go")
	linuxSource, err := os.ReadFile(linuxPath)
	if err != nil {
		t.Fatalf("read Linux adapter: %v", err)
	}
	productionTraps := parseLinuxTrapMap(t, linuxSource)
	goRoot := strings.TrimSpace(runLocalGo(t, "env", "GOROOT"))
	for _, required := range []string{"amd64", "arm64"} {
		if _, ok := productionTraps[required]; !ok {
			t.Fatalf("Linux renameat2 trap map lacks required phase target %s", required)
		}
	}
	architectures := make([]string, 0, len(productionTraps))
	for architecture := range productionTraps {
		architectures = append(architectures, architecture)
	}
	sort.Strings(architectures)
	for _, architecture := range architectures {
		want := localGoRenameat2Trap(t, goRoot, architecture)
		if got := productionTraps[architecture]; got != want {
			t.Errorf("Linux %s renameat2 trap = %d, want local Go 1.25 definition %d", architecture, got, want)
		}
	}

	sourceText := string(linuxSource)
	for _, want := range []string{
		"linuxRenameNoReplaceFlag uintptr = 1",
		"linuxRenameExchangeFlag  uintptr = 2",
	} {
		if !strings.Contains(sourceText, want) {
			t.Fatalf("Linux adapter lacks audited constant %q", want)
		}
	}
	auditAtomicRenamePlatformAST(t, linuxPath)
}

func TestAtomicRenameDarwinKeepsSyscallPointersAlive(t *testing.T) {
	darwinPath := filepath.Join(repositoryRoot(t), "core", "cli", "atomic_rename_darwin.go")
	set := token.NewFileSet()
	file, err := parser.ParseFile(set, darwinPath, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var platform *ast.FuncDecl
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if ok && function.Name.Name == "atomicRenameAtWithSyscallPlatform" {
			platform = function
			break
		}
	}
	if platform == nil {
		t.Fatal("Darwin adapter lacks atomicRenameAtWithSyscallPlatform")
	}
	var rawCallEnd token.Pos
	keepAlive := map[string]token.Pos{}
	ast.Inspect(platform.Body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		if identifier, ok := call.Fun.(*ast.Ident); ok && identifier.Name == "call" {
			rawCallEnd = call.End()
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		packageName, packageOK := selector.X.(*ast.Ident)
		argument, argumentOK := singleIdentifierArgument(call)
		if packageOK && packageName.Name == "runtime" && selector.Sel.Name == "KeepAlive" && argumentOK {
			keepAlive[argument] = call.Pos()
		}
		return true
	})
	if rawCallEnd == token.NoPos {
		t.Fatal("Darwin adapter lacks the raw rename syscall")
	}
	for _, pointer := range []string{"fromPointer", "toPointer"} {
		if keepAlive[pointer] <= rawCallEnd {
			t.Fatalf("Darwin adapter does not keep %s alive after the raw syscall", pointer)
		}
	}
}

func singleIdentifierArgument(call *ast.CallExpr) (string, bool) {
	if len(call.Args) != 1 {
		return "", false
	}
	identifier, ok := call.Args[0].(*ast.Ident)
	if !ok {
		return "", false
	}
	return identifier.Name, true
}

func runLocalGo(t *testing.T, args ...string) string {
	t.Helper()
	cmd := exec.Command("go", args...)
	cmd.Env = append(withoutEnvironmentKey(os.Environ(), "GOTOOLCHAIN"), "GOTOOLCHAIN=local")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("GOTOOLCHAIN=local go %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

func withoutEnvironmentKey(environment []string, key string) []string {
	prefix := key + "="
	filtered := make([]string, 0, len(environment))
	for _, entry := range environment {
		if !strings.HasPrefix(entry, prefix) {
			filtered = append(filtered, entry)
		}
	}
	return filtered
}

func localGoRenameat2Trap(t *testing.T, goRoot, architecture string) uint64 {
	t.Helper()
	paths := []string{
		filepath.Join(goRoot, "src", "syscall", "zsysnum_linux_"+architecture+".go"),
		filepath.Join(goRoot, "src", "cmd", "vendor", "golang.org", "x", "sys", "unix", "zsysnum_linux_"+architecture+".go"),
	}
	expression := regexp.MustCompile(`SYS_RENAMEAT2\s*=\s*([0-9]+)`)
	for _, path := range paths {
		source, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		match := expression.FindSubmatch(source)
		if len(match) != 2 {
			continue
		}
		value, err := strconv.ParseUint(string(match[1]), 10, 64)
		if err != nil {
			t.Fatal(err)
		}
		return value
	}
	t.Fatalf("local Go 1.25 sources contain no SYS_RENAMEAT2 for linux/%s", architecture)
	return 0
}

func parseLinuxTrapMap(t *testing.T, source []byte) map[string]uint64 {
	t.Helper()
	start := bytesIndexOrFatal(t, source, []byte("linuxRenameat2TrapByArch = map[string]uintptr{"))
	rest := source[start:]
	end := bytesIndexOrFatal(t, rest, []byte("}\n"))
	entries := regexp.MustCompile(`"([a-z0-9]+)"\s*:\s*([0-9]+)`).FindAllSubmatch(rest[:end], -1)
	result := make(map[string]uint64, len(entries))
	for _, entry := range entries {
		value, err := strconv.ParseUint(string(entry[2]), 10, 64)
		if err != nil {
			t.Fatal(err)
		}
		result[string(entry[1])] = value
	}
	if len(result) == 0 {
		t.Fatal("Linux adapter trap map is empty")
	}
	return result
}

func bytesIndexOrFatal(t *testing.T, source, needle []byte) int {
	t.Helper()
	index := strings.Index(string(source), string(needle))
	if index < 0 {
		t.Fatalf("source lacks %q", needle)
	}
	return index
}

func auditAtomicRenamePlatformAST(t *testing.T, path string) {
	t.Helper()
	set := token.NewFileSet()
	file, err := parser.ParseFile(set, path, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var platform *ast.FuncDecl
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if ok && function.Name.Name == "atomicRenameAtWithSyscallPlatform" {
			platform = function
			break
		}
	}
	if platform == nil {
		t.Fatal("Linux adapter lacks atomicRenameAtWithSyscallPlatform")
	}
	rawCalls := 0
	forbidden := map[string]bool{"Rename": true, "Renameat": true, "Link": true, "Linkat": true, "Unlink": true, "Unlinkat": true, "Remove": true, "RemoveAll": true}
	ast.Inspect(platform.Body, func(node ast.Node) bool {
		switch value := node.(type) {
		case *ast.ForStmt, *ast.RangeStmt:
			t.Errorf("atomic rename platform adapter contains a retry/fallback loop at %s", set.Position(node.Pos()))
		case *ast.CallExpr:
			if identifier, ok := value.Fun.(*ast.Ident); ok && identifier.Name == "call" {
				rawCalls++
				if len(value.Args) != 7 {
					t.Errorf("raw syscall arguments = %d, want trap plus six ABI arguments", len(value.Args))
				}
			}
			if selector, ok := value.Fun.(*ast.SelectorExpr); ok && forbidden[selector.Sel.Name] {
				t.Errorf("atomic rename platform adapter contains forbidden fallback %s", selector.Sel.Name)
			}
		}
		return true
	})
	if rawCalls != 1 {
		t.Fatalf("Linux adapter raw calls = %d, want exactly 1", rawCalls)
	}
}
