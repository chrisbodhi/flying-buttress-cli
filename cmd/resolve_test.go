package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"buttress/internal/config"
	"buttress/internal/ref"
)

func TestLangExt(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in, want string
	}{
		{"go", "go"},
		{"Go", "go"},
		{"typescript", "ts"},
		{"TypeScript", "ts"},
		{"ts", "ts"},
		{"javascript", "js"},
		{"js", "js"},
		{"python", "py"},
		{"py", "py"},
		{"rust", "rs"},
		{"rs", "rs"},
		{"java", "java"},
		{"ruby", "rb"},
		{"rb", "rb"},
		{"c", "c"},
		{"cpp", "cpp"},
		{"c++", "cpp"},
		{"csharp", "cs"},
		{"c#", "cs"},
		{"kotlin", "kt"},
		{"kt", "kt"},
		{"swift", "swift"},
		{"unknown", "unknown"}, // passthrough
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			t.Parallel()
			if got := langExt(tt.in); got != tt.want {
				t.Errorf("langExt(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestResolveGenOutput_DefaultPathFromConfigLanguage(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cfg := &config.Config{Project: config.ProjectConfig{Language: "typescript"}}
	pkg := ref.PackageRef{Org: "acme", Pkg: "widget"}

	out, lang, err := resolveGenOutput(dir, pkg, cfg)
	if err != nil {
		t.Fatalf("resolveGenOutput() error: %v", err)
	}
	if lang != "typescript" {
		t.Errorf("lang = %q, want typescript", lang)
	}
	want := filepath.Join(dir, "widget.ts")
	if out != want {
		t.Errorf("out = %q, want %q", out, want)
	}
}

func TestResolveGenOutput_NoLanguageConfigured(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cfg := &config.Config{}
	pkg := ref.PackageRef{Org: "acme", Pkg: "widget"}

	_, _, err := resolveGenOutput(dir, pkg, cfg)
	if err == nil {
		t.Fatal("expected error when no language is configured")
	}
	if !strings.Contains(err.Error(), "no language configured") {
		t.Errorf("error = %q, want substring 'no language configured'", err)
	}
}

func TestResolveGenOutput_LocalLanguageOverride(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".buttress.local.toml"),
		[]byte(`["@acme/widget"]
language = "go"
`), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{Project: config.ProjectConfig{Language: "typescript"}}
	pkg := ref.PackageRef{Org: "acme", Pkg: "widget"}

	out, lang, err := resolveGenOutput(dir, pkg, cfg)
	if err != nil {
		t.Fatalf("resolveGenOutput() error: %v", err)
	}
	if lang != "go" {
		t.Errorf("lang = %q, want go (from override)", lang)
	}
	want := filepath.Join(dir, "widget.go")
	if out != want {
		t.Errorf("out = %q, want %q", out, want)
	}
}

func TestResolveGenOutput_LocalOutputOverride(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".buttress.local.toml"),
		[]byte(`["@acme/widget"]
output = "src/lib/widget.ts"
language = "typescript"
`), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{}
	pkg := ref.PackageRef{Org: "acme", Pkg: "widget"}

	out, lang, err := resolveGenOutput(dir, pkg, cfg)
	if err != nil {
		t.Fatalf("resolveGenOutput() error: %v", err)
	}
	if lang != "typescript" {
		t.Errorf("lang = %q, want typescript", lang)
	}
	want := filepath.Join(dir, "src", "lib", "widget.ts")
	if out != want {
		t.Errorf("out = %q, want %q", out, want)
	}
}

func TestResolveGenOutput_OverrideOnlyLanguage(t *testing.T) {
	// Override sets language but no output — should still use the default path
	// derivation but with the override language.
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".buttress.local.toml"),
		[]byte(`["@acme/widget"]
language = "rust"
`), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{Project: config.ProjectConfig{Language: "go"}}
	pkg := ref.PackageRef{Org: "acme", Pkg: "widget"}

	out, lang, err := resolveGenOutput(dir, pkg, cfg)
	if err != nil {
		t.Fatalf("resolveGenOutput() error: %v", err)
	}
	if lang != "rust" {
		t.Errorf("lang = %q, want rust", lang)
	}
	if !strings.HasSuffix(out, ".rs") {
		t.Errorf("out = %q, want .rs extension", out)
	}
}

func TestResolveGenOutput_OverrideForOtherPackageIgnored(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".buttress.local.toml"),
		[]byte(`["@other/pkg"]
output = "should-not-be-used.ts"
language = "ts"
`), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{Project: config.ProjectConfig{Language: "go"}}
	pkg := ref.PackageRef{Org: "acme", Pkg: "widget"}

	out, lang, err := resolveGenOutput(dir, pkg, cfg)
	if err != nil {
		t.Fatalf("resolveGenOutput() error: %v", err)
	}
	if lang != "go" {
		t.Errorf("lang = %q, want go (override for different pkg)", lang)
	}
	if !strings.HasSuffix(out, "widget.go") {
		t.Errorf("out = %q, want widget.go suffix", out)
	}
}

func TestResolveGenOutput_LocalConfigInvalid(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".buttress.local.toml"),
		[]byte("not = = valid"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Project: config.ProjectConfig{Language: "go"}}
	pkg := ref.PackageRef{Org: "acme", Pkg: "widget"}

	_, _, err := resolveGenOutput(dir, pkg, cfg)
	if err == nil {
		t.Fatal("expected error when localconfig is invalid")
	}
}
