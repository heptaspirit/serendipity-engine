# 架构总览 · Serendipity Engine

> 本文档面向**未来接手本项目的开发者/AI**：先读这一份建立全局，再按需深入
> 各分册（数据模型 / 适配器 / 引擎 / 同步 / Web / 维护指南）。
> 设计文档 `docs/design.md` 是作者给自己看的设计过程记录（含评审与实测），
> 架构文档是**维护说明书**——以代码现状为准。

## 1. 这是什么

**Serendipity Engine（奇遇记引擎）**：给个人笔记的双链装上一套**查询驱动的激活引擎**。
用户输入一个点（笔记名 / 标签 / 任意词），引擎返回"一批筛选、排序、可解释的相关节点簇"——
结构分（查询锚定的 Personalized PageRank）+ 激活分（激活扩散）+ 跳数配额（serendipity 机制）。
一句话：**你问一个点，它给你一片。**

载体：Obsidian vault（文件解析）+ 虎鲸 Orca Note（SQLite 直读）。
产品形态：本地 CLI + Web UI（REST），三入口（CLI / Web / 未来 MCP）。

## 2. 设计哲学（作者原创，贯穿所有决策）

1. **结构 × 激活**：图结构提供"可能相关"，激活机制提供"此刻相关"——只有结构没有激活
   的 wiki 是死的；激活引擎让"wiki 真正跑起来"。（哲学源自作者在 dsh-mneme 图谱
   增强设计中的沉淀；同一套引擎，mneme 管 agent 记忆，本引擎管人的笔记导航。）
2. **白盒原则**：每条推荐可解释（激活路径 `A → B → C`）、可干预（点击续漫游）、
   可跳回原软件。不做黑盒。
3. **解析抽离（VaultProfile）**："解析方案不是放之四海皆准"——通用语法（`[[...]]`、
   markdown 链接、frontmatter、H1）代码固定；**语义映射（title/别名/标签/类型规则）YAML 画像化**，
   换库改 YAML 不改代码。
4. **克制设计（防正反馈跑飞）**：自动监听用轮询+节流合并、排除自身产物目录；
   反馈埋点只记录不演化边权；任何"点击→边权变→结果变→再点击"的循环都在源头切断。
   本地工具优先稳定，不追求实时。
5. **安全红线（不可违反）**：虎鲸 `Repo` 表含用户凭据（API key 等）——**绝不读取**；
   活库先做一致性快照再读（绝不锁活库）；个人数据不进 git。

## 3. 模块架构

```
┌─────────────────────────────────────────────────────────────┐
│ cmd/seren (CLI 入口)                                         │
│   index / roam / serve / refresh / profile-detect / version  │
└───────────────┬─────────────────────────────────────────────┘
                │ loadSource / parseSource（统一加载：--db > 虎鲸 .db > Obsidian vault）
                ▼
┌──────────── internal/adapter（格式翻译层 = VFS 哲学）────────┐
│  Document（统一图节点）  obsidian.go  orca.go  profile.go   │
│  OKF 通用格式 · VaultProfile 画像 · 快照（VACUUM INTO/拷贝） │
└───────────────┬─────────────────────────────────────────────┘
                │ []*adapter.Document
                ▼
┌──────────── internal/graph（内存图引擎）────────────────────┐
│  Build（无向邻接） Resolve（锚定 5 级） TextSearch（全文兜底）│
│  PPR（teleport 0.15） Activate（λ/θ/hops） Stats（统计）     │
└───────────────┬─────────────────────────────────────────────┘
                ▼
┌──────────── internal/score + internal/roam（漫游管线）───────┐
│  Rank：min-max 归一化融合 + 跳数配额（serendipity 旋钮）     │
│  Compute：锚定→PPR+激活→排除（种子/枢纽/结构类型）→融合→降级 │
└───────────────┬─────────────────────────────────────────────┘
                ▼
┌── internal/store（SQLite 持久化） internal/sync（对账 diff）─┐
│  documents/links/touch 表（全量重写幂等；touch 独立+容量上限）│
└───────────────┬─────────────────────────────────────────────┘
                ▼
┌── internal/watch（自动监听） internal/web（REST + 前端）─────┐
│  轮询+节流合并 · 排除自身产物 · 失败节流重试                 │
│  /api/stats hot roam refresh touch · go:embed index.html    │
└─────────────────────────────────────────────────────────────┘
```

依赖极简：标准库 + `gopkg.in/yaml.v3` + `modernc.org/sqlite`（纯 Go 零 CGO）。
无其他第三方依赖；无网络出口。

## 4. 关键数据流

- **index**：解析 → `graph.Build` → 统计 →（可选 `store.Save` 持久化）
- **roam**：`loadSource` → `roam.Compute`（锚定 → PPR+激活 → 排除 → 融合+跳数配额 → 降级兜底）
- **serve**：启动加载图 → REST 服务；`/api/refresh`（手动）与 watch（自动）共用同一
  `refreshFunc`：重解析 → `sync.Diff` 与上次状态比对 → `store.Save` → `ReplaceGraph`（revision+1）
- **watch**：轮询变化 → 节流窗口合并 → 触发上面的 refreshFunc
- **touch**：Web 点击 → `POST /api/touch` → store.touch 表（仅记录，不演化）

## 5. 目录结构

```
cmd/seren/           CLI 入口（子命令、参数解析、loadSource、refreshFunc）
internal/adapter/    Document 定义 + Obsidian/Orca 解析 + VaultProfile + 快照
internal/graph/      内存邻接表图引擎（Build/Resolve/PPR/Activate/TextSearch/Stats）
internal/score/      归一化融合打分 + 跳数配额
internal/roam/       漫游管线（CLI 与 Web 共用）
internal/store/      SQLite 持久化（documents/links/touch）
internal/sync/       对账 diff（增/删/改字段级明细）
internal/watch/      自动监听（轮询 + 节流，克制设计）
internal/web/        REST 服务 + 嵌入的前端页面（static/index.html）
docs/                design.md（设计过程）/ architecture/（维护文档，本文档套）
```

## 6. 版本脉络（速览）

| 版本 | 内容 |
|---|---|
| v0.1.0 | 初版：解析/建图/漫游/打分/CLI+Web |
| v0.1.1 | 虎鲸页面聚合（页面块 vs 内容块）；OKF 通用格式入默认解析 |
| v0.1.2 | 对账刷新（seren refresh + /api/refresh）；快照双路径 |
| v0.1.3 | 虎鲸空壳页面清理（container 类型化）；前端点击加固 |
| v0.1.4 | 自动监听、反馈埋点（touch）、虎鲸跳转（orca-note://） |

详细历史见 `PROGRESS_LOG.md`（本地，不入库）。
