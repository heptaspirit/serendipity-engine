# Serendipity Engine · 奇遇记引擎

> **v0.1.4** · Graph roaming: query-driven navigation over your personal notes — **ask a point, get a cluster.**
> An activation engine (query-anchored PPR + spreading activation + hop-quota) bolted onto the
> bi-directional links of your notes, so the wiki "actually comes alive".
> English README; [中文](README.md).

> 🙏 **Inspiration**: the graph-enhancement philosophy of this project (**structure × activation**,
> spreading activation, white-box principle) shares its roots with [dsh-mneme](https://github.com/modusensus/dsh-mneme).
> The philosophy is my own, distilled while contributing to mneme's graph-enhancement design — the idea being:
> the same activation engine, a different carrier and a different consumer, becomes "making note navigation
> alive". Same philosophy, different domain: mneme manages agent memory; this engine manages **human note navigation**.

## Features

- **Query-driven roaming**: type a note name / tag / any word → get a filtered, ranked, explainable cluster
  of related nodes (with activation paths)
- **Serendipity mechanism**: hop-quota mixing (1:2:3-hop = 50/30/20) so surprising "I didn't think of it,
  but it's related" deep-hop nodes appear steadily
- **White-box & interactive**: every recommendation is explainable (activation path), click-to-roam,
  and can jump back to the original note app
- **Parsing rules extracted**: VaultProfile per-vault profile (title/alias/tag/type rules in YAML) +
  `profile-detect` onboarding for new vaults — change vaults without changing code; Google's OKF
  (Open Knowledge Format) is built into the default parsing
- **Two data sources**: Obsidian vault (file parsing) + Orca Note (SQLite snapshot read; the Repo
  credentials table is **never** touched)
- **Reconciliation**: `seren refresh` / Web `↻` / **auto-watch** keep the graph in sync with
  add/edit/delete (60s throttled merging — deliberately restrained against feedback loops)
- **Feedback telemetry**: clicks are recorded as touches (dedicated table, capped size; **v1 does
  NOT evolve edge weights**, cutting off runaway feedback at the source)
- **Jump back**: Obsidian `obsidian://` / Orca `orca-note://` URIs on cards
- **Three entry points**: CLI / REST + Web UI / (future MCP)

## Design Philosophy

1. **Structure × Activation**: the graph structure provides "possibly relevant"; activation provides
   "relevant right now" — a wiki with only structure is dead; the activation engine makes it alive.
2. **White-box**: every recommendation is explainable, interactive, and can jump back to the source
   app. No black boxes.
3. **Parsing extracted**: "a parsing scheme is not universal" — generic syntax is fixed in code;
   semantic mapping (title/type rules, etc.) lives in YAML profiles.
4. **Restraint by design**: watching uses polling + throttled merging and excludes its own artifacts;
   telemetry only records, never evolves — any "click → weight → result" positive-feedback loop is
   cut off at the source. A local tool should be stable first.
5. **Security red lines**: Orca credential tables are never read; live DBs are snapshotted before
   reading; personal data never enters git.

## Architecture Overview

```
cmd/seren (CLI: index/roam/serve/refresh/profile-detect)
   │ loadSource / parseSource (--db > Orca .db > Obsidian vault)
   ▼
adapter (format translation: Document / Obsidian / Orca / VaultProfile / snapshot)
   │ []*Document
   ▼
graph (in-memory adjacency: Build/Resolve/PPR/Activate/TextSearch)
   ▼
score + roam (normalized fusion + hop-quota / roam pipeline: anchor→spread→exclude→fallback)
   ▼
store (SQLite: documents/links/touch) · sync (reconcile diff) · watch (auto-watch) · web (REST + frontend)
```

Minimal dependencies: the standard library + `gopkg.in/yaml.v3` + `modernc.org/sqlite`
(pure Go, zero CGO), no network egress.
**Maintainer-facing architecture docs live in `docs/architecture/`** (data model / adapters /
engine / sync / web / maintenance guide).

## Quick Start

```powershell
# Build (Go 1.26+)
go build -o seren.exe ./cmd/seren

# Roam
.\seren.exe roam <vault> "search"              # Obsidian vault
.\seren.exe roam "D:\...\OrcaNote.db" "history" # Orca DB (.db auto-detected)

# Web UI (hero page with floating hot-node bubbles; auto-watch on by default)
.\seren.exe serve <vault> --port 8080
#   Obsidian: add --vault-name to enable the "Open ↗" jump back to the app
#   Orca: jump uses orca-note:// automatically (override with --repo)
#   --watch-off disables auto-watch; --watch-interval / --watch-throttle tune frequency

# Reconcile (sync after you add/edit/delete notes; prints added/updated/deleted)
.\seren.exe refresh <vault> --store <file.sqlite>

# New vault onboarding
.\seren.exe profile-detect <unknown-vault>     # auto-generate a profile YAML

# Persistence (avoid re-parsing)
.\seren.exe index <vault> --persist
.\seren.exe roam <vault> "query" --db <store>
```

## Docs

| Doc | Description |
|---|---|
| **`docs/architecture/`** | **Architecture docs (for future maintainers)**: 00 overview & philosophy / 01 data model / 02 adapters / 03 engine / 04 sync / 05 web / 06 maintenance guide |
| [`docs/design.md`](docs/design.md) | Design document (rev. v2: review decisions + spike findings + VaultProfile) — the author's design-process record |
| [`docs/DESIGN_REVIEW.md`](docs/DESIGN_REVIEW.md) | 13 accepted design-review decisions |
| [`docs/spike-report.md`](docs/spike-report.md) | Spike verification report (mechanism conclusions / parameter re-measurement, sanitized) |
| [`docs/product-form.md`](docs/product-form.md) | Product-form decision (jump-back vs plugin vs MCP) |

> Dev log & open issues live in the local `PROGRESS_LOG.md` (not committed).

## Status

v0.1.4 (2026-08-21): reconciliation, auto-watch, touch telemetry, and Orca jump-back are done.
Next: feedback-loop observation (touch data), qualitative validation with a real query set,
Playwright frontend test automation, MCP server.

## License

MIT License — see [LICENSE](LICENSE).
