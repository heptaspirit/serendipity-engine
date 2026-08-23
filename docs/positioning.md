# Serendipity Engine · 战略定位

> 日期：2026-08-23（由外部审计定位草稿定稿并汇入仓库）
> 来源：与用户讨论（2026-08-23）+ 生态调研（GraphRAG / PPR / agent 记忆 / LLM Wiki）
> 性质：**我们为什么做、做哪一层、不做哪一层**——战略层，回答"为什么"，不回答"怎么做"（后者见 [`docs/design.md`](design.md) 与组件架构）。
> 相关：后端积压 [`docs/backend-backlog.md`](backend-backlog.md) · 前端计划 [`docs/frontend.md`](frontend.md) · 总路线图 [`docs/roadmap.md`](roadmap.md) · 推广策略见本地非库文件（不追踪）。

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
| agent 消费层 | Karpathy 推荐 qmd（BM25/vector + MCP server） | **已落地：MCP 四件套**（graph.roam/random/relation/stats） |

**关键差异：搜索 vs 漫游。**
- qmd 类工具解决"找到"（命中相关页面）；serendipity 解决"发现"（结构相关簇、随机漫步、关系证据链）。
- LLM Wiki 维护出有双链的 wiki 后，需要的正是激活层——**Karpathy 本人未解决这块**（他推荐的也只是搜索工具）。
- 共同哲学：两者都拒绝 embedding 基础设施（gist 明言 index.md 是为 avoid embedding RAG）——结构信号即可。

**生态时点（为什么现在有机会）**：LLM Wiki 模式（2026-04）发布后一周内 7+ 项目、3000+ star；qmd（Shopify CEO）已 ~19700 star——**赛道极热，需求被验证**。生态现状：几乎所有项目做**构建层**（wiki 编译器 / Obsidian 集成 / 记忆系统）；检索层只有 qmd（BM25+vector+MCP）——**但它做的是"搜索"，不是"激活"**。本引擎的生态位正是 **qmd 的互补者**：qmd 解决"找到"（命中页面），serendipity 解决"发现"（结构相关簇、随机漫步、关系证据链）。生态里缺的正是这块。

**合体价值**：LLM Wiki 生成/维护（持续积累）→ serendipity 激活/漫游（当下相关）→ MCP 给 agent 消费（已铺路）。这正是"笔记库 = agent 记忆"叙事的最强注脚。

## 四、embedding 边界决策（已定）

- **引擎不碰 embedding**：保持结构×激活、零依赖、白盒可解释。
- **口子留在 Web 层**（不是插件）：语义候选**注入 Web API / 前端融合**；引擎核心（graph/roam/score）不知道 embedding 存在。
- **插件只是 Web 的壳**：Obsidian 插件 = iframe 嵌引擎 Web UI（M2 方案，见 [`docs/roadmap.md`](roadmap.md)），语义融合逻辑全部在 Web 前端，插件不感知。
- 效果：引擎 = 结构 + 语义双通道的融合点；引擎核心保持纯净。
- 顺带收益：任何想要语义的插件/工具都要经过引擎 Web 层融合 → 引擎成为「结构侧守门人」。

## 五、运行架构：embedding 在哪一层（已定）

> Obsidian 场景示意（语义服务为可选旁路，虚线）：

```
Obsidian（宿主）
 ├── Obsidian vault（笔记 + 双链）          ← 数据层，无人拥有
 └── 插件（iframe 壳，仅嵌 Web UI）         ← 消费端，不感知语义
        ▲            ▲
        │结构候选API   │语义候选API（可选注入）
        │            │
 seren serve ────── 语义服务·可选旁路（Smart Connections + Ollama）
 （引擎：adapter→图→PPR） （向量索引 → 语义候选 top-k）
```

| 层 | 放什么 | 是否碰 embedding |
|---|---|---|
| 数据层（vault） | 笔记 + 双链 | 不碰——语义服务直接读文件 |
| 结构引擎（seren serve） | 图构建、PPR、激活、roam/relation API | **不碰** |
| 语义旁路（可选） | Smart Connections / Ollama / 向量索引 | 独立进程或独立插件，与引擎平级 |
| 融合点（Web 前端） | 合并两条候选流、排序、展示 | 唯一知道两条流的地方 |

**两个关键设计：**
1. **引擎与语义服务是两个平行的消费者**——都从 vault 读数据，互不依赖；语义服务挂掉，引擎照常，插件降级为纯结构而非不可用。
2. **融合点放 Web 前端**——融合逻辑必须"同时懂结构分和语义分"，引擎刻意不懂语义；Web 层已存在（index.html），未来换语义服务只动前端/API，不动引擎核心。

**给开发留的口子（M2 时落实，见 [`docs/roadmap.md`](roadmap.md)）：**
- Web API 侧：`/api/roam` 等接受**外部候选注入**（`extra_candidates: [{id, score}]`），并入现有 score 融合管线（可选参数，默认关闭，不破坏契约）。
- 前端侧：融合视图预留「语义候选」数据槽位，与结构候选并行渲染。
- 引擎核心：零改动。

## 六、数据源边界：adapter 是唯一接入点（已定）

- **内核只认识 Document**（ID/Title/Aliases/Type/Refs/Tags/Text）——任何数据源都是"翻译成 Document 流"的 adapter，包括未来可能的 **agent 会话记忆类工具**（Mem0 / OpenMemory / 官方 server-memory / PLUR 等）。
- **形态不变**：每类数据源一个 adapter（Obsidian / 虎鲸 / OKF / agent 记忆…），引擎零改动。
- **agent 记忆类 adapter 的关键差异（不是白拿的）**：
  1. **边从哪来是最大问题**：笔记的边 = 用户写的双链（真实、显式）；agent 记忆的"关系"通常是隐式的（共现、语义相似、会话时序）——**需要 adapter 自己定义 Refs 映射规则**，否则建不出图。
  2. **数据可达性**：Mem0/OpenMemory 有本地 SQLite（可仿虎鲸快照直读）；Zep 等是远程 API——**踩"个人数据不出本机"红线，默认不做**，除非用户显式要求。
  3. **官方 server-memory 是 KG 型**（实体+关系+观察），天然可映射 Document（实体→节点、关系→边）——是最容易接入的一类。
- **边界声明**：agent 记忆适配是**远期可选项**，与 embedding 同级——先服务好笔记库（真实链接），agent 记忆类等真实场景出现再说。

## 七、对 AI agent 的价值（已铺路）

MCP 四件套（只读）即 agent 接口（见 [`docs/architecture/07-mcp.md`](architecture/07-mcp.md)）：
- `graph.stats` — 库概况
- `graph.roam` — 查询驱动漫游（激活簇 + 路径）
- `graph.random` — 发散入口（seed 可复现）
- `graph.relation` — 两实体关联强度 + 证据链

定位句：**给 agent 一个「结构侧记忆导航」——比 RAG 便宜、比向量记忆可信。**

## 八、明确不做（坦诚声明）

- TS 移植 / WASM / 移动端
- GraphRAG 全家桶 / LLM 建图
- embedding 内置 / 在线 API
- graph 数据库（Neo4j/Kuzu）——数千节点规模是负资产
- SaaS / 远程 / 云模式（个人数据不出本机红线）

## 九、护城河

1. **克制即护城河**：纯 Go 零依赖、单二进制、白盒可解释——在云端 agent 记忆泛滥的时代是稀缺品
2. **结构侧守门人**：语义候选需经引擎 Web 层融合，引擎位置稳固
3. **可复现**：`--seed` 让漫游可复现——对 agent 联调、测试、分享有实际价值
4. **双数据源 + 对账刷新 + 自动监听**：真实库上已验证（两库 36 单测绿）

## 十、后续动作

- [ ] README 差异化章节引用本定位
- [ ] M2 开发时落实「Web 层语义注入口」（API `extra_candidates` + 前端语义候选槽位，见 §五）
- [ ] 远期评估 agent 记忆类 adapter 时，先解决「边从哪来」的映射规则（§六）
