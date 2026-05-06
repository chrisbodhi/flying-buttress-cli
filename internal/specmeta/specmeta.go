// Package specmeta parses buttress.toml from a spec archive.
package specmeta

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// SpecMeta holds the parsed contents of a spec package's buttress.toml.
type SpecMeta struct {
	Name        string     `toml:"name"`
	Description string     `toml:"description"`
	Languages   []Language `toml:"language"`
}

// Language describes one supported target language and its test configuration.
type Language struct {
	Name           string       `toml:"name"`
	Runtime        string       `toml:"runtime"`         // e.g. "bun", "node"
	RuntimeMinimum string       `toml:"runtime_minimum"` // e.g. "1.3.13"
	PackageManager string       `toml:"package_manager"` // e.g. "bun", "npm", "pnpm", "yarn"
	Tests          []TestConfig `toml:"test"`
}

// TestConfig holds the test runner and framework declared for a language.
// Packages is a list of [name, version] pairs the test suite requires.
type TestConfig struct {
	Framework        string     `toml:"framework"`
	FrameworkVersion string     `toml:"framework_version"`
	Runner           string     `toml:"runner"`
	RunnerVersion    string     `toml:"runner_version"`
	Packages         [][]string `toml:"packages"`
}

// Load reads buttress.toml from specDir.
// Returns an empty SpecMeta (no error) if the file does not exist.
func Load(specDir string) (*SpecMeta, error) {
	path := filepath.Join(specDir, "buttress.toml")
	var meta SpecMeta
	_, err := toml.DecodeFile(path, &meta)
	if os.IsNotExist(err) {
		return &SpecMeta{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading buttress.toml: %w", err)
	}
	return &meta, nil
}

// LanguageConfig returns the Language entry for the given language name,
// or nil if the language is not declared in this spec.
func (m *SpecMeta) LanguageConfig(lang string) *Language {
	for i := range m.Languages {
		if strings.EqualFold(m.Languages[i].Name, lang) {
			return &m.Languages[i]
		}
	}
	return nil
}

// TestRunner returns the first declared test runner for lang, or "" if unset.
func (m *SpecMeta) TestRunner(lang string) (runner, version string) {
	lc := m.LanguageConfig(lang)
	if lc == nil || len(lc.Tests) == 0 {
		return "", ""
	}
	return lc.Tests[0].Runner, lc.Tests[0].RunnerVersion
}

// PackageManagerFor returns the declared package manager for lang, defaulting
// to "npm" when the field is absent.
func (m *SpecMeta) PackageManagerFor(lang string) string {
	lc := m.LanguageConfig(lang)
	if lc == nil || lc.PackageManager == "" {
		return "npm"
	}
	return strings.ToLower(lc.PackageManager)
}

// TestPackages returns the required packages for lang as "name@version" install
// specs ready to pass directly to npm or bun. Entries without a version are
// returned as bare names.
func (m *SpecMeta) TestPackages(lang string) []string {
	lc := m.LanguageConfig(lang)
	if lc == nil || len(lc.Tests) == 0 {
		return nil
	}
	var specs []string
	for _, p := range lc.Tests[0].Packages {
		if len(p) == 0 {
			continue
		}
		s := p[0]
		if len(p) > 1 && p[1] != "" {
			s += "@" + p[1]
		}
		specs = append(specs, s)
	}
	return specs
}
