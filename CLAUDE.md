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
- `cmd/` — cobra commands wired with `charmbracelet/fang`; owns TUI output and flag parsing; orchestrates service calls. Subcommands: `add`, `config`, `generate`, `hash`, `init`, `list`
- `internal/ref/` — parses `@org/pkg[@hash]` package reference strings
- `internal/versions/` — defines the `Version` type and parses the VERSIONS.txt format
- `internal/github/` — HTTP client; fetches VERSIONS.txt from any raw URL; downloads and verifies spec tarballs from GitHub tag archives
- `internal/store/` — disk I/O; global content-addressed cache at `~/.cache/buttress/packages/@org/pkg/<hash>/`, hard-linked into `./buttress/@org/pkg/`; reads/writes `buttress.lock` (JSON)
- `internal/config/` — TOML config at `~/.config/buttress/config.toml`
- `internal/localconfig/` — per-project `.buttress.local.toml` overrides (package manager, per-package output/language)
- `internal/contenthash/` — canonical SHA-256 over files under `human/` and `machine/` only; files walked in lexical order. `extractTarGz` in `internal/github/` uses the same algorithm and must agree
- `internal/specmeta/` — parses `buttress.toml` from a spec archive (name, description, per-language test config)
- `internal/scaffold/` — `Plan(Spec) []File` + `Write` for `buttress init` to generate spec package layouts
- `internal/sandbox/` — runs untrusted commands in isolation; backends are Apple `container` CLI (macOS) and Docker, tried in that order by `Detect`
- `internal/generate/` — `Generator` interface; `Stub` returns `ErrNotConfigured`; `LLMGenerator` calls an OpenAI-compatible chat completions endpoint with retry/verification; `SpecArchive` struct
- `internal/tui/` — lipgloss styles and bubbletea version picker
- `internal/testutil/` — shared test helpers; `WriteTree(t, root, map[path]content)` for seeding temp directory trees

**`buttress config` wizard:** uses `github.com/charmbracelet/huh` in accessible mode (`WithAccessible(true)`) for testability. Three sequential forms: type selection (local/frontier), credential fields, language. Pre-populates from existing config. `config set <key> <value>` and `config get <key>` bypass the wizard for scripting.

**Testing huh forms:** use `io.Pipe` (not `strings.NewReader`) as the stdin source. Each huh field creates its own `bufio.Scanner`; `strings.NewReader` gets fully buffered by the first scanner, starving subsequent fields. With `io.Pipe`, each write blocks until the scanner reads it, so fields get lines one at a time.

**Content hash:** The hash in VERSIONS.txt (`sha256:abc123…`) doubles as the git tag name in the spec repo (colons replaced with hyphens for the tag, e.g. `sha256-abc123…`). It covers only files under `human/` and `machine/` — everything else (`buttress.toml`, `VERSIONS.txt`, README, tooling config) is packaging and excluded, which keeps the hash stable and avoids hashing files that reference the hash. The canonical implementation is `internal/contenthash`; `extractTarGz` in `internal/github` recomputes the same hash on the tarball stream to verify downloads.

**VERSIONS.txt format:** Strict alternating lines — hash, description, hash, description — most recent first, no blank separators between pairs. Blank lines and `#` comments are skipped by the parser.

**`--generate` flow:** Fails early in `runAdd` if `cfg.HasLLM()` is false. The active backend is `generate.LLMGenerator`, which posts to an OpenAI-compatible chat completions endpoint and retries with verification (see `verify_ts.go` for the TypeScript verifier). `generate.Stub` is retained as the unconfigured fallback that returns `ErrNotConfigured`.

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
