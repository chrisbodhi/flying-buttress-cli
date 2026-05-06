package contenthash

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"buttress/internal/testutil"
)


// expectedHash mirrors the algorithm in HashDirVerbose so tests can assert
// the exact hash for a known set of files (in lexical order).
func expectedHash(orderedFiles []struct{ Rel, Body string }) string {
	h := sha256.New()
	for _, f := range orderedFiles {
		fmt.Fprintf(h, "%s\n", f.Rel)
		h.Write([]byte(f.Body))
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil))
}

func TestIncluded(t *testing.T) {
	t.Parallel()

	tests := []struct {
		path string
		want bool
	}{
		{"human/spec.md", true},
		{"human/sub/file.txt", true},
		{"machine/types.ts", true},
		{"machine/tests/example.test.ts", true},
		{"VERSIONS.txt", false},
		{"buttress.toml", false},
		{"README.md", false},
		{".gitignore", false},
		{"tsconfig.json", false},
		{"", false},
		{"humanlike/file.md", false}, // must require trailing slash
		{"machine_other/file.md", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			t.Parallel()
			if got := Included(tt.path); got != tt.want {
				t.Errorf("Included(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestHashDir_HashesOnlySpecDirs(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	testutil.WriteTree(t, dir, map[string]string{
		"human/spec.md":         "# Spec",
		"machine/types.ts":      "export type X = {}",
		"VERSIONS.txt":          "should be excluded",
		"buttress.toml":         "should be excluded",
		"README.md":             "should be excluded",
		".gitignore":            "should be excluded",
		"random/elsewhere/file": "should be excluded",
	})

	hash, files, err := HashDirVerbose(dir)
	if err != nil {
		t.Fatalf("HashDirVerbose() error: %v", err)
	}
	if !strings.HasPrefix(hash, "sha256:") {
		t.Errorf("hash = %q, want sha256: prefix", hash)
	}
	if len(files) != 2 {
		t.Errorf("included files = %v, want 2 entries", files)
	}
	for _, f := range files {
		if !strings.HasPrefix(f, "human/") && !strings.HasPrefix(f, "machine/") {
			t.Errorf("included file %q is outside spec dirs", f)
		}
	}
}

func TestHashDir_StableAcrossRuns(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	testutil.WriteTree(t, dir, map[string]string{
		"human/a.md":   "alpha",
		"human/b.md":   "bravo",
		"machine/x.ts": "type X = number",
	})

	first, err := HashDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	second, err := HashDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Errorf("hash unstable: %q vs %q", first, second)
	}
}

func TestHashDir_DiffersWhenContentChanges(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	testutil.WriteTree(t, dir, map[string]string{"human/spec.md": "v1"})
	h1, err := HashDir(dir)
	if err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(dir, "human", "spec.md"), []byte("v2"), 0o644); err != nil {
		t.Fatal(err)
	}
	h2, err := HashDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if h1 == h2 {
		t.Error("hash unchanged after content change")
	}
}

func TestHashDir_IgnoresPackagingFiles(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	testutil.WriteTree(t, dir, map[string]string{"human/spec.md": "alpha"})
	h1, err := HashDir(dir)
	if err != nil {
		t.Fatal(err)
	}

	// Add a packaging-only file. Hash must not change.
	if err := os.WriteFile(filepath.Join(dir, "VERSIONS.txt"), []byte("anything"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("a readme"), 0o644); err != nil {
		t.Fatal(err)
	}
	h2, err := HashDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if h1 != h2 {
		t.Errorf("hash changed when only packaging files were added: %q → %q", h1, h2)
	}
}

func TestHashDir_KnownValue(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	testutil.WriteTree(t, dir, map[string]string{
		"human/a.md":   "alpha",
		"machine/x.ts": "beta",
	})

	got, err := HashDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	// Files are walked in lexical order under each spec dir, in specDirs order
	// (human/ then machine/).
	want := expectedHash([]struct{ Rel, Body string }{
		{"human/a.md", "alpha"},
		{"machine/x.ts", "beta"},
	})
	if got != want {
		t.Errorf("HashDir() = %q, want %q", got, want)
	}
}

func TestHashDir_MissingSpecDirsIsOK(t *testing.T) {
	t.Parallel()

	// Only human/ exists.
	dir := t.TempDir()
	testutil.WriteTree(t, dir, map[string]string{"human/spec.md": "x"})
	if _, err := HashDir(dir); err != nil {
		t.Errorf("HashDir() error: %v", err)
	}

	// Neither dir exists — should yield the empty hash, no error.
	dir2 := t.TempDir()
	hash, files, err := HashDirVerbose(dir2)
	if err != nil {
		t.Fatalf("HashDirVerbose() error: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("expected 0 included files, got %d: %v", len(files), files)
	}
	if !strings.HasPrefix(hash, "sha256:") {
		t.Errorf("hash should still be sha256:-prefixed, got %q", hash)
	}
}

func TestHashDir_MissingDir(t *testing.T) {
	t.Parallel()
	_, _, err := HashDirVerbose(filepath.Join(t.TempDir(), "does-not-exist"))
	// HashDirVerbose tolerates missing spec subdirs but the parent dir not
	// existing is also fine — Stat returns IsNotExist for each spec subdir.
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestHashDir_OrderedLexically(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	testutil.WriteTree(t, dir, map[string]string{
		"human/c.md": "c",
		"human/a.md": "a",
		"human/b.md": "b",
	})
	_, files, err := HashDirVerbose(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"human/a.md", "human/b.md", "human/c.md"}
	for i, w := range want {
		if i >= len(files) || files[i] != w {
			t.Errorf("files[%d] = %v, want %q", i, files, w)
			break
		}
	}
}

func TestHashDir_UnreadableFile(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root: chmod won't make file unreadable")
	}
	t.Parallel()

	dir := t.TempDir()
	testutil.WriteTree(t, dir, map[string]string{"human/spec.md": "x"})
	if err := os.Chmod(filepath.Join(dir, "human", "spec.md"), 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Chmod(filepath.Join(dir, "human", "spec.md"), 0o644)
	})

	_, err := HashDir(dir)
	if err == nil {
		t.Fatal("expected error opening unreadable file")
	}
}
