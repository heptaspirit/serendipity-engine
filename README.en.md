# Serendipity Engine

<p align="center"><img src="docs/logo.png" alt="Serendipity Engine" width="160"></p>

> Graph roaming: an activation engine on top of your personal wiki's backlinks — **ask a point, get a cluster.**
>
> White-box, local, pure-Go zero-dependency. One structural signal, two consumers: **humans** roam for inspiration, **agents** skip the crawl and consume clusters / evidence chains / weight distributions directly.

[![Version](https://img.shields.io/badge/version-v0.1.12-7aa2f7)](https://github.com/heptaspirit/serendipity-engine/tags) [![License](https://img.shields.io/badge/License-MIT-9cf)](LICENSE) [![Go](https://img.shields.io/badge/Go-1.26%2B-00ADD8)](go.mod) [![Pure Go](https://img.shields.io/badge/Pure%20Go-Zero%20CGO-4c566a)](go.mod) [![Single Binary](https://img.shields.io/badge/Single%20Binary-✅-7aa2f7)](https://github.com/heptaspirit/serendipity-engine/releases) [![Local-first](https://img.shields.io/badge/Local--first-✅-7aa2f7)](https://github.com/heptaspirit/serendipity-engine) [![MCP Server](https://img.shields.io/badge/MCP%20Server-AI%20ready-7aa2f7)](https://github.com/heptaspirit/serendipity-engine) [![Leiden](https://img.shields.io/badge/Leiden-community%20detection-4c566a)](https://github.com/heptaspirit/serendipity-engine) [![i18n](https://img.shields.io/badge/i18n-ZH%2FEN-7aa2f7)](https://github.com/heptaspirit/serendipity-engine) [![Top Language](https://img.shields.io/github/languages/top/heptaspirit/serendipity-engine)](https://github.com/heptaspirit/serendipity-engine) [![English](https://img.shields.io/badge/English-README.en-7aa2f7)](README.en.md) [![简体中文](https://img.shields.io/badge/简体中文-README-7aa2f7)](README.md)

## Features

- **Query-driven roaming**: type a note name / tag / any word → get a filtered, ranked, explainable cluster of related nodes (with activation paths)
- **🎲 Random walk**: "just wander around" when you don't know what to ask — a random start node + its cluster in one shot (quality-gate filtering + degree weighting + anti-repeat + reproducible seed)
- **Serendipity mechanism**: hop-quota mixing (1:2:3-hop = 50/30/20) so surprising "I didn't think of it, but it's related" deep-hop nodes appear steadily
- **White-box & interactive**: every recommendation is explainable (activation path), click-to-roam, and can jump back to the original note app
- **Two data sources**: Obsidian vault (file parsing) + Orca Note (SQLite snapshot read; the credentials table is **never** touched)
- **Reconciliation**: `seren refresh` / Web ↻ / auto-watch keep the graph in sync with add/edit/delete (throttled merging — deliberately restrained against feedback loops)
- **Relation query**: shortest path + bidirectional PPR strength + evidence chain between any two nodes (white-box)
- **Structural similarity**: node pairs with many common neighbors but no direct link (Adamic-Adar degree-weighted, with shared-neighbor evidence) — a pure-structure substitute for the embedding semantic axis
- **Roam export**: `/api/roam?export=1` → Markdown card list, so discoveries can be captured back into your notes
- **Community detection (Leiden)**: split the graph into topic clusters (`/api/communities` + MCP `graph.community`) — agents localize knowledge gaps without crawling the whole library (diagnosis layer)
- **Refresh consistency & UX**: `is_pending` pre-refresh hint + manual instant refresh; dangling-link details (`dangling_refs`) + ghost-touch filtering
- **LLM Wiki compatible**: `--profile-name llm-wiki` (excludes `raw/` and `index.md/log.md`; real links, content-credibility caveat)
- **Frontend P0 (plugin prep)**: compact embed `?embed=1` + postMessage bridge (`{type:'open'}`) + i18n (ZH/EN all user-visible copy)
- **Five entry points**: CLI / REST + Web UI / MCP (`seren mcp`, AI channel, seven read-only tools) / CLI subcommand help + `--json` structured output

## Design Philosophy

1. **Structure × Activation**: the graph structure provides "possibly relevant"; activation provides "relevant right now" — a wiki with only structure is dead; the activation engine makes it alive.
2. **White-box**: every recommendation is explainable, interactive, and can jump back to the source app. No black boxes.
3. **Parsing extracted**: generic syntax is fixed in code; semantic mapping (title/type rules, etc.) lives in YAML profiles — change vaults without changing code.
4. **Restraint by design**: watching uses polling + throttled merging; telemetry only records, never evolves — any "click → weight → result" positive-feedback loop is cut off at the source. A local tool should be stable first.
5. **Security red lines**: Orca credential tables are never read; live DBs are snapshotted before reading; personal data never enters git.

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
(pure Go, zero CGO) + `github.com/vsuryav/leiden-go` (MIT, community detection, go.sum-pinned),
no network egress.
Maintainer-facing architecture docs live in `docs/architecture/`.

## Quick Start

```powershell
# Build (Go 1.26+)
go build -o seren.exe ./cmd/seren

# Roam
.\seren.exe roam <vault> "search"                # Obsidian vault
.\seren.exe roam "D:\...\OrcaNote.db" "history"   # Orca DB (.db auto-detected)
.\seren.exe roam <vault> --random --seed 42        # 🎲 random walk (--seed = reproducible)

# Web UI (auto-watch on by default; Obsidian: add --vault-name, Orca: orca-note:// jump)
.\seren.exe serve <vault> --port 8080

# Reconcile (sync after add/edit/delete; prints added/updated/deleted)
.\seren.exe refresh <vault> --store <file.sqlite>

# MCP (AI channel, seven read-only tools; point a stdio MCP client at this)
.\seren.exe mcp <vault> --db <file.sqlite>

# Community detection (Leiden, diagnosis layer)
.\seren.exe serve <vault> --port 8080   # open http://127.0.0.1:8080/api/communities

# Subcommand help + structured output (CLI triplet)
.\seren.exe help roam          # per-subcommand help (or .\seren.exe roam -h)
.\seren.exe roam <vault> "word" --json   # structured JSON (agent-consumable)
```

LLM Wiki vault profile: `.\seren.exe roam <llm-wiki-vault> "word" --profile-name llm-wiki`.

## Docs

| Doc | Description |
|---|---|
| [`docs/README.md`](docs/README.md) | Doc navigation (topic-layered index) |
| [`docs/architecture/`](docs/architecture/) | Architecture docs (for maintainers): overview / data model / adapters / engine / sync / web / maintenance / MCP study |
| [`docs/design.md`](docs/design.md) | Core design: graph-roaming mechanics, 4-dimension scoring (PPR + activation + hop quota), stack & product form |
| [`docs/positioning.md`](docs/positioning.md) | Strategic positioning: notes-as-agent-memory "activation layer", LLM Wiki complementarity, boundaries & non-goals |
| [`docs/roadmap.md`](docs/roadmap.md) | Master roadmap: Phase 1 engine core + Web UI polish (self-use) / 2 plugin shells (M2), with dependency chain & status |
| [`docs/frontend.md`](docs/frontend.md) | Frontend plan (Web UI): plugin prep + UI/UX polish spec + test quick-reference |
| [`docs/backend-backlog.md`](docs/backend-backlog.md) | Backend backlog: perf optimizations, similar/export/touch stats, CLI & MCP polish |
| [`docs/api-contract.md`](docs/api-contract.md) | API contract: 11 endpoints + auth (the only shared artifact between the plugin repo and the engine) |
| [`docs/history/`](docs/history/) | Archived decisions/verifications (content absorbed into design/roadmap; full narrative retained) |

## Special Thanks

- **[dsh-mneme](https://github.com/modusensus/dsh-mneme)** — the philosophical source of the activation engine: structure × activation, spreading activation, white-box principle. The same engine with a different carrier and a different consumer is where this project started.
- **[Dinosaur Toolbox](https://github.com/hqweay/orca-hqweay-go) (Orca Note plugin)** — inspiration for the SRS review-roam and random-walk interactions.
- **[leiden-go](https://github.com/vsuryav/leiden-go)** (MIT) — community-detection (Leiden) third-party library, go.sum-pinned.
- **[graphwizard](https://github.com/intelligrit/graphwizard)** (MIT) — graph-algorithm learning reference (Adamic-Adar similarity / community detection / structure analysis); the actual implementation here is self-written, no dependency pulled in.

## License

MIT License — see [LICENSE](LICENSE).
