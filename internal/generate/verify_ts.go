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
		if !looksLikeTSError(tscOut) {
			req.progress("tsc environment error (not a code issue):\n%s", tscOut)
			return nil, fmt.Errorf("tsc failed with a tooling error — check that TypeScript is available in this project:\n%s", tscOut)
		}
		sections = append(sections, "--- TypeScript compiler errors ---\n"+tscOut)
	}

	if err := installTestPackages(ctx, req, meta); err != nil {
		return nil, err
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

// installTestPackages installs the packages declared in buttress.toml for the
// target language. Uses bun when the declared runner is bun, npm otherwise.
func installTestPackages(ctx context.Context, req Request, meta *specmeta.SpecMeta) error {
	pkgs := meta.TestPackages(req.Language)
	if len(pkgs) == 0 {
		return nil
	}

	req.progress("installing test packages: %s", strings.Join(pkgs, ", "))

	var result *cmdResult
	var err error
	switch meta.PackageManagerFor(req.Language) {
	case "bun":
		result, err = runCmd(ctx, req.ProjectDir, "bun", append([]string{"add", "--dev"}, pkgs...)...)
	case "pnpm":
		result, err = runCmd(ctx, req.ProjectDir, "pnpm", append([]string{"add", "--save-dev"}, pkgs...)...)
	case "yarn":
		result, err = runCmd(ctx, req.ProjectDir, "yarn", append([]string{"add", "--dev"}, pkgs...)...)
	default:
		result, err = runCmd(ctx, req.ProjectDir, "npm", append([]string{"install", "--no-save"}, pkgs...)...)
	}
	if err != nil {
		return fmt.Errorf("installing test packages: %w", err)
	}
	if result.exitCode != 0 {
		return fmt.Errorf("installing test packages failed:\n%s", trimCmdOutput(result.output))
	}
	return nil
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

	// --package typescript pins npx to the typescript package's tsc binary,
	// preventing it from picking up an unrelated tsc on PATH.
	result, err := runCmd(ctx, projectDir, "npx", "--yes", "--package", "typescript", "tsc", "--project", cfgPath)
	if err != nil {
		return "", fmt.Errorf("running tsc: %w", err)
	}
	if result.exitCode != 0 {
		return trimCmdOutput(result.output), nil
	}
	return "", nil
}

// looksLikeTSError reports whether output contains at least one genuine
// TypeScript diagnostic (error TS####). Output that lacks this is a tooling or
// environment failure, not a code error, and should not be sent to the LLM.
func looksLikeTSError(output string) bool {
	return strings.Contains(output, "error TS")
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
	cfgPath, cleanup, err := writeTempBunTSConfig(req.ProjectDir, req.OutputPath, req.PackageName)
	if err != nil {
		return "", fmt.Errorf("writing bun tsconfig: %w", err)
	}
	defer cleanup()

	result, err := runCmd(ctx, req.ProjectDir, "bun", "test", "--tsconfig-override", cfgPath, testsDir)
	if err != nil {
		return "", fmt.Errorf("running bun test: %w", err)
	}
	if result.exitCode != 0 {
		return trimCmdOutput(result.output), nil
	}
	return "", nil
}

// writeTempBunTSConfig writes a tsconfig whose only job is to alias pkgName
// to outputPath via compilerOptions.paths, for use with bun test --tsconfig-override.
// The file is written into the project directory, not the system temp dir, to avoid
// a bun bug with /var/folders paths on macOS causing "directory mismatch" errors.
func writeTempBunTSConfig(projectDir, outputPath, pkgName string) (path string, cleanup func(), err error) {
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
	f, err := os.CreateTemp(projectDir, "buttress-bun-tsconfig-*.json")
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
