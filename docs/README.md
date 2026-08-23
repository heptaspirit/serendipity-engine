# 文档导航（docs/）

> 本目录按**主题分层**组织 Serendipity Engine 的文档。除组件架构（`architecture/`）与历史归档（`history/`）外，均为当前有效的设计文档。改算法前请先读对应分层文档。

## 分层速览

| 层 | 文档 | 回答的问题 |
|---|---|---|
| **设计 · 战略** | [`design.md`](design.md) · [`positioning.md`](positioning.md) | 机制怎么走？为什么做、做哪一层？ |
| **计划** | [`roadmap.md`](roadmap.md) · [`frontend.md`](frontend.md) · [`backend-backlog.md`](backend-backlog.md) | 接下来做什么？ |
| **契约** | [`api-contract.md`](api-contract.md) | 引擎与外部（插件/MCP）怎么对接？ |
| **架构** | [`architecture/`](architecture/) | 组件怎么组织、如何维护？ |
| **历史** | [`history/`](history/) | 哪些决策/验证已被吸收进 design/roadmap？ |

## 设计 · 战略

| 文档 | 说明 |
|---|---|
| [`design.md`](design.md) | **核心设计**（修订版 v2）：图谱漫游机制、四维打分（PPR + 激活 + 跳数配额）、边语义、技术栈与产品形态。历史决策与 spike 实测均已在此吸收。 |
| [`positioning.md`](positioning.md) | **战略定位**：笔记库 = agent 记忆的「激活层」、生态差异化、LLM Wiki 互补、embedding/数据源边界、明确不做、护城河。 |

## 计划

| 文档 | 说明 |
|---|---|
| [`roadmap.md`](roadmap.md) | **总路线图（唯一权威）**：阶段 1 引擎核心+Web UI 完善(作者自用) / 2 插件薄壳(M2) / 3 发布+推广(M2 后)，含依赖链与状态快照。 |
| [`frontend.md`](frontend.md) | **前端计划（Web UI）**：插件化前置、易用性、UI/UX 打磨规范 + 测试速查与交接。 |
| [`backend-backlog.md`](backend-backlog.md) | **后端积压清单**：性能优化、功能缺口（similar/export/touch 统计）、CLI 三件套、MCP 工具扩展、风险红线与优先级。 |

## 契约

| 文档 | 说明 |
|---|---|
| [`api-contract.md`](api-contract.md) | **API 契约**：REST `/api/*` 端点 + 鉴权。插件仓库与引擎的**唯一共享物**，改 API 必须同步。 |

## 架构（维护者向）

| 文档 | 说明 |
|---|---|
| [`architecture/00-overview.md`](architecture/00-overview.md) | 总览与维护指南入口 |
| [`architecture/`](architecture/) | 数据模型 / 适配器 / 引擎 / 同步 / Web / 维护指南 / MCP 研究 |

## 历史（已归档）

> 以下文档的内容**已被 `design.md` / `roadmap.md` 吸收**，仅保留完整的历史叙事与原始依据。主文档区不再重复。

| 文档 | 被谁吸收 |
|---|---|
| [`history/DESIGN_REVIEW.md`](history/DESIGN_REVIEW.md) | 设计评审 13 条决策 → `design.md` 修订版 v2 |
| [`history/spike-report.md`](history/spike-report.md) | spike 实测（F1–F7 + λ/θ/hops）→ `design.md` §3.4 / §6.1 + `architecture/03-engine.md` |
| [`history/product-form.md`](history/product-form.md) | 三层产品形态决策 → `design.md` §6.8 + `history/plugin-evaluation.md` + `roadmap.md` |
| [`history/plugin-evaluation.md`](history/plugin-evaluation.md) | 插件薄壳决策（D1–D8 + 工作清单）→ `roadmap.md` M2 |
| [`history/frontend-issues.md`](history/frontend-issues.md) | 前端问题/测试移交 → `frontend.md`（测试速查 + 防回归清单折叠保留） |

## 目录约定

- `architecture/` 与 `history/` 之外为**当前有效**文档，改动需同步本导航。
- 属于具体推广操作、联调记录等本地敏感内容，放 `docs-local/`（已在 `.gitignore`，不入库）。
