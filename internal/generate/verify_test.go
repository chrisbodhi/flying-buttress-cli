package generate

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"buttress/internal/specmeta"
)

func TestTrimCmdOutput(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"trims surrounding whitespace", "  \nhello\n  ", "hello"},
		{"short stays short", "ok", "ok"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := trimCmdOutput(tt.in); got != tt.want {
				t.Errorf("trimCmdOutput(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestTrimCmdOutput_TruncatesLongOutput(t *testing.T) {
	t.Parallel()
	in := strings.Repeat("a", 10*1024) // 10 KiB
	got := trimCmdOutput(in)
	if !strings.HasSuffix(got, "[output truncated]") {
		t.Errorf("expected truncation marker, got tail: %q", got[len(got)-30:])
	}
	if len(got) > 10*1024 {
		t.Errorf("output not truncated: len=%d", len(got))
	}
}

func TestRunCmd_Success(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("relies on POSIX 'true' / 'echo'")
	}
	t.Parallel()
	res, err := runCmd(context.Background(), t.TempDir(), "echo", "hi")
	if err != nil {
		t.Fatalf("runCmd() error: %v", err)
	}
	if res.exitCode != 0 {
		t.Errorf("exitCode = %d, want 0", res.exitCode)
	}
	if !strings.Contains(res.output, "hi") {
		t.Errorf("output = %q, want substring 'hi'", res.output)
	}
}

func TestRunCmd_NonZeroExit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("relies on POSIX 'false'")
	}
	t.Parallel()
	res, err := runCmd(context.Background(), t.TempDir(), "false")
	if err != nil {
		t.Fatalf("runCmd() error: %v (non-zero exit shouldn't surface as error)", err)
	}
	if res.exitCode == 0 {
		t.Errorf("exitCode = 0, want non-zero from 'false'")
	}
}

func TestRunCmd_BinaryNotFound(t *testing.T) {
	t.Parallel()
	_, err := runCmd(context.Background(), t.TempDir(), "buttress-no-such-binary-9f3a2d")
	if err == nil {
		t.Fatal("expected error for missing binary")
	}
}

func TestWriteTempTSConfig(t *testing.T) {
	t.Parallel()
	path, cleanup, err := writeTempTSConfig("/tmp/out.ts")
	if err != nil {
		t.Fatalf("writeTempTSConfig() error: %v", err)
	}
	defer cleanup()

	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Errorf("not valid JSON: %v", err)
	}
	co, ok := parsed["compilerOptions"].(map[string]any)
	if !ok {
		t.Fatal("missing compilerOptions")
	}
	if co["strict"] != true {
		t.Errorf("strict = %v, want true", co["strict"])
	}
	if co["noEmit"] != true {
		t.Errorf("noEmit = %v, want true", co["noEmit"])
	}
	files, _ := parsed["files"].([]any)
	if len(files) != 1 || files[0] != "/tmp/out.ts" {
		t.Errorf("files = %v, want [/tmp/out.ts]", files)
	}

	// Cleanup actually removes the file.
	cleanup()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("temp file still exists after cleanup: %v", err)
	}
}

func TestWriteTempBunTSConfig(t *testing.T) {
	t.Parallel()
	path, cleanup, err := writeTempBunTSConfig("/abs/out.ts", "@org/pkg")
	if err != nil {
		t.Fatalf("writeTempBunTSConfig() error: %v", err)
	}
	defer cleanup()

	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `"@org/pkg"`) {
		t.Errorf("config missing pkgName, got %s", body)
	}
	if !strings.Contains(string(body), `"/abs/out.ts"`) {
		t.Errorf("config missing outputPath, got %s", body)
	}
}

func TestWriteTempVitestConfig(t *testing.T) {
	t.Parallel()
	path, cleanup, err := writeTempVitestConfig("/abs/out.ts", "/spec/machine", "@org/pkg")
	if err != nil {
		t.Fatalf("writeTempVitestConfig() error: %v", err)
	}
	defer cleanup()

	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	s := string(body)
	if !strings.Contains(s, `"@org/pkg"`) {
		t.Errorf("config missing pkgName, got %s", s)
	}
	if !strings.Contains(s, "/abs/out.ts") {
		t.Errorf("config missing outputPath, got %s", s)
	}
	if !strings.Contains(s, "test:") {
		t.Errorf("config missing test block, got %s", s)
	}
}

func TestVerifyTypeScript_NoNpxAvailable(t *testing.T) {
	// Not parallel: mutates $PATH.
	t.Setenv("PATH", "")

	req := Request{
		Spec:        &SpecArchive{Dir: t.TempDir()},
		Language:    "typescript",
		OutputPath:  filepath.Join(t.TempDir(), "out.ts"),
		PackageName: "@org/pkg",
		ProjectDir:  t.TempDir(),
	}
	_, err := verifyTypeScript(context.Background(), req, &specmeta.SpecMeta{})
	if err == nil {
		t.Fatal("expected error when npx is missing from PATH")
	}
	if !strings.Contains(err.Error(), "running tsc") {
		t.Errorf("error = %q, want substring 'running tsc'", err)
	}
}

func TestRunTSCheck_NoNpxAvailable(t *testing.T) {
	t.Setenv("PATH", "")

	out, err := runTSCheck(context.Background(), filepath.Join(t.TempDir(), "out.ts"), t.TempDir())
	if err == nil {
		t.Fatalf("expected error when npx is missing, got out=%q", out)
	}
	if !strings.Contains(err.Error(), "running tsc") {
		t.Errorf("error = %q, want substring 'running tsc'", err)
	}
}

func TestRunVitest_NoNpxAvailable(t *testing.T) {
	t.Setenv("PATH", "")

	specDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(specDir, "machine"), 0o755); err != nil {
		t.Fatal(err)
	}
	req := Request{
		Spec:        &SpecArchive{Dir: specDir},
		Language:    "typescript",
		PackageName: "@org/pkg",
		OutputPath:  filepath.Join(t.TempDir(), "out.ts"),
		ProjectDir:  t.TempDir(),
	}
	_, err := runTSTests(context.Background(), req, &specmeta.SpecMeta{
		Languages: []specmeta.Language{
			{Name: "typescript", Tests: []specmeta.TestConfig{{Runner: "vitest", RunnerVersion: "3.2.4"}}},
		},
	})
	if err == nil {
		t.Fatal("expected error when npx is missing")
	}
	if !strings.Contains(err.Error(), "running vitest") {
		t.Errorf("error = %q, want substring 'running vitest'", err)
	}
}

func TestRunBunTest_NoBunAvailable(t *testing.T) {
	t.Setenv("PATH", "")

	specDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(specDir, "machine"), 0o755); err != nil {
		t.Fatal(err)
	}
	req := Request{
		Spec:        &SpecArchive{Dir: specDir},
		Language:    "typescript",
		PackageName: "@org/pkg",
		OutputPath:  filepath.Join(t.TempDir(), "out.ts"),
		ProjectDir:  t.TempDir(),
	}
	_, err := runTSTests(context.Background(), req, &specmeta.SpecMeta{
		Languages: []specmeta.Language{
			{Name: "typescript", Tests: []specmeta.TestConfig{{Runner: "bun"}}},
		},
	})
	if err == nil {
		t.Fatal("expected error when bun is missing")
	}
	if !strings.Contains(err.Error(), "running bun test") {
		t.Errorf("error = %q, want substring 'running bun test'", err)
	}
}

func TestRunTSTests_NoMachineDirSkips(t *testing.T) {
	t.Parallel()
	// When the spec has no machine/ directory, runTSTests is a no-op.
	req := Request{
		Spec:       &SpecArchive{Dir: t.TempDir()},
		Language:   "typescript",
		ProjectDir: t.TempDir(),
	}
	out, err := runTSTests(context.Background(), req, &specmeta.SpecMeta{})
	if err != nil {
		t.Errorf("runTSTests() error: %v", err)
	}
	if out != "" {
		t.Errorf("output = %q, want empty", out)
	}
}

func TestRunTSTests_UnsupportedRunner(t *testing.T) {
	t.Parallel()
	specDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(specDir, "machine"), 0o755); err != nil {
		t.Fatal(err)
	}

	req := Request{
		Spec:       &SpecArchive{Dir: specDir},
		Language:   "typescript",
		ProjectDir: t.TempDir(),
	}
	meta := &specmeta.SpecMeta{
		Languages: []specmeta.Language{
			{Name: "typescript", Tests: []specmeta.TestConfig{{Runner: "rspec"}}},
		},
	}
	_, err := runTSTests(context.Background(), req, meta)
	if err == nil {
		t.Fatal("expected error for unsupported runner")
	}
	if !strings.Contains(err.Error(), "unsupported test runner") {
		t.Errorf("error = %q, want substring 'unsupported test runner'", err)
	}
}

func TestVerifyOutput_TypeScriptDispatch(t *testing.T) {
	// Not parallel: mutates $PATH so we don't need the real toolchain.
	t.Setenv("PATH", "")

	req := Request{
		Spec:       &SpecArchive{Dir: t.TempDir()},
		Language:   "TypeScript", // case-insensitive
		ProjectDir: t.TempDir(),
		OutputPath: filepath.Join(t.TempDir(), "out.ts"),
	}
	if _, err := verifyOutput(context.Background(), req, &specmeta.SpecMeta{}); err == nil {
		t.Error("expected verifyOutput to surface tooling error for typescript")
	}
}
