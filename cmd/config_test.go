package cmd

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"buttress/internal/config"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// newConfigFixture builds a configDeps wired for testing.
//
// stdinLines are fed to the wizard one line at a time via io.Pipe so that
// each huh field's internal bufio.Scanner gets exactly one line before the
// next field is reached (strings.NewReader would be buffered in full by the
// first scanner, starving subsequent fields).
func newConfigFixture(t *testing.T, stdinLines []string) (deps *configDeps, savedPath *string, stdout *bytes.Buffer) {
	t.Helper()

	home := t.TempDir()
	path := filepath.Join(home, ".config", "buttress", "config.toml")

	out := &bytes.Buffer{}
	pr, pw := io.Pipe()

	go func() {
		defer pw.Close()
		for _, line := range stdinLines {
			if _, err := fmt.Fprintln(pw, line); err != nil {
				return // test finished early, pipe closed
			}
		}
	}()
	t.Cleanup(func() { pr.Close() }) // unblocks the goroutine on early exit

	deps = &configDeps{
		stdin:      pr,
		stdout:     out,
		accessible: true,
		configPath: func() (string, error) { return path, nil },
	}
	savedPath = &path
	stdout = out
	return
}

// seedConfig writes a config file at deps' path location.
func seedConfig(t *testing.T, savedPath string, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(savedPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(savedPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// ---------------------------------------------------------------------------
// Command surface
// ---------------------------------------------------------------------------

func TestConfigCmd_Use(t *testing.T) {
	t.Parallel()
	cmd := newConfigCmd()
	if cmd.Use != "config" {
		t.Errorf("Use = %q, want %q", cmd.Use, "config")
	}
}

func TestConfigCmd_HasSetSubcommand(t *testing.T) {
	t.Parallel()
	cmd := newConfigCmd()
	for _, sub := range cmd.Commands() {
		if sub.Use == "set <key> <value>" {
			return
		}
	}
	t.Error("config command has no 'set' subcommand")
}

// ---------------------------------------------------------------------------
// Wizard — local LLM
// ---------------------------------------------------------------------------

// In accessible mode the Select field prints numbered options and expects a
// number: 1 = local, 2 = frontier.

func TestRunConfig_LocalLLM_WritesBaseURLNoAPIKey(t *testing.T) {
	t.Parallel()

	deps, savedPath, _ := newConfigFixture(t, []string{
		"1",                       // local
		"ollama",                  // provider
		"http://localhost:11434",  // base_url
		"llama3",                  // model
		"go",                      // language
	})

	if err := runConfig(deps); err != nil {
		t.Fatalf("runConfig() error: %v", err)
	}

	cfg, err := config.LoadFrom(*savedPath)
	if err != nil {
		t.Fatalf("LoadFrom() error: %v", err)
	}
	if cfg.LLM.Provider != "ollama" {
		t.Errorf("Provider = %q, want %q", cfg.LLM.Provider, "ollama")
	}
	if cfg.LLM.BaseURL != "http://localhost:11434" {
		t.Errorf("BaseURL = %q, want %q", cfg.LLM.BaseURL, "http://localhost:11434")
	}
	if cfg.LLM.Model != "llama3" {
		t.Errorf("Model = %q, want %q", cfg.LLM.Model, "llama3")
	}
	if cfg.LLM.APIKey != "" {
		t.Errorf("APIKey = %q, want empty for local LLM", cfg.LLM.APIKey)
	}
	if cfg.Project.Language != "go" {
		t.Errorf("Language = %q, want %q", cfg.Project.Language, "go")
	}
}

func TestRunConfig_LocalLLM_CreatesConfigDir(t *testing.T) {
	t.Parallel()

	deps, savedPath, _ := newConfigFixture(t, []string{
		"1", "lmstudio", "http://localhost:1234", "mistral", "typescript",
	})

	if err := runConfig(deps); err != nil {
		t.Fatalf("runConfig() error: %v", err)
	}
	if _, err := os.Stat(*savedPath); err != nil {
		t.Errorf("config file not created at %s: %v", *savedPath, err)
	}
}

// ---------------------------------------------------------------------------
// Wizard — frontier model
// ---------------------------------------------------------------------------

func TestRunConfig_Frontier_WritesAPIKeyNoBaseURL(t *testing.T) {
	t.Parallel()

	deps, savedPath, _ := newConfigFixture(t, []string{
		"2",        // frontier
		"openai",   // provider
		"sk-abc123", // api_key
		"gpt-4o",   // model
		"python",   // language
	})

	if err := runConfig(deps); err != nil {
		t.Fatalf("runConfig() error: %v", err)
	}

	cfg, err := config.LoadFrom(*savedPath)
	if err != nil {
		t.Fatalf("LoadFrom() error: %v", err)
	}
	if cfg.LLM.Provider != "openai" {
		t.Errorf("Provider = %q, want %q", cfg.LLM.Provider, "openai")
	}
	if cfg.LLM.APIKey != "sk-abc123" {
		t.Errorf("APIKey = %q, want %q", cfg.LLM.APIKey, "sk-abc123")
	}
	if cfg.LLM.Model != "gpt-4o" {
		t.Errorf("Model = %q, want %q", cfg.LLM.Model, "gpt-4o")
	}
	if cfg.LLM.BaseURL != "" {
		t.Errorf("BaseURL = %q, want empty for frontier model", cfg.LLM.BaseURL)
	}
	if cfg.Project.Language != "python" {
		t.Errorf("Language = %q, want %q", cfg.Project.Language, "python")
	}
}

// ---------------------------------------------------------------------------
// Wizard — existing config
// ---------------------------------------------------------------------------

func TestRunConfig_ExistingConfig_PreservesValuesOnEmptyInput(t *testing.T) {
	t.Parallel()

	deps, savedPath, _ := newConfigFixture(t, []string{
		"",  // keep existing type (local = option 1, default)
		"",  // keep provider
		"",  // keep base_url
		"",  // keep model
		"",  // keep language
	})
	seedConfig(t, *savedPath, `
[llm]
provider = "ollama"
base_url = "http://localhost:11434"
model    = "llama3"

[project]
language = "go"
`)

	if err := runConfig(deps); err != nil {
		t.Fatalf("runConfig() error: %v", err)
	}

	cfg, err := config.LoadFrom(*savedPath)
	if err != nil {
		t.Fatalf("LoadFrom() error: %v", err)
	}
	if cfg.LLM.Provider != "ollama" {
		t.Errorf("Provider = %q, want preserved %q", cfg.LLM.Provider, "ollama")
	}
	if cfg.LLM.BaseURL != "http://localhost:11434" {
		t.Errorf("BaseURL = %q, want preserved", cfg.LLM.BaseURL)
	}
	if cfg.LLM.Model != "llama3" {
		t.Errorf("Model = %q, want preserved", cfg.LLM.Model)
	}
	if cfg.Project.Language != "go" {
		t.Errorf("Language = %q, want preserved", cfg.Project.Language)
	}
}

// ---------------------------------------------------------------------------
// Wizard — output
// ---------------------------------------------------------------------------

func TestRunConfig_PrintsSuccessMessage(t *testing.T) {
	t.Parallel()

	deps, _, stdout := newConfigFixture(t, []string{
		"1", "ollama", "http://localhost:11434", "llama3", "go",
	})

	if err := runConfig(deps); err != nil {
		t.Fatalf("runConfig() error: %v", err)
	}
	if !strings.Contains(stdout.String(), "config.toml") {
		t.Errorf("stdout %q should mention config.toml", stdout.String())
	}
}

// ---------------------------------------------------------------------------
// Wizard — switching between local and frontier
// ---------------------------------------------------------------------------

func TestRunConfig_BothModels_LocalThenFrontier(t *testing.T) {
	t.Parallel()

	// First run: local LLM.
	deps, savedPath, _ := newConfigFixture(t, []string{
		"1", "ollama", "http://localhost:11434", "llama3", "go",
	})
	if err := runConfig(deps); err != nil {
		t.Fatalf("first runConfig() error: %v", err)
	}

	// Second run: switch to frontier — pre-populated with existing values but
	// user selects option 2 and types new values.
	deps2, _, _ := newConfigFixture(t, []string{
		"2",      // frontier
		"openai", // provider (change from ollama)
		"sk-xyz", // api_key (new)
		"gpt-4o", // model (change from llama3)
		"go",     // language (keep)
	})
	deps2.configPath = func() (string, error) { return *savedPath, nil }

	if err := runConfig(deps2); err != nil {
		t.Fatalf("second runConfig() error: %v", err)
	}

	cfg, err := config.LoadFrom(*savedPath)
	if err != nil {
		t.Fatalf("LoadFrom() error: %v", err)
	}
	if cfg.LLM.Provider != "openai" {
		t.Errorf("Provider = %q, want %q", cfg.LLM.Provider, "openai")
	}
	if cfg.LLM.APIKey != "sk-xyz" {
		t.Errorf("APIKey = %q, want %q", cfg.LLM.APIKey, "sk-xyz")
	}
	if cfg.LLM.BaseURL != "" {
		t.Errorf("BaseURL = %q, want empty after switching to frontier", cfg.LLM.BaseURL)
	}
}

func TestRunConfig_BothModels_FrontierThenLocal(t *testing.T) {
	t.Parallel()

	deps, savedPath, _ := newConfigFixture(t, []string{
		"2", "anthropic", "sk-ant-abc", "claude-opus-4-7", "go",
	})
	if err := runConfig(deps); err != nil {
		t.Fatalf("first runConfig() error: %v", err)
	}

	deps2, _, _ := newConfigFixture(t, []string{
		"1",                      // local
		"lmstudio",               // provider
		"http://localhost:1234",  // base_url
		"mistral",                // model
		"go",                     // language
	})
	deps2.configPath = func() (string, error) { return *savedPath, nil }

	if err := runConfig(deps2); err != nil {
		t.Fatalf("second runConfig() error: %v", err)
	}

	cfg, err := config.LoadFrom(*savedPath)
	if err != nil {
		t.Fatalf("LoadFrom() error: %v", err)
	}
	if cfg.LLM.Provider != "lmstudio" {
		t.Errorf("Provider = %q, want %q", cfg.LLM.Provider, "lmstudio")
	}
	if cfg.LLM.BaseURL != "http://localhost:1234" {
		t.Errorf("BaseURL = %q, want %q", cfg.LLM.BaseURL, "http://localhost:1234")
	}
	if cfg.LLM.APIKey != "" {
		t.Errorf("APIKey = %q, want empty after switching to local", cfg.LLM.APIKey)
	}
}

// ---------------------------------------------------------------------------
// config set
// ---------------------------------------------------------------------------

func TestRunConfigSet_UpdatesAPIKey(t *testing.T) {
	t.Parallel()

	deps, savedPath, _ := newConfigFixture(t, nil)
	seedConfig(t, *savedPath, "[llm]\nprovider = \"openai\"\nmodel = \"gpt-4o\"\n")

	if err := runConfigSet(deps, "llm.api_key", "sk-new"); err != nil {
		t.Fatalf("runConfigSet() error: %v", err)
	}

	cfg, err := config.LoadFrom(*savedPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.LLM.APIKey != "sk-new" {
		t.Errorf("APIKey = %q, want %q", cfg.LLM.APIKey, "sk-new")
	}
	// Other fields untouched.
	if cfg.LLM.Provider != "openai" {
		t.Errorf("Provider = %q, want unchanged %q", cfg.LLM.Provider, "openai")
	}
}

func TestRunConfigSet_UpdatesProvider(t *testing.T) {
	t.Parallel()

	deps, savedPath, _ := newConfigFixture(t, nil)
	seedConfig(t, *savedPath, "[llm]\nprovider = \"openai\"\nmodel = \"gpt-4o\"\n")

	if err := runConfigSet(deps, "llm.provider", "anthropic"); err != nil {
		t.Fatalf("runConfigSet() error: %v", err)
	}

	cfg, _ := config.LoadFrom(*savedPath)
	if cfg.LLM.Provider != "anthropic" {
		t.Errorf("Provider = %q, want %q", cfg.LLM.Provider, "anthropic")
	}
}

func TestRunConfigSet_CreatesConfigIfMissing(t *testing.T) {
	t.Parallel()

	deps, savedPath, _ := newConfigFixture(t, nil)
	// No seedConfig — file doesn't exist yet.

	if err := runConfigSet(deps, "llm.model", "llama3"); err != nil {
		t.Fatalf("runConfigSet() error: %v", err)
	}

	cfg, err := config.LoadFrom(*savedPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.LLM.Model != "llama3" {
		t.Errorf("Model = %q, want %q", cfg.LLM.Model, "llama3")
	}
}

func TestRunConfigSet_UnknownKeyErrors(t *testing.T) {
	t.Parallel()

	deps, _, _ := newConfigFixture(t, nil)
	err := runConfigSet(deps, "llm.nonexistent", "value")
	if err == nil {
		t.Fatal("expected error for unknown key")
	}
	if !strings.Contains(err.Error(), "unknown config key") {
		t.Errorf("error = %q, want 'unknown config key'", err)
	}
}

func TestRunConfigSet_PrintsConfirmation(t *testing.T) {
	t.Parallel()

	deps, savedPath, stdout := newConfigFixture(t, nil)
	seedConfig(t, *savedPath, "")

	if err := runConfigSet(deps, "llm.model", "gpt-4o"); err != nil {
		t.Fatalf("runConfigSet() error: %v", err)
	}
	if !strings.Contains(stdout.String(), "llm.model") {
		t.Errorf("stdout %q should mention the key", stdout.String())
	}
}

// ---------------------------------------------------------------------------
// config get
// ---------------------------------------------------------------------------

func TestConfigCmd_HasGetSubcommand(t *testing.T) {
	t.Parallel()
	cmd := newConfigCmd()
	for _, sub := range cmd.Commands() {
		if sub.Use == "get <key>" {
			return
		}
	}
	t.Error("config command has no 'get' subcommand")
}

func TestRunConfigGet_ReturnsValue(t *testing.T) {
	t.Parallel()

	deps, savedPath, stdout := newConfigFixture(t, nil)
	seedConfig(t, *savedPath, "[llm]\nprovider = \"openai\"\nmodel = \"gpt-4o\"\napi_key = \"sk-abc\"\n")

	if err := runConfigGet(deps, "llm.api_key"); err != nil {
		t.Fatalf("runConfigGet() error: %v", err)
	}
	if strings.TrimSpace(stdout.String()) != "sk-abc" {
		t.Errorf("stdout = %q, want %q", strings.TrimSpace(stdout.String()), "sk-abc")
	}
}

func TestRunConfigGet_AllKeys(t *testing.T) {
	t.Parallel()

	body := `
[llm]
provider = "ollama"
base_url = "http://localhost:11434"
api_key  = "sk-x"
model    = "llama3"

[project]
language = "go"

[registry]
cache_dir = "/tmp/buttress"
`
	tests := []struct {
		key  string
		want string
	}{
		{"llm.provider", "ollama"},
		{"llm.base_url", "http://localhost:11434"},
		{"llm.api_key", "sk-x"},
		{"llm.model", "llama3"},
		{"project.language", "go"},
		{"registry.cache_dir", "/tmp/buttress"},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			t.Parallel()
			deps, savedPath, stdout := newConfigFixture(t, nil)
			seedConfig(t, *savedPath, body)

			if err := runConfigGet(deps, tt.key); err != nil {
				t.Fatalf("runConfigGet(%q) error: %v", tt.key, err)
			}
			if strings.TrimSpace(stdout.String()) != tt.want {
				t.Errorf("stdout = %q, want %q", strings.TrimSpace(stdout.String()), tt.want)
			}
		})
	}
}

func TestRunConfigGet_UnknownKeyErrors(t *testing.T) {
	t.Parallel()

	deps, savedPath, _ := newConfigFixture(t, nil)
	seedConfig(t, *savedPath, "")

	err := runConfigGet(deps, "llm.nonexistent")
	if err == nil {
		t.Fatal("expected error for unknown key")
	}
	if !strings.Contains(err.Error(), "unknown config key") {
		t.Errorf("error = %q, want 'unknown config key'", err)
	}
}

func TestRunConfigGet_MissingConfigReturnsEmpty(t *testing.T) {
	t.Parallel()

	deps, _, stdout := newConfigFixture(t, nil)
	// No seedConfig — file doesn't exist.

	if err := runConfigGet(deps, "llm.model"); err != nil {
		t.Fatalf("runConfigGet() error on missing file: %v", err)
	}
	if stdout.String() != "\n" {
		t.Errorf("stdout = %q, want empty line for unset value", stdout.String())
	}
}
