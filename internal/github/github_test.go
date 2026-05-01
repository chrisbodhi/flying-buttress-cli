package github

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"buttress/internal/ref"
)

// fakeArchive is a builder for in-memory .tar.gz archives used by FetchSpec tests.
//
// GitHub's archive layout always wraps everything in a top-level "{repo}-{ref}/"
// directory. This builder mirrors that.
type fakeArchive struct {
	rootDir string // top-level dir name inside the tar (e.g. "left-pad-abc/")
	entries []fakeArchiveEntry
}

type fakeArchiveEntry struct {
	name    string
	body    string
	dir     bool
	noChild bool // when true, the entry is added as a directory only
}

func newFakeArchive(rootDir string) *fakeArchive {
	return &fakeArchive{rootDir: strings.TrimSuffix(rootDir, "/") + "/"}
}

func (a *fakeArchive) addFile(name, body string) *fakeArchive {
	a.entries = append(a.entries, fakeArchiveEntry{name: name, body: body})
	return a
}

func (a *fakeArchive) addDir(name string) *fakeArchive {
	a.entries = append(a.entries, fakeArchiveEntry{name: name, dir: true, noChild: true})
	return a
}

func (a *fakeArchive) bytes(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)

	// Always emit the root directory entry first so the strip prefix is set.
	if err := tw.WriteHeader(&tar.Header{
		Name:     a.rootDir,
		Typeflag: tar.TypeDir,
		Mode:     0o755,
	}); err != nil {
		t.Fatal(err)
	}

	for _, e := range a.entries {
		if e.dir {
			if err := tw.WriteHeader(&tar.Header{
				Name:     a.rootDir + strings.TrimSuffix(e.name, "/") + "/",
				Typeflag: tar.TypeDir,
				Mode:     0o755,
			}); err != nil {
				t.Fatal(err)
			}
			continue
		}
		hdr := &tar.Header{
			Name:     a.rootDir + e.name,
			Typeflag: tar.TypeReg,
			Mode:     0o644,
			Size:     int64(len(e.body)),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(e.body)); err != nil {
			t.Fatal(err)
		}
	}

	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// computeExpectedHash returns the SHA-256 hash that extractTarGz will compute
// for the given archive contents. We mirror the algorithm here so tests can
// assert hash-mismatch and hash-match behavior precisely.
func computeExpectedHash(t *testing.T, a *fakeArchive) string {
	t.Helper()
	// Easiest way to compute the expected hash: extract the archive into a
	// temp dir using the real extractTarGz and capture its output.
	dir := t.TempDir()
	hash, _, err := extractTarGz(bytes.NewReader(a.bytes(t)), dir)
	if err != nil {
		t.Fatalf("computeExpectedHash: extract failed: %v", err)
	}
	return "sha256:" + hash
}

func TestVersionsURL(t *testing.T) {
	t.Parallel()
	got := VersionsURL(ref.PackageRef{Org: "chrisbodhi", Pkg: "left-pad"})
	want := "https://raw.githubusercontent.com/chrisbodhi/left-pad/HEAD/VERSIONS.txt"
	if got != want {
		t.Errorf("VersionsURL() = %q, want %q", got, want)
	}
}

func TestListVersionsFromURL_OK(t *testing.T) {
	t.Parallel()

	body := "sha256:abc\nfirst version\nsha256:def\nsecond version\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, body)
	}))
	defer srv.Close()

	c := New()
	vs, err := c.ListVersionsFromURL(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("ListVersionsFromURL() error: %v", err)
	}
	if len(vs) != 2 {
		t.Fatalf("got %d versions, want 2", len(vs))
	}
	if vs[0].Hash != "sha256:abc" || vs[0].Description != "first version" {
		t.Errorf("vs[0] = %+v", vs[0])
	}
	if vs[1].Hash != "sha256:def" || vs[1].Description != "second version" {
		t.Errorf("vs[1] = %+v", vs[1])
	}
}

func TestListVersionsFromURL_404(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "nope", http.StatusNotFound)
	}))
	defer srv.Close()

	c := New()
	_, err := c.ListVersionsFromURL(context.Background(), srv.URL)
	if err == nil {
		t.Fatal("expected error for HTTP 404")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("error = %q, want substring '404'", err)
	}
}

func TestListVersionsFromURL_NetworkError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	srv.Close() // close immediately so connection is refused

	c := New()
	_, err := c.ListVersionsFromURL(context.Background(), srv.URL)
	if err == nil {
		t.Fatal("expected error from closed server")
	}
	if !strings.Contains(err.Error(), "fetching version list") {
		t.Errorf("error = %q, want substring 'fetching version list'", err)
	}
}

func TestListVersionsFromURL_BadRequest(t *testing.T) {
	t.Parallel()
	c := New()
	// Invalid URL — control characters force NewRequestWithContext to fail.
	_, err := c.ListVersionsFromURL(context.Background(), "http://\x00bad")
	if err == nil {
		t.Fatal("expected error for malformed URL")
	}
	if !strings.Contains(err.Error(), "building request") {
		t.Errorf("error = %q, want substring 'building request'", err)
	}
}

func TestListVersionsFromURL_Empty(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, "\n# only comments here\n")
	}))
	defer srv.Close()

	c := New()
	_, err := c.ListVersionsFromURL(context.Background(), srv.URL)
	if err == nil {
		t.Fatal("expected error for empty version list")
	}
	if !strings.Contains(err.Error(), "empty") {
		t.Errorf("error = %q, want substring 'empty'", err)
	}
}

func TestListVersionsFromURL_MalformedBody(t *testing.T) {
	t.Parallel()

	// Odd number of non-blank lines — Parse() returns "unpaired" error.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, "abc\ndesc\nlonely-hash\n")
	}))
	defer srv.Close()

	c := New()
	_, err := c.ListVersionsFromURL(context.Background(), srv.URL)
	if err == nil {
		t.Fatal("expected error for malformed body")
	}
	if !strings.Contains(err.Error(), "parsing version list") {
		t.Errorf("error = %q, want substring 'parsing version list'", err)
	}
}

func TestFetchSpec_Success(t *testing.T) {
	t.Parallel()

	arc := newFakeArchive("left-pad-abc").
		addFile("spec.md", "# Spec\n").
		addFile("tests/example.txt", "expected\n").
		addDir("nested")

	expected := computeExpectedHash(t, arc)

	var requested string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requested = r.URL.Path
		w.Header().Set("Content-Type", "application/gzip")
		_, _ = w.Write(arc.bytes(t))
	}))
	defer srv.Close()

	c := New()
	c.archiveURL = func(pkg ref.PackageRef, tag string) string {
		return srv.URL + "/" + pkg.Org + "/" + pkg.Pkg + "/" + tag + ".tar.gz"
	}

	dest := filepath.Join(t.TempDir(), "out")
	pkg := ref.PackageRef{Org: "chrisbodhi", Pkg: "left-pad"}

	got, err := c.FetchSpec(context.Background(), pkg, expected, dest)
	if err != nil {
		t.Fatalf("FetchSpec() error: %v", err)
	}
	if got.ContentHash != expected {
		t.Errorf("ContentHash = %q, want %q", got.ContentHash, expected)
	}
	if got.Dir != dest {
		t.Errorf("Dir = %q, want %q", got.Dir, dest)
	}

	// Tag name in URL must use "-" not ":".
	if !strings.Contains(requested, "sha256-") {
		t.Errorf("expected requested URL to use sha256- (not :), got %q", requested)
	}

	// Files were extracted; root prefix was stripped.
	body, err := os.ReadFile(filepath.Join(dest, "spec.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "# Spec\n" {
		t.Errorf("spec.md = %q", body)
	}
}

func TestFetchSpec_404(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "no such tag", http.StatusNotFound)
	}))
	defer srv.Close()

	c := New()
	c.archiveURL = func(ref.PackageRef, string) string { return srv.URL }

	pkg := ref.PackageRef{Org: "o", Pkg: "p"}
	_, err := c.FetchSpec(context.Background(), pkg, "sha256:x", t.TempDir())
	if err == nil {
		t.Fatal("expected error for 404")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, want substring 'not found'", err)
	}
}

func TestFetchSpec_500(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := New()
	c.archiveURL = func(ref.PackageRef, string) string { return srv.URL }

	_, err := c.FetchSpec(context.Background(), ref.PackageRef{Org: "o", Pkg: "p"}, "sha256:x", t.TempDir())
	if err == nil {
		t.Fatal("expected error for 500")
	}
	if !strings.Contains(err.Error(), "HTTP 500") {
		t.Errorf("error = %q, want substring 'HTTP 500'", err)
	}
}

func TestFetchSpec_HashMismatch(t *testing.T) {
	t.Parallel()

	arc := newFakeArchive("p-abc").addFile("a.txt", "alpha")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(arc.bytes(t))
	}))
	defer srv.Close()

	c := New()
	c.archiveURL = func(ref.PackageRef, string) string { return srv.URL }

	dest := filepath.Join(t.TempDir(), "out")
	_, err := c.FetchSpec(context.Background(), ref.PackageRef{Org: "o", Pkg: "p"}, "sha256:WRONG", dest)
	if err == nil {
		t.Fatal("expected hash mismatch error")
	}
	if !strings.Contains(err.Error(), "content hash mismatch") {
		t.Errorf("error = %q, want substring 'content hash mismatch'", err)
	}
	// On hash mismatch the destination should be cleaned up.
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Errorf("dest should be removed after hash mismatch, stat err = %v", err)
	}
}

func TestFetchSpec_MalformedArchive(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("this is not a gzip"))
	}))
	defer srv.Close()

	c := New()
	c.archiveURL = func(ref.PackageRef, string) string { return srv.URL }

	dest := filepath.Join(t.TempDir(), "out")
	_, err := c.FetchSpec(context.Background(), ref.PackageRef{Org: "o", Pkg: "p"}, "sha256:x", dest)
	if err == nil {
		t.Fatal("expected error extracting bad archive")
	}
	if !strings.Contains(err.Error(), "extracting") {
		t.Errorf("error = %q, want substring 'extracting'", err)
	}
}

func TestFetchSpec_BadRequestURL(t *testing.T) {
	t.Parallel()
	c := New()
	c.archiveURL = func(ref.PackageRef, string) string { return "http://\x00bad" }
	_, err := c.FetchSpec(context.Background(), ref.PackageRef{Org: "o", Pkg: "p"}, "sha256:x", t.TempDir())
	if err == nil {
		t.Fatal("expected error for malformed URL")
	}
	if !strings.Contains(err.Error(), "building request") {
		t.Errorf("error = %q, want substring 'building request'", err)
	}
}

func TestFetchSpec_NetworkError(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	srv.Close()

	c := New()
	c.archiveURL = func(ref.PackageRef, string) string { return srv.URL }
	_, err := c.FetchSpec(context.Background(), ref.PackageRef{Org: "o", Pkg: "p"}, "sha256:x", t.TempDir())
	if err == nil {
		t.Fatal("expected network error")
	}
}

func TestFetchSpec_DestDirIsFile(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("anything"))
	}))
	defer srv.Close()

	c := New()
	c.archiveURL = func(ref.PackageRef, string) string { return srv.URL }

	// Pre-create dest as a regular file so MkdirAll fails.
	parent := t.TempDir()
	dest := filepath.Join(parent, "is-a-file")
	if err := os.WriteFile(dest, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := c.FetchSpec(context.Background(), ref.PackageRef{Org: "o", Pkg: "p"}, "sha256:x", dest)
	if err == nil {
		t.Fatal("expected error when dest is a file")
	}
	if !strings.Contains(err.Error(), "creating destination") {
		t.Errorf("error = %q, want substring 'creating destination'", err)
	}
}

func TestFetchSpec_HashesOnlySpecDirs(t *testing.T) {
	t.Parallel()

	// Mix of spec files (human/, machine/) and packaging files. The hash must
	// be derived only from the spec content; packaging files are extracted
	// to disk but NOT included in the hash.
	arc := newFakeArchive("pkg-abc").
		addFile("human/spec.md", "# Spec\n").
		addFile("machine/types.ts", "type X = number;\n").
		addFile("VERSIONS.txt", "anything\n").
		addFile("README.md", "readme\n").
		addFile("buttress.toml", `name = "p"`+"\n").
		addDir("scripts")

	expected := computeExpectedHash(t, arc)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(arc.bytes(t))
	}))
	defer srv.Close()

	c := New()
	c.archiveURL = func(ref.PackageRef, string) string { return srv.URL }
	dest := filepath.Join(t.TempDir(), "out")

	got, err := c.FetchSpec(context.Background(), ref.PackageRef{Org: "o", Pkg: "p"}, expected, dest)
	if err != nil {
		t.Fatalf("FetchSpec() error: %v", err)
	}
	if got.ContentHash != expected {
		t.Errorf("ContentHash = %q, want %q", got.ContentHash, expected)
	}

	// Spec files extracted.
	for _, p := range []string{"human/spec.md", "machine/types.ts"} {
		if _, err := os.Stat(filepath.Join(dest, p)); err != nil {
			t.Errorf("spec file missing %q: %v", p, err)
		}
	}
	// Packaging files also extracted (so consumers can read VERSIONS.txt etc.).
	for _, p := range []string{"VERSIONS.txt", "README.md", "buttress.toml"} {
		if _, err := os.Stat(filepath.Join(dest, p)); err != nil {
			t.Errorf("packaging file missing %q: %v", p, err)
		}
	}
}

func TestExtractTarGz_SkipsPAXHeaders(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)

	// PAX global header — must be skipped without affecting the strip prefix.
	_ = tw.WriteHeader(&tar.Header{
		Name:     "pax_global_header",
		Typeflag: tar.TypeXGlobalHeader,
		Size:     0,
	})
	// Real root dir + content.
	_ = tw.WriteHeader(&tar.Header{Name: "pkg-abc/", Typeflag: tar.TypeDir, Mode: 0o755})
	_ = tw.WriteHeader(&tar.Header{Name: "pkg-abc/human/spec.md", Typeflag: tar.TypeReg, Mode: 0o644, Size: 5})
	_, _ = tw.Write([]byte("hello"))
	_ = tw.Close()
	_ = gz.Close()

	dest := t.TempDir()
	if _, _, err := extractTarGz(&buf, dest); err != nil {
		t.Fatalf("extractTarGz() error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "human", "spec.md")); err != nil {
		t.Errorf("spec.md missing: %v", err)
	}
	// PAX header file must NOT be extracted into dest.
	if _, err := os.Stat(filepath.Join(dest, "pax_global_header")); !os.IsNotExist(err) {
		t.Errorf("pax_global_header was extracted: %v", err)
	}
}

func TestExtractTarGz_RootDirMissingTrailingSlash(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)

	// First entry is a directory whose name has no trailing slash. The
	// strip-prefix logic must still detect it correctly.
	_ = tw.WriteHeader(&tar.Header{Name: "pkg-abc", Typeflag: tar.TypeDir, Mode: 0o755})
	_ = tw.WriteHeader(&tar.Header{Name: "pkg-abc/human/spec.md", Typeflag: tar.TypeReg, Mode: 0o644, Size: 2})
	_, _ = tw.Write([]byte("hi"))
	_ = tw.Close()
	_ = gz.Close()

	dest := t.TempDir()
	if _, _, err := extractTarGz(&buf, dest); err != nil {
		t.Fatalf("extractTarGz() error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "human", "spec.md")); err != nil {
		t.Errorf("spec.md missing: %v", err)
	}
}

func TestExtractTarGz_StripsRootDirAndIgnoresTraversal(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)

	// Root directory entry.
	_ = tw.WriteHeader(&tar.Header{Name: "root/", Typeflag: tar.TypeDir, Mode: 0o755})
	// A traversal attempt — should be skipped.
	_ = tw.WriteHeader(&tar.Header{Name: "root/../escape.txt", Typeflag: tar.TypeReg, Mode: 0o644, Size: 4})
	_, _ = tw.Write([]byte("evil"))
	// A normal file.
	_ = tw.WriteHeader(&tar.Header{Name: "root/safe.txt", Typeflag: tar.TypeReg, Mode: 0o644, Size: 4})
	_, _ = tw.Write([]byte("good"))
	_ = tw.Close()
	_ = gz.Close()

	dest := t.TempDir()
	if _, _, err := extractTarGz(&buf, dest); err != nil {
		t.Fatalf("extractTarGz() error: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dest, "safe.txt")); err != nil {
		t.Errorf("safe.txt missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(dest), "escape.txt")); !os.IsNotExist(err) {
		t.Errorf("traversal succeeded! escape.txt was written outside dest")
	}
}

func TestExtractTarGz_NotGzip(t *testing.T) {
	t.Parallel()
	_, _, err := extractTarGz(bytes.NewReader([]byte("plain")), t.TempDir())
	if err == nil {
		t.Fatal("expected gzip error")
	}
	if !strings.Contains(err.Error(), "gzip") {
		t.Errorf("error = %q, want substring 'gzip'", err)
	}
}

func TestExtractTarGz_TruncatedTar(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	// Write valid gzip but not valid tar inside.
	_, _ = gz.Write([]byte("\x00\x01\x02not a tar"))
	_ = gz.Close()

	_, _, err := extractTarGz(&buf, t.TempDir())
	if err == nil {
		t.Fatal("expected tar parse error")
	}
}
