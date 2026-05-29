package cmd

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"buttress/internal/store"
)

func TestNewGenerateCmd_Metadata(t *testing.T) {
	t.Parallel()
	c := newGenerateCmd()
	if !strings.HasPrefix(c.Use, "generate") {
		t.Errorf("Use = %q", c.Use)
	}
	if c.Short == "" {
		t.Error("Short is empty")
	}
}

func TestNewGenerateCmd_RequiresExactlyOneArg(t *testing.T) {
	t.Parallel()
	c := newGenerateCmd()
	if err := c.Args(c, nil); err == nil {
		t.Error("expected error for 0 args")
	}
	if err := c.Args(c, []string{"@a/b", "@c/d"}); err == nil {
		t.Error("expected error for 2 args")
	}
	if err := c.Args(c, []string{"@a/b"}); err != nil {
		t.Errorf("one arg should be allowed: %v", err)
	}
}

// runGenerateCmd reads from the global config in $HOME, so we point HOME at a
// temp dir to control its behavior.
func TestRunGenerateCmd_NoLLMConfigured(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	err := runGenerateCmd(context.Background(), "@a/b")
	if err == nil {
		t.Fatal("expected error when no LLM is configured")
	}
	if !strings.Contains(err.Error(), "LLM credentials") {
		t.Errorf("error = %q, want substring 'LLM credentials'", err)
	}
}

func TestRunGenerateCmd_BadRef(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	// Configure LLM so we get past the LLM gate.
	configDir := filepath.Join(tmpHome, ".config", "buttress")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "config.toml"),
		[]byte("[llm]\nprovider=\"x\"\nmodel=\"y\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := runGenerateCmd(context.Background(), "not-a-valid-ref")
	if err == nil {
		t.Fatal("expected error for invalid ref")
	}
	if !strings.Contains(err.Error(), "must start with @") {
		t.Errorf("error = %q, want substring 'must start with @'", err)
	}
}

func TestRunGenerateCmd_PackageNotInstalled(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	configDir := filepath.Join(tmpHome, ".config", "buttress")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "config.toml"),
		[]byte("[llm]\nprovider=\"x\"\nmodel=\"y\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Run from an empty cwd so no buttress.lock exists.
	cwd := t.TempDir()
	origWd, _ := os.Getwd()
	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(origWd) })

	err := runGenerateCmd(context.Background(), "@a/b")
	if err == nil {
		t.Fatal("expected error when package is not installed")
	}
	if !strings.Contains(err.Error(), "is not installed") {
		t.Errorf("error = %q, want substring 'is not installed'", err)
	}
	if !strings.Contains(err.Error(), "buttress spec add") {
		t.Errorf("error = %q, want substring 'buttress spec add' as guidance", err)
	}
}

// TestRunGenerateCmd_EndToEnd exercises the full flow with everything
// stubbed: real config on disk, real lock file, real spec dir on disk,
// and a fake LLM HTTP server. The success message is asserted.
func TestRunGenerateCmd_EndToEnd(t *testing.T) {
	// Not parallel: mutates HOME and cwd.
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	// Fake LLM server that returns a single complete reply.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"choices":[{"message":{"role":"assistant","content":"// generated\n"}}]}`)
	}))
	defer srv.Close()

	// Buttress config pointing at the fake LLM, with project language so
	// resolveGenOutput can determine the file extension.
	configDir := filepath.Join(tmpHome, ".config", "buttress")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cacheDir := filepath.Join(tmpHome, "cache")
	cfgBody := fmt.Sprintf(`
[llm]
provider = "openai"
base_url = %q
api_key = "test"
model = "test-model"

[project]
language = "go"

[registry]
cache_dir = %q
`, srv.URL, cacheDir)
	if err := os.WriteFile(filepath.Join(configDir, "config.toml"), []byte(cfgBody), 0o644); err != nil {
		t.Fatal(err)
	}

	// Project dir with a "fake-installed" package: lock entry + spec dir.
	projDir := t.TempDir()
	st, err := store.New(cacheDir, projDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.WriteLock(map[string]store.LockEntry{
		"@acme/widget": {Org: "acme", Pkg: "widget", Hash: "sha256:abc"},
	}); err != nil {
		t.Fatal(err)
	}
	specDir := filepath.Join(projDir, "buttress", "@acme", "widget")
	if err := os.MkdirAll(filepath.Join(specDir, "machine"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(specDir, "machine", "types.ts"),
		[]byte("export type W = number;\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	origWd, _ := os.Getwd()
	if err := os.Chdir(projDir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(origWd) })

	if err := runGenerateCmd(context.Background(), "@acme/widget"); err != nil {
		t.Fatalf("runGenerateCmd() error: %v", err)
	}

	// The output file is widget.go relative to projDir (because language=go).
	out := filepath.Join(projDir, "widget.go")
	body, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("output not written: %v", err)
	}
	if !strings.Contains(string(body), "// generated") {
		t.Errorf("output = %q, want contains '// generated'", body)
	}
}

func TestRunGenerateCmd_LLMFailureBubblesUp(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "model unavailable", http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	configDir := filepath.Join(tmpHome, ".config", "buttress")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cacheDir := filepath.Join(tmpHome, "cache")
	cfgBody := fmt.Sprintf(`
[llm]
provider = "openai"
base_url = %q
model = "m"

[project]
language = "go"

[registry]
cache_dir = %q
`, srv.URL, cacheDir)
	if err := os.WriteFile(filepath.Join(configDir, "config.toml"), []byte(cfgBody), 0o644); err != nil {
		t.Fatal(err)
	}

	projDir := t.TempDir()
	st, _ := store.New(cacheDir, projDir)
	_ = st.WriteLock(map[string]store.LockEntry{
		"@a/b": {Org: "a", Pkg: "b", Hash: "sha256:x"},
	})
	specDir := filepath.Join(projDir, "buttress", "@a", "b")
	_ = os.MkdirAll(filepath.Join(specDir, "machine"), 0o755)
	_ = os.WriteFile(filepath.Join(specDir, "machine", "t.ts"), []byte("type X={}"), 0o644)

	origWd, _ := os.Getwd()
	_ = os.Chdir(projDir)
	t.Cleanup(func() { _ = os.Chdir(origWd) })

	err := runGenerateCmd(context.Background(), "@a/b")
	if err == nil {
		t.Fatal("expected generation failure")
	}
	if !strings.Contains(err.Error(), "generation failed") {
		t.Errorf("error = %q, want substring 'generation failed'", err)
	}
}

func TestGenerateCmd_HelpExecutes(t *testing.T) {
	t.Parallel()
	c := newGenerateCmd()
	c.SetArgs([]string{"--help"})
	out := &bytes.Buffer{}
	c.SetOut(out)
	c.SetErr(out)
	if err := c.Execute(); err != nil {
		t.Errorf("--help should not error: %v", err)
	}
	if !strings.Contains(out.String(), "implementation") {
		t.Errorf("--help output missing description, got: %s", out.String())
	}
}
