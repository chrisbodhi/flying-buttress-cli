package scaffold

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
)

func sampleSpec(lang string) Spec {
	return Spec{
		Org:         "chrisbodhi",
		Pkg:         "left-pad",
		Description: "Pad a string on the left with a fill character.",
		Behaviors: []string{
			"left-pads a string to a target length",
			"returns the original string if already at or above the target length",
		},
		EdgeCases: []string{
			"handles empty string input",
			"target length of zero",
		},
		Language: defaultLang(lang),
	}
}

func defaultLang(name string) Language {
	switch name {
	case "typescript":
		return Language{Name: "typescript", RuntimeMinimum: "node 20", TestFramework: "vitest", TestFrameworkVer: "3.0.0", TestRunner: "vitest", TestRunnerVer: "3.0.0"}
	case "go":
		return Language{Name: "go", RuntimeMinimum: "go 1.22", TestFramework: "testing", TestRunner: "go test"}
	case "python":
		return Language{Name: "python", RuntimeMinimum: "python 3.11", TestFramework: "pytest", TestFrameworkVer: "8.0.0", TestRunner: "pytest", TestRunnerVer: "8.0.0"}
	}
	return Language{}
}

// --- Spec helpers ---

func TestSpec_PackageNameAndIdent(t *testing.T) {
	s := Spec{Org: "chrisbodhi", Pkg: "left-pad"}
	if got, want := s.PackageName(), "@chrisbodhi/left-pad"; got != want {
		t.Errorf("PackageName = %q, want %q", got, want)
	}
	if got, want := s.IdentSafePkg(), "left_pad"; got != want {
		t.Errorf("IdentSafePkg = %q, want %q", got, want)
	}
}

func TestSpec_Validate(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(*Spec)
		wantErr string // substring; "" means must succeed
	}{
		{"valid", func(s *Spec) {}, ""},
		{"missing org", func(s *Spec) { s.Org = "" }, "org must match"},
		{"uppercase org", func(s *Spec) { s.Org = "Acme" }, "org must match"},
		{"leading hyphen org", func(s *Spec) { s.Org = "-x" }, "org must match"},
		{"missing pkg", func(s *Spec) { s.Pkg = "" }, "pkg must match"},
		{"slash in pkg", func(s *Spec) { s.Pkg = "a/b" }, "pkg must match"},
		{"missing description", func(s *Spec) { s.Description = "  " }, "description is required"},
		{"unsupported language", func(s *Spec) { s.Language.Name = "haskell" }, "not supported"},
		{"missing test framework", func(s *Spec) { s.Language.TestFramework = "" }, "test_framework"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := sampleSpec("typescript")
			tc.mutate(&s)
			err := s.Validate()
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate: unexpected error %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("Validate: want error containing %q, got nil", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("Validate err = %q, want substring %q", err.Error(), tc.wantErr)
			}
		})
	}
}

// --- Plan: the file set for each language ---

func TestPlan_FilePaths(t *testing.T) {
	cases := []struct {
		lang string
		want []string
	}{
		{"typescript", []string{
			"buttress.toml", "versions.txt", "README.md", "CLAUDE.md", "AGENTS.md",
			"docs/overview.md", "spec/behavior.md",
			"tests/typescript/left-pad.test.ts",
			"types/typescript/left-pad.d.ts",
		}},
		{"go", []string{
			"buttress.toml", "versions.txt", "README.md", "CLAUDE.md", "AGENTS.md",
			"docs/overview.md", "spec/behavior.md",
			"tests/go/left_pad_test.go",
			"types/go/left_pad.go",
		}},
		{"python", []string{
			"buttress.toml", "versions.txt", "README.md", "CLAUDE.md", "AGENTS.md",
			"docs/overview.md", "spec/behavior.md",
			"tests/python/test_left_pad.py",
			"types/python/left_pad.pyi",
		}},
	}
	for _, tc := range cases {
		t.Run(tc.lang, func(t *testing.T) {
			files, err := Plan(sampleSpec(tc.lang))
			if err != nil {
				t.Fatalf("Plan: %v", err)
			}
			got := make([]string, 0, len(files))
			for _, f := range files {
				got = append(got, f.Path)
			}
			sort.Strings(got)
			want := append([]string(nil), tc.want...)
			sort.Strings(want)
			if !reflect.DeepEqual(got, want) {
				t.Errorf("file set mismatch\n got: %v\nwant: %v", got, want)
			}
		})
	}
}

func TestPlan_RejectsInvalidSpec(t *testing.T) {
	s := sampleSpec("typescript")
	s.Org = "" // invalid
	if _, err := Plan(s); err == nil {
		t.Fatal("Plan: expected error for invalid spec, got nil")
	}
}

// --- buttress.toml ---

func TestRenderButtressTOML_RoundTrips(t *testing.T) {
	s := sampleSpec("typescript")
	out := renderButtressTOML(s)

	var parsed struct {
		Name        string `toml:"name"`
		Description string `toml:"description"`
		Spec        struct {
			History string `toml:"history"`
		} `toml:"spec"`
		Language []struct {
			Name           string `toml:"name"`
			RuntimeMinimum string `toml:"runtime_minimum"`
			Test           []struct {
				Framework        string `toml:"framework"`
				FrameworkVersion string `toml:"framework_version"`
				Runner           string `toml:"runner"`
				RunnerVersion    string `toml:"runner_version"`
			} `toml:"test"`
		} `toml:"language"`
	}
	if err := toml.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("toml unmarshal: %v\noutput:\n%s", err, out)
	}

	if parsed.Name != "@chrisbodhi/left-pad" {
		t.Errorf("name = %q", parsed.Name)
	}
	if parsed.Description != s.Description {
		t.Errorf("description = %q", parsed.Description)
	}
	if parsed.Spec.History != "versions.txt" {
		t.Errorf("spec.history = %q", parsed.Spec.History)
	}
	if len(parsed.Language) != 1 {
		t.Fatalf("language entries = %d, want 1", len(parsed.Language))
	}
	lg := parsed.Language[0]
	if lg.Name != "typescript" || lg.RuntimeMinimum != "node 20" {
		t.Errorf("language = %+v", lg)
	}
	if len(lg.Test) != 1 {
		t.Fatalf("language.test entries = %d, want 1", len(lg.Test))
	}
	tf := lg.Test[0]
	if tf.Framework != "vitest" || tf.FrameworkVersion != "3.0.0" || tf.Runner != "vitest" || tf.RunnerVersion != "3.0.0" {
		t.Errorf("test entry = %+v", tf)
	}
}

func TestRenderButtressTOML_OmitsEmptyOptionals(t *testing.T) {
	s := sampleSpec("go") // go has no FrameworkVer / RunnerVer
	out := string(renderButtressTOML(s))
	if strings.Contains(out, "framework_version") {
		t.Errorf("expected no framework_version line, got:\n%s", out)
	}
	if strings.Contains(out, "runner_version") {
		t.Errorf("expected no runner_version line, got:\n%s", out)
	}
	if !strings.Contains(out, "runtime_minimum = \"go 1.22\"") {
		t.Errorf("expected runtime_minimum line, got:\n%s", out)
	}
}

func TestTomlString_Escaping(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"plain", `plain`, `"plain"`},
		{"double quotes", `with "quotes"`, `"with \"quotes\""`},
		{"backslash", `back\slash`, `"back\\slash"`},
		{"newline", "line\nbreak", `"line\nbreak"`},
		{"tab", "tab\there", `"tab\there"`},
		{"control byte 0x01", "x" + string([]byte{0x01}) + "y", `"x\u0001y"`},
		{"DEL byte 0x7f", "x" + string([]byte{0x7f}) + "y", `"x\u007Fy"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tomlString(tc.in); got != tc.want {
				t.Errorf("tomlString(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestRenderButtressTOML_NastyDescription(t *testing.T) {
	s := sampleSpec("typescript")
	s.Description = `Has "quotes", \backslashes\, and a` + "\ttab"
	out := renderButtressTOML(s)

	var parsed struct {
		Description string `toml:"description"`
	}
	if err := toml.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("toml unmarshal: %v\n%s", err, out)
	}
	if parsed.Description != s.Description {
		t.Errorf("description round-trip:\n got: %q\nwant: %q", parsed.Description, s.Description)
	}
}

// --- TS test stubs ---

func TestRenderTSTests_Shape(t *testing.T) {
	s := sampleSpec("typescript")
	out := string(renderTSTests(s))

	// Imports + describe wrapper.
	if !strings.HasPrefix(out, "import { describe, it } from \"vitest\";\n") {
		t.Errorf("expected vitest import at top; got:\n%s", out)
	}
	if !strings.Contains(out, `describe("@chrisbodhi/left-pad", () => {`) {
		t.Errorf("expected describe header; got:\n%s", out)
	}

	// Headers and stub count.
	for _, want := range []string{"// Behaviors", "// Edge cases"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in output:\n%s", want, out)
		}
	}
	gotIts := strings.Count(out, "it(")
	wantIts := len(s.Behaviors) + len(s.EdgeCases)
	if gotIts != wantIts {
		t.Errorf("it() stubs = %d, want %d", gotIts, wantIts)
	}

	// Each bullet is reproduced verbatim inside an it().
	for _, x := range append(append([]string{}, s.Behaviors...), s.EdgeCases...) {
		if !strings.Contains(out, `it("`+x+`",`) {
			t.Errorf("missing stub for %q in:\n%s", x, out)
		}
	}
}

func TestRenderTSTests_BulletEscaping(t *testing.T) {
	s := sampleSpec("typescript")
	s.Behaviors = []string{`accepts "smart" quotes`, "uses \\ as separator", "newline\nin bullet"}
	s.EdgeCases = nil
	out := string(renderTSTests(s))

	for _, want := range []string{
		`it("accepts \"smart\" quotes"`,
		`it("uses \\ as separator"`,
		`it("newline\nin bullet"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in output:\n%s", want, out)
		}
	}
	// And no Edge cases header when there are none.
	if strings.Contains(out, "Edge cases") {
		t.Errorf("did not expect Edge cases header; got:\n%s", out)
	}
}

func TestRenderTSTests_EmptyLists(t *testing.T) {
	s := sampleSpec("typescript")
	s.Behaviors = nil
	s.EdgeCases = nil
	out := string(renderTSTests(s))
	if strings.Contains(out, "Behaviors") || strings.Contains(out, "Edge cases") {
		t.Errorf("expected no section headers for empty input; got:\n%s", out)
	}
	if strings.Contains(out, "it(") {
		t.Errorf("expected no it() stubs for empty input; got:\n%s", out)
	}
}

// --- Go test stubs: must parse as valid Go ---

func TestRenderGoTests_Parses(t *testing.T) {
	s := sampleSpec("go")
	out := renderGoTests(s)
	fset := token.NewFileSet()
	if _, err := parser.ParseFile(fset, "left_pad_test.go", out, parser.ParseComments); err != nil {
		t.Fatalf("generated Go test file does not parse: %v\n----\n%s", err, out)
	}
	src := string(out)
	if !strings.Contains(src, "package left_pad_test") {
		t.Errorf("expected package left_pad_test; got:\n%s", src)
	}
	gotRuns := strings.Count(src, "t.Run(")
	want := len(s.Behaviors) + len(s.EdgeCases)
	if gotRuns != want {
		t.Errorf("t.Run stubs = %d, want %d", gotRuns, want)
	}
}

func TestRenderGoTests_BulletEscaping(t *testing.T) {
	s := sampleSpec("go")
	s.Behaviors = []string{`bullet with "quotes" and a \ slash`}
	s.EdgeCases = nil
	out := renderGoTests(s)
	fset := token.NewFileSet()
	if _, err := parser.ParseFile(fset, "x_test.go", out, 0); err != nil {
		t.Fatalf("generated Go did not parse with tricky bullet: %v\n%s", err, out)
	}
}

// --- Python stubs ---

func TestRenderPyTests_Shape(t *testing.T) {
	s := sampleSpec("python")
	out := string(renderPyTests(s))
	if !strings.HasPrefix(out, "import pytest\n") {
		t.Errorf("expected pytest import at top; got:\n%s", out)
	}
	gotDefs := strings.Count(out, "def test_")
	want := len(s.Behaviors) + len(s.EdgeCases)
	if gotDefs != want {
		t.Errorf("def test_ stubs = %d, want %d", gotDefs, want)
	}
	if !strings.Contains(out, "def test_left_pads_a_string_to_a_target_length") {
		t.Errorf("expected slugified test name; got:\n%s", out)
	}
}

func TestRenderPyTests_DuplicateSlugsDisambiguated(t *testing.T) {
	s := sampleSpec("python")
	s.Behaviors = []string{"handles X", "handles X!", "handles X."}
	s.EdgeCases = nil
	out := string(renderPyTests(s))
	wantNames := []string{"def test_handles_x(", "def test_handles_x_2(", "def test_handles_x_3("}
	for _, want := range wantNames {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in output:\n%s", want, out)
		}
	}
}

func TestPySlug(t *testing.T) {
	cases := []struct{ in, want string }{
		{"left-pads a string", "left_pads_a_string"},
		{"  trims  spaces  ", "trims_spaces"},
		{"UPPER Case", "upper_case"},
		{"!!!only-symbols!!!", "only_symbols"},
		{"!!!", "case"},
		{"42 starts with digit", "_42_starts_with_digit"},
		{"unicode — emdash", "unicode_emdash"},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			if got := pySlug(tc.in); got != tc.want {
				t.Errorf("pySlug(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// --- type stubs ---

func TestRenderTypes_Go_Parses(t *testing.T) {
	f, err := renderTypes(sampleSpec("go"))
	if err != nil {
		t.Fatalf("renderTypes: %v", err)
	}
	fset := token.NewFileSet()
	if _, err := parser.ParseFile(fset, f.Path, f.Content, parser.ParseComments); err != nil {
		t.Fatalf("generated Go type file does not parse: %v\n%s", err, f.Content)
	}
	if !strings.Contains(string(f.Content), "package left_pad") {
		t.Errorf("expected package left_pad; got:\n%s", f.Content)
	}
}

func TestRenderTypes_TypeScript(t *testing.T) {
	f, err := renderTypes(sampleSpec("typescript"))
	if err != nil {
		t.Fatalf("renderTypes: %v", err)
	}
	if got := f.Path; got != "types/typescript/left-pad.d.ts" {
		t.Errorf("path = %q", got)
	}
	if !strings.Contains(string(f.Content), "export {};") {
		t.Errorf("expected export {} placeholder; got:\n%s", f.Content)
	}
}

func TestRenderTypes_Python(t *testing.T) {
	f, err := renderTypes(sampleSpec("python"))
	if err != nil {
		t.Fatalf("renderTypes: %v", err)
	}
	if got := f.Path; got != "types/python/left_pad.pyi" {
		t.Errorf("path = %q", got)
	}
	if !strings.HasPrefix(string(f.Content), `"""Type stubs for @chrisbodhi/left-pad.`) {
		t.Errorf("expected docstring header; got:\n%s", f.Content)
	}
}

// --- behavior + overview ---

func TestRenderBehavior(t *testing.T) {
	t.Run("populated", func(t *testing.T) {
		s := sampleSpec("typescript")
		out := string(renderBehavior(s))
		for _, x := range s.Behaviors {
			if !strings.Contains(out, "- "+x) {
				t.Errorf("missing behavior bullet %q in:\n%s", x, out)
			}
		}
		for _, x := range s.EdgeCases {
			if !strings.Contains(out, "- "+x) {
				t.Errorf("missing edge bullet %q in:\n%s", x, out)
			}
		}
	})
	t.Run("empty", func(t *testing.T) {
		s := sampleSpec("typescript")
		s.Behaviors = nil
		s.EdgeCases = nil
		out := string(renderBehavior(s))
		if !strings.Contains(out, "_No behaviors defined yet._") {
			t.Errorf("expected empty-state placeholder for behaviors; got:\n%s", out)
		}
		if !strings.Contains(out, "_No edge cases defined yet._") {
			t.Errorf("expected empty-state placeholder for edge cases; got:\n%s", out)
		}
	})
}

func TestRenderOverview(t *testing.T) {
	t.Run("default when empty", func(t *testing.T) {
		s := sampleSpec("typescript")
		s.Overview = ""
		out := string(renderOverview(s))
		if !strings.Contains(out, "# @chrisbodhi/left-pad") {
			t.Errorf("expected default H1; got:\n%s", out)
		}
	})
	t.Run("user overview verbatim with trailing newline", func(t *testing.T) {
		s := sampleSpec("typescript")
		s.Overview = "custom prose"
		out := string(renderOverview(s))
		if out != "custom prose\n" {
			t.Errorf("got %q, want %q", out, "custom prose\n")
		}
	})
	t.Run("preserves existing trailing newline", func(t *testing.T) {
		s := sampleSpec("typescript")
		s.Overview = "already has newline\n"
		out := string(renderOverview(s))
		if out != "already has newline\n" {
			t.Errorf("got %q", out)
		}
	})
}

// --- Write ---

func TestWrite_CreatesNestedDirsAndFiles(t *testing.T) {
	dir := t.TempDir()
	files := []File{
		{Path: "a.txt", Content: []byte("A\n")},
		{Path: "deep/nested/b.txt", Content: []byte("B\n")},
	}
	if err := Write(files, dir); err != nil {
		t.Fatalf("Write: %v", err)
	}
	for _, f := range files {
		got, err := os.ReadFile(filepath.Join(dir, f.Path))
		if err != nil {
			t.Errorf("missing file %s: %v", f.Path, err)
			continue
		}
		if string(got) != string(f.Content) {
			t.Errorf("file %s = %q, want %q", f.Path, got, f.Content)
		}
	}
}

func TestWrite_RefusesOverwrite(t *testing.T) {
	dir := t.TempDir()
	existing := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(existing, []byte("existing\n"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	err := Write([]File{{Path: "a.txt", Content: []byte("new\n")}}, dir)
	if err == nil {
		t.Fatal("Write: expected error overwriting existing file, got nil")
	}
	if !strings.Contains(err.Error(), "refusing to overwrite") {
		t.Errorf("err = %v, want 'refusing to overwrite'", err)
	}
	got, _ := os.ReadFile(existing)
	if string(got) != "existing\n" {
		t.Errorf("existing file mutated: %q", got)
	}
}

func TestWriteSpec_EndToEnd(t *testing.T) {
	dir := t.TempDir()
	s := sampleSpec("go")
	if err := WriteSpec(s, dir); err != nil {
		t.Fatalf("WriteSpec: %v", err)
	}

	// Every planned file must exist on disk.
	planned, err := Plan(s)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	for _, f := range planned {
		full := filepath.Join(dir, f.Path)
		got, err := os.ReadFile(full)
		if err != nil {
			t.Errorf("missing %s: %v", f.Path, err)
			continue
		}
		if string(got) != string(f.Content) {
			t.Errorf("file %s content drift", f.Path)
		}
	}

	// buttress.toml must round-trip through a real TOML parser.
	tomlBytes, err := os.ReadFile(filepath.Join(dir, "buttress.toml"))
	if err != nil {
		t.Fatal(err)
	}
	var meta map[string]interface{}
	if err := toml.Unmarshal(tomlBytes, &meta); err != nil {
		t.Errorf("buttress.toml does not parse: %v", err)
	}

	// Generated Go test file must parse.
	goBytes, err := os.ReadFile(filepath.Join(dir, "tests/go/left_pad_test.go"))
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	if _, err := parser.ParseFile(fset, "left_pad_test.go", goBytes, parser.ParseComments); err != nil {
		t.Errorf("generated Go test does not parse: %v", err)
	}
}
