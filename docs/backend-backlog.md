# Serendipity Engine · 后端积压清单（Backend Backlog）

> 性质：**后端做什么**——未完成积压 + 开发纪律 + 红线。已落地事项不保留决策叙事，见「已落地速查」（§六）。
> 相关：战略定位 [`docs/positioning.md`](positioning.md) · 前端计划 [`docs/frontend.md`](frontend.md) · 组件架构 [`docs/architecture/`](architecture/) · 契约 [`docs/api-contract.md`](api-contract.md)。

## 一、开发纪律（横切所有后端开发）

1. **单文件 500 行左右，最好不超千行**——超过即拆。
2. **按领域拆文件，不按函数碎片化**；算法 = 包级可复用函数（为接口 / MCP 暴露直接可调用）。
3. **第三方算法库可引入**（用户拍板）——"克制"指**零依赖单二进制**（不背运行时 / 网络栈 / 服务），不是"永远不用库"。条件：MIT 类许可 + go.sum 锁定 + attribution 一行。first case = `github.com/vsuryav/leiden-go`（社区发现）。
4. **文档克制**：不搞文档工程——只记「决定 + 理由 + 状态」；已落地与已放弃的决策不保留叙事。

## 二、性能优化（当前状态）

- ✅ 已落地（v0.1.13，#16 bbolt）：P1 刷新增量写（差值 Put/Delete）/ P2 mmap + NoSync / P5 幽灵 touch 过滤 O(1) / P8 serve-while-refresh 读不阻塞（RWMutex 换图，已有）。
- ⏸ 等规模信号（当前千级节点感知不到，数万节点再看）：P3 PPR 结果缓存 / P4 similar 缓存（缓存只加速、不污染白盒）/ P7 TextSearch 有序索引（bbolt 有序键建 `idx` bucket）。

### 二.1 存储层选型：SQLite → bbolt（✅ v0.1.13 已落地）

`store.go` 换 `go.etcd.io/bbolt` v1.5.0（MIT），四表 → 四 bucket（`docs`/`links`/`touch`/`renames`），签名保持调用点零改动；无迁移（旧 `.sqlite` 直接删，refresh 重建）；编译分钟级 → 秒级。**注意**：`modernc.org/sqlite` 仍留依赖——`internal/adapter/orca.go` 用它解析虎鲸活库快照（源数据读取，与 store 持久化无关）。

### 二.2 bbolt 解锁的有趣能力（M3 候选，非承诺）

- **图谱时间旅行**：COW 快照进 `snap-<ts>` bucket，可回看潜在关联何时浮现、边何时加入。
- **探索日志 / 偶遇时刻**：touch 升格完整事件流，生成"两篇无关笔记其实 N 跳相连"的奇遇记录。
- **跨库元索引**：中心 `index` bucket 映射 `node→vault`，多库漫游成统一知识网。
- **What-if 实验图**：fork 即建桶，A/B 对比"加候选边前后"的漫游差异。
- 边界：bbolt 无查询语言，复杂全文检索别硬上（必要时外挂）；均不引入新算法依赖。

### 二.3 bbolt 对已有能力的增强（P1–P8，候选表已折叠）

P1/P2/P5/P8 已落地（见上）；P3/P4/P7 等规模信号。均不新增算法依赖。

## 三、功能缺口（已落地为主，只留骨架）

### 3.1 similar 结构相似（✅ v0.1.11 Jaccard → v0.1.12 Adamic-Adar）

找"共同邻居多但互不链接"的节点对——**embedding 语义轴的纯结构替代**（白盒、零依赖、证据可解释："因为都链接了人物B/C"）。`graph.Similar` + `/api/similar` + MCP `graph.similar`；复用 rollSeed 排除逻辑（枢纽 / 空标题 / 孤立）；区分"相关(roam)"与"相似(similar)"语义。

### 3.2 漫游导出 export（✅ v0.1.11）

`/api/roam?export=1` → 当前簇渲染为 Markdown 卡片清单（标题 + 类型 + hop + 路径 + 分数），一键带走。语义 = 卡片清单而非重新生成笔记；导出不额外 touch。

### 3.3 touch 统计 API（✅ v0.1.11，v0.1.12 幽灵过滤）

`GET /api/touch/stats` 返回"哪些节点被反复点击"（只读分析）。**绝不反馈到排序/hot**（红线 2）；不进 MCP（隐私敏感）；幽灵 touch 过滤 = targets 关联 documents 存在性。

### 3.4 社区发现 Leiden → 诊断层（✅ v0.1.12）

`internal/graph/community.go`（leiden-go，MIT）+ `/api/communities` + MCP `graph.community`——回答「库里有哪些主题簇、哪些区域互不相连」，诊断层定位知识缺口。孤立节点（度=0）检测前过滤（由 Stats.Orphans 承接）。可选真增量：介数中心性（Brandes，诊断层信号），最短路径/SCC 不引入（无场景）。

### 3.5 LLM Wiki adapter 画像（✅ v0.1.12）

真实性门槛下唯一「可接受」的 LLM 数据源（raw 事实锚点 + 人力维护）。落地：`ExcludedFiles`（文件级排除）+ 内置画像 `llm-wiki`（排除 raw/ 等目录与 index.md/log.md 等文件）+ 结构探测提示（只提示不自动启用）+ watch 排除同源。边界：`index.md` 排除**必须**经显式画像启用，绝不进默认画像。

### 3.6 潜在关联（✅ v0.1.13 候选清单落地；剩余留 M2）

取代原「mentions API（正文文本扫描）」方案（**已否决**：暴力枚举成倍加边 / 中文子串边界无解 / 虎鲸无模糊提及通道）。新方案 = 引擎从拓扑多算法估算候选对（**白盒补位，非 AI 生态位**）：
- 算法：2-hop 候选 + Adamic-Adar / Jaccard / RA 原始分加权求和 + top-K 节流（O(N·d²)，边数硬上限 K×N）。
- 落点：`graph.PotentialLinks` + `GET /api/suggest-links`（待审清单，替代原 `/api/mentions`，登记 api-contract §12）。**未落图**（候选是 `kind=approx` 形态，不进 PPR/Activate——落图需带权边改造 + 改变已验证 roam 行为）。
- **剩余（M2）**：落图进 PPR（带权边改造，需谨慎评估）；co-touch 行为信号（由插件 L1 经 touch 通道喂入——adapter 是静态翻译器、无访问时间，**绝不在 adapter 扫描**）；前端"潜在关联"展示。
- **红线**：禁语义/LLM 推断边；近似边永远标注算法 + 共享邻居证据。

### 3.7 touch 行为信号子系统（✅ v0.1.14 落地）

touch 从图库拆独立 `touch-<hash>.bbolt`（`touch`/`meta`/`backups` 三 bucket）——不可重建的原始行为信号，图库重建不再连坐。已落地：
- digest 触发双逻辑：计数 ≥ `digest_count`（默认 500，[200,600]）优先 + 间隔 ≥ `digest_days`（默认 3 天，[1,5]）兜底 + serve 启动补查；**通知先于淘汰**。
- 出口：`GET /api/touch/digest` + `POST /api/touch/digest/ack` + `/api/stats.digest_available` + MCP `seren.touch_digest`——全部只读、被动（不弹窗）；**引擎零写 vault**（digest md 由插件导出）。
- 备份：每次 digest 滚聚合快照进 `backups`，`backup_max`（默认 5）超则删最旧。
- 配置：`<vault>/.serendipity/touch.yaml`（与 profile.yaml 同约定，参数钳制区间）。
- 红线：touch 只读、不演化边权（杜绝"点击→边权变→结果变→再点击"正反馈跑飞）。

### 3.8 MCP 语义可发现性（✅ v0.2.0 落地：Layer A + Layer B 完成）

> 问题：AI 接 MCP 后"闷头调用、不知传达了什么"——所有图工具返回的是**候选/建议（探索），不是库内事实**，AI 容易把 roam/similar/relation 输出当既定知识。

- **Layer A（强制普适，MANDATORY）**：MCP 原生富化——工具 description 三件套（WHEN 何时调用 + HOW 如何解读 + 反模式）+ 只读语义注解（9 工具全只读，协议层结构化 `readOnlyHint`/`destructiveHint`/`idempotentHint`）+ `required` 字段标注。`internal/mcp` 已落地。
- **Layer B（生态增强，prompts 优先、skill 按需）**：`seren_orientation` 两个载体都已落地——MCP `prompt`（`AddPrompt`，随客户端走，Claude Code 显示为斜杠命令 `/seren_orientation`）+ Skill 资产（常驻行为准则，本仓库已分发 [`SKILL.md`](SKILL.md)）。**prompts ≠ Skill**：prompts 按需触发（说明书），Skill 常驻（行为准则）；两者内容同源（定位/能力边界/工具速查/反模式），全英文。
- 反模式清单（写入 description + Skill）：把 roam/similar/relation 输出说成库内事实；用 touch 计数推导重要性；主动刷 digest 打扰；用 touch 演化边权。

### 3.9 MCP 传输层升级：Streamable HTTP + mcp-go（✅ v0.2.0 落地）

- 引 `github.com/mark3labs/mcp-go`（Go 事实标准 MCP SDK），`seren serve` 加 `/mcp` 端点——Web + REST + MCP 三合一，一份 live 图服务所有客户端（`GraphProvider` 每次调用取当前图，修 mcp 子进程快照吃不到中途改动）；stdio 一并迁移上 SDK、**删手写 JSON-RPC**（不并存两套协议栈）。`seren mcp` stdio 兜底（Claude Desktop 类）。
- 已落地：`internal/mcp` transport-agnostic（一套工具两个入口：`Handler()`/`ServeStdio`）；`/mcp`（Streamable HTTP）+ `/api/mcp/status` + `/api/mcp/enable|disable`；前端 MCP 状态面板 + 一键配置复制。
- 鉴权：本地 127.0.0.1 + 现有 token（`/api/mcp/*` 走 token）+ Host 校验够用；`/mcp` 除 Host 校验外不强制 token（本地消费），OAuth 2.1/PKCE 留给远程公开部署。
- 工具扩至九件套：+`seren.state`（未配库引导，永远可用）。

## 四、风险分析与红线（防污染已验证行为）

1. **similar 绝不并入 roam 管线**——serendipity 结果分布是验证过的（单测），只能独立入口。
2. **touch 统计绝不反馈到排序/hot**——否则等于偷偷启动边权演化，违背克制设计。
3. **export 不改变默认行为**——可选参数，默认路径零回归。
4. **引擎零 AI 依赖**——不调用任何模型；AI 全部在引擎之外（外部 AI/agent 经只读 MCP 消费），引擎无任何 AI 代码路径。
5. **API 契约同步**——每加端点都要登记 [`docs/api-contract.md`](api-contract.md)（插件唯一共享物）。

### 缓存/累积状态盘点（已收敛，无无限增长）

touch 表 5000 条硬上限 / recent ring 32 / watch 快照 = 文件数 / 内存图 refresh 整体替换 / revision int / WAL autocheckpoint=1000 / **renames 链式折叠**（v0.1.11 `collapseChains` 只留链头→最终目标）——全部有界。空查询必须拦截（`Resolve` 对空串恒真，roam 入口已拦，新入口同样要拦）。

## 五、CLI 与壳

### 五.1 CLI 三件套（✅ v0.1.11 落地）

`seren help <cmd>` 子命令级帮助 / `--json` 结构化输出（roam/index/refresh）/ 退出码语义化（0 成功 / 2 用法错误 / 1 运行时错误）。CLI 是"双消费者"：人是第一消费者，agent（shell 直调）次级，MCP 是 AI 正式通道。

### 五.2 Wails 桌面壳（`serendipity-desktop` 独立仓库，M3 排期）

> 背景：TUI 评估后放弃（**被放弃**——"终端里开合"仍是终端，门槛未真正降低）；用户要的是"打开一个应用、手动指向笔记库、界面上管 MCP/serve 启停"的 GUI 壳。

- 形态：双击启动 → 原生对话框选库（vault 目录 / 虎鲸 .db）→ spawn `seren serve` 子进程 → 系统 WebView2 嵌现有 Web UI（`?embed=1` + postMessage 桥 + i18n 全复用）→ 面板管 MCP / serve 启停（进程状态、URL、token、重启）。
- 仓库形态同 Obsidian 插件薄壳：壳不 import 引擎包，只 spawn 二进制 + REST 契约；**引擎改动为零**（壳只需 `POST /api/vault` 配库，v0.1.15 地基已落地）。
- 壳层破零 CGO（Wails 需 gcc/mingw + WebView2）；引擎核心保持零 CGO 红线不变。体积约 27–30MB（Wails ~11MB + seren 16MB）。

## 六、已落地速查（✅，不保留叙事）

| 功能 | 版本 | 一句话 |
|---|---|---|
| similar 结构相似 | v0.1.11→12 | Jaccard → Adamic-Adar（度加权抗枢纽偏置）；`/api/similar` + MCP graph.similar |
| graph.node 节点详情 | v0.1.11 | `/api/node` + MCP graph.node（L0 摘要 + L1 邻居） |
| export 漫游导出 | v0.1.11 | `/api/roam?export=1` → Markdown 卡片清单 |
| touch 统计 API | v0.1.11→12 | `/api/touch/stats` 只读聚合 + 幽灵 touch 过滤 |
| CLI 三件套 | v0.1.11 | help / --json / 退出码 0-2-1 |
| 刷新一致性 + 体验 | v0.1.12 | 悬挂链接明细 + 幽灵 touch 过滤 + is_pending 提示 + 手动刷新清 pending |
| Leiden 诊断层 | v0.1.12 | `/api/communities` + MCP graph.community（leiden-go） |
| LLM Wiki 画像 | v0.1.12 | `llm-wiki` 画像 + `ExcludedFiles` + watch 排除同源 |
| bbolt 存储层 | v0.1.13 | SQLite → bbolt 四 bucket；无迁移；编译秒级 |
| 性能 P1/P2/P5/P8 | v0.1.13 | 增量写 / mmap+NoSync / 幽灵过滤 O(1) / 读不阻塞 |
| 潜在关联候选清单 | v0.1.13/v0.2.1 | `graph.PotentialLinks` + `GET /api/suggest-links`（未落图）+ **MCP `graph.suggest`**（v0.2.1 暴露） |
| touch 行为信号子系统 | v0.1.14 | 独立 touch store + digest（计数/间隔双触发 + 启动补查）+ ack + MCP seren.touch_digest |
| 无库启动 + 配库 | v0.1.15 | 空库 serve + `POST /api/vault` 配库/换库 + 优雅退出 + OSC 8 链接 |

## 七、后续动作

- [x] M2：§3.8 MCP 语义可发现性（Layer A + Layer B prompt/skill 均落地）
- [x] M2：§3.9 MCP Streamable HTTP 迁移（mcp-go，删手写 JSON-RPC，`/mcp` 端点 + `/api/mcp/*`）
- [ ] M2：§3.6 潜在关联落图（带权边改造）+ co-touch（插件 L1 喂入）+ 前端展示
- [ ] M3：Wails 桌面壳（§五.2）；bbolt 有趣能力（§二.2）；canvas 白板检索
- [ ] ⏸ 等规模信号：P3/P4/P7 性能增强（§二）
- [ ]  GitHub Actions 自动构建二进制，本地 `scratch/seren.exe` 仅供联调
