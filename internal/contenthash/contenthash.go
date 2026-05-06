// Package contenthash computes the canonical content hash for a spec directory.
//
// The hash covers only files under human/ and machine/ — the actual spec content.
// Everything else (VERSIONS.txt, buttress.toml, README, .gitignore, tsconfig.json,
// etc.) is packaging or tooling. This keeps the hash stable and avoids the circular
// dependency of hashing files that reference the hash.
// Files are walked in lexical order so the hash is stable across platforms and
// identical to what the tarball verifier computes.
package contenthash

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// specDirs are the only directories whose contents are included in the
// content hash. Everything else is packaging or tooling — not spec.
var specDirs = []string{"human/", "machine/"}

// Included reports whether relPath should be part of the content hash.
func Included(relPath string) bool {
	for _, dir := range specDirs {
		if strings.HasPrefix(relPath, dir) {
			return true
		}
	}
	return false
}

// HashDir computes the sha256 content hash of dir and returns it as
// "sha256:<hex>". This is the canonical hash used in VERSIONS.txt and
// buttress.lock, and must match what the tarball verifier computes.
func HashDir(dir string) (string, error) {
	hash, _, err := HashDirVerbose(dir)
	return hash, err
}

// HashDirVerbose is like HashDir but also returns the ordered list of files
// that were included in the hash, for debugging.
func HashDirVerbose(dir string) (hash string, included []string, err error) {
	hasher := sha256.New()

	for _, specDir := range specDirs {
		root := filepath.Join(dir, specDir)
		if _, err := os.Stat(root); os.IsNotExist(err) {
			continue
		}
		err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			rel, err := filepath.Rel(dir, path)
			if err != nil {
				return err
			}
			rel = filepath.ToSlash(rel)
			included = append(included, rel)
			fmt.Fprintf(hasher, "%s\n", rel)
			f, err := os.Open(path)
			if err != nil {
				return fmt.Errorf("opening %s: %w", rel, err)
			}
			_, copyErr := io.Copy(hasher, f)
			f.Close()
			if copyErr != nil {
				return fmt.Errorf("hashing %s: %w", rel, copyErr)
			}
			return nil
		})
		if err != nil {
			return "", nil, err
		}
	}

	return "sha256:" + hex.EncodeToString(hasher.Sum(nil)), included, nil
}
