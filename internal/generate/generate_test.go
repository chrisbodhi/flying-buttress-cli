package generate

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"buttress/internal/specmeta"
)

func TestStub_GenerateReturnsErrNotConfigured(t *testing.T) {
	t.Parallel()

	g := Stub{}
	err := g.Generate(context.Background(), Request{
		Spec:        &SpecArchive{Dir: "/x", ContentHash: "sha256:abc"},
		Language:    "go",
		OutputPath:  "/out/file.go",
		PackageName: "@org/pkg",
		ProjectDir:  "/proj",
	})

	if !errors.Is(err, ErrNotConfigured) {
		t.Errorf("Generate() = %v, want errors.Is ErrNotConfigured", err)
	}
}

// TestStub_ImplementsGenerator pins down the interface contract: Stub must be
// a Generator. If this stops compiling, the interface or Stub has drifted.
func TestStub_ImplementsGenerator(t *testing.T) {
	t.Parallel()
	var _ Generator = Stub{}
}

func TestSpecArchive_FieldsAccessible(t *testing.T) {
	t.Parallel()
	a := SpecArchive{Dir: "/tmp/x", ContentHash: "sha256:abc"}
	if a.Dir != "/tmp/x" {
		t.Errorf("Dir = %q", a.Dir)
	}
	if a.ContentHash != "sha256:abc" {
		t.Errorf("ContentHash = %q", a.ContentHash)
	}
}

func TestRequest_FieldsAccessible(t *testing.T) {
	t.Parallel()
	r := Request{
		Spec:        &SpecArchive{Dir: "/d"},
		Language:    "ts",
		OutputPath:  "/o.ts",
		PackageName: "@a/b",
		ProjectDir:  "/p",
		MaxAttempts: 5,
	}
	if r.Spec.Dir != "/d" || r.Language != "ts" || r.OutputPath != "/o.ts" {
		t.Errorf("Request fields not accessible: %+v", r)
	}
	if r.PackageName != "@a/b" || r.ProjectDir != "/p" || r.MaxAttempts != 5 {
		t.Errorf("Request fields not accessible: %+v", r)
	}
}

func TestProgress_NilCallback(t *testing.T) {
	t.Parallel()
	// progress must not panic when Progress is nil.
	r := Request{Progress: nil}
	r.progress("hello %s", "world") // must not panic
}

func TestProgress_NonNilCallback(t *testing.T) {
	t.Parallel()
	var got string
	r := Request{Progress: func(msg string) { got = msg }}
	r.progress("attempt %d/%d: calling LLM…", 1, 3)
	if got != "attempt 1/3: calling LLM…" {
		t.Errorf("progress callback got %q, want formatted string", got)
	}
}

func TestLooksLikeTSError_True(t *testing.T) {
	t.Parallel()
	cases := []string{
		"src/index.ts(1,5): error TS2304: Cannot find name 'x'.",
		"error TS1005: ';' expected.",
		"Multiple error TS2345: issues here",
	}
	for _, c := range cases {
		if !looksLikeTSError(c) {
			t.Errorf("looksLikeTSError(%q) = false, want true", c)
		}
	}
}

func TestLooksLikeTSError_False(t *testing.T) {
	t.Parallel()
	cases := []string{
		"",
		"npm error ENOENT",
		"command not found: tsc",
		"error: something went wrong",
		"Error: module not found",
	}
	for _, c := range cases {
		if looksLikeTSError(c) {
			t.Errorf("looksLikeTSError(%q) = true, want false", c)
		}
	}
}

func TestInstallTestPackages_NoPackages(t *testing.T) {
	t.Parallel()
	// When TestPackages returns empty, installTestPackages is a no-op.
	req := Request{
		Language:   "typescript",
		ProjectDir: t.TempDir(),
	}
	meta := &specmeta.SpecMeta{} // no languages — TestPackages returns nil
	if err := installTestPackages(context.Background(), req, meta); err != nil {
		t.Errorf("installTestPackages with no packages: %v", err)
	}
}

func TestInstallTestPackages_NpmNotFound(t *testing.T) {
	// Not parallel: mutates PATH
	t.Setenv("PATH", "")

	req := Request{
		Language:   "typescript",
		ProjectDir: t.TempDir(),
	}
	meta := &specmeta.SpecMeta{
		Languages: []specmeta.Language{
			{
				Name: "typescript",
				Tests: []specmeta.TestConfig{
					{Packages: [][]string{{"vitest", "3.0.0"}}},
				},
			},
		},
	}
	err := installTestPackages(context.Background(), req, meta)
	if err == nil {
		t.Fatal("expected error when npm is not on PATH")
	}
	if !strings.Contains(err.Error(), "installing test packages") {
		t.Errorf("error = %q, want substring 'installing test packages'", err)
	}
}

func TestInstallTestPackages_BunNotFound(t *testing.T) {
	t.Setenv("PATH", "")

	req := Request{
		Language:   "typescript",
		ProjectDir: t.TempDir(),
	}
	meta := &specmeta.SpecMeta{
		Languages: []specmeta.Language{
			{
				Name:           "typescript",
				PackageManager: "bun",
				Tests: []specmeta.TestConfig{
					{Packages: [][]string{{"vitest", "3.0.0"}}},
				},
			},
		},
	}
	err := installTestPackages(context.Background(), req, meta)
	if err == nil {
		t.Fatal("expected error when bun is not on PATH")
	}
	if !strings.Contains(err.Error(), "installing test packages") {
		t.Errorf("error = %q, want substring 'installing test packages'", err)
	}
}

func TestInstallTestPackages_PnpmNotFound(t *testing.T) {
	t.Setenv("PATH", "")

	req := Request{
		Language:   "typescript",
		ProjectDir: t.TempDir(),
	}
	meta := &specmeta.SpecMeta{
		Languages: []specmeta.Language{
			{
				Name:           "typescript",
				PackageManager: "pnpm",
				Tests: []specmeta.TestConfig{
					{Packages: [][]string{{"vitest", "3.0.0"}}},
				},
			},
		},
	}
	err := installTestPackages(context.Background(), req, meta)
	if err == nil {
		t.Fatal("expected error when pnpm is not on PATH")
	}
	if !strings.Contains(err.Error(), "installing test packages") {
		t.Errorf("error = %q, want substring 'installing test packages'", err)
	}
}

func TestInstallTestPackages_YarnNotFound(t *testing.T) {
	t.Setenv("PATH", "")

	req := Request{
		Language:   "typescript",
		ProjectDir: t.TempDir(),
	}
	meta := &specmeta.SpecMeta{
		Languages: []specmeta.Language{
			{
				Name:           "typescript",
				PackageManager: "yarn",
				Tests: []specmeta.TestConfig{
					{Packages: [][]string{{"vitest", "3.0.0"}}},
				},
			},
		},
	}
	err := installTestPackages(context.Background(), req, meta)
	if err == nil {
		t.Fatal("expected error when yarn is not on PATH")
	}
	if !strings.Contains(err.Error(), "installing test packages") {
		t.Errorf("error = %q, want substring 'installing test packages'", err)
	}
}

func TestLLMGenerator_Generate_InvalidSpecMeta(t *testing.T) {
	t.Parallel()
	// Make buttress.toml a directory so specmeta.Load returns an error.
	specDir := t.TempDir()
	if err := os.Mkdir(filepath.Join(specDir, "buttress.toml"), 0o755); err != nil {
		t.Fatal(err)
	}

	srv := newFakeChatServer(t)
	srv.replies = []string{"impl"}
	g := NewLLMGenerator(LLMConfig{BaseURL: srv.URL(), Model: "m"})
	err := g.Generate(context.Background(), Request{
		Spec:       &SpecArchive{Dir: specDir},
		Language:   "go",
		OutputPath: filepath.Join(t.TempDir(), "out.go"),
		ProjectDir: t.TempDir(),
	})
	if err == nil {
		t.Fatal("expected error loading invalid spec metadata")
	}
	if !strings.Contains(err.Error(), "loading spec metadata") {
		t.Errorf("error = %q, want substring 'loading spec metadata'", err)
	}
}

func TestLLMGenerator_Generate_WriteFileError(t *testing.T) {
	t.Parallel()
	srv := newFakeChatServer(t)
	srv.replies = []string{"impl"}
	g := NewLLMGenerator(LLMConfig{BaseURL: srv.URL(), Model: "m"})

	// outputPath parent exists but is read-only so WriteFile will fail.
	dir := t.TempDir()
	roDir := filepath.Join(dir, "ro")
	if err := os.MkdirAll(roDir, 0o555); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(roDir, "out.go")

	err := g.Generate(context.Background(), Request{
		Spec:       writeMinimalSpec(t),
		Language:   "go",
		OutputPath: out,
		ProjectDir: t.TempDir(),
	})
	if err == nil {
		t.Fatal("expected error writing to read-only directory")
	}
	if !strings.Contains(err.Error(), "writing generated file") {
		t.Errorf("error = %q, want substring 'writing generated file'", err)
	}
}

func TestLLMGenerator_Generate_ExhaustsMaxAttempts(t *testing.T) {
	// Not parallel: mutates PATH so TypeScript verification fails early
	// in a predictable way (npx not found → tooling error → verify error).
	t.Setenv("PATH", "")

	srv := newFakeChatServer(t)
	srv.replies = []string{"impl", "impl", "impl"}
	g := NewLLMGenerator(LLMConfig{BaseURL: srv.URL(), Model: "m"})

	// Use TypeScript so verifyTypeScript is called; with PATH="" npx is not
	// found and verifyTypeScript returns an error, which surfaces via
	// "running verification".
	err := g.Generate(context.Background(), Request{
		Spec:        writeMinimalSpec(t),
		Language:    "typescript",
		OutputPath:  filepath.Join(t.TempDir(), "out.ts"),
		PackageName: "@org/pkg",
		ProjectDir:  t.TempDir(),
		MaxAttempts: 1, // single attempt so we hit the exhausted-attempts path
	})
	if err == nil {
		t.Fatal("expected error when verification fails")
	}
	// With PATH="" and npx missing, verifyTypeScript returns "running tsc" error,
	// which surfaces as "running verification".
	if !strings.Contains(err.Error(), "running verification") {
		t.Errorf("error = %q, want substring 'running verification'", err)
	}
}
