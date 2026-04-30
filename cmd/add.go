package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"buttress/internal/config"
	"buttress/internal/generate"
	"buttress/internal/github"
	"buttress/internal/localconfig"
	"buttress/internal/ref"
	"buttress/internal/store"
	"buttress/internal/tui"
)

func newAddCmd() *cobra.Command {
	var (
		versionsURL string
		doGenerate  bool
	)

	cmd := &cobra.Command{
		Use:   "add <@org/pkg[@hash]>",
		Short: "Download a spec package from the registry",
		Long: `Download a spec package and pin it in buttress.lock.

The optional hash is the content hash of the spec version (e.g. sha256:abc123…),
which also serves as the git tag in the spec repository. If omitted, an
interactive version picker is shown.

Examples:
  buttress add @chrisbodhi/left-pad
  buttress add @chrisbodhi/left-pad@sha256:abc123
  buttress add @chrisbodhi/left-pad --versions-url https://raw.githubusercontent.com/chrisbodhi/left-pad/HEAD/VERSIONS.txt
  buttress add @chrisbodhi/left-pad --generate`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAdd(cmd.Context(), args[0], versionsURL, doGenerate)
		},
	}

	cmd.Flags().StringVar(&versionsURL, "versions-url", "",
		"URL of the plain-text version list (overrides the conventional repo path)")
	cmd.Flags().BoolVar(&doGenerate, "generate", false,
		"generate implementation after downloading the spec")

	return cmd
}

func runAdd(ctx context.Context, rawRef, versionsURL string, doGenerate bool) error {
	// --- 0. Load config (fail early for --generate without LLM) ---
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	if doGenerate && !cfg.HasLLM() {
		return errors.New("--generate requires LLM credentials in ~/.config/buttress/config.toml")
	}

	// --- 1. Parse the package reference ---
	pkg, err := ref.Parse(rawRef)
	if err != nil {
		return err
	}

	ghClient := github.New()

	// --- 2. Resolve the content hash ---
	hash := pkg.GitRef // GitRef holds the hash when provided inline (@org/pkg@sha256:...)
	if hash == "" {
		// No hash provided — fetch the version list and let the user pick.
		url := versionsURL
		if url == "" {
			url = github.VersionsURL(pkg)
		}

		fmt.Fprintf(os.Stderr, "%s\n", tui.StyleDim.Render(fmt.Sprintf("fetching version list from %s…", url)))

		vs, err := ghClient.ListVersionsFromURL(ctx, url)
		if err != nil {
			return fmt.Errorf("listing versions for %s: %w", pkg.Name(), err)
		}

		chosen, err := tui.RunPicker(pkg.Name(), vs)
		if err != nil {
			return err
		}
		if chosen == nil {
			return errors.New("no version selected")
		}
		hash = chosen.Hash
	}

	// --- 3. Check if already installed ---
	st, err := store.New(cfg.Registry.CacheDir, "")
	if err != nil {
		return err
	}

	lock, err := st.ReadLock()
	if err != nil {
		return err
	}

	key := store.LockKey(pkg.Org, pkg.Pkg)
	if existing, ok := lock[key]; ok && existing.Hash == hash {
		fmt.Printf("%s %s is already installed at %s\n",
			tui.StyleSuccess.Render("✓"),
			tui.StyleTitle.Render(pkg.Name()),
			tui.StyleDim.Render(tui.ShortHash(hash)))
		return nil
	}
	if existing, ok := lock[key]; ok {
		fmt.Printf("%s %s is currently at %s — replacing with %s\n",
			tui.StyleDim.Render("~"),
			tui.StyleTitle.Render(pkg.Name()),
			tui.StyleDim.Render(tui.ShortHash(existing.Hash)),
			tui.StyleSHA.Render(tui.ShortHash(hash)))
	}

	// --- 4. Download and verify the spec ---
	fmt.Fprintf(os.Stderr, "%s\n",
		tui.StyleDim.Render(fmt.Sprintf("downloading %s@%s…", pkg.Name(), tui.ShortHash(hash))))

	tmpDir := filepath.Join(os.TempDir(), fmt.Sprintf("buttress-%s-%s-%s", pkg.Org, pkg.Pkg, sanitize(hash)))
	archive, err := ghClient.FetchSpec(ctx, pkg, hash, tmpDir)
	if err != nil {
		return err
	}

	// --- 5. Commit to store ---
	_, err = st.Commit(pkg.Org, pkg.Pkg, archive.ContentHash, archive.Dir)
	if err != nil {
		return err
	}

	// --- 6. Update lock file ---
	lock[key] = store.LockEntry{
		Org:         pkg.Org,
		Pkg:         pkg.Pkg,
		Hash:        hash,
		InstalledAt: time.Now().UTC(),
	}
	if err := st.WriteLock(lock); err != nil {
		return err
	}

	fmt.Printf("%s added %s (%s)\n",
		tui.StyleSuccess.Render("✓"),
		tui.StyleTitle.Render(pkg.Name()),
		tui.StyleDim.Render(tui.ShortHash(hash)))

	// --- 7. Optionally generate ---
	if doGenerate {
		wd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("determining working directory: %w", err)
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
		}
		if err := gen.Generate(ctx, req); err != nil {
			return fmt.Errorf("generation failed: %w", err)
		}

		fmt.Printf("%s generated %s → %s\n",
			tui.StyleSuccess.Render("✓"),
			tui.StyleTitle.Render(pkg.Name()),
			tui.StyleDim.Render(outPath))
	}

	return nil
}

// resolveGenOutput returns the output path and language for generation,
// consulting .buttress.local.toml if present.
func resolveGenOutput(projectDir string, pkg ref.PackageRef, cfg *config.Config) (outPath, lang string, err error) {
	localCfg, err := localconfig.Load(projectDir)
	if err != nil {
		return "", "", err
	}

	lang = cfg.Project.Language
	key := "@" + pkg.Org + "/" + pkg.Pkg
	if override, ok := localCfg.Overrides[key]; ok {
		if override.Language != "" {
			lang = override.Language
		}
		if override.Output != "" {
			outPath = filepath.Join(projectDir, override.Output)
			return outPath, lang, nil
		}
	}

	if lang == "" {
		return "", "", fmt.Errorf("no language configured: set [project].language in ~/.config/buttress/config.toml or add a language override in .buttress.local.toml")
	}
	outPath = filepath.Join(projectDir, pkg.Pkg+"."+langExt(lang))
	return outPath, lang, nil
}

// sanitize replaces characters that are unsafe in a filesystem path.
func sanitize(s string) string {
	return strings.NewReplacer(":", "-", "/", "-").Replace(s)
}

// langExt returns the conventional file extension for a language name.
func langExt(lang string) string {
	switch strings.ToLower(lang) {
	case "go":
		return "go"
	case "typescript", "ts":
		return "ts"
	case "javascript", "js":
		return "js"
	case "python", "py":
		return "py"
	case "rust", "rs":
		return "rs"
	case "java":
		return "java"
	case "ruby", "rb":
		return "rb"
	case "c":
		return "c"
	case "cpp", "c++":
		return "cpp"
	case "csharp", "c#":
		return "cs"
	case "kotlin", "kt":
		return "kt"
	case "swift":
		return "swift"
	default:
		return lang
	}
}
