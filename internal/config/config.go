// Package config reads Flying Buttress configuration from
// ~/.config/buttress/config.toml.
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// Config holds all buttress configuration.
type Config struct {
	LLM      LLMConfig      `toml:"llm"`
	Project  ProjectConfig  `toml:"project"`
	Registry RegistryConfig `toml:"registry"`
}

// LLMConfig holds credentials and settings for the code-generation LLM.
type LLMConfig struct {
	Provider string `toml:"provider"` // "ollama", "lmstudio", "openai", etc.
	BaseURL  string `toml:"base_url"` // e.g. http://localhost:11434
	APIKey   string `toml:"api_key"`
	Model    string `toml:"model"`
}

// ProjectConfig holds per-project generation settings.
type ProjectConfig struct {
	Language string `toml:"language"` // e.g. "go", "typescript", "python"
}

// RegistryConfig holds optional overrides for registry behaviour.
type RegistryConfig struct {
	CacheDir string `toml:"cache_dir"` // default: ~/.cache/buttress
}

// DefaultPath returns the default path to the config file.
func DefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}
	return filepath.Join(home, ".config", "buttress", "config.toml"), nil
}

// Load reads the config from the default path.
// It is not an error if the file does not exist; an empty Config is returned.
func Load() (*Config, error) {
	path, err := DefaultPath()
	if err != nil {
		return nil, err
	}
	return LoadFrom(path)
}

// LoadFrom reads the config from the given path.
func LoadFrom(path string) (*Config, error) {
	cfg := &Config{}

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return cfg, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading config %s: %w", path, err)
	}

	if err := toml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing config %s: %w", path, err)
	}

	// Apply defaults.
	if cfg.Registry.CacheDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("cannot determine home directory: %w", err)
		}
		cfg.Registry.CacheDir = filepath.Join(home, ".cache", "buttress")
	}

	return cfg, nil
}

// HasLLM reports whether the config contains enough information to call an LLM.
func (c *Config) HasLLM() bool {
	return c.LLM.Provider != "" && c.LLM.Model != ""
}
