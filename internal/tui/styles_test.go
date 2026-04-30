package tui

import "testing"

func TestShortHash(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{"sha256 long", "sha256:a1b2c3d4e5f6deadbeef", "sha256:a1b2c3d4"},
		{"sha256 exactly 8 chars", "sha256:a1b2c3d4", "sha256:a1b2c3d4"},
		{"sha256 short — under 8 chars", "sha256:abc", "sha256:abc"},
		{"no algorithm prefix, long", "abcdefghijklmnopqrst", "abcdefghijkl"},
		{"no algorithm prefix, exactly 12", "abcdefghijkl", "abcdefghijkl"},
		{"no algorithm prefix, short", "abc", "abc"},
		{"empty", "", ""},
		{"only colon", ":", ":"},
		{"trailing colon", "sha256:", "sha256:"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := ShortHash(tt.in); got != tt.want {
				t.Errorf("ShortHash(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
