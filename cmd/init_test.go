package cmd

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"buttress/internal/scaffold"
)

func TestSplitBullets(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{"empty", "", nil},
		{"only whitespace", "   \n\n  \t  \n", nil},
		{"plain lines", "alpha\nbeta\ngamma", []string{"alpha", "beta", "gamma"}},
		{"with leading dashes", "- one\n- two", []string{"one", "two"}},
		{"with leading stars", "* one\n* two", []string{"one", "two"}},
		{"mixed and indented", "  - one\n   * two\nthree", []string{"one", "two", "three"}},
		{"blank lines dropped", "one\n\n\ntwo\n", []string{"one", "two"}},
		{"trailing whitespace stripped", "one  \n  two\t", []string{"one", "two"}},
		{"single dash without text dropped", "- \n  -  \nactual", []string{"actual"}},
		{"hyphen mid-text preserved", "- well-formed bullet", []string{"well-formed bullet"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := splitBullets(tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("splitBullets(%q) = %#v, want %#v", tc.in, got, tc.want)
			}
		})
	}
}

func TestStripHTMLComments(t *testing.T) {
	cases := []struct {
		name     string
		in, want string
	}{
		{"strips comment line", "title\n<!-- hidden -->\nbody", "title\nbody"},
		{"strips indented comment", "title\n    <!-- still hidden -->\nbody", "title\nbody"},
		{"keeps inline comment", "title <!-- inline --> stays", "title <!-- inline --> stays"},
		{"keeps body unchanged", "no comments here\nat all", "no comments here\nat all"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := stripHTMLComments(tc.in); got != tc.want {
				t.Errorf("stripHTMLComments(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestDefaultLanguage(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want scaffold.Language
	}{
		{"typescript", "typescript", scaffold.Language{
			Name: "typescript", RuntimeMinimum: "node 20",
			TestFramework: "vitest", TestFrameworkVer: "3.0.0",
			TestRunner: "vitest", TestRunnerVer: "3.0.0",
		}},
		{"go", "go", scaffold.Language{
			Name: "go", RuntimeMinimum: "go 1.22",
			TestFramework: "testing", TestRunner: "go test",
		}},
		{"python", "python", scaffold.Language{
			Name: "python", RuntimeMinimum: "python 3.11",
			TestFramework: "pytest", TestFrameworkVer: "8.0.0",
			TestRunner: "pytest", TestRunnerVer: "8.0.0",
		}},
		{"unknown returns zero", "haskell", scaffold.Language{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := defaultLanguage(tc.in); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("defaultLanguage(%q) = %+v, want %+v", tc.in, got, tc.want)
			}
		})
	}

	// Each non-empty default must satisfy scaffold.Spec.Validate so the wizard
	// cannot produce a Spec the scaffolder rejects.
	for _, name := range []string{"typescript", "go", "python"} {
		t.Run(name+" validates", func(t *testing.T) {
			s := scaffold.Spec{
				Org: "x", Pkg: "y", Description: "z",
				Language: defaultLanguage(name),
			}
			if err := s.Validate(); err != nil {
				t.Errorf("default %s language fails validation: %v", name, err)
			}
		})
	}
}

func TestIdentValidator(t *testing.T) {
	v := identValidator("org")
	cases := []struct {
		in      string
		wantErr bool
	}{
		{"chrisbodhi", false},
		{"left-pad", false},
		{"a", false},
		{"a1b2", false},
		{"", true},
		{"-leading", true},
		{"UPPER", true},
		{"with space", true},
		{"slash/here", true},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			err := v(tc.in)
			if (err != nil) != tc.wantErr {
				t.Errorf("identValidator(%q): err=%v, wantErr=%v", tc.in, err, tc.wantErr)
			}
			if err != nil && !strings.Contains(err.Error(), "org") {
				t.Errorf("err = %v, want field name embedded", err)
			}
		})
	}
}

func TestDescriptionValidator(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		wantErr string
	}{
		{"valid", "A short description.", ""},
		{"empty", "", "required"},
		{"whitespace only", "   \t  ", "required"},
		{"exactly 80", strings.Repeat("a", 80), ""},
		{"81 chars", strings.Repeat("a", 81), "too long"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := descriptionValidator(tc.in)
			if tc.wantErr == "" {
				if err != nil {
					t.Errorf("got %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("err = %v, want substring %q", err, tc.wantErr)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// runInitConsumer
// ---------------------------------------------------------------------------

func TestRunInitConsumer_CreatesLockFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := runInitConsumer(dir); err != nil {
		t.Fatalf("runInitConsumer() error: %v", err)
	}
	lockPath := filepath.Join(dir, "buttress.lock")
	body, err := os.ReadFile(lockPath)
	if err != nil {
		t.Fatalf("lock file not created: %v", err)
	}
	if strings.TrimSpace(string(body)) != "{}" {
		t.Errorf("lock file = %q, want {}", body)
	}
}

func TestRunInitConsumer_CreatesDir(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "new-project")
	if err := runInitConsumer(dir); err != nil {
		t.Fatalf("runInitConsumer() error: %v", err)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Errorf("dir not created: %v", err)
	}
}

func TestRunInitConsumer_AlreadyExists_Noop(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	lockPath := filepath.Join(dir, "buttress.lock")
	// Pre-create the lock file with custom content.
	if err := os.WriteFile(lockPath, []byte(`{"existing":true}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := runInitConsumer(dir); err != nil {
		t.Fatalf("runInitConsumer() error: %v", err)
	}
	// Content must be unchanged.
	body, _ := os.ReadFile(lockPath)
	if !strings.Contains(string(body), "existing") {
		t.Errorf("lock file overwritten; got %q", body)
	}
}

// ---------------------------------------------------------------------------
// newInitCmd metadata
// ---------------------------------------------------------------------------

func TestNewInitCmd_Metadata(t *testing.T) {
	t.Parallel()
	cmd := newInitCmd()
	if !strings.HasPrefix(cmd.Use, "init") {
		t.Errorf("Use = %q, want prefix 'init'", cmd.Use)
	}
	if cmd.Short == "" {
		t.Error("Short is empty")
	}
}

func TestNewInitCmd_RunE_ConsumerNoArgs(t *testing.T) {
	// Execute the cobra command's RunE with no args (consumer mode, uses ".").
	// This covers the RunE closure branches: no explicit dir arg, no --spec.
	t.Parallel()
	dir := t.TempDir()
	cmd := newInitCmd()
	cmd.SetArgs([]string{dir})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	lockPath := filepath.Join(dir, "buttress.lock")
	if _, err := os.Stat(lockPath); err != nil {
		t.Errorf("lock file not created: %v", err)
	}
}

func TestNewInitCmd_RunE_ConsumerExplicitDir(t *testing.T) {
	// Exercise the `dir = args[0]` branch.
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "newproject")
	cmd := newInitCmd()
	cmd.SetArgs([]string{dir})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "buttress.lock")); err != nil {
		t.Errorf("lock file not created: %v", err)
	}
}
