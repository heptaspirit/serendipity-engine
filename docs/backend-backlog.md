# Serendipity Engine · 后端积压清单（Backend Backlog）

> 日期：2026-08-23（由外部审计机会清单定稿并汇入仓库）
> 来源：与用户讨论（2026-08-23）+ 代码评审（graph/store/sync/roam）+ 调研（PPR）
> 性质：**后端做什么**——性能优化与功能缺口的可选清单，不承诺全做。原则：**新增旁路，不修改核心；守住红线，不污染已验证行为**。
> 相关：战略定位 [`docs/positioning.md`](positioning.md) · 前端计划 [`docs/frontend.md`](frontend.md) · 组件架构 [`docs/architecture/`](architecture/) · 历史的验证/决策 [`docs/history/`](history/)。

> 目标：在不增加过多无用功能、保留后端简洁可靠的前提下，找出值得做的性能优化与功能缺口。

## 一、总览

| 类别 | 项目 | 价值 | 风险 |
|---|---|---|---|
| 性能优化 | Stats 缓存 / PPR 提前收敛 / TextSearch 小写缓存 / Store 增量写 | 大库（数万节点）时显著 | ✅ Stats 缓存已落地（v0.1.11）；其余前三个低，Store 增量写中 |
| 功能缺口 | 结构相似节点（similar） | **高**——补语义缺口的白盒方案 | ✅ v0.1.11（Jaccard）→ v0.1.12 升级 **Adamic-Adar**（度加权，抗枢纽偏置） |
| 功能缺口 | 漫游导出（export） | 中高——工作流闭环 | ✅ v0.1.11 已落地（/api/roam?export=1 → Markdown） |
| 功能缺口 | touch 统计 API | 中——反馈闭环只读第一步 | ✅ v0.1.11 已落地（/api/touch/stats 只读聚合，绝不反馈排序）；v0.1.12 加幽灵 touch 过滤 |
| 功能缺口 | 单节点详情（graph.node） | **最缺的"确认这是什么"** | ✅ v0.1.11 已落地（graph.NodeDetail + /api/node + MCP graph.node） |
| 功能缺口 | 社区发现（Leiden）→ 诊断层 | 中——知识缺口诊断 + 簇级导航 | ✅ v0.1.12 落地（leiden-go vendor + /api/communities + MCP graph.community） |
| 功能缺口 | LLM Wiki adapter 画像（`llm-wiki`） | 中——真实性门槛下唯一可接受的 LLM 数据源兼容 | ✅ v0.1.12 落地（VaultProfile ExcludedFiles + 内置画像 llm-wiki + 结构探测 + watch 排除同源） |

## 一.5 开发纪律（2026-08-24 用户拍板，横切所有后端开发）

> 原则级表述见 [`docs/architecture/00-overview.md`](architecture/00-overview.md) §2 设计哲学第 6 条（工程纪律）；本节为操作细节与 first case。

**文件组织**：
1. **单文件 500 行左右，最好不超千行**——超过即拆（现有最大 web/server.go 787，安全但留意）
2. **按领域域拆文件，不按函数碎片化**：同领域相关函数放一个文件（如 `structure.go` 装聚类系数 + K-Core 两个小函数）
3. **算法/模块 = 包级可复用函数**——独立导出，为未来接口暴露（如 MCP `graph.community`）直接可调用（现状 graph 包已是此模式：activation/score/similar 各一文件）

**依赖策略**：
4. **第三方算法库可引入**（用户拍板）——"克制"指**零依赖单二进制**（不背运行时/网络栈/服务），不是"永远不用库"。条件：MIT 类宽松许可 + **go.sum 以 pseudo-version 锁定版本**（版本不可变、上游免疫——本仓库沿用 go.sum 而非 vendor 大树，与已用 sqlite/yaml 一致）+ README attribution 一行。（可选 vendor 锁全树，本仓库不引入。）
5. **first case：Leiden 社区发现用 `github.com/vsuryav/leiden-go`（MIT）**——手写 Leiden 会超 500 行红线（比 Louvain 复杂，含 refinement 阶段）。✅ v0.1.12 已引入（go.sum 锁定），`community.go` 只是适配层（~80 行）。注意：**Config 必须从 `DefaultConfig()` 起再覆盖 resolution/seed**——直接用 `&Config{Resolution, RandomSeed}` 会让 MaxIterations=0，Leiden 循环不跑、全部节点各成一簇（实测抓出）。落地细节见 [`docs/history/agent-memory-research.md`](history/agent-memory-research.md) 附录 D.4.4。

## 二、性能优化（克制、低风险、有代码实据）

| 优化 | 代码实据 | 收益 | 风险 |
|---|---|---|---|
| **缓存 `g.Stats()`** | `roam.go:136` 每次漫游都调 `g.Stats()`（全图并查集 + 枢纽排序），图不变结果就不变；`roam.go:213` rollSeed 也用 | 纯收益——查询响应省掉全图遍历；refresh 换图后失效即可 | 零风险 |
| **PPR 提前收敛** | `activation.go:23` 固定 60 次幂迭代；PPR 是收敛的，实际 20-30 次即稳定 | 大图（数万节点）查询提速 2 倍级 | 需单测锁住输出不变性 |
| **TextSearch 小写缓存** | `graph.go:251` 每次查询对全图 Text 做 `strings.ToLower`（分配 + 遍历） | 高频查询显著提速；文档未变则小写文本可缓存 | 低，注意 refresh 联动失效 |
| **Store 增量写** | `store.go:274` 全量 `DELETE + INSERT`，即便只有 1 个文档变化 | 大库刷新从全量写变增量写（v1.5 已规划） | 中——涉及对账语义，建议 M1 后 |

**诚实说明**：前三项对当前千级节点是"感觉不到但架构正确"的优化；真正到数万节点（虎鲸块级）才见分晓。建议 Stats 缓存顺手做掉（10 分钟），其余进 backlog 等规模信号。

**〔2026-08-23 外部验证〕Store 增量写获 Graphiti（Zep 时序知识图谱）印证**——其「增量 episode 摄入」机制（新数据只动它触及的实体/边，不批量重算）正是本 backlog「增量 vs 全量对账」的成熟实现参考。见 [`docs/history/agent-memory-research.md`](history/agent-memory-research.md) §4.3。

### 二.1 存储层选型：SQLite → bbolt（2026-08-24 已定，✅ v0.1.13 已落地）

> **决策**：把 `modernc.org/sqlite` 换成 `go.etcd.io/bbolt`（MIT，etcd 团队维护的 BoltDB 活跃 fork，Kubernetes 全集群状态生产验证）。用户拍板"收益明确，何乐而不为"。
> **状态（v0.1.13，2026-08-25）**：✅ 已落地——`store.go` 整体换 bbolt v1.5.0（四个 bucket：docs/links/touch/renames），签名保持、调用点零改动；无迁移（旧 `.sqlite` 直接删，refresh 重建）；端到端验证通过（真实 vault 150 文档 + 幂等刷新「未变 150」+ roam 回读）。现代编译从分钟级 → 秒级。**注意**：`modernc.org/sqlite` 仍留在依赖中——`internal/adapter/orca.go` 用它解析虎鲸活库快照（源数据读取，与 store 持久化无关）。

- **为什么值得换（seren 的 SQLite 是「伪关系型」，SQL 能力没用足）**：
  - 四张表实际用法全是 KV 语义：documents（全量快照 id→Document）/ links（有向行 a\x00b→1.0）/ touch（自增事件流+5000 截断）/ renames（映射 old→new）——bbolt 四个 bucket 一一映射，比关系模型更自然
  - 唯一"SQL 味"的 TouchStats（GROUP BY）对 5000 条事件，内存遍历毫秒级即可替代
  - **编译时间**：modernc 是 C→Go 翻译（生成代码百万行级），`go build` 分钟级地狱；bbolt 原生 Go 秒级
  - **二进制体积 / 哲学纯度**：modernc 驱动几 MB 且是"纯 Go 零依赖"里最不纯的一块（C 翻译）；bbolt 是 etcd 官方原生库
- **为什么代价可控（关键红利）**：
  - **无迁移负担**——SQLite 存的是派生快照（源数据 = vault，源数据权威原则），换 bbolt 后旧 sqlite 直接删，下次 refresh 重建
  - **侵入面 = internal/store 包内**——Save/Load/AppendTouch/TouchStats/LoadRenames 等签名保持（它们不暴露 SQL 类型），sync/web/mcp/cmd 调用点零改动
  - bbolt 写事务串行（单写者）——seren 单进程无并发写者，天然适配
- **bucket 布局**：`docs`（id→docRow JSON，去 Refs）/ `links`（a\x00b→1.0）/ `touch`（seq→{ts,target,src}，cursor 截断 5000）/ `renames`（old→new）
- **注意点**：touch 截断语义保持（删最旧）；幽灵 touch 过滤改为遍历时查 docs bucket 存在性（O(1)，P5）；DBPath 语义保持（<vault>/.serendipity/db-<hash>，扩展名已定为 **.bbolt**）
- **与 gonum 无关**：bbolt 是存储层接受，不构成"算法框架层"（gonum）也接受的依据——见 agent-memory-research.md D.4.2/D.4.3 边界澄清
- **时机**：✅ 已落地（v0.1.13，2026-08-25），2 阶段（插件薄壳）开始前的基础设施收尾完成；roadmap #16

### 二.2 bbolt 解锁的有趣能力（候选，非承诺，2026-08-25 梳理）

bbolt 不是"更轻的 SQLite"——它的三个硬特性（**零 schema / COW 多版本 / 单文件纯 Go 可嵌入**）解锁的是 SQLite 不擅长、但契合 serendipity 精神（漫游 / 探索 / 白盒 / 离线）的能力。下面按"工程优化"与"有趣"两档列候选，**#16 落地后再评估排期**，不在 M1/M2 阻塞项内。对**已有端点**的性能 / 准确性增强（增量写、PPR 缓存、TextSearch 索引、幽灵过滤 O(1) 等）已单列精确映射至 **§二.3**。

**A. 工程优化（顺手可得，几乎免费）**
1. **真·增量写（v1.5 优化自然落地）**：当前 `Save` 全量 `DELETE+INSERT`（幂等但 O(N)）。bbolt 单写者 + 逐 key `Put`，可只写变更文档/边——大库 refresh 从"全量重写"变"差量落盘"，秒级→亚秒；无需 schema 迁移（建 bucket 零成本）。
2. **幽灵过滤 O(1)**：touch/链接指向已删节点，bbolt 用 `bucket.Has(id)` 一次命中，替代现在 SQLite 的 `IN (SELECT id FROM documents)` 子查询。dead-link 检测从此零成本——可升格为"知识缺口 / 断链"一等功能。
3. **PPR / 激活结果缓存**：per-node 缓存 keyed `(node, paramHash)` 落 bucket，roam 首算后"瞬时"回访；参数变才失效。白盒可解释性不变（缓存只加速，不污染算法）。

**B. 有趣（bbolt 独有、契合漫游精神）**
4. **图谱时间旅行（版本化快照）**：bbolt 的 COW 天然适合存多版本——每次 refresh 把图状态（docs/links/潜在关联）快照进 `snap-<ts>` bucket。用户可"回到上周的图谱"，看某条潜在关联是何时浮现、哪些边是 AI 后来加的。探索的"成长史"成为可回看的对象。
5. **探索日志 / 偶遇时刻（append-only event log）**：touch 已是事件流，bbolt 顺序键 append 超顺——把它升格为完整"探索日志"：记录每次漫游起点、走过的路径、点击的节点。由此生成"本周你探索了哪些角落""两篇看似无关的笔记其实通过 N 跳相连（**偶遇时刻**）"。这是 serendipity 字面意义的"奇遇记"。
6. **离线优先的插件侧缓存 / AI 边 sidecar**：bbolt 纯 Go 可嵌入——插件不再依赖外部 JSON 文件存 AI 确认边，而可携一个微型 bbolt 库（`<vault>/.serendipity-ai/links.bbolt`），离线、原子、可被引擎开机探活。AI 边与引擎派生图彻底解耦又便携（呼应 plugin-ai-cooperation.md 的 sidecar 方案）。
7. **跨库元索引（multi-vault）**：引擎已支持多 vault，bbolt 单文件可移植——建中心 `index` bucket 映射 `node→vault`，让漫游跨越多个笔记库形成统一知识网（"我在 A 库写的 X 和 B 库写的 Y 其实是同一主题"）。
8. **What-if 实验图（fork 即建桶）**：建 bucket 零成本——用户可 fork 当前图、套用 AI 建议边、对比"加之前 vs 加之后"的漫游差异，满意再 commit 回主图。把"AI 补图"从黑箱变成可 A/B 的实验。

**C. 边界（别过度）**
- bbolt 无查询语言：`TextSearch`（前缀/模糊搜）可借 bucket 有序键自建索引，但复杂全文检索仍别硬上（必要时外挂）。
- 上述均不引入新算法依赖，符合"组件即插即用、不引入框架"的边界（§二.1 末）。

### 二.3 bbolt 对已有能力的性能 / 准确性增强（候选，依赖 #16，2026-08-25）

下列**不是新功能**，而是让已上线的 11 个端点更快 / 更准。每一项对应一个具体现有能力，bbolt 的硬特性（零 schema / mmap / MVCC / 有序键 / O(1) Has）是其前提。**#16 落地后评估排期，均为 ⏸ 可选。** 与 §二.2 的关系：本节 = **增强已有能力**（用户可感知的性能 / 准确性，优先级更高）；§二.2 B = **净新增有趣能力**（时间旅行 / 探索日志等）。

| # | 增强对象（现有能力） | bbolt 机制 | 效果（before → after） | 状态 |
|---|---|---|---|---|
| P1 | `/api/refresh` 全量写（`Save` DELETE+INSERT，O(N)） | 单写者 + 逐 key `Put`，diff 只写变更文档/边 | refresh 成本随**变化量**而非总量 → 大库秒级→亚秒，watch 频繁刷新更顺 | ✅ v0.1.13（差值 Put/Delete，重复 Save 零写入；实测幂等刷新「未变 150」零写） |
| P2 | 启动 `Load` / 每次 refresh 开库 | mmap 内存映射，无 WAL 恢复、无 SQL 解析 | 开库/加载更快；serve 启动更轻 | ✅ v0.1.13（bbolt 原生 mmap；AppendTouch 高频路径 NoSync） |
| P3 | `/api/roam`、`/api/relation` 的 PPR（每次调用从零迭代） | `ppr` bucket 缓存 `PPR(node, paramsHash)` | 同锚点/同参数重复查询瞬时；首次仍走算法，白盒不变 | ⏸ 可选（graph 层改造，等规模信号） |
| P4 | `/api/similar` 的 Adamic-Adar（每次 O(邻居²)） | `similar` bucket 缓存 `AA(node)` | 重复查同节点相似瞬时；证据/排除逻辑不变 | ⏸ 可选（graph 层改造，等规模信号） |
| P5 | `/api/touch/stats` 幽灵过滤（`IN (SELECT id FROM documents)` 子查询 join） | `bucket.Has(id)` O(1) | 5000 行聚合免 join；幽灵过滤零成本，热度榜更稳 | ✅ v0.1.13（docs bucket 存在性判断替代 SQL join） |
| P6 | `/api/stats` 悬空链接计算（`graph.Build` 逐边查存在性） | 链接目标存在性 `bucket.Has` O(1) | dangling 计算更快；可给**完整**明细（非截断 50），断链诊断更准 | ⏸ 可选（graph.Build 内存态，当前无瓶颈） |
| P7 | `graph.TextSearch`（漫游 `q` / fallback 的全文扫描，O(N) 内存扫） | bbolt 有序键建 `idx` bucket（token/前缀 → nodeIDs） | 前缀/子串检索瞬时；支撑搜索框 autocomplete，不再每次扫全库正文 | ⏸ 可选（graph 层改造，等规模信号） |
| P8 | serve-while-refresh（`is_pending` 自动刷新时仍服务） | MVCC：刷新写事务与漫游读事务互不阻塞，读见一致快照 | 刷新不再短暂阻塞读；大库自动刷新体验更顺 | ✅ 内存层已有（server.go RWMutex 换图，读接口持 RLock 不阻塞）；bbolt MVCC 无额外收益 |

> 说明：P1–P2 属"存储层自身提速"；P3–P4 属"算法结果缓存"（缓存只加速、不污染白盒）；P5–P6 属"存在性查询 O(1)"（顺带让 dangling 从截断升级为完整）；P7 属"索引提速已有搜索"；P8 属"并发读不阻塞"。均不新增算法依赖。

## 三、功能缺口

### 3.1 结构相似节点（similar）—— 最高价值

- **概念**：找"共同邻居多但互不链接"的节点对（Jaccard 相似度）。两篇笔记都关联同一批人物但彼此无链接 → 大概率主题相近。
- **价值**：**embedding 语义轴的纯结构替代**——白盒、零依赖、证据可解释（"因为都链接了人物B/C"）。把"不做 embedding"的决策从妥协变成有替代方案。
- **实现**：`graph.go` 加 `Similar(id, k)`（局部按需计算，O(邻居²)，不预计算全图）；Web 加 `/api/similar`；卡片加「相似」按钮；UI 展示共享邻居清单作为证据。
- **风险**：Jaccard 度偏置（枢纽天然像所有人）→ 复用 rollSeed 排除逻辑（枢纽/空标题/孤立）+ 相似度阈值；区分"相关(roam)"与"相似(similar)"语义（不同入口、不同标签）。
- **〔2026-08-24 借鉴〕graphwizard 的 Adamic-Adar（链接预测）**是同类白盒结构相似的正确实现参考（共同邻居度加权 `Σ 1/log(deg)`，~20 行手写）。✅ v0.1.12 已升级落地（`graph.Similar` 用 AA，证据/排除/排序全复用）——Jaccard 比例错位 + 共享邻居不加权问题一并解决。

### 3.2 漫游导出（export）—— 工作流闭环

- **概念**：`/api/roam?export=1`（或 `Accept: text/markdown`）把当前簇渲染为 Markdown 卡片清单（标题 + 类型 + hop + 路径 + 分数），一键带走。
- **价值**：漫游发现的东西能沉淀进笔记，而不是截图/手抄。对创作工作流（如小说人物关系网）尤其有用。
- **实现**：服务端把同一份 JSON 结果渲染成 Markdown——复用现有管线，零侵入；默认路径（无参数）行为完全不变。
- **风险**：导出语义要明确 = 卡片清单而非重新生成笔记；导出不额外 touch。

### 3.3 touch 统计 API —— 反馈闭环只读第一步

- **概念**：`GET /api/touch/stats` 返回"哪些节点被反复点击、哪些边被反复激活"（只读分析）。
- **价值**：先看懂数据，再决定是否演化边权（呼应 M1）；回答"越用越准"是否有依据。
- **实现**：`store.go` touch 表已埋点（5000 条容量），只读 SQL 查询即可。
- **风险**：**绝不反馈到排序/hot**——否则等于偷偷启动边权演化，违背 v0.1.4"埋点只记录不演化"决策；不进 MCP（隐私敏感）。**〔2026-08-24 边际情况〕幽灵 touch 缺口**：纯 SQL 聚合不关联节点表，已删节点仍进热度榜 → ✅ v0.1.12 修复（targets 关联 documents 过滤；sources 是自由查询词不过滤）。
- **〔2026-08-23 外部验证〕A-MEM（NeurIPS 2025）的记忆演化机制与「touch 边权演化」同题**——A-MEM 用 LLM 判断"新信息是否更新旧记忆"，seren 计划用**用户点击数据**判断"哪些边值得强化"，更白盒（行为证据 vs LLM 猜测）。这是「touch 边权演化」方向正确的印证（见 [`docs/history/agent-memory-research.md`](history/agent-memory-research.md) §4.4）。
- **〔远期设计参考〕OpenViking 的 `used()` success 字段**——「点了但没深入」比「点了」更有信号，是 touch 统计 API 远期演进时的设计参考（§4.2 #5）。

### 3.4 社区发现（Leiden）→ 诊断层（知识缺口诊断，等场景）

- **概念**：对无向无权图跑 Leiden 社区检测，回答「库里有哪些主题簇、哪些区域互不相连」——这是「激活层」之外的第二种 agent 价值：**诊断层**（agent 不用遍历全库就能定位知识缺口）。✅ v0.1.12 落地（`internal/graph/community.go` + `/api/communities` + MCP `graph.community`；leiden-go MIT vendor）。
- **选型（已定，2026-08-24）**：算法用 **Leiden**（Louvain 官方改进版，保证 well-connected 社区）；Go 实现用 `github.com/vsuryav/leiden-go`（MIT、string 节点直通、零新增依赖、自带 Modularity 质量分，go.sum 锁定）。孤立节点（度=0）检测前过滤（其诊断信号由 `Stats().Orphans` 承接）。落地草图见 [`docs/history/agent-memory-research.md`](history/agent-memory-research.md) 附录 D.4。
- **原则「算法等场景」**：社区发现/介数中心性的价值要落到具体功能（知识缺口诊断 API / 结构导航视图）才有意义——不提前做，等「诊断层」功能排期时顺带实现。先拿已有连通分量做粗糙版（哪些区域互不相连），不够再上 Leiden。
- **选型（2026-08-24 用户拍板）**：Leiden 直接引 `github.com/vsuryav/leiden-go`（MIT，vendor 锁版本）——手写会超行数红线，引库后 `community.go` 仅适配层（~50 行）。**文件组织见 §一.5 开发纪律**：community.go（Leiden）/ centrality.go（Betweenness）/ structure.go（聚类系数 + K-Core）/ similar.go 扩展（Adamic-Adar）。落地时直接执行，无需重新调研。
- **未来 MCP**：✅ v0.1.12 顺势加了 `graph.community` 工具，与 roam/random/relation/node/similar/stats 并列（七件套）。
- **可选真增量**：介数中心性（桥接节点检测，诊断层信号，Brandes O(nm) 千级~2 万节点跑得起）；最短路径/紧密度/SCC 不引入（hop 路径已覆盖，无场景）。
- **合规**：MIT，硬性要求仅「保留版权声明」（vendor 时 Go 自动记录 LICENSE）；README 标注一行 attribution。

### 3.5 LLM Wiki adapter 画像（真实性门槛下唯一「可接受」的 LLM 数据源）

- **背景**：LLM Wiki（Karpathy 模式）有 raw 事实锚点 + 人力维护，是真实性门槛下唯一「可接受但默认谨慎」的 LLM 生成数据源（见 [`docs/positioning.md`](positioning.md) §六）。对 Obsidian 里做 LLM Wiki 的用户，adapter 值得做专门兼容。
- **改动（A 方案已定，2026-08-23）—— ✅ v0.1.12 全部落地**：
  1. **VaultProfile 新增 `ExcludedFiles []string`**（文件名级排除）——现有 `ExcludedDirs` 只管目录（SkipDir）；LLM Wiki 的 index.md/log.md 是文件级。实现：`ParseVault` / `ParseVaultIncremental` 的 WalkDir 加文件名判断（各 3 行），收益不止 LLM Wiki。
  2. **新增内置画像 `llm-wiki`**：`excluded_dirs: [raw, audit, output, outputs]` + `excluded_files: [index.md, log.md, CLAUDE.md, AGENTS.md]`，其余字段继承 default-obsidian（`ProfileByName` 现合并默认填充）。用法 `--profile-name llm-wiki`，开箱即用。
  3. **结构发现器**：`adapter.DetectLLMWiki`（raw/ + wiki/index.md 组合命中）→ 启动/索引日志提示「检测到 LLM Wiki 结构，可用 `--profile-name llm-wiki`」（只提示不自动启用）。
- **watch 排除同源**（缺口③）：`watch.NewVaultChecker` 现同时接受画像 `ExcludedDirs`（目录）与 `ExcludedFiles`（文件名）——与 ParseVault 排除同源，raw/（及 index.md/log.md）变化不再无效触发刷新。
- **边界（诚实声明）**：wiki/ 页面是 LLM 写的，进图 = 接受「二手编译内容」（链接仍真实，内容可信度降级）；`index.md` 排除**必须**通过显式画像启用，绝不进默认画像（Obsidian 用户常拿 index.md 做 MOC，文件名相同无法区分手写 vs LLM 生成）；raw/ 整体不扫（含其中 .md）——零新增解析能力，只认 wiki/ 里的 markdown。

### 3.6 潜在关联（restrained approximate edges；用户向名，内部沿用「近似边 / kind=approx」，2026-08-25 重新定调）

> 取代原「mentions API（虚拟引用 / 未链接提及，AC 文本扫描）」方案。原方案对 Obsidian 正文做标题子串扫描、把命中当"虚拟边"——经外部审计与虎鲸真实库验证后**否决**：暴力枚举会成倍加边、中文子串边界无解、且虎鲸根本没有模糊提及通道（见下方实测）。新方案改为**引擎从拓扑多算法估算近似边**，与「AI 生态补位」定位一致。

- **定位（克制 + AI 补位）**：用户完全可以把笔记成批喂给 LLM 让它判链接——那是 AI 的生态位，引擎**不占**。引擎只做**算法层、永远在线、免费、可解释**的拓扑近似：用多重算法评估两个已有节点"近似相关"的程度，结果作为**有界、明确标注为近似**的边。这是 agent 记忆生态的**补位**，不是对手。
- **红线改写（关键）**：
  - ❌ 旧：「虚拟引用是引擎猜的边，绝不进图」。
  - ✅ 新：**禁语义/LLM 推断边；允许有界、明确标注 `kind=approx`、带算法溯源与低权的拓扑潜在关联（内部称近似边）**。潜在关联永远显示来源（"与 B 共享 3 邻居，AA=2.1 + 常先后打开"），反而更白盒。
- **算法（多算法评估，复用 #12 底座）**：
  1. **候选生成有界**：只对每节点 2-hop 邻域打分（图稀疏，O(N·d²)），不枚举全图。
  2. **拓扑指数**：Adamic-Adar / Jaccard / Resource-Allocation（取公共邻居，与 #12 similar 共用 `commonNeighbors`）。
  3. **行为信号（来自插件运行期，非 adapter）**：`touch` 表的共现频率（已有 5000 上限，按"**插件上报**的同窗口先后打开"增量累加，零刷新成本）——第二正交信号。**关键澄清**：co-touch 只有插件 L1 知道（用户在看哪两篇、先后关系）；adapter 是静态摄入翻译器（读文件/库快照），`Document` 只有 `MTime`（修改时间）无访问时间，**绝不在 adapter 里扫描 co-touch**。引擎 touch 表本就从外部事件累加，自洽。
  4. **排名聚合**：Borda / 加权聚合多指数 → 每节点取 **top-K**（K=2~3）最近似者。
  5. **节流是防爆核心**：边数硬上限 = K×N（与图密度无关），区别于暴力文本扫描的"每匹配都成边"。
- **落图与可解释**：近似边 `kind=approx`、低权（λ′<λ，或 AA/RA 分数作权重）进入 PPR/Activate；roam 输出**永远标注 kind + 算法名**。`Stats` 暴露 link/approx 边构成。
- **性能（2026-08-25 估算 + TestOrca 实测印证极稀疏）**：refresh 期一次性 pass，候选来自 2-hop（O(sparse)）；N=540（TestOrca 实测）边 188、平均度 0.7，2-hop 候选极少；推算 N=100k/d=5 约 1–2s、产出 ≤30 万近似边、~12MB。查询零新增开销（预计算缓存）。原 #5「PPR 提前收敛」因边数有界，优先级下调。
- **adapter 解耦（实测关键结论）**：**近似边完全在引擎层算，适配器只给真实链接**——Obsidian 的 `[[ ]]`/md 链接、虎鲸的 `BlockRef` 都是结构化真实边，适配器无需任何正文扫描。这彻底删掉了原方案的"adapter 模糊提及能力声明 / Document.Mentions"复杂度，**多软件适配反而零额外成本**。
- **虎鲸实测验证（TestOrca，2026-08-25）**：复制副本离线解析（遵守不读活库红线）。`integrity_check=ok`；表清单无 mention/backlink/unlinked 表，链接 100% 来自 `BlockRef`（结构化 ID 引用，type 1/2）；`Block/BlockAlias/BlockRef` schema 与适配器逐列对齐；悬空引用 0/0；聚合 540 文档、解析边 188（自环 0、悬空 0）、平均度 0.7（极稀疏）。→ **确认虎鲸无模糊提及通道，适配器只吃真实链接即可，与克制设计完美契合**。附带：`BlockAlias.name_p = to_pinyin(name)` 生成列要求确定性函数，适配器 `init()` 注册 `to_pinyin` stub（已验证必要，否则 schema 复制/校验报 malformed）。
- **与 similar / 诊断层互补**：similar（#12）= 结构相似入口；潜在关联层 = 把"近似"固化成可漫游的有界边；诊断层（#10）可取"高近似度但零真实链接"的隐藏枢纽信号。
- **落点（✅ v0.1.13 已落地）**：引擎层潜在关联 pass（`graph.PotentialLinks`，2-hop + AA/Jaccard/RA + Borda 聚合 + top-K 节流）→ 暴露 `GET /api/suggest-links`（top-K 待审清单，替代原 `/api/mentions`；登记 api-contract §12）。**未落图**：候选是 kind=approx 形态而非真实边，不进 PPR/Activate——落图需带权边改造 + 改变已验证 roam 行为（红线 1 精神），留待 M2 插件 AI 协作真正需要漫游进近似边时评估（plugin-ai-cooperation Flow 1 消费 suggest-links 即可）。**co-touch 行为信号**：需插件 L1 经 touch 通道喂入（backlog §3.6 #3 澄清），v0.1.13 为纯拓扑。前端节点详情页"潜在关联"展示留作 M2 插件 UI。
- **优先级**：✅ v0.1.13 落地（roadmap #15）；落图/co-touch 与 #12（Adamic-Adar 底座）、#10（诊断层）、M2 插件协同。

#### 3.6.0 与 similar / 旧「虚拟链接」的区别（命名澄清）

三者都围绕「没明文写的联系」，但机制与产物不同，避免混淆：

| 项目 | 机制 | 产物 | 状态 |
|---|---|---|---|
| 旧「虚拟链接 / mentions」（已否决） | 扫描笔记**正文文本**（标题子串）把命中当虚拟边 | 虚拟边 | ❌ 否决：暴力枚举 / 中文边界无解 / 虎鲸无模糊提及通道 |
| similar（#1/#12，已上线） | **查询**：给定节点实时算结构相似排行（Adamic-Adar） | 带证据的 ranked 列表，**不往图里加边** | ✅ 已上线 |
| **潜在关联（本功能，#15）** | **落图**：refresh 时从**真实链接图**算有界、低权、`kind=approx` 的近似边并写入图 | 可漫游的 `kind=approx` 边，参与 roam/Activate | ✅ v0.1.13 候选清单已落地（suggest-links）；**落图留 M2** |

- 潜在关联与 similar **共用 Adamic-Adar 算法**（复用 #12 的 `commonNeighbors` 底座），但一个是「查看器」、一个是「富集器」：`similar` 回答"X 跟谁像"，而潜在关联把"像"固化成可漫游的边。
- 潜在关联是旧「虚拟链接 / mentions」诉求的**克制重做版**：机制从"扫文本造虚拟边"换成"从真实图估算拓扑近似边"，不再依赖正文扫描。内部技术名仍沿用「近似边 / kind=approx」，对外统称「潜在关联」。

> **用户视角文案（届时用于 UI / README）**
> serendipity 发现你没写、但结构上看似相关的笔记对，自动标成「潜在关联」——权重低、标注来源，漫游时会顺着它们扩散，你也可以忽略。

#### 3.6.1 对照 Obsidian / 虎鲸开发文档的 adapter 复核（2026-08-25）

读完两份官方开发文档后，对已完成 adapter 工作的复核结论（多数确认无问题，三处需记）：

- **共现/touch 信号不来自 adapter（对 §3.6 #3 的纠错澄清）**：「同窗口先后打开」的 co-touch 源头是**插件运行期（L1）**——只有插件知道用户当前在看哪篇、开了哪两篇以及先后。`ParseVault`/`ParseOrcaDB` 是**静态摄入翻译器**（读文件 / 库快照），`Document` 只有 `MTime`（修改时间）没有访问时间，无法派生 co-touch。→ 近似边的行为信号**必须经插件运行时通道喂给引擎**（touch 事件流 / overlay），绝不能回头去 adapter 扫描。引擎 `touch` 表本就从外部累加，这一点自洽。
- **Obsidian `.canvas` 白板不可见（已知缺口，**远期 / M2 之后，低优先**）**：开发文档确认 Canvas 是 Obsidian 一等节点，存为 `.canvas` JSON（nodes/edges），**非 markdown**。当前 `ParseVault` 只 `WalkDir` `.md`（obsidian.go:74），白板节点及其到 `.md` 的引用**完全不进图**。v1 可接受（个人库白板多作草稿）；**用户本人使用 `.canvas` 也很少，对其功用与使用环境尚不清晰，故不纳入 M2 排期，列为远期项**。待明确需求后再做 `obsidian_canvas.go` 解析 canvas 节点（按 path 引用 `.md`）补为 `Refs`。
- **悬空链接不建 phantom 节点（与 Obsidian 开世界图的有意差异）**：`graph.Build`（graph.go:58-62）对 `Refs` 指向的不存在节点只记 `Dangling`、不建节点。Obsidian 自身会显示 `[[不存在笔记]]` 的幽灵节点，我们只渲染已存在文档。**这是有意的闭世界选择**（不无中生有；近似边 / AI 建议边也只挂已有节点），但需用户知晓此差异——插件 / AI 层未来可把「你链了 X 但 X 不存在」做成创建提示。
- **已确认无问题的部分**：`![[...]]` 嵌入被 `linkRe`（`\[\[([^\]]+)\]\]`）正常捕获为链接（合理，嵌入是强关系信号）；虎鲸 `CopyDBForRead` + `to_pinyin` 确定性 stub 双路径快照经 TestOrca 实测稳健；`BlockRef` 只取结构化真实边，与克制设计完美契合。
- **可选增强（非必须，不立即做）**：虎鲸 `BlockRef.type`（1/2/3）暂被扁平成无类型 `Refs`（alias 列留作边标签备用，orca.go:177）。若未来做 typed overlay 边可保留 `refType`，但当前克制设计下不必。

### 3.7 touch 行为信号子系统（M2 排期，2026-08-26 定稿）

> 背景：Obsidian 插件已能记录 touch（节点点击）。当前 touch 与图库同存于 `db-<hash>.bbolt` 的 touch bucket——本该是**不可从 vault 派生的原始行为信号**，却被当成可丢弃派生快照的一部分（"源数据权威原则"允许删 `db-*.bbolt` 重建，touch 会连坐丢失）。M2 阶段把 touch 升级为**独立、有生命周期、有告知机制**的行为信号子系统。
> 设计立场源为工作区草稿 `serendipity-drive/serendipity-positioning.md` §十一，已吸收内联于本节（2026-08-26 定稿，引擎仓库内不再依赖外部文件）。

**设计立场（已定）**：
- touch 是长期资产但带噪声：点击流混大量无意识操作（误点 / 划过），不是每条都该永生。
- 长期未被注意的 touch 淘汰 = 健康遗忘：半年没被 surfaced 的 touch 信息量极低，留着只是噪声。
- 记忆巩固模型：原始 touch（感觉记忆，易失、有上限、会淘汰）→ 通知 / surfacing（复述，无意识变有意识的瞬间）→ 备份 / 固化（长期记忆，蒸馏过的聚合，非原始事件）。
- 红线不变：touch 只读、不演化边权（杜绝「点击→边权变→结果变→再点击」正反馈跑飞）。

**3.7.1 存储解耦（修复原 bug）**：
- touch 从图库 `db-<hash>.bbolt` 拆为独立 store：`<vault>/.serendipity/touch-<hash>.bbolt`（独立 bucket 集：`touch` / `meta` / `backups`）。
- 图库重建 / 误删（"源数据权威原则"允许删派生快照）**不再连坐** touch。touch 是 **first-class 但 secondary** 的 store：独立于图拓扑、自带保留 / 备份策略、不受图库重建影响。
- 图库 `docs/links/renames` 仍为可重建派生数据；touch 为不可重建原始信号。
- **实现注意（跨 store 访问）**：digest 生成与 TouchStats 的幽灵过滤需要**同时打开 touch store 与图库**（touch 库取事件、图库 `docs` bucket 做存在性过滤 + ID→标题解析）。函数签名带双 dbPath（touchDBPath + graphDBPath）；serve 持有两者。touch 库打开复用 `open()`（已建父目录），图库只读打开即可。

**3.7.2 digest 触发与内容（阈值双逻辑，计数优先）**：
- 计数触发（主）：自上次 digest 起累计 touch ≥ `digest_count`（默认 500）。
- 间隔触发（兜底）：距上次 digest ≥ `digest_days`（默认 3 天）。
- 计数优先 = 评估时先判计数，计数达标即触发，间隔仅兜底。
- **启动补查**：serve 启动时检查一次——距上次 digest ≥ `digest_days` 则立即补生成（引擎未跑期间错过的间隔兜底不丢）。成本 = bbolt meta 一次读。
- **顺序铁律：通知先于淘汰**。流水线：累积 → 达阈值先生成 digest → 宽限期 → 仍未被用户 act 的 touch 才淘汰。淘汰不得与通知竞速（否则用户永远没机会看一眼）。
- **通知形态（被动，不主动弹窗）**：CLI / MCP 不主动弹出；至多「有新的 digest 可供查看」轻量状态提醒（非模态、不阻断工作流）。用户 / 插件需经只读接口**主动查询**才取回内容。
- digest 内容：窗口内 touch 聚合 TopN targets + TopN sources + 时间跨度 + 新增总数；指向**具体节点 / 聚类**（「X/Y/Z 聚成簇，疑似 A 主题升温——要不要连一下」），不是「你点了 N 次」。targets 必须做幽灵过滤（关联图库 `docs` 存在性），标题经图库解析。

**3.7.3 digest 出口（被动告知 + 可查询；引擎不写 vault）**：
- **(a) 引擎零写用户目录**：引擎**不生成、不写入任何 vault 文件**——`serendipity-digest-*.md` 由**前端插件**在用户主动触发导出时生成（引擎保持"源数据权威原则"下零写 vault 的既有边界）。
- **(b) 只读接口暴露**（被动、可查询，全部只读）：
  - REST `GET /api/touch/digest`：返回最新 digest（含唯一 `id`、窗口聚合 TopN、时间跨度、新增总数）。无 digest 时返回空摘要。
  - MCP `seren_touch_digest`：同语义的只读工具，AI / 插件主动查询时返回最新 digest 或摘要。
  - `/api/stats` 增加 `digest_available: bool`：自上次被读取/ack 后有新 digest 时为 true（轻量状态提醒的开关）。
- **已读判定（ack）**：`POST /api/touch/digest/ack` 把 digest id 记入 touch store `meta` bucket（`last_ack_id`），`digest_available` 随之转 false。ack 只写 meta、不碰 touch 事件、不反馈排序，符合红线。
- **导出（插件侧，非引擎）**：插件读 `GET /api/touch/digest` 内容，用户确认后经 `app.vault` 写入 `serendipity-digest-<YYYYMMDD-HHMMSS>.md`（**带时间戳防同日冲突**；中英双语由插件 i18n 生成）。文件保留策略归插件侧（vault 内用户自管），引擎不管、不轮转。
- 告知层级保持 ambient、低频——刻意不让无意识行为被强行拽成有意识提醒。

**3.7.4 摘要备份（聚合快照，有上限，独立于 digest 出口）**：
- 每次 digest 同步滚一份**聚合排序后的快照**（只留算法认为高价值的部分，TopN，非原始事件），存入 touch store 的 `backups` bucket。
- 上限 `backup_max`（默认 5）份，超则自动删最旧 → 存储不爆炸，等于「用算法把排序后认为有价值的信息留着了」。
- **与 3.7.3(b) 区分**：备份是 store 内聚合快照（算法长期记忆，TopN，不可读，无 ack 概念）；digest 接口是可读报告。两者独立、互不复用。

**3.7.5 参数配置（YAML，与 profile.yaml 同 convention）**：
- 落 `<vault>/.serendipity/touch.yaml`（与 `profile.yaml` 同目录、同 YAML + 注释风格），随库走、可手改、便携。
- 引擎启动读取并钳制到区间，缺省用默认：

| 参数 | 默认 | 区间 | 说明 |
|---|---|---|---|
| `digest_count` | 500 | [200, 600] | 计数触发阈值（主） |
| `digest_days` | 3 | [1, 5] | 间隔触发阈值，单位天（兜底） |
| `backup_max` | 5 | [1, 20] | 聚合快照保留上限，超则删最旧 |

> 无 `digest_max`：digest 不落文件、不轮转（3.7.3(a) 改由插件导出）；文件保留策略在插件侧。早期讨论曾误记为 `config.json`；经核对既有参数配置一律 YAML（`profile.yaml`），故 touch 配置统一 YAML。

**3.7.6 落地边界（M2 开发，代码未动）**：
- 引擎代码改动：store 拆分（touch 独立文件 + meta/backups bucket）→ digest 触发（AppendTouch 后检查 + 启动补查）→ digest 生成（双 store 聚合）→ 备份轮转 → `GET /api/touch/digest` + ack + `/api/stats.digest_available` → MCP `seren_touch_digest` → `touch.yaml` 加载。
- 与 M2 插件壳（iframe 嵌 Web UI）契合：插件负责被动状态提醒（`digest_available`）/ 查询展示 / **导出到 vault**（用户触发，非弹窗）；引擎负责生成 + 只读接口 + 备份。引擎零写 vault。
- 同步义务：新端点与 MCP 工具登记 [`docs/api-contract.md`](api-contract.md)；插件 `seren-api.d.ts` 同步（D5）。

## 四、风险分析与红线（防污染已验证行为）

**必须隔离的三条红线：**
1. **similar 绝不并入 roam 管线**——serendipity 结果分布是验证过的（36 单测），只能独立入口
2. **touch 统计绝不反馈到排序/hot**——否则等于偷偷启动边权演化，违背克制设计
3. **export 不改变默认行为**——可选参数，默认路径零回归

**共性问题：**
- API 契约膨胀：每加端点都要同步 [`docs/api-contract.md`](api-contract.md)（插件唯一共享物）。建议三个端点一起加、一起登记，集中一次契约变更。
- 测试成本：similar 4-5 个单测 + export 2-3 个 + 统计 2-3 个，36 → 45+。
- 前端膨胀：`index.html` 720 行单文件。统计面板做成独立折叠面板（如"关系"面板），不挤占主界面。

### 缓存/累积状态盘点（2026-08-23 评审，防无限增长）

> 全量盘点项目所有累积状态。结论：**绝大多数有界；renames 表曾无上限，v0.1.11 链式折叠后已收敛**。
> 以下条目"看着情况删"——修复后即勾掉，不永久占位。

| 状态 | 位置 | 上限机制 | 状态 |
|---|---|---|---|
| touch 埋点表 | store.go AppendTouch | 5000 条硬上限，超出删最旧 | ✅ 有界 |
| 随机漫步 recent ring | web/server.go:112 | 32 个上限，超出截断 | ✅ 有界 |
| watch 文件快照 | watch.go:85 | = 文件数，删文件同步删 key | ✅ 有界 |
| 前端 localStorage 参数 | index.html | 白名单 key，值有界 | ✅ 有界 |
| 内存图（graph） | 全程 | = 节点数，refresh 整体替换不累积 | ✅ 有界 |
| revision 计数 | web/server.go:107 | int 溢出需 21 亿次刷新 | ✅ 实际不可能 |
| SQLite WAL | store | 未设 wal_autocheckpoint，长跑 + 频繁 touch 可能缓慢增长 | ✅ v0.1.11 已设 `PRAGMA wal_autocheckpoint=1000` |
| **renames 表** | store.go SaveRenames + sync.go MergeRenames | 曾无上限（链式中间环累积） | ✅ v0.1.11 已修复（`collapseChains` 只留链头→最终目标） |
| 日志 | watch/web log.Printf | 走 stderr 不落盘 | ✅ 安全 |

**renames 表风险**：`MergeRenames`（sync.go:379）失效逻辑 = 旧名重现才删；"目标消失但旧名未重现"的中间环节点永久保留（为链式改名传递解析）。**文件反复改名（A→B→C→D…）时每轮留一行，条目数 = 历史改名总次数，理论无上限**（实际每条几十字节，千次改名才几十 KB，严重度低）。**✅ v0.1.11 修法**：`collapseChains` 只保留"链头→最终目标"直达映射，丢弃被其他映射覆盖的中间环（A→B、B→C 存在时 A→B 可删）——条目数从"历史改名总次数"收敛为"仍存活的链头数"。语义权衡（backlog §四）：中间名是改名过程的短暂状态，Obsidian 内改名自动更新引用、文件系统手动改名时引用仍指向原始名（链头），中间名引用在实践中不存在或极罕见，丢弃换取有界增长。

**〔2026-08-23 借鉴〕Graphiti 的「边失效而非删除」给出另一条路**：新事实与旧事实矛盾时，旧边打 `invalid_at` 时间戳**保留**（可查询任意历史时点的图状态），而非删除——「历史映射有价值，用失效标记而非删除」。这与 renames 中间环的「清理 vs 标记失效」是同一个权衡，值得在 M1 增量写落地时重新评估（见 [`docs/history/agent-memory-research.md`](history/agent-memory-research.md) §4.3）。

**空查询 + 全图锚定**：`Resolve` 对空串 Contains 恒真——`roam.Compute`（roam.go:64）已拦截，但未来新入口必须同样拦截（计算放大，非存储放大）。

### 刷新边际情况与体验增强（2026-08-24 用户提出，全链路审查）

> 背景：自动刷新 1min 节流（watch throttle），用户增删改后存在边际情况。审查链路：watch → sync.Diff → graph.Build → touch。

**A 类：中间态（1min 窗口内"建了又删"）——已免疫，无需处理**

快照对账 + 图整体重建使"最终态一致"即可；建块+链接、建块+链接+删除、改名又改回（A→B→A）、轮询漏掉瞬态文件，全部安全。**不要加事件级中间态恢复逻辑**（复杂度换不来价值）。

**B 类：最终态不一致——两缺口**

| 边际情况 | 现状 | 处理 |
|---|---|---|
| 删块没清链接（悬挂链接） | ✅ graph.Build 统计 `Dangling`，悬空不进图（漫游不崩） | ✅ **缺口① v0.1.12 已落地**：`g.DanglingRefs()` 返回 `{source,target}` 明细，`/api/stats` 暴露 `dangling_refs`（截断 50）→ 统计面板/诊断层可见"有悬空链接该修" |
| 点击过已删节点（幽灵 touch） | ⚠️ `TouchStats` 纯 SQL 聚合，**不关联节点表**——已删节点仍出现在热度榜 | ✅ **缺口② v0.1.12 已落地**：targets 关联 documents 表过滤已删节点；sources 是自由查询词不过滤 |
| 改名链（A→B→C）/ 重名消歧 ID 变化 / 同秒多改 / 孤立节点 | ✅ 均已处理（MergeRenames/ApplyRenames/RenameTouch；pathSimilarity 救回；全量 diff 免疫；Orphans 统计） | 无需动 |

**C 类：LLM Wiki 联动（roadmap #9 绑定）**

- ✅ **缺口③ v0.1.12 已落地**：`watch.NewVaultChecker` 现同时接受画像 `ExcludedDirs`（目录）与 `ExcludedFiles`（文件名），与 ParseVault 排除同源——raw/（及 index.md/log.md）变化不再无效触发刷新。

**体验增强：事前提示 + 手动刷新联动（用户方案）—— ✅ v0.1.12 全部落地**

- 现状：手动刷新按钮 ✅（v0.1.2）+ 自动刷新**后**提示 ✅（v0.1.4 轮询 revision）；缺"刷新**前**有待刷新"提示
- 方案：`/api/stats` 加 `is_pending` 字段（暴露 watch pending）→ 前端轮询（已有 setInterval）显示"库有变化，将自动刷新 · [立即刷新]"（复用 #refresh）→ M2 插件 Obsidian 状态栏 / 虎鲸 notify 同款
- 粒度诚实：事前提示只能到"有变化"（watch 未解析，不知增删）；具体明细在刷新后的 diff 摘要
- 细节：手动刷新需**清 pending**，否则 watch 下个 tick 可能重复自动刷一次（幂等无害但浪费）——✅ v0.1.12 `refreshFn` 手动刷新成功后 `pending.Store(false)` 实现

## 五、CLI 打磨三件套（人机双消费者，2026-08-23 用户提出）

> CLI 是「双消费者」：人是第一消费者，agent（shell 直调场景）是次级——MCP 才是 AI 正式通道。
> 实测现状（v0.1.11 源码）：三件套**已全部落地**——子命令级 help（`seren help <cmd>` /
> `<cmd> -h`）、`--json` 结构化输出（roam/index/refresh）、退出码语义化（0 成功 / 2 用法
> 错误 / 1 运行时错误）。

| # | 改进 | 现状 | 建议 |
|---|---|---|---|
| 1 | **子命令级 help** | `seren roam -h` 不显示 roam 特有参数 | `seren help roam` / `-h` → 只显示该子命令参数（纯文本，零依赖，半天量级） |
| 2 | **--json 结构化输出** | 全是人类格式化文本，AI/脚本 grep 硬解析 | `roam --json` / `index --json` / `refresh --json` → 复用现有结构体（roam.Outcome / sync.Result）序列化 |
| 3 | **退出码语义化** | 只有 0/1，参数错误与运行时错误不分 | 0 成功 / 2 参数或用法错误 / 1 运行时错误（解析失败、库不存在）——agent 能自纠而非误报 |

**补充发现**：仓库根 `seren.exe` 是旧二进制（源码 v0.1.11）——本地测试二进制 `scratch\seren.exe`
gitignore 不入库；正式发布用 GitHub Actions 平台构建（本地二进制不上库）。

 ✅（v0.1.11）已完成开发

## 六、MCP 工具扩展评估（2026-08-23 用户提出）

> 结构不变（stdio JSON-RPC 薄协议、只读、零第三方依赖），只评估工具集扩展。
> 现状七件套：`graph.stats` / `graph.roam` / `graph.random` / `graph.relation` /
> `graph.node` / `graph.similar` / `graph.community`（v0.1.9 四件 → v0.1.11 六 → v0.1.12 七，
> 见 [`docs/architecture/07-mcp.md`](architecture/07-mcp.md)）。

### 建议新增（按价值排序）

| 工具 | 作用 | 理由 |
|---|---|---|
| **graph.node** | 单节点详情：Title/Type/Aliases/Tags/Text 摘要 + 邻居列表 + 被引用（backlinks） | **最该加**——AI 漫游到节点后需要"确认这是不是我要的"，现在缺这个基础动作；与 [`docs/frontend.md`](frontend.md) #3 节点详情 API 同源，一次实现两端受益 |
| **graph.similar** | 结构相似节点（Jaccard 孪生） | 与 §3.1 `similar` 联动；AI 判断"哪些笔记在说同一件事"时用，补语义缺口的白盒替代 |
| **graph.search** | 显式全文检索（LIKE 命中列表） | roam 无锚点时已降级全文，显式暴露更清晰——AI 精确找词场景 |
| **graph.list**（可选） | 按类型/标签过滤列节点（分页） | AI 摸库用（"列出所有 type=人物 的节点"）；stats 只有数量没名单 |

**〔2026-08-24 远期〕graph.community**：Leiden 社区检测落地时（§3.4，诊断层排期时）顺势加，与 roam/random/relation/stats 并列——「算法等场景」，不提前加。

### 明确不加（克制边界）

- **graph.read（读正文全文）**：引擎是"发现层"不是"阅读层"（见 [`docs/history/product-form.md`](history/product-form.md)）——正文由 Obsidian/虎鲸宿主负责；node 只给 Text 摘要截断。
- **写类工具（touch / refresh / 边权）**：违背只读红线，AI 会话不能改动本地状态。
- **graph.hot**：graph.stats 已含 TopHubs，增量价值≈0。

## 七、研究借鉴清单（agent 记忆库研究，远期参考，非行动项）

> 来源：[`docs/history/agent-memory-research.md`](history/agent-memory-research.md)（OpenViking / Graphiti / A-MEM 研究，2026-08-23/24）。已验证无需改动的见该文档 §4.1。

| # | 借鉴点 | 来源 | 对应 seren 场景 | 状态 |
|---|---|---|---|---|
| 1 | 节点详情分级 L0/L1（summary 截断 / overview 摘要+导航） | OpenViking | 前端 #3 节点详情 API | 并入 [frontend](frontend.md) #3 |
| 2 | 多锚点漫游（多源 PPR 替代 LLM 意图分析） | OpenViking | Web 手动多锚点 → 发散增强 | 远期评估 |
| 3 | 确定性排序（稳定采样） | OpenViking | 锚点排序稳定（Resolve map 序） | 与既有「锚点排序稳定」合并 |
| 4 | 簇级导航（按 hop 分组视图） | OpenViking | roam 结果可读性 | 远期评估 |
| 5 | used() success 字段 | OpenViking | touch 统计 API 设计 | 见 §3.3 |
| 6 | 边失效而非删除（invalid_at） | Graphiti | renames 表「清理 vs 标记失效」权衡 | 见 §四 |
| 7 | 双时态模型（valid/transaction time） | Graphiti | touch 边权演化前的「边状态」模板 | 远期评估 |
| 8 | 增量 episode 摄入 | Graphiti | Store 增量写（等 M1） | 见 §二 |
| 9 | 记忆演化（新经验触发旧记忆修订） | A-MEM | touch 边权演化方向（保持行为驱动而非 LLM） | 见 §3.3 |
| 10 | 链接类型化（方向/类型） | A-MEM | Refs 无类型双链的远期扩展 | 远期评估（谨慎） |
| 11 | Adamic-Adar / 链接预测 | graphwizard | similar 的白盒结构相似升级选项 | 见 §3.1 |
| 12 | 聚类系数 / K-Core / 介数 | graphwizard | 诊断层「簇紧密度 / 核心边缘 / 桥接节点」信号 | 与 §3.4 同批 |

**共同指向**：三个系统都验证了 seren 的两个 backlog 方向——**Store 增量写**（Graphiti 增量摄入）和 **touch 边权演化**（A-MEM 记忆演化）——走在正确轨道上；同时它们全都依赖 LLM 判断（黑盒），而 seren 用真实链接 + 用户行为（白盒），这正是差异化的根基。

## 八、优先级建议

1. **Stats 缓存**（✅ v0.1.11 已落地：Graph.Stats memoize，refresh 换图即新缓存）
2. **similar 结构相似**（✅ v0.1.11 已落地：graph.Similar + /api/similar + MCP graph.similar）
3. **export 漫游导出**（✅ v0.1.11 已落地：/api/roam?export=1 → text/markdown 卡片清单）
4. **touch 统计 API**（✅ v0.1.11 已落地：/api/touch/stats 只读聚合，绝不反馈排序）
5. **graph.node**（✅ v0.1.11 已落地：graph.NodeDetail + /api/node + MCP graph.node）
6. **CLI 打磨三件套**（✅ v0.1.11 已落地：`seren help <cmd>` 子命令帮助 / `--json` 结构化输出 / 退出码 0-2-1）
7. **LLM Wiki adapter 画像**（✅ v0.1.12：`llm-wiki` 画像 + `ExcludedFiles` + watch 排除同源 + 结构探测）
8. **社区发现（Leiden）**（✅ v0.1.12：leiden-go vendor，/api/communities + MCP graph.community；诊断层排期时使用）
9. **刷新一致性补全**（✅ v0.1.12：缺口① DanglingRefs 明细 + 缺口② 幽灵 touch 过滤）
10. **刷新体验增强**（✅ v0.1.12：is_pending 事前提示 + 手动刷新清 pending）

## 九、与现有文档的关系

| 文档 | 主题 | 边界 |
|---|---|---|
| [`docs/positioning.md`](positioning.md) | 战略定位（笔记库=agent 记忆、embedding 边界、数据源真实性门槛） | 为什么做 |
| [`docs/frontend.md`](frontend.md) | Web UI 功能计划（P0 插件化前置 / P0.5 hero / P1 / P2） | 前端做什么 |
| **本文件** | 后端性能优化 + 功能缺口 + CLI/MCP 打磨 + 风险红线 | 后端做什么 |

## 十、后续动作

- [x] similar / export / touch-stats 三个端点登记进 [`docs/api-contract.md`](api-contract.md)（✅ v0.1.11）
- [x] similar 实现复用 rollSeed 排除逻辑 + 相似度阈值 + 共享邻居证据（✅ v0.1.11）
- [x] Stats 缓存与 refresh 换图联动失效（✅ v0.1.11：Graph 不可变 memoize，换图即新缓存）
- [x] graph.node 与前端节点详情 API 一起实现（✅ v0.1.11，两端受益）
- [x] CLI 三件套（help/--json/退出码）完成（✅ v0.1.11，onboarding 体验）
- [ ] 重建 seren.exe（源码 v0.1.13；仓库根 seren.exe 为旧二进制，用 GitHub Actions 平台构建，本地进制不入库——**注：用户拍板本版暂不做 GitHub Actions 自动构建**，本地 `scratch/seren.exe` 仅供本地联调）
- [x] LLM Wiki adapter 画像：VaultProfile `ExcludedFiles` + 内置画像 `llm-wiki` + 结构发现器（✅ v0.1.12，含 watch 排除同源——缺口③）
- [x] 社区发现（Leiden）实现（✅ v0.1.12：/api/communities + MCP graph.community）
- [x] 边际情况三待办（✅ v0.1.12）：缺口① 悬挂链接明细 DanglingRefs（stats.dangling_refs）、缺口② 幽灵 touch 过滤（targets 关联 documents）
- [x] 刷新体验增强（✅ v0.1.12）：/api/stats 加 `is_pending` + 前端"有待刷新"提示条 + 手动刷新清 pending
- [ ] 潜在关联（§3.6，✅ v0.1.13 候选清单落地）：`graph.PotentialLinks` + `/api/suggest-links`（2-hop + AA/Jaccard/RA + Borda + top-K 节流，带算法与共享邻居证据）；**剩余留 M2**：落图进 PPR（带权边改造）、co-touch 行为信号（插件 L1 喂入）、前端"潜在关联"展示。**禁语义/LLM 推断边**；共现信号须由插件 L1 经 touch 通道喂入，不在 adapter 扫描。
- [x] renames 中间环清理（✅ v0.1.11：MergeRenames 链式折叠，只留链头→最终目标）
- [x] store 加 `PRAGMA wal_autocheckpoint=1000`（✅ v0.1.11）
