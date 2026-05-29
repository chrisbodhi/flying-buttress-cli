package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"buttress/internal/config"
	"buttress/internal/generate"
	"buttress/internal/ref"
	"buttress/internal/store"
	"buttress/internal/tui"
)

func newGenerateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "generate <@org/pkg>",
		Short: "Generate an implementation from an installed spec",
		Long: `Generate an implementation source file from a spec package that has
already been installed with 'buttress spec add'.

Requires LLM credentials in ~/.config/buttress/config.toml.
Output path and language can be overridden per-package in .buttress.local.toml.

Examples:
  buttress spec generate @chrisbodhi/left-pad`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGenerateCmd(cmd.Context(), args[0])
		},
	}
	return cmd
}

func runGenerateCmd(ctx context.Context, rawRef string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	if !cfg.HasLLM() {
		return errors.New("generate requires LLM credentials in ~/.config/buttress/config.toml")
	}

	pkg, err := ref.ParsePackageRef(rawRef)
	if err != nil {
		return err
	}

	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("determining working directory: %w", err)
	}

	// Confirm the package is installed.
	st, err := store.New(cfg.Registry.CacheDir, wd)
	if err != nil {
		return err
	}
	lock, err := st.ReadLock()
	if err != nil {
		return err
	}
	entry, ok := lock[store.LockKey(pkg.Org, pkg.Pkg)]
	if !ok {
		return fmt.Errorf("%s is not installed — run 'buttress spec add %s' first", pkg.Name(), pkg.Name())
	}

	// The spec lives in ./buttress/@org/pkg/ under the project root.
	specDir := fmt.Sprintf("%s/buttress/@%s/%s", wd, pkg.Org, pkg.Pkg)
	archive := &generate.SpecArchive{
		Dir:         specDir,
		ContentHash: entry.Hash,
	}

	outPath, lang, err := resolveGenOutput(wd, pkg, cfg)
	if err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "%s\n",
		tui.StyleDim.Render(fmt.Sprintf("generating %s implementation → %s…", lang, outPath)))

	gen := generate.NewLLMGenerator(generate.LLMConfig{
		Provider: cfg.LLM.Provider,
		BaseURL:  cfg.LLM.BaseURL,
		APIKey:   cfg.LLM.APIKey,
		Model:    cfg.LLM.Model,
	})
	req := generate.Request{
		Spec:        archive,
		Language:    lang,
		OutputPath:  outPath,
		PackageName: pkg.Name(),
		ProjectDir:  wd,
		Progress: func(msg string) {
			if strings.Contains(msg, "failed") {
				fmt.Fprintf(os.Stderr, "%s %s\n", tui.StyleError.Render("✗"), tui.StyleDim.Render(msg))
			} else {
				fmt.Fprintf(os.Stderr, "%s\n", tui.StyleDim.Render(msg))
			}
		},
	}
	if err := gen.Generate(ctx, req); err != nil {
		return fmt.Errorf("generation failed: %w", err)
	}

	fmt.Printf("%s generated %s → %s\n",
		tui.StyleSuccess.Render("✓"),
		tui.StyleTitle.Render(pkg.Name()),
		tui.StyleDim.Render(outPath))
	return nil
}
