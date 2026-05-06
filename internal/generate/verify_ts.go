package generate

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"buttress/internal/specmeta"
)

// verifyTypeScript runs tsc (type check) and the declared test runner against
// the generated output file. Returns one labeled error section per failing step.
func verifyTypeScript(ctx context.Context, req Request, meta *specmeta.SpecMeta) ([]string, error) {
	var sections []string

	tscOut, err := runTSCheck(ctx, req.OutputPath, req.ProjectDir)
	if err != nil {
		return nil, err
	}
	if tscOut != "" {
		sections = append(sections, "--- TypeScript compiler errors ---\n"+tscOut)
	}

	testOut, err := runTSTests(ctx, req, meta)
	if err != nil {
		return nil, err
	}
	if testOut != "" {
		sections = append(sections, "--- Test failures ---\n"+testOut)
	}

	return sections, nil
}

// runTSCheck type-checks outputPath using a temporary tsconfig. The compiler
// follows imports from the output file, so the spec's type declarations are
// reached via the relative import path already embedded in the generated code.
func runTSCheck(ctx context.Context, outputPath, projectDir string) (string, error) {
	cfgPath, cleanup, err := writeTempTSConfig(outputPath)
	if err != nil {
		return "", fmt.Errorf("writing temp tsconfig: %w", err)
	}
	defer cleanup()

	result, err := runCmd(ctx, projectDir, "npx", "--yes", "tsc", "--project", cfgPath)
	if err != nil {
		return "", fmt.Errorf("running tsc: %w", err)
	}
	if result.exitCode != 0 {
		return trimCmdOutput(result.output), nil
	}
	return "", nil
}

// runTSTests runs the test runner declared in buttress.toml against the spec's
// test directory, with a generated vitest config that aliases the package name
// to the output file.
func runTSTests(ctx context.Context, req Request, meta *specmeta.SpecMeta) (string, error) {
	testsDir := filepath.Join(req.Spec.Dir, "machine")
	if _, err := os.Stat(testsDir); os.IsNotExist(err) {
		return "", nil
	}

	runner, version := meta.TestRunner("typescript")
	if runner == "" {
		runner = "vitest"
	}

	switch strings.ToLower(runner) {
	case "vitest":
		return runVitest(ctx, req, testsDir, version)
	case "bun":
		return runBunTest(ctx, req, testsDir)
	default:
		return "", fmt.Errorf("unsupported test runner %q — supported: vitest, bun", runner)
	}
}

// runVitest runs vitest against testsDir using a generated config that maps
// the package import name to the output file.
func runVitest(ctx context.Context, req Request, testsDir, version string) (string, error) {
	cfgPath, cleanup, err := writeTempVitestConfig(req.OutputPath, testsDir, req.PackageName)
	if err != nil {
		return "", fmt.Errorf("writing vitest config: %w", err)
	}
	defer cleanup()

	runnerCmd := "vitest"
	if version != "" {
		runnerCmd = "vitest@" + version
	}

	result, err := runCmd(ctx, req.ProjectDir, "npx", "--yes", runnerCmd, "run", "--config", cfgPath)
	if err != nil {
		return "", fmt.Errorf("running vitest: %w", err)
	}
	if result.exitCode != 0 {
		return trimCmdOutput(result.output), nil
	}
	return "", nil
}

// runBunTest runs the spec's test suite via bun test, injecting the generated
// output file via a temporary tsconfig passed with --tsconfig-override.
func runBunTest(ctx context.Context, req Request, testsDir string) (string, error) {
	cfgPath, cleanup, err := writeTempBunTSConfig(req.OutputPath, req.PackageName)
	if err != nil {
		return "", fmt.Errorf("writing bun tsconfig: %w", err)
	}
	defer cleanup()

	result, err := runCmd(ctx, req.ProjectDir, "bun", "--tsconfig-override", cfgPath, "test", testsDir)
	if err != nil {
		return "", fmt.Errorf("running bun test: %w", err)
	}
	if result.exitCode != 0 {
		return trimCmdOutput(result.output), nil
	}
	return "", nil
}

// writeTempBunTSConfig writes a tsconfig whose only job is to alias pkgName
// to outputPath via compilerOptions.paths, for use with bun --tsconfig-override.
func writeTempBunTSConfig(outputPath, pkgName string) (path string, cleanup func(), err error) {
	type compilerOptions struct {
		Paths map[string][]string `json:"paths"`
	}
	type tsConfig struct {
		CompilerOptions compilerOptions `json:"compilerOptions"`
	}

	cfg := tsConfig{
		CompilerOptions: compilerOptions{
			Paths: map[string][]string{pkgName: {outputPath}},
		},
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return "", nil, err
	}
	f, err := os.CreateTemp("", "buttress-bun-tsconfig-*.json")
	if err != nil {
		return "", nil, err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(f.Name())
		return "", nil, err
	}
	f.Close()
	return f.Name(), func() { os.Remove(f.Name()) }, nil
}

// writeTempTSConfig writes a minimal tsconfig.json to a temp file and returns
// its path along with a cleanup function.
func writeTempTSConfig(outputPath string) (path string, cleanup func(), err error) {
	type compilerOptions struct {
		Strict           bool   `json:"strict"`
		NoEmit           bool   `json:"noEmit"`
		Module           string `json:"module"`
		ModuleResolution string `json:"moduleResolution"`
		Target           string `json:"target"`
	}
	type tsConfig struct {
		CompilerOptions compilerOptions `json:"compilerOptions"`
		Files           []string        `json:"files"`
	}

	cfg := tsConfig{
		CompilerOptions: compilerOptions{
			Strict:           true,
			NoEmit:           true,
			Module:           "esnext",
			ModuleResolution: "bundler",
			Target:           "ES2022",
		},
		Files: []string{outputPath},
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return "", nil, err
	}

	f, err := os.CreateTemp("", "buttress-tsconfig-*.json")
	if err != nil {
		return "", nil, err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(f.Name())
		return "", nil, err
	}
	f.Close()
	return f.Name(), func() { os.Remove(f.Name()) }, nil
}

// writeTempVitestConfig writes a vitest config that aliases pkgName to the
// generated output file and scopes the test run to testsDir.
func writeTempVitestConfig(outputPath, testsDir, pkgName string) (path string, cleanup func(), err error) {
	// Use a plain JS object export — no import from vitest/config needed,
	// so the config works even before vitest is installed locally.
	include := filepath.ToSlash(filepath.Join(testsDir, "**", "*.{test,spec}.{ts,mts,js,mjs}"))
	content := fmt.Sprintf(`export default {
  resolve: {
    alias: {
      %q: %q,
    },
  },
  test: {
    include: [%q],
  },
}
`, pkgName, outputPath, include)

	f, err := os.CreateTemp("", "buttress-vitest-*.mjs")
	if err != nil {
		return "", nil, err
	}
	if _, err := f.WriteString(content); err != nil {
		f.Close()
		os.Remove(f.Name())
		return "", nil, err
	}
	f.Close()
	return f.Name(), func() { os.Remove(f.Name()) }, nil
}

type cmdResult struct {
	output   string
	exitCode int
}

// runCmd executes name+args from dir, capturing stdout and stderr combined.
// A non-zero exit code is not treated as an error — callers inspect exitCode.
func runCmd(ctx context.Context, dir, name string, args ...string) (*cmdResult, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf

	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return &cmdResult{output: buf.String(), exitCode: exitErr.ExitCode()}, nil
		}
		// Binary not found or other execution error.
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	return &cmdResult{output: buf.String(), exitCode: 0}, nil
}

// trimCmdOutput caps tool output at 8 KiB so it fits comfortably in a chat turn.
func trimCmdOutput(s string) string {
	const maxBytes = 8 * 1024
	s = strings.TrimSpace(s)
	if len(s) > maxBytes {
		return s[:maxBytes] + "\n[output truncated]"
	}
	return s
}
