// Package store manages the local Flying Buttress spec cache.
//
// Layout:
//
//	~/.cache/buttress/packages/@org/pkg/<contentHash>/   ← global cache
//	<project>/buttress/@org/pkg/                         ← hard-linked project copy
//	<project>/buttress.lock                              ← pinned refs
package store

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Store manages spec packages on disk.
type Store struct {
	cacheDir   string // e.g. ~/.cache/buttress
	projectDir string // project root (where buttress.lock lives)
}

// New creates a Store.
// cacheDir defaults to ~/.cache/buttress if empty.
// projectDir defaults to the current working directory if empty.
func New(cacheDir, projectDir string) (*Store, error) {
	if cacheDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("cannot determine home directory: %w", err)
		}
		cacheDir = filepath.Join(home, ".cache", "buttress")
	}
	if projectDir == "" {
		wd, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("cannot determine working directory: %w", err)
		}
		projectDir = wd
	}
	return &Store{cacheDir: cacheDir, projectDir: projectDir}, nil
}

// cachePath returns the cache directory for a specific content hash.
func (s *Store) cachePath(org, pkg, contentHash string) string {
	return filepath.Join(s.cacheDir, "packages", "@"+org, pkg, contentHash)
}

// projectPath returns the project-local directory for a package.
func (s *Store) projectPath(org, pkg string) string {
	return filepath.Join(s.projectDir, "buttress", "@"+org, pkg)
}

// Has reports whether the given content hash is already in the global cache.
func (s *Store) Has(org, pkg, contentHash string) bool {
	_, err := os.Stat(s.cachePath(org, pkg, contentHash))
	return err == nil
}

// Commit moves srcDir (a freshly extracted spec directory) into the global
// cache under contentHash and hard-links it into the project directory.
// If the content hash is already cached, srcDir is removed and the cached
// copy is linked into the project instead.
func (s *Store) Commit(org, pkg, contentHash, srcDir string) (string, error) {
	dest := s.cachePath(org, pkg, contentHash)

	if !s.Has(org, pkg, contentHash) {
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return "", fmt.Errorf("creating cache directory: %w", err)
		}
		if err := os.Rename(srcDir, dest); err != nil {
			// Rename may fail across filesystems; fall back to copy.
			if err2 := copyDir(srcDir, dest); err2 != nil {
				return "", fmt.Errorf("moving spec to cache: %w", err2)
			}
			_ = os.RemoveAll(srcDir)
		}
	} else {
		_ = os.RemoveAll(srcDir)
	}

	projectDest := s.projectPath(org, pkg)
	if err := linkDir(dest, projectDest); err != nil {
		return "", fmt.Errorf("linking spec into project: %w", err)
	}

	return dest, nil
}

// LockEntry represents one entry in buttress.lock.
// Hash is the content hash of the spec (e.g. "sha256:abc123…"), which also
// serves as the git tag in the spec repository.
type LockEntry struct {
	Org         string    `json:"org"`
	Pkg         string    `json:"pkg"`
	Hash        string    `json:"hash"`
	InstalledAt time.Time `json:"installed_at"`
}

// lockfilePath returns the path to buttress.lock.
func (s *Store) lockfilePath() string {
	return filepath.Join(s.projectDir, "buttress.lock")
}

// ReadLock reads the current lock file. Returns an empty map if it doesn't exist.
func (s *Store) ReadLock() (map[string]LockEntry, error) {
	data, err := os.ReadFile(s.lockfilePath())
	if os.IsNotExist(err) {
		return make(map[string]LockEntry), nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading buttress.lock: %w", err)
	}

	var entries map[string]LockEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("parsing buttress.lock: %w", err)
	}
	return entries, nil
}

// WriteLock atomically writes the lock file.
func (s *Store) WriteLock(entries map[string]LockEntry) error {
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return fmt.Errorf("marshalling buttress.lock: %w", err)
	}
	data = append(data, '\n')

	tmp := s.lockfilePath() + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("writing buttress.lock: %w", err)
	}
	return os.Rename(tmp, s.lockfilePath())
}

// LockKey returns the map key for a package entry.
func LockKey(org, pkg string) string {
	return "@" + org + "/" + pkg
}

// linkDir creates projectDest as a copy of cacheDir using hard links where
// possible (same filesystem), falling back to a regular copy.
func linkDir(cacheDir, projectDest string) error {
	// Remove stale project copy.
	if err := os.RemoveAll(projectDest); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(projectDest), 0o755); err != nil {
		return err
	}

	return filepath.Walk(cacheDir, func(src string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(cacheDir, src)
		dst := filepath.Join(projectDest, rel)

		if info.IsDir() {
			return os.MkdirAll(dst, 0o755)
		}

		// Try hard link first.
		if linkErr := os.Link(src, dst); linkErr == nil {
			return nil
		}
		// Fall back to copy.
		return copyFile(src, dst, info.Mode())
	})
}

// copyDir recursively copies src to dst.
func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		return copyFile(path, target, info.Mode())
	})
}

func copyFile(src, dst string, mode os.FileMode) error {
	// Prevent path traversal (belt-and-suspenders; already filtered in extraction).
	if strings.Contains(dst, "..") {
		return fmt.Errorf("refusing to write to path containing '..': %s", dst)
	}

	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}
