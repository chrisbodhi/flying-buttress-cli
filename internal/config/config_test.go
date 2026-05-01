package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHasLLM(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		llm  LLMConfig
		want bool
	}{
		{"empty", LLMConfig{}, false},
		{"only provider", LLMConfig{Provider: "ollama"}, false},
		{"only model", LLMConfig{Model: "llama3"}, false},
		{"provider + model", LLMConfig{Provider: "ollama", Model: "llama3"}, true},
		{"full config", LLMConfig{Provider: "openai", BaseURL: "https://api", APIKey: "k", Model: "gpt-4"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cfg := &Config{LLM: tt.llm}
			if got := cfg.HasLLM(); got != tt.want {
				t.Errorf("HasLLM() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLoadFrom_Missing(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "does-not-exist.toml")

	cfg, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("LoadFrom missing file should not error: %v", err)
	}
	if cfg == nil {
		t.Fatal("LoadFrom returned nil config")
	}
	// Defaults shouldn't be applied when file is missing — Load only applies them
	// when reading was successful. Document current behavior.
	if cfg.Registry.CacheDir != "" {
		t.Errorf("missing config should not auto-fill CacheDir, got %q", cfg.Registry.CacheDir)
	}
}

func TestLoadFrom_Valid(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	body := `
[llm]
provider = "ollama"
base_url = "http://localhost:11434"
api_key = "secret"
model = "llama3"

[project]
language = "go"

[registry]
cache_dir = "/tmp/buttress-cache"
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("LoadFrom() error: %v", err)
	}

	if cfg.LLM.Provider != "ollama" {
		t.Errorf("LLM.Provider = %q, want %q", cfg.LLM.Provider, "ollama")
	}
	if cfg.LLM.BaseURL != "http://localhost:11434" {
		t.Errorf("LLM.BaseURL = %q", cfg.LLM.BaseURL)
	}
	if cfg.LLM.APIKey != "secret" {
		t.Errorf("LLM.APIKey = %q", cfg.LLM.APIKey)
	}
	if cfg.LLM.Model != "llama3" {
		t.Errorf("LLM.Model = %q", cfg.LLM.Model)
	}
	if cfg.Project.Language != "go" {
		t.Errorf("Project.Language = %q", cfg.Project.Language)
	}
	if cfg.Registry.CacheDir != "/tmp/buttress-cache" {
		t.Errorf("Registry.CacheDir = %q", cfg.Registry.CacheDir)
	}
	if !cfg.HasLLM() {
		t.Error("HasLLM() should be true for fully populated config")
	}
}

func TestLoadFrom_DefaultCacheDir(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte(`[project]
language = "go"
`), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("LoadFrom() error: %v", err)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, ".cache", "buttress")
	if cfg.Registry.CacheDir != want {
		t.Errorf("default Registry.CacheDir = %q, want %q", cfg.Registry.CacheDir, want)
	}
}

func TestLoadFrom_InvalidTOML(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte("not = = valid"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadFrom(path)
	if err == nil {
		t.Fatal("expected error for invalid TOML")
	}
	if !strings.Contains(err.Error(), "parsing") {
		t.Errorf("error = %q, want substring %q", err, "parsing")
	}
}

func TestLoadFrom_ReadError(t *testing.T) {
	t.Parallel()

	// A directory at the config path causes ReadFile to return a non-IsNotExist error.
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")
	if err := os.Mkdir(configPath, 0o755); err != nil {
		t.Fatal(err)
	}

	_, err := LoadFrom(configPath)
	if err == nil {
		t.Fatal("expected read error when config path is a directory")
	}
	if !strings.Contains(err.Error(), "reading config") {
		t.Errorf("error = %q, want substring %q", err, "reading config")
	}
}

func TestDefaultPath(t *testing.T) {
	t.Parallel()

	got, err := DefaultPath()
	if err != nil {
		t.Fatalf("DefaultPath() error: %v", err)
	}
	if !strings.HasSuffix(got, filepath.Join(".config", "buttress", "config.toml")) {
		t.Errorf("DefaultPath() = %q, want path ending in .config/buttress/config.toml", got)
	}
}

func TestLoad_FallsThroughToDefaultPath(t *testing.T) {
	// Not parallel: Load() reads from $HOME and we want to override it without
	// race-conditioning other tests.
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() unexpected error when default file is missing: %v", err)
	}
	if cfg == nil {
		t.Fatal("Load() returned nil config")
	}
}

func TestLoadFrom_DefaultCacheDir_HomeError(t *testing.T) {
	t.Setenv("HOME", "")

	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte(`[project]
language = "go"
`), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadFrom(path)
	if err == nil {
		t.Skip("UserHomeDir succeeded with HOME=\"\" on this platform; skipping")
	}
	if !strings.Contains(err.Error(), "home directory") {
		t.Errorf("error = %q, want substring 'home directory'", err)
	}
}

func TestDefaultPath_HomeError(t *testing.T) {
	t.Setenv("HOME", "")
	_, err := DefaultPath()
	if err == nil {
		t.Skip("UserHomeDir succeeded with HOME=\"\" on this platform; skipping")
	}
	if !strings.Contains(err.Error(), "home directory") {
		t.Errorf("error = %q, want substring 'home directory'", err)
	}
}

func TestLoad_HomeError(t *testing.T) {
	t.Setenv("HOME", "")
	_, err := Load()
	if err == nil {
		t.Skip("UserHomeDir succeeded with HOME=\"\" on this platform; skipping")
	}
}

func TestLoad_ParsesDefaultLocation(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	configDir := filepath.Join(tmpHome, ".config", "buttress")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "config.toml"),
		[]byte("[llm]\nprovider = \"x\"\nmodel = \"y\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if !cfg.HasLLM() {
		t.Error("HasLLM() = false, want true after loading from default path")
	}
}
