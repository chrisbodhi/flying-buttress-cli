package generate

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeChatServer is a configurable httptest.Server that mimics an OpenAI-compatible
// chat completions endpoint. Each Generate() call iterates through replies in order.
type fakeChatServer struct {
	t       *testing.T
	server  *httptest.Server
	replies []string // contents to return on the i-th call
	status  int      // HTTP status; 0 → 200
	errMsg  string   // body-level error.message; empty → none
	reqs    []chatRequest
}

func newFakeChatServer(t *testing.T) *fakeChatServer {
	t.Helper()
	f := &fakeChatServer{t: t}
	f.server = httptest.NewServer(http.HandlerFunc(f.handle))
	t.Cleanup(f.server.Close)
	return f
}

func (f *fakeChatServer) URL() string { return f.server.URL }

func (f *fakeChatServer) handle(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	var req chatRequest
	if err := json.Unmarshal(body, &req); err == nil {
		f.reqs = append(f.reqs, req)
	}

	if f.status != 0 && f.status != http.StatusOK {
		w.WriteHeader(f.status)
		fmt.Fprintf(w, `{"error":{"message":%q}}`, "server boom")
		return
	}
	if f.errMsg != "" {
		fmt.Fprintf(w, `{"error":{"message":%q}}`, f.errMsg)
		return
	}

	idx := len(f.reqs) - 1
	if idx >= len(f.replies) {
		idx = len(f.replies) - 1
	}
	if idx < 0 {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":{"message":"no replies configured"}}`))
		return
	}
	resp := chatResponse{Choices: []struct {
		Message chatMessage `json:"message"`
	}{
		{Message: chatMessage{Role: "assistant", Content: f.replies[idx]}},
	}}
	_ = json.NewEncoder(w).Encode(resp)
}

// writeMinimalSpec creates a spec dir with a machine/types.ts file.
func writeMinimalSpec(t *testing.T) *SpecArchive {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "machine"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "machine", "types.ts"),
		[]byte("export type X = number;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return &SpecArchive{Dir: dir, ContentHash: "sha256:test"}
}

func TestStripCodeFences(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, in, want string
	}{
		{"plain", "code here", "code here\n"},
		{"fenced no lang", "```\ncode\n```", "code\n"},
		{"fenced with lang", "```typescript\ncode\n```", "code\n"},
		{"fenced trailing whitespace", "  ```ts\nbody\n```\n", "body\n"},
		{"empty", "", "\n"},
		{"only fences", "```\n```", "\n"},
		{"trailing prose", "```ts\nimpl\n```\nthen prose", "impl\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := stripCodeFences(tt.in); got != tt.want {
				t.Errorf("stripCodeFences(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestIsTextFile(t *testing.T) {
	t.Parallel()
	tests := []struct {
		path string
		want bool
	}{
		{"spec.md", true},
		{"types.ts", true},
		{"img.png", false},
		{"img.JPG", false}, // case insensitive
		{"a.zip", false},
		{"a.tar", false},
		{"binary", true}, // unknown ext defaults to text
		{"", true},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			t.Parallel()
			if got := isTextFile(tt.path); got != tt.want {
				t.Errorf("isTextFile(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestLLMGenerator_BaseURL(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		cfg  LLMConfig
		want string
	}{
		{"explicit", LLMConfig{BaseURL: "https://my.host/api"}, "https://my.host/api"},
		{"explicit trims trailing slash", LLMConfig{BaseURL: "https://my.host/"}, "https://my.host"},
		{"openai default", LLMConfig{Provider: "openai"}, "https://api.openai.com"},
		{"OpenAI uppercase", LLMConfig{Provider: "OpenAI"}, "https://api.openai.com"},
		{"lmstudio default", LLMConfig{Provider: "lmstudio"}, "http://localhost:1234"},
		{"unknown provider falls back to ollama", LLMConfig{Provider: "anything"}, "http://localhost:11434"},
		{"empty provider falls back to ollama", LLMConfig{}, "http://localhost:11434"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			g := NewLLMGenerator(tt.cfg)
			if got := g.baseURL(); got != tt.want {
				t.Errorf("baseURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLLMGenerator_Generate_HappyPath(t *testing.T) {
	t.Parallel()

	srv := newFakeChatServer(t)
	srv.replies = []string{"// generated\nexport default 1;\n"}

	spec := writeMinimalSpec(t)
	out := filepath.Join(t.TempDir(), "impl.go")

	g := NewLLMGenerator(LLMConfig{
		BaseURL: srv.URL(),
		Model:   "test-model",
		APIKey:  "test-key",
	})
	err := g.Generate(context.Background(), Request{
		Spec:        spec,
		Language:    "go", // go has no verifier — single attempt suffices
		OutputPath:  out,
		PackageName: "@org/pkg",
		ProjectDir:  t.TempDir(),
	})
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}

	body, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("output not written: %v", err)
	}
	if !strings.Contains(string(body), "export default 1") {
		t.Errorf("output = %q, want generated content", body)
	}
	if len(srv.reqs) != 1 {
		t.Errorf("got %d requests, want 1 (no verification → no retries)", len(srv.reqs))
	}
	if srv.reqs[0].Model != "test-model" {
		t.Errorf("model = %q, want test-model", srv.reqs[0].Model)
	}
}

func TestLLMGenerator_Generate_DefaultMaxAttempts(t *testing.T) {
	t.Parallel()
	srv := newFakeChatServer(t)
	srv.replies = []string{"impl"}
	g := NewLLMGenerator(LLMConfig{BaseURL: srv.URL(), Model: "m"})

	// Use a language without verification so a single attempt always succeeds;
	// we just want to confirm MaxAttempts=0 doesn't loop forever or panic.
	err := g.Generate(context.Background(), Request{
		Spec:       writeMinimalSpec(t),
		Language:   "go",
		OutputPath: filepath.Join(t.TempDir(), "out.go"),
		ProjectDir: t.TempDir(),
		// MaxAttempts intentionally unset
	})
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
}

func TestLLMGenerator_Generate_StripsCodeFences(t *testing.T) {
	t.Parallel()
	srv := newFakeChatServer(t)
	srv.replies = []string{"```typescript\nconst x = 1;\n```"}

	out := filepath.Join(t.TempDir(), "out.ts")
	g := NewLLMGenerator(LLMConfig{BaseURL: srv.URL(), Model: "m"})
	// Use language "go" to skip TS verification.
	err := g.Generate(context.Background(), Request{
		Spec:       writeMinimalSpec(t),
		Language:   "go",
		OutputPath: out,
		ProjectDir: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	body, _ := os.ReadFile(out)
	if strings.Contains(string(body), "```") {
		t.Errorf("output contains code fences: %q", body)
	}
	if !strings.Contains(string(body), "const x = 1;") {
		t.Errorf("output missing code: %q", body)
	}
}

func TestLLMGenerator_Generate_LLMError(t *testing.T) {
	t.Parallel()
	srv := newFakeChatServer(t)
	srv.errMsg = "model not found"

	g := NewLLMGenerator(LLMConfig{BaseURL: srv.URL(), Model: "m"})
	err := g.Generate(context.Background(), Request{
		Spec:       writeMinimalSpec(t),
		Language:   "go",
		OutputPath: filepath.Join(t.TempDir(), "out.go"),
		ProjectDir: t.TempDir(),
	})
	if err == nil {
		t.Fatal("expected error from LLM")
	}
	if !strings.Contains(err.Error(), "model not found") {
		t.Errorf("error = %q, want substring 'model not found'", err)
	}
}

func TestLLMGenerator_Generate_NetworkError(t *testing.T) {
	t.Parallel()
	srv := newFakeChatServer(t)
	srv.server.Close() // make connections fail

	g := NewLLMGenerator(LLMConfig{BaseURL: srv.URL(), Model: "m"})
	err := g.Generate(context.Background(), Request{
		Spec:       writeMinimalSpec(t),
		Language:   "go",
		OutputPath: filepath.Join(t.TempDir(), "out.go"),
		ProjectDir: t.TempDir(),
	})
	if err == nil {
		t.Fatal("expected network error")
	}
	if !strings.Contains(err.Error(), "calling LLM") {
		t.Errorf("error = %q, want substring 'calling LLM'", err)
	}
}

func TestLLMGenerator_Generate_BadResponseJSON(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("not json"))
	}))
	defer srv.Close()

	g := NewLLMGenerator(LLMConfig{BaseURL: srv.URL, Model: "m"})
	err := g.Generate(context.Background(), Request{
		Spec:       writeMinimalSpec(t),
		Language:   "go",
		OutputPath: filepath.Join(t.TempDir(), "out.go"),
		ProjectDir: t.TempDir(),
	})
	if err == nil {
		t.Fatal("expected parse error")
	}
	if !strings.Contains(err.Error(), "parsing LLM response") {
		t.Errorf("error = %q, want substring 'parsing LLM response'", err)
	}
}

func TestLLMGenerator_Generate_NoChoices(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[]}`))
	}))
	defer srv.Close()

	g := NewLLMGenerator(LLMConfig{BaseURL: srv.URL, Model: "m"})
	err := g.Generate(context.Background(), Request{
		Spec:       writeMinimalSpec(t),
		Language:   "go",
		OutputPath: filepath.Join(t.TempDir(), "out.go"),
		ProjectDir: t.TempDir(),
	})
	if err == nil {
		t.Fatal("expected error for no choices")
	}
	if !strings.Contains(err.Error(), "no choices") {
		t.Errorf("error = %q, want substring 'no choices'", err)
	}
}

func TestLLMGenerator_Generate_HTTPError(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// Status 500 with no JSON-parseable error body.
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	g := NewLLMGenerator(LLMConfig{BaseURL: srv.URL, Model: "m"})
	err := g.Generate(context.Background(), Request{
		Spec:       writeMinimalSpec(t),
		Language:   "go",
		OutputPath: filepath.Join(t.TempDir(), "out.go"),
		ProjectDir: t.TempDir(),
	})
	if err == nil {
		t.Fatal("expected HTTP 500 error")
	}
	if !strings.Contains(err.Error(), "HTTP 500") {
		t.Errorf("error = %q, want substring 'HTTP 500'", err)
	}
}

func TestLLMGenerator_Generate_InvalidOutputDir(t *testing.T) {
	t.Parallel()
	srv := newFakeChatServer(t)
	srv.replies = []string{"impl"}

	g := NewLLMGenerator(LLMConfig{BaseURL: srv.URL(), Model: "m"})
	// Use a path whose parent contains a regular file masquerading as a dir.
	parent := t.TempDir()
	notADir := filepath.Join(parent, "notadir")
	if err := os.WriteFile(notADir, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(notADir, "out.go")

	err := g.Generate(context.Background(), Request{
		Spec:       writeMinimalSpec(t),
		Language:   "go",
		OutputPath: out,
		ProjectDir: t.TempDir(),
	})
	if err == nil {
		t.Fatal("expected mkdir error")
	}
	if !strings.Contains(err.Error(), "creating output directory") {
		t.Errorf("error = %q, want substring 'creating output directory'", err)
	}
}

func TestLLMGenerator_AuthHeader(t *testing.T) {
	t.Parallel()
	var seenAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"impl"}}]}`))
	}))
	defer srv.Close()

	g := NewLLMGenerator(LLMConfig{BaseURL: srv.URL, Model: "m", APIKey: "secret"})
	err := g.Generate(context.Background(), Request{
		Spec:       writeMinimalSpec(t),
		Language:   "go",
		OutputPath: filepath.Join(t.TempDir(), "out.go"),
		ProjectDir: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	if seenAuth != "Bearer secret" {
		t.Errorf("Authorization = %q, want 'Bearer secret'", seenAuth)
	}
}

func TestLLMGenerator_NoAuthHeaderWhenNoKey(t *testing.T) {
	t.Parallel()
	var seenAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"impl"}}]}`))
	}))
	defer srv.Close()

	g := NewLLMGenerator(LLMConfig{BaseURL: srv.URL, Model: "m"})
	err := g.Generate(context.Background(), Request{
		Spec:       writeMinimalSpec(t),
		Language:   "go",
		OutputPath: filepath.Join(t.TempDir(), "out.go"),
		ProjectDir: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	if seenAuth != "" {
		t.Errorf("Authorization should not be set when APIKey empty, got %q", seenAuth)
	}
}

func TestLLMGenerator_BadRequestURL(t *testing.T) {
	t.Parallel()
	g := NewLLMGenerator(LLMConfig{BaseURL: "http://\x00bad", Model: "m"})
	err := g.Generate(context.Background(), Request{
		Spec:       writeMinimalSpec(t),
		Language:   "go",
		OutputPath: filepath.Join(t.TempDir(), "out.go"),
		ProjectDir: t.TempDir(),
	})
	if err == nil {
		t.Fatal("expected error for malformed URL")
	}
	if !strings.Contains(err.Error(), "building LLM request") {
		t.Errorf("error = %q, want substring 'building LLM request'", err)
	}
}

func TestVerifyOutput_UnknownLanguage(t *testing.T) {
	t.Parallel()
	// Languages other than ts/typescript skip verification entirely.
	sections, err := verifyOutput(context.Background(), Request{Language: "go"}, nil)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if len(sections) != 0 {
		t.Errorf("expected no sections, got %v", sections)
	}
}
