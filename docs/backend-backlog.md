# Serendipity Engine · 后端积压清单（Backend Backlog）

> 日期：2026-08-23（由外部审计机会清单定稿并汇入仓库）
> 来源：与用户讨论（2026-08-23）+ 代码评审（graph/store/sync/roam）+ 调研（PPR）
> 性质：**后端做什么**——性能优化与功能缺口的可选清单，不承诺全做。原则：**新增旁路，不修改核心；守住红线，不污染已验证行为**。
> 相关：战略定位 [`docs/positioning.md`](positioning.md) · 前端计划 [`docs/frontend.md`](frontend.md) · 组件架构 [`docs/architecture/`](architecture/) · 历史的验证/决策 [`docs/history/`](history/)。

> 目标：在不增加过多无用功能、保留后端简洁可靠的前提下，找出值得做的性能优化与功能缺口。

## 一、总览

| 类别 | 项目 | 价值 | 风险 |
|---|---|---|---|
| 性能优化 | Stats 缓存 / PPR 提前收敛 / TextSearch 小写缓存 / Store 增量写 | 大库（数万节点）时显著 | 前三个低，Store 增量写中 |
| 功能缺口 | 结构相似节点（similar） | **高**——补语义缺口的白盒方案 | 中（需防度偏置） |
| 功能缺口 | 漫游导出（export） | 中高——工作流闭环 | 低 |
| 功能缺口 | touch 统计 API | 中——反馈闭环只读第一步 | 低（防滑坡） |

## 二、性能优化（克制、低风险、有代码实据）

| 优化 | 代码实据 | 收益 | 风险 |
|---|---|---|---|
| **缓存 `g.Stats()`** | `roam.go:136` 每次漫游都调 `g.Stats()`（全图并查集 + 枢纽排序），图不变结果就不变；`roam.go:213` rollSeed 也用 | 纯收益——查询响应省掉全图遍历；refresh 换图后失效即可 | 零风险 |
| **PPR 提前收敛** | `activation.go:23` 固定 60 次幂迭代；PPR 是收敛的，实际 20-30 次即稳定 | 大图（数万节点）查询提速 2 倍级 | 需单测锁住输出不变性 |
| **TextSearch 小写缓存** | `graph.go:251` 每次查询对全图 Text 做 `strings.ToLower`（分配 + 遍历） | 高频查询显著提速；文档未变则小写文本可缓存 | 低，注意 refresh 联动失效 |
| **Store 增量写** | `store.go:274` 全量 `DELETE + INSERT`，即便只有 1 个文档变化 | 大库刷新从全量写变增量写（v1.5 已规划） | 中——涉及对账语义，建议 M1 后 |

**诚实说明**：前三项对当前千级节点是"感觉不到但架构正确"的优化；真正到数万节点（虎鲸块级）才见分晓。建议 Stats 缓存顺手做掉（10 分钟），其余进 backlog 等规模信号。

## 三、功能缺口

### 3.1 结构相似节点（similar）—— 最高价值

- **概念**：找"共同邻居多但互不链接"的节点对（Jaccard 相似度）。两篇笔记都关联同一批人物但彼此无链接 → 大概率主题相近。
- **价值**：**embedding 语义轴的纯结构替代**——白盒、零依赖、证据可解释（"因为都链接了人物B/C"）。把"不做 embedding"的决策从妥协变成有替代方案。
- **实现**：`graph.go` 加 `Similar(id, k)`（局部按需计算，O(邻居²)，不预计算全图）；Web 加 `/api/similar`；卡片加「相似」按钮；UI 展示共享邻居清单作为证据。
- **风险**：Jaccard 度偏置（枢纽天然像所有人）→ 复用 rollSeed 排除逻辑（枢纽/空标题/孤立）+ 相似度阈值；区分"相关(roam)"与"相似(similar)"语义（不同入口、不同标签）。

### 3.2 漫游导出（export）—— 工作流闭环

- **概念**：`/api/roam?export=1`（或 `Accept: text/markdown`）把当前簇渲染为 Markdown 卡片清单（标题 + 类型 + hop + 路径 + 分数），一键带走。
- **价值**：漫游发现的东西能沉淀进笔记，而不是截图/手抄。对创作工作流（如小说人物关系网）尤其有用。
- **实现**：服务端把同一份 JSON 结果渲染成 Markdown——复用现有管线，零侵入；默认路径（无参数）行为完全不变。
- **风险**：导出语义要明确 = 卡片清单而非重新生成笔记；导出不额外 touch。

### 3.3 touch 统计 API —— 反馈闭环只读第一步

- **概念**：`GET /api/touch/stats` 返回"哪些节点被反复点击、哪些边被反复激活"（只读分析）。
- **价值**：先看懂数据，再决定是否演化边权（呼应 M1）；回答"越用越准"是否有依据。
- **实现**：`store.go` touch 表已埋点（5000 条容量），只读 SQL 查询即可。
- **风险**：**绝不反馈到排序/hot**——否则等于偷偷启动边权演化，违背 v0.1.4"埋点只记录不演化"决策；不进 MCP（隐私敏感）。

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

> 全量盘点项目所有累积状态。结论：**绝大多数有界，唯一无上限的是 renames 表**。
> 以下条目"看着情况删"——修复后即勾掉，不永久占位。

| 状态 | 位置 | 上限机制 | 状态 |
|---|---|---|---|
| touch 埋点表 | store.go AppendTouch | 5000 条硬上限，超出删最旧 | ✅ 有界 |
| 随机漫步 recent ring | web/server.go:112 | 32 个上限，超出截断 | ✅ 有界 |
| watch 文件快照 | watch.go:85 | = 文件数，删文件同步删 key | ✅ 有界 |
| 前端 localStorage 参数 | index.html | 白名单 key，值有界 | ✅ 有界 |
| 内存图（graph） | 全程 | = 节点数，refresh 整体替换不累积 | ✅ 有界 |
| revision 计数 | web/server.go:107 | int 溢出需 21 亿次刷新 | ✅ 实际不可能 |
| SQLite WAL | store | 未设 wal_autocheckpoint，长跑 + 频繁 touch 可能缓慢增长 | ⚠️ 建议 `PRAGMA wal_autocheckpoint=1000` |
| **renames 表** | store.go SaveRenames + sync.go MergeRenames | **无上限** | ❌ **唯一真实风险点** |
| 日志 | watch/web log.Printf | 走 stderr 不落盘 | ✅ 安全 |

**renames 表风险**：`MergeRenames`（sync.go:379）失效逻辑 = 旧名重现才删；"目标消失但旧名未重现"的中间环节点永久保留（为链式改名传递解析）。**文件反复改名（A→B→C→D…）时每轮留一行，条目数 = 历史改名总次数，理论无上限**（实际每条几十字节，千次改名才几十 KB，严重度低）。**修法**：保留映射但丢弃被其他映射覆盖的中间环（A→B、B→C 存在时 A→B 可删，传递解析已不需要），每条改名链只留"链头→最终目标"。

**空查询 + 全图锚定**：`Resolve` 对空串 Contains 恒真——`roam.Compute`（roam.go:64）已拦截，但未来新入口必须同样拦截（计算放大，非存储放大）。

## 五、CLI 打磨三件套（人机双消费者，2026-08-23 用户提出）

> CLI 是「双消费者」：人是第一消费者，agent（shell 直调场景）是次级——MCP 才是 AI 正式通道。
> 实测现状（v0.1.10 源码）：已有全局 help / stderr 错误 + exit 1；缺子命令级帮助、结构化输出、退出码语义化。

| # | 改进 | 现状 | 建议 |
|---|---|---|---|
| 1 | **子命令级 help** | `seren roam -h` 不显示 roam 特有参数 | `seren help roam` / `-h` → 只显示该子命令参数（纯文本，零依赖，半天量级） |
| 2 | **--json 结构化输出** | 全是人类格式化文本，AI/脚本 grep 硬解析 | `roam --json` / `index --json` / `refresh --json` → 复用现有结构体（roam.Outcome / sync.Result）序列化 |
| 3 | **退出码语义化** | 只有 0/1，参数错误与运行时错误不分 | 0 成功 / 2 参数或用法错误 / 1 运行时错误（解析失败、库不存在）——agent 能自纠而非误报 |

**补充发现**：仓库根 `seren.exe` 是 v0.1.5 旧二进制（源码 v0.1.10）——需 `go build -o seren.exe ./cmd/seren` 重建，避免旧行为误导测试/演示。

## 六、MCP 工具扩展评估（2026-08-23 用户提出）

> 结构不变（stdio JSON-RPC 薄协议、只读、零第三方依赖），只评估工具集扩展。
> 现状四件套：`graph.stats` / `graph.roam` / `graph.random` / `graph.relation`（见 [`docs/architecture/07-mcp.md`](architecture/07-mcp.md)）。

### 建议新增（按价值排序）

| 工具 | 作用 | 理由 |
|---|---|---|
| **graph.node** | 单节点详情：Title/Type/Aliases/Tags/Text 摘要 + 邻居列表 + 被引用（backlinks） | **最该加**——AI 漫游到节点后需要"确认这是不是我要的"，现在缺这个基础动作；与 [`docs/frontend.md`](frontend.md) #3 节点详情 API 同源，一次实现两端受益 |
| **graph.similar** | 结构相似节点（Jaccard 孪生） | 与 §3.1 `similar` 联动；AI 判断"哪些笔记在说同一件事"时用，补语义缺口的白盒替代 |
| **graph.search** | 显式全文检索（LIKE 命中列表） | roam 无锚点时已降级全文，显式暴露更清晰——AI 精确找词场景 |
| **graph.list**（可选） | 按类型/标签过滤列节点（分页） | AI 摸库用（"列出所有 type=人物 的节点"）；stats 只有数量没名单 |

### 明确不加（克制边界）

- **graph.read（读正文全文）**：引擎是"发现层"不是"阅读层"（见 [`docs/history/product-form.md`](history/product-form.md)）——正文由 Obsidian/虎鲸宿主负责；node 只给 Text 摘要截断。
- **写类工具（touch / refresh / 边权）**：违背只读红线，AI 会话不能改动本地状态。
- **graph.hot**：graph.stats 已含 TopHubs，增量价值≈0。

## 七、优先级建议

1. **Stats 缓存**（顺手做掉，10 分钟）
2. **similar 结构相似**（补语义缺口的白盒方案，一次投入长期受益）
3. **export 漫游导出**（工作流闭环，成本低）
4. **touch 统计 API**（反馈闭环第一步，只读）
5. **graph.node**（MCP 最缺的基础动作，与前端节点详情 API 同源）
6. **CLI 打磨三件套**（子命令 help / --json / 退出码——发布前做，onboarding 体验）

## 八、与现有文档的关系

| 文档 | 主题 | 边界 |
|---|---|---|
| [`docs/positioning.md`](positioning.md) | 战略定位（笔记库=agent 记忆、embedding 边界、数据源边界） | 为什么做 |
| [`docs/frontend.md`](frontend.md) | Web UI 功能计划（P0 插件化前置 / P0.5 hero / P1 / P2） | 前端做什么 |
| **本文件** | 后端性能优化 + 功能缺口 + CLI/MCP 打磨 + 风险红线 | 后端做什么 |

## 九、后续动作

- [ ] similar / export / touch-stats 三个端点一起登记进 [`docs/api-contract.md`](api-contract.md)
- [ ] similar 实现时复用 rollSeed 排除逻辑 + 相似度阈值 + 共享邻居证据展示
- [ ] Stats 缓存与 refresh 换图联动失效（单测覆盖）
- [ ] graph.node 与前端节点详情 API 一起实现（两端受益，见 [`docs/frontend.md`](frontend.md)）
- [ ] CLI 三件套（help/--json/退出码）发布前完成
- [ ] 重建 seren.exe（当前仓库根是 v0.1.5 旧二进制）
- [ ] renames 中间环清理（MergeRenames 丢弃被覆盖的旧映射，每条改名链只留链头→最终）
- [ ] store 加 `PRAGMA wal_autocheckpoint=1000`（长跑 + 频繁 touch 稳住 WAL）
