# Serendipity Engine · 奇遇记引擎

<p align="center">
  <strong>🌐 语言 / Language：</strong>
  🇨🇳 <strong>简体中文</strong> ·
  <a href="README.en.md">🇺🇸 English</a>
</p>

<p align="center"><img src="docs/logo.png" alt="Serendipity Engine" width="160"></p>

> 图谱漫游：给个人笔记的双链装上激活引擎——**你问一个点，它给你一片。**
>
> 白盒、本地、纯 Go 零依赖。一份结构信号，两个消费者：**人**在笔记库里漫游寻灵感，**agent** 免于闷头遍历、直接消费相关簇 / 证据链 / 权重分布。

[![版本](https://img.shields.io/badge/%E7%89%88%E6%9C%AC-v0.1.14-7aa2f7)](https://github.com/heptaspirit/serendipity-engine/tags) [![License](https://img.shields.io/badge/License-MIT-9cf)](LICENSE) [![Go](https://img.shields.io/badge/Go-1.26%2B-00ADD8)](go.mod) [![纯 Go](https://img.shields.io/badge/%E7%BA%AF%20Go-%E9%9B%B6%20CGO-4c566a)](go.mod) [![Single Binary](https://img.shields.io/badge/Single%20Binary-%E5%8D%95%E4%BA%8C%E8%BF%9B%E5%88%B6-7aa2f7)](https://github.com/heptaspirit/serendipity-engine/releases) [![Local-first](https://img.shields.io/badge/Local--first-%E6%95%B0%E6%8D%AE%E4%B8%8D%E5%87%BA%E6%9C%AC%E6%9C%BA-7aa2f7)](https://github.com/heptaspirit/serendipity-engine) [![MCP Server](https://img.shields.io/badge/MCP%20Server-AI%20%E5%8F%AF%E6%8E%A5%E5%85%A5-7aa2f7)](https://github.com/heptaspirit/serendipity-engine) [![Top Language](https://img.shields.io/github/languages/top/heptaspirit/serendipity-engine)](https://github.com/heptaspirit/serendipity-engine) [![简体中文](https://img.shields.io/badge/%E7%AE%80%E4%BD%93%E4%B8%AD%E6%96%87-README-7aa2f7)](README.md) [![English](https://img.shields.io/badge/English-README.en-7aa2f7)](README.en.md)

## 特性

- **漫游**：查询驱动（锚点 → 可解释的相关节点簇）· 🎲 随机漫步（可复现种子）
- **白盒**：每条推荐带激活路径、可点击续漫游、可跳回原笔记软件
- **数据源可扩展**：adapter 接口支持任意双链类笔记软件——目前适配 Obsidian vault + 虎鲸快照直读（凭据类数据一律不读）
- **查询**：两节点证据链 · 结构相似节点（共享邻居证据）· 潜在关联待审清单（`/api/suggest-links`，供 AI 研判）· 漫游结果导出 Markdown
- **对账刷新**：手动 / 自动监听三路同步增删改 · 事前提示 + 即时刷新
- **AI 可接入**：MCP 只读工具 + CLI `--json` 结构化输出
- **可选兼容**：LLM Wiki 库画像（`--profile-name llm-wiki`）

## 设计哲学

1. **结构 × 激活**：图结构提供"可能相关"，激活机制提供"此刻相关"——只有结构没有激活的 wiki 是死的。
2. **白盒原则**：每条推荐可解释、可干预、可跳回原软件，不做黑盒。
3. **解析抽离**：通用语法固定，语义映射（title/类型规则）YAML 画像化，换库不改代码。
4. **克制设计**：监听节流合并、埋点只记录不演化——任何"点击→边权→结果"的正反馈循环在源头切断，本地工具优先稳定。
5. **安全红线**：凭据类数据一律不读取；活库先一致性快照再读；个人数据不进 git。
6. **明确不做（非目标）**：embedding / GraphRAG / 图数据库 / LLM 建图——图必须是用户手写的真实链接（详见 [`docs/positioning.md`](docs/positioning.md)）。

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
store（bbolt: docs/links/touch/renames 四 bucket） · sync（对账 diff） · watch（自动监听） · web（REST+前端）
```

依赖极简：标准库 + `gopkg.in/yaml.v3` + `go.etcd.io/bbolt`（MIT，原生 Go 零 CGO，存储层）+ `github.com/vsuryav/leiden-go`（MIT，社区发现），无网络出口。
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
.\seren.exe refresh <vault> --store <file.bbolt>

# MCP（AI 通道，只读七工具；给 dsh/agent 配 stdio MCP 指向此命令）
.\seren.exe mcp <vault> --db <file.bbolt>

# 子命令级帮助 + 结构化输出（CLI 三件套）
.\seren.exe help roam          # 某子命令专属帮助（或 .\seren.exe roam -h）
.\seren.exe roam <vault> "词" --json   # 结构化 JSON（数据可给 agent 直接消费）
```

LLM Wiki 库画像：`.\seren.exe roam <llm-wiki-vault> "词" --profile-name llm-wiki`。

## AI 接入（MCP）

任何 MCP 客户端（Codex / DeepSeek Harness（dsh）/ Claude Code / Cursor / 其他 agent）把下面配置加进 `mcpServers` 即可：

```json
{
  "mcpServers": {
    "seren": {
      "command": "seren",
      "args": ["mcp", "<vault>", "--db", "<file.bbolt>"]
    }
  }
}
```

七个只读工具：`graph.stats / roam / random / relation / node / similar / community`（不写 touch、不触发 refresh——AI 会话不能改动本地状态）。

## 开发

```bash
go build ./cmd/seren   # 构建
go test ./...          # 测试
go vet ./...           # 静态检查
```

AI agent 请先读 [`AGENTS.md`](AGENTS.md)（定位 / 仓库地图 / 开发红线）。

## 文档

| 文档 | 说明 |
|---|---|
| [`docs/README.md`](docs/README.md) | 文档导航（按主题分层索引） |
| [`docs/architecture/`](docs/architecture/) | 架构文档（维护者向）：总览 / 数据模型 / 适配器 / 引擎 / 同步 / Web / 维护指南 / MCP 研究 |
| [`docs/design.md`](docs/design.md) | 核心设计：图谱漫游机制、四维打分（PPR + 激活 + 跳数配额）、技术栈与产品形态 |
| [`docs/positioning.md`](docs/positioning.md) | 战略定位：笔记库 = agent 记忆的「激活层」、LLM Wiki 互补、边界与明确不做 |
| [`docs/roadmap.md`](docs/roadmap.md) | 总路线图：阶段 1 引擎核心 + Web UI 完善（作者自用）/ 2 插件薄壳（M2），含依赖链与状态 |
| [`docs/plugin-dev-plan.md`](docs/plugin-dev-plan.md) | **插件开发计划（M2）**：生命周期四态机 / 多平台分发 / 插件×AI 协作架构。⚠️ 注意：具体插件代码在独立仓库开发（不在本仓库），本仓库只承载引擎内核（与插件唯一的共享物是 `docs/api-contract.md`） |
| [`docs/frontend.md`](docs/frontend.md) | 前端计划（Web UI）：插件化前置 + UI/UX 打磨规范 + 测试速查与交接 |
| [`docs/backend-backlog.md`](docs/backend-backlog.md) | 后端积压清单：性能优化、similar/export/touch 统计、CLI/MCP 打磨 |
| [`docs/api-contract.md`](docs/api-contract.md) | API 契约：14 端点 + 鉴权（插件仓库与引擎的唯一共享物，改 API 必同步） |
| [`docs/history/`](docs/history/) | 历史决策/验证归档（内容已吸收进 design/roadmap，保留完整叙事） |

## 特别鸣谢

- **[dsh-mneme](https://github.com/modusensus/dsh-mneme)** —— 激活引擎哲学的起点（结构 × 激活、白盒）
- **[恐龙工具箱](https://github.com/hqweay/orca-hqweay-go)**（虎鲸笔记插件）—— 随机漫步交互灵感
- **[leiden-go](https://github.com/vsuryav/leiden-go)**（MIT）—— 社区发现（Leiden）实现
- **[bbolt](https://github.com/etcd-io/bbolt)**（MIT）—— 存储层（etcd 团队维护的 BoltDB 活跃 fork）
- **[graphwizard](https://github.com/intelligrit/graphwizard)**（MIT）—— 图算法正确性参考（本项目实现为自研）

## License

MIT License —— see [LICENSE](LICENSE).
