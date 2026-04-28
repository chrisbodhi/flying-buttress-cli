// Package generate defines the Generator interface for producing implementation
// source files from a downloaded spec.
//
// No concrete implementation is provided yet; this package exists to establish
// the interface so that cmd/add can fail early when --generate is requested
// but no LLM is configured.
package generate

import (
	"context"
	"errors"
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

// Generator generates implementation source files from a spec archive.
type Generator interface {
	Generate(ctx context.Context, spec *SpecArchive, language string) error
}

// Stub is a Generator that always returns ErrNotConfigured.
// It is used as a safe default until a real implementation is wired in.
type Stub struct{}

func (Stub) Generate(_ context.Context, _ *SpecArchive, _ string) error {
	return ErrNotConfigured
}
