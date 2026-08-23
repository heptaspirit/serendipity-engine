# Serendipity Engine · 奇遇记引擎

> 图谱漫游：给个人笔记的双链装上激活引擎——**你问一个点，它给你一片。**

[![版本](https://img.shields.io/badge/%E7%89%88%E6%9C%AC-v0.1.10-7aa2f7)](https://github.com/heptaspirit/serendipity-engine/tags)
[![License](https://img.shields.io/badge/License-MIT-9cf)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.26%2B-00ADD8)](go.mod)
[![纯 Go](https://img.shields.io/badge/%E7%BA%AF%20Go-%E9%9B%B6%20CGO-4c566a)](go.mod)

[English](README.en.md) · 简体中文

## 特性

- **查询驱动漫游**：输入笔记名 / 标签 / 任意词 → 输出一批筛选、排序、可解释的相关节点簇（带激活路径）
- **🎲 随机漫步**：不想打字时"随便逛逛"——随机起点 + 它的簇一次给出（质量门槛过滤 + 度加权 + 防重复 + 可复现种子）
- **serendipity 机制**：跳数配额混合（1:2:3-hop = 50/30/20），"我没想到但确实相关"的深跳惊喜稳定出现
- **白盒可干预**：每条推荐可解释（激活路径）、可点击续漫游、可跳回原笔记软件
- **双数据源**：Obsidian vault（文件解析）+ 虎鲸 Orca Note（SQLite 快照直读，凭据表绝不碰）
- **对账刷新**：`seren refresh` / Web ↻ / 自动监听三路同步增删改（节流合并，克制防跑飞）
- **关系查询**：任意两节点的最短路径 + 双向 PPR 强度 + 证据链（white-box）
- **四入口**：CLI / REST + Web UI / MCP（`seren mcp`，AI 通道）

## 设计哲学

1. **结构 × 激活**：图结构提供"可能相关"，激活机制提供"此刻相关"——只有结构没有激活的 wiki 是死的。
2. **白盒原则**：每条推荐可解释、可干预、可跳回原软件，不做黑盒。
3. **解析抽离**：通用语法固定，语义映射（title/类型规则）YAML 画像化，换库不改代码。
4. **克制设计**：监听节流合并、埋点只记录不演化——任何"点击→边权→结果"的正反馈循环在源头切断，本地工具优先稳定。
5. **安全红线**：虎鲸凭据表绝不读取；活库先一致性快照再读；个人数据不进 git。

## 架构总览

```
cmd/seren (CLI: index/roam/serve/refresh/profile-detect)
   │ loadSource / parseSource（--db > 虎鲸 .db > Obsidian vault）
   ▼
adapter（格式翻译：Document / Obsidian / Orca / VaultProfile / 快照）
   │ []*Document
   ▼
graph（内存邻接表：Build/Resolve/PPR/Activate/TextSearch）
   ▼
score + roam（归一化融合 + 跳数配额 / 漫游管线：锚定→扩散→排除→降级）
   ▼
store（SQLite: documents/links/touch） · sync（对账 diff） · watch（自动监听） · web（REST+前端）
```

依赖极简：标准库 + `gopkg.in/yaml.v3` + `modernc.org/sqlite`（纯 Go 零 CGO），无网络出口。
维护者向架构文档在 `docs/architecture/`。

## 快速开始

```powershell
# 构建（Go 1.26+）
go build -o seren.exe ./cmd/seren

# 漫游
.\seren.exe roam <vault> "寻找"                  # Obsidian 库
.\seren.exe roam "D:\...\OrcaNote.db" "历史"       # 虎鲸库（.db 自动识别）
.\seren.exe roam <vault> --random --seed 42         # 🎲 随机漫步（--seed 可复现）

# Web UI（自动监听默认开；Obsidian 加 --vault-name、虎鲸自动 orca-note:// 跳转）
.\seren.exe serve <vault> --port 8080

# 对账刷新（增删改后同步，输出 增/删/改 明细）
.\seren.exe refresh <vault> --store <file.sqlite>

# MCP（AI 通道，只读四件套；给 dsh/agent 配 stdio MCP 指向此命令）
.\seren.exe mcp <vault> --db <file.sqlite>
```

## 文档

| 文档 | 说明 |
|---|---|
| [`docs/architecture/`](docs/architecture/) | 架构文档（维护者向）：总览 / 数据模型 / 适配器 / 引擎 / 同步 / Web / 维护指南 / MCP 研究 |
| [`docs/api-contract.md`](docs/api-contract.md) | API 契约：7 端点 + 鉴权（插件仓库与引擎的唯一共享物，改 API 必同步） |
| [`docs/roadmap.md`](docs/roadmap.md) | 路线图：M0 安全前置 + MCP / M1 核心完善 / M2 插件薄壳 |
| [`docs/frontend-issues.md`](docs/frontend-issues.md) | 前端问题记录与交接（前端专项 session 先读这里） |
| [`docs/design.md`](docs/design.md) | 设计过程记录（评审决策 + spike 实测） |

## 特别鸣谢

- **[dsh-mneme](https://github.com/modusensus/dsh-mneme)** —— 激活引擎哲学源头：结构 × 激活、激活扩散、白盒原则。同一套引擎换个载体、换个人当消费者，就是本项目的起点。
- **[恐龙工具箱](https://github.com/hqweay/orca-hqweay-go)（虎鲸笔记插件）** —— SRS 复习漫游与随机漫步交互的灵感来源。

## License

MIT License —— see [LICENSE](LICENSE).
