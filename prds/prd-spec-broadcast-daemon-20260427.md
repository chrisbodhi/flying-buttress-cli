# PRD: Spec Update Broadcast Daemon

**Status:** Draft

**Date:** 2026-04-27

**Author:** Flying Buttress Product

**Document type:** Architectural Exploration + Product Requirements

---

## Executive Summary

When an org member publishes a new spec version, other members who have that spec installed should be notified and offered the option to apply it. This document defines requirements for a background daemon that polls for updates and surfaces them via `buttress status` and a persistent banner on every CLI command.

Two long-term architectural models are evaluated (SaaS broadcast and peer gossip), but neither is built yet. The recommended immediate path is a minimal hybrid daemon with a pluggable `Watcher` interface that forecloses neither model and can ship within the current open-source, no-infrastructure constraints.

---

## Product Vision

Flying Buttress users should never be caught off-guard by a stale spec. The update broadcast system makes spec versioning a first-class, ambient concern — visible without being intrusive — so that breaking changes are surfaced before they cause divergence, and non-breaking updates can be applied with a single command.

The architecture is designed to preserve a future choice between a SaaS revenue model (Model A) and a fully decentralized open-source positioning (Model B) without committing to either prematurely.

---

## Target Users

**Primary:** Org members who have specs installed via `buttress add` and need to stay current with spec authors' published changes.

**Secondary:** Spec authors who publish new versions and need confidence that downstream consumers will be notified.

**Tertiary:** Org administrators who may need rollout control over breaking spec changes in the future (addressed by Model A / Phase 2).

---

## User Stories

1. As an org member, I want to see a banner when I run any `buttress` command if a spec I have installed has a new version available, so I am passively informed without having to check manually.
2. As an org member, I want to run `buttress status` to see all installed specs, their current hash, the available hash, whether the update is breaking, and the command to apply it.
3. As an org member, I want to run `buttress apply @org/pkg` to fetch, verify, and commit a pending update in one step.
4. As a spec author, I want to publish a new version by pushing to `VERSIONS.txt` on GitHub and have consumers be notified within the configured poll interval (default 2 hours).
5. As a power user, I want to configure poll interval, notification style (banner, system, silent), and daemon socket path in my project config.

---

## Success Metrics

- Daemon adoption: percentage of active CLI users who have daemon enabled after 30 days.
- Update awareness: reduction in "I didn't know there was a new version" support requests.
- Apply latency: time from spec publication to a consumer running `buttress apply` (target: within one poll cycle for 80% of users).
- Backward compatibility: zero regressions on existing `VERSIONS.txt` files that lack the optional third metadata line.

---

## MVP Scope (Phase 1: Hybrid Daemon)

The MVP ships the `GitHubPoller` watcher only. It introduces:

- `buttress daemon start|stop|status` subcommand.
- Background polling of `VERSIONS.txt` every 2 hours (configurable).
- `~/.cache/buttress/pending-updates.json` as the local pending-update store.
- A dim banner printed by `PersistentPreRun` in `cmd/root.go` when pending updates exist.
- `buttress status [@org/pkg]` displaying current hash, available hash, breaking flag, and apply command.
- `buttress apply [@org/pkg]` reusing the existing full add flow (fetch → verify → commit → lock).
- A `[daemon]` config section in the project TOML.
- A backward-compatible optional third metadata line in `VERSIONS.txt`.

The MVP explicitly does NOT include: SaaS backend, gossip protocol, system-level OS notifications, blue-green rollout controls, or a `buttress publish` workflow.

---

## Functional Requirements

### FR-1: Background Daemon

- The daemon runs as a persistent background process with a PID file.
- It communicates with the CLI via a Unix domain socket (IPC).
- It reads the installed spec lock on startup and on each poll tick.
- It writes discovered pending updates to `~/.cache/buttress/pending-updates.json`.
- It respects a `[daemon]` config section with at minimum: `enabled`, `poll_interval`, `notify`, `socket_path`.

### FR-2: Watcher Interface (Pluggable Seam)

The daemon uses a `Watcher` interface so that `SaaSSubscriber` (Model A) or `GossipListener` (Model B) can be added in Phase 2 without restructuring the daemon core.

```go
type Watcher interface {
    Watch(ctx context.Context, lock map[string]store.LockEntry, ch chan<- VersionEvent) error
}

type VersionEvent struct {
    Org, Pkg      string
    AvailableHash string
    Entry         ChangelogEntry
    Source        string // "github-poll" | "saas-push" | "peer-gossip"
}

type ChangelogEntry struct {
    Hash, Description, Author, MigrationNotes string
    PublishedAt time.Time
    Breaking    bool
}
```

Phase 1 ships only `GitHubPoller`, which wraps `github.Client.ListVersionsFromURL` in a ticker loop.

### FR-3: `buttress status` Command

Output format:

```
  @chrisbodhi/left-pad   sha256:7ffcaa7b  →  sha256:a1b2c3d4  [non-breaking]
    "Add handling for unicode input"
    run: buttress apply @chrisbodhi/left-pad

  @chrisbodhi/right-pad  sha256:deadbeef  (up to date)
```

- Shows all installed specs.
- For each spec with a pending update: current hash, available hash, breaking flag, description, apply command.
- For up-to-date specs: current hash and "(up to date)".
- Accepts optional `@org/pkg` argument to filter to a single spec.

### FR-4: `buttress apply` Command

- Accepts `@org/pkg` argument.
- Reuses the full existing add flow: fetch → verify → commit → lock.
- Clears the spec's entry from `pending-updates.json` after successful apply.
- Errors with a clear message if no pending update exists for the given spec.

### FR-5: Banner Hook

- `cmd/root.go` registers a `PersistentPreRun` hook.
- Hook reads `pending-updates.json`; if non-empty, prints a dim single-line banner: e.g., `  [buttress] 2 spec update(s) available — run buttress status`.
- Banner is suppressed when `notify = "silent"` in daemon config.
- Banner can be replaced with macOS/Linux system notifications when `notify = "system"` (Phase 2).

### FR-6: VERSIONS.txt Backward-Compatible Extension

Optional third line per entry for richer metadata:

```
sha256:a1b2c3d4
Add handling for unicode input.
breaking=false author=chrisbodhi ts=2026-04-27T10:00:00Z

sha256:7ffcaa7b
Initial spec and tests.
```

- The parser ignores the third line if absent.
- The parser is lenient — unknown keys on the third line are ignored.
- This is fully backward-compatible with all existing `VERSIONS.txt` files.

### FR-7: Config Extension

New `[daemon]` section in project TOML:

```toml
[daemon]
enabled       = false
poll_interval = "2h"
notify        = "banner"   # "banner" | "system" | "silent"
socket_path   = ""
```

- `enabled = false` by default (opt-in, not opt-out, for Phase 1).
- `socket_path` defaults to a platform-appropriate temp path if empty.

---

## Model A: SaaS Blue-Green Broadcast (Future Phase 2 Option)

A Flying Buttress backend service fans out notifications with blue-green rollout control.

- Spec authors publish to the service; service fans out to subscribers.
- Blue cohort (5%) gets the update first; green cohort (95%) receives after quorum validates.
- Enables cloud-side generation and validation.
- Revenue model: orgs pay for private specs, rollout controls, analytics.
- Privacy cost: service sees user identity and update timing.
- Implemented by adding a `SaaSSubscriber` watcher — no daemon restructuring required.
- Estimated build cost: 4–8 weeks (requires operating a service).

## Model B: Local Daemon + Peer Gossip (Future Phase 2 Option)

Each user runs `buttress daemon`. Daemons discover org peers via a `buttress-peers.json` convention file in the org's GitHub config repo.

- Rumor-mongering gossip protocol (SWIM/Cassandra-style, not full Raft) propagates version announcements.
- Content is always fetched from GitHub directly; peers share only metadata (hashes, descriptions, who applied what).
- ed25519 keys for message authentication.
- No central server; fully private.
- Higher setup friction (keygen, peer registration).
- Implemented by adding a `GossipListener` watcher — no daemon restructuring required.
- Estimated build cost: 3–6 weeks.

---

## Architectural Tradeoffs

| Dimension | Model A (SaaS) | Model B (Gossip) | Hybrid Daemon (Phase 1) |
|---|---|---|---|
| Infrastructure to operate | Yes | No | No |
| Setup friction per user | Low | High (keygen, peers.json) | Low |
| Notification latency | <1s (SSE) | gossip-speed | 2h (poll) |
| Blue-green rollout | Native | Approximate | No |
| Privacy | Service sees identity | Fully private | GitHub only |
| Revenue model | SaaS subscriptions | Open-source only | — |
| Survivability | Blocked if service down | Fully resilient | Resilient |
| Build cost | 4–8 weeks | 3–6 weeks | 4–6 weeks |

---

## Technical Considerations

### New Package Structure

```
internal/
  daemon/
    daemon.go         — Daemon struct, Run() loop, PID file, socket lifecycle
    pending.go        — PendingUpdate type, ~/.cache/buttress/pending-updates.json
    ipc.go            — Unix domain socket for CLI <-> daemon IPC
  watcher/
    watcher.go        — Watcher interface, VersionEvent, ChangelogEntry
    github_poller.go  — GitHubPoller: wraps github.Client.ListVersionsFromURL on a ticker
cmd/
  daemon.go           — buttress daemon start|stop|status
  status.go           — buttress status [@org/pkg]
  apply.go            — buttress apply [@org/pkg]
```

### Files Modified in Existing Codebase

- `internal/store/store.go`: add `LastCheckedAt time.Time` to `LockEntry`.
- `internal/versions/versions.go`: extend `Parse` with optional third metadata line per entry (`breaking=false author=jdoe ts=...`); lenient — ignored if absent.
- `internal/config/config.go`: add `[daemon]` section with `enabled`, `poll_interval`, `notify`, `socket_path`.
- `cmd/root.go`: add `PersistentPreRun` hook printing dim banner when `pending-updates.json` is non-empty.

### Existing Code Reused

- `github.Client.ListVersionsFromURL` — core of `GitHubPoller` (add a ticker around it).
- `store.ReadLock` / `store.WriteLock` — daemon reads installed state, writes `LastCheckedAt`.
- `store.Commit` — `buttress apply` reuses full add flow (fetch → verify → commit → lock).
- `tui.ShortHash` — used in status output.
- `versions.Parse` — extended, not replaced.

---

## Timeline

### Phase 1: Hybrid Daemon (4–6 weeks)

Deliver the `GitHubPoller`-only daemon, `buttress status`, `buttress apply`, banner hook, config extension, and VERSIONS.txt metadata extension. No infrastructure required.

- Week 1–2: Package scaffolding (`internal/daemon`, `internal/watcher`), `Watcher` interface, `GitHubPoller` implementation, store extension (`LastCheckedAt`).
- Week 3–4: `daemon.go` IPC and PID lifecycle, `pending.go` read/write, `cmd/daemon.go` subcommand.
- Week 5: `cmd/status.go`, `cmd/apply.go`, `cmd/root.go` banner hook, config extension.
- Week 6: `versions.Parse` extension, integration tests, documentation, opt-in rollout.

### Phase 2: Choose and Build Next Watcher (3–8 weeks, timeline depends on model choice)

Based on the open questions below, add either `SaaSSubscriber` (Model A) or `GossipListener` (Model B) as an additional watcher implementation. No changes to the daemon core or CLI commands required.

---

## Risks

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| Daemon process management is fragile across platforms (macOS launchd, Linux systemd, plain background) | Medium | Medium | Ship with explicit `start|stop|status` commands and a PID file; do not rely on init system integration in Phase 1 |
| Poll interval of 2h is too coarse for time-sensitive breaking changes | Medium | Medium | Make interval configurable; document that system notifications (`notify = "system"`) are a Phase 2 option |
| VERSIONS.txt third-line extension causes parser regressions on existing files | Low | High | Parser is lenient-by-design; add a dedicated regression test suite against a corpus of existing VERSIONS.txt fixtures |
| Users do not enable the daemon (opt-in default) and miss the feature | Medium | Low | Consider opt-out after Phase 1 adoption data is available; measure daemon enable rate |
| Choosing neither Model A nor Model B in Phase 2 leaves the Watcher interface unused | Low | Low | The interface has minimal maintenance cost even if unused; `GitHubPoller` alone delivers persistent value |
| Unix domain socket path conflicts or permission issues on multi-user systems | Low | Medium | Default to a user-scoped temp path; document socket_path override |

---

## Open Questions

1. **Business model**: Is Flying Buttress primarily a SaaS product (Model A revenue) or an open-source tool (Model B positioning)? This choice drives whether `SaaSSubscriber` or `GossipListener` is the next Watcher implementation and should be resolved before Phase 2 begins.

2. **Notification UX**: Is the passive banner sufficient, or do users need system-level notifications (macOS/Linux) for time-sensitive breaking changes? If system notifications are needed, should that be in Phase 1 or Phase 2?

3. **`buttress-peers.json` convention**: If Model B is pursued, where does the peers file live? Options: per-org GitHub repo, per-project `.buttress/`, or a registry-hosted endpoint.

4. **VERSIONS.txt metadata documentation**: Should the third-line extension be documented as a spec-author convention now, or wait until a `buttress publish` workflow exists? Publishing without tooling support may create inconsistent adoption.

5. **Blue-green approximation in Model B**: Can blue-green rollout be approximated in Model B via a version vector quorum signal, or is that a fundamentally different guarantee that only Model A can provide?

6. **Daemon default**: Should `enabled = false` remain the default after Phase 1 ships, or should it flip to opt-out once adoption data is available?

---

## Analytics Implementation

Phase 1 analytics (CLI-local, no telemetry service required):

- Log daemon start/stop events with timestamp to a local log file.
- Record `last_checked_at` per spec in `LockEntry` to measure poll cadence.
- Record `applied_at` in lock after `buttress apply` to measure update latency (publication → apply).

Phase 2 analytics (requires opt-in telemetry or SaaS backend if Model A is chosen):

- Update awareness rate: percentage of users who saw a banner and ran `buttress status`.
- Apply rate: percentage of pending updates that were applied within 24h of discovery.
- Breaking change flag accuracy: user-reported regressions on updates flagged `breaking=false`.
