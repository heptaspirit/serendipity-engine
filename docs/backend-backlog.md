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

### 3.6 mentions API（虚拟引用 / 未链接提及，2026-08-24 讨论，可选做）

- **价值**：发现「正文提到但没链上」的潜在关系（Obsidian unlinked mentions 同款）。三个利用方向：
  1. **潜在链接发现**：`/api/mentions?id=X` 返回「提到了你但没链你」的节点清单 → 用户决定补不补 `[[]]`。
  2. **诊断层信号**：**隐藏枢纽**（被大量提及但零链接的节点 = 库里最值得整理的待连节点）+ 库级「链接成熟度」指标（提及数/节点数）。
  3. **touch 转化（远期）**：插件里候选点击确认 → 虚拟引用转真实链接，本身是高质量 touch 事件。
- **红线：绝不进图**——虚拟引用是引擎「猜」的边，不是用户写的；进图污染「图 = 真实链接」信任承诺（stance §二）。只做只读建议层 API，永不为边。
- **与 similar 互补**：similar = 结构侧（共同邻居），mentions = 文本侧（标题/别名出现在正文）——同一需求的两个正交维度。
- **实现 = 反向 Resolve**：对每篇文档 Text 找出哪些节点的 Title/Alias 出现其中、且不在该文档 Refs 里；复用 Resolve 锚定语义（MatchTitle / MatchAlias 级别，graph.go:217）。
- **性能方案（用户实测过万级节点，必须按此实现）**：
  - ❌ 朴素「每文档 × 每节点标题」子串匹配 = O(N² × TextLen)，万级节点 ≈ 1 亿次 Contains，不可接受。
  - ✅ **refresh 时建提及索引**：Aho-Corasick（或等价多模式扫描）把所有 Title/Alias 作模式，单遍扫描每篇 Text（O(总文本长度)），存 `mentionedTerm → []docIDs` 反向索引；查询 O(1)。refresh 本来就全量重建图，顺带建索引是自然延伸——与 Stats 缓存同一哲学（查询无关计算移到 refresh）。
  - 误报控制：标题长度阈值（≤2 字符跳过，防「数据」类泛词）；中文无空格分词，按子串 + 长度阈值即可，可接受。
  - 存储：内存索引，随图重建不落盘（派生数据，源数据权威原则——索引可重建）。
- **落点**：引擎 `/api/mentions`（与 similar/export/touch-stats 同批登记 api-contract.md）；前端节点详情页（frontend #3）加「未链接提及」区；诊断层（#10）落地时可取「隐藏枢纽」信号。
- **优先级**：可选做（低优先）——价值成立但非核心闭环；M1 内随手可做（与 #13 同批也行）。

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
- [ ] 重建 seren.exe（源码 v0.1.12；仓库根 seren.exe 为旧二进制，用 GitHub Actions 平台构建，本地进制不入库——**注：用户拍板本版暂不做 GitHub Actions 自动构建**，本地 `scratch/seren.exe` 仅供本地联调）
- [x] LLM Wiki adapter 画像：VaultProfile `ExcludedFiles` + 内置画像 `llm-wiki` + 结构发现器（✅ v0.1.12，含 watch 排除同源——缺口③）
- [x] 社区发现（Leiden）实现（✅ v0.1.12：/api/communities + MCP graph.community）
- [x] 边际情况三待办（✅ v0.1.12）：缺口① 悬挂链接明细 DanglingRefs（stats.dangling_refs）、缺口② 幽灵 touch 过滤（targets 关联 documents）
- [x] 刷新体验增强（✅ v0.1.12）：/api/stats 加 `is_pending` + 前端"有待刷新"提示条 + 手动刷新清 pending
- [ ] mentions API（§3.6，可选低优先）：refresh 时建提及索引（AC 多模式扫描） + `/api/mentions` + 契约登记；**绝不进图**（留待诊断层/引用索引排期）
- [x] renames 中间环清理（✅ v0.1.11：MergeRenames 链式折叠，只留链头→最终目标）
- [x] store 加 `PRAGMA wal_autocheckpoint=1000`（✅ v0.1.11）
