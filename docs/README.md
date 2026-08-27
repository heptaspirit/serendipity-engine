# 文档导航（docs/）

> 本目录按**主题分层**组织 Serendipity Engine 的文档。除组件架构（`architecture/`）外，均为当前有效的设计文档。改算法前请先读对应分层文档。
>
> **AI agent 入口**：根目录 [`AGENTS.md`](../AGENTS.md)（30 秒定位 / 命令 / 仓库地图 / 红线）——本文档是完整的分层导航。

## 分层速览

| 层 | 文档 | 回答的问题 |
|---|---|---|
| **设计 · 战略** | [`design.md`](design.md) · [`positioning.md`](positioning.md) | 机制怎么走？为什么做、做哪一层？ |
| **计划** | [`roadmap.md`](roadmap.md) · [`frontend.md`](frontend.md) · [`backend-backlog.md`](backend-backlog.md) | 接下来做什么？ |
| **契约** | [`api-contract.md`](api-contract.md) | 引擎与外部（插件/MCP）怎么对接？ |
| **架构** | [`architecture/`](architecture/) | 组件怎么组织、如何维护？ |

## 设计 · 战略

| 文档 | 说明 |
|---|---|
| [`design.md`](design.md) | **核心设计**（修订版 v3）：图谱漫游机制、四维打分（PPR + 激活 + 跳数配额）、边语义、技术栈与产品形态。历史决策与 spike 实测均已在此吸收。 |
| [`positioning.md`](positioning.md) | **战略定位**：笔记库 = agent 记忆的「激活层」、生态差异化、LLM Wiki 互补、纯本地算法定位/数据源边界、明确不做、护城河。 |

## 计划

| 文档 | 说明 |
|---|---|
| [`roadmap.md`](roadmap.md) | **总路线图（唯一权威）**：阶段 1 引擎核心+Web UI 完善(作者自用) / 2 插件薄壳(M2)，含依赖链与状态快照。 |
| [`plugin-dev-plan.md`](plugin-dev-plan.md) | **插件开发计划（M2）**：生命周期四态机 / 多平台分发 / 引擎×AI 协作边界（插件薄壳不内置 AI）。⚠️ 具体插件代码在独立仓库开发（不在本仓库），本仓库仅放引擎内核（与插件唯一的共享物是 `api-contract.md`） |
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

## 目录约定

- `architecture/` 之外为**当前有效**文档，改动需同步本导航。
- 属于联调记录等本地敏感内容，放 `docs-local/`（已在 `.gitignore`，不入库）。
