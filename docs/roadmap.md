# Serendipity Engine · 总路线图（Master Roadmap）

> 建立：2026-08-23（升级为**项目总路线图**——单一权威，把引擎 / 前端 / 插件安排进同一张表）。
> 原则（用户拍板 2026-08-23，2026-08-24 修订）：**优先让引擎能用、好用（作者自用闭环），再做插件让它在笔记软件里可用**。推广/营销不在项目开发考虑内——本仓库不承载任何推广内容。
> 分工：本文件管**阶段 / 依赖 / 状态**；细分计划在 [`backend-backlog.md`](backend-backlog.md)（后端）与 [`frontend.md`](frontend.md)（前端 + UI 规范）；战略定位 [`positioning.md`](positioning.md)；设计 [`design.md`](design.md)；契约 [`api-contract.md`](api-contract.md)。

---

## 顶层目标

> 定位（positioning 一句话）：把用户笔记库变成 agent 的记忆——本引擎是记忆的「激活层」。
> **落地目标**：先让作者本人把「阅读 + 漫游」用起来（引擎 + Web UI 顺手）；再做插件让它对别人也可用、不硬核。

## 当前状态快照（2026-08-24）

- 引擎 **v0.1.10**；**M0 已落地**（serve 安全前置 + API 契约 + MCP v3）；**36 单测绿**；Obsidian + 虎鲸真实库验证通过（双数据源、对账刷新、自动监听）。
- **现状判断**：目前几乎是「只有后端的引擎」——Web UI 只是「漫游工具」，缺让非技术用户可用的插件/入口，对外偏硬核。
- 未完成：引擎核心缺口（similar / node / export / touch / 社区发现 / LLM Wiki 画像…）、Web UI 完善、插件薄壳。
- **2026-08-24 新增方向**（来自 agent 记忆库研究，见 [`history/agent-memory-research.md`](history/agent-memory-research.md)）：**诊断层**（Leiden 社区发现 → 知识缺口诊断）、**LLM Wiki adapter 画像**；两个 backlog 方向获外部验证（Store 增量写 ← Graphiti、touch 边权演化 ← A-MEM）。

## 里程碑总览

| 里程碑 | 内容 | 阶段 | 状态 |
|---|---|---|---|
| **M0** | 插件前置（serve 安全）+ API 契约 + MCP v3 | — | ✅ 已落地 |
| **M1** | 引擎核心 + Web UI 完善（作者自用） | 阶段 1 | 进行 |
| **M2** | 插件薄壳（对外可用） | 阶段 2 | 待做 |

---

## 阶段计划（实施顺序）

### 阶段 1 · 引擎核心 + Web UI 完善（优先，作者自用）

> 目标：本体自己用得顺。做完后作者开箱即地把「阅读 + 漫游」跑起来。

| # | 任务 | 落点 | 说明 |
|---|---|---|---|
| 1 | **similar 结构相似**（Jaccard 孪生） | [backend-backlog](backend-backlog.md) §3.1 | 最高价值；解锁 `graph.similar` + Web 相似面板；白盒替代语义缺口 |
| 2 | **graph.node**（MCP + 前端节点详情，一次双端） | [backend-backlog](backend-backlog.md) §六 · [frontend](frontend.md) #3 | MCP 最缺的「确认这是什么」 |
| 3 | **export 漫游导出** | [backend-backlog](backend-backlog.md) §3.2 · [frontend](frontend.md) | 漫游发现能沉淀进笔记 |
| 4 | **touch 统计 API**（只读） | [backend-backlog](backend-backlog.md) §3.3 | 反馈闭环只读第一步 |
| 5 | 性能：PPR 提前收敛 / TextSearch 小写缓存 / Store 增量写 | [backend-backlog](backend-backlog.md) §二 | 数万节点规模（等信号） |
| 6 | 风险修复：renames 中间环 + WAL autocheckpoint | [backend-backlog](backend-backlog.md) §四 | 防无限增长 |
| 7 | **Web UI 完善**：hero 改静态（P0.5）、节点详情、相似/统计/导出面板、易用性（P1）、侧滑抽屉（§九） | [frontend](frontend.md) §三/§九 | 从「漫游工具」→「阅读 + 漫游工具」 |
| 8 | 前端测试：**JSON 契约测试**（优先）+ Playwright 前端自动化（**可选/按需**，用户后装环境） | [frontend](frontend.md) 附录 | 质量门槛 |
| 9 | **LLM Wiki adapter 画像**（`llm-wiki` + `ExcludedFiles`） | [backend-backlog](backend-backlog.md) §3.5 | 真实性门槛下唯一可接受的 LLM 数据源兼容，低成本 |
| 10 | **社区发现（Leiden）→ 诊断层**（知识缺口诊断） | [backend-backlog](backend-backlog.md) §3.4 | 「算法等场景」：诊断层排期时落地，选型已定 |

### 阶段 2 · 插件薄壳（M2；对外可用）

> 目标：让产品对非技术用户可用——当前后端引擎对外偏硬核，**插件化是「拿出来给别人」的关键**。

| # | 任务 | 落点 | 说明 |
|---|---|---|---|
| 1 | 前端 P0（紧凑嵌入 + postMessage 桥 + 节点详情） | [frontend](frontend.md) §二 | 插件化前置 |
| 2 | 插件薄壳 repos（`serendipity-obsidian` / `serendipity-orca`，两个独立仓库，零构建时依赖） | [history/plugin-evaluation](history/plugin-evaluation.md) | 对外可用 |
| 3 | 插件市场发布 | [history/plugin-evaluation](history/plugin-evaluation.md) §六 | Obsidian 社区目录 / 虎鲸 zip |

---

## 依赖链

```
引擎核心 + Web UI → 作者自用闭环（阶段 1）
前端 P0（紧凑嵌入 + postMessage）→ 插件薄壳 → 插件市场（阶段 2，对外可用）
graph.node → 前端节点详情 + MCP「确认这是什么」（阶段 1）
Leiden 社区发现 → 知识缺口诊断 API（阶段 1，诊断层排期时）
```

---

## 明确不做（仓库级）

- TS / WASM / 移动端；GraphRAG / LLM 建图；embedding 内置 / 在线 API；图数据库；SaaS / 远程 / 云。
- **LLM 生成记忆库 adapter**（OpenViking / Graphiti / A-MEM / Mem0 建图）与其结构筛选/重排器——真实性门槛，见 [`positioning.md`](positioning.md) §六。
- **推广/营销**（渠道、文案、节奏）——不在项目开发考虑内，本仓库不承载。

---

## 版本记录

| 日期 | 变更 |
|---|---|
| 2026-08-23 | 建立；M0（安全前置 + 契约 + MCP 提前）、M1（核心完善）、M2（插件薄壳）。 |
| 2026-08-23 | 升级为总路线图（阶段 + 依赖链 + 状态快照）。 |
| 2026-08-23 | 按用户拍板**重排优先级**：优先完善项目自用（阶段 1）→ 插件薄壳（阶段 2，M2）。 |
| 2026-08-24 | **移除发布/推广阶段**（推广不在项目开发考虑内）；并入 agent 记忆库研究结论（诊断层 / LLM Wiki 画像 / 外部验证）。 |
