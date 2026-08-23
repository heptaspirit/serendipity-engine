# Serendipity Engine · 奇遇记引擎

> **v0.1.5** · 图谱漫游：查询驱动的笔记导航——**你问一个点，它给你一片。**
> 给个人笔记的双链装上激活引擎（查询锚定 PPR + 激活扩散 + 跳数配额），让"wiki 真正跑起来"。
> 中文 README；[English](README.en.md)。

> 🙏 **灵感来源**：本项目的图谱增强哲学（**结构 × 激活**、激活扩散、白盒原则等）与
> [dsh-mneme](https://github.com/modusensus/dsh-mneme) 一脉相承——这套哲学出自我在参与
> mneme 图谱增强设计时的沉淀思考。当时突发奇想：同一套激活引擎，换个载体、换个人当消费者，
> 就是"让笔记导航活过来"。于是做了这套给人用的工具。与 mneme 同哲学、不同领域：
> mneme 管 agent 记忆，本引擎管**人的笔记导航**。

## 特性

- **查询驱动漫游**：输入笔记名 / 标签 / 任意词 → 输出一批筛选、排序、可解释的相关节点簇（带激活路径）
- **serendipity 机制**：跳数配额混合（1:2:3-hop = 50/30/20），保证"我没想到但确实相关"的深跳惊喜节点稳定出现
- **白盒可干预**：每条推荐可解释（激活路径）、可点击续漫游、可跳回原笔记软件
- **解析规则抽离**：VaultProfile 库画像（title/别名/标签/类型规则 YAML 化）+ `profile-detect` 新库自动探测——换库不改代码；Google OKF 通用格式入默认解析
- **双数据源**：Obsidian vault（文件解析）+ 虎鲸 Orca Note（SQLite 快照直读，Repo 凭据表绝不碰）
- **对账刷新**：`seren refresh` / Web `↻` / **自动监听**三路同步增删改（60s 节流合并，克制防正反馈）
- **改名迁移**（v0.1.5）：Obsidian 改名不再断链——内容哈希+路径相似度识别、他人链接重定向、touch 埋点迁移（持久化 renames 表）
- **关系查询**（v0.1.5）：`GET /api/relation?from=&to=`——两节点最短路径 + 双向 PPR 强度 + 证据链（white-box，为未来 MCP 暴露铺路）
- **反馈埋点**：点击记录 touch（独立表、容量上限；v1 不演化边权，杜绝跑飞）
- **跳回原软件**：Obsidian `obsidian://` / 虎鲸 `orca-note://` 跳转
- **三入口**：CLI / REST + Web UI / （未来 MCP）

## 设计哲学

1. **结构 × 激活**：图结构提供"可能相关"，激活机制提供"此刻相关"——只有结构没有激活的 wiki 是死的。
2. **白盒原则**：每条推荐可解释、可干预、可跳回原软件，不做黑盒。
3. **解析抽离**："解析方案不是放之四海皆准"——通用语法代码固定，语义映射（title/类型规则等）YAML 画像化。
4. **克制设计**：监听轮询+节流合并、排除自身产物；埋点只记录不演化——任何"点击→边权→结果"的正反馈循环都在源头切断，本地工具优先稳定。
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
**面向维护者的架构文档在 `docs/architecture/`**（数据模型 / 适配器 / 引擎 / 同步 / Web / 维护指南）。

## 快速开始

```powershell
# 构建（Go 1.26+）
go build -o seren.exe ./cmd/seren

# 漫游
.\seren.exe roam <vault> "寻找"               # Obsidian 库
.\seren.exe roam "D:\...\OrcaNote.db" "历史"    # 虎鲸库（.db 自动识别）

# Web UI（初始页漂浮热门节点，点击即漫游；自动监听默认开）
.\seren.exe serve <vault> --port 8080
#   Obsidian 加 --vault-name 启用卡片「打开 ↗」跳回笔记软件
#   虎鲸库自动用 orca-note:// 跳转（--repo 可覆盖库名）
#   --watch-off 关闭自动监听；--watch-interval / --watch-throttle 调频率

# 对账刷新（使用者增删改后同步，输出 增/删/改 明细）
.\seren.exe refresh <vault> --store <file.sqlite>

# 新库 onboarding
.\seren.exe profile-detect <陌生库>            # 自动产出画像 YAML

# 持久化（防重解析）
.\seren.exe index <vault> --persist
.\seren.exe roam <vault> "可乐" --db <store>
```

## 文档

| 文档 | 说明 |
|---|---|
| **`docs/architecture/`** | **架构文档（面向未来维护者）**：00 总览与哲学 / 01 数据模型 / 02 适配器 / 03 引擎 / 04 同步 / 05 Web / 06 维护指南 |
| [`docs/design.md`](docs/design.md) | 设计文档（修订版 v2，含评审决策 + spike 实测 + VaultProfile）——作者设计过程记录 |
| [`docs/frontend-issues.md`](docs/frontend-issues.md) | **前端问题记录与交接**（Web UI 历史问题/修复/验证 + 遗留待办 + 测试方法；前端专项 session 先读这里） |
| [`docs/DESIGN_REVIEW.md`](docs/DESIGN_REVIEW.md) | 设计评审 13 条决策（已全部接受） |
| [`docs/spike-report.md`](docs/spike-report.md) | Spike 验证报告（机制结论 / 参数重测，示例内容已脱敏） |
| [`docs/product-form.md`](docs/product-form.md) | 产品形态决策（跳回软件 vs 插件 vs MCP） |

> 开发日志与遗留问题见本地 `PROGRESS_LOG.md`（不入库）。

## 状态

v0.1.5（2026-08-22）：改名迁移（修订 #8，含 links 有向化修复）、关系查询
`/api/relation`、真实查询集定性验证（决策 #6 收尾）已完成。
下一步：反馈闭环观察（touch 数据）、Playwright 前端自动化测试、MCP server（v3）。

## License

MIT License — 见 [LICENSE](LICENSE)。
