# Flying Buttress (`buttress`)

[![CI](https://github.com/chrisbodhi/flying-buttress-cli/actions/workflows/ci.yml/badge.svg?branch=trunk)](https://github.com/chrisbodhi/flying-buttress-cli/actions/workflows/ci.yml)
[![Coverage](https://img.shields.io/endpoint?url=https://raw.githubusercontent.com/chrisbodhi/flying-buttress-cli/trunk/.github/badges/coverage.json)](https://github.com/chrisbodhi/flying-buttress-cli/actions/workflows/ci.yml)

Spec registry CLI for the agentic coding era. Download verified, SHA-pinned spec packages — docs, types, tests, and behavioral contracts — then generate the implementation locally with your LLM.

The premise: generating software is now nearly free, and so is finding vulnerabilities in it. The traditional model of installing opaque third-party binaries is no longer tenable. Flying Buttress distributes specs, not implementations. Your team owns and manages the code.

## Installation

```bash
go install buttress
```

Or build from source:

```bash
git clone https://github.com/chrisbodhi/flying-buttress-cli
cd flying-buttress-cli
go build -o buttress .
```

## Usage

```bash
# Download a spec and pin it in buttress.lock
buttress add @org/pkg

# Pin a specific version by content hash
buttress add @org/pkg@sha256:abc123…

# Download and immediately generate an implementation
buttress add @org/pkg --generate

# Use a custom version list URL (e.g. a gist, for testing)
buttress add @org/pkg --versions-url https://…/VERSIONS.txt

# Generate an implementation from an already-installed spec
buttress generate @org/pkg
```

When no hash is provided, an interactive picker shows available versions pulled from the spec repo's `VERSIONS.txt`.

## Configuration

`~/.config/buttress/config.toml` — created manually, never committed.

```toml
[llm]
provider = "openai"        # "ollama", "lmstudio", or any OpenAI-compatible provider
base_url = ""              # required for local providers (e.g. http://localhost:11434)
api_key  = ""
model    = "gpt-4o"

[project]
language = "go"            # target language for --generate

[registry]
cache_dir = ""             # default: ~/.cache/buttress
```

`--generate` fails early if `[llm] provider` and `[llm] model` are not set.

## How it works

Spec packages live in GitHub repos. The `@org/pkg` reference maps directly to `github.com/org/pkg`. Each version is a git tag named after its content hash (e.g. `sha256-abc123…`).

**After `buttress add`:**

```
~/.cache/buttress/packages/@org/pkg/<hash>/   ← global content-addressed cache
./buttress/@org/pkg/                           ← hard-linked into your project
./buttress.lock                                ← pinned refs, commit this
```

The content hash is computed over all file paths and contents in the extracted archive. The hash in `buttress.lock` and `VERSIONS.txt` use colon form (`sha256:abc123…`); git tags use hyphen form (`sha256-abc123…`) because git doesn't allow colons in tag names.

## Spec package format

A spec repo contains:

```
buttress.toml    ← package manifest
VERSIONS.txt     ← version history (alternating SHA / description lines, newest first)
human/           ← prose specs, docs, behavioral contracts (Markdown)
machine/         ← types, tests, schemas — anything the LLM consumes verbatim
```

Only files under `human/` and `machine/` contribute to the content hash. Everything else (`buttress.toml`, `VERSIONS.txt`, README, tooling config) is packaging and is ignored by the hasher — this keeps the hash stable and avoids the circular dependency of hashing files that reference the hash.

`VERSIONS.txt` format — no blank lines between entries, most recent first:

```
sha256:ghi789…
Improve spec wording for ambiguous usage.
sha256:def456…
Include test for existing spacing.
sha256:abc123…
Initial spec and tests.
```

## Project layout

```
<project>/
  buttress/
    @org/
      pkg/               ← hard-linked from cache; inspect the spec here
  buttress.lock           ← commit this
  .buttress.local.toml   ← per-project overrides, gitignore this
```

## Status

| Command | Status |
|---|---|
| `buttress add` | Working |
| `buttress add --generate` | Working |
| `buttress generate` | Working |
| `buttress list` | Planned |
| `buttress remove` | Planned |
| `buttress update` | Planned |
| `buttress verify` | Planned |
