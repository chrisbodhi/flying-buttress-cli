// Package github implements VersionLister and SpecFetcher backed by GitHub
// and raw HTTP (for gist-hosted version lists).
package github

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"buttress/internal/contenthash"
	"buttress/internal/generate"
	"buttress/internal/ref"
	"buttress/internal/versions"
)

const defaultTimeout = 30 * time.Second

// Client fetches spec data from GitHub.
type Client struct {
	http *http.Client
	// archiveURL builds the URL for a spec archive. If nil, the conventional
	// github.com URL is used. Tests may override this to point at a fake server.
	archiveURL func(pkg ref.PackageRef, tagName string) string
}

// New returns a new Client with a sensible default timeout.
func New() *Client {
	return &Client{
		http: &http.Client{Timeout: defaultTimeout},
	}
}

// ListVersionsFromURL fetches and parses a version list from any raw HTTP URL
// (GitHub repo file, gist raw URL, etc.).
func (c *Client) ListVersionsFromURL(ctx context.Context, url string) ([]versions.Version, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("building request for %s: %w", url, err)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching version list from %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetching version list from %s: HTTP %d", url, resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1 MiB max
	if err != nil {
		return nil, fmt.Errorf("reading version list from %s: %w", url, err)
	}

	vs, err := versions.Parse(string(body))
	if err != nil {
		return nil, fmt.Errorf("parsing version list from %s: %w", url, err)
	}
	if len(vs) == 0 {
		return nil, fmt.Errorf("version list at %s is empty", url)
	}

	return vs, nil
}

// VersionsURL returns the conventional URL for a package's version list.
// The file is expected at VERSIONS.txt in the default branch of the spec repo.
func VersionsURL(pkg ref.PackageRef) string {
	return fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/HEAD/VERSIONS.txt", pkg.Org, pkg.Pkg)
}

// FetchSpec downloads the spec archive for pkg at the given content hash
// (which is also the git tag name) into destDir.
// After extraction it verifies the computed content hash matches expectedHash.
// destDir is created if it does not exist.
func (c *Client) FetchSpec(ctx context.Context, pkg ref.PackageRef, expectedHash string, destDir string) (*generate.SpecArchive, error) {
	// Git tag names cannot contain colons, so "sha256:abc123" is stored in
	// the repo as "sha256-abc123". Convert at the point of URL construction;
	// the canonical colon form is preserved everywhere else (VERSIONS.txt, lock).
	tagName := strings.ReplaceAll(expectedHash, ":", "-")
	var fetchURL string
	if c.archiveURL != nil {
		fetchURL = c.archiveURL(pkg, tagName)
	} else {
		fetchURL = fmt.Sprintf("https://github.com/%s/%s/archive/refs/tags/%s.tar.gz",
			pkg.Org, pkg.Pkg, tagName)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fetchURL, nil)
	if err != nil {
		return nil, fmt.Errorf("building request for %s: %w", fetchURL, err)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching spec from %s: %w", fetchURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("spec %s@%s not found (HTTP 404 — has the tag been pushed?)", pkg.Name(), expectedHash)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetching spec %s@%s: HTTP %d", pkg.Name(), expectedHash, resp.StatusCode)
	}

	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating destination %s: %w", destDir, err)
	}

	computedHash, hashedFiles, err := extractTarGz(resp.Body, destDir)
	if err != nil {
		_ = os.RemoveAll(destDir)
		return nil, fmt.Errorf("extracting spec archive: %w", err)
	}

	// Verify integrity: computed hash must match the one from VERSIONS.txt.
	fullComputed := "sha256:" + computedHash
	if fullComputed != expectedHash {
		_ = os.RemoveAll(destDir)
		return nil, fmt.Errorf(
			"content hash mismatch for %s:\n  expected: %s\n  got:      %s\n\nfiles hashed from tarball:\n  %s",
			pkg.Name(), expectedHash, fullComputed,
			strings.Join(hashedFiles, "\n  "),
		)
	}

	return &generate.SpecArchive{
		Dir:         destDir,
		ContentHash: expectedHash,
	}, nil
}

// extractTarGz extracts a .tar.gz stream into dir and returns a SHA-256
// content hash computed over all file contents in sorted path order.
func extractTarGz(r io.Reader, dir string) (hash string, hashedFiles []string, err error) {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return "", nil, fmt.Errorf("opening gzip stream: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	hasher := sha256.New()

	// GitHub wraps everything in a top-level "{repo}-{ref}/" directory.
	// We strip that prefix so the spec lands directly in dir.
	var stripPrefix string

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", nil, fmt.Errorf("reading tar entry: %w", err)
		}

		// Skip PAX extended header entries — they appear before the real entries
		// and must not influence the strip prefix or be extracted as files.
		if hdr.Typeflag == tar.TypeXGlobalHeader || hdr.Typeflag == tar.TypeXHeader {
			continue
		}

		// Determine the strip prefix from the first entry.
		// TrimRight handles tarballs where the top-level dir entry omits the trailing slash.
		if stripPrefix == "" {
			parts := strings.SplitN(strings.TrimRight(filepath.ToSlash(hdr.Name), "/"), "/", 2)
			if len(parts) > 0 {
				stripPrefix = parts[0] + "/"
			}
		}

		relPath := strings.TrimPrefix(filepath.ToSlash(hdr.Name), stripPrefix)
		if relPath == "" || strings.Contains(relPath, "..") {
			continue // skip the root entry and any path traversal attempts
		}

		dest := filepath.Join(dir, filepath.FromSlash(relPath))

		if !contenthash.Included(relPath) {
			// Still extract the file; just don't include it in the hash.
			switch hdr.Typeflag {
			case tar.TypeDir:
				if err := os.MkdirAll(dest, 0o755); err != nil {
					return "", nil, err
				}
			case tar.TypeReg:
				if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
					return "", nil, err
				}
				f, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, hdr.FileInfo().Mode())
				if err != nil {
					return "", nil, err
				}
				_, copyErr := io.Copy(f, tr)
				f.Close()
				if copyErr != nil {
					return "", nil, copyErr
				}
			}
			continue
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(dest, 0o755); err != nil {
				return "", nil, err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
				return "", nil, err
			}
			f, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, hdr.FileInfo().Mode())
			if err != nil {
				return "", nil, err
			}
			hashedFiles = append(hashedFiles, relPath)
			fmt.Fprintf(hasher, "%s\n", relPath)
			if _, err := io.Copy(io.MultiWriter(f, hasher), tr); err != nil {
				f.Close()
				return "", nil, err
			}
			f.Close()
		}
	}

	return hex.EncodeToString(hasher.Sum(nil)), hashedFiles, nil
}
