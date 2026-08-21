# Serendipity Engine · 奇遇记引擎

> **v0.1.0** · 图谱漫游：查询驱动的笔记导航——**你问一个点，它给你一片。**
> 给个人笔记的双链装上激活引擎（查询锚定 PPR + 激活扩散 + 跳数配额），让"wiki 真正跑起来"。

> 🙏 **灵感来源**：本项目的图谱增强哲学（**结构 × 激活**、激活扩散、白盒原则等）与 [dsh-mneme](https://github.com/modusensus/dsh-mneme) 一脉相承——这套哲学出自我在参与 mneme 图谱增强设计时的沉淀思考。当时突发奇想：同一套激活引擎，换个载体、换个人当消费者，就是"让笔记导航活过来"。于是做了这套给人用的工具。与 mneme 同哲学、不同领域：mneme 管 agent 记忆，本引擎管**人的笔记导航**。

## 特性

- **查询驱动漫游**：输入笔记名 / 标签 / 任意词 → 输出一批筛选、排序、可解释的相关节点簇（带激活路径）
- **serendipity 机制**：跳数配额混合（1:2:3-hop = 50/30/20），保证"我没想到但确实相关"的深跳惊喜节点稳定出现
- **白盒可干预**：每条推荐可解释（激活路径）、可点击续漫游、可跳回原笔记软件
- **解析规则抽离**：VaultProfile 库画像（title/别名/标签/类型规则 YAML 化）+ `profile-detect` 新库自动探测——换库不改代码
- **双数据源**：Obsidian vault（文件解析）+ 虎鲸 Orca Note（SQLite 直读，先拷贝再读，Repo 凭据表绝不碰）
- **三入口**：CLI / REST + Web UI / （未来 MCP）

## 快速开始

```powershell
# 构建（Go 1.26+，唯一第三方依赖 modernc.org/sqlite 纯 Go 零 CGO）
go build -o seren.exe ./cmd/seren

# 漫游
.\seren.exe roam <vault> "寻找"               # Obsidian 库
.\seren.exe roam "D:\...\OrcaNote.db" "历史"    # 虎鲸库（.db 自动识别）

# Web UI（初始页漂浮热门节点，点击即漫游）
.\seren.exe serve <vault> --port 8080
# Obsidian 库加 --vault-name 启用卡片"打开 ↗"跳回笔记软件

# 新库 onboarding
.\seren.exe profile-detect <陌生库>            # 自动产出画像 YAML

# 持久化（防重解析）
.\seren.exe index <vault> --persist
.\seren.exe roam <vault> "可乐" --db <store>
```

## 文档

| 文档 | 说明 |
|---|---|
| [`docs/design.md`](docs/design.md) | 设计文档（修订版 v2，含评审决策 + spike 实测 + VaultProfile 架构） |
| [`docs/DESIGN_REVIEW.md`](docs/DESIGN_REVIEW.md) | 设计评审 13 条决策（已全部接受） |
| [`docs/spike-report.md`](docs/spike-report.md) | Spike 验证报告（机制结论 / 参数重测 / 实现状态，示例内容已脱敏） |
| [`docs/product-form.md`](docs/product-form.md) | 产品形态决策（跳回软件 vs 插件 vs MCP） |

> 开发日志与遗留问题见本地 `PROGRESS_LOG.md`（不入库）。

## 状态

v0.1.0 初步可用（2026-08-21）。下一步：反馈闭环（touch 边权演化）、启动对账、MCP server。

## License

MIT License — 见 [LICENSE](LICENSE)。
