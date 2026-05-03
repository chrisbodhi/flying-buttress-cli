# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
go build ./...          # build
go test ./...           # run all tests
go test ./internal/...  # run a specific package
go run . add @org/pkg   # run the CLI directly
```

The binary name is `buttress` (the `Use` field in root command).

## Architecture

Flying Buttress is a spec registry CLI — it distributes docs, tests, specs, and type definitions (SHA-pinned, not versioned). Users generate implementations from specs via LLM. Think Bun-style package manager but for specs, not code.

**Layer separation:**
- `cmd/` — cobra commands wired with `charmbracelet/fang`; owns TUI output and flag parsing; orchestrates service calls
- `internal/ref/` — parses `@org/pkg[@hash]` package reference strings
- `internal/versions/` — defines the `Version` type and parses the VERSIONS.txt format
- `internal/github/` — HTTP client; fetches VERSIONS.txt from any raw URL; downloads and verifies spec tarballs from GitHub tag archives
- `internal/store/` — disk I/O; global content-addressed cache at `~/.cache/buttress/packages/@org/pkg/<hash>/`, hard-linked into `./buttress/@org/pkg/`; reads/writes `buttress.lock` (JSON)
- `internal/config/` — TOML config at `~/.config/buttress/config.toml`
- `internal/generate/` — `Generator` interface + `Stub` (not yet implemented); `SpecArchive` struct
- `internal/tui/` — lipgloss styles and bubbletea version picker
- `internal/testutil/` — shared test helpers; `WriteTree(t, root, map[path]content)` for seeding temp directory trees

**Content hash:** The hash in VERSIONS.txt (`sha256:abc123…`) doubles as the git tag name in the spec repo (colons replaced with hyphens for the tag, e.g. `sha256-abc123…`). It's computed by `extractTarGz` over file paths + contents in traversal order.

**VERSIONS.txt format:** Strict alternating lines — hash, description, hash, description — most recent first, no blank separators between pairs. Blank lines and `#` comments are skipped by the parser.

**`--generate` flow:** Fails early in `runAdd` if `cfg.HasLLM()` is false. `generate.Stub` always returns `ErrNotConfigured`; a real implementation goes here when the LLM layer is built.

## Config file shape

```toml
[llm]
provider = "openai"   # or "ollama", "lmstudio", etc.
base_url = ""
api_key  = ""
model    = ""

[project]
language = "go"       # target language for --generate

[registry]
cache_dir = ""        # default: ~/.cache/buttress
```
