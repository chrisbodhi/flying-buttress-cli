package cmd

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"

	"buttress/internal/scaffold"
	"buttress/internal/tui"
)

func newInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init [dir]",
		Short: "Scaffold a new spec package",
		Long: `Run an interactive wizard to scaffold a new spec package.

Examples:
  buttress spec init
  buttress spec init ./my-spec`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := "."
			if len(args) > 0 {
				dir = args[0]
			}
			return runInitSpec(dir)
		},
	}
}

func runInitSpec(dir string) error {
	s, err := runSpecWizard()
	if err != nil {
		return err
	}
	if err := scaffold.WriteSpec(*s, dir); err != nil {
		return err
	}
	fmt.Printf("%s scaffolded %s in %s\n",
		tui.StyleSuccess.Render("✓"),
		tui.StyleTitle.Render(s.PackageName()),
		tui.StyleDim.Render(dir))
	for _, line := range []string{
		"Next steps:",
		"  1. Fill in assertions in tests/",
		"  2. Define types in types/",
		"  3. Run `buttress spec hash .` to compute the spec hash",
		"  4. Run `buttress publish` to publish the first version",
	} {
		fmt.Fprintln(os.Stderr, tui.StyleDim.Render(line))
	}
	return nil
}

// runSpecWizard runs the interactive form that gathers a Spec from the user.
// It opens $EDITOR for the overview prose section.
func runSpecWizard() (*scaffold.Spec, error) {
	s := &scaffold.Spec{}
	var langChoice string
	var behaviorsRaw, edgeCasesRaw string

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().Title("Org").
				Description("e.g. chrisbodhi (no leading @)").
				Value(&s.Org).
				Validate(identValidator("org")),
			huh.NewInput().Title("Package name").
				Description("e.g. left-pad").
				Value(&s.Pkg).
				Validate(identValidator("pkg")),
			huh.NewInput().Title("One-line description").
				Description("max 80 chars; appears in buttress.toml").
				Value(&s.Description).
				Validate(descriptionValidator),
		),
		huh.NewGroup(
			huh.NewText().Title("Behaviors").
				Description("One per line. Each becomes a test stub.").
				Value(&behaviorsRaw),
			huh.NewText().Title("Edge cases").
				Description("One per line. Each becomes a test stub.").
				Value(&edgeCasesRaw),
		),
		huh.NewGroup(
			huh.NewSelect[string]().Title("Language").
				Options(
					huh.NewOption("TypeScript (vitest)", "typescript"),
					huh.NewOption("Go (testing)", "go"),
					huh.NewOption("Python (pytest)", "python"),
				).
				Value(&langChoice),
		),
	)

	if err := form.Run(); err != nil {
		return nil, err
	}

	s.Behaviors = splitBullets(behaviorsRaw)
	s.EdgeCases = splitBullets(edgeCasesRaw)
	s.Language = defaultLanguage(langChoice)

	overview, err := openEditorForOverview(*s)
	if err != nil {
		return nil, err
	}
	s.Overview = overview

	return s, nil
}

// defaultLanguage returns sensible per-language defaults for runtime and
// test framework. The values match what the wizard advertises in its
// option labels.
func defaultLanguage(name string) scaffold.Language {
	switch name {
	case "typescript":
		return scaffold.Language{
			Name:             "typescript",
			RuntimeMinimum:   "node 20",
			TestFramework:    "vitest",
			TestFrameworkVer: "3.0.0",
			TestRunner:       "vitest",
			TestRunnerVer:    "3.0.0",
		}
	case "go":
		return scaffold.Language{
			Name:           "go",
			RuntimeMinimum: "go 1.22",
			TestFramework:  "testing",
			TestRunner:     "go test",
		}
	case "python":
		return scaffold.Language{
			Name:             "python",
			RuntimeMinimum:   "python 3.11",
			TestFramework:    "pytest",
			TestFrameworkVer: "8.0.0",
			TestRunner:       "pytest",
			TestRunnerVer:    "8.0.0",
		}
	}
	return scaffold.Language{}
}

var identRE = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

func identValidator(field string) func(string) error {
	return func(v string) error {
		if v == "" {
			return errors.New(field + " is required")
		}
		if !identRE.MatchString(v) {
			return errors.New(field + " must match [a-z0-9][a-z0-9-]*")
		}
		return nil
	}
}

func descriptionValidator(v string) error {
	v = strings.TrimSpace(v)
	if v == "" {
		return errors.New("description is required")
	}
	if len(v) > 80 {
		return fmt.Errorf("description too long (%d chars, max 80)", len(v))
	}
	return nil
}

// splitBullets parses a multi-line text block into bullets. Leading/trailing
// whitespace and a single leading "-" or "*" marker are stripped from each
// line; empty lines are dropped.
func splitBullets(s string) []string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		line = strings.TrimPrefix(line, "-")
		line = strings.TrimPrefix(line, "*")
		line = strings.TrimSpace(line)
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

// openEditorForOverview launches $EDITOR (or vi) on a temp file pre-filled
// with a markdown template. Lines that begin with "<!--" are stripped before
// the result is returned, so the template's instructional comments don't
// leak into docs/overview.md.
func openEditorForOverview(s scaffold.Spec) (string, error) {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}

	f, err := os.CreateTemp("", "buttress-overview-*.md")
	if err != nil {
		return "", err
	}
	tmpPath := f.Name()
	defer os.Remove(tmpPath)

	tmpl := fmt.Sprintf(`# %s

%s

Describe the package's purpose, intended audience, and behavior here.

<!-- Lines starting with "<!--" are stripped before the file is written. -->
`, s.PackageName(), s.Description)
	if _, err := f.WriteString(tmpl); err != nil {
		_ = f.Close()
		return "", err
	}
	if err := f.Close(); err != nil {
		return "", err
	}

	cmd := exec.Command(editor, tmpPath)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("editor %s failed: %w", editor, err)
	}

	raw, err := os.ReadFile(tmpPath)
	if err != nil {
		return "", err
	}
	return stripHTMLComments(string(raw)), nil
}

// stripHTMLComments removes lines whose first non-whitespace characters are
// "<!--". It is intentionally line-based, not a real HTML parser.
func stripHTMLComments(s string) string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "<!--") {
			continue
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}
