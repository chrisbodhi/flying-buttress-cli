// Package versions defines the VersionLister interface and the plain-text
// parser for the Flying Buttress spec version list format.
//
// Format: alternating lines, no blank separators, most recent first:
//
//	ghi789
//	Improve spec wording for ambiguous usage.
//	def456
//	Include test for existing spacing.
//	abc123
//	Initial spec and tests.
package versions

import (
	"bufio"
	"context"
	"fmt"
	"strings"

	"buttress/internal/ref"
)

// Version represents one entry in a spec package's version history.
// Hash is the content hash of the spec at that version (e.g. "sha256:abc123…"),
// which also serves as the git tag name in the spec repository.
type Version struct {
	Hash        string
	Description string
}

// VersionLister lists available versions for a spec package.
type VersionLister interface {
	ListVersions(ctx context.Context, pkg ref.PackageRef) ([]Version, error)
}

// Parse parses the plain-text version list format from r.
// Lines are consumed in pairs: (gitref, description).
// Blank lines and lines beginning with # are ignored.
func Parse(text string) ([]Version, error) {
	var versions []Version
	scanner := bufio.NewScanner(strings.NewReader(text))

	var pending string
	lineNum := 0

	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r")

		// Skip blank lines and comments.
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		lineNum++
		if lineNum%2 == 1 {
			// Odd lines are content hashes (also git tag names).
			pending = line
		} else {
			// Even lines are descriptions.
			versions = append(versions, Version{
				Hash:        pending,
				Description: line,
			})
			pending = ""
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scanning version list: %w", err)
	}

	if pending != "" {
		return nil, fmt.Errorf("version list has an unpaired git ref %q (odd number of non-blank lines)", pending)
	}

	return versions, nil
}
