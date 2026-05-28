package generate

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"buttress/internal/specmeta"
)

// buildSystemPrompt constructs the initial system message.
// Spec files (types, spec, docs, tests) are included here so that
// subsequent fix messages only need to carry the error diff — not the
// full spec again.
func buildSystemPrompt(req Request, _ *specmeta.SpecMeta) (string, error) {
	var sb strings.Builder

	fmt.Fprintf(&sb,
		"You are generating a %s implementation for the spec package %q.\n"+
			"Output file: %s\n\n",
		req.Language, req.PackageName, req.OutputPath,
	)

	// Tell the LLM the relative import path to the machine/ directory.
	machineDir := filepath.Join(req.Spec.Dir, "machine")
	if _, err := os.Stat(machineDir); err == nil {
		rel, err := filepath.Rel(filepath.Dir(req.OutputPath), machineDir)
		if err == nil {
			rel = filepath.ToSlash(rel)
			if !strings.HasPrefix(rel, ".") {
				rel = "./" + rel
			}
			fmt.Fprintf(&sb, "Import types using: import type { ... } from %q\n\n", rel)
		}
	}

	// machine/ — type definitions and co-located tests; the machine-verifiable contract.
	if err := appendSection(&sb, machineDir, "TYPES AND TESTS (your implementation must pass every test)"); err != nil {
		return "", err
	}

	// human/ — prose spec and documentation; the human-readable intent.
	if err := appendSection(&sb, filepath.Join(req.Spec.Dir, "human"), "SPEC AND DOCS"); err != nil {
		return "", err
	}

	sb.WriteString("=== INSTRUCTIONS ===\n")
	fmt.Fprintf(&sb,
		"Generate a single %s file that implements the spec above.\n"+
			"Requirements:\n"+
			"  1. Use the exact types declared in TYPE DEFINITIONS\n"+
			"  2. Pass every test in the TESTS section\n"+
			"  3. No TypeScript compiler errors (strict mode)\n"+
			"Output ONLY the implementation code. No code fences, no explanations.\n",
		req.Language,
	)

	return sb.String(), nil
}

// buildFixMessage builds the user turn that follows a failed verification.
// It carries only the error output — the spec files stay in the system message.
func buildFixMessage(errSections []string) string {
	var sb strings.Builder
	sb.WriteString("The implementation has errors. Fix them and output ONLY the corrected code.\n\n")
	for _, s := range errSections {
		sb.WriteString(s)
		sb.WriteString("\n\n")
	}
	return strings.TrimRight(sb.String(), "\n")
}

// appendSection writes all text files from dir under a labeled heading.
// Silently skips missing directories.
func appendSection(sb *strings.Builder, dir, heading string) error {
	files, err := collectTextFiles(dir)
	if os.IsNotExist(err) || len(files) == 0 {
		return nil
	}
	if err != nil {
		return err
	}
	fmt.Fprintf(sb, "=== %s ===\n\n", heading)
	for _, f := range files {
		rel, _ := filepath.Rel(dir, f.path)
		fmt.Fprintf(sb, "--- %s ---\n%s\n\n", filepath.ToSlash(rel), f.content)
	}
	return nil
}

type specFile struct {
	path    string
	content string
}

func collectTextFiles(dir string) ([]specFile, error) {
	var files []specFile
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if !isTextFile(path) {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("reading %s: %w", path, err)
		}
		content := string(data)
		if len(data) > 64*1024 {
			content = string(data[:64*1024]) + "\n[truncated]"
		}
		files = append(files, specFile{path: path, content: content})
		return nil
	})
	return files, err
}
