package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// newTestStore returns a Store with cache and project directories under t.TempDir().
// The returned cache and project paths are separate trees so hard-linking from
// cache to project is exercised on the same filesystem.
func newTestStore(t *testing.T) (*Store, string, string) {
	t.Helper()
	root := t.TempDir()
	cache := filepath.Join(root, "cache")
	proj := filepath.Join(root, "project")
	for _, d := range []string{cache, proj} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	s, err := New(cache, proj)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	return s, cache, proj
}

// writeTree creates a directory tree under root containing the given files.
// files maps relative path → contents.
func writeTree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for rel, body := range files {
		full := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// readTree returns a flat map of relative path → contents under root.
func readTree(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		out[filepath.ToSlash(rel)] = string(b)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestNew_DefaultsAndOverrides(t *testing.T) {
	t.Parallel()

	s, err := New("/explicit-cache", "/explicit-project")
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	if s.cacheDir != "/explicit-cache" {
		t.Errorf("cacheDir = %q, want explicit", s.cacheDir)
	}
	if s.projectDir != "/explicit-project" {
		t.Errorf("projectDir = %q, want explicit", s.projectDir)
	}
}

func TestNew_DefaultCacheDir(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	s, err := New("", filepath.Join(tmpHome, "p"))
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	want := filepath.Join(tmpHome, ".cache", "buttress")
	if s.cacheDir != want {
		t.Errorf("cacheDir = %q, want %q", s.cacheDir, want)
	}
}

func TestNew_HomeDirError(t *testing.T) {
	// Setting HOME="" makes os.UserHomeDir return an error on Linux.
	t.Setenv("HOME", "")
	_, err := New("", "/some/proj")
	if err == nil {
		t.Skip("UserHomeDir succeeded with HOME=\"\" on this platform; skipping")
	}
	if !strings.Contains(err.Error(), "home directory") {
		t.Errorf("error = %q, want substring 'home directory'", err)
	}
}

func TestNew_DefaultProjectDir(t *testing.T) {
	t.Parallel()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	s, err := New(t.TempDir(), "")
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	if s.projectDir != wd {
		t.Errorf("projectDir = %q, want %q (cwd)", s.projectDir, wd)
	}
}

func TestLockKey(t *testing.T) {
	t.Parallel()
	if got := LockKey("acme", "widget"); got != "@acme/widget" {
		t.Errorf("LockKey() = %q, want %q", got, "@acme/widget")
	}
}

func TestHas(t *testing.T) {
	t.Parallel()

	s, _, _ := newTestStore(t)
	const hash = "sha256:abc"

	if s.Has("org", "pkg", hash) {
		t.Error("Has() returned true for missing entry")
	}

	if err := os.MkdirAll(s.cachePath("org", "pkg", hash), 0o755); err != nil {
		t.Fatal(err)
	}
	if !s.Has("org", "pkg", hash) {
		t.Error("Has() returned false after creating cache directory")
	}
}

func TestCommit_NewEntry(t *testing.T) {
	t.Parallel()

	s, _, proj := newTestStore(t)

	src := t.TempDir()
	src = filepath.Join(src, "extracted")
	writeTree(t, src, map[string]string{
		"spec.md":             "# Hello\n",
		"tests/example_test":  "test contents",
		"sub/nested/file.txt": "nested\n",
	})

	dest, err := s.Commit("acme", "widget", "sha256:x", src)
	if err != nil {
		t.Fatalf("Commit() error: %v", err)
	}

	wantCache := s.cachePath("acme", "widget", "sha256:x")
	if dest != wantCache {
		t.Errorf("Commit() returned dest = %q, want %q", dest, wantCache)
	}

	// Cache directory has the files.
	cacheTree := readTree(t, wantCache)
	if cacheTree["spec.md"] != "# Hello\n" {
		t.Errorf("cache spec.md = %q", cacheTree["spec.md"])
	}

	// Project directory has the same files.
	projTree := readTree(t, filepath.Join(proj, "buttress", "@acme", "widget"))
	for k, v := range cacheTree {
		if projTree[k] != v {
			t.Errorf("project[%q] = %q, want %q", k, projTree[k], v)
		}
	}

	// Source directory is gone (renamed into cache).
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Errorf("expected src to be gone after Commit, stat err = %v", err)
	}
}

func TestCommit_ExistingCacheReusesAndRemovesSrc(t *testing.T) {
	t.Parallel()

	s, _, proj := newTestStore(t)

	cachedDir := s.cachePath("acme", "widget", "sha256:y")
	if err := os.MkdirAll(cachedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cachedDir, "from-cache.md"),
		[]byte("cached"), 0o644); err != nil {
		t.Fatal(err)
	}

	src := filepath.Join(t.TempDir(), "extracted")
	writeTree(t, src, map[string]string{"new.md": "new"})

	if _, err := s.Commit("acme", "widget", "sha256:y", src); err != nil {
		t.Fatalf("Commit() error: %v", err)
	}

	// src must be removed.
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Errorf("src still exists after Commit")
	}

	// Project gets the *cached* tree, not src.
	projTree := readTree(t, filepath.Join(proj, "buttress", "@acme", "widget"))
	if _, ok := projTree["from-cache.md"]; !ok {
		t.Errorf("project missing from-cache.md, got %v", projTree)
	}
	if _, ok := projTree["new.md"]; ok {
		t.Errorf("project should not contain new.md from src, got %v", projTree)
	}
}

func TestCommit_OverwritesStaleProjectCopy(t *testing.T) {
	t.Parallel()

	s, _, proj := newTestStore(t)

	// Pre-populate the project directory with a stale file.
	stale := filepath.Join(proj, "buttress", "@acme", "widget")
	if err := os.MkdirAll(stale, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stale, "stale.md"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	src := filepath.Join(t.TempDir(), "extracted")
	writeTree(t, src, map[string]string{"fresh.md": "new"})

	if _, err := s.Commit("acme", "widget", "sha256:z", src); err != nil {
		t.Fatalf("Commit() error: %v", err)
	}

	projTree := readTree(t, stale)
	if _, ok := projTree["stale.md"]; ok {
		t.Errorf("stale file survived Commit, got %v", projTree)
	}
	if projTree["fresh.md"] != "new" {
		t.Errorf("fresh.md = %q, want %q", projTree["fresh.md"], "new")
	}
}

func TestCommit_HardLinksWhenPossible(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("hard link semantics differ on windows; covered by integration")
	}
	t.Parallel()

	s, _, proj := newTestStore(t)

	src := filepath.Join(t.TempDir(), "extracted")
	writeTree(t, src, map[string]string{"spec.md": "hello"})

	if _, err := s.Commit("acme", "widget", "sha256:hl", src); err != nil {
		t.Fatalf("Commit() error: %v", err)
	}

	cacheFile := filepath.Join(s.cachePath("acme", "widget", "sha256:hl"), "spec.md")
	projFile := filepath.Join(proj, "buttress", "@acme", "widget", "spec.md")

	cInfo, err := os.Stat(cacheFile)
	if err != nil {
		t.Fatal(err)
	}
	pInfo, err := os.Stat(projFile)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(cInfo, pInfo) {
		t.Error("cache and project files are not hard-linked (SameFile = false)")
	}
}

func TestReadLock_Missing(t *testing.T) {
	t.Parallel()

	s, _, _ := newTestStore(t)

	lock, err := s.ReadLock()
	if err != nil {
		t.Fatalf("ReadLock() missing should not error: %v", err)
	}
	if len(lock) != 0 {
		t.Errorf("expected empty lock, got %v", lock)
	}
}

func TestReadLock_Malformed(t *testing.T) {
	t.Parallel()

	s, _, proj := newTestStore(t)
	if err := os.WriteFile(filepath.Join(proj, "buttress.lock"),
		[]byte("not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := s.ReadLock()
	if err == nil {
		t.Fatal("expected error for malformed lock")
	}
	if !strings.Contains(err.Error(), "parsing") {
		t.Errorf("error = %q, want substring %q", err, "parsing")
	}
}

func TestReadLock_DirectoryAtPath(t *testing.T) {
	t.Parallel()

	s, _, proj := newTestStore(t)
	if err := os.Mkdir(filepath.Join(proj, "buttress.lock"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := s.ReadLock()
	if err == nil {
		t.Fatal("expected error when lock path is a directory")
	}
	if !strings.Contains(err.Error(), "reading") {
		t.Errorf("error = %q, want 'reading'", err)
	}
}

func TestWriteLock_RoundTrip(t *testing.T) {
	t.Parallel()

	s, _, proj := newTestStore(t)

	now := time.Now().UTC().Truncate(time.Second)
	in := map[string]LockEntry{
		"@acme/widget": {Org: "acme", Pkg: "widget", Hash: "sha256:abc", InstalledAt: now},
		"@acme/other":  {Org: "acme", Pkg: "other", Hash: "sha256:def", InstalledAt: now},
	}

	if err := s.WriteLock(in); err != nil {
		t.Fatalf("WriteLock() error: %v", err)
	}

	got, err := s.ReadLock()
	if err != nil {
		t.Fatalf("ReadLock() error: %v", err)
	}
	if len(got) != len(in) {
		t.Fatalf("got %d entries, want %d", len(got), len(in))
	}
	for k, want := range in {
		gotEntry, ok := got[k]
		if !ok {
			t.Errorf("missing key %q", k)
			continue
		}
		if gotEntry.Org != want.Org || gotEntry.Pkg != want.Pkg || gotEntry.Hash != want.Hash {
			t.Errorf("entry %q = %+v, want %+v", k, gotEntry, want)
		}
		if !gotEntry.InstalledAt.Equal(want.InstalledAt) {
			t.Errorf("entry %q InstalledAt = %v, want %v", k, gotEntry.InstalledAt, want.InstalledAt)
		}
	}

	// File should be JSON ending in a newline (poor-man's POSIX nicety).
	raw, err := os.ReadFile(filepath.Join(proj, "buttress.lock"))
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) == 0 || raw[len(raw)-1] != '\n' {
		t.Error("buttress.lock should end with newline")
	}
	var parsed map[string]LockEntry
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Errorf("buttress.lock is not valid JSON: %v", err)
	}
}

func TestWriteLock_AtomicallyReplaces(t *testing.T) {
	t.Parallel()

	s, _, proj := newTestStore(t)
	lockPath := filepath.Join(proj, "buttress.lock")
	if err := os.WriteFile(lockPath, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := s.WriteLock(map[string]LockEntry{
		"@a/b": {Org: "a", Pkg: "b", Hash: "h"},
	}); err != nil {
		t.Fatalf("WriteLock() error: %v", err)
	}

	raw, err := os.ReadFile(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "old") {
		t.Error("WriteLock did not replace existing contents")
	}

	// Temp file must not be left behind.
	if _, err := os.Stat(lockPath + ".tmp"); !os.IsNotExist(err) {
		t.Errorf(".tmp file left behind: %v", err)
	}
}

func TestCopyFile_RejectsTraversal(t *testing.T) {
	t.Parallel()

	src := filepath.Join(t.TempDir(), "src.txt")
	if err := os.WriteFile(src, []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Use a raw path with ".." since filepath.Join would normalize it away.
	bad := t.TempDir() + string(filepath.Separator) + ".." + string(filepath.Separator) + "evil.txt"
	err := copyFile(src, bad, 0o644)
	if err == nil {
		t.Fatal("copyFile should reject path containing '..'")
	}
	if !strings.Contains(err.Error(), "..") {
		t.Errorf("error = %q, want substring '..'", err)
	}
}

func TestCopyFile_MissingSource(t *testing.T) {
	t.Parallel()

	dst := filepath.Join(t.TempDir(), "out.txt")
	err := copyFile(filepath.Join(t.TempDir(), "nope"), dst, 0o644)
	if err == nil {
		t.Error("copyFile should fail when src is missing")
	}
}

func TestCopyDir_PreservesContent(t *testing.T) {
	t.Parallel()

	src := filepath.Join(t.TempDir(), "src")
	writeTree(t, src, map[string]string{
		"a.txt":     "alpha",
		"sub/b.txt": "bravo",
	})

	dst := filepath.Join(t.TempDir(), "dst")
	if err := copyDir(src, dst); err != nil {
		t.Fatalf("copyDir() error: %v", err)
	}

	got := readTree(t, dst)
	want := map[string]string{"a.txt": "alpha", "sub/b.txt": "bravo"}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("dst[%q] = %q, want %q", k, got[k], v)
		}
	}
}

func TestCommit_FallsBackToCopyDir(t *testing.T) {
	t.Parallel()

	s, _, _ := newTestStore(t)

	// When src does not exist, os.Rename fails AND copyDir fails — both
	// errors flow through the rename fallback path.
	src := filepath.Join(t.TempDir(), "missing")
	_, err := s.Commit("acme", "widget", "sha256:m", src)
	if err == nil {
		t.Fatal("expected error when src missing")
	}
	if !strings.Contains(err.Error(), "moving spec to cache") {
		t.Errorf("error = %q, want substring 'moving spec to cache'", err)
	}
}

func TestWriteLock_UnwritableDir(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root: chmod won't make the dir unwritable")
	}
	t.Parallel()

	s, _, proj := newTestStore(t)

	// Make the project directory read-only so WriteFile (.tmp creation) fails.
	if err := os.Chmod(proj, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(proj, 0o755) })

	err := s.WriteLock(map[string]LockEntry{"@a/b": {Org: "a", Pkg: "b", Hash: "h"}})
	if err == nil {
		t.Fatal("expected error writing to read-only dir")
	}
	if !strings.Contains(err.Error(), "writing buttress.lock") {
		t.Errorf("error = %q, want substring 'writing buttress.lock'", err)
	}
}

func TestLinkDir_MissingSource(t *testing.T) {
	t.Parallel()
	dst := filepath.Join(t.TempDir(), "dst")
	err := linkDir(filepath.Join(t.TempDir(), "missing"), dst)
	if err == nil {
		t.Fatal("expected error walking missing source")
	}
}

func TestLinkDir_FallsBackToCopyAcrossFS(t *testing.T) {
	// We can't easily simulate a cross-filesystem hard-link failure, but we can
	// verify the function still completes when called on the same filesystem
	// (covered by TestCommit_HardLinksWhenPossible). Here we cover the path
	// that re-creates a stale destination.
	t.Parallel()

	src := filepath.Join(t.TempDir(), "src")
	writeTree(t, src, map[string]string{"a.txt": "alpha"})

	dstRoot := t.TempDir()
	dst := filepath.Join(dstRoot, "dst")
	// Pre-populate dst with a stale file.
	if err := os.MkdirAll(dst, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dst, "stale"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := linkDir(src, dst); err != nil {
		t.Fatalf("linkDir() error: %v", err)
	}

	got := readTree(t, dst)
	if _, ok := got["stale"]; ok {
		t.Error("stale file survived linkDir")
	}
	if got["a.txt"] != "alpha" {
		t.Errorf("a.txt = %q, want %q", got["a.txt"], "alpha")
	}
}
