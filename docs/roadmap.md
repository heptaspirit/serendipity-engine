# Serendipity Engine · 总路线图（Master Roadmap）

> 建立：2026-08-23（升级为**项目总路线图**——单一权威，把引擎 / 前端 / 插件安排进同一张表）。
> 原则（用户拍板 2026-08-23，2026-08-24 修订）：**优先让引擎能用、好用（作者自用闭环），再做插件让它在笔记软件里可用**。推广/营销不在项目开发考虑内——本仓库不承载任何推广内容。
> 分工：本文件管**阶段 / 依赖 / 状态**；细分计划在 [`backend-backlog.md`](backend-backlog.md)（后端）与 [`frontend.md`](frontend.md)（前端 + UI 规范）；插件开发计划 [`plugin-dev-plan.md`](plugin-dev-plan.md)（M2 执行计划）；插件×AI 协作 [`plugin-ai-cooperation.md`](plugin-ai-cooperation.md)（M2 AI 专题）；战略定位 [`positioning.md`](positioning.md)；设计 [`design.md`](design.md)；契约 [`api-contract.md`](api-contract.md)。

---

## 顶层目标

> 定位（positioning 一句话）：把用户笔记库变成 agent 的记忆——本引擎是记忆的「激活层」。
> **落地目标**：先让作者本人把「阅读 + 漫游」用起来（引擎 + Web UI 顺手）；再做插件让它对别人也可用、不硬核。

## 当前状态快照（2026-08-25）

- **引擎 v0.1.12**；**M0 已落地**（serve 安全前置 + API 契约 + MCP v3）；**74 单测绿**；Obsidian + 虎鲸真实库验证通过（双数据源、对账刷新、自动监听）。**v0.1.13 进行中：#16 bbolt 存储层替换已落地**（P1/P2/P5 顺手项完成，P3/P4/P7 等规模信号）。
- **M1 阶段 1 基本收尾**：similar 结构相似（Adamic-Adar）、graph.node 节点详情、export 漫游导出、touch 统计 API、Stats 缓存、renames 中间环清理、WAL autocheckpoint、CLI 三件套、**刷新一致性（悬挂链接明细 + 幽灵 touch 过滤）**、**刷新体验（is_pending 事前提示 + 手动即时刷新）**、**LLM Wiki adapter 画像（llm-wiki + ExcludedFiles + watch 排除同源）**、**Leiden 社区发现诊断层（/api/communities + MCP graph.community）**、**前端 JSON 契约测试**全部落地。
- **前端 P0（阶段 2 #1，插件化前置）本轮同步完成**：紧凑嵌入 `?embed=1` + postMessage 桥（`{type:'open',id}` / 宿主注入 theme/locale）+ i18n 中英双语全部文案；Web UI 完善（hero 改静态热门列表 / 侧滑抽屉统一面板 / 卡片 ID 收敛 + scores 折叠 + 🎲 升级主按钮）。
- **similar 实测反馈（2026-08-24）→ 已修复**：v0.1.11 的 Jaccard 被用户实测"挂出来的节点不太对"；v0.1.12 升级 **Adamic-Adar**（共同邻居度加权 `Σ 1/log(deg)`），度偏置与共享邻居不加权问题一并解决。
- **现状判断**：引擎侧（阶段 1）的能力面已覆盖作者自用闭环；前端 P0 就绪 → 下一步进 **M2 插件薄壳**（Obsidian / 虎鲸）。
- 未完成（阶段 1 剩余，均"等场景/可选"）：性能优化（PPR 提前收敛等 pre-bbolt 项仍等数万节点规模信号）、潜在关联（#15 已落地 suggest-links 候选清单；**落图进 PPR 与 co-touch 行为信号留待 M2 插件场景**）。
- **2026-08-24 新增方向**（诊断层 / LLM Wiki 画像）本轮已落地（Leiden `graph.community` + `llm-wiki` 画像）。

## 里程碑总览

| 里程碑 | 内容 | 阶段 | 状态 |
|---|---|---|---|
| **M0** | 插件前置（serve 安全）+ API 契约 + MCP v3 | — | ✅ 已落地 |
| **M1** | 引擎核心 + Web UI 完善（作者自用） | 阶段 1 | ✅ 能力面完成（剩余项均"等场景/可选"） |
| **M2** | 插件薄壳（对外可用） | 阶段 2 | 前端 P0 已就绪，可开工 |
| **M3** | 远期增强（canvas 检索 + bbolt 有趣能力） | 阶段 3 | ⏸ 远期（canvas 用户少用；bbolt 有趣档待 #16 落地后评估） |

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
| 5 | 性能（pre-bbolt）：PPR 提前收敛 / TextSearch 小写缓存 | [backend-backlog](backend-backlog.md) §二 | ⏸ 等数万节点规模信号；Stats 缓存 ✅（v0.1.11 顺手做掉）；bbolt 相关增量写/PPR·similar 缓存/索引（P1–P8）已并入 #16 顺手做 |
| 6 | 风险修复：renames 中间环 + WAL autocheckpoint | [backend-backlog](backend-backlog.md) §四 | ✅ v0.1.11（collapseChains + wal_autocheckpoint） |
| 7 | **Web UI 完善**：hero 静态（P0.5）、节点详情、相似/统计/导出面板、侧滑抽屉（§九） | [frontend](frontend.md) §三/§九 | ✅ v0.1.11 面板 + v0.1.12 hero 改静态 + 侧滑抽屉统一 |
| 8 | 前端测试：**JSON 契约测试**（优先）+ Playwright 可选 | [frontend](frontend.md) 附录 | ✅ v0.1.12 JSON 契约测试（10 端点循环校验）；Playwright 留作可选防回归 |
| 9 | **LLM Wiki adapter 画像**（`llm-wiki` + `ExcludedFiles` + **watch 排除同源**） | [backend-backlog](backend-backlog.md) §3.5 | ✅ v0.1.12；llm-wiki 画像 + 文件名级排除 + watch 同源 + 结构探测提示 |
| 10 | **诊断层（Leiden 社区发现 → 知识缺口诊断）** | [backend-backlog](backend-backlog.md) §3.4 · [history/agent-memory-research](history/agent-memory-research.md) 附录 E | ✅ v0.1.12（leiden-go vendor + /api/communities + MCP graph.community）；聚类系数/K-Core/Betweenness 作内部信号按需取用 |
| 11 | **CLI 打磨三件套** | [backend-backlog](backend-backlog.md) §五 | ✅ v0.1.11（seren help <cmd> / --json / 退出码 0-2-1） |
| 12 | **similar 评分升级：Jaccard → Adamic-Adar** | [backend-backlog](backend-backlog.md) §3.1 · [history/agent-memory-research](history/agent-memory-research.md) 附录 E | ✅ v0.1.12；用户实测驱动（2026-08-24），~20 行，证据/排除/排序全复用 |
| 13 | **刷新一致性补全**（悬挂链接明细 + 幽灵 touch 过滤） | [backend-backlog](backend-backlog.md) §四 | ✅ v0.1.12；`DanglingRefs()` 明细（stats.dangling_refs）+ TouchStats targets 关联 documents 过滤已删节点 |
| 14 | **刷新体验增强**（事前提示 + 手动即时刷新） | [backend-backlog](backend-backlog.md) §四 | ✅ v0.1.12；stats.is_pending + 前端"库有变化，将自动刷新 · 立即刷新"提示条 + 手动刷新清 pending |
| 15 | **潜在关联（多算法 / 有界 / 标注；原「近似边估计」）** | [backend-backlog](backend-backlog.md) §3.6 | ✅ v0.1.13（2026-08-25）：**suggest-links 待审清单落地**（2-hop 候选 + AA/Jaccard/RA + Borda 聚合 + top-K 节流 → `graph.PotentialLinks` + `GET /api/suggest-links`）；**未落图**（kind=approx 候选形态，不进 PPR/Activate——落图需带权边改造 + 改变已验证 roam 行为，等 M2 插件真需要漫游进近似边时评估）；co-touch 行为信号需插件 L1 喂入（v0.1.13 纯拓扑）；**取代原 mentions API（AC 文本扫描，已否决）**；复用 #12 底座；虎鲸实测验证 adapter 只给真实链接即可 |
| 16 | **存储层替换：SQLite → bbolt + 性能增强（顺手做）** | [backend-backlog](backend-backlog.md) §二.1 / §二.3 | ✅ v0.1.13（2026-08-25）：modernc → `go.etcd.io/bbolt` v1.5.0（原生 Go，MIT）；四表→四 bucket（docs/links/touch/renames）；**无迁移**（旧 `.sqlite` 直接删，refresh 重建）；侵入面 = store 包内，签名保持调用点零改动；端到端验证（真实 vault 150 文档 + 幂等刷新 + roam 回读）。**顺手项已落地**：P1 刷新增量写（差值 Put/Delete，重复 Save 零写入）、P2 mmap + NoSync（开库提速）、P5 幽灵 touch 过滤 O(1)（bucket.Has）；P8 serve-while-refresh 读不阻塞内存层已有（RWMutex 换图）。**剩余 ⏸ 等规模信号**：P3 PPR 缓存 / P4 similar 缓存 / P7 TextSearch 有序索引（graph 层改造，§二.3 注明"#16 落地后评估排期"） |

### 阶段 2 · 插件薄壳（M2；对外可用）

> 目标：让产品对非技术用户可用——当前后端引擎对外偏硬核，**插件化是「拿出来给别人」的关键**。

| # | 任务 | 落点 | 说明 |
|---|---|---|---|
| 1 | 前端 P0（紧凑嵌入 + postMessage 桥 + 节点详情 + i18n 双语） | [frontend](frontend.md) §二 | ✅ v0.1.12；`?embed=1` + `{type:'open'}` 桥 + 中英双语全部文案——插件化前置已就绪 |
| 2 | 插件薄壳 repos + 生命周期四态机（`serendipity-obsidian` / `serendipity-orca`，两个独立仓库，零构建时依赖；INSTALLED→CONFIGURED→RUNNING→DISABLED；managed/external 双模式） | [plugin-dev-plan](plugin-dev-plan.md) §五/§六 | 先 Obsidian（managed，spawn 核心）后 虎鲸（external，只连） |
| 3 | 核心引擎多平台分发（goreleaser + GitHub Actions 四平台 asset；Q2 插件内下载按钮） | [plugin-dev-plan](plugin-dev-plan.md) §四 | win-amd64 / mac-amd64 / mac-arm64 / linux-amd64 全出包 |
| 4 | 插件 × AI 协作（引擎端点 `suggest-links` / `edges` overlay + 插件 AIBackend） | [plugin-dev-plan](plugin-dev-plan.md) §九 · [plugin-ai-cooperation](plugin-ai-cooperation.md) | **引擎零 AI、只暴露接口与算法**；`suggest-links` 复用 #15 候选 pass，`edges` overlay 内存态（AI 建议边 `kind=ai` + 溯源，可撤销） |
| 5 | 插件市场发布 | [plugin-dev-plan](plugin-dev-plan.md) §四/§七 | Obsidian 社区目录 / 虎鲸 zip（手动解压） |
| 6 | **touch 行为信号子系统**（独立 store + digest 告知 + 聚合备份 + 被动只读） | [backend-backlog](backend-backlog.md) §3.7 | **M2 排期（2026-08-26 定稿）**：touch 从图库拆独立 `touch-<hash>.bbolt`（touch/meta/backups）；digest 触发（计数≥500 主 / 间隔≥3天 兜底，计数优先 + 启动补查）+ `GET /api/touch/digest` + ack + `/api/stats.digest_available` + MCP 只读 `seren_touch_digest` + 聚合备份 `backup_max`；YAML `touch.yaml` 配置。**引擎零写 vault**——`serendipity-digest-*.md` 由插件导出。代码未动 |

### 阶段 3 · 远期增强（M3）

> 目标：在 M2 插件化之后，补两类**非阻塞、按需**的远期能力——Obsidian 白板检索，以及 bbolt 落地后解锁的「有趣」功能（探索 / 时间旅行 / 跨库）。**均不阻塞 M1/M2**，等明确需求或 #16 落地后再评估排期。

| # | 任务 | 落点 | 说明 |
|---|---|---|---|
| 1 | **Obsidian `.canvas` 白板检索**（远期 / 低优先） | [backend-backlog](backend-backlog.md) §3.6.1 | 用户本人少用、功用待明确；`obsidian_canvas.go` 解析 `.canvas` JSON（nodes/edges，非 markdown）补为 `Refs`；当前 `ParseVault` 只 Walk `.md`，白板节点不进图 |
| 2 | **bbolt 有趣能力：图谱时间旅行**（COW 版本快照） | [backend-backlog](backend-backlog.md) §二.2 B | 每次 refresh COW 快照进 `snap-<ts>`，可"回到上周图谱"看潜在关联何时浮现、AI 边何时加入 |
| 3 | **bbolt 有趣能力：探索日志 / 偶遇时刻**（append-only event log） | [backend-backlog](backend-backlog.md) §二.2 B | touch 升格完整事件流，生成"两篇无关笔记其实 N 跳相连"的奇遇记录（serendipity 字面意义的奇遇记） |
| 4 | **bbolt 有趣能力：离线 AI 边 sidecar** | [backend-backlog](backend-backlog.md) §二.2 B | 插件携微型 bbolt 库存 AI 确认边，离线原子、引擎开机探活 |
| 5 | **bbolt 有趣能力：跨库元索引**（多 vault 统一知识网） | [backend-backlog](backend-backlog.md) §二.2 B | 多数据源漫游成统一图（`node→vault` 中心索引） |
| 6 | **bbolt 有趣能力：What-if 实验图**（fork 即建桶 A/B 对比） | [backend-backlog](backend-backlog.md) §二.2 B | 加 AI 边前 vs 后对比，把"AI 补图"变可实验 |

---

## 依赖链

```
引擎核心 + Web UI → 作者自用闭环（阶段 1）
前端 P0（紧凑嵌入 + postMessage）→ 插件薄壳 → 插件市场（阶段 2，对外可用）
graph.node → 前端节点详情 + MCP「确认这是什么」（阶段 1）
Leiden 社区发现 → 知识缺口诊断 API（阶段 1，诊断层排期时）
潜在关联（#15，阶段 1）→ 插件×AI 协作 `suggest-links` 候选清单（阶段 2 #4）
bbolt 落地 + 性能增强（#16，阶段 1，顺手做）→ 阶段 3 M3（canvas 检索 + bbolt 有趣能力）
```

---

## 明确不做（仓库级）

- TS / WASM / 移动端；GraphRAG / LLM 建图；embedding 内置 / 在线 API；图数据库；SaaS / 远程 / 云。
- **LLM 生成记忆库 adapter**（OpenViking / Graphiti / A-MEM / Mem0 建图）与其结构筛选/重排器——真实性门槛，见 [`positioning.md`](positioning.md) §六。
- **引擎内置 AI**：引擎零 AI 依赖，只暴露 REST/MCP 接口与确定性算法（图 / PPR / 激活 / similar / 潜在关联 / communities…），**绝不调用任何模型 / 在线 API**；语义研判全在插件层（AIBackend），见 [`plugin-ai-cooperation.md`](plugin-ai-cooperation.md)。
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
| 2026-08-25 | **M2 设计文档落地 + roadmap 同步**：新建 `plugin-dev-plan.md`（执行计划）与 `plugin-ai-cooperation.md`（插件×AI 协作，引擎仅暴露接口与算法、零 AI）；阶段 2 补插件生命周期四态机 / 多平台分发 / AI 协作引擎端点（#2–#5），并指向新 M2 计划文档；`明确不做` 补「引擎零 AI」；`.canvas` 适配推至 M2 之后（远期 / 低优先，用户本人少用）。 |
| 2026-08-25 | **roadmap 收编远期项**：性能增强（P1–P8，§二.3）并入 #16「bbolt 落地 + 性能增强（顺手做）」，取消独立 #17–#20；新增 **M3 阶段 3 远期增强**，收编 `.canvas` 白板检索（#1）与 bbolt 有趣能力（图谱时间旅行 / 探索日志·偶遇时刻 / 离线 AI 边 sidecar / 跨库元索引 / What-if 实验图，#2–#6）。里程碑总览、依赖链、未完成段、#5 性能行同步。 |
| 2026-08-25 | **#16 bbolt 存储层替换落地（v0.1.13）**：SQLite → bbolt v1.5.0，四表→四 bucket，无迁移，签名保持调用点零改动；顺手项 P1 增量写 / P2 mmap+NoSync / P5 幽灵过滤 O(1) 完成；P8 读不阻塞内存层已有。端到端验证通过（真实 vault 150 文档 + 幂等刷新 + roam 回读）。P3/P4/P7 留待规模信号（§二.3 可选）。 |
| 2026-08-25 | **#15 潜在关联落地（v0.1.13）**：suggest-links 待审清单（graph.PotentialLinks + GET /api/suggest-links，2-hop + AA/Jaccard/RA + Borda + top-K 节流，带算法与共享邻居证据）。未落图、co-touch 留 M2。端到端验证（真实 vault 输出有意义的人物↔人物/设定候选）。 |
| 2026-08-26 | **M2 排期：touch 行为信号子系统定稿入档**：`backend-backlog.md` 新增 §3.7（独立 store 拆分修原 bug / digest 触发双逻辑计数优先 + 启动补查 / 被动非弹窗 + REST·MCP 只读 / **引擎零写 vault，digest md 由插件导出** / 聚合备份 / YAML touch.yaml）；roadmap 阶段 2 补 #6 排期指向 §3.7。**代码未动，M2 实现待启动**。设计立场已吸收内联（原 `serendipity-drive/serendipity-positioning.md` §十一）。 |
