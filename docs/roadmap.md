# Serendipity Engine · 总路线图（Master Roadmap）

> 建立：2026-08-23（升级为**项目总路线图**——单一权威，把引擎 / 前端 / 插件安排进同一张表）。
> 原则（用户拍板）：**优先让引擎能用、好用（作者自用闭环），再做插件让它在笔记软件里可用**。推广/营销不在项目开发考虑内。
> 分工：本文件管**阶段 / 依赖 / 状态**；细分计划在 [`backend-backlog.md`](backend-backlog.md)（后端）· [`frontend.md`](frontend.md)（前端）· [`plugin-dev-plan.md`](plugin-dev-plan.md)（插件 M2）；引擎与 AI 的协作边界（引擎零 AI、经只读 MCP 暴露）见 [`design.md`](design.md) §7.6 · [`api-contract.md`](api-contract.md)；战略定位 [`positioning.md`](positioning.md)。

---

## 顶层目标

> 定位：把用户笔记库变成 agent 的记忆——引擎是记忆的「激活层」。
> 落地：先让作者本人把「阅读 + 漫游」用起来（引擎 + Web UI 顺手）；再做插件让它对别人也可用、不硬核。

## 当前状态快照（2026-08-27）

- **引擎 v0.2.0**；M0/M1 全部落地（74 单测绿）；Obsidian + 虎鲸真实库验证通过。
- **M2 引擎侧收工**：前端 P0 ✅ → Obsidian 插件 `serendipity-obsidian` v0.1.0 可用；MCP 升级（§3.8 语义可发现性 Layer A + Layer B + §3.9 Streamable HTTP/mcp-go + 九件套含 `seren.state`）✅ 落地。
- **M3 排期**：Wails 桌面壳 + bbolt 有趣能力 + canvas 检索（均候选，等 M2 完）。
- 虎鲸插件已停止开发（架构限制），内核直读虎鲸库能力保留。

## 里程碑总览

| 里程碑 | 内容 | 状态 |
|---|---|---|
| **M0** | 插件前置（serve 安全）+ API 契约 + MCP | ✅ 已落地 |
| **M1** | 引擎核心 + Web UI 完善（作者自用） | ✅ 能力面完成 |
| **M2** | 插件薄壳（对外可用）+ MCP 升级 | 前端 P0 ✅；Obsidian 插件 v0.1.0 ✅；MCP 升级 ✅（引擎侧收工） |
| **M3** | 远期增强（Wails 壳 / bbolt 有趣 / canvas） | ⏸ 远期候选 |

---

## 阶段计划（实施顺序）

### 阶段 1 · 引擎核心 + Web UI（✅ 已全部落地，v0.1.11–v0.1.15）

similar（Adamic-Adar）· graph.node · export 导出 · touch 统计 · CLI 三件套 · 刷新一致性 + 体验 · LLM Wiki 画像 · Leiden 诊断层 · 前端 P0（embed + postMessage + i18n）· bbolt 存储层 #16 · 潜在关联候选 #15 · touch 行为信号子系统 · 无库启动 + 配库。详见 [`backend-backlog.md`](backend-backlog.md) §六 已落地速查。

### 阶段 2 · 插件薄壳 + MCP 升级（M2）

| # | 任务 | 落点 | 状态 |
|---|---|---|---|
| 1 | **MCP 语义可发现性**（Layer A 描述富化 + readOnlyHint + required；Layer B `seren_orientation` prompt + skill） | [backend-backlog](backend-backlog.md) §3.8 | ✅ Layer A + Layer B 落地 |
| 2 | **MCP 传输升级：Streamable HTTP + mcp-go**（serve 加 `/mcp` 端点，Web+REST+MCP 三合一；stdio 迁移上 SDK、删手写 JSON-RPC；+`seren.state`） | [backend-backlog](backend-backlog.md) §3.9 | ✅ 已落地 |
| 3 | 前端 P0（紧凑嵌入 + postMessage 桥 + i18n 双语） | [frontend](frontend.md) | ✅ v0.1.12 |
| 4 | 插件薄壳 `serendipity-obsidian`（生命周期四态机；managed 模式） | [plugin-dev-plan](plugin-dev-plan.md) §五/§六 | ✅ v0.1.0（依赖引擎 ≥ v0.1.14） |
| 5 | 核心引擎多平台分发（goreleaser + Actions 四平台 asset） | [plugin-dev-plan](plugin-dev-plan.md) §四 | win/mac/linux 全出包 |
| 6 | 引擎 × AI 协作边界（引擎经只读 MCP 暴露；插件薄壳不内置 AI） | [plugin-dev-plan](plugin-dev-plan.md) §九 · [design](design.md) §7.6 | 已定稿 |
| 7 | 插件市场发布（Obsidian 社区目录） | [plugin-dev-plan](plugin-dev-plan.md) §四/§七 | 待插件收尾 |
| 8 | touch 行为信号子系统（独立 store + digest + 备份 + 被动只读） | [backend-backlog](backend-backlog.md) §3.7 | ✅ v0.1.14 |

### 阶段 3 · 远期增强（M3，候选非承诺）

| # | 任务 | 落点 |
|---|---|---|
| 1 | **Wails 桌面壳** `serendipity-desktop`（选库 + spawn serve + WebView2 嵌 UI + MCP/serve 启停） | [backend-backlog](backend-backlog.md) §五.2 |
| 2 | **bbolt 有趣能力**：图谱时间旅行 / 探索日志·偶遇时刻 / 跨库元索引 / What-if 实验图 | [backend-backlog](backend-backlog.md) §二.2 |
| 3 | **Obsidian `.canvas` 白板检索**（解析 `.canvas` JSON 补为 Refs） | 用户本人少用，低优先 |

---

## 依赖链

```
引擎核心 + Web UI → 作者自用闭环（阶段 1）
前端 P0（embed + postMessage）→ 插件薄壳 → 插件市场（阶段 2）
graph.node → 前端节点详情 + MCP「确认这是什么」（阶段 1）
Leiden 社区发现 → 知识缺口诊断（阶段 1）
潜在关联 #15 → 外部 AI 经 MCP 消费 suggest-links 候选（阶段 2 #6）
bbolt #16 → M3（bbolt 有趣能力 + Wails 壳）
```

## 明确不做（仓库级）

- TS / WASM / 移动端；GraphRAG / LLM 建图；**embedding / 语义层**（纯本地算法）；图数据库；SaaS / 远程 / 云。
- **LLM 生成记忆库 adapter**（OpenViking / Graphiti / A-MEM / Mem0）与其结构筛选/重排器——真实性门槛，见 [`positioning.md`](positioning.md) §六。
- **引擎内置 AI**：引擎零 AI 依赖，只暴露 REST/MCP 接口与确定性算法，绝不调用任何模型；语义研判全在引擎之外（外部 AI/agent 经只读 MCP 消费），插件不内置 AI。
- **推广/营销**——不在项目开发考虑内，本仓库不承载。

---

## 版本记录

| 日期 | 变更 |
|---|---|
| 2026-08-23 | 建立总路线图（M0/M1/M2）；随后 v0.1.11–v0.1.15 连续落地阶段 1 全部能力（详见 backend-backlog §六）与 M2 前端 P0、touch 子系统、无库配库。 |
| 2026-08-25~26 | M2 设计文档定稿（plugin-dev-plan / §3.7–§3.9）；Obsidian 插件 v0.1.0 可用；**虎鲸插件停止开发**（架构限制，内核直读能力保留）；Wails 桌面壳立项（M3）。 |
| 2026-08-26~27 | 文档清理：删语义层设计（纯本地算法）、删 history/ 与 plugin-ai-cooperation、skill 扁平化为 SKILL.md；已落地/已放弃决策不保留叙事。 |
| 2026-08-27 | MCP 升级落地（**v0.2.0**）：mcp-go + Streamable HTTP（serve `/mcp` + `/api/mcp/*`）+ 九件套（+seren.state）+ `seren_orientation` prompt + 前端 MCP 状态面板/一键配置；suggest-links 聚合改原始分（修 Borda 非确定）；CLI/MCP 用户可见输出英文化；skill 资产 SKILL.md。**M2 引擎侧收工**。 |
