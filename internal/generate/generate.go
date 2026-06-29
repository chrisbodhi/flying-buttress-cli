// Package generate implements LLM-based code generation from a spec archive.
package generate

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"buttress/internal/specmeta"
)

// ErrNotConfigured is returned when generation is requested but the config
// contains no LLM credentials.
var ErrNotConfigured = errors.New("no LLM configured: set [llm] provider and model in ~/.config/buttress/config.toml")

// SpecArchive is a local directory tree containing a downloaded spec package.
type SpecArchive struct {
	// Dir is the absolute path to the extracted spec directory.
	Dir string
	// ContentHash is the content hash computed over the spec files.
	ContentHash string
}

// Request holds everything needed to run a generation.
type Request struct {
	Spec        *SpecArchive
	Language    string
	OutputPath  string           // absolute path where the generated file should be written
	PackageName string           // "@org/pkg" — used in the prompt and progress output
	ProjectDir  string           // working directory for verification commands
	MaxAttempts int              // 0 → default (3)
	Progress    func(msg string) // optional; called with status messages during generation
}

func (r *Request) progress(format string, args ...any) {
	if r.Progress != nil {
		r.Progress(fmt.Sprintf(format, args...))
	}
}

// Generator generates implementation source files from a spec archive.
type Generator interface {
	Generate(ctx context.Context, req Request) error
}

// Stub is a Generator that always returns ErrNotConfigured.
type Stub struct{}

func (Stub) Generate(_ context.Context, _ Request) error {
	return ErrNotConfigured
}

// LLMConfig holds the provider settings consumed by LLMGenerator.
type LLMConfig struct {
	Provider string
	BaseURL  string
	APIKey   string
	Model    string
}

// LLMGenerator calls an OpenAI-compatible chat completions endpoint.
type LLMGenerator struct {
	cfg       LLMConfig
	client    *http.Client
	writeFile func(string, []byte, os.FileMode) error
}

// NewLLMGenerator returns a generator backed by the given LLM config.
func NewLLMGenerator(cfg LLMConfig) *LLMGenerator {
	return &LLMGenerator{
		cfg:       cfg,
		client:    &http.Client{Timeout: 5 * time.Minute},
		writeFile: os.WriteFile,
	}
}

const defaultMaxAttempts = 3

// Generate runs the generate → verify → fix loop.
//
// The spec files (types, tests, docs) are sent once in the system message.
// Each fix iteration appends only the generated code and error output as new
// chat turns, keeping the growing context bounded.
func (g *LLMGenerator) Generate(ctx context.Context, req Request) error {
	if req.MaxAttempts == 0 {
		req.MaxAttempts = defaultMaxAttempts
	}

	meta, err := specmeta.Load(req.Spec.Dir)
	if err != nil {
		return fmt.Errorf("loading spec metadata: %w", err)
	}

	systemPrompt, err := buildSystemPrompt(req, meta)
	if err != nil {
		return fmt.Errorf("building system prompt: %w", err)
	}

	messages := []chatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: fmt.Sprintf("Generate the %s implementation.", req.Language)},
	}

	var lastErrSections []string
	for attempt := 1; attempt <= req.MaxAttempts; attempt++ {
		req.progress("attempt %d/%d: calling LLM…", attempt, req.MaxAttempts)
		code, err := g.chat(ctx, messages)
		if err != nil {
			return err
		}
		code = extractCode(code)

		if err := os.MkdirAll(filepath.Dir(req.OutputPath), 0o755); err != nil {
			return fmt.Errorf("creating output directory: %w", err)
		}
		if err := g.writeFile(req.OutputPath, []byte(code), 0o644); err != nil {
			return fmt.Errorf("writing generated file: %w", err)
		}

		req.progress("attempt %d/%d: verifying…", attempt, req.MaxAttempts)
		errSections, err := verifyOutput(ctx, req, meta)
		if err != nil {
			return fmt.Errorf("running verification: %w", err)
		}
		if len(errSections) == 0 {
			return nil // passes
		}

		lastErrSections = errSections
		if attempt < req.MaxAttempts {
			req.progress("attempt %d/%d: verification failed, retrying…\n%s", attempt, req.MaxAttempts, strings.Join(errSections, "\n\n"))
			// Append the failed attempt and the errors as a new conversation turn.
			// The spec files remain in the system message — not re-sent.
			messages = append(messages,
				chatMessage{Role: "assistant", Content: code},
				chatMessage{Role: "user", Content: buildFixMessage(errSections)},
			)
		}
	}

	return fmt.Errorf("generation did not pass verification after %d attempt(s):\n%s",
		req.MaxAttempts, strings.Join(lastErrSections, "\n\n"))
}

// verifyOutput dispatches to language-specific verification.
// Returns nil (no error, no sections) if the language has no verification support yet.
func verifyOutput(ctx context.Context, req Request, meta *specmeta.SpecMeta) ([]string, error) {
	switch strings.ToLower(req.Language) {
	case "typescript", "ts":
		return verifyTypeScript(ctx, req, meta)
	default:
		return nil, nil
	}
}

// --- LLM transport ---

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
}

type chatResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (g *LLMGenerator) chat(ctx context.Context, messages []chatMessage) (string, error) {
	body, err := json.Marshal(chatRequest{
		Model:    g.cfg.Model,
		Messages: messages,
	})
	if err != nil {
		return "", fmt.Errorf("marshalling LLM request: %w", err)
	}

	url := g.baseURL() + "/v1/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("building LLM request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if g.cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+g.cfg.APIKey)
	}

	resp, err := g.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("calling LLM at %s: %w", url, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return "", fmt.Errorf("reading LLM response: %w", err)
	}

	var result chatResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("parsing LLM response: %w", err)
	}
	if result.Error != nil {
		return "", fmt.Errorf("LLM error: %s", result.Error.Message)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("LLM returned HTTP %d", resp.StatusCode)
	}
	if len(result.Choices) == 0 {
		return "", fmt.Errorf("LLM returned no choices")
	}
	return result.Choices[0].Message.Content, nil
}

func (g *LLMGenerator) baseURL() string {
	if g.cfg.BaseURL != "" {
		return strings.TrimRight(g.cfg.BaseURL, "/")
	}
	switch strings.ToLower(g.cfg.Provider) {
	case "openai":
		return "https://api.openai.com"
	case "lmstudio":
		return "http://localhost:1234"
	default:
		return "http://localhost:11434"
	}
}

// --- Helpers shared by prompt.go and verify_ts.go ---

// isTextFile reports whether the file should be included in prompts.
func isTextFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	skip := map[string]bool{
		".png": true, ".jpg": true, ".jpeg": true, ".gif": true,
		".ico": true, ".pdf": true, ".zip": true, ".tar": true, ".gz": true,
		".exe": true, ".bin": true, ".wasm": true,
	}
	return !skip[ext]
}

// extractCode extracts the implementation from an LLM response.
//
// Real-world output patterns handled:
//  1. Clean code   — no fences, just source (returned trimmed).
//  2. Single fence — entire response wrapped in ```lang … ``` (fences stripped).
//  3. CoT + fence  — prose or a draft followed by a fenced final answer;
//     the LAST complete code block is returned because chain-of-thought
//     models revise their output and put the correct version last.
//  4. Think tags   — <think>…</think> sections are removed before extraction.
func extractCode(s string) string {
	s = strings.TrimSpace(s)
	s = stripThinkBlocks(s)
	s = strings.TrimSpace(s)
	if block, ok := lastCodeBlock(s); ok {
		return block + "\n"
	}
	return s + "\n"
}

// stripThinkBlocks removes <think>…</think> sections emitted by reasoning
// models. Unclosed tags are stripped from the open tag to end of string.
func stripThinkBlocks(s string) string {
	const open, close = "<think>", "</think>"
	for {
		lo := strings.ToLower(s)
		start := strings.Index(lo, open)
		if start == -1 {
			break
		}
		end := strings.Index(lo[start:], close)
		if end == -1 {
			s = strings.TrimSpace(s[:start])
			break
		}
		end += start + len(close)
		s = s[:start] + s[end:]
	}
	return s
}

// lastCodeBlock returns the content of the last complete code fence block in s
// and true. If the final open fence has no matching close (truncated response),
// the content from that fence to end-of-string is returned with true.
// Returns ("", false) when no code fence is found at all.
func lastCodeBlock(s string) (string, bool) {
	lines := strings.Split(s, "\n")

	type span struct{ start, end int }
	var complete []span
	inFence := false
	fenceStart := 0

	for i, line := range lines {
		t := strings.TrimSpace(line)
		if !inFence && strings.HasPrefix(t, "```") {
			inFence = true
			fenceStart = i
		} else if inFence && t == "```" {
			complete = append(complete, span{fenceStart, i})
			inFence = false
		}
	}

	if inFence {
		// Unclosed block — take everything after the opening fence.
		inner := lines[fenceStart+1:]
		return strings.TrimRight(strings.Join(inner, "\n"), "\n"), true
	}
	if len(complete) == 0 {
		return "", false
	}
	last := complete[len(complete)-1]
	inner := lines[last.start+1 : last.end]
	return strings.TrimRight(strings.Join(inner, "\n"), "\n"), true
}
