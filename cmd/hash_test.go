package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// captureStdoutStderr swaps os.Stdout/Stderr for pipes during fn and returns
// what was written. Used because runHash currently writes directly to
// os.Stdout/Stderr — a small trade-off vs. the deps-based pattern in cmd/add.
func captureStdoutStderr(t *testing.T, fn func() error) (stdout, stderr string, err error) {
	t.Helper()

	origOut := os.Stdout
	origErr := os.Stderr

	rOut, wOut, _ := os.Pipe()
	rErr, wErr, _ := os.Pipe()
	os.Stdout = wOut
	os.Stderr = wErr

	defer func() {
		os.Stdout = origOut
		os.Stderr = origErr
	}()

	err = fn()

	_ = wOut.Close()
	_ = wErr.Close()

	var bufOut, bufErr bytes.Buffer
	_, _ = bufOut.ReadFrom(rOut)
	_, _ = bufErr.ReadFrom(rErr)

	return bufOut.String(), bufErr.String(), err
}

func TestNewHashCmd_Metadata(t *testing.T) {
	t.Parallel()
	c := newHashCmd()
	if !strings.HasPrefix(c.Use, "hash") {
		t.Errorf("Use = %q", c.Use)
	}
	if c.Short == "" {
		t.Error("Short is empty")
	}
	if c.Flag("verbose") == nil {
		t.Error("missing --verbose flag")
	}
}

func TestNewHashCmd_AcceptsZeroOrOneArg(t *testing.T) {
	t.Parallel()
	c := newHashCmd()
	if err := c.Args(c, nil); err != nil {
		t.Errorf("zero args should be allowed: %v", err)
	}
	if err := c.Args(c, []string{"."}); err != nil {
		t.Errorf("one arg should be allowed: %v", err)
	}
	if err := c.Args(c, []string{".", "."}); err == nil {
		t.Error("two args should be rejected")
	}
}

func TestRunHash_HappyPath(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "human"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "human", "spec.md"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	stdout, _, err := captureStdoutStderr(t, func() error {
		return runHash(dir, false)
	})
	if err != nil {
		t.Fatalf("runHash() error: %v", err)
	}
	if !strings.HasPrefix(strings.TrimSpace(stdout), "sha256:") {
		t.Errorf("stdout = %q, want sha256:- prefix", stdout)
	}
}

func TestRunHash_Verbose(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "human"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "human", "spec.md"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "machine"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "machine", "types.ts"), []byte("type X={}"), 0o644); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, err := captureStdoutStderr(t, func() error {
		return runHash(dir, true)
	})
	if err != nil {
		t.Fatalf("runHash() error: %v", err)
	}
	if !strings.HasPrefix(strings.TrimSpace(stdout), "sha256:") {
		t.Errorf("stdout missing hash: %q", stdout)
	}
	for _, want := range []string{"human/spec.md", "machine/types.ts"} {
		if !strings.Contains(stderr, want) {
			t.Errorf("verbose stderr missing %q, got: %s", want, stderr)
		}
	}
}

func TestRunHash_MissingDir(t *testing.T) {
	err := runHash(filepath.Join(t.TempDir(), "nope"), false)
	if err == nil {
		t.Fatal("expected error for missing directory")
	}
}

func TestRunHash_NotADirectory(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "f")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := runHash(file, false)
	if err == nil {
		t.Fatal("expected error when path is a file")
	}
	if !strings.Contains(err.Error(), "not a directory") {
		t.Errorf("error = %q, want substring 'not a directory'", err)
	}
}

func TestHashCmd_DefaultsToCurrentDir(t *testing.T) {
	// Invoking the command with no positional args defaults to ".". The runner
	// should at least try to stat "." successfully (cwd is always a dir).
	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		// Restore cwd so other tests aren't surprised.
		_ = os.Chdir("/")
	})

	c := newHashCmd()
	c.SetArgs([]string{})
	c.SetOut(&bytes.Buffer{})
	c.SetErr(&bytes.Buffer{})

	stdout, _, err := captureStdoutStderr(t, func() error {
		return c.Execute()
	})
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if !strings.HasPrefix(strings.TrimSpace(stdout), "sha256:") {
		t.Errorf("stdout = %q, want sha256: prefix", stdout)
	}
}
