---
title: "AGENTS.md — AI 开发引导"
summary: "给 AI agent 的快速上手：30 秒定位、常用命令、仓库地图、文档决策树、开发约定与红线。本文件是「索引 + 指令」，细节一律去引用的文档读"
owner: heptaspirit
status: active
date: 2026-08-24
---

# AGENTS.md — AI 开发引导

> 给 AI agent 的快速上手（Codex / DeepSeek Harness（dsh）/ Claude Code / Cursor 等会自动读取本文件）。
> 人类维护者请读 [`docs/architecture/00-overview.md`](docs/architecture/00-overview.md)；完整文档导航见 [`docs/README.md`](docs/README.md)。
> **本文件是索引 + 指令，不复制细节**——具体内容一律去引用的文档里读，避免双份维护漂移。

## 这个项目是什么（30 秒）

**Serendipity Engine（seren）**：给个人笔记的双链网络装「激活引擎」——查询锚定 PPR + 激活扩散，从笔记库里取回一批**可解释**的相关节点簇。一句话：**你问一个点，它给你一片。**

- 图 = 用户手写的**真实双链**（白盒、可解释），不是 LLM 编的、不是 embedding 相似度
- 本地优先、纯 Go 零 CGO、编译产物单二进制、数据不出本机
- 两个消费者：**人**在笔记库里漫游寻灵感，**agent** 直接消费相关簇 / 证据链 / 权重分布（MCP）

**它不是**：RAG 检索器 / 图数据库 / GraphRAG / LLM 建图 / 语义 embedding 引擎（见「明确不做」）。

## 常用命令

```bash
go build ./cmd/seren          # 构建
go test ./...                  # 测试（改代码后必须跑，全绿为准）
go vet ./...                   # 静态检查
go run ./cmd/seren roam <vault> "关键词"   # 漫游试跑（vault = Obsidian 目录或虎鲸 .db 路径）
go run ./cmd/seren serve [<vault>] --port 8080  # 起 Web UI（不带 vault = 无库启动，POST /api/vault 配库）
go run ./cmd/seren mcp <vault>              # MCP（stdio JSON-RPC，AI 通道）
```

## 仓库地图

| 路径 | 职责 |
|---|---|
| `cmd/seren` | CLI 入口：index / roam / serve / refresh / profile-detect / mcp / version / help |
| `internal/adapter` | 格式翻译：`Document` 抽象（内核唯一认识的格式）+ Obsidian / 虎鲸 / VaultProfile 画像 |
| `internal/graph` | 内存图：Build / Resolve / PPR / 激活扩散 / TextSearch / Similar / 社区发现 |
| `internal/score` | 归一化融合打分（PPR × 激活 × 跳数） |
| `internal/roam` | 漫游管线：锚定 → 扩散 → 排除 → 降级 |
| `internal/store` | 持久化（bbolt；图库 `db-<hash>.bbolt` 三 bucket + touch 独立 `touch-<hash>.bbolt`，见 backend-backlog §3.7） |
| `internal/sync` | 对账 diff（增 / 删 / 改 / 改名） |
| `internal/watch` | 自动监听（轮询 + 节流合并） |
| `internal/web` | REST `/api/*`（15 端点，见 api-contract.md）+ Web UI（static/index.html）+ 无库启动配库（/api/vault） |
| `internal/mcp` | MCP 只读八工具：graph.stats / roam / random / relation / node / similar / community / seren.touch_digest |

## 文档地图（改什么，先读什么）

| 任务 | 先读 |
|---|---|
| 了解整体机制 / 四维打分 | [`docs/design.md`](docs/design.md) |
| 了解战略定位 / 边界 / 明确不做 | [`docs/positioning.md`](docs/positioning.md) |
| 看接下来做什么 / 排期 | [`docs/roadmap.md`](docs/roadmap.md)（唯一权威总表） |
| **改 API / MCP / 前端交互** | [`docs/api-contract.md`](docs/api-contract.md)（**改 API 必同步**） |
| 改后端（性能 / 功能 / CLI / MCP / 存储） | [`docs/backend-backlog.md`](docs/backend-backlog.md) |
| 改 Web UI | [`docs/frontend.md`](docs/frontend.md) |
| 组件级实现细节 | [`docs/architecture/`](docs/architecture/)（从 00-overview.md 入口） |
| 查历史决策 / 验证记录 | [`docs/history/`](docs/history/)（内容已吸收进主文档，作溯源用） |

## 开发约定（红线，违反需谨慎）

1. **设计哲学**（architecture/00-overview §2）：结构×激活 / 白盒可解释 / 解析抽离（画像 YAML，换库不改代码）/ 克制设计 / 安全红线 / 工程纪律。
2. **工程纪律**（backend-backlog §一.5）：单文件 500 行左右、不超千行；按领域拆文件不碎片化；算法 = 包级可复用函数（为 MCP 暴露准备）；第三方算法库可引入（MIT + vendor 锁版本 + attribution，first case = leiden-go）。
3. **克制哲学**：埋点只记录不演化（touch 绝不反馈排序/hot）；监听节流合并；**不加中间态恢复逻辑**（1 分钟快照对账已免疫中间态）。
4. **安全红线**：凭据类数据一律不读；活库先一致性快照再读；个人数据不进 git；MCP 只读（不写 touch、不触发 refresh）。
5. **真实性门槛**：无事实锚点、纯 LLM 生成且无人验证的数据源默认拒绝接入（见 positioning / history/agent-memory-research）。

## 明确不做（防跑偏）

- **embedding / 本地语义模型 / 在线语义 API**——结构替代语义（similar/mentions）；语义注入口只留在 Web 层供插件调用，引擎核心不碰
- **GraphRAG / LLM 建图**——图必须是真实链接
- **graph 数据库**（Neo4j / Kuzu / FalkorDB）——数千~2 万节点规模是负资产，内存图 + 自研算法足够
- **LLM 生成记忆库的 adapter**（OpenViking / Mem0 / server-memory 等）——真实性门槛；架构预留但官方不做
- **TS / WASM / 移动端移植、SaaS / 云模式**——个人数据不出本机

## 完成任务前检查

- [ ] `go build ./...` 通过
- [ ] `go test ./...` 全绿（改动涉及的部分）
- [ ] 改了 API / MCP → `docs/api-contract.md` 已同步
- [ ] 改了文档 → `docs/README.md` 导航仍准确
- [ ] 新增功能未触碰上述红线
