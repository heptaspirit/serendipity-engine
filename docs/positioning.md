# Serendipity Engine · 战略定位

> 日期：2026-08-23（由外部审计定位草稿定稿并汇入仓库）
> 来源：与用户讨论（2026-08-23）+ 生态调研（GraphRAG / PPR / agent 记忆 / LLM Wiki）
> 性质：**我们为什么做、做哪一层、不做哪一层**——战略层，回答"为什么"，不回答"怎么做"（后者见 [`docs/design.md`](design.md) 与组件架构）。
> 相关：后端积压 [`docs/backend-backlog.md`](backend-backlog.md) · 前端计划 [`docs/frontend.md`](frontend.md) · 总路线图 [`docs/roadmap.md`](roadmap.md) · 数据源立场依据 agent-memory-research（已归档）。

> **一句话定位：把用户笔记库变成 agent 的记忆——引擎是记忆的「激活层」。**

---

## 一、核心主张：两层记忆，引擎占第二层

| 层 | 做什么 | 谁在做 | 本引擎的角色 |
|---|---|---|---|
| **存储层**（数据） | 笔记本身是知识底座 | Obsidian / 虎鲸 / 笔记软件 | 只读接入，不接管 |
| **激活层**（关联） | 让库在正确时刻被唤起、给出「结构上下文」 | **本引擎（结构侧）** / dsh-mneme（向量侧） | **独占生态位** |

市面上的 agent 记忆（Mem0、官方 `server-memory` 等）几乎都在做第一层——存什么、怎么存。
本引擎做第二层：**怎么让已有的库活起来**。这一层目前生态很空。

## 二、与生态的差异化

| 方案 | 建图方式 | 可解释性 | 依赖 | 定位 |
|---|---|---|---|---|
| GraphRAG / LightRAG | LLM 提取实体 → 图 | 黑盒 | LLM + embedding | 文档 QA |
| HippoRAG / HippoRAG 2 | LLM 提取实体 → 图 + PPR | 部分 | LLM | 多跳 QA / 记忆 |
| Mem0 / OpenMemory | agent 会话记忆 | 部分 | LLM/embedding | agent 记忆存储 |
| dsh-mneme | 向量记忆 | 黑盒 | 本地模型 | agent 向量记忆 |
| **Serendipity Engine** | **用户笔记真实链接 → 图** | **白盒（激活路径/证据链）** | **纯 Go 零依赖** | **笔记库激活层 / agent 记忆结构侧** |

关键差异：**图是真实产生的，不是 LLM 编的**。白盒、零成本、可信——agent 记忆生态里的稀缺定位。

## 三、与 LLM Wiki（Karpathy 模式）的互补关系

> 背景：用户实测 LLM Wiki 数月后察觉其检索局限，由此得到本项目灵感。Karpathy gist（llm-wiki.md，2026-04）三层架构：raw sources（不可变）→ wiki（LLM 维护的 markdown 知识体）→ schema（CLAUDE.md/AGENTS.md 维护规则）。

**定位：LLM Wiki 是构建层，本引擎是激活/检索层——互补，不竞争。**

| LLM Wiki 环节 | 谁做 | 本引擎角色 |
|---|---|---|
| 构建层（LLM 写 wiki） | Karpathy 模式 / 用户在用 | 不碰——wiki 由 LLM 维护 |
| 检索层（index.md 线性扫描） | 原始方案，规模受限（~100 sources 后吃力） | **补充：结构激活漫游**（PPR 相关簇 + 激活路径 + 可解释） |
| agent 消费层 | Karpathy 推荐 qmd（BM25/vector + MCP server） | **已落地：MCP 八工具**（graph.stats/roam/random/relation/node/similar/community + seren.touch_digest） |

**关键差异：搜索 vs 漫游。**
- qmd 类工具解决"找到"（命中相关页面）；serendipity 解决"发现"（结构相关簇、随机漫步、关系证据链）。
- LLM Wiki 维护出有双链的 wiki 后，需要的正是激活层——**Karpathy 本人未解决这块**（他推荐的也只是搜索工具）。
- 共同哲学：两者都拒绝 embedding 基础设施（gist 明言 index.md 是为 avoid embedding RAG）——结构信号即可。

**生态空白（为什么是现在）**：几乎所有项目都在做**构建层**（wiki 编译器 / Obsidian 集成 / 记忆系统）；检索层只有 qmd（BM25+vector+MCP）——**但它做的是"搜索"，不是"激活"**。本引擎的生态位正是 **qmd 的互补者**：qmd 解决"找到"（命中页面），serendipity 解决"发现"（结构相关簇、随机漫步、关系证据链）。生态里缺的正是这块。

**合体价值**：LLM Wiki 生成/维护（持续积累）→ serendipity 激活/漫游（当下相关）→ MCP 给 agent 消费（已铺路）。这正是"笔记库 = agent 记忆"叙事的最强注脚。

## 四、纯本地算法定位（已定：不引入语义层）

- **引擎是纯本地算法核心，零 AI 依赖，不引入任何语义 / embedding 层**。所有能力（图构建 / PPR / 激活扩散 / similar / 潜在关联 / communities）均为确定性、可解释、永远在线、免费的结构算法。
- **先前讨论过的「Web 层语义注入口 / 可选语义旁路」不作为项目方向**——保持引擎与 Web UI 纯结构，避免任何隐藏语义黑箱。
- 效果：引擎输出始终可信、可解释，信任承诺稳固（见 [`docs/roadmap.md`](roadmap.md) 明确不做）。

## 五、运行架构（纯本地，无语义旁路）

```
Obsidian（宿主）
 ├── Obsidian vault（笔记 + 双链）          ← 数据层，无人拥有
 └── 插件薄壳（iframe 嵌 Web UI + 隐式 touch + digest 导出，不内置 AI）
        ▲
        │ 结构候选 API（只读）
        │
 seren serve（引擎：adapter → 图 → PPR / 激活 / roam / relation）
        ▲
        │ 只读 MCP 工具面
        │
 外部 AI / agent（独立消费者，经 MCP 消费引擎产出，不在引擎/插件内）
```

| 层 | 放什么 | 是否碰语义/AI |
|---|---|---|
| 数据层（vault） | 笔记 + 双链 | 不碰 |
| 结构引擎（seren serve） | 图构建、PPR、激活、roam/relation API | **不碰** |
| 插件薄壳 | iframe + 隐式 touch + digest 导出 | 不内置 AI |
| 外部 AI/agent | 经只读 MCP 消费引擎产出、在其上推理 | 在引擎之外 |

**关键设计：**
1. **引擎与插件均不感知语义**——只做结构算法；外部 AI/agent 作为独立消费者经只读 MCP 读取引擎产出，不在引擎/插件内部。
2. **融合点不存在于引擎**——引擎刻意只懂结构分；任何语义推理都在引擎之外的外部 AI 完成，引擎核心零改动。

## 六、数据源边界：真实性门槛 + adapter 唯一接入点（已定）

> **〔2026-08-23 补〕真实性门槛是本节的总纲**——seren 的一切价值建立在「图 = 真实链接」之上。如果图的数据源是 LLM 生成的记忆（二手蒸馏物、无事实锚点），白盒承诺、可信度、GIGO 三条全部崩塌。故 adapter 唯一接入点之上，再加一条硬门槛：**接入任何新数据源前，先回答两个问题——内容有没有「不可变的事实锚点」？有没有人工验证环节？两者皆无 → 官方默认拒绝接入。**

- **内核只认识 Document**（ID/Title/Aliases/Type/Refs/Tags/Text）——任何数据源都是"翻译成 Document 流"的 adapter。**形态不变**：每类数据源一个 adapter（Obsidian / 虎鲸 / OKF / …），引擎零改动。

### 6.1 事实锚点分类（数据源可否建图的标准）

正确的分界线不是「笔记软件 vs agent 记忆库」，而是内容有没有「不可变的事实锚点」：

| 数据源 | 事实锚点 | seren 态度 |
|---|---|---|
| Obsidian / 虎鲸笔记 | 人类手写（锚点 = 人脑） | ✅ **核心场景** |
| LLM Wiki（Karpathy 模式） | raw sources 是不可变事实材料 + **人力维护**（人工验证） | ⚠️ 可接受但默认谨慎（专门画像 `llm-wiki`，见 design §6.8） |
| OpenViking / Graphiti / A-MEM / Mem0 生成内容 | 无——纯会话蒸馏 / LLM 提取，无校验环节 | ❌ **明确不做** |
| 纯 LLM 生成 + 自动维护 | 无 | ❌ 明确不做 |

### 6.2 必须保留的区分：语义检索通道 ≠ LLM 生成数据源

| 形态 | 节点本身 | 是否允许 |
|---|---|---|
| **语义检索通道**（如第三方语义检索，节点是用户真实笔记） | 真实笔记 | ✅ 保留（第三方可选，不内置引擎、不经 Web 层融合） |
| **LLM 生成数据源**（如 OpenViking 记忆库，节点本身是 LLM 编的） | LLM 生成 | ❌ 不做 |

语义通道只是「检索线索」来源不同，节点仍是真实笔记——没有 GIGO 问题；LLM 生成数据源的节点本身不可信——无论怎么筛都是垃圾进垃圾出。**两者不要混淆。**

### 6.3 明确不做的清单（官方方向，不限制第三方）

1. **官方不做 OpenViking / Graphiti / A-MEM / Mem0 等 LLM 生成记忆库的 adapter**（建图数据源）——即使技术上可行（Document 抽象天然兼容），内容真实性不达标，且掏出来对人没有直接使用价值。
2. **官方不做 LLM 生成记忆的结构筛选 / 重排器**——从 LLM 折腾出的东西里再做筛选本身没有意义（GIGO），还可能因「结构化包装」让幻觉更可信。
3. **官方不把「增强筛选器」作为对外定位**（OpenViking find top-k → seren 重排的组合仅停留在讨论层面）。
4. **agent 记忆类 adapter 若真要做，先解决「边从哪来」**：笔记的边 = 用户写的双链（真实、显式）；agent 记忆的「关系」通常是隐式（共现、语义相似、会话时序）——需 adapter 自己定义 Refs 映射规则。且 Zep 等远程 API 踩「个人数据不出本机」红线，默认不做。

### 6.4 对外叙事与第三方边界

- **对外叙事**：被问「支不支持 X 记忆库」时，标准回答是「**我们官方刻意不做**——那是给幻觉加结构化包装，违背我们的信任承诺」；同时补一句「架构已预留，第三方开发者欢迎自己接」。这是护城河，不是缺陷；是开放，不是封闭。
- **第三方集成边界（重要）**：维护者不替任何 LLM 记忆库集成方向背书，也不承担其内容质量责任；第三方基于预留的 Document/adapter 做自己的集成完全欢迎——seren 只保证「结构引擎本身可信」，不为「接进来的数据是否可信」担保。**引擎的信任承诺是「对真实链接做白盒激活」，不是「对我们筛过的任何东西负责」。**
- **立场变更条件**：§6.3 的红线若需变更，必须由 owner（boyang）确认——任何 agent 无权单方面放宽真实性门槛。

## 七、对 AI agent 的价值（已铺路）

MCP 八工具（只读）即 agent 接口（见 [`docs/architecture/07-mcp.md`](architecture/07-mcp.md)）：
- `graph.stats` — 库概况
- `graph.roam` — 查询驱动漫游（激活簇 + 路径）
- `graph.random` — 发散入口（seed 可复现）
- `graph.relation` — 两实体关联强度 + 证据链
- `graph.node` — 单节点详情（确认"这是不是我要的"）
- `graph.similar` — 结构相似节点（共享邻居证据，Adamic-Adar）
- `graph.community` — 社区发现（Leiden，诊断层：定位主题簇/知识缺口）
- `seren.touch_digest` — 行为信号 digest（§3.7：点击聚合 TopN，识别主题升温）

定位句：**给 agent 一个「结构侧记忆导航」——比 RAG 便宜、比向量记忆可信。**

## 八、明确不做（坦诚声明）

- TS 移植 / WASM / 移动端
- GraphRAG 全家桶 / LLM 建图
- embedding 内置 / 在线 API
- graph 数据库（Neo4j/Kuzu）——数千节点规模是负资产
- SaaS / 远程 / 云模式（个人数据不出本机红线）
- **LLM 生成记忆库 adapter**（OpenViking / Graphiti / A-MEM / Mem0 建图数据源）——真实性门槛（§六）
- **LLM 生成记忆的结构筛选 / 重排器**——GIGO + 结构化包装幻觉（§6.3）

## 九、护城河

1. **克制即护城河**：纯 Go 零依赖、单二进制、白盒可解释——在云端 agent 记忆泛滥的时代是稀缺品
2. **白盒可解释**：引擎输出确定性、可解释、永远在线、免费，无隐藏语义黑箱，信任承诺稳固
3. **可复现**：`--seed` 让漫游可复现——对 agent 联调、测试、分享有实际价值
4. **双数据源 + 对账刷新 + 自动监听**：真实库上已验证（两库 74 单测绿）

## 十、后续动作

- [x] LLM Wiki adapter 画像（`llm-wiki` + `ExcludedFiles`）落地，见 [`docs/backend-backlog.md`](backend-backlog.md) §3.5
- [x] 诊断层（Leiden 社区发现 → 知识缺口诊断）排期时实现，见 [`docs/backend-backlog.md`](backend-backlog.md) §3.4
