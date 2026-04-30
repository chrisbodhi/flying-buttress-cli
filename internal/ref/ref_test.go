package ref

import (
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    PackageRef
		wantErr string // substring; empty = no error
	}{
		{
			name:  "bare org/pkg",
			input: "@chrisbodhi/left-pad",
			want:  PackageRef{Org: "chrisbodhi", Pkg: "left-pad"},
		},
		{
			name:  "with sha256 hash",
			input: "@chrisbodhi/left-pad@sha256:abc123",
			want:  PackageRef{Org: "chrisbodhi", Pkg: "left-pad", GitRef: "sha256:abc123"},
		},
		{
			name:  "with plain git tag",
			input: "@org/pkg@v1.2.3",
			want:  PackageRef{Org: "org", Pkg: "pkg", GitRef: "v1.2.3"},
		},
		{
			name:  "package name with dots",
			input: "@org/some.pkg",
			want:  PackageRef{Org: "org", Pkg: "some.pkg"},
		},
		{
			name:  "package name with hyphens",
			input: "@my-org/my-pkg-name",
			want:  PackageRef{Org: "my-org", Pkg: "my-pkg-name"},
		},
		{
			name:  "hash containing slashes still parses",
			input: "@org/pkg@refs/heads/main",
			// LastIndex of @ splits at the last @, leaving the rest as a "/"-containing pkg.
			// This documents current behaviour — the parser splits org/pkg before the last @.
			want: PackageRef{Org: "org", Pkg: "pkg", GitRef: "refs/heads/main"},
		},
		{
			name:    "missing leading @",
			input:   "org/pkg",
			wantErr: "must start with @",
		},
		{
			name:    "missing slash",
			input:   "@orgonly",
			wantErr: "must be @org/pkg",
		},
		{
			name:    "empty org",
			input:   "@/pkg",
			wantErr: "must be @org/pkg",
		},
		{
			name:    "empty pkg",
			input:   "@org/",
			wantErr: "must be @org/pkg",
		},
		{
			name:    "just an @",
			input:   "@",
			wantErr: "must be @org/pkg",
		},
		{
			name:    "empty string",
			input:   "",
			wantErr: "must start with @",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := Parse(tt.input)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("Parse(%q) = %+v, want error containing %q", tt.input, got, tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Parse(%q) error = %q, want substring %q", tt.input, err, tt.wantErr)
				}
				return
			}

			if err != nil {
				t.Fatalf("Parse(%q) unexpected error: %v", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("Parse(%q) = %+v, want %+v", tt.input, got, tt.want)
			}
		})
	}
}

func TestPackageRef_String(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		ref  PackageRef
		want string
	}{
		{
			name: "without git ref",
			ref:  PackageRef{Org: "org", Pkg: "pkg"},
			want: "@org/pkg",
		},
		{
			name: "with git ref",
			ref:  PackageRef{Org: "org", Pkg: "pkg", GitRef: "sha256:abc"},
			want: "@org/pkg@sha256:abc",
		},
		{
			name: "empty ref",
			ref:  PackageRef{},
			want: "@/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.ref.String(); got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPackageRef_Name(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		ref  PackageRef
		want string
	}{
		{"no git ref", PackageRef{Org: "a", Pkg: "b"}, "@a/b"},
		{"git ref is omitted", PackageRef{Org: "a", Pkg: "b", GitRef: "sha256:x"}, "@a/b"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.ref.Name(); got != tt.want {
				t.Errorf("Name() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestParseRoundTrip verifies that String() produces output that round-trips
// back through Parse() — a key invariant for any user-visible serialization.
func TestParseRoundTrip(t *testing.T) {
	t.Parallel()

	inputs := []string{
		"@org/pkg",
		"@org/pkg@sha256:abc123",
		"@my-org/my-pkg@v1.0.0",
	}

	for _, in := range inputs {
		t.Run(in, func(t *testing.T) {
			t.Parallel()
			parsed, err := Parse(in)
			if err != nil {
				t.Fatalf("Parse(%q) failed: %v", in, err)
			}
			if got := parsed.String(); got != in {
				t.Errorf("round-trip: Parse(%q).String() = %q", in, got)
			}
		})
	}
}
