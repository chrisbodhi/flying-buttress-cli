package cmd

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/charmbracelet/huh"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	"buttress/internal/config"
)

type configDeps struct {
	stdin      io.Reader
	stdout     io.Writer
	accessible bool // true = line-by-line mode (non-TTY / tests)
	configPath func() (string, error)
}

func defaultConfigDeps() *configDeps {
	return &configDeps{
		stdin:      os.Stdin,
		stdout:     os.Stdout,
		accessible: !isatty.IsTerminal(os.Stdin.Fd()),
		configPath: config.DefaultPath,
	}
}

func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "config",
		Short:        "Configure your LLM connection",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConfig(defaultConfigDeps())
		},
	}

	set := &cobra.Command{
		Use:          "set <key> <value>",
		Short:        "Set a single config value (e.g. llm.api_key sk-123)",
		Args:         cobra.ExactArgs(2),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConfigSet(defaultConfigDeps(), args[0], args[1])
		},
	}

	get := &cobra.Command{
		Use:          "get <key>",
		Short:        "Print a single config value (e.g. llm.model)",
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConfigGet(defaultConfigDeps(), args[0])
		},
	}

	cmd.AddCommand(set, get)
	return cmd
}

// runConfig runs the interactive wizard, pre-populating from any existing config.
func runConfig(deps *configDeps) error {
	path, err := deps.configPath()
	if err != nil {
		return err
	}

	// Pre-populate from existing config; ignore errors (missing = empty defaults).
	existing, _ := config.LoadFrom(path)

	kind := "local"
	if existing.LLM.APIKey != "" {
		kind = "frontier"
	}
	provider := existing.LLM.Provider
	baseURL := existing.LLM.BaseURL
	apiKey := existing.LLM.APIKey
	model := existing.LLM.Model
	lang := existing.Project.Language

	newForm := func(groups ...*huh.Group) *huh.Form {
		return huh.NewForm(groups...).
			WithAccessible(deps.accessible).
			WithInput(deps.stdin).
			WithOutput(deps.stdout)
	}

	// Step 1: local or frontier?
	if err := newForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("LLM type").
				Options(
					huh.NewOption("Local (ollama, lmstudio, …)", "local"),
					huh.NewOption("Frontier (openai, anthropic, …)", "frontier"),
				).
				Value(&kind),
		),
	).Run(); err != nil {
		return err
	}

	// Step 2: type-specific credential field.
	var credField huh.Field
	if kind == "local" {
		apiKey = ""
		credField = huh.NewInput().Title("Base URL").Value(&baseURL)
	} else {
		baseURL = ""
		credField = huh.NewInput().Title("API key").Value(&apiKey)
	}

	if err := newForm(
		huh.NewGroup(
			huh.NewInput().Title("Provider").Value(&provider),
			credField,
			huh.NewInput().Title("Model").Value(&model),
		),
	).Run(); err != nil {
		return err
	}

	// Step 3: target language.
	if err := newForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Target language").
				Suggestions([]string{"go", "typescript", "python", "rust"}).
				Value(&lang),
		),
	).Run(); err != nil {
		return err
	}

	cfg := &config.Config{
		LLM: config.LLMConfig{
			Provider: provider,
			BaseURL:  baseURL,
			APIKey:   apiKey,
			Model:    model,
		},
		Project: config.ProjectConfig{Language: lang},
	}
	if err := writeConfig(path, cfg); err != nil {
		return err
	}

	fmt.Fprintf(deps.stdout, "\nSaved config to %s\n", path)
	return nil
}

// runConfigSet updates a single dotted key in the config file.
func runConfigSet(deps *configDeps, key, value string) error {
	path, err := deps.configPath()
	if err != nil {
		return err
	}

	cfg, err := config.LoadFrom(path)
	if err != nil {
		return err
	}

	switch key {
	case "llm.provider":
		cfg.LLM.Provider = value
	case "llm.base_url":
		cfg.LLM.BaseURL = value
	case "llm.api_key":
		cfg.LLM.APIKey = value
	case "llm.model":
		cfg.LLM.Model = value
	case "project.language":
		cfg.Project.Language = value
	case "registry.cache_dir":
		cfg.Registry.CacheDir = value
	default:
		return fmt.Errorf("unknown config key %q\nvalid keys: llm.provider, llm.base_url, llm.api_key, llm.model, project.language, registry.cache_dir", key)
	}

	if err := writeConfig(path, cfg); err != nil {
		return err
	}

	fmt.Fprintf(deps.stdout, "Set %s = %q in %s\n", key, value, path)
	return nil
}

// runConfigGet prints a single dotted key from the config file.
func runConfigGet(deps *configDeps, key string) error {
	path, err := deps.configPath()
	if err != nil {
		return err
	}

	cfg, err := config.LoadFrom(path)
	if err != nil {
		return err
	}

	var value string
	switch key {
	case "llm.provider":
		value = cfg.LLM.Provider
	case "llm.base_url":
		value = cfg.LLM.BaseURL
	case "llm.api_key":
		value = cfg.LLM.APIKey
	case "llm.model":
		value = cfg.LLM.Model
	case "project.language":
		value = cfg.Project.Language
	case "registry.cache_dir":
		value = cfg.Registry.CacheDir
	default:
		return fmt.Errorf("unknown config key %q\nvalid keys: llm.provider, llm.base_url, llm.api_key, llm.model, project.language, registry.cache_dir", key)
	}

	fmt.Fprintln(deps.stdout, value)
	return nil
}

func writeConfig(path string, cfg *config.Config) error {
	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(cfg); err != nil {
		return fmt.Errorf("encoding config: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating config dir: %w", err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}
	return nil
}
