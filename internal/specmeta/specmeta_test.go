package specmeta

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeMeta(t *testing.T, dir, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "buttress.toml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoad_Missing(t *testing.T) {
	t.Parallel()
	m, err := Load(t.TempDir())
	if err != nil {
		t.Fatalf("Load() should not error when file is missing: %v", err)
	}
	if m == nil {
		t.Fatal("Load() returned nil meta")
	}
	if m.Name != "" || m.Description != "" || len(m.Languages) != 0 {
		t.Errorf("expected empty meta, got %+v", m)
	}
}

func TestLoad_Full(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeMeta(t, dir, `
name = "left-pad"
description = "Pads a string."

[[language]]
name = "typescript"
runtime = "bun"
runtime_minimum = "1.3.13"
package_manager = "bun"

  [[language.test]]
  framework = "vitest"
  framework_version = "3.2.4"
  runner = "vitest"
  runner_version = "3.2.4"
  packages = [["@types/node", "20.0.0"], ["typescript", "5.4.0"]]

[[language]]
name = "go"
runtime = "go"

  [[language.test]]
  framework = "testing"
  runner = "go test"
`)

	m, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if m.Name != "left-pad" {
		t.Errorf("Name = %q", m.Name)
	}
	if m.Description != "Pads a string." {
		t.Errorf("Description = %q", m.Description)
	}
	if len(m.Languages) != 2 {
		t.Fatalf("Languages = %+v, want 2", m.Languages)
	}

	ts := m.LanguageConfig("typescript")
	if ts == nil {
		t.Fatal("LanguageConfig(typescript) = nil")
	}
	if ts.Runtime != "bun" || ts.RuntimeMinimum != "1.3.13" {
		t.Errorf("typescript runtime = %+v", ts)
	}
	if len(ts.Tests) != 1 {
		t.Fatalf("typescript Tests = %+v", ts.Tests)
	}
	if ts.Tests[0].Framework != "vitest" {
		t.Errorf("framework = %q", ts.Tests[0].Framework)
	}
	if len(ts.Tests[0].Packages) != 2 {
		t.Errorf("packages = %v", ts.Tests[0].Packages)
	}

	runner, version := m.TestRunner("typescript")
	if runner != "vitest" || version != "3.2.4" {
		t.Errorf("TestRunner = (%q, %q), want (vitest, 3.2.4)", runner, version)
	}

	if pm := m.PackageManagerFor("typescript"); pm != "bun" {
		t.Errorf("PackageManagerFor(typescript) = %q, want bun", pm)
	}
	if pm := m.PackageManagerFor("go"); pm != "npm" {
		t.Errorf("PackageManagerFor(go) = %q, want npm (default)", pm)
	}
	if pm := m.PackageManagerFor("rust"); pm != "npm" {
		t.Errorf("PackageManagerFor(rust) = %q, want npm (missing lang)", pm)
	}
}

func TestLoad_InvalidTOML(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeMeta(t, dir, "name = = invalid")
	_, err := Load(dir)
	if err == nil {
		t.Fatal("expected parse error")
	}
	if !strings.Contains(err.Error(), "buttress.toml") {
		t.Errorf("error = %q, want substring 'buttress.toml'", err)
	}
}

func TestLoad_ReadError(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	// Make buttress.toml a directory so toml.DecodeFile returns a non-IsNotExist error.
	if err := os.Mkdir(filepath.Join(dir, "buttress.toml"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := Load(dir)
	if err == nil {
		t.Fatal("expected error when buttress.toml is a directory")
	}
}

func TestLanguageConfig(t *testing.T) {
	t.Parallel()

	m := &SpecMeta{
		Languages: []Language{
			{Name: "TypeScript"},
			{Name: "Go"},
		},
	}
	if got := m.LanguageConfig("typescript"); got == nil {
		t.Error("expected case-insensitive match for TypeScript")
	}
	if got := m.LanguageConfig("GO"); got == nil {
		t.Error("expected case-insensitive match for Go")
	}
	if got := m.LanguageConfig("python"); got != nil {
		t.Errorf("expected nil for missing language, got %+v", got)
	}
}

func TestTestRunner_NoLanguage(t *testing.T) {
	t.Parallel()
	m := &SpecMeta{}
	runner, version := m.TestRunner("ts")
	if runner != "" || version != "" {
		t.Errorf("TestRunner = (%q, %q), want empty", runner, version)
	}
}

func TestTestRunner_NoTests(t *testing.T) {
	t.Parallel()
	m := &SpecMeta{Languages: []Language{{Name: "ts"}}}
	runner, version := m.TestRunner("ts")
	if runner != "" || version != "" {
		t.Errorf("TestRunner = (%q, %q), want empty", runner, version)
	}
}
