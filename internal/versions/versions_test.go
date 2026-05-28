package versions

import (
	"strings"
	"testing"
)

func TestParseVersionsFile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    []Version
		wantErr string
	}{
		{
			name: "single entry",
			input: "abc123\n" +
				"Initial spec.\n",
			want: []Version{{Hash: "abc123", Description: "Initial spec."}},
		},
		{
			name: "multiple entries",
			input: "ghi789\n" +
				"Improve spec wording.\n" +
				"def456\n" +
				"Add tests.\n" +
				"abc123\n" +
				"Initial spec.\n",
			want: []Version{
				{Hash: "ghi789", Description: "Improve spec wording."},
				{Hash: "def456", Description: "Add tests."},
				{Hash: "abc123", Description: "Initial spec."},
			},
		},
		{
			name: "blank lines are skipped",
			input: "\n\n" +
				"abc123\n" +
				"\n" +
				"first description\n" +
				"\n" +
				"def456\n" +
				"second description\n",
			want: []Version{
				{Hash: "abc123", Description: "first description"},
				{Hash: "def456", Description: "second description"},
			},
		},
		{
			name: "comment lines are skipped",
			input: "# this is a header\n" +
				"abc123\n" +
				"# inline comment\n" +
				"first description\n",
			want: []Version{{Hash: "abc123", Description: "first description"}},
		},
		{
			name:  "carriage returns are trimmed",
			input: "abc123\r\nfirst description\r\n",
			want:  []Version{{Hash: "abc123", Description: "first description"}},
		},
		{
			name:  "empty input yields empty slice",
			input: "",
			want:  nil,
		},
		{
			name:  "only blank and comments yields empty slice",
			input: "\n\n# header only\n\n",
			want:  nil,
		},
		{
			name: "sha256 prefixed hash",
			input: "sha256:abcdef1234\n" +
				"First version.\n",
			want: []Version{{Hash: "sha256:abcdef1234", Description: "First version."}},
		},
		{
			name:    "unpaired final hash returns error",
			input:   "abc123\nfirst description\ndef456\n",
			wantErr: "unpaired",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := ParseVersionsFile(tt.input)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("ParseVersionsFile() = %+v, want error containing %q", got, tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("ParseVersionsFile() error = %q, want substring %q", err, tt.wantErr)
				}
				return
			}

			if err != nil {
				t.Fatalf("ParseVersionsFile() unexpected error: %v", err)
			}
			if !versionsEqual(got, tt.want) {
				t.Errorf("ParseVersionsFile() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

// TestParse_OrderPreserved verifies the parser preserves the input order, which
// downstream UIs depend on (most-recent first).
func TestParse_OrderPreserved(t *testing.T) {
	t.Parallel()

	in := "v3\nthird\nv2\nsecond\nv1\nfirst\n"
	got, err := ParseVersionsFile(in)
	if err != nil {
		t.Fatalf("ParseVersionsFile() error: %v", err)
	}

	wantOrder := []string{"v3", "v2", "v1"}
	if len(got) != len(wantOrder) {
		t.Fatalf("got %d versions, want %d", len(got), len(wantOrder))
	}
	for i, hash := range wantOrder {
		if got[i].Hash != hash {
			t.Errorf("got[%d].Hash = %q, want %q", i, got[i].Hash, hash)
		}
	}
}

// TestParse_ScannerError verifies that bufio.Scanner errors propagate.
// A line longer than 64KB exceeds the default scanner buffer.
func TestParse_ScannerError(t *testing.T) {
	t.Parallel()
	huge := strings.Repeat("a", 100*1024)
	_, err := ParseVersionsFile(huge + "\n")
	if err == nil {
		t.Fatal("expected scanner error for oversized line")
	}
	if !strings.Contains(err.Error(), "scanning") {
		t.Errorf("error = %q, want substring 'scanning'", err)
	}
}

func versionsEqual(a, b []Version) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
