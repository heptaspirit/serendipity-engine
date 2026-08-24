# Serendipity Engine · 总路线图（Master Roadmap）

> 建立：2026-08-23（升级为**项目总路线图**——单一权威，把引擎 / 前端 / 插件安排进同一张表）。
> 原则（用户拍板 2026-08-23，2026-08-24 修订）：**优先让引擎能用、好用（作者自用闭环），再做插件让它在笔记软件里可用**。推广/营销不在项目开发考虑内——本仓库不承载任何推广内容。
> 分工：本文件管**阶段 / 依赖 / 状态**；细分计划在 [`backend-backlog.md`](backend-backlog.md)（后端）与 [`frontend.md`](frontend.md)（前端 + UI 规范）；战略定位 [`positioning.md`](positioning.md)；设计 [`design.md`](design.md)；契约 [`api-contract.md`](api-contract.md)。

---

## 顶层目标

> 定位（positioning 一句话）：把用户笔记库变成 agent 的记忆——本引擎是记忆的「激活层」。
> **落地目标**：先让作者本人把「阅读 + 漫游」用起来（引擎 + Web UI 顺手）；再做插件让它对别人也可用、不硬核。

## 当前状态快照（2026-08-25）

- 引擎 **v0.1.12**；**M0 已落地**（serve 安全前置 + API 契约 + MCP v3）；**74 单测绿**；Obsidian + 虎鲸真实库验证通过（双数据源、对账刷新、自动监听）。
- **M1 阶段 1 基本收尾**：similar 结构相似（Adamic-Adar）、graph.node 节点详情、export 漫游导出、touch 统计 API、Stats 缓存、renames 中间环清理、WAL autocheckpoint、CLI 三件套、**刷新一致性（悬挂链接明细 + 幽灵 touch 过滤）**、**刷新体验（is_pending 事前提示 + 手动即时刷新）**、**LLM Wiki adapter 画像（llm-wiki + ExcludedFiles + watch 排除同源）**、**Leiden 社区发现诊断层（/api/communities + MCP graph.community）**、**前端 JSON 契约测试**全部落地。
- **前端 P0（阶段 2 #1，插件化前置）本轮同步完成**：紧凑嵌入 `?embed=1` + postMessage 桥（`{type:'open',id}` / 宿主注入 theme/locale）+ i18n 中英双语全部文案；Web UI 完善（hero 改静态热门列表 / 侧滑抽屉统一面板 / 卡片 ID 收敛 + scores 折叠 + 🎲 升级主按钮）。
- **similar 实测反馈（2026-08-24）→ 已修复**：v0.1.11 的 Jaccard 被用户实测"挂出来的节点不太对"；v0.1.12 升级 **Adamic-Adar**（共同邻居度加权 `Σ 1/log(deg)`），度偏置与共享邻居不加权问题一并解决。
- **现状判断**：引擎侧（阶段 1）的能力面已覆盖作者自用闭环；前端 P0 就绪 → 下一步进 **M2 插件薄壳**（Obsidian / 虎鲸）。
- 未完成（阶段 1 剩余，均"等场景/可选"）：性能优化（PPR 提前收敛 / TextSearch 小写缓存 / Store 增量写，等数万节点规模信号）、mentions API（可选低优先，诊断层引用索引一起做）。
- **2026-08-24 新增方向**（诊断层 / LLM Wiki 画像）本轮已落地（Leiden `graph.community` + `llm-wiki` 画像）。

## 里程碑总览

| 里程碑 | 内容 | 阶段 | 状态 |
|---|---|---|---|
| **M0** | 插件前置（serve 安全）+ API 契约 + MCP v3 | — | ✅ 已落地 |
| **M1** | 引擎核心 + Web UI 完善（作者自用） | 阶段 1 | ✅ 能力面完成（剩余项均"等场景/可选"） |
| **M2** | 插件薄壳（对外可用） | 阶段 2 | 前端 P0 已就绪，可开工 |

---

## 阶段计划（实施顺序）

### 阶段 1 · 引擎核心 + Web UI 完善（优先，作者自用）

> 目标：本体自己用得顺。做完后作者开箱即地把「阅读 + 漫游」跑起来。

| # | 任务 | 落点 | 说明 |
|---|---|---|---|
| 1 | **similar 结构相似**（Jaccard 孪生） | [backend-backlog](backend-backlog.md) §3.1 | ✅ v0.1.11；v0.1.12 升级 Adamic-Adar（见 #12） |
| 2 | **graph.node**（MCP + 前端节点详情，一次双端） | [backend-backlog](backend-backlog.md) §六 · [frontend](frontend.md) #3 | ✅ v0.1.11；MCP graph.node + /api/node + 卡片「预览」 |
| 3 | **export 漫游导出** | [backend-backlog](backend-backlog.md) §3.2 · [frontend](frontend.md) | ✅ v0.1.11；/api/roam?export=1 → Markdown 卡片清单 |
| 4 | **touch 统计 API**（只读） | [backend-backlog](backend-backlog.md) §3.3 | ✅ v0.1.11；/api/touch/stats 只读聚合，绝不反馈排序；v0.1.12 加幽灵 touch 过滤（见 #13） |
| 5 | 性能：PPR 提前收敛 / TextSearch 小写缓存 / Store 增量写 | [backend-backlog](backend-backlog.md) §二 | ⏸ 等数万节点规模信号；Stats 缓存 ✅（v0.1.11 顺手做掉） |
| 6 | 风险修复：renames 中间环 + WAL autocheckpoint | [backend-backlog](backend-backlog.md) §四 | ✅ v0.1.11（collapseChains + wal_autocheckpoint） |
| 7 | **Web UI 完善**：hero 静态（P0.5）、节点详情、相似/统计/导出面板、侧滑抽屉（§九） | [frontend](frontend.md) §三/§九 | ✅ v0.1.11 面板 + v0.1.12 hero 改静态 + 侧滑抽屉统一 |
| 8 | 前端测试：**JSON 契约测试**（优先）+ Playwright 可选 | [frontend](frontend.md) 附录 | ✅ v0.1.12 JSON 契约测试（10 端点循环校验）；Playwright 留作可选防回归 |
| 9 | **LLM Wiki adapter 画像**（`llm-wiki` + `ExcludedFiles` + **watch 排除同源**） | [backend-backlog](backend-backlog.md) §3.5 | ✅ v0.1.12；llm-wiki 画像 + 文件名级排除 + watch 同源 + 结构探测提示 |
| 10 | **诊断层（Leiden 社区发现 → 知识缺口诊断）** | [backend-backlog](backend-backlog.md) §3.4 · [history/agent-memory-research](history/agent-memory-research.md) 附录 E | ✅ v0.1.12（leiden-go vendor + /api/communities + MCP graph.community）；聚类系数/K-Core/Betweenness 作内部信号按需取用 |
| 11 | **CLI 打磨三件套** | [backend-backlog](backend-backlog.md) §五 | ✅ v0.1.11（seren help <cmd> / --json / 退出码 0-2-1） |
| 12 | **similar 评分升级：Jaccard → Adamic-Adar** | [backend-backlog](backend-backlog.md) §3.1 · [history/agent-memory-research](history/agent-memory-research.md) 附录 E | ✅ v0.1.12；用户实测驱动（2026-08-24），~20 行，证据/排除/排序全复用 |
| 13 | **刷新一致性补全**（悬挂链接明细 + 幽灵 touch 过滤） | [backend-backlog](backend-backlog.md) §四 | ✅ v0.1.12；`DanglingRefs()` 明细（stats.dangling_refs）+ TouchStats targets 关联 documents 过滤已删节点 |
| 14 | **刷新体验增强**（事前提示 + 手动即时刷新） | [backend-backlog](backend-backlog.md) §四 | ✅ v0.1.12；stats.is_pending + 前端"库有变化，将自动刷新 · 立即刷新"提示条 + 手动刷新清 pending |
| 15 | **mentions API（虚拟引用 / 未链接提及）** | [backend-backlog](backend-backlog.md) §3.6 | ⏸ 可选（低优先）：refresh 时建提及索引（AC 扫描）——登录到诊断层/引用索引排期一起做 |

### 阶段 2 · 插件薄壳（M2；对外可用）

> 目标：让产品对非技术用户可用——当前后端引擎对外偏硬核，**插件化是「拿出来给别人」的关键**。

| # | 任务 | 落点 | 说明 |
|---|---|---|---|
| 1 | 前端 P0（紧凑嵌入 + postMessage 桥 + 节点详情 + i18n 双语） | [frontend](frontend.md) §二 | ✅ v0.1.12；`?embed=1` + `{type:'open'}` 桥 + 中英双语全部文案——插件化前置已就绪 |
| 2 | 插件薄壳 repos（`serendipity-obsidian` / `serendipity-orca`，两个独立仓库，零构建时依赖） | [history/plugin-evaluation](history/plugin-evaluation.md) | 对外可用（下一个里程碑） |
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
| 2026-08-24 | **similar 评分升级排入阶段 1（#12）**：Jaccard → Adamic-Adar（用户实测"挂出来的节点不太对"驱动；附录 E 参考已备，~20 行低成本）。 |
| 2026-08-24 | **阶段 1 补三待办**：#13 刷新一致性补全（悬挂链接明细 + 幽灵 touch 过滤）、#14 刷新体验增强（is_pending 事前提示 + 手动刷新清 pending）；#9 补 watch 排除同源。 |
| 2026-08-25 | **v0.1.12 收官**：M1 阶段 1 能力面完成（#12 Adamic-Adar / #13 刷新一致性 / #14 刷新体验 / #9 LLM Wiki 画像 / #10 Leiden 诊断层 / #8 JSON 契约测试 / #7 Web UI 完善）；**前端 P0**（阶段 2 #1：紧凑嵌入 + postMessage 桥 + i18n 双语）同步完成。M1 → M2（插件薄壳）在即。 |
