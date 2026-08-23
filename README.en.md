# Serendipity Engine

> Graph roaming: an activation engine on top of your personal wiki's backlinks — **ask a point, get a cluster.**

[![Version](https://img.shields.io/badge/version-v0.1.8-7aa2f7)](https://github.com/heptaspirit/serendipity-engine/tags)
[![License](https://img.shields.io/badge/License-MIT-9cf)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.26%2B-00ADD8)](go.mod)
[![Pure Go](https://img.shields.io/badge/Pure%20Go-Zero%20CGO-4c566a)](go.mod)

English · [简体中文](README.md)

## Features

- **Query-driven roaming**: type a note name / tag / any word → get a filtered, ranked, explainable cluster of related nodes (with activation paths)
- **🎲 Random walk**: "just wander around" when you don't know what to ask — a random start node + its cluster in one shot (quality-gate filtering + degree weighting + anti-repeat + reproducible seed)
- **Serendipity mechanism**: hop-quota mixing (1:2:3-hop = 50/30/20) so surprising "I didn't think of it, but it's related" deep-hop nodes appear steadily
- **White-box & interactive**: every recommendation is explainable (activation path), click-to-roam, and can jump back to the original note app
- **Two data sources**: Obsidian vault (file parsing) + Orca Note (SQLite snapshot read; the credentials table is **never** touched)
- **Reconciliation**: `seren refresh` / Web ↻ / auto-watch keep the graph in sync with add/edit/delete (throttled merging — deliberately restrained against feedback loops)
- **Relation query**: shortest path + bidirectional PPR strength + evidence chain between any two nodes (white-box)
- **Three entry points**: CLI / REST + Web UI / MCP (planned)

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
(pure Go, zero CGO), no network egress.
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
```

## Docs

| Doc | Description |
|---|---|
| [`docs/architecture/`](docs/architecture/) | Architecture docs (for maintainers): overview / data model / adapters / engine / sync / web / maintenance / MCP study |
| [`docs/roadmap.md`](docs/roadmap.md) | Roadmap: M0 security + MCP / M1 core polish / M2 plugin shells |
| [`docs/design.md`](docs/design.md) | Design process record (review decisions + spike findings) |

## Special Thanks

- **[dsh-mneme](https://github.com/modusensus/dsh-mneme)** — the philosophical source of the activation engine: structure × activation, spreading activation, white-box principle. The same engine with a different carrier and a different consumer is where this project started.
- **[Dinosaur Toolbox](https://github.com/hqweay/orca-hqweay-go) (Orca Note plugin)** — inspiration for the SRS review-roam and random-walk interactions.

## License

MIT License — see [LICENSE](LICENSE).
