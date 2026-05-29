// Package scaffold generates Flying Buttress spec package layouts from a
// high-level Spec description.
//
// The scaffolder is split in two halves:
//
//   - Plan: pure, takes a Spec, returns []File. Used by tests and dry-runs.
//   - Write: applies a []File to a directory.
//
// Renderers (renderButtressTOML, renderTSTests, …) are package-private and
// each returns []byte for one file. They are pure and individually testable.
package scaffold

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// Spec describes a spec package to scaffold.
type Spec struct {
	Org         string
	Pkg         string
	Description string
	Overview    string   // free-form markdown body for docs/overview.md
	Behaviors   []string // one bullet per behavior; becomes a test stub
	EdgeCases   []string // one bullet per edge case; becomes a test stub
	Language    Language
}

// Language captures the per-language testing/runtime metadata that ends up
// in buttress.toml and drives test/type stub generation.
type Language struct {
	Name             string // "typescript", "go", "python"
	RuntimeMinimum   string // e.g. "node 20"
	TestFramework    string // e.g. "vitest"
	TestFrameworkVer string // semver string; may be empty
	TestRunner       string // e.g. "vitest"
	TestRunnerVer    string // semver string; may be empty
}

// File is one generated artifact, addressed by path relative to the spec dir.
type File struct {
	Path    string
	Content []byte
}

// PackageName returns "@org/pkg".
func (s Spec) PackageName() string { return "@" + s.Org + "/" + s.Pkg }

// IdentSafePkg returns Pkg with hyphens replaced by underscores so it can be
// used as a Go package name or a Python module name.
func (s Spec) IdentSafePkg() string { return strings.ReplaceAll(s.Pkg, "-", "_") }

var validIdent = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// SupportedLanguages enumerates the language names Plan/Write accept.
var SupportedLanguages = []string{"typescript", "go", "python"}

func isSupportedLang(name string) bool {
	for _, l := range SupportedLanguages {
		if l == name {
			return true
		}
	}
	return false
}

// Validate reports configuration problems with s.
func (s Spec) Validate() error {
	var errs []string
	if !validIdent.MatchString(s.Org) {
		errs = append(errs, "org must match [a-z0-9][a-z0-9-]*")
	}
	if !validIdent.MatchString(s.Pkg) {
		errs = append(errs, "pkg must match [a-z0-9][a-z0-9-]*")
	}
	if strings.TrimSpace(s.Description) == "" {
		errs = append(errs, "description is required")
	}
	if !isSupportedLang(s.Language.Name) {
		errs = append(errs, fmt.Sprintf("language %q is not supported (want one of %v)",
			s.Language.Name, SupportedLanguages))
	}
	if s.Language.TestFramework == "" {
		errs = append(errs, "language.test_framework is required")
	}
	if len(errs) == 0 {
		return nil
	}
	return fmt.Errorf("invalid spec: %s", strings.Join(errs, "; "))
}

// Plan computes the full set of files for s without touching the filesystem.
func Plan(s Spec) ([]File, error) {
	if err := s.Validate(); err != nil {
		return nil, err
	}
	files := []File{
		{Path: "buttress.toml", Content: renderButtressTOML(s)},
		{Path: "versions.txt", Content: renderVersionsTXT()},
		{Path: "README.md", Content: renderREADME(s)},
		{Path: "CLAUDE.md", Content: renderCLAUDEMD(s)},
		{Path: "AGENTS.md", Content: renderAGENTSMD()},
		{Path: "docs/overview.md", Content: renderOverview(s)},
		{Path: "spec/behavior.md", Content: renderBehavior(s)},
	}

	tests, err := renderTests(s)
	if err != nil {
		return nil, err
	}
	files = append(files, tests)

	types, err := renderTypes(s)
	if err != nil {
		return nil, err
	}
	files = append(files, types)

	return files, nil
}

// Write applies files to dir. dir is created if missing. If any target file
// already exists, Write returns an error and stops; partial writes from
// earlier in the list are left in place (this is intentional — re-running
// after fixing the conflict is idempotent for the missing files).
func Write(files []File, dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for _, f := range files {
		full := filepath.Join(dir, f.Path)
		if _, err := os.Stat(full); err == nil {
			return fmt.Errorf("refusing to overwrite existing file %s", full)
		} else if !os.IsNotExist(err) {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(full, f.Content, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// WriteSpec is Plan followed by Write.
func WriteSpec(s Spec, dir string) error {
	files, err := Plan(s)
	if err != nil {
		return err
	}
	return Write(files, dir)
}

// --- buttress.toml ---

func renderButtressTOML(s Spec) []byte {
	var b strings.Builder
	fmt.Fprintf(&b, "name = %s\n", tomlString(s.PackageName()))
	fmt.Fprintf(&b, "description = %s\n", tomlString(s.Description))
	b.WriteString("\n[spec]\n")
	b.WriteString("history = \"versions.txt\"\n")
	b.WriteString("\n[[language]]\n")
	fmt.Fprintf(&b, "name = %s\n", tomlString(s.Language.Name))
	if s.Language.RuntimeMinimum != "" {
		fmt.Fprintf(&b, "runtime_minimum = %s\n", tomlString(s.Language.RuntimeMinimum))
	}
	b.WriteString("\n  [[language.test]]\n")
	fmt.Fprintf(&b, "  framework = %s\n", tomlString(s.Language.TestFramework))
	if s.Language.TestFrameworkVer != "" {
		fmt.Fprintf(&b, "  framework_version = %s\n", tomlString(s.Language.TestFrameworkVer))
	}
	if s.Language.TestRunner != "" {
		fmt.Fprintf(&b, "  runner = %s\n", tomlString(s.Language.TestRunner))
	}
	if s.Language.TestRunnerVer != "" {
		fmt.Fprintf(&b, "  runner_version = %s\n", tomlString(s.Language.TestRunnerVer))
	}
	return []byte(b.String())
}

// tomlString renders s as a TOML basic string with proper escaping.
func tomlString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '\\':
			b.WriteString(`\\`)
		case '"':
			b.WriteString(`\"`)
		case '\b':
			b.WriteString(`\b`)
		case '\t':
			b.WriteString(`\t`)
		case '\n':
			b.WriteString(`\n`)
		case '\f':
			b.WriteString(`\f`)
		case '\r':
			b.WriteString(`\r`)
		default:
			if r < 0x20 || r == 0x7f {
				fmt.Fprintf(&b, `\u%04X`, r)
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
	return b.String()
}

// --- versions.txt ---

func renderVersionsTXT() []byte {
	return []byte(`# versions.txt — published spec versions, most recent first.
# Alternating lines: one content hash, then a one-line description.
# Compute a new hash with: buttress hash .
# Then run: buttress publish (or append manually).
`)
}

// --- README.md ---

func renderREADME(s Spec) []byte {
	return []byte(fmt.Sprintf("# %s\n\n%s\n\n"+
		"This is a Flying Buttress spec package: documentation, behavioral specs,\n"+
		"types, and tests for an interface — but no implementation. Consumers\n"+
		"download this spec and generate an implementation locally with their\n"+
		"preferred LLM.\n\n"+
		"## Layout\n\n"+
		"- `docs/` — prose documentation\n"+
		"- `spec/` — behavioral specs\n"+
		"- `tests/%s/` — test cases\n"+
		"- `types/%s/` — type definitions\n"+
		"- `buttress.toml` — package metadata\n"+
		"- `versions.txt` — published versions, most recent first\n",
		s.PackageName(), s.Description, s.Language.Name, s.Language.Name))
}

// --- docs/overview.md ---

func renderOverview(s Spec) []byte {
	if strings.TrimSpace(s.Overview) == "" {
		return []byte(fmt.Sprintf("# %s\n\n%s\n\nDescribe the package's purpose, intended audience, and behavior here.\n",
			s.PackageName(), s.Description))
	}
	out := s.Overview
	if !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	return []byte(out)
}

// --- spec/behavior.md ---

func renderBehavior(s Spec) []byte {
	var b strings.Builder
	fmt.Fprintf(&b, "# Behavior — %s\n\n", s.PackageName())
	b.WriteString("## Behaviors\n\n")
	if len(s.Behaviors) == 0 {
		b.WriteString("_No behaviors defined yet._\n")
	} else {
		for _, x := range s.Behaviors {
			fmt.Fprintf(&b, "- %s\n", x)
		}
	}
	b.WriteString("\n## Edge cases\n\n")
	if len(s.EdgeCases) == 0 {
		b.WriteString("_No edge cases defined yet._\n")
	} else {
		for _, x := range s.EdgeCases {
			fmt.Fprintf(&b, "- %s\n", x)
		}
	}
	return []byte(b.String())
}

// --- tests ---

func renderTests(s Spec) (File, error) {
	switch s.Language.Name {
	case "typescript":
		return File{Path: fmt.Sprintf("tests/typescript/%s.test.ts", s.Pkg), Content: renderTSTests(s)}, nil
	case "go":
		return File{Path: fmt.Sprintf("tests/go/%s_test.go", s.IdentSafePkg()), Content: renderGoTests(s)}, nil
	case "python":
		return File{Path: fmt.Sprintf("tests/python/test_%s.py", s.IdentSafePkg()), Content: renderPyTests(s)}, nil
	}
	return File{}, fmt.Errorf("unsupported language %q", s.Language.Name)
}

func renderTSTests(s Spec) []byte {
	var b strings.Builder
	b.WriteString("import { describe, it } from \"vitest\";\n\n")
	fmt.Fprintf(&b, "describe(%s, () => {\n", jsString(s.PackageName()))
	writeTSStubs(&b, "Behaviors", s.Behaviors)
	writeTSStubs(&b, "Edge cases", s.EdgeCases)
	b.WriteString("});\n")
	return []byte(b.String())
}

func writeTSStubs(b *strings.Builder, header string, items []string) {
	if len(items) == 0 {
		return
	}
	fmt.Fprintf(b, "  // %s\n", header)
	for _, x := range items {
		fmt.Fprintf(b, "  it(%s, () => {\n    // TODO\n  });\n\n", jsString(x))
	}
}

// jsString produces a JavaScript double-quoted string literal.
func jsString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '\\':
			b.WriteString(`\\`)
		case '"':
			b.WriteString(`\"`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			if r < 0x20 {
				fmt.Fprintf(&b, `\u%04X`, r)
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
	return b.String()
}

func renderGoTests(s Spec) []byte {
	var b strings.Builder
	fmt.Fprintf(&b, "package %s_test\n\n", s.IdentSafePkg())
	b.WriteString("import \"testing\"\n\n")
	b.WriteString("func TestSpec(t *testing.T) {\n")
	writeGoStubs(&b, "Behaviors", s.Behaviors)
	writeGoStubs(&b, "Edge cases", s.EdgeCases)
	b.WriteString("}\n")
	return []byte(b.String())
}

func writeGoStubs(b *strings.Builder, header string, items []string) {
	if len(items) == 0 {
		return
	}
	fmt.Fprintf(b, "\t// %s\n", header)
	for _, x := range items {
		fmt.Fprintf(b, "\tt.Run(%s, func(t *testing.T) {\n\t\tt.Skip(\"TODO\")\n\t})\n\n", strconv.Quote(x))
	}
}

func renderPyTests(s Spec) []byte {
	var b strings.Builder
	b.WriteString("import pytest\n\n")
	writePyStubs(&b, "Behaviors", s.Behaviors)
	writePyStubs(&b, "Edge cases", s.EdgeCases)
	return []byte(b.String())
}

func writePyStubs(b *strings.Builder, header string, items []string) {
	if len(items) == 0 {
		return
	}
	fmt.Fprintf(b, "\n# %s\n\n", header)
	seen := map[string]int{}
	for _, x := range items {
		slug := pySlug(x)
		// disambiguate duplicate slugs so all stubs end up in the file
		if n := seen[slug]; n > 0 {
			seen[slug] = n + 1
			slug = fmt.Sprintf("%s_%d", slug, n+1)
		} else {
			seen[slug] = 1
		}
		fmt.Fprintf(b, "@pytest.mark.skip(reason=\"TODO\")\ndef test_%s():\n    %s\n\n",
			slug, pyDocstring(x))
	}
}

var pyNonAlnumRE = regexp.MustCompile(`[^a-z0-9]+`)

// pySlug returns a snake_case Python identifier derived from s.
// Non-alphanumerics collapse to a single underscore; leading/trailing
// underscores are stripped; an empty result becomes "case".
func pySlug(s string) string {
	s = strings.ToLower(s)
	s = pyNonAlnumRE.ReplaceAllString(s, "_")
	s = strings.Trim(s, "_")
	if s == "" {
		return "case"
	}
	if s[0] >= '0' && s[0] <= '9' {
		s = "_" + s
	}
	return s
}

// pyDocstring returns a single-line triple-quoted Python docstring for s.
// Triple-double-quote sequences inside s are escaped.
func pyDocstring(s string) string {
	return `"""` + strings.ReplaceAll(s, `"""`, `\"\"\"`) + `"""`
}

// --- CLAUDE.md ---

func renderCLAUDEMD(s Spec) []byte {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", s.PackageName())
	fmt.Fprintf(&b, "%s\n\n", s.Description)
	b.WriteString("This is a Flying Buttress spec package. It contains documentation,\n")
	b.WriteString("behavioral specs, types, and test stubs — but no implementation.\n")
	b.WriteString("Consumers generate implementations locally with their preferred LLM.\n\n")
	b.WriteString("## Layout\n\n")
	b.WriteString("- `docs/` — prose documentation\n")
	b.WriteString("- `spec/` — behavioral specs\n")
	fmt.Fprintf(&b, "- `tests/%s/` — test stubs\n", s.Language.Name)
	fmt.Fprintf(&b, "- `types/%s/` — type definitions\n\n", s.Language.Name)
	if len(s.Behaviors) > 0 {
		b.WriteString("## Behaviors\n\n")
		for _, x := range s.Behaviors {
			fmt.Fprintf(&b, "- %s\n", x)
		}
		b.WriteByte('\n')
	}
	if len(s.EdgeCases) > 0 {
		b.WriteString("## Edge cases\n\n")
		for _, x := range s.EdgeCases {
			fmt.Fprintf(&b, "- %s\n", x)
		}
		b.WriteByte('\n')
	}
	return []byte(b.String())
}

// --- AGENTS.md ---

func renderAGENTSMD() []byte {
	return []byte("@CLAUDE.md\n")
}

// --- types ---

func renderTypes(s Spec) (File, error) {
	switch s.Language.Name {
	case "typescript":
		return File{
			Path: fmt.Sprintf("types/typescript/%s.d.ts", s.Pkg),
			Content: []byte(fmt.Sprintf("// Type definitions for %s.\n//\n"+
				"// Fill in the signature(s) the implementation should provide.\n\n"+
				"export {};\n", s.PackageName())),
		}, nil
	case "go":
		return File{
			Path: fmt.Sprintf("types/go/%s.go", s.IdentSafePkg()),
			Content: []byte(fmt.Sprintf("// Package %s declares the spec interface for %s.\n//\n"+
				"// Fill in the function signature(s) the implementation must satisfy.\n"+
				"package %s\n",
				s.IdentSafePkg(), s.PackageName(), s.IdentSafePkg())),
		}, nil
	case "python":
		return File{
			Path: fmt.Sprintf("types/python/%s.pyi", s.IdentSafePkg()),
			Content: []byte(fmt.Sprintf("\"\"\"Type stubs for %s.\n\n"+
				"Fill in the signature(s) the implementation should provide.\n"+
				"\"\"\"\n", s.PackageName())),
		}, nil
	}
	return File{}, fmt.Errorf("unsupported language %q", s.Language.Name)
}
