# 插件化调研与决策：Obsidian / 虎鲸薄壳（不做移植）

> ⚠️ **历史记录（已归档）**：本文是 2026-08-23「插件化」调研与拍板记录（D1–D8 + 薄壳工作清单）。决策已由 [`docs/roadmap.md`](../roadmap.md) M2 承接（两个独立仓库、零构建时依赖、唯一共享物 = [`docs/api-contract.md`](../api-contract.md)）。保留本文作完整决策与平台调研依据。

> 日期：2026-08-23
> 起因：评估"把引擎做成笔记插件"——需要做什么工作？什么形式更合适？多平台怎么解？
> 结论：**插件薄壳，两个独立新仓库，引擎零改动；不做 TS/WASM 移植（拒绝双重维护）；移动端坦诚不支持；MCP 提前。**
> 关联：[product-form.md](product-form.md)（产品形态分层）· [architecture/07-mcp.md](../architecture/07-mcp.md)（MCP 研究稿）· [roadmap.md](../roadmap.md)（引擎路线图）

---

## 一、背景与三个问题

用户提出三个问题：

1. 后续把引擎做成笔记插件需要做什么工作？
2. 什么样的形式更合适？
3. 如何解决多平台的问题？

本文是这轮调研的结论与最终决策记录。**执行时序**：先完善引擎核心（见 `docs/roadmap.md`），插件化是远期动作——本文先定方向，避免届时重新调研。

## 二、平台能力调研（基于官方文档与源码，2026-08-23）

| 维度 | 引擎现状 | Obsidian 插件 | 虎鲸 Orca Note 插件 |
|---|---|---|---|
| 运行时 | Go 单二进制（seren.exe） | 桌面：Electron renderer；移动：Capacitor webview | 渲染进程（React + Valtio） |
| 语言/构建 | Go | TypeScript → esbuild → main.js + manifest.json | TypeScript → Vite |
| 能否跑 Go 代码 | — | ❌（除非 WASM 或 sidecar） | ❌（纯前端，仅 `orca.invokeBackend` IPC） |
| Node API | — | ✅ 桌面可用（fs / child_process / net，可 spawn 进程、连 localhost）；❌ 移动端完全无 | ❌ 无进程能力 |
| 图数据来源 | 自解析 vault 文件 / 直读虎鲸 SQLite | Obsidian 自带（MetadataCache：wikilink/tag/frontmatter 已解析） | 官方后端块级图库（get-blocks-with-tags / get-ref-tos / query 等 IPC） |
| 面板/UI | Web UI（REST + 前端，go:embed） | ItemView 侧边栏/标签页 | ViewPanel（nav.addTo / goTo） |
| 分发 | 单二进制 | 社区插件目录（tag 发布 + 自动扫描 + 审核） | zip 解压到 `Documents/orca/plugins` |
| 官方 MCP | 无（v3 计划中） | — | ✅ 已有（orca-note-cli，Streamable HTTP + Bearer），可互操作 |

**关键事实：**

- **Obsidian 桌面是唯一允许"插件 + sidecar 进程"的平台**（Node API 可用），先例：[Local REST API](https://github.com/coddingtonbear/obsidian-local-rest-api)、[obsidian-ai](https://github.com/spencermarx/obsidian-ai) 均以插件拉起本地服务。
- Obsidian 安全模型（[Plugin security](https://help.obsidian.md/Extending+Obsidian/Plugin+security)）：Restricted mode = 整体禁用社区插件；开启后插件继承 Obsidian 权限（读文件 / 联网 / 装程序）。
- Obsidian 移动端（[Mobile development](https://docs.obsidian.md/Plugins/Getting+started/Mobile+development)）：**无 Node / Electron API**，纯浏览器环境；`isDesktopOnly: true` 可让移动端直接不可安装。
- 虎鲸插件为纯前端壳，只能通过 `invokeBackend` 与官方后端通信，**不能跑引擎代码**。
- 引擎现状核对：`serve` 已绑定 `127.0.0.1`（cmd/seren/main.go）；`/api/stats` 已返回 `version` + `revision`；**尚无 token 鉴权**（薄壳上线前置项）。

## 三、三条路线对比

| 路线 | 描述 | 引擎改动 | 移动端 | 维护负担 | 结论 |
|---|---|---|---|---|---|
| **① 插件薄壳** | 面板 iframe 指向引擎自服务的 Web UI（本地 REST） | 零改动 | ❌ | 极低（插件只是客户端） | ✅ **采用** |
| ② TS 移植 | 把 graph/score/roam 翻译成 TS，无 sidecar | 无（但算法双实现） | ✅ | 高（Go/TS 双维护、漂移风险） | ❌ 不做 |
| ③ Go → WASM | 编译引擎核心为 wasm 嵌入插件 | 需 IO 抽象 | ✅ | 最高（wasm 胶水 + 体积 + 审核） | ❌ 不做 |

**薄壳的关键设计**：插件**不搬运、不打包任何前端**——引擎的 `serve` 本来就服务 Web UI，插件面板直接 iframe `http://127.0.0.1:PORT/`。UI 永远与引擎版本一致，彻底消灭前端双重维护。

## 四、决策：不做移植（含移动端坦诚）

- **不做 ②③**：双重维护风险 > 收益。引擎核心（Go）是唯一实现，保持单一事实源。
- **移动端做不了就坦诚说明**：
  - Obsidian `manifest.json` 设 `"isDesktopOnly": true` → 移动端用户根本看不到、装不上（官方机制，非遮羞布）。
  - 插件 README 第一行声明"仅桌面端；移动端不做，欢迎 fork 移植"。
- **为他人移植铺路**：独立仓库 + MIT 协议 + 清晰的 API 契约文档——未来若有人觉得有价值，可在薄壳仓库基础上做 TS/WASM 移植，不触碰引擎本体。

## 五、最终决策（拍板清单）

| # | 决策 |
|---|---|
| D1 | **形态 = 插件薄壳**：面板 iframe 引擎自服务的 Web UI；引擎零代码改动；插件只做"发现引擎 → iframe → 补齐跳回与刷新" |
| D2 | **不做 TS/WASM 移植**，拒绝双重维护；引擎核心（Go）保持单一实现 |
| D3 | **移动端不支持**：isDesktopOnly + README 坦诚声明，欢迎 fork |
| D4 | **仓库 = 两个独立新仓库**：`serendipity-obsidian` / `serendipity-orca`；与主引擎为**运行时契约引用、零构建时依赖**（不 submodule、不 vendor） |
| D5 | **唯一共享物 = API 契约**：引擎侧 `docs/api-contract.md`（7 端点 + version/revision）；插件侧放一份手写副本 `seren-api.d.ts`（文件头注明以契约为准） |
| D6 | **版本兼容策略**：插件连接时调 `/api/stats` 比对 `version`，不匹配弹"请升级引擎到 vX.Y.Z"；不引入版本协商协议 |
| D7 | **时机 = 远期**：先完善引擎核心（roadmap M0/M1），插件化（M2）在其后 |
| D8 | **MCP 提前**：v3 与引擎安全前置（token 鉴权等）同批推进，作为下一步（roadmap M0） |

## 六、插件薄壳的工作清单（远期执行，最小化）

**前置（引擎侧，已列入 roadmap M0）**：serve token 鉴权 + Host 头校验；`docs/api-contract.md`。

**serendipity-obsidian**（照 obsidian-sample-plugin 骨架）：
- `manifest.json`（id 全局唯一，如 `serendipity-roam`；`isDesktopOnly: true`）、`main.ts`、`styles.css`
- 注册侧边栏 ItemView → iframe 指向本地 serve（端口可配）
- 探测 / 自动拉起 seren.exe（设置里可配路径；onunload 不杀用户进程）
- 跳回：`app.workspace.openLinkText()` 就地打开笔记（替代 obsidian:// URI 跳出去）
- 刷新联动：`vault.on('create'/'modify'/'rename')` 节流触发 `POST /api/refresh`
- 版本提示：`/api/stats.version` 与最低版本比对
- 发布：git tag + GitHub Actions 构建 main.js 挂 release（社区目录标准流程）

**serendipity-orca**（照 orca-simple-task 模板）：
- React + Valtio + Vite；`orca.nav` 加 ViewPanel 嵌同一份 iframe
- 跳回：`orca.invokeBackend` 打开块
- 发布：build 后 zip 解压到 `Documents/orca/plugins`

## 七、多平台策略

拆成两个维度分别处理：

**客户端平台（Win / macOS / Linux / iOS / Android）**
- 总纲：分层解耦——Go 内核（任何平台可编译）× REST 契约 × 浏览器级 Web UI（任何壳可嵌），插件永远是壳。
- 桌面三平台：薄壳全通（Obsidian 桌面可拉起 seren；虎鲸连本地 REST）。
- 移动端：**明确不做**（无 sidecar 进程，唯一干净解是 TS 移植，已否决）。"远程模式"（seren 跑在桌面/服务器、移动端连远程 REST）踩"个人数据不出本机"红线，默认不做，除非用户显式要求。

**笔记软件生态（Obsidian / 虎鲸 / 未来）**
- 引擎天然双数据源；插件按数据源各做一薄壳，共享同一份 Web UI 与 API 契约。
- 数据一致性：多设备同步下 SQLite store 随 vault 一起同步，现有"快照增量解析"策略可平移。
- 远期：MCP 通道让 AI 层跨软件；插件只服务人类用户。

## 八、风险与边界

- **Obsidian 社区审核**：spawn 外部进程 + 监听端口的插件需在 README/审核中写清用途（Local REST API 过审先例，可行但要认真对待）；大体积/性能遵守 [Optimize plugin load time](https://docs.obsidian.md/Plugins/Guides/Optimize+plugin+load+time)。
- **虎鲸生态小**（orca-note 约 200 stars，插件模板单一）：虎鲸薄壳的价值取决于用户是否真在虎鲸里工作——先做 Obsidian，虎鲸次之。
- **隐私红线**：任何"远程/云"形态需显式用户同意，默认不开。
- **依赖极简卖点**：插件壳必然引入 node 生态（esbuild / react），但只影响壳，不影响 Go 内核（内核仍为零第三方依赖 + 无网络出口）。
- **契约漂移**：REST 契约已稳定（stats/hot/roam/relation/config/refresh/touch），副本 `seren-api.d.ts` 漂移风险低；引擎侧改 API 时须同步契约文档（列入维护指南）。

## 九、相关文档

- [product-form.md](product-form.md)：产品形态分层（跳回软件 > 插件薄壳 > MCP）——本文是其"② 插件薄壳"的细化与拍板
- [roadmap.md](../roadmap.md)：总路线图（阶段 1 引擎核心+Web UI / 2 插件薄壳 M2）
- [architecture/07-mcp.md](../architecture/07-mcp.md)：MCP 架构研究稿（seren mcp 子命令、只读三件套）
- [architecture/00-overview.md](../architecture/00-overview.md)：架构总览
