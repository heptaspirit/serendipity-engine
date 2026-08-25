---
title: "Agent 记忆库研究：立场声明与借鉴清单"
summary: "seren 的立场与定位：维护者不做 LLM 生成记忆库的兼容（二手物对人无直接价值），但架构预留、欢迎第三方开发者集成；核心是平衡点——让双链对记笔记的人更有价值（库里漫游寻灵感），也让链接成为 agent 可消费的结构信号（免遍历、可评估、可推算知识缺口）。附三个研究对象的价值借鉴清单与验证结果。本文件为增补文档，供开发 agent 综合进项目文档时参考"
owner: boyang
status: draft
date: 2026-08-23
related:
  - serendipity-positioning.md
  - serendipity-frontend-roadmap.md
  - serendipity-opportunities.md
  - serendipity-promotion.md
  - serendipity-openviking-notes.md
note: 已归档（2026-08-24）——原为本地增补文档，现汇入 docs/history/ 作研究依据与选型溯源。立场结论并入 positioning.md §六/§八；借鉴清单并入 backend-backlog.md §三/§四/§七；LLM Wiki 画像并入 backend-backlog.md §3.5 + design.md §6.8。
---

# Agent 记忆库研究：立场声明与借鉴清单

> ⚠️ **历史记录（已归档）**：本文是 2026-08-23/24 agent 记忆库生态研究（OpenViking / Graphiti / A-MEM / GraphFlow）的**立场结论 + 借鉴清单**。硬约束（真实性门槛 / 明确不做 / 第三方集成边界）已并入 [`docs/positioning.md`](../positioning.md) §六/§八；借鉴清单（Graphiti 增量摄入 / A-MEM 演化方向 / GraphFlow 基准方法学 / Leiden 社区发现选型 / graphwizard 参考）已并入 [`docs/backend-backlog.md`](../backend-backlog.md) §三/§四/§七；LLM Wiki adapter 方案（VaultProfile `ExcludedFiles` + 内置画像 `llm-wiki`）已并入 [`docs/backend-backlog.md`](../backend-backlog.md) §3.5 与 [`docs/design.md`](../design.md) §6.8。保留本文作研究依据与选型溯源。

> 本文档是 serendipity-engine 对 agent 记忆库生态研究的**正式结论**。研究对象：
> ① **OpenViking**（字节，agent 上下文数据库，文件系统范式）
> ② **Graphiti / Zep**（Zep 的时序知识图谱引擎，~26.7k⭐）
> ③ **A-MEM**（NeurIPS 2025，Zettelkasten 式自组织记忆）
> ④ **GraphFlow**（Roarpeng，coding agent 的记忆与上下文引擎，纯 TS/Node 本地优先）
>
> 核心内容：① 我们对 LLM 生成记忆库的立场（明确不做什么）② 值得借鉴的设计点（已验证/远期可做）。
> 给开发 agent 的用途：综合进项目文档时，本文档是"边界依据"——防止未来误入 LLM 二手内容筛选的歧途；借鉴清单（§四）按各自优先级进入 roadmap 评估。

## 〇、如何使用本文档（先读这节）

| 部分 | 性质 | 对 agent 的含义 |
|---|---|---|
| §一~三（立场/红线） | **硬约束**（已定决策） | 综合进项目文档时必须遵守，不得写入与之冲突的规划 |
| §四（借鉴清单） | 软参考（按需评估） | 4.1 已验证=无需改动；4.2~4.4 远期可借鉴=进 roadmap 评估，不立即实现 |
| §五（不学） | 硬约束 + 理由 | 与 §三 同级，防重复发明/误入 |
| §六（落地指引） | **执行动作** | 每项标注了时机（综合时/M1/远期/触发式） |
| 附录 | 上下文对齐 | 术语不熟时查 A；想知道"为什么这么定"查 B；要复核来源查 C |

**一句话**：立场是死规矩（但只约束维护者自身，见 §三.6），借鉴是活参考，落地有明确时机。若 §六 与其他文档冲突，以本文件立场（§二/§三）为准。

## 一、整体态度：我们寻找的平衡点

**seren 的定位不是"拒绝一切 LLM 记忆库"，而是站在一个平衡点上——让双链笔记的链接同时服务两个消费者：记笔记的人，和消费这些链接的 agent。**

拆成三层说清楚：

**1. 维护者自己不做 agent 记忆库的兼容。** LLM 生成的记忆是二手蒸馏物（无事实锚点、充满幻觉风险），掏出来对人没有直接使用价值——所以维护者不把它建进图里。这是**维护者对引擎自身的定位选择**，不是技术上的"做不到"。

**2. 但如果别的开发者想用 seren 去做集成，我们欢迎——那是别人的事情。** 我们预留了相应的架构（Document 抽象 + adapter 唯一接入点），第三方想接任何数据源（包括 LLM 记忆库）做起来都很方便。维护者不做 ≠ 禁止别人做；我们只是不替这个方向背书，也不承担其内容质量责任。

**3. 我们真正在做的，是那个平衡点本身：**
- **对人**：让双链的链接对记笔记的用户更有价值——用户可以在自己的笔记库里漫游，借结构激活寻找灵感（而不是让笔记变成一摞只进不出的死文件）。
- **对 agent**：让这些链接本身变成 agent 可以直接消费的东西——agent 不用每次都自己闷头遍历整个知识库，而是消费 seren 的结构信号：相关簇、证据链、权重分布。
- **更进一步**：agent 甚至可以用 seren 去**推算当前知识库可能缺了什么**——通过图的统计信号（枢纽密度、孤立节点、低连接区域、权重分布），不用完全遍历就能定位知识缺口。这是"激活层"之外的第二种 agent 价值：**诊断层**。

**一句话**：seren 站在"人的灵感漫游"与"agent 的结构消费"之间——链接对两边都有用，而维护者的克制（不做 LLM 二手物）恰恰是让这两边都可信的前提。

## 二、核心立场：真实性门槛（最重要，请务必理解）

seren 的一切价值建立在"**图 = 真实链接**"之上：

- 白盒可解释：为什么推荐 A 与 B 相关 → 因为用户手写双链连接了它们
- 比 RAG 便宜：无 LLM 建图成本
- 比向量记忆可信：图是真实结构，不是向量相似度

如果图的数据源是 LLM 生成的记忆，上述三条全部崩塌：

| 崩塌点 | 表现 |
|---|---|
| 白盒承诺 | "为什么推荐"的答案是"因为 LLM 觉得相关"——不可解释 |
| 幻觉扩散 | 节点内容是 LLM 编的，seren 会把幻觉**包装成结构化证据**传播 |
| GIGO | seren 是激活层不是净化层——输入幻觉，激活出来还是幻觉，只是更好看 |

> **A-MEM 论文自己的局限声明就是这条立场的活证据**：它承认 LLM 判断错误会"错误传播、越连越牢"，"改错一次可能覆盖真实偏好"，因此记忆演化需要"证据意识"（保留原始事件、推断结论、更新时间、触发原因）和人工纠正。seren 拒绝 LLM 生成的链接，正是规避这个问题的正确决策——不是保守，是清醒。

### 2.1 事实锚点理论（数据源分类标准）

正确的分界线不是"笔记软件 vs agent 记忆库"，而是**内容有没有"不可变的事实锚点"**：

| 数据源 | 事实锚点 | seren 态度 |
|---|---|---|
| Obsidian / 虎鲸笔记 | 人类手写（锚点 = 人脑） | ✅ **核心场景** |
| LLM Wiki（Karpathy 模式） | raw sources 是不可变事实材料 + **人力维护**（人工验证） | ⚠️ 可接受，但默认谨慎（人力维护是前提） |
| OpenViking / Graphiti / A-MEM 生成内容 | 无——纯会话蒸馏/LLM 提取，无锚点 | ❌ **明确不做** |
| 纯 LLM 生成 + 自动维护 | 无 | ❌ 明确不做 |

关键区别：LLM Wiki 有 raw sources 这层"事实压舱石"（幻觉空间小）且需人力维护（人工验证）；OpenViking/Graphiti/A-MEM 是"输入 → LLM 提取"单线流水线，**无任何校验环节**（幻觉空间大）。"LLM 生成物有价值的前提是有人力维护"——没有这个前提，其价值与直接对话上下文无异，绕一圈加工毫无意义。

### 2.2 一个必须保留的区分：语义检索通道 ≠ LLM 生成数据源

| 形态 | 节点本身 | 是否允许 |
|---|---|---|
| **语义检索通道**（如 Smart Connections 注入候选，节点是用户真实笔记） | 真实笔记 | ✅ 保留（已有 Web 层注入口设计） |
| **LLM 生成数据源**（如 OpenViking 记忆库，节点本身是 LLM 编的） | LLM 生成 | ❌ 不做 |

语义通道只是"检索线索"的不同来源，节点仍是真实笔记——没有 GIGO 问题。而 LLM 生成数据源的节点本身不可信——无论怎么筛都是垃圾进垃圾出。**两者不要混淆。**

## 三、明确不做的清单（给 agent 的边界红线）

以下内容是**维护者对引擎自身的定位决策**——这些内容**不要**出现在项目规划、roadmap 或未来功能里（除非本文档立场变更）。**注意：这约束的是"seren 官方方向"，不限制第三方开发者基于本项目做自己的集成（见第 6 条）。**

1. **官方不做 OpenViking / Graphiti / A-MEM / Mem0 等 LLM 生成记忆库的 adapter**（建图数据源）——即使技术上可行（Document 抽象天然兼容），内容真实性不达标，且掏出来对人没有直接使用价值。
2. **官方不做 LLM 生成记忆的结构筛选 / 重排器**——从 LLM 折腾出的东西里再做筛选本身没有意义（GIGO），还可能因"结构化包装"让幻觉看起来更可信，反而更危险。
3. **官方不把"增强筛选器"作为对外定位**——OpenViking find top-k → seren 重排的组合，仅停留在讨论层面，不写入官方产品能力。
4. **adapter 接入点保留，但官方接入加真实性门槛**——未来任何新数据源接入前，先回答：内容有事实锚点吗？有人工验证环节吗？两者皆无 → 官方拒绝接入。
5. **对外叙事**：被问"支不支持 X 记忆库"时，标准回答是"**我们官方刻意不做**——那是给幻觉加结构化包装，违背我们的信任承诺"；同时补一句"架构已预留，第三方开发者欢迎自己接"。这是护城河，不是缺陷；是开放，不是封闭。
6. **第三方集成的边界（重要）**：维护者不替任何 LLM 记忆库集成方向背书，也不承担其内容质量责任；第三方基于预留架构（Document/adapter）做自己的集成完全欢迎——seren 只保证"结构引擎本身可信"，不为"接进来的数据是否可信"担保。**引擎的信任承诺是"对真实链接做白盒激活"，不是"对我们筛过的任何东西负责"。**

## 四、值得借鉴的部分（设计层面，放心用）

### 4.1 已验证：我们的设计踩对了主流（无需改动）

| OpenViking 原则 | seren 对应 | 结论 |
|---|---|---|
| 解析与语义分离（Parser 无 LLM） | adapter 与 graph 分离 | ✅ 天然符合 |
| 单一数据源（内容从存储读，索引只存引用） | store 中心化，graph 从 store 构建 | ✅ 同一原则 |
| mv 必须同步更新索引 URI | sync.MergeRenames（改名保持节点身份） | ✅ 早就在做，验证 renames 表必要性 |
| active_count（使用次数入索引元数据） | touch 表（埋点，事件流超集） | ✅ 强验证 touch 方向 |
| 分数传播 + 收敛检测 | PPR 幂迭代 + 激活扩散（λ衰减/θ剪枝/跳数配额） | ✅ 同构：图/树上分数传播家族 |
| 显式 Service 层 | graph/roam/store 传输无关，web/mcp 薄转发 | ✅ 隐式具备，无需新抽象 |

### 4.2 远期可借鉴（参考，非行动项，按需评估）

| # | 借鉴点 | 内容 | 对应 seren 场景 |
|---|---|---|---|
| 1 | 节点详情分级 L0/L1 | summary（截断）/ overview（摘要+导航）两级 | 前端 #3 节点详情 API（低成本，纯截断零依赖） |
| 2 | 多锚点漫游 | 多源 PPR（学术现成）替代 LLM 意图分析 | Web 手动多锚点 → 发散增强 |
| 3 | 确定性排序 | 稳定采样思想（>32 子项时确定性保序采样） | 锚点排序稳定（Resolve map 序），验证为普遍痛点 |
| 4 | 簇级导航 | 激活簇的结构化导航视图（按 hop 分组） | roam 结果可读性 |
| 5 | used() 的 success 字段 | 知道「点了但没深入」比「点了」有信号 | touch 统计 API 设计（远期） |
| 6 | 「源数据权威」哲学 | 索引可重建、源数据不可丢；**宁可搜不到，不要搜到坏结果** | 建议显式化为 design 原则，与白盒/克制并列 |

### 4.3 Graphiti（Zep 时序知识图谱引擎）——图结构机制的直接参考

Graphiti 是三个研究对象中**与 seren 的图最同构**的一个（节点=实体、边=事实、带时间戳）。它同样是 LLM 建图（不碰数据），但**图结构的组织机制**是纯设计参考，可放心借鉴：

| Graphiti 机制 | 细节 | 对 seren 的借鉴 |
|---|---|---|
| **边失效而非删除** | 新事实与旧事实矛盾时，旧边打 `invalid_at` 时间戳**保留**，不删除——可查询任意历史时间点的图状态 | **直接回应 renames 表的设计**：之前我们把 renames 中间环判定为"无限增长要清理"；Graphiti 给出了另一条路——"历史映射有价值，用失效标记而非删除"。两条路（清理 vs 标记失效）值得在 M1 增量写时重新权衡 |
| **双时态模型** | 每条边两个时间轴：`valid_at/invalid_at`（现实世界何时成立）+ `created_at/expired_at`（何时被摄入） | seren 已有 transaction time（MTime），缺"边生命周期"概念——若未来 touch 统计驱动边权演化，这是演化前的"边状态"模板 |
| **增量 episode 摄入** | 新数据只动它触及的实体/边，**不批量重算** | 正是 backlog 里"Store 增量写"（等 M1）的成熟实现参考——它解决了"增量 vs 全量对账"怎么做 |
| 三层图结构 | episodes（原始不可变）→ entities/facts（派生）→ communities（聚类摘要） | 与 OpenViking L0/L1/L2 同构——"渐进式加载"在 agent 记忆领域的第二次验证，前端 #3 节点详情分级更有底气 |
| 混合检索 | 语义 + BM25 + **图遍历（BFS）** | seren 已有"结构+全文"（roam 降级全文）；图遍历作为一等检索信号，验证 roam 设计 |
| 完整谱系（lineage） | 每个实体/边追溯到产生它的 episode | seren 的路径链（证据链）就是谱系展示——验证"可追溯"是图检索核心价值 |
| 确定性优先消解 | "deterministic matching with LLM fallbacks" | seren 的 Resolve 全确定性（更白盒）——验证确定性优先策略是主流 |

**Graphiti 明确不学的**：图数据库依赖（Neo4j/FalkorDB/Neptune）、LLM 实体提取/消解兜底、cross-encoder rerank、时态历史存储成本（保留全部失效事实 = 数据持续增长，seren 千级节点不需要）。

### 4.4 A-MEM（NeurIPS 2025，Zettelkasten 式自组织记忆）——演化方向的白盒对照

A-MEM 是笔记域研究对象中**哲学与 seren 最接近**的（Zettelkasten 卡片网络 + 笔记互联，GraphFlow 的接近是代码域另说，见 §4.6）。它的核心机制恰好和 seren roadmap 里的"touch 边权演化"形成**同题对照**：

| A-MEM 机制 | 细节 | 对 seren 的借鉴 |
|---|---|---|
| **Memory Evolution（记忆演化）** | 新记忆触发旧记忆修订（更新上下文/关键词/标签）——"记忆随新经验主动改写" | **验证 touch 边权演化方向正确**！同一问题的两个版本：A-MEM 用 LLM 判断"新信息是否更新旧记忆"，seren 计划用**用户点击数据**判断"哪些边值得强化"——seren 的方案更白盒（行为证据 vs LLM 猜测） |
| 链接类型化 | 链接有方向/类型（补充/修正/对比/时序） | seren 的 Refs 是无类型双链——远期可选扩展（需扩展双链语法，谨慎） |
| 相对检索 | Top-k 向量 + 沿链接扩展关联记忆 | seren 的 roam 激活簇就是"沿链接扩展"——同构验证 |
| 原子卡片 | 多属性记忆（内容+关键词+标签+上下文+链接） | seren 的 Document 多字段（Title/Aliases/Type/Tags/Text）——同构验证 |

**A-MEM 明确不学的**：LLM 笔记生成（关键词/标签/上下文全 LLM 写）、LLM 链接判断（embedding 邻居 + LLM 决策）、LLM 演化改写——黑盒 + 错误传播风险（论文自己承认"改错一次可能覆盖真实偏好"）。**这一条恰好是真实性门槛的活论据（见 §二）。**

### 4.5 四个研究对象的横向结论

| 研究对象 | 范式 | 可借鉴（结构机制） | 不可借鉴（LLM 数据管线） |
|---|---|---|---|
| OpenViking | 文件系统树 + 语义 | L0/L1/L2 分级、目录级 sidecar、分数传播 | 意图分析、rerank、记忆提取 |
| Graphiti | 时序知识图谱 | **边失效/双时态/增量摄入**、三层图、谱系 | LLM 提取、图数据库、rerank |
| A-MEM | Zettelkasten 卡片网络 | **记忆演化方向**、链接类型化、相对检索 | LLM 笔记生成、LLM 链接/演化判断 |
| GraphFlow | 代码图谱 + 上下文压缩 + 证据晋升 | **证据晋升门禁、黄金检索集基准、L0-L3 锚点契约**（详见 §4.6） | 学习飞轮自动捕获（LLM 二手物，虽带 commit 锚点）、AST 代码域 |

**共同指向**：四个系统都验证了 seren 的两个 backlog 方向——**Store 增量写**（Graphiti 增量摄入）和 **touch 边权演化**（A-MEM 记忆演化 / GraphFlow 证据晋升）——是走在正确轨道上的；同时它们全都依赖 LLM 判断（黑盒），而 seren 用真实链接 + 用户行为（白盒），这正是差异化的根基。**GraphFlow 是唯一把"证据锚定 + 晋升门禁"做成完整工程的产品——它证明白盒 + 证据这条路不仅能走通，还能自证（可复现基准）。**

### 4.6 GraphFlow（Roarpeng，coding agent 记忆引擎）——证据晋升门禁的工程化范本

> 研究日期：2026-08-24。定位：面向 **coding agent** 的「记忆与上下文引擎」——代码知识图谱（12 语言 AST）+ L0-L3 分层上下文压缩 + 学习飞轮。纯 TS/Node、完全离线、无 API key，MCP 暴露 10 工具（支持 15+ agent，含 DeepSeek Harness/dsh）。**域不同（代码 vs 笔记），不竞争不兼容——但对 seren 有 3 个直接可抄的机制。**

| GraphFlow 机制 | 细节 | 对 seren 的借鉴 |
|---|---|---|
| **证据晋升门禁**（team-memory-security.md） | 记忆当代码对待：外部技能一律**未证实**（no direct trust），必须本地证据积累（pass/fail episodes）才晋升 `proven`；四类生命周期（proven/correctable/anti-pattern/noise）；金丝雀验证（真实任务验证后才影响规划）；anti-pattern **隔离不删除**（留审计、可追踪 blast radius）。核心句："假设传输不可信，信任决策全部放在晋升边界，本地证据是唯一通行证" | **stance 真实性门槛（§二）的最完整工程化参照**。如果将来 touch 数据真演化边权（roadmap M1 远期），四类生命周期 + 金丝雀门禁是现成模板——正好解决 A-MEM 承认的"改错一次覆盖真实偏好"问题（§4.4） |
| **黄金检索集基准**（benchmark-standards.md） | 26 条 retrieval-golden 查询（10 条 **indirect**：golden 节点与查询词零 token 重叠，纯检索必失败）+ 36 条 HARD 集（cross-module/disambiguation/indirect）；A/B 对照（ON 100% vs OFF 56.5%，rescued/hurt 分析）；**commit 锚定**（结果 JSON 带 commit hash，跨 commit 必须锚定）；自测 vs 独立复现边界诚实声明 | **roadmap #8 契约测试的升级模板**：固定查询集 + 预期结果 + commit 锚定。尤其 **indirect 任务思想**——构造"查询词与目标节点无文本重叠、只有结构信号能命中"的用例，正好验证 seren 结构激活的独有价值（纯文本检索做不到的事） |
| **上下文契约 L0-L3 + refill**（context-contract.md） | L0 检索 → L1 锚点（File/Symbol，可扩展句柄）→ L2 模块概览 → L3 经验；**refill 模式**：先给锚点包（summary + anchors），按 `anchorId` 按需扩展单个锚点，不一次性倒全文；预览明确标注"preview ≠ 全文"（estimatedSavings ≠ 信息保真度） | **前端 #3 节点详情 API 的交互模型升级**：先给「摘要 + 邻居列表」锚点包，用户点某个邻居再 refill 扩展详情——比 OpenViking L0/L1/L2 多一个"按需扩展"维度；诚实标注预览与全文的边界与白盒哲学一致 |

**GraphFlow 验证了什么**：① "Storage → Reflection → Experience"三段论——**"Without reflection, storage is only a log"** 正是 seren touch 表现状（只埋不消费）的一语道破；② 证据文化（"用证据说话，而非承诺"）与 seren 白盒哲学同源；③ 本地优先 + 反纯 RAG + 图结构（PageRank 压缩）——与 seren 定位完全平行，说明"结构派 agent 记忆"在代码域已被验证可做成完整产品。

**明确不学的**：代码 AST 图谱（seren 不做代码域）；学习飞轮自动捕获（agent 经验自动写回 = LLM 二手物，与"官方不做 LLM 二手筛选"有张力——虽带 commit/diff/tests 锚点，可信度高于 OpenViking 但仍是二手）；TS/Node + npx 运行时（与 Go 零依赖冲突）；L3 经验注入（seren 不面向 prompt 注入）。

## 五、明确不学的（设计层面克制）

**跨研究对象共性不学**：
- LLM 意图分析 / rerank 模型 / LLM 实体提取——黑盒 + 外部依赖，踩两条红线
- LLM 笔记生成 / LLM 链接判断 / LLM 演化改写（A-MEM）——黑盒 + 错误传播风险
- 路径锁 / 崩溃恢复 / 多写存储事务全套——单进程 SQLite 天然免疫（架构红利，不是缺失）
- memory_diff 审计——动机是"LLM 自动改记忆要留痕"，seren 只读无此需求
- 图数据库依赖（Neo4j/FalkorDB）——数千节点规模是负资产
- 时态历史无限存储（Graphiti 保留全部失效事实）——seren 用"清理 + 标记"的克制版
- OKF sidecar schema / 异步语义队列 / 路径变量 / 多租户 / 加密——单用户本地无场景

## 六、给 agent 的落地指引（每项标时机）

| # | 动作 | 时机 |
|---|---|---|
| 1 | **positioning.md 数据源边界章节补真实性门槛**：adapter 唯一接入点不变，新增"无事实锚点、纯 LLM 生成且无人验证的数据源默认拒绝接入"条款；原"server-memory 最容易接入"判断加此前置条件 | **综合文档时（立即）** |
| 2 | **roadmap 不新增任何 LLM 记忆库兼容项**（OpenViking/Graphiti/A-MEM/Mem0 均不建图）；若出现相关 issue/需求，按本文档 §三 回答 | 持续（触发式） |
| 3 | **两个 backlog 方向获外部验证，优先级可提升**：Store 增量写 → 参考 Graphiti 增量 episode 摄入机制（§4.3）；touch 边权演化 → 参考 A-MEM 记忆演化方向，但保持"用户行为驱动"而非 LLM 判断（§4.4） | M1 落地时 |
| 4 | **借鉴点按优先级并入现有 roadmap**：节点详情分级（§4.2#1）可直接并入前端 #3 设计；多锚点漫游（§4.2#2）进机会文档评估；确定性排序（§4.2#3）与既有"锚点排序稳定"项合并 | 各 roadmap 迭代时 |
| 5 | **本文件与 openviking-notes.md 的关系**：notes 是研究过程记录（含技术可行性细节），本文件是立场结论——综合进项目文档时以本文件为准，notes 作为背景材料 | 综合文档时 |
| 6 | **LLM Wiki adapter 兼容**：VaultProfile 加 `ExcludedFiles` 字段 + 内置画像 `llm-wiki`（见 §七，方案已定）+ 可选结构发现器 | 综合文档时（顺手） |
| 7 | **roadmap #8 契约测试升级为「检索黄金集」**（GraphFlow §4.6 借鉴）：固定查询集 + 预期结果 + commit 锚定；构造 **indirect 用例**（查询词与目标节点零文本重叠、只有结构信号能命中）验证 seren 结构激活独有价值 | roadmap #8 排期时 |
| 8 | **前端 #3 节点详情采用「锚点包 + refill」交互**（GraphFlow §4.6 借鉴）：先给摘要 + 邻居列表，按需扩展单个锚点详情，预览与全文边界诚实标注 | 前端 #3 设计时 |

**执行顺序建议**：先做 #1（改 positioning）→ #5（确定文档关系）→ #4（借鉴点归位）→ #6（LLM Wiki 画像，顺手）→ #3（M1 时兑现）→ #2（长期红线）。

## 七、LLM Wiki 支持方案（adapter 层，已定 A 方案）

> 背景：LLM Wiki（Karpathy 模式）是真实性门槛下**唯一"可接受但默认谨慎"的 LLM 生成数据源**（§2.1）——它有 raw 事实锚点 + 人力维护。因此对 Obsidian 里做了 LLM Wiki 的用户，adapter 值得做专门兼容。

### 7.1 为什么可以做（边界确认）

| 条件 | LLM Wiki | 一般 LLM 记忆库 |
|---|---|---|
| 事实锚点 | ✅ raw/ 不可变原始素材 | ❌ 无 |
| 人工验证 | ⚠️ 部分（用户浏览 + lint + audit 反馈管道） | ❌ 纯自动 |
| 数据形态 | ✅ 就是 Obsidian vault 里的 markdown + 双链 | ⚠️ 独立系统 |
| 结论 | ✅ adapter 兼容 | ❌ 不兼容 |

**关键区分**：我们兼容的不是"LLM 生成内容"本身，而是"**用户在自己 vault 里维护的 LLM 辅助知识库**"——raw 是用户收集的（人类输入）、wiki 是 LLM 编译但用户持续浏览/纠正的（有人工环节）、双链是真实存在的。它本质上更接近"人力维护的 wiki"，只是编写助手是 LLM。

### 7.2 目标结构（LLM Wiki 常见约定，调研 2026-08-23）

```
vault/
├── CLAUDE.md / AGENTS.md      # schema 层：LLM 行为规范（非知识，排除）
├── raw/                       # 原始素材（人类写入，LLM 只读，不可变）→ 整体排除
│   ├── articles/ papers/ notes/ ...（含其中 .md，也不扫）
├── wiki/                      # LLM 编译产物（LLM 读写，用户浏览）→ 保留
│   ├── index.md               # 全局索引/目录（导航，排除）
│   ├── log.md                 # 操作日志（排除）
│   ├── entities/              # 实体页（进图）
│   ├── concepts/              # 概念页（进图）
│   ├── summaries/             # 素材摘要页（进图）
│   └── comparisons/ synthesis/ ...（进图）
├── audit/                     # 人工反馈收件箱（排除）
└── output/ / outputs/         # 成品输出（排除）
```

### 7.2a 决策：raw/ 整体不扫（含其中的 .md）

> 有人用 LLM Wiki 很"暴力"——raw 里图片、PDF、附件什么都有。**seren 不扫 raw/ 整个目录，也不尝试解析其中的非 md 文件。** 理由：

| 层面 | 理由 |
|---|---|
| 技术边界 | adapter 只认 `.md`，PDF/图片天然进不来；但即使 raw 里有 md，也是"素材"不是"知识体" |
| 零依赖红线 | 解析 PDF/图片需要 OCR / VLM / 文档解析——与"引擎不碰 embedding"同一逻辑，为扫素材堆引入整套解析能力是本末倒置 |
| 图价值 | LLM Wiki 哲学 = raw 是输入、wiki 是产物；用户漫游在 wiki 不在 raw。raw 是"配料间"不是"餐厅"，扫进来全是低价值节点 |
| 白盒质量 | raw 内容不可控（扫描件/转码乱稿/外部文档），进图污染"图=真实链接"的信任，且最占空间 |

**推论**：这也意味着 seren 对 LLM Wiki 的支持**零新增解析能力**——只认 wiki/ 里的 markdown，raw 整个跳过。魔改版用户若把个人笔记混进 raw，自行调整 excluded_dirs（§7.4）。

**注意**：用户实测的 LLM Wiki 可能是"魔改版"（目录名/层级不同），所以规则要**按约定支持 + 画像可调**，不能硬编码。

### 7.3 实现方案（A 方案，已定）

**改动 1：VaultProfile 新增 `ExcludedFiles []string` 字段（文件名级排除）**
- 现有 `ExcludedDirs` 只管目录（SkipDir）；LLM Wiki 的 index.md/log.md 是**文件级**，需要新字段
- 实现：`ParseVault` / `ParseVaultIncremental` 的 WalkDir 里加一个文件名判断（各 3 行）
- 收益不止 LLM Wiki：任何想排除特定文件名的库都能用

**改动 2：新增内置画像 `llm-wiki`**
```yaml
name: llm-wiki
excluded_dirs: [raw, audit, output, outputs]   # LLM Wiki 约定目录
excluded_files: [index.md, log.md, CLAUDE.md, AGENTS.md]  # 导航/日志/schema 文件
# 其余字段继承 default-obsidian（title/alias/tag 键等）
```
- 用法：`--profile-name llm-wiki`，开箱即用
- **wiki/ 内的实体/概念/摘要页保留进图**（它们才是知识价值所在）

**改动 3（可选加分项）：LLM Wiki 结构发现器**
- 启动/索引时检测 vault 结构特征（存在 `raw/` + `wiki/` + `wiki/index.md` 组合）→ 日志提示"检测到 LLM Wiki 结构，可用 `--profile-name llm-wiki`"
- 只提示不自动启用（白盒原则，用户明确选择）
- 低成本：一次目录探测

### 7.4 边界与权衡（诚实声明）

- **wiki/ 页面是 LLM 写的**——进图意味着接受"二手编译内容"。这与"图=真实链接"不冲突：链接是真实存在的双链，只是内容是 LLM 写的。白盒解释仍然成立（"为什么相关"→ 因为有链接），但"内容可信度"降级——这正是 §2.1 里"⚠️ 可接受但默认谨慎"的含义。
- **魔改版结构**：发现器+固定画像只覆盖约定结构；魔改版用户需自行在 profile.yaml 调 excluded_dirs/files（文档写明示例）。
- **不默认启用**：llm-wiki 是显式画像，不是默认行为——保护普通 Obsidian 用户（他们的 index.md 可能是正经正文/MOC，见 §7.5）。

### 7.5 为什么不默认排除 index.md（关键权衡，勿改）

Obsidian 里大量用户用 `index.md` 做 MOC（内容地图）——那是用户手写的枢纽页，是图的高价值节点。**文件名相同，无法区分手写 vs LLM 生成**。因此 index.md 排除**必须**通过显式画像（llm-wiki）启用，绝不进默认画像。这是"为 LLM Wiki 用户开一扇门，但不为所有用户关一扇窗"的克制。

## 八、一句话总结

**seren 站在平衡点上：双链链接既让用户在自己的笔记库里漫游寻灵感，也让 agent 免于闷头遍历——直接消费结构信号、评估权重、推算知识缺口。维护者官方不做 LLM 记忆库兼容（二手物对人无价值），但架构预留、欢迎第三方开发者自己做集成——克制是护城河，开放是格局，两者不冲突。**

---

## 附录 A：术语速查（给不熟悉讨论语境的 agent）

| 术语 | 含义 |
|---|---|
| seren | 本引擎（serendipity-engine）的 CLI 命令名，全文以"seren"指代引擎 |
| 白盒 | 每条推荐可解释——能回答"为什么推荐 A 与 B 相关"（因为用户手写双链连接了它们） |
| 事实锚点 | 判断数据源可否建图的标准：内容是否有不可变、可验证的事实基础（人类手写/raw sources/人工审核） |
| GIGO | Garbage In Garbage Out——输入是幻觉，激活出来还是幻觉 |
| Document 抽象 | 引擎内核唯一的格式抽象（ID/Title/Aliases/Type/Path/Refs/Tags/Text），每个数据源一个 adapter 翻译成它 |
| adapter | 数据源接入点（设计 §6.8）：Obsidian/虎鲸各一个，负责"格式翻译"；内核只认识 Document |
| touch 表 | store 里的埋点表（5000 条上限）：记录用户点击/漫游行为，目前只记录不消费 |
| roam | 查询锚定漫游（PPR + 激活扩散）——引擎核心检索 API |
| renames 表 | store 里的改名映射表：sync.MergeRenames 维护，用于链式改名解析；曾判定"中间环无限增长"待清理 |
| Resolve | 锚点解析（别名/标题匹配）——目前全确定性（无 LLM） |
| Store 增量写 | 机会文档 backlog：当前 refresh 全量 DELETE+INSERT，计划改增量（等 M1） |
| 前端 #3 | 前端路线图 P0 项：节点详情预览 API（/api/node?id=）——唯一动引擎的前端项 |

## 附录 B：决策溯源（为什么是现在的立场）

| 时间线 | 曾考虑的方案 | 结论与原因 |
|---|---|---|
| 早期讨论 | 把 seren 用作 OpenViking 的"增强筛选器"（find top-k → seren 重排） | ❌ **官方放弃**——数据是 LLM 生成的二手物，从里面筛是垃圾进垃圾出，且"结构化包装幻觉"更危险；但对第三方开放（§三.6） |
| 早期讨论 | 未来兼容 agent 记忆库（Mem0/server-memory/PLUR）走 adapter | ⚠️ **官方降级为"真实性门槛"**——adapter 接入点保留，但官方不接无事实锚点的数据源；第三方不受限 |
| 本文件 | OpenViking 兼容性技术评估 | ✅ 技术上可行（本地目录+markdown），但**内容真实性不达标 → 官方不做**，仅留技术笔记 |
| 扩展研究 | Graphiti / A-MEM 架构借鉴 | ✅ 借鉴结构机制（边失效/双时态/增量摄入/演化方向），**不碰 LLM 数据管线** |

**立场变更条件**：本文件 §三 红线若需变更，必须由 owner（boyang）确认——任何 agent 无权单方面放宽真实性门槛。

## 附录 C：研究来源（可复核）

| 对象 | 来源 | 抓取日期 |
|---|---|---|
| OpenViking | 官方 docs（docs.openviking.ai/zh/concepts/01~09）+ GitHub（volcengine/OpenViking） | 2026-08-23 |
| Graphiti | GitHub README（getzep/graphiti）+ 相关评测文章（Zep 论文 arXiv:2501.13956） | 2026-08-23 |
| A-MEM | NeurIPS 2025 论文（arXiv:2502.12110）+ 两篇独立解读 | 2026-08-23 |
| GraphFlow | GitHub（Roarpeng/GraphFlow）docs/：context-contract.md / benchmark-standards.md / experience-memory.md / team-memory-security.md | 2026-08-24 |

> 研究过程记录（含各文档的技术细节摘录）见 `serendipity-openviking-notes.md`。

## 附录 D：图数据库与图论算法评估（2026-08-23）

### D.1 图数据库（Neo4j / FalkorDB）为什么不需要

图数据库的"神奇功能"本质是一件事的三个面：**把"关系"从算出来的变成存好的**（关系是一等公民）。但 seren **已经用代码实现了图数据库的核心能力**，只是不叫那个名字：

| 图数据库给的 | seren 的对应物 |
|---|---|
| 深度遍历 | BFS 激活扩散（λ衰减 + 跳数配额） |
| PageRank | 查询锚定 PPR 幂迭代 |
| 节点/边存储 | SQLite 存快照 + 内存邻接表 |
| Cypher 查询 | roam/relation API（MCP 四件套） |

**规模实测（用户提供）**：seren 已跑过 **2 万节点**的场景（关系稀疏，多为节点）——运行无碍。这抬高了规模信号上限：SQLite + 内存图在 2 万节点、稀疏图下依然够用，换图数据库的触发线进一步推后。

### D.2 图论算法盘点：已在用 vs 真增量

| 算法 | seren 现状 | 评估 |
|---|---|---|
| PageRank / PPR | ✅ 查询锚定 PPR 已在用 | 无需动 |
| 连通分量 | ✅ `g.Stats()` 并查集已在用（喂给死路/孤立检测） | 无需动 |
| 度中心性 | ✅ `g.Stats()` TopHubs 已在用 | 无需动 |
| BFS 遍历 | ✅ 激活扩散已在用 | 无需动 |
| **社区发现（Leiden，替代 Louvain）** | ❌ 未实现 | 📌 **真增量**——域级知识缺口诊断 + 簇级导航（算法与 Go 实现已选型，见 D.4） |
| **介数中心性** | ❌ 未实现 | 📌 可选——桥接节点检测（诊断层信号） |
| 最短路径 / 紧密度 / SCC | ❌ 未实现 | ❌ 不引入（hop 路径已覆盖，无场景） |

### D.3 原则：算法等场景

社区发现/介数中心性的价值要落到具体功能（知识缺口诊断 API / 结构导航视图）才有意义——**不提前做，等"诊断层"功能排期时顺带实现**。先拿已有连通分量做粗糙版（哪些区域互不相连），觉得不够再上 Leiden（2026-08-24 已完成算法与 Go 实现的选型，见 D.4——落地时直接按 D.4 执行，无需重新调研）。这符合"算法等场景，场景等需求"的克制哲学。

### D.4 社区发现选型落地（2026-08-24，已定）

> 昨天把 Louvain 记为"真增量但等场景"；今天研究确认**算法用 Leiden（Louvain 的官方改进版），Go 实现用 `github.com/vsuryav/leiden-go`**。本节是落地依据，含选型理由、接入方式与合规要求。

#### D.4.1 为什么是 Leiden 而不是 Louvain

Leiden（Traag et al., *Scientific Reports* 2019）是 Louvain（Blondel et al., 2008）作者同源的改进版——作者 Traag 本身就是 Louvain 生态核心维护者，论文题目即 *"From Louvain to Leiden: guaranteeing well-connected communities"*。改进点是**在"节点移动 → 聚合"之间插入细化阶段（refinement）**：

| 维度 | Louvain | Leiden |
|---|---|---|
| 社区连通性 | 不保证，桥接节点移走后社区可能内部断裂 | **保证 well-connected**（数学保证） |
| 模块度分辨率极限 | 严重（小社区被吞并） | 支持 CPM / 分辨率参数 γ 缓解 |
| 速度 | O(n log n) 近似 | **通常更快**（细化后聚合图更小） |
| 结论 | ❌ 弃用 | ✅ 选用 |

#### D.4.2 Go 实现选型：leiden-go（对照 graphwizard）

Go 生态两个候选（均已核对源码/仓库/许可证）：

| 维度 | **vsuryav/leiden-go** ✅ 选用 | intelligrit/graphwizard（community 子包） |
|---|---|---|
| 许可证 | MIT（已确认） | MIT |
| 节点 ID | **string 直通**（输入输出均 string） | int64 → 需双向映射层 |
| 图输入 | `map[string]map[string]float64`（与 seren `adj map[string][]string` 一层循环转换） | gonum `graph.Undirected` 接口 → 需适配层 |
| 依赖增量 | **零**（纯 Go） | 引入整棵 gonum/graph 依赖树 |
| 维护形态 | 小仓库 3 文件 + 14 测试 + benchmark + karate club 验证（2025-11，无正式 tag） | 18 子包 40+ 算法全家桶（大而全，Leiden 只是其一） |
| 输出 | 自带 **Modularity 质量分** | 仅社区映射 |

**选用理由（三条硬理由）**：① seren 节点 ID 是 string，leiden-go 输入输出全 string，天然直通；② 零依赖新增，保住 go.mod 只 2 个直接依赖的克制；③ 输入格式与 seren 邻接表同构（无向、无权、去重、无自环——正好满足 leiden-go "边对称/权重正/自环不推荐"）。graphwizard 的价值建立在"gonum 当图底座"之上，而 seren 的底座是自研的——为拿一个 Leiden 引入全家桶不符合项目哲学。

**〔2026-08-24 更新〕gonum 已加入 Leiden，但选型不变**：复查发现 gonum/graph 在 2026 年新增了 `community.Leiden`（Traag 2019，含 refinement 阶段）——当初"gonum 只有 Louvain"的认知已过时。**仍选 leiden-go，理由不变**：string ID 直通 vs gonum int64 适配层、零依赖 vs 整棵 gonum 依赖树、leiden-go 自带 Modularity 输出。gonum 有 Leiden 只说明"社区检测在主流库不缺位"，不构成换的理由。

#### D.4.3 graphwizard 其余算法的处置（诚实评估）

逐包核对后，**大部分与 seren 手写实现重复，不引入**：

| graphwizard 能力 | 处置 |
|---|---|
| centrality（PPR/PageRank/Betweenness…） | ❌ PPR 已手写（且 gonum 的 PageRank 不支持个性化——只有标准版；graphwizard 的 PersonalizedPageRank 也仅单 seed，seren 是多锚点定制管线） |
| connectivity（并查集/WCC/桥） | ❌ 并查集已有（`Stats()`） |
| paths / traverse | ❌ 无权图 BFS 已是最优，已有 |
| LouvainQ（模块度评估） | ⚠️ 有用但 leiden-go 的 `result.Modularity` 已自带 |
| similarity（Adamic-Adar/链接预测） | ⚠️ 未来"你可能想关联的笔记"有价值，手写 20 行内 |
| structure（聚类系数） | ⚠️ 衡量笔记簇紧密度有意义，手写 ~15 行 |
| embedding（Node2Vec/DeepWalk） | ⚠️ 远期笔记向量化再评估（现代做法是调 embedding API），不为它引入 gonum |

**原则延续**：需要什么手写什么（seren 一贯风格），不背 gonum。

**〔2026-08-24 边界澄清：bbolt 接受 ≠ gonum 接受〕**：同日决定存储层从 SQLite 换 bbolt（backend-backlog §二.1，MIT，etcd 维护）。bbolt 被接受**不构成** gonum 也接受的依据，二者性质不同：
- **bbolt 是存储引擎**（基础设施层）——seren 的 store 语义（全量快照/有界事件流/映射）本来就是 KV 语义，bbolt 是"更贴合的底座"，换来收益明确（编译/体积/纯度）且签名透明
- **gonum 是算法框架层**（业务逻辑层）——它的价值必须通过"塞进 gonum 的 graph.Graph 接口（int64 ID）"兑现，对 seren 意味着适配层成本；而 seren 的核心算法（查询锚定 PPR + 激活扩散）是定制的、gonum 无对应物；gonum 能给的（Leiden）已被 leiden-go 覆盖；剩下的诊断层小算法手写 15-100 行白盒可控
- **"成熟算法优先"原则（用户提出，正确）的落法**：**选择性地用成熟组件**（存储用 bbolt、社区发现用 leiden-go），而不是**引入算法框架依赖树**（gonum）。边界 = 组件是否"即插即用、签名直通"；需要适配层/全家桶的就是负债。graphwizard/gonum 保留为"正确性参考"（附录 E 对拍验证）

#### D.4.4 接入方式与合规（执行方案）

**引入方式（二选一）**：
1. **直接依赖 + vendor（推荐，最省事）**：`go get github.com/vsuryav/leiden-go`（伪版本天然锁定 commit，go.sum 记账）→ `go mod vendor` 把代码锁进仓库，离线可构建、上游变动免疫。
2. **fork + replace（需要改上游代码时）**：fork 到自有账号/本地，`replace github.com/vsuryav/leiden-go => <fork 路径>`；fork 仓库保留原 LICENSE。

**合规要求（MIT 很宽松，回答"要不要声明"）**：
- ✅ 硬性要求只有一条：**保留原版权声明**——vendor 时 Go 自动在 `vendor/modules.txt` 记录许可证并保留 LICENSE 文件；fork 时保留 LICENSE 即可。
- ✅ 无 copyleft（不像 GPL 有传染性），不需要额外开源自己的代码。
- 📌 良好实践（非强制）：在 README / 本文档标注"社区检测使用 `github.com/vsuryav/leiden-go`（MIT）"一行 attribution。

**适配层草图**（落地点：`internal/graph/community.go`）：

```go
package graph

import "github.com/vsuryav/leiden-go"

// Communities 用 Leiden 做无向无权社区检测（v1 边权全 1.0）。
// 孤立节点（度=0）不成社区（2026-08-24 拍板：社区相似性需要结构信号，
// 孤立节点社区无区分度，且其诊断信号由 Stats().Orphans 承接）。
func (g *Graph) Communities(resolution float64, seed int64) (map[string]int, float64, error) {
    wadj := make(map[string]map[string]float64, len(g.adj))
    for id, nbs := range g.adj {
        if len(nbs) == 0 { continue } // 过滤孤立节点
        m := make(map[string]float64, len(nbs))
        for _, nb := range nbs { m[nb] = 1.0 }
        wadj[id] = m
    }
    res, err := leiden.Leiden(leiden.NewGraph(wadj), &leiden.Config{
        Resolution: resolution, RandomSeed: seed,
    })
    if err != nil { return nil, 0, err }
    // res.Partition.Communities() → map[commID][]string；res.Modularity 自带质量分
    // 返回 nodeID → commID
    return nil, res.Modularity, nil // 占位，落地时补全
}
```

**两个落地注意点**：
- ✅ **孤立节点（度=0）在社区检测前过滤（已拍板 2026-08-24）**——不把孤立节点当成社区：① 社区相似性计算需要社区间结构信号（inter-community 边 / PPR 激活 / 邻居集合），孤立节点社区与任何社区边数恒 0、激活传播不出去、Jaccard 无定义——三条路全失效，只剩 LLM 语义分析（踩零依赖 + 白盒红线）；② 孤立节点的诊断信号已由 `Stats().Orphans` 承接，过滤不丢信息（职责分离：Orphans 统计管孤立，社区检测只管有内部结构的簇）。
- 📌 未来 MCP 暴露可顺势加 `graph.community` 工具，与 roam/random/relation/stats 四件套并列（时机：诊断层功能排期时，遵循 D.3 "算法等场景"）。

## 附录 E：graphwizard 学习参考（2026-08-24）

> 第三方图算法库 **不引入为依赖**，作为"算法目录 + 正确性参考"按需取用（seren 底座自研，gonum 依赖树与轻量哲学冲突）。学习方式与 leiden-go 同款：借鉴思想，不吸收代码。

### E.1 地址信息

| 项 | 值 |
|---|---|
| GitHub 仓库 | `github.com/intelligrit/graphwizard` |
| 包文档 | `pkg.go.dev/github.com/intelligrit/graphwizard` |
| 许可证 | MIT（已确认） |
| 形态 | 18 子包、40+ 算法（community/centrality/similarity/structure/connectivity/embedding/flow/paths/…），cleanroom 实现 + gonum 封装 |
| 依赖 | gonum/graph + bbolt（这正是不引入的原因） |

### E.2 值得学习清单（按 seren 场景排序）

| # | 算法（包） | seren 场景 | 价值 | 手写成本 |
|---|---|---|---|---|
| 1 | **Adamic-Adar / 链接预测**（similarity） | opportunities 的 similar 功能缺口（结构相似节点 = embedding 白盒替代） | 共同邻居度加权 `Σ 1/log(deg)` 的正确实现细节 | ~20 行 |
| 2 | **聚类系数**（structure） | 诊断层"簇紧密度"信号 | 高聚类 = 主题簇核心，低聚类 = 结构洞；基于已有度 + 邻居交集，纯白盒 | ~15 行 |
| 3 | **K-Core 分解**（connectivity） | 知识库"核心 vs 边缘主题"诊断 | 剥洋葱找度≥k 的鲁棒核心子图，比孤立计数更强的缺口信号 | ~20 行 |
| 4 | **Betweenness 介数**（centrality） | 桥接节点检测（D.2 已标可选真增量） | Brandes O(nm)，seren 千级~2 万节点直接跑得起 | 中等 |

### E.3 测试方法论（最值得偷的不是代码，是这套）

- 社区检测落地的标准验证图：**karate club**（Zachary 空手道俱乐部，34 节点、2 个已知社区）与 **LFR 基准图**（带已知社区结构的合成图）——leiden-go 自己就用 karate club 验证。
- **`graph.community` 落地时，测试用标准图而非只靠自建 fixture**，社区质量才有客观对照。

### E.4 使用方式与引用声明

- **使用方式**：不引入依赖；按需手写（E.2 清单）；graphwizard 当"正确性参考"——手写实现与它的输出对拍验证。
- **README 引用声明建议**（非强制，良好实践；综合进项目文档时顺手加）：
  > 图算法学习参考：`github.com/intelligrit/graphwizard`（MIT）——社区检测/相似度/结构分析算法的正确性参考，实际实现为自研。
