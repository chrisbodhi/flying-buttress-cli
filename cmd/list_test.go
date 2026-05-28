package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"buttress/internal/store"
)

// ansiRE matches CSI SGR (Select Graphic Rendition) escape sequences emitted
// by lipgloss when a color profile is active. Tests strip these so assertions
// stay valid even if a future test runner has a TTY attached.
var ansiRE = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func stripANSI(s string) string { return ansiRE.ReplaceAllString(s, "") }

// TestMain pins time.Local to UTC so that calls to t.Local().Format(...) in
// renderList produce a stable string regardless of the host time zone.
// Production behaviour (display in the user's local time) is unchanged.
func TestMain(m *testing.M) {
	time.Local = time.UTC
	os.Exit(m.Run())
}

func mkEntry(org, pkg, hash string, t time.Time) store.LockEntry {
	return store.LockEntry{Org: org, Pkg: pkg, Hash: hash, InstalledAt: t}
}

// fixedTime returns a deterministic UTC timestamp for tests.
func fixedTime(day int) time.Time {
	return time.Date(2026, 4, day, 12, 34, 56, 0, time.UTC)
}

func TestRenderList_Empty(t *testing.T) {
	t.Run("human", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		if err := renderList(map[string]store.LockEntry{}, false, &stdout, &stderr); err != nil {
			t.Fatalf("renderList: %v", err)
		}
		if stdout.Len() != 0 {
			t.Errorf("stdout = %q, want empty", stdout.String())
		}
		got := stripANSI(stderr.String())
		if !strings.Contains(got, "No specs installed") {
			t.Errorf("stderr = %q, want hint mentioning 'No specs installed'", got)
		}
		if !strings.Contains(got, "buttress add") {
			t.Errorf("stderr = %q, want hint mentioning 'buttress add'", got)
		}
	})

	t.Run("json", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		if err := renderList(map[string]store.LockEntry{}, true, &stdout, &stderr); err != nil {
			t.Fatalf("renderList: %v", err)
		}
		if got := strings.TrimSpace(stdout.String()); got != "{}" {
			t.Errorf("stdout = %q, want %q", got, "{}")
		}
		if stderr.Len() != 0 {
			t.Errorf("stderr = %q, want empty (no hint in --json mode)", stderr.String())
		}
	})

	t.Run("nil map", func(t *testing.T) {
		// A nil map is the zero value of map[string]LockEntry; renderList must
		// treat it identically to an empty map and not panic on iteration.
		var stdout, stderr bytes.Buffer
		if err := renderList(nil, false, &stdout, &stderr); err != nil {
			t.Fatalf("renderList(nil): %v", err)
		}
		if !strings.Contains(stripANSI(stderr.String()), "No specs installed") {
			t.Errorf("stderr = %q, want empty-state hint", stderr.String())
		}
	})
}

func TestRenderList_Human(t *testing.T) {
	lock := map[string]store.LockEntry{
		"@org/pkg":             mkEntry("org", "pkg", "sha256:abc123def4567890", fixedTime(30)),
		"@chrisbodhi/left-pad": mkEntry("chrisbodhi", "left-pad", "sha256:ffeeddccbbaa9988", fixedTime(29)),
		"@acme/widget":         mkEntry("acme", "widget", "sha256:0011223344556677", fixedTime(28)),
	}

	var stdout, stderr bytes.Buffer
	if err := renderList(lock, false, &stdout, &stderr); err != nil {
		t.Fatalf("renderList: %v", err)
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, want empty", stderr.String())
	}

	out := stripANSI(stdout.String())
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if got, want := len(lines), len(lock); got != want {
		t.Fatalf("got %d output lines, want %d:\n%s", got, want, out)
	}

	t.Run("alphabetical order", func(t *testing.T) {
		wantOrder := []string{"@acme/widget", "@chrisbodhi/left-pad", "@org/pkg"}
		for i, prefix := range wantOrder {
			if !strings.HasPrefix(lines[i], prefix) {
				t.Errorf("line %d = %q, want prefix %q", i, lines[i], prefix)
			}
		}
	})

	t.Run("hash truncated", func(t *testing.T) {
		for _, line := range lines {
			if strings.Contains(line, "ffeeddccbbaa9988") ||
				strings.Contains(line, "abc123def4567890") ||
				strings.Contains(line, "0011223344556677") {
				t.Errorf("line %q contains full hash; expected ShortHash truncation", line)
			}
		}
		// short hashes should be present
		wantShort := []string{"sha256:0011223", "sha256:ffeeddcc", "sha256:abc123de"}
		for _, s := range wantShort {
			if !strings.Contains(out, s) {
				t.Errorf("output = %q, want short hash %q", out, s)
			}
		}
	})

	t.Run("install date in RFC3339", func(t *testing.T) {
		// fixedTime(28..30) at 12:34:56 UTC.
		wantTimes := []string{"2026-04-28T12:34:56Z", "2026-04-29T12:34:56Z", "2026-04-30T12:34:56Z"}
		for _, ts := range wantTimes {
			if !strings.Contains(out, ts) {
				t.Errorf("output = %q, want timestamp %q", out, ts)
			}
		}
	})

	t.Run("columns separated by double space", func(t *testing.T) {
		// "name  short-hash  date" — at least two double-space separators per row.
		for i, line := range lines {
			if got := strings.Count(line, "  "); got < 2 {
				t.Errorf("line %d = %q has %d double-space separators, want >= 2", i, line, got)
			}
		}
	})
}

func TestRenderList_JSON(t *testing.T) {
	lock := map[string]store.LockEntry{
		"@org/pkg":             mkEntry("org", "pkg", "sha256:abc123def4567890", fixedTime(30)),
		"@chrisbodhi/left-pad": mkEntry("chrisbodhi", "left-pad", "sha256:ffeeddccbbaa9988", fixedTime(29)),
	}

	var stdout, stderr bytes.Buffer
	if err := renderList(lock, true, &stdout, &stderr); err != nil {
		t.Fatalf("renderList: %v", err)
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, want empty", stderr.String())
	}

	t.Run("round-trips", func(t *testing.T) {
		var got map[string]store.LockEntry
		if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
			t.Fatalf("unmarshal: %v\noutput:\n%s", err, stdout.String())
		}
		if len(got) != len(lock) {
			t.Fatalf("decoded %d entries, want %d", len(got), len(lock))
		}
		for k, want := range lock {
			g, ok := got[k]
			if !ok {
				t.Errorf("missing key %q", k)
				continue
			}
			if g.Org != want.Org || g.Pkg != want.Pkg || g.Hash != want.Hash {
				t.Errorf("entry %q: got %+v, want %+v", k, g, want)
			}
			if !g.InstalledAt.Equal(want.InstalledAt) {
				t.Errorf("entry %q: InstalledAt = %s, want %s", k, g.InstalledAt, want.InstalledAt)
			}
		}
	})

	t.Run("keys sorted", func(t *testing.T) {
		// encoding/json sorts map keys by default; assert the contract holds
		// for the output we produce.
		out := stdout.String()
		idxC := strings.Index(out, `"@chrisbodhi/left-pad"`)
		idxO := strings.Index(out, `"@org/pkg"`)
		if idxC < 0 || idxO < 0 {
			t.Fatalf("missing expected keys in output:\n%s", out)
		}
		if idxC >= idxO {
			t.Errorf("expected @chrisbodhi key before @org key; got positions %d and %d", idxC, idxO)
		}
	})

	t.Run("two-space indent", func(t *testing.T) {
		if !strings.Contains(stdout.String(), "\n  \"@") {
			t.Errorf("expected 2-space indented JSON; got:\n%s", stdout.String())
		}
	})

	t.Run("trailing newline", func(t *testing.T) {
		// json.Encoder.Encode appends a newline; preserve that for POSIX-friendly output.
		if !strings.HasSuffix(stdout.String(), "\n") {
			t.Errorf("expected trailing newline; got %q", stdout.String())
		}
	})

	t.Run("no ANSI codes", func(t *testing.T) {
		// JSON output must be machine-parseable; styles must not leak in.
		if ansiRE.MatchString(stdout.String()) {
			t.Errorf("JSON output contains ANSI escape codes: %q", stdout.String())
		}
	})
}

func TestRenderList_HashFormats(t *testing.T) {
	// Ensure renderList does not mangle non-sha256 or unprefixed hashes.
	cases := []struct {
		name     string
		hash     string
		wantSubs []string
	}{
		{"short sha256", "sha256:abc", []string{"sha256:abc"}},
		{"long sha256 truncated", "sha256:0123456789abcdef0123", []string{"sha256:01234567"}},
		{"unprefixed long", "0123456789abcdef0123", []string{"0123456789ab"}},
		{"unprefixed short", "abcd", []string{"abcd"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			lock := map[string]store.LockEntry{
				"@a/b": mkEntry("a", "b", tc.hash, fixedTime(1)),
			}
			var stdout, stderr bytes.Buffer
			if err := renderList(lock, false, &stdout, &stderr); err != nil {
				t.Fatalf("renderList: %v", err)
			}
			out := stripANSI(stdout.String())
			for _, sub := range tc.wantSubs {
				if !strings.Contains(out, sub) {
					t.Errorf("output = %q, want substring %q", out, sub)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// runList integration tests
// ---------------------------------------------------------------------------

func TestRunList_EmptyLock(t *testing.T) {
	// Not parallel: mutates HOME and cwd.
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	projDir := t.TempDir()
	origWd, _ := os.Getwd()
	if err := os.Chdir(projDir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(origWd) })

	// No buttress.lock → ReadLock returns empty map.
	if err := runList(false); err != nil {
		t.Fatalf("runList(false) error: %v", err)
	}
}

func TestRunList_JSONFlag(t *testing.T) {
	// Not parallel: mutates HOME and cwd.
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	projDir := t.TempDir()
	// Seed a non-empty lock file so JSON output is non-trivial.
	lockPath := filepath.Join(projDir, "buttress.lock")
	if err := os.WriteFile(lockPath, []byte(`{"@a/b":{"org":"a","pkg":"b","hash":"sha256:abc","installed_at":"2026-01-01T00:00:00Z"}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	origWd, _ := os.Getwd()
	if err := os.Chdir(projDir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(origWd) })

	if err := runList(true); err != nil {
		t.Fatalf("runList(true) error: %v", err)
	}
}

func TestRunList_LoadConfigError(t *testing.T) {
	// Not parallel: mutates HOME.
	// Make HOME point at a file (not a directory) so os.UserHomeDir() works but
	// config.Load() will fail to read the config file when the config dir is unreadable.
	// Easiest: point HOME at a temp dir but put a file where the config dir would be.
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	// Create .config/buttress as a file (not dir) so config.DefaultPath resolution fails to stat.
	configBase := filepath.Join(tmpHome, ".config")
	if err := os.MkdirAll(configBase, 0o755); err != nil {
		t.Fatal(err)
	}
	// Make config.toml a directory so TOML parsing fails with a non-NotExist error.
	cfgDir := filepath.Join(configBase, "buttress")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Write an invalid TOML file.
	if err := os.WriteFile(filepath.Join(cfgDir, "config.toml"), []byte("not toml = = bad"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := runList(false); err == nil {
		t.Fatal("expected error from runList when config is invalid")
	}
}

func TestRunList_HumanOutput(t *testing.T) {
	// Not parallel: mutates HOME and cwd.
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	projDir := t.TempDir()
	lockPath := filepath.Join(projDir, "buttress.lock")
	if err := os.WriteFile(lockPath, []byte(`{"@org/pkg":{"org":"org","pkg":"pkg","hash":"sha256:abc123","installed_at":"2026-01-01T00:00:00Z"}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	origWd, _ := os.Getwd()
	if err := os.Chdir(projDir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(origWd) })

	if err := runList(false); err != nil {
		t.Fatalf("runList(false) error: %v", err)
	}
}

func TestRunList_NewListCmd_Metadata(t *testing.T) {
	t.Parallel()
	cmd := newListCmd()
	if !strings.HasPrefix(cmd.Use, "list") {
		t.Errorf("Use = %q, want prefix 'list'", cmd.Use)
	}
	if cmd.Short == "" {
		t.Error("Short is empty")
	}
}

func TestNewListCmd_Execute(t *testing.T) {
	// Execute newListCmd via cobra to cover the RunE closure body.
	// Not parallel: mutates HOME and cwd.
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	projDir := t.TempDir()
	origWd, _ := os.Getwd()
	if err := os.Chdir(projDir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(origWd) })

	cmd := newListCmd()
	cmd.SetArgs([]string{})
	var out strings.Builder
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("newListCmd.Execute() error: %v", err)
	}
}

func TestRunList_Deterministic(t *testing.T) {
	// Map iteration is randomized; renderList must produce byte-identical
	// output across invocations for the same input.
	lock := map[string]store.LockEntry{
		"@org/a": mkEntry("org", "a", "sha256:aaa", fixedTime(1)),
		"@org/b": mkEntry("org", "b", "sha256:bbb", fixedTime(2)),
		"@org/c": mkEntry("org", "c", "sha256:ccc", fixedTime(3)),
		"@org/d": mkEntry("org", "d", "sha256:ddd", fixedTime(4)),
		"@org/e": mkEntry("org", "e", "sha256:eee", fixedTime(5)),
	}

	for _, asJSON := range []bool{false, true} {
		asJSON := asJSON
		name := "human"
		if asJSON {
			name = "json"
		}
		t.Run(name, func(t *testing.T) {
			var first bytes.Buffer
			if err := renderList(lock, asJSON, &first, &bytes.Buffer{}); err != nil {
				t.Fatalf("renderList: %v", err)
			}
			for i := 0; i < 20; i++ {
				var next bytes.Buffer
				if err := renderList(lock, asJSON, &next, &bytes.Buffer{}); err != nil {
					t.Fatalf("renderList iteration %d: %v", i, err)
				}
				if first.String() != next.String() {
					t.Fatalf("non-deterministic output on iteration %d:\nfirst:\n%s\nnext:\n%s",
						i, first.String(), next.String())
				}
			}
		})
	}
}
