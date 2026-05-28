package generate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"buttress/internal/specmeta"
)

func writePromptSpec(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for rel, body := range files {
		full := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestBuildSystemPrompt_Comprehensive(t *testing.T) {
	t.Parallel()

	specDir := writePromptSpec(t, map[string]string{
		"machine/types.ts":        "export type X = number;",
		"machine/tests/x.test.ts": "test('x', () => {})",
		"human/spec.md":           "# Spec",
		"human/docs/usage.md":     "Usage docs.",
		"human/img.png":           "binary content here",
	})

	req := Request{
		Spec:        &SpecArchive{Dir: specDir},
		Language:    "typescript",
		PackageName: "@org/pkg",
		OutputPath:  "/proj/lib/pkg.ts",
	}

	prompt, err := buildSystemPrompt(req, &specmeta.SpecMeta{})
	if err != nil {
		t.Fatalf("buildSystemPrompt() error: %v", err)
	}

	// Header includes language + package + output path.
	for _, want := range []string{"typescript", "@org/pkg", "/proj/lib/pkg.ts"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt missing %q", want)
		}
	}

	// Sections rendered in expected order.
	idxTypes := strings.Index(prompt, "TYPES AND TESTS")
	idxSpec := strings.Index(prompt, "SPEC AND DOCS")
	idxInstr := strings.Index(prompt, "=== INSTRUCTIONS ===")
	if idxTypes < 0 || idxSpec < 0 || idxInstr < 0 {
		t.Fatalf("missing expected sections: types=%d spec=%d instr=%d", idxTypes, idxSpec, idxInstr)
	}
	if idxTypes >= idxSpec || idxSpec >= idxInstr {
		t.Errorf("section order wrong: types=%d spec=%d instr=%d", idxTypes, idxSpec, idxInstr)
	}

	// Spec file contents are inlined.
	for _, want := range []string{"export type X = number;", "Usage docs.", "# Spec"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt missing spec content %q", want)
		}
	}

	// Binary files are excluded.
	if strings.Contains(prompt, "binary content here") {
		t.Error("prompt should not include binary file content")
	}

	// Import path hint is included since machine/ exists.
	if !strings.Contains(prompt, "import type") {
		t.Errorf("prompt missing import-type hint, got:\n%s", prompt)
	}
}

func TestBuildSystemPrompt_NoMachineDir(t *testing.T) {
	t.Parallel()

	specDir := writePromptSpec(t, map[string]string{
		"human/spec.md": "# Just docs",
	})
	req := Request{
		Spec:        &SpecArchive{Dir: specDir},
		Language:    "go",
		PackageName: "@org/pkg",
		OutputPath:  "/out.go",
	}
	prompt, err := buildSystemPrompt(req, &specmeta.SpecMeta{})
	if err != nil {
		t.Fatalf("buildSystemPrompt() error: %v", err)
	}
	if strings.Contains(prompt, "import type") {
		t.Error("import hint should be absent when machine/ is missing")
	}
	if !strings.Contains(prompt, "# Just docs") {
		t.Error("human/ content missing")
	}
}

func TestBuildFixMessage(t *testing.T) {
	t.Parallel()
	got := buildFixMessage([]string{"--- TS errors ---\nfoo", "--- Tests ---\nbar"})
	if !strings.Contains(got, "implementation has errors") {
		t.Errorf("missing leading instruction: %q", got)
	}
	if !strings.Contains(got, "TS errors") || !strings.Contains(got, "Tests") {
		t.Errorf("missing section labels: %q", got)
	}
	// No trailing newline (TrimRight).
	if strings.HasSuffix(got, "\n") {
		t.Errorf("trailing newline present: %q", got)
	}
}

func TestCollectTextFiles_TruncatesLargeFiles(t *testing.T) {
	t.Parallel()

	dir := writePromptSpec(t, map[string]string{
		"big.txt": strings.Repeat("a", 70*1024), // > 64 KiB
	})
	files, err := collectTextFiles(dir)
	if err != nil {
		t.Fatalf("collectTextFiles() error: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("got %d files, want 1", len(files))
	}
	if !strings.HasSuffix(files[0].content, "[truncated]") {
		t.Errorf("expected truncation marker, got tail: %q", files[0].content[len(files[0].content)-30:])
	}
}

func TestCollectTextFiles_SkipsBinary(t *testing.T) {
	t.Parallel()

	dir := writePromptSpec(t, map[string]string{
		"a.md":  "alpha",
		"b.png": "binary",
		"c.zip": "archive",
	})
	files, err := collectTextFiles(dir)
	if err != nil {
		t.Fatalf("collectTextFiles() error: %v", err)
	}
	if len(files) != 1 {
		t.Errorf("got %d files, want 1; files=%v", len(files), files)
	}
}

func TestCollectTextFiles_MissingDir(t *testing.T) {
	t.Parallel()
	_, err := collectTextFiles(filepath.Join(t.TempDir(), "nope"))
	if err == nil {
		t.Error("expected error walking missing dir")
	}
}

func TestAppendSection_MissingDirIsSilent(t *testing.T) {
	t.Parallel()
	var sb strings.Builder
	if err := appendSection(&sb, filepath.Join(t.TempDir(), "missing"), "HEADING"); err != nil {
		t.Errorf("appendSection() error: %v", err)
	}
	if sb.Len() != 0 {
		t.Errorf("appendSection wrote content for missing dir: %q", sb.String())
	}
}

func TestAppendSection_EmptyDirIsSilent(t *testing.T) {
	t.Parallel()
	var sb strings.Builder
	if err := appendSection(&sb, t.TempDir(), "HEADING"); err != nil {
		t.Errorf("appendSection() error: %v", err)
	}
	if sb.Len() != 0 {
		t.Errorf("appendSection wrote content for empty dir: %q", sb.String())
	}
}

func TestCollectTextFiles_UnreadableFile(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root can read any file; permission test skipped")
	}
	dir := t.TempDir()
	// "a.md" is readable; "b.md" is not. Walk order is lexical so "a.md"
	// is collected first, then "b.md" fails, leaving files=[a] err=permission.
	if err := os.WriteFile(filepath.Join(dir, "a.md"), []byte("visible"), 0o644); err != nil {
		t.Fatal(err)
	}
	unreadable := filepath.Join(dir, "b.md")
	if err := os.WriteFile(unreadable, []byte("secret"), 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(unreadable, 0o644) })

	files, err := collectTextFiles(dir)
	if err == nil {
		t.Fatal("expected error reading unreadable file")
	}
	// The readable file was collected before the error.
	if len(files) == 0 {
		t.Log("no files collected before the error (acceptable but won't cover appendSection error path)")
	}
}

func TestAppendSection_CollectError(t *testing.T) {
	// appendSection must propagate a non-NotExist, non-empty-files error from
	// collectTextFiles. We achieve this by having one readable file (so
	// len(files) > 0) and one unreadable file (so err != nil).
	if os.Getuid() == 0 {
		t.Skip("root can read any file; permission test skipped")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.md"), []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	unreadable := filepath.Join(dir, "b.md")
	if err := os.WriteFile(unreadable, []byte("secret"), 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(unreadable, 0o644) })

	var sb strings.Builder
	err := appendSection(&sb, dir, "HEADING")
	if err == nil {
		t.Fatal("expected error from appendSection when collectTextFiles partially fails")
	}
}

func TestBuildSystemPrompt_ImportHintPrefixedWithDotSlash(t *testing.T) {
	t.Parallel()
	// When outputPath is in the spec directory itself, filepath.Rel returns
	// "machine" (no leading dot), which the code must prepend "./" to.
	specDir := writePromptSpec(t, map[string]string{
		"machine/types.ts": "export type Y = string;",
	})
	req := Request{
		Spec:        &SpecArchive{Dir: specDir},
		Language:    "typescript",
		PackageName: "@org/pkg",
		// OutputPath is inside specDir so filepath.Rel(specDir, machine) = "machine"
		OutputPath: filepath.Join(specDir, "out.ts"),
	}
	prompt, err := buildSystemPrompt(req, &specmeta.SpecMeta{})
	if err != nil {
		t.Fatalf("buildSystemPrompt() error: %v", err)
	}
	// The import hint must use "./" prefix.
	if !strings.Contains(prompt, "./machine") {
		t.Errorf("prompt missing './machine' import hint, got:\n%s", prompt)
	}
}

func TestAppendSection_WritesContentWhenFilesPresent(t *testing.T) {
	t.Parallel()
	dir := writePromptSpec(t, map[string]string{
		"spec.md": "# Hello",
	})
	var sb strings.Builder
	if err := appendSection(&sb, dir, "MY HEADING"); err != nil {
		t.Fatalf("appendSection() error: %v", err)
	}
	if !strings.Contains(sb.String(), "MY HEADING") {
		t.Errorf("output missing heading: %q", sb.String())
	}
	if !strings.Contains(sb.String(), "# Hello") {
		t.Errorf("output missing file content: %q", sb.String())
	}
}
