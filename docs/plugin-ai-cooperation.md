# 插件与 AI 的协作架构（引擎仅暴露接口与算法，零 AI 依赖）

> 日期：2026-08-25（M2 设计，承接「AI 能力全在插件层、内核零 AI 依赖」决议）
> 性质：**详细设计 / 决策档案**——插件开发计划的「插件 × AI 协作」专题深入版。规划级简化见 [`plugin-dev-plan.md` §九](plugin-dev-plan.md)。
> **本文视角**：主语是**插件**——讲插件如何与 AI 配合。引擎是被动的「接口 + 算法」提供方：它只暴露 REST/MCP 契约与确定性算法（图 / PPR / 激活扩散 / similar / 潜在关联(近似边) / communities…），**永不调用、也不依赖任何 AI**。所有 AI 行为都发生在插件层，引擎对此无感知。
> 关联：[`plugin-dev-plan.md`](plugin-dev-plan.md)（M2 执行计划）· [`backend-backlog.md` §3.6](backend-backlog.md)（潜在关联，克制+AI 补位）· [`api-contract.md`](api-contract.md)（引擎 REST 契约）· 立场 [`../serendipity-openviking-stance.md`](serendipity-openviking-stance.md)（真实性门槛 / 红线）

---

## 〇、定位与红线（与现有决策对齐）

**核心决议（已定）**：AI 能力全部放在插件层，引擎内核零 AI 依赖、离线、确定性。引擎**永不调用**任何 AI；AI **不污染**引擎的白盒承诺。

> **引擎的边界（白盒铁律）**：引擎 = 算法 + 接口。它对外只提供「确定性算法结果」与「只读/写入 REST 契约」，**没有任何 AI 代码路径**。插件把 AI 的产出（语义判定、建议边）以「带溯源的数据」形式交回引擎，引擎只当数据存、不消费语义。因此——**AI 是插件的伙伴，不是引擎的伙伴**。

**与三条已定红线的对齐**：

| 红线（出处） | 对 AI 协作的约束 |
|---|---|
| 引擎禁语义/LLM 推断边（backend-backlog §3.6） | AI 推断出的边**不构成"真实链接"**——必须 `kind=ai` + 强制溯源 + 用户可一键撤销。引擎只接收、不推断。 |
| 节点内容须是真实笔记（stance §2.2：语义检索通道 ✅ / LLM 生成节点 ❌） | AI **只建议"链接"**，绝不生成笔记正文进图。候选对永远来自引擎的真实节点对。 |
| 引擎离线可用、确定性（开发纪律） | AI 是**可选富集**：关掉 AI，前端仍是纯壳、引擎照常跑。AI 态不进引擎持久层。 |

> 关键澄清：**「引擎接受 AI 建议的边」≠「引擎做语义推断」**。语义判断发生在插件 AI 层（明确允许有 AI）；引擎只把这条边当数据存（typed edge + provenance），并原样展示溯源。白盒承诺靠「provenance 永远可见」维持，而非靠「引擎自己算」。

---

## 一、三层模型

```
┌─────────────────────────────────────────────────────────────┐
│ Layer 0 · 宿主笔记软件（Obsidian；~~虎鲸 Orca 插件暂停~~）   │
│   笔记内容 · 当前块/选择上下文 · （可选）本地 AI（Ollama）    │
├─────────────────────────────────────────────────────────────┤
│ Layer 1 · 插件（薄壳 + AI 大脑）                             │
│   (a) 薄壳：iframe 引擎 Web UI + 连接 + 跳回 + 隐式 touch    │
│   (b) AI 大脑：语义理解 / NL 查询 / 候选边研判 / 解释生成     │
│       └─ AIBackend 抽象（local-cloud HTTP | host-AI bridge）│
├─────────────────────────────────────────────────────────────┤
│ Layer 2 · 引擎内核（纯算法，零 AI，只读 REST/MCP）          │
│   图 / PPR / 激活扩散 / similar / 潜在关联(拓扑) / touch /      │
│   communities / node / relation / suggest-links              │
└─────────────────────────────────────────────────────────────┘
```

- **Layer 1 与 Layer 2 之间只有一套 REST 契约**（即 [`api-contract.md`](api-contract.md)）——AI 不引入第二条通道，避免契约分裂。
- **Layer 0 与 Layer 1 之间**：插件用宿主 API 读笔记正文、取当前块、呈现结果（Obsidian `app.vault` / ~~虎鲸 `orca.plugins.readFile` + `BlockEditorContext.rootBlockId`，暂停~~）。

---

## 二、协作三原则（清晰边界）

1. **引擎供给事实，AI 在其上推理（Engine → AI supply）**
   引擎产出确定性、可解释、永远在线、免费的事实：激活簇（roam）、结构相似（similar）、潜在关联候选（suggest-links）、社区/知识缺口（communities）、单节点证据（node/relation）。AI 把这些当"原料"做语义判断。

2. **AI 通过「typed-edge + provenance」回流，引擎只存数据不推断（AI → Engine enrichment）**
   AI 的语义结论以**带溯源的 typed edge** 形式提交引擎（见 §三 `POST /api/edges`）。引擎不知道也不关心边是人是 AI 提的——它只是又一条 `kind=ai` 的边，查询时照常并入。引擎对 provenance **只展示、不消费**。

3. **引擎 = 广而便宜的网，AI = 语义过滤/升级器（潜在关联协同）**
   引擎 refresh 时产出**有界、拓扑、永远在线**的潜在关联候选；AI 取候选 + 两边笔记正文做语义研判，接受的升级为 `kind=ai` 边、拒绝的丢弃。这正对应 §3.6 的「AI 补位」定位——引擎不占 AI 生态位，只做算法层补位，AI 做它擅长的语义判断。

---

## 三、引擎 ↔ 插件 REST 契约增量

**复用（现有只读端点）**：`/api/stats` `/api/roam` `/api/similar` `/api/relation` `/api/node` `/api/touch/stats` `/api/communities`。

**新增（为 AI 协作，须登记 api-contract）**：

### `GET /api/suggest-links?k=20` · 潜在关联待审清单（复用 §3.6 候选 pass）
返回引擎 refresh 时产出的拓扑潜在关联候选 top-K：`{a,b,score,algorithms:[...],evidence:{shared_neighbors,touch_cooccur}}`。这是 AI 研判的"原料清单"。只读、无副作用。

### `POST /api/edges` · AI/用户建议边 overlay（内存态）
请求体（数组，支持批量）：
```json
[{"from":"<id>","to":"<id>","kind":"ai","weight":0.3,
  "provenance":{"source":"ai","model":"<name>","confidence":0.8,"rationale":"都讲 X 的背叛"}}]
```
- `kind` ∈ `ai`（AI 建议）`| user`（用户在插件里手动加的关联）。引擎**不新增** `approx-ai`——潜在关联已由引擎自身以 `kind=approx`（近似边）产出，AI 只在其上做"接受/拒绝/升级"。
- **引擎语义**：存**内存 overlay**；roam / activate / similar 查询时并入（带其 weight）。引擎**不重算** AI 度、不调任何模型。
- **引擎无 AI 态持久化（纯度保护）**：overlay 仅内存；持久化由**插件负责**（插件数据目录 sidecar，如 `<vault>/.serendipity-ai/links.json`）。插件在① 引擎启动探活后 ② 每次 `/api/stats` 检测到 `revision` 变化（refresh 后）时，把 sidecar 重推一次 `POST /api/edges`。这样引擎零 AI 持久态、refresh 不丢 AI 边。
- 撤销：`DELETE /api/edges`（按 from/to 或 provenance.source 批量删）——用户在前端点"撤销 AI 建议"即调用。

---

## 四、五个协作流（具体）

### Flow 1 · AI 建议链接研判（头条协同，对应 §3.6）
1. 引擎 refresh 产出潜在关联候选 → `GET /api/suggest-links` 返回 top-K（A,B,score,evidence）。
2. 插件 AI 取候选 + 两边笔记正文（Layer 0 读：Obsidian `app.vault.read` / ~~虎鲸 `orca.plugins.readFile`，暂停~~）→ 批量问："A 与 B 是否语义相关？为什么？"
3. AI 返回判定（`related?` `confidence?` `rationale?`）。
4. 插件把接受的 pair 写 sidecar + `POST /api/edges`（`kind=ai`, provenance）。
5. roam 现包含它们；node 详情显示「AI 建议关联 · 置信 0.8 · 因为…」——白盒、可撤销。

### Flow 2 · NL 查询 → 引擎执行
用户问"找和主角成长弧相关的笔记" → 插件 AI 解析意图 → 锚定（`/api/search` 或 AI 从 `/api/communities`/stats 挑候选节点）→ `GET /api/roam` → 合成 NL 答案 + roam 卡片（iframe 或渲染）。引擎是 AI 背后的"查询执行引擎"。

### Flow 3 · "为什么相关" 解释
引擎 similar/approx 给**拓扑证据**（共享邻居）→ 插件 AI 加**语义注释**（"它们都讲 X 的背叛"）。白盒拓扑 + 语义 gloss 双层解释。

### Flow 4 · 知识缺口叙述
`/api/communities` + `stats.orphans` + `dangling_refs` → AI 叙述"你有 3 个互不相连的主题簇，X 区域是盲区"。把诊断层信号转成 prose。

### Flow 5 · 被动富集（v2 可选）
打开笔记（touch / `rootBlockId` 变化）→ 插件问 AI "库里哪些语义邻近？" → 与 similar/approx 交叉 → 侧栏"你可能想看"。

---

## 五、AI 大脑内部（插件层，平台无关）

**AIBackend 抽象**（插件内，两个实现，共享 80% 胶水）：
- `interface AIBackend { complete(prompt) → text; completeStream(prompt) → stream }`
- **impl A（Obsidian）**：`fetch` 到用户配置的 endpoint（本地 Ollama `http://localhost:11434` 或 OpenAI 兼容）。key 存插件 settings，**不进引擎、不进图**。
- ~~**impl B（虎鲸）**：`orca.ai.sendMessage` / `sendStreamMessage`（host 提供 AI，若用户已配）——薄壳直接桥接，少写一层。~~（〔2026-08-26 暂停〕）

**语义通道 vs 引擎**：AI 只看「笔记内容 + 引擎给的事实」，不替引擎建图、不生成笔记内容。

**隐私披露**：local 读笔记已由「二次确认启动」覆盖；若用户启用**云 AI**，需额外披露「笔记内容将发往你配置的 AI 服务」（设置页勾选 + 状态栏标识）。

---

## 六、与红线的对账（确保不破白盒）

| 检查项 | 结论 |
|---|---|
| 引擎无 AI 依赖 | ✅ overlay 是 push-only 数据；引擎从不调用任何模型 |
| AI 边非真实链接 | ✅ `kind=ai` + provenance 强制显示；用户可一键撤销 |
| 节点内容真实 | ✅ AI 只建议链接，绝不生成笔记内容进图（语义检索通道 ✅） |
| 离线可用 | ✅ 引擎零 AI 依赖；AI 是可选富集，关闭后前端仍是纯壳 |
| 不污染已验证行为 | ✅ 新增端点独立；roam/similar 等既有管线零改动；overlay 仅叠加 |

---

## 七、开发落点（M2 阶段）

| 部件 | 改动 | 向前兼容 |
|---|---|---|
| 引擎 | 新增 `POST /api/edges`（内存 overlay）+ `DELETE /api/edges` + `GET /api/suggest-links`（复用 §3.6 候选 pass）；登记 api-contract | 无 AI 时这些端点闲置，既有行为不变 |
| 插件（薄壳） | iframe Web UI + 连接/跳回/隐式 touch（先不做 AI 也能跑） | — |
| 插件（AI 模块） | AIBackend 抽象 + 设置页（endpoint/key/model + 云 AI 披露）+ AI 面板；Flow 1 起（最高价值最低风险） | 薄壳先落地，AI 模块增量叠加 |

**建议顺序**：引擎端点（无 AI 也能跑）→ 插件薄壳 → 插件 AI 模块（Flow 1 起）。

---

## 八、与现有文档的关系

| 文档 | 主题 | 边界 |
|---|---|---|
| [`plugin-dev-plan.md` §九](plugin-dev-plan.md) | 插件 × AI 协作的**规划级简化** | 决定 + 理由 |
| **本文** | 插件 × AI 协作**详细设计**（端点增量 + 五流 + AIBackend + 红线对账） | 如何做 |
| [`backend-backlog.md` §3.6](backend-backlog.md) | 潜在关联（引擎侧拓扑估算，AI 补位来源） | 引擎做什么 |
| [`api-contract.md`](api-contract.md) | 引擎 REST 契约（含本节新增端点） | 接口事实源 |
| [`../serendipity-openviking-stance.md`](serendipity-openviking-stance.md) | 真实性门槛 / 数据源红线 | 为什么禁 LLM 生成节点 |
