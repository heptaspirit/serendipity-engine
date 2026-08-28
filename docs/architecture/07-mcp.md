# MCP 暴露架构研究（v3，已落地 v0.1.9 / 扩至八工具 v0.1.14 / 迁移 mcp-go v0.2.0）

> 状态：**已落地**（v0.1.9，2026-08-23）。此前为研究稿（用户指示"先不开工，研究靠谱
> 架构 + 不影响本体的接入方式"）。方向已在 design.md §6.10 拍板：**"AI 通道用 MCP
> 而非自定义 REST……REST 给 Web / CLI 自用，MCP 给 AI 用，同一个核心。"** 本文把
> 形态、边界、tools、风险落成可执行方案；落地实现见 `internal/mcp` + `cmd/seren`
> `seren mcp` 子命令。
>
> **v0.2.0 迁移 mcp-go**：传输层从自实现薄 JSON-RPC 换成 `github.com/mark3labs/mcp-go`
> （事实标准 SDK）；`seren serve` 内嵌 `/mcp` 端点（Streamable HTTP，Web+REST+MCP
> 三合一，一份 live 图服务所有客户端——修"子进程快照吃不到中途刷新改动"）；stdio
> 一并迁移上 SDK（`seren mcp` 兜底 Claude Desktop）。工具扩至九件套（+`seren.state`，
> 未配库引导）。手写 JSON-RPC 已删除，不并存两套协议栈。

## 0. 目标与硬约束

- **目标**：让 AI（agent，含 dsh-mneme 类）能调用引擎能力——漫游、关系查询、
  统计，把"结构信号"（多路径可达性、间接关联——LLM 单看文本推不出的）交给 AI。
- **硬约束（用户）**：**不影响本体**——现有 CLI / REST / Web / 自动监听 / 反馈
  埋点 / 对账全部零改动；MCP 是**第四个入口**，与 CLI/REST 平级，共享同一内核
  （`internal` 纯库），而不是寄生在 serve 里。
- **克制原则延续**：MCP 默认**只读**——不写 touch、不触发 refresh；AI 会话不该
  有能力改动本地状态或触发全量刷新。

## 1. 形态对比：独立入口（推荐）

| 形态 | 优点 | 缺点 | 结论 |
|---|---|---|---|
| **独立入口 `seren mcp`**（新子命令，进程内复用 internal 包） | 进程隔离（MCP 崩了核心照跑）；零侵入现有 serve；单二进制分发不变 | 多一个进程入口 | ✅ 推荐 |
| serve 内嵌 MCP（同一进程开 MCP 端点） | 少一个进程、少一次图加载 | 侵入现有服务生命周期；MCP 挂掉可能拖垮 Web；违反"不影响本体" | ❌ |
| 独立 MCP server 进程（新二进制） | 解耦最彻底 | 破坏单二进制分发；多一个发布物 | ❌（过度） |

**"不影响本体"的落点**：MCP 子命令只 `import internal/{graph, roam, adapter,
store, score, sync}`（纯库、无副作用、不启动监听）；**绝不 import
`internal/web`（Web 是消费者不是内核）、`internal/watch`（监听是 serve 的事）**。
本体代码零改动，MCP 只是复用同一批库函数的新壳。

## 2. 传输与协议

- **MCP 规范**：JSON-RPC 2.0；本地 agent/remote 走 **Streamable HTTP**（serve 内嵌 `/mcp`，
  传输为 process 内同一份 live 图）；独立 `seren mcp` 子进程走 **stdio transport**
  （Claude Desktop 类启动子进程，走 stdin/stdout）。**initialize 回显客户端请求的
  protocolVersion**（v0.1.10；mcp-go 标准协商处理版本兼容）。
- **Go 实现（v0.2.0 定稿）**：用事实标准 SDK `github.com/mark3labs/mcp-go`
  （`NewMCPServer` + `AddTool` + `NewStreamableHTTPServer` / `ServeStdio`）。工具注册
  transport-agnostic（一套工具两个入口）；**手写 JSON-RPC 已删除，不并存两套协议栈**。
  用户已确认"正经功能不再零依赖"（mcp-go 纯 Go 无 CGO，单二进制不变）。
- **接入 dsh**：dsh 支持配置 MCP server（stdio / streamable-http），指向
  `seren mcp --db <store>` 或 `seren serve` 的 `/mcp`；dsh-mneme 类 agent 即可调用。
  这条通道就是 design.md §6.10 "dsh 生态现成的 AI 桥"。

## 3. Tools 设计（只读十一工具；v0.1.9 四件套 + v0.1.11 扩 node/similar + v0.1.12 加 community + v0.1.14 加 touch_digest + v0.2.0 加 state + v0.2.1 加 suggest/touch_stats）

| tool | 入参 | 复用内核 | 说明 |
|---|---|---|---|
| `graph.stats` | 无 | `Graph.Stats()` | 库规模/连通/枢纽——AI 先摸库。v0.2.1 起附带 `dangling_refs` 悬空明细（截断），供 AI 定位格式噪声 |
| `graph.roam` | `q, top, lambda, theta, hops` | `roam.Compute` | 查询漫游 → 节点簇（锚点+路径+分数） |
| `graph.random` | `top, seed, rand_alpha` | `roam.ComputeRandom` | 🎲 随机漫步（v0.1.7）：随机起点 + 簇——AI 无明确目标时的"随便逛逛"入口；`seed` 固定可复现 |
| `graph.relation` | `from, to` | `Graph.ComputeRelation` | 两节点：最短路径 + 双向 PPR + 证据链（v0.1.5 已铺路） |
| `graph.node` | `id` | `Graph.NodeDetail` | 单节点详情（v0.1.11）：L0 Text 摘要 + L1 邻居/被引用——AI 漫游到节点后"确认这是不是我要的"。v0.2.1 起 text 摘要带 `text_len`（全文 rune 长度）+ `text_truncated`（是否被截断），AI 不会误当全文 |
| `graph.similar` | `id, k` | `Graph.Similar` | 结构相似节点（v0.1.11 Jaccard → v0.1.12 **Adamic-Adar** 度加权）：共同邻居多但互不链接，带共享邻居证据——AI 判断"哪些笔记说同一件事" |
| `graph.community` | `resolution, seed, top, node` | `Graph.Communities` | 社区发现（v0.1.12，Leiden）：把图拆成主题簇——AI 定位"有哪些主题簇、哪块互不相连"（诊断层）。v0.2.1 加 `top`（默认 10 = 只回最大几个簇并裁剪 membership；0 = 全量）+ `node`（只回该节点所在社区，避免吞全图 membership） |
| `graph.suggest` | `k` | `Graph.PotentialLinks` | 潜在关联候选（v0.2.1，roadmap #15 / backlog §3.6 落 MCP）：2-hop 邻域 AA/Jaccard/RA 三算法融合，带共享邻居证据（"都链接了 X/Y"）+ 端点标题 `a_title`/`b_title`（反馈 #5，与 REST 对齐）、**未落图**——AI 取清单 + 笔记正文研判，接受者写回 kind=ai 边。互链补全缺口 |
| `seren.touch_digest` | 无 | store 只读闭包（§3.7） | 行为信号 digest（v0.1.14）：窗口点击聚合 TopN（幽灵过滤 + 标题）+ 来源 TopN——AI 识别"哪些主题在升温"。只读、被动（当前窗口无活动返回空摘要，非累计统计）。v0.2.1 起明确与 `seren.touch_stats`（累计）区分 |
| `seren.touch_stats` | 无 | store.TouchStats（§3.7） | 累计点击统计（v0.2.1，反馈 #1）：total + TopN targets/sources（幽灵过滤 + 标题），等价 REST /api/touch/stats——补 MCP 侧的"非空"累计视角，与窗口 digest 互补 |
| `seren.state` | 无 | server 状态（provider） | 会话状态（v0.2.0）：是否已配库 / 传输方式 / 工具数——未配库时给出引导提示；永远可用、不依赖图 |

> 2026-08-23 用户指示：把随机漫游也加进 MCP（灵感：恐龙工具箱 SRS 的 roam /
> 随机漫步交互）。`graph.random` 与 `graph.roam` 共用同一簇管线（clusterFromSeeds），
> 只是起点从查询锚定换成 roll——实现成本几乎为零。

> **会话管理（v0.2.1，反馈 #9）**：serve 的 `/mcp` 是 mcp-go Streamable HTTP。
> 默认 `StatelessGeneratingSessionIdManager` 会校验客户端回传的 `Mcp-Session-Id`，
> 不匹配即回 404 "Invalid session ID"——`.NET HttpClient` 等客户端在重连/未正确回传
> 会话 id 时被拒（curl 同流程正常）。seren 是只读工具服务、无推送/采样/会话内状态，
> 改用宽松 `SessionIdManager`：`Generate` 返回随机 id（客户端可拿），`Validate/Terminate`
> 恒放行——任何/空的会话 id 都接受，最大化兼容各类 MCP 客户端。代价是服务端不做会话
> 一致性（seren 本就不依赖）。

原则：
- **全部只读**；不暴露 refresh / touch / 配置写接口。v0.2.0 起工具级 `readOnlyHint`/
  `destructiveHint`/`idempotentHint` 注解（§3.8 Layer A）在协议层结构化声明只读语义。
- **prompts**（§3.8 Layer B）：`seren_orientation` 经 `AddPrompt` 注册——Claude Code 等
  客户端以斜杠命令 `/seren_orientation` 触发，把"定位/能力边界/工具速查/反模式"注入下文
  （按需说明书，与常驻 SKILL.md 行为准则互补，内容同源、全英文）。
- 输出用引擎现有 JSON 结构（roam.Outcome / graph.Relation 直接序列化），
  白盒（带路径和证据），AI 可直接消费。工具描述为英文（跨 AI 客户端兼容）。
- 未来可加：`vault.list`（多库切换）、`graph.neighbors`（局部邻域）——按需，不急。

## 4. 数据加载（图生命周期）

- `seren mcp --db <store.bbolt>`：从持久化存储加载图（复用 `store.Load` +
  `graph.Build`，含改名重定向），**启动时加载一次**，会话期间持有内存图。
  **自 v0.2.0 起**：serve 内嵌 `/mcp` 由 `web.Server` 经 `GraphProvider` 每次调用
  取**当前**（RLock）`G/P`——库 refresh/换库后自动用新图，不再需要重开会话。
  stdio 场景（`seren mcp` 子进程）同样经 `GraphProvider` 返回构建时的图（无 watch，
  库更新重开会话即可）。
- 与 serve 同构但 **stdio 无自动监听**：AI 会话短、不需要实时；库更新了重开会话即可。
  serve `/mcp` 已 live（吃得到中途刷新）。
- 多库：v1 单库（启动参数定），多库用多个 MCP 实例或 tool 参数选库（v2 再定）。

## 5. 与本体边界总结

```
cmd/seren 子命令:
  index / roam / serve / refresh / profile-detect   ← 现有（不动）
  mcp                                               ← 新增（v3）
        │ 只 import internal 纯库
        ▼
  internal/{graph,roam,adapter,store,score,sync}    ← 内核（复用）
        ▲
  internal/{web,watch}                              ← 消费者（MCP 不碰）
```

对账 / 监听 / 埋点 / Web 全部不动；MCP 与它们**平行**，共享图与查询内核。

## 6. 风险与克制

- **AI 触发成本**：roam / relation 是内存计算，毫秒~秒级、无外部副作用——安全。
- **只读防线**：tools 层面就不暴露写操作，防 AI 误改本地状态（库数据 / touch）。
- **不触发刷新**：MCP 会话不跑 watch/refresh；避免 AI 循环调用导致频繁全量解析
  （正反馈/资源风险，与既有克制原则一致）。
- **凭证安全**：MCP 只读图数据；虎鲸 Repo 表红线不变（从不解析，见 02-adapters）。

## 7. 落地步骤（v0.1.9 已走完 1-3）

1. **[x] 协议实验**：Go 里实现 `initialize` / `tools/list` / `tools/call` 的最小
   stdio JSON-RPC 服务（`internal/mcp`），单测覆盖生命周期与错误路径。
2. **[x] `seren mcp` 子命令**：解析 `--db`（复用 storePathFor / loadSource 逻辑），
   启动时建图，注册八件套 tools（stats / roam / random / relation / node / similar / community / touch_digest）。
3. **[x] 联调（本地验证）**：echo 管道调 `initialize` / `tools/list` / tools/call，
   验证返回结构与 AI 可读性；确认不触发任何写操作；import 边界守护（维护指南 §4.1）。
4. **[✓ 已测 / ⏸ 临时停用] dsh 联调**：已在 DSH profile 的 `cordis.patch.yml` 注册 `mcp-seren`
   stdio 实例（`transport: stdio` + `command: seren.exe mcp <vault> --db <store>`，
   serverName=seren，failOnStartupError:false；原文件已备份）。DSH web **重启后**生效，
   工具以 `mcp__seren__graph.stats / .roam / .random / .relation` 出现。DSH 的 MCP 客户端
   用官方 `@modelcontextprotocol/sdk` 的 StdioClientTransport（与我们自实现的
   initialize/tools/list/tools/call 兼容）；env 清洗只丢凭据形状与过期 DSH_* 变量，
   TEMP/PATH 保留（sqlite 无碍）。v0.1.10 修复 initialize 回显 protocolVersion（消除
   SDK 客户端重连循环）+ 启动横幅仅 TTY 打印，**已重启并实测通过**（stats=235 /
   roam / random / relation 四件套齐全且控制台安静）。
   **当前该实例在 cordis.patch.yml 中被注释停用**（临时接入测试用，避免一直占 MCP 位）；
   需要时取消注释 + DSH web 重启即重新启用。

## 8. 决策已定 / 留待

- **[定] Go SDK vs 自实现薄协议**：**改用 `github.com/mark3labs/mcp-go`**（v0.2.0，
  用户拍板"正经功能不再零依赖"）——Streamable HTTP + stdio 同一 SDK，删手写
  JSON-RPC；mcp-go 纯 Go 零 CGO，单二进制不变。
- **[定] MCP 留在主二进制**（用户确认 2026-08-23）：子命令=独立进程已隔离 + import
  边界守护，"不影响本体"已达成；不拆独立二进制（破坏单二进制 + 同一内核原则）。
- **[定] MCP 双入口**（v0.2.0）：serve 内嵌 `/mcp`（Streamable HTTP，live 图）为主；
  `seren mcp`（stdio）兜底 Claude Desktop 类。前端/插件经 `/api/mcp/status` 查状态、
  `/api/mcp/enable|disable` 启停。
- [待定] 是否允许 `graph.roam` 带 `from`（锚点种子）以外的写类参数（默认不允许）；
- [待定] 多库形态（v2）。
