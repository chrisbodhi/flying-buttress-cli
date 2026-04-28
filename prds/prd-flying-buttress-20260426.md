# Product Requirements Document: Flying Buttress (`buttress`)

Generated: 2026-04-26

Author: Product Manager Agent

Status: Draft

---

## Executive Summary

Flying Buttress is a CLI tool and spec registry for the agentic coding era. It solves a structural problem created by the simultaneous collapse of two costs: generating software (LLMs) and finding vulnerabilities (automated scanning). The result is a supply-chain crisis for AI-generated code. Flying Buttress's answer is to distribute specs, not implementations — giving developers a verified, SHA-pinned contract they generate against locally, rather than opaque third-party binaries they have to trust blindly.

The working metaphor is deliberate: flying buttresses are the external scaffolding that allows cathedrals to stand. The software bazaar is collapsing under its own weight. Flying Buttress is the structural layer that makes trustworthy software possible while the ground is still shaking.

A working `buttress add` flow exists today. This PRD covers the full product vision, the immediate next phase of commands and spec authoring tooling, and the open architectural questions that must be resolved to move from a working prototype to a production-grade tool.

---

## Product Vision

### Problem Statement

AI coding tools have made generating software nearly free. That same economic shift has made finding exploits in that software nearly free too. The result is a new attack surface: AI-generated code that looks plausible, passes linting, and ships with vulnerabilities that automated scanners will find faster than any human reviewer can. The traditional npm/pip/cargo model compounds this — when you `npm install`, you are trusting not just the package author but the entire transitive dependency graph, every build script, and the registry's own supply chain.

Developers using Cursor, Claude Code, or GitHub Copilot have no verified layer between "LLM generated this" and "this is running in production." There is no standard for what a trustworthy AI-ready interface looks like, and no infrastructure for distributing one.

### Solution Overview

Flying Buttress separates the spec (what something should do, tested and typed) from the implementation (the code that does it). Spec packages contain docs, behavioral specs, type definitions, and test cases — but no runnable implementation. Consumers download the spec, verify it against a content hash, and generate an implementation locally using their preferred LLM. The generation happens on the consumer's machine, with the consumer's model, against a contract they can inspect.

This is `npm install` redesigned for an era where you would rather generate the implementation yourself than run someone else's.

### Value Proposition

For spec consumers: pull in a verified, hash-pinned interface contract and generate a trusted implementation — rather than installing opaque third-party code that your LLM might have hallucinated anyway.

For spec authors: distribute a tested, typed, documented interface without shipping an implementation. Reach consumers across every language without maintaining per-language packages. Let the consumer's LLM do the porting.

For organizations: establish internal spec standards that every team's LLM generates against, creating consistency across codebases that use different languages and different models.

---

## Target Users

### Primary Personas

**Persona 1: The Spec Consumer — Agentic Developer**
A developer using Cursor, Claude Code, or Copilot for day-to-day feature work. They regularly pull in third-party packages but are increasingly uncomfortable with the supply-chain risk of installing code they did not write and cannot easily audit. They want the same ergonomics as `npm install` but with a generation step they control. They care about SHA pinning because they have been burned by mutable package references. They do not want to think about LLM configuration more than once.

Pain points: opaque npm/pip packages with unknown AI-generated internals; no standard way to verify what an LLM generated matches what was promised; lock file drift when upstream packages change silently.

**Persona 2: The Spec Author — Library Maintainer or API Designer**
A developer or team maintaining a library or internal API. They want to define a contract (types, tests, behavioral specs) once and have it work across languages. Today they maintain separate TypeScript, Go, and Python packages that diverge over time. They want a publishing workflow that is as simple as a git push and want version history that is content-addressed, not semver-dependent.

Pain points: maintaining per-language implementations of the same interface; no canonical format for "here is the spec, generate the rest"; semver is a leaky abstraction over content changes.

**Persona 3: The Platform Engineer — Internal Standards Owner**
An engineer at a company with multiple product teams. They want to publish internal specs (auth libraries, logging interfaces, observability hooks) that every team's LLM generates against, ensuring consistency without mandating a specific runtime library. They need private spec repos with authenticated access and a way to verify that generated implementations are up to date with the pinned spec.

Pain points: internal library drift across teams; no standard way to enforce interface contracts across polyglot codebases; semver-based versioning creates false confidence in compatibility.

### User Stories

**Spec Consumer**
- As a developer, I want to run `buttress add @org/pkg` so that I can download a verified spec and pin it to my project without installing runnable third-party code.
- As a developer, I want to run `buttress add @org/pkg --generate` so that my LLM generates an implementation against the verified spec without me having to write a prompt.
- As a developer, I want `buttress add` to write a lock file so that my team can reproduce the exact same spec version I used.
- As a developer, I want to run `buttress list` so that I can see all specs installed in my project and their pinned hashes.
- As a developer, I want to run `buttress update @org/pkg` so that I can interactively choose a newer spec version when one is available.
- As a developer, I want to run `buttress remove @org/pkg` so that I can cleanly remove a spec from my project and lock file.
- As a developer, I want to run `buttress verify` so that I can confirm that my installed specs still match their pinned content hashes.
- As a developer on a private project, I want to authenticate with a GitHub token so that I can install specs from private repositories.

**Spec Author**
- As a spec author, I want a `buttress.toml` format so that I can declare the languages, test frameworks, and runtimes my spec supports.
- As a spec author, I want to run `buttress publish` (or a git-based equivalent) so that I can add a new version to my `versions.txt` without manually editing it.
- As a spec author, I want the spec package format to be language-agnostic at the top level so that consumers in any language can use it.
- As a spec author, I want the content hash to cover exactly the files in the spec so that consumers can verify nothing has changed since publication.

**Platform Engineer**
- As a platform engineer, I want to host spec repos in my organization's private GitHub so that internal specs are not publicly accessible.
- As a platform engineer, I want `.buttress.local.toml` to support per-project output path overrides so that generated implementations land in the right place for each consumer's project structure.
- As a platform engineer, I want `buttress verify` in CI so that generated implementations are flagged if the upstream spec has changed.

---

## Functional Requirements

### Phase 1 — Core Consumer Commands (Current State + Immediate Next)

**1. `buttress add @org/pkg[@sha]`** — BUILT
- Downloads spec archive from GitHub at the given git ref (or prompts for one)
- Computes SHA-256 content hash over extracted files
- Stores in global content-addressed cache (`~/.cache/buttress/packages/@org/pkg/<hash>/`)
- Hard-links into project (`./buttress/@org/pkg/`)
- Writes/updates `buttress.lock`
- Acceptance criteria: idempotent re-runs, correct hash in lock file, hard link verified on same filesystem, graceful fallback to copy on cross-filesystem installs
- Priority: Must Have (built)

**2. `buttress add @org/pkg --generate`** — STUBBED
- Requires LLM config (`~/.config/buttress/config.toml`, `[llm]` section with `provider` and `model`)
- Reads `buttress.toml` from the downloaded spec to determine target language (from `[project].language` in config, overridable per-spec in `.buttress.local.toml`)
- Constructs a generation prompt using spec docs, types, and behavioral specs
- Calls the configured LLM provider (Ollama, LM Studio, OpenAI-compatible) and writes the generated implementation to the output path
- Acceptance criteria: clear error if no LLM configured; generated file written to correct output path; generation does not overwrite spec files
- Priority: Must Have (next milestone)

**3. `buttress list`** — NOT BUILT
- Reads `buttress.lock` and displays all installed packages with org, package name, git ref (truncated), content hash (truncated), and install date
- Styled output using lipgloss
- Acceptance criteria: works with empty lock file (prints empty state message); output is machine-parseable with `--json` flag
- Priority: Must Have (next milestone)

**4. `buttress remove @org/pkg`** — NOT BUILT
- Removes entry from `buttress.lock`
- Removes project-local copy (`./buttress/@org/pkg/`)
- Does not remove global cache entry (cache is content-addressed and may be shared)
- Prompts for confirmation unless `--yes` flag is passed
- Acceptance criteria: idempotent; does not error if package was not installed; lock file is valid JSON after removal
- Priority: Must Have (next milestone)

**5. `buttress update @org/pkg`** — NOT BUILT
- Fetches the version list for the package, shows an interactive picker (same bubbletea picker as `buttress add`)
- If a newer version is selected, replaces the installed spec and updates the lock file
- Passes `--generate` through if specified
- Acceptance criteria: shows current version in picker as selected; no-op if already at latest
- Priority: Should Have

**6. `buttress verify`** — NOT BUILT
- Reads `buttress.lock`, recomputes content hashes for all installed spec directories in `./buttress/`, compares against stored hashes
- Exits non-zero and reports mismatches (suitable for CI use)
- Acceptance criteria: detects file additions, deletions, and modifications; passes cleanly on a fresh `buttress add`
- Priority: Must Have (for CI story)

**7. `buttress generate @org/pkg`** — NOT BUILT
- Standalone generation command for when a spec is already installed but no implementation exists yet (or needs to be regenerated)
- Reads spec from `./buttress/@org/pkg/`, applies same generation logic as `--generate` flag
- Acceptance criteria: requires spec to be installed; fails clearly if not
- Priority: Should Have

### Phase 2 — Spec Authoring Tooling

**8. Spec package format validation** — NOT BUILT
- `buttress check` (name TBD) validates a local directory against the expected spec package layout
- Checks for required files (`buttress.toml`, `versions.txt`), valid TOML syntax, hash consistency
- Intended for use in spec author CI
- Priority: Should Have

**9. `buttress publish`** — NOT BUILT
- Appends a new entry to `versions.txt` (SHA + description on alternating lines)
- Computes the content hash of the current spec files and writes/updates `[spec].hash` in `buttress.toml`
- Commits and pushes to GitHub (or prints instructions for manual git operations)
- Open question: does this require a git integration, or is it a file-mutation-only command that authors commit themselves?
- Priority: Nice to Have (MVP can be manual git push)

### Phase 3 — Registry and Discovery

**10. Registry index** — NOT BUILT
- Currently: direct GitHub mapping (`@org/pkg` → `github.com/org/pkg`)
- Future: a registry index that supports search, description metadata, and potentially mirrors for reliability
- The `github.Client` interface in the codebase is already abstracted behind a struct; the registry backend is swappable
- Priority: Nice to Have (direct GitHub mapping is sufficient for early adopters)

**11. `buttress search <query>`** — NOT BUILT
- Depends on registry index existing
- Priority: Nice to Have

### Configuration and Local Overrides

**12. `.buttress.local.toml` resolution** — NOT BUILT (config struct exists, resolution logic does not)
- Walk from spec directory up to project root (identified by presence of `buttress.lock`), collecting all `.buttress.local.toml` files
- Merge root-first, nearest-wins per key (cargo workspace semantics)
- Fields: `package_manager`, per-package `output` path override, language override
- This file is gitignored; it is consumer-local
- Acceptance criteria: nearest `.buttress.local.toml` wins; walk stops at `buttress.lock` boundary; no `.buttress.local.toml` is not an error
- Priority: Must Have (generation output path is unresolvable without this)

**13. Authentication** — NOT BUILT
- GitHub token support for private spec repos
- Token read from config (`[registry].github_token`) or `BUTTRESS_GITHUB_TOKEN` environment variable
- Passed as `Authorization: Bearer <token>` header on GitHub API and archive download requests
- Priority: Must Have for Persona 3 (platform engineer), Should Have overall

### User Flow — Happy Path (Consumer)

1. Developer runs `buttress add @chrisbodhi/left-pad` in their project directory
2. `buttress` fetches `versions.txt` from `https://raw.githubusercontent.com/chrisbodhi/left-pad/HEAD/versions.txt`
3. Interactive bubbletea picker shows available versions with descriptions; developer selects one
4. `buttress` downloads the tarball from GitHub, extracts it, computes SHA-256 content hash
5. Spec is stored in `~/.cache/buttress/packages/@chrisbodhi/left-pad/<hash>/`
6. Spec is hard-linked into `./buttress/@chrisbodhi/left-pad/`
7. `buttress.lock` is written with `git_ref`, `content_hash`, and `installed_at`
8. If `--generate` was passed: `buttress` reads `buttress.toml` for language/framework info, constructs a prompt, calls the configured LLM, writes the implementation to the resolved output path
9. Developer sees a styled success message; generated file is ready to use

---

## Non-Functional Requirements

### Performance
- `buttress add` (no generation): target under 3 seconds on a normal connection for a typical spec package (docs + types + tests, <1 MB)
- Cache hit (same content hash already present): under 200ms (just hard-link + lock write)
- `buttress verify`: under 1 second for projects with up to 20 installed specs
- LLM generation latency is provider-dependent and outside `buttress`'s control; progress feedback must be shown

### Security
- Content hash computed over file paths and contents (current implementation includes path names in the hash via `fmt.Fprintf(hasher, "%s\n", relPath)`) — this is correct and should be documented
- Path traversal protection in tar extraction (already implemented via `..` check in `extractTarGz`)
- GitHub token must never be written to `buttress.lock` or logged
- `--generate` must not write implementation files outside the resolved output path
- The spec cache is read-only after `Commit`; no spec file should be mutated post-installation

### Reliability
- `buttress add` is idempotent: re-running with the same ref at the same hash is a no-op
- Partial downloads are cleaned up (`os.RemoveAll(tmpDir)` on error — already implemented)
- Lock file writes are atomic (write to `.tmp` then rename — already implemented)

### Developer Experience
- All user-facing output uses lipgloss styling (success checkmarks, dim secondary info, SHA display style) — already established
- Error messages must be actionable: tell the user what to do, not just what went wrong
- `--help` output is styled via fang (already wired)
- `buttress` must work offline for already-cached specs (verify, list, remove do not require network)

### Platform Support
- Primary: macOS (darwin), Linux
- Secondary: Windows (hard-link semantics differ; copyDir fallback already implemented)
- Go 1.26+, single binary distribution

### Accessibility
- CLI tool; WCAG does not apply directly
- TUI components (bubbletea picker) must be navigable by keyboard only
- Color output must degrade gracefully in non-color terminals (lipgloss handles this via color profile detection)

---

## Success Metrics

### Key Performance Indicators

**Adoption**
- Number of distinct `@org/pkg` packages published to GitHub with a valid `buttress.toml` and `versions.txt`
- Number of `buttress add` invocations per week (via opt-in telemetry, if implemented)
- Number of unique `buttress.lock` files in public GitHub repos

**Quality**
- Hash verification failure rate in `buttress verify` runs (target: <0.1% of CI runs fail due to unexpected hash mismatch after clean install)
- `buttress add` error rate excluding user configuration errors (target: <1%)

**Generation**
- `buttress add --generate` success rate by LLM provider
- Time from `buttress add --generate` invocation to generated file written (p50, p95)

### Success Criteria for MVP Launch

- `buttress add`, `buttress list`, `buttress remove`, `buttress verify` all functional and tested
- `buttress add --generate` functional against at least Ollama and OpenAI-compatible providers
- At least one public spec package (`@chrisbodhi/left-pad` or equivalent) usable as a real-world test
- `.buttress.local.toml` resolution implemented
- GitHub token authentication implemented
- `buttress.lock` format documented and stable (no breaking changes after 1.0)

---

## Technical Considerations

### Architecture Overview

The codebase follows a clean package separation:

- `cmd/` — cobra commands (`add` today; `list`, `remove`, `update`, `verify`, `generate` to come)
- `internal/config/` — TOML config load/save with defaults
- `internal/ref/` — package reference parsing (`@org/pkg@sha`)
- `internal/github/` — HTTP client for version list fetching and tarball download/extraction
- `internal/store/` — content-addressed cache + hard-link project copy + lock file read/write
- `internal/generate/` — Generator interface (stubbed today)
- `internal/versions/` — version list parsing
- `internal/tui/` — bubbletea picker + lipgloss styles

The `github.Client` struct is the only concrete registry backend today. The `Generator` interface is already defined for pluggable LLM backends. Adding new commands is additive (new files in `cmd/`, new packages in `internal/`).

### Dependencies

**Direct (from go.mod)**
- `github.com/charmbracelet/fang` — styled cobra wrapper
- `github.com/spf13/cobra` — CLI framework
- `github.com/BurntSushi/toml` — TOML parsing for config and `buttress.toml`
- `github.com/charmbracelet/bubbletea` — TUI picker
- `charm.land/lipgloss` — terminal styling

**External Services**
- GitHub (raw content delivery and archive downloads) — no API key required for public repos
- LLM providers: Ollama (local), LM Studio (local), OpenAI-compatible APIs (remote) — key required for remote

**To Be Added**
- Nothing mandatory for Phase 1; registry index (Phase 3) may require a new HTTP backend

### Data Requirements

**`buttress.lock` (project root, committed)**
```json
{
  "@org/pkg": {
    "org": "org",
    "pkg": "pkg",
    "git_ref": "<sha>",
    "content_hash": "<sha256hex>",
    "installed_at": "<RFC3339>"
  }
}
```
This format is stable. Breaking changes require a migration path.

**`buttress.toml` (spec repo root, published by spec author)**
```toml
name = "@org/pkg"
description = "..."

[spec]
hash = "sha256:..."
history = "versions.txt"

[[language]]
name = "typescript"
runtime_minimum = "node 20"   # optional

  [[language.test]]
  framework = "fast-check"
  framework_version = "4.7.0"
  runner = "vitest"
  runner_version = "3.0.0"
```

**`.buttress.local.toml` (consumer project tree, gitignored)**
```toml
package_manager = "bun"

["@org/pkg"]
output = "src/generated/pkg.ts"
language = "typescript"
```

**`versions.txt` (spec repo, published by spec author)**
```
abc123def456...
First stable version with full type definitions
789xyz...
Added property-based tests
```
Alternating lines: SHA / human description, most recent first.

**Global cache layout**
```
~/.cache/buttress/
  packages/
    @org/
      pkg/
        <content_hash>/
          buttress.toml
          versions.txt
          docs/
          spec/
          tests/
          types/
```

**Project layout**
```
<project>/
  buttress/
    @org/
      pkg/          ← hard-linked from cache
  buttress.lock
  .buttress.local.toml   ← gitignored
```

### Privacy Considerations
- `buttress.lock` contains package identifiers and timestamps — commit this; it is non-sensitive
- `~/.config/buttress/config.toml` contains LLM API keys — never committed, never logged
- No telemetry in MVP; if added later, opt-in only with explicit documentation

---

## Timeline and Milestones

### Phase 1 — Complete Consumer CLI (Target: 6 weeks from start)

**Week 1-2**
- `buttress list` command
- `buttress remove` command
- `.buttress.local.toml` resolution logic

**Week 3-4**
- Real LLM generation backend (Ollama + OpenAI-compatible)
- `buttress generate` standalone command
- GitHub token authentication

**Week 5-6**
- `buttress verify` command
- `buttress update` command
- Integration tests against a real public spec package
- Documentation: README, config format, spec package format

### Phase 2 — Spec Authoring Tooling (Target: 4 weeks after Phase 1)

**Week 7-8**
- `buttress.toml` parser/validator in `buttress`
- `buttress check` (spec validation command for authors)
- `versions.txt` tooling

**Week 9-10**
- `buttress publish` (local file mutation; git operations optional)
- Authoring guide documentation

### Phase 3 — Registry and Ecosystem (Target: post-1.0)
- Registry index design and implementation
- `buttress search`
- Homebrew/scoop distribution
- GitHub Actions integration example

### Key Milestones
- M1: `buttress add` + `buttress list` + `buttress remove` + `buttress verify` all working — **Phase 1, Week 2**
- M2: Real LLM generation end-to-end with Ollama — **Phase 1, Week 4**
- M3: First public spec package installable and generatable by an external developer — **Phase 1, Week 6**
- M4: `buttress check` + `buttress publish` for spec authors — **Phase 2, Week 10**
- M5: Registry index v1 — **Phase 3, TBD**

---

## Risks and Mitigation

### Technical Risks

**GitHub as the only registry backend**
Risk: GitHub rate limits unauthenticated requests (60/hour per IP). A developer running `buttress add` for many packages in CI could hit this.
Mitigation: Add `Authorization: Bearer <token>` support via config or `BUTTRESS_GITHUB_TOKEN` env var. Document CI setup. Long-term: registry mirror.

**Hard-link behavior on Windows and cross-filesystem mounts**
Risk: `os.Link` fails across filesystems (Docker volumes, NFS, Windows cross-drive). The `copyDir` fallback exists but is slower and results in duplicate disk usage.
Mitigation: Already handled via fallback. Document that cache and project should be on the same filesystem for best performance. Consider a symlink alternative for cross-filesystem cases.

**Content hash stability**
Risk: If the hash algorithm or the set of files included in the hash changes, all existing `buttress.lock` files become invalid.
Mitigation: Lock the hash format to `sha256` prefix (already in `LockEntry.ContentHash`). Document the hash algorithm. Never change it without a migration path and major version bump.

**LLM generation quality**
Risk: Generated implementations are incorrect, insecure, or incomplete. Users blame `buttress`.
Mitigation: `buttress` is not responsible for generation quality — it is a delivery mechanism. Document this clearly. `buttress verify` verifies the spec, not the generated implementation. Consider a `buttress test` command (future) that runs the spec's test suite against the generated implementation.

**`versions.txt` format brittleness**
Risk: Spec authors accidentally break the alternating-line format (extra blank lines, comments, etc.).
Mitigation: The parser should be lenient about blank lines. Add `buttress check` to validate the format before publishing.

### Business Risks

**Chicken-and-egg: no specs, no consumers**
Risk: The tool has no value until there are public spec packages to install.
Mitigation: Ship at least one canonical public spec package (`@chrisbodhi/left-pad` or similar) before any external announcement. Author's own packages are the first-mover advantage.

**Developer skepticism about "generate your own implementation"**
Risk: Developers are accustomed to `npm install` and may not see the value of generating locally.
Mitigation: Frame the value around supply-chain trust and auditability, not developer convenience. The first adopters are security-conscious developers and platform engineers, not general-purpose developers.

**Spec author adoption**
Risk: Spec authors don't want to maintain a separate spec repo alongside their implementation repo.
Mitigation: `buttress publish` should be as low-friction as possible. Explore whether a spec can be extracted from an existing repo (a subdirectory) rather than requiring a separate repo.

---

## Open Questions

The following questions are unresolved and represent decisions that block specific implementation paths. Each should be resolved before the relevant milestone is reached.

**1. Runtime version enforcement in `buttress.toml`**
The `runtime_minimum` field on `[[language]]` is optional today. Should `buttress` check the consumer's actual runtime version (e.g., `node --version`) against this constraint before generating? If so, what happens when the check fails — hard error or warning?
Current lean: optional field, skip check if absent, warn if present and version is unresolvable. Resolution needed before LLM generation is implemented.

**2. Version list URL location**
Should the `versions.txt` URL be derived by convention (`https://raw.githubusercontent.com/{org}/{pkg}/HEAD/versions.txt`) or declared explicitly in `buttress.toml` under `[spec].history`? Convention is simpler for authors; explicit declaration supports non-GitHub hosts and non-standard repo layouts.
Current lean: relative path in `[spec].history`, resolved by `buttress` at fetch time. This is a breaking change from the current convention-based implementation in `github.VersionsURL`. Resolution needed before `buttress publish` is designed.

**3. Command set completeness after `buttress add`**
The commands described above (`list`, `remove`, `update`, `generate`, `verify`) represent the author's current best guess at the MVP command surface. Are there commands missing from this list? Notably: `buttress init` (scaffold a new spec repo), `buttress sync` (re-run generation for all installed specs), `buttress audit` (check installed specs against known-bad hashes). Each of these adds scope.
Resolution needed before Phase 1 Week 1 begins.

**4. Publishing workflow**
Should `buttress publish` be a `buttress` command that mutates `versions.txt` and `buttress.toml` locally (leaving the git commit/push to the author), or should it also perform the git operations? A fully automated `buttress publish` that commits and pushes is more convenient but introduces git credential management complexity.
Current lean: `buttress publish` is file-mutation-only; authors commit and push themselves. Resolution needed before Phase 2.

**5. Registry index timeline**
The direct GitHub mapping (`@org/pkg` → `github.com/org/pkg`) is elegant and requires no registry infrastructure. However, it couples `buttress` to GitHub and makes discovery impossible. When does this need to become a real registry with search?
Current lean: direct GitHub mapping is sufficient through 1.0. A registry index is a post-1.0 concern. This should be revisited when the number of public spec packages exceeds what can be discovered by word-of-mouth. The `github.Client` struct is the only concrete implementation of the fetch interface; a registry client can be added behind the same interface without breaking consumers.

**6. Authentication for private spec repos**
GitHub tokens for private repos are clearly needed for Persona 3. The question is the priority relative to other Phase 1 work. Without this, the platform engineer persona is blocked.
Current lean: implement GitHub token support in Phase 1 Week 4. The `http.Client` in `github.Client` needs a token-injection layer; the config struct already has a `[registry]` section that can hold `github_token`.

**7. Project language granularity**
The `[project].language` field in `~/.config/buttress/config.toml` sets a global default. A developer working in a polyglot monorepo will need per-spec language overrides. This is addressed by `.buttress.local.toml` (per-package `language` field), but the resolution precedence needs to be explicit: `buttress.toml` declares supported languages; `.buttress.local.toml` selects among them; `config.toml` is the fallback default.
Resolution needed before LLM generation is implemented.

---

## Appendix

### Competitive Analysis

**npm / pip / cargo / go mod**
Traditional package managers install compiled or source implementations. They have no concept of a spec-only package or LLM-mediated generation. Flying Buttress is not competing with them for the same use case; it is proposing a new category of artifact alongside them.

**OpenAPI / Protobuf / GraphQL schema distribution**
These distribute interface contracts (schemas) that code generators use to produce clients and servers. Flying Buttress is philosophically similar but more general: it is not limited to API schemas, and it uses LLMs instead of template-based code generators. The key difference is that spec packages in Flying Buttress can include behavioral tests and documentation, not just type signatures.

**Dagger / Nix / Guix (reproducible builds)**
These focus on reproducible build environments. Flying Buttress focuses on reproducible spec contracts. They are complementary: a developer might use Nix to pin their build environment and Flying Buttress to pin their spec contracts.

**jsrepo / pkgx**
jsrepo distributes source code snippets (not compiled packages) that consumers copy into their projects. This is similar in spirit to Flying Buttress's "you own the code" philosophy, but without the verification layer, the spec/implementation separation, or the LLM generation step.

### References

- Codebase: `/Users/b/code/flying-buttress-cli/`
- Key files:
  - `cmd/add.go` — `buttress add` implementation
  - `internal/store/store.go` — cache and lock file management
  - `internal/github/github.go` — spec fetching and hash computation
  - `internal/config/config.go` — config loading
  - `internal/generate/generate.go` — Generator interface (stubbed)
  - `go.mod` — dependency manifest
- Spec package format: defined in this document; no separate schema file exists yet
- Lock file format: JSON, defined by `store.LockEntry` struct in `internal/store/store.go`
