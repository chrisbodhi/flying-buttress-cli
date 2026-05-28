// Package ref parses Flying Buttress package references.
//
// A package reference has the form:
//
//	@org/pkg
//	@org/pkg@gitref
package ref

import (
	"fmt"
	"strings"
)

// PackageRef identifies a spec package and an optional git ref.
type PackageRef struct {
	Org    string
	Pkg    string
	GitRef string // empty means "latest" (user will pick from version list)
}

// String returns the canonical string form of the reference, including the
// git ref if one is set (e.g. @org/pkg@abc123).
func (r PackageRef) String() string {
	s := fmt.Sprintf("@%s/%s", r.Org, r.Pkg)
	if r.GitRef != "" {
		s += "@" + r.GitRef
	}
	return s
}

// Name returns just the package identifier without any git ref (@org/pkg).
func (r PackageRef) Name() string {
	return fmt.Sprintf("@%s/%s", r.Org, r.Pkg)
}

// ParsePackageRef parses a package reference string.
// Accepted forms: @org/pkg or @org/pkg@sha
func ParsePackageRef(s string) (PackageRef, error) {
	if !strings.HasPrefix(s, "@") {
		return PackageRef{}, fmt.Errorf("package reference must start with @, got %q", s)
	}
	s = strings.TrimPrefix(s, "@")

	// Split off optional @gitref suffix (last @)
	var gitRef string
	if idx := strings.LastIndex(s, "@"); idx != -1 {
		gitRef = s[idx+1:]
		s = s[:idx]
	}

	parts := strings.SplitN(s, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return PackageRef{}, fmt.Errorf("package reference must be @org/pkg, got %q", "@"+s)
	}

	return PackageRef{
		Org:    parts[0],
		Pkg:    parts[1],
		GitRef: gitRef,
	}, nil
}
