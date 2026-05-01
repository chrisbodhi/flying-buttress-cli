package localconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeLocalConfig(t *testing.T, dir, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, ".buttress.local.toml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoad_Missing(t *testing.T) {
	t.Parallel()
	cfg, err := Load(t.TempDir())
	if err != nil {
		t.Fatalf("Load() should not error when missing: %v", err)
	}
	if cfg == nil {
		t.Fatal("Load() returned nil")
	}
	if cfg.PackageManager != "" {
		t.Errorf("PackageManager = %q, want empty", cfg.PackageManager)
	}
	if cfg.Overrides == nil {
		t.Error("Overrides map should be initialised even when missing")
	}
	if len(cfg.Overrides) != 0 {
		t.Errorf("Overrides = %v, want empty", cfg.Overrides)
	}
}

func TestLoad_PackageManager(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeLocalConfig(t, dir, `package_manager = "bun"`)

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.PackageManager != "bun" {
		t.Errorf("PackageManager = %q, want bun", cfg.PackageManager)
	}
}

func TestLoad_PackageOverrides(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeLocalConfig(t, dir, `
package_manager = "npm"

["@chrisbodhi/left-pad"]
output = "src/lib/left-pad.ts"
language = "typescript"

["@acme/widget"]
output = "internal/widget.go"
`)

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.PackageManager != "npm" {
		t.Errorf("PackageManager = %q", cfg.PackageManager)
	}

	leftPad, ok := cfg.Overrides["@chrisbodhi/left-pad"]
	if !ok {
		t.Fatal("missing @chrisbodhi/left-pad override")
	}
	if leftPad.Output != "src/lib/left-pad.ts" {
		t.Errorf("left-pad Output = %q", leftPad.Output)
	}
	if leftPad.Language != "typescript" {
		t.Errorf("left-pad Language = %q", leftPad.Language)
	}

	widget, ok := cfg.Overrides["@acme/widget"]
	if !ok {
		t.Fatal("missing @acme/widget override")
	}
	if widget.Output != "internal/widget.go" {
		t.Errorf("widget Output = %q", widget.Output)
	}
	if widget.Language != "" {
		t.Errorf("widget Language = %q, want empty (no language set)", widget.Language)
	}
}

func TestLoad_OverrideWithoutOutputOrLanguage(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	// A table with no fields — should still produce an empty PackageOverride.
	writeLocalConfig(t, dir, `["@acme/widget"]`)

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	o, ok := cfg.Overrides["@acme/widget"]
	if !ok {
		t.Fatal("missing override entry")
	}
	if o.Output != "" || o.Language != "" {
		t.Errorf("override = %+v, want empty fields", o)
	}
}

func TestLoad_NonStringPackageManager(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	// package_manager set to a non-string value should be silently ignored.
	writeLocalConfig(t, dir, `package_manager = 42`)

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.PackageManager != "" {
		t.Errorf("PackageManager = %q, want empty for non-string", cfg.PackageManager)
	}
}

func TestLoad_InvalidTOML(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeLocalConfig(t, dir, "not = = valid")
	_, err := Load(dir)
	if err == nil {
		t.Fatal("expected parse error")
	}
	if !strings.Contains(err.Error(), ".buttress.local.toml") {
		t.Errorf("error = %q, want substring '.buttress.local.toml'", err)
	}
}

func TestLoad_ReadError(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".buttress.local.toml"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := Load(dir)
	if err == nil {
		t.Fatal("expected error when .buttress.local.toml is a directory")
	}
}

func TestLoad_IgnoresUnknownTopLevelKeys(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	// Top-level scalar key (other than package_manager) should be silently dropped.
	writeLocalConfig(t, dir, `
package_manager = "bun"
some_other_key = "ignored"

["@a/b"]
output = "x.ts"
`)
	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.PackageManager != "bun" {
		t.Errorf("PackageManager = %q", cfg.PackageManager)
	}
	if _, ok := cfg.Overrides["@a/b"]; !ok {
		t.Error("missing @a/b override")
	}
}
