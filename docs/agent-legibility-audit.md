# Agent-Legibility Audit: flying-buttress-cli

Evaluated against Armin Ronacher's criteria from *The Friction Is Your Judgment*
(AI Engineer London, April 2026). Source: https://mitsuhiko.github.io/talks/ai-engineer-talk/

---

## Summary

| Criterion | Status |
|---|---|
| Modularized with clear boundaries | ✅ Meets |
| Known patterns and enforced conventions | ⚠️ Partial |
| Simple core, complexity in upper layers | ✅ Meets |
| Visible mechanics — no hidden magic | ✅ Meets |
| No bare catch-alls | ✅ Meets |
| Enforced abstraction layers | ⚠️ Partial |
| No dynamic dispatch / dynamic imports | ✅ Meets |
| Unique function names (discovery > duplication) | ⚠️ Partial |
| Entropy resistance | ⚠️ Partial |
| Agent-assisted PR review: automated gates exist | ❌ Lacking |

**6 fully met · 3 partially met · 1 lacking**

---

## Criteria

### 1. Modularized with Clear Boundaries ✅

Twelve packages with single responsibilities. `cmd/` owns TUI output and flag parsing; it orchestrates but does not implement. All business logic lives in `internal/`. Leaf packages (`internal/ref`, `internal/versions`, `internal/config`) have no imports from other internal packages.

The layer separation is documented in CLAUDE.md and enforced structurally — there is no cyclic import to break.

**Gaps:** None significant.

---

### 2. Known Patterns and Enforced Conventions ⚠️

The patterns are consistent and good:

- Dependency injection via `*Deps` structs (`addDeps`, `configDeps`) — no global state.
- Interface-based service design (`specClient`, `Generator`, `Sandbox`).
- `fakeXxx` test doubles wired via fixture functions.
- `fmt.Errorf("context: %w", err)` error wrapping throughout.
- Two-phase operations: `Plan()` (pure, no I/O) → `Write()` (applies to disk).

The problem is that none of these are enforced by tooling. They survive because the codebase is small and the author is consistent. An agent producing new code has no mechanical feedback if it deviates — no linter rejects a cmd/ file that calls `internal/github` directly, no CI step flags a missing error wrap.

**Gaps:**
- No `.golangci.yml`.
- No `depguard` or `go-arch-lint` import rules.
- No custom vet passes.

**Fix:** See [Priority Improvements §1](#1-add-golangci-lint-to-ci).

---

### 3. Simple Core, Complexity in Upper Layers ✅

Core packages handle parsing and data modeling only, with 100% test coverage:

- `internal/ref` — parses `@org/pkg[@hash]` strings.
- `internal/versions` — parses VERSIONS.txt format.
- `internal/config` — reads/writes `~/.config/buttress/config.toml`.

Complexity is correctly above: `internal/generate` manages the LLM loop with bounded context growth; `internal/sandbox` handles platform-specific container execution; `cmd/` orchestrates them.

The two-phase scaffolding pattern (`scaffold.Plan()` is a pure function; `Write()` applies to disk) demonstrates this principle at the sub-package level.

**Gaps:** None significant.

---

### 4. Visible Mechanics — No Hidden Magic ✅

- No `init()` side effects.
- No global state (all deps injected via structs).
- No reflection-based wiring.
- No implicit middleware or hook chains.
- Config file path is explicit and documented.
- Hard-link-with-copy-fallback behavior is documented in CLAUDE.md.

An agent reading `cmd/add.go` sees the complete dependency surface from the `addDeps` struct — no invisible setup required.

**Minor gap:** The hash format transition (`sha256:` in VERSIONS.txt → `sha256-` in git tag names) is documented in CLAUDE.md but not expressed as named constants in code. An agent generating new code that constructs tag names has to know this implicitly.

**Fix:** Define `const sha256Prefix = "sha256:"` and `sha256TagPrefix = "sha256-"` alongside the hash logic in `internal/contenthash` or `internal/versions`.

---

### 5. No Bare Catch-alls ✅

Error handling is explicit throughout:

```go
return fmt.Errorf("loading config: %w", err)
```

Sentinel errors signal control flow:

```go
if errors.Is(err, generate.ErrNotConfigured) { ... }
```

Early validation prevents silent failures at the boundary that matters:

```go
if doGenerate && !cfg.HasLLM() {
    return errors.New("--generate requires LLM credentials in ~/.config/buttress/config.toml")
}
```

No `recover()` calls. No `_ = err` suppressions observed. Missing config returns an empty `Config{}` by design (documented), not a suppressed error.

**Gaps:** None observed. This is the strongest area in the codebase.

---

### 6. Enforced Abstraction Layers ⚠️

Abstraction layers exist and are correctly designed:

- `cmd/add.go` uses `specClient` interface — no direct `github.Client` in command code.
- `cmd/generate.go` uses `Generator` interface — no direct LLM HTTP calls in commands.
- `internal/sandbox` exposes `Sandbox` interface — Apple and Docker backends are not visible to callers.

The problem is that these are conventions, not constraints. Nothing mechanical prevents a future `cmd/` file from importing `internal/github` directly. CI does not run import graph checks.

**Gaps:**
- No `depguard` rules.
- No `go-arch-lint` or equivalent architectural fitness function.
- The abstraction layers are invisible to an agent reading a single file in context.

**Fix:** See [Priority Improvements §2](#2-enforce-import-layer-rules).

---

### 7. No Dynamic Dispatch / Dynamic Imports ✅

Go's static compilation eliminates dynamic imports by design. The equivalent concerns — runtime plugin loading, `reflect`-based dispatch, `interface{}` abuse — are absent:

- No `plugin` package usage.
- No `reflect.Value` dispatch chains.
- Interface usage is bounded (known implementors, not open extension points for runtime loading).

**Gaps:** None. Language design handles this criterion.

---

### 8. Unique Function Names (Discovery > Duplication) ⚠️

Package namespacing provides natural disambiguation, and no obvious duplication was found. The risk is structural, not current: Ronacher's concern is that agents reading partial context reinvent utilities rather than discovering existing ones. Generic export names accelerate this failure mode.

Current examples of names that could cause ambiguity for an agent reading a single package:
- `internal/ref` exports `Parse` — ambiguous without the package qualifier.
- `internal/versions` exports `Parse` — same.

An agent holding only one of these files in context cannot know the other `Parse` exists.

**Gaps:**
- Generic export names (`Parse`, `New`) are common.
- No `go doc` package comments on most `internal/` packages to give agents a fast orientation.

**Fix:** See [Priority Improvements §3](#3-rename-generic-exports-and-add-package-doc-comments).

---

### 9. Entropy Resistance ⚠️

The codebase has strong foundations against entropy:
- High coverage in core packages (100% for `ref`, `versions`, `config`, `localconfig`).
- Interface-based design makes regressions visible.
- Atomic writes prevent lock file corruption.
- DI structs prevent global state accumulation.

But the coverage floor is drifting in exactly the packages where agents are most likely to make mistakes:

| Package | Coverage | Risk |
|---|---|---|
| `internal/specmeta` | 62.5% | Metadata parsing edge cases untested |
| `cmd/add` | 72% | Primary user-facing path |
| `internal/generate` | 72.3% | LLM loop logic |
| `internal/sandbox` | 75.2% | Platform-specific container execution |

Ronacher's formulation: *"Messier codebase → worse agent recall → more duplication → more mess."* A coverage gap is not just a quality metric — it is a map of where agents will confidently produce wrong code that passes CI.

There is no coverage gate in CI. The badge updates automatically but does not fail the build.

**Gaps:**
- No minimum coverage threshold in CI.
- No `golangci-lint` step in CI workflow.

**Fix:** See [Priority Improvements §1](#1-add-golangci-lint-to-ci) and [§4](#4-add-coverage-gate-to-ci).

---

### 10. Agent-Assisted PR Review: Automated Gates Exist ❌

Ronacher's PR review framework distinguishes:

- **Agent-handled (immediately actionable):** style violations, mechanical bugs, clear engineering rule breaks.
- **Human-required (call-out):** new dependencies, auth/permissions changes, backwards-incompatible API changes, irreversible destructive operations.

The current CI pipeline (`go vet`, `gofmt -l`, `go test -race`) catches correctness issues but provides no automated style or convention enforcement beyond formatting. There is no mechanism to flag high-risk changes (new external dependencies, changes to the lock file format, sandbox execution path changes) for mandatory human review.

An agent submitting a PR that adds a direct HTTP call in `cmd/` and drops test coverage on `internal/generate` to 60% would pass CI today.

**Gaps:**
- No `golangci-lint` in CI (no style/convention enforcement).
- No PR labeling or automated review requests for high-risk change patterns.
- No CODEOWNERS or required-reviewer rules for `internal/sandbox`, `internal/generate`.

**Fix:** See [Priority Improvements §1](#1-add-golangci-lint-to-ci), [§2](#2-enforce-import-layer-rules), and [§5](#5-add-codeowners-for-high-risk-packages).

---

## Priority Improvements

These are ordered by leverage against the entropy spiral. The first two have an outsized effect because they convert conventions into constraints — the distinction Ronacher draws between "patterns" and "enforced patterns."

---

### 1. Add golangci-lint to CI

**What to do:**

Add `.golangci.yml` to the repo root and a CI step after `go vet`.

First, add the installer to `.github/workflows/ci.yml` before the vet step:

```yaml
- name: Lint
  uses: golangci/golangci-lint-action@v6
  with:
    version: latest
```

Then add `.golangci.yml` to the repo root with a minimal starting config:

```yaml
linters:
  enable:
    - errcheck    # catches _ = err suppressions
    - wrapcheck   # enforces fmt.Errorf wrapping at package boundaries

linters-settings:
  wrapcheck:
    ignorePackageGlobs:
      - "errors"
      - "fmt"

# Roadmap (enable incrementally, one at a time):
# - gocritic     # style issues
# - depguard     # import layer rules (see §2)
# - noctx        # HTTP calls without context
# - exhaustive   # switch statements cover all enum values
```

**Design note:** Enabling many linters at once produces noise that gets suppressed. Two linters that always pass are better than ten with a `nolint` comment on every third line.

**Test:** CI fails on a PR that introduces `_ = someFunc()`.

---

### 2. Enforce Import Layer Rules

**What to do:**

First, enable `depguard` in the `linters.enable` list created in §1. Then add the following to the `linters-settings:` block in the same `.golangci.yml`:

```yaml
  depguard:
    rules:
      cmd-no-direct-internals:
        files:
          - "**/cmd/**/*.go"
        deny:
          - pkg: "github.com/chrisbodhi/flying-buttress-cli/internal/github"
            desc: "cmd/ must use the specClient interface, not github.Client directly"
          - pkg: "github.com/chrisbodhi/flying-buttress-cli/internal/store"
            desc: "cmd/ must use the store through addDeps, not directly"
```

**Design note:** This encodes an architecture decision that currently exists only in CLAUDE.md. Making it a lint rule means an agent that violates it gets immediate CI feedback.

**Test:** `golangci-lint run ./cmd/...` fails on a file that imports `internal/github` directly.

---

### 3. Rename Generic Exports and Add Package Doc Comments

**What to do:**

Two changes, independent but related:

**Rename generics:**
- `internal/ref`: `Parse` → `ParsePackageRef`
- `internal/versions`: `Parse` → `ParseVersionsFile`

This is a one-commit change with a global find-and-replace. The names are more specific, reducing the probability an agent writes a new `Parse` function in one of these packages because it didn't know one existed.

**Add package doc comments** to every `internal/` package:

```go
// Package ref parses @org/pkg[@hash] package reference strings into PackageRef values.
// The canonical format is @org/pkg or @org/pkg@sha256:abc123.
package ref
```

These appear in `go doc` output and are the first thing an agent (or a human) reads when exploring the package. Currently most packages have none.

**Test:** `go build ./...` confirms no broken call sites after the rename. `go doc ./internal/...` shows a non-empty description for every package.

---

### 4a. Write Tests to Reach 80% in Under-Covered Packages

**Prerequisite for §4b.** Four packages are currently below the threshold:

| Package | Current | Gap |
|---|---|---|
| `internal/specmeta` | 62.5% | Metadata parsing edge cases |
| `cmd/add` | 72% | Primary user-facing path |
| `internal/generate` | 72.3% | LLM loop iteration logic |
| `internal/sandbox` | 75.2% | Docker and Apple container backends |

Write tests for each before adding the gate — otherwise CI fails on day one.

**Test:** `go test -cover ./internal/specmeta ./cmd/add ./internal/generate ./internal/sandbox` shows each package at ≥ 80%.

---

### 4b. Add Coverage Gate to CI

**What to do:**

Replace the existing coverage step in `.github/workflows/ci.yml` with one that checks per-package coverage, not just the aggregate total:

```yaml
- name: Test with coverage
  run: |
    go test -coverprofile=coverage.out ./...
    go tool cover -func=coverage.out | tee coverage.txt
    # Check per-package lines (not total:), fail if any package < 80%
    awk '!/^total:/ && /[0-9]+\.[0-9]+%$/ {
      pct = substr($NF, 1, length($NF)-1) + 0
      if (pct < 80.0) { print "FAIL: " $0; failed=1 }
    } END { exit failed }' coverage.txt
```

**Design note:** Checking only `total:` masks per-package failures — a package at 40% can hide behind packages at 100%. Per-line checking catches the actual problem.

**Test:** A PR that removes tests from `internal/generate` fails CI. A PR that improves `internal/specmeta` to 85% passes.

---

### 5. Add CODEOWNERS for High-Risk Packages

**What to do:**

Create `.github/CODEOWNERS`:

```
# Changes to these packages require explicit human review.
# They involve platform-specific execution, LLM integration,
# or the lock file format — all high-consequence, hard-to-reverse.

/internal/sandbox/    @chrisbodhi
/internal/generate/   @chrisbodhi
/internal/store/      @chrisbodhi
```

This implements the Ronacher PR review framework directly: agent-produced changes to these paths will not auto-merge and will require a human reviewer. It is a low-effort, high-signal change.

**Test (manual):** Open a draft PR touching `internal/sandbox/` and confirm `@chrisbodhi` appears as a required reviewer in the GitHub UI before merge is unblocked.

---

### 6. Expose Hash Format as Named Constants

**What to do:**

In `internal/contenthash` or `internal/versions`, define:

```go
const (
    HashPrefix    = "sha256:"   // format used in VERSIONS.txt
    HashTagPrefix = "sha256-"   // format used as git tag names (colons illegal in tags)
)
```

Reference these constants wherever tag names are constructed or parsed. This surfaces an implicit invariant as explicit, searchable code. An agent constructing a tag name will find the constant rather than guessing the format.

**Test:** `grep -r "sha256-"` in non-constant context returns zero results.

---

## What Good Looks Like

After the above improvements:

- An agent producing a PR that violates import layers, drops coverage, or suppresses errors gets immediate CI feedback — not a code review comment days later.
- CODEOWNERS ensures a human reviews changes to sandbox and generate logic regardless of how confident the agent's commit message sounds.
- Named constants and specific export names reduce the surface area for context-window-induced hallucination.
- The codebase remains as structurally clean as it is today, but the cleanliness is load-bearing rather than fragile.

The core architecture — content addressing, interface-based services, two-phase scaffolding, DI via structs — is already good. The gap is that "good by convention" and "good by enforcement" are different things when agents are writing the next 10,000 lines.
