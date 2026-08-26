# 插件化调研与决策：Obsidian / 虎鲸薄壳（不做移植）

> ⚠️ **历史记录（已归档）**：本文是 2026-08-23「插件化」调研与拍板记录（D1–D8 + 薄壳工作清单）。决策已由 [`docs/roadmap.md`](../roadmap.md) M2 承接（两个独立仓库、零构建时依赖、唯一共享物 = [`docs/api-contract.md`](../api-contract.md)）。保留本文作完整决策与平台调研依据。
> **〔2026-08-26 更新〕虎鲸插件已暂停开发**（生态小、壳收益低；内核直读虎鲸库照常可用，等于用内核完成插件功能）——M2 收敛为 Obsidian 单壳。本文虎鲸调研内容保留作溯源，执行口径以 [`plugin-dev-plan.md`](../plugin-dev-plan.md) 头部暂停声明为准。

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

### 插件形态升级：隐式优先三层（2026-08-24 用户拍板）

> 用户对插件形态的思考：用户多数时候"开着插件但自己在操作"，**不打开插件面板**——插件应是无感存在的工具，**隐式触发（宿主内操作）比显式面板漫游更重要**。

| 层 | 内容 | 交互 |
|---|---|---|
| **后台感知层** | 监听宿主操作 → 隐式 touch → 自动 refresh（现有 watch） | 零交互（无感） |
| **命令触发层** | 命令面板/快捷键/右键/斜杠命令，**锚点 = 当前活跃笔记** | 轻交互（不打开面板） |
| **可选面板层** | iframe 壳 Web UI（现有设计保留，深度漫游用） | 低频 |

**关键设计**：命令锚点 = 用户正在看的笔记（Obsidian `activeFile` / 虎鲸当前块），无需输入——"无感使用"的核心交互。面板层代码不浪费（保留），但插件主价值在后台感知 + 命令触发。

### touch 数据真实化（显式 + 隐式）

| touch 来源 | 机制 | 工作量 |
|---|---|---|
| **显式**（seren 界面内漫游） | iframe 壳内 Web UI 埋点，现有 `/api/touch` 原样生效 | **零工作** |
| **隐式**（宿主内日常操作，主信号） | Obsidian：`workspace.on('file-open' / 'active-leaf-change' / 'editor-change')` → `POST /api/touch`；虎鲸：Valtio `subscribe(orca.state.activePanel / panels / blocks)`（模板先例：main.ts `const { subscribe } = window.Valtio`） | 插件实现 |

红线保持：隐式 touch 与显式同表、**只记录不演化**（v0.1.4 决策；是否进边权演化是 M1 边权演化时再议）。

### 宿主命令注册（隐式优先的主入口）

- **Obsidian**：`addCommand`（回调读 `activeFile` 作锚点）+ `workspace.on('file-menu')` 右键菜单 + `addStatusBarItem` 状态栏无感提示
- **虎鲸**（orca.d.ts 已确认）：`commands.registerCommand` / `slashCommands.registerSlashCommand` / `toolbar.registerToolbarButton` / `headbar.registerHeadbarButton`（`blockMenuCommands` / `tagMenuCommands` 在 2026-08-25 模板 d.ts 片段未见，列为待核实）

### 虎鲸待确认点（开发时问开发者本人）

1. `subscribe` 大对象（blocks/panels）的性能 vs 只 subscribe `activePanel`
2. "当前活跃块"的现成获取方式（invokeBackend 无明确 get-active-block 消息；需从 activePanel + panels 解析？）
3. `broadcasts` 是否承载宿主事件（若是，比订阅 state 更轻）

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

## 九、Orca 插件 API 深读与开发难度评估（2026-08-25 补充）

> 来源：Orca Note 官方插件文档 + 官方模板仓库类型定义。本节能补 §二/§六 的"虎鲸纯前端壳"结论，并**纠正此前"虎鲸无面板 API、只能块内 iframe"的保守判断**——`orca.panels.registerPanel` 与 `orca.nav.goTo` 真实存在。

### 9.1 官方文档链接（采集于 2026-08-25，含全站 modules 深读）
> ⚠️ **权威来源说明**：Quick Start 文档**不完整**（仅展示 commands / toolbar / headbar / block-renderer 四类 API），且未含 `panels` / `nav` / `broadcasts` / `state` 全字段。**完整、权威的 API 面是 `modules.html` 类型参考 + `orca.d.ts` 类型定义**，以下以类型定义为准。

- 插件开发 Quick Start（入门，不完整）：https://www.orca-studio.com/orcanote-docs/documents/Quick_Start.html
- **API 类型参考总目录（权威）**：https://www.orca-studio.com/orcanote-docs/modules.html
- 官方插件模板仓库：https://github.com/sethyuan/orca-plugin-template
  - `orca.d.ts`（插件 API 类型定义 / 事实上的 API 参考）：https://raw.githubusercontent.com/sethyuan/orca-plugin-template/main/src/orca.d.ts
  - `plugin-docs/` 文档源目录：https://github.com/sethyuan/orca-plugin-template/tree/main/plugin-docs
- **关键类型页（深读确认）**：
  - `Orca` 根接口（24 个命名空间全签名）：https://www.orca-studio.com/orcanote-docs/interfaces/types_orca.Orca.html
  - `Plugin` 生命周期接口：https://www.orca-studio.com/orcanote-docs/interfaces/types_orca.Plugin.html
  - `Block` 数据模型（refs/backRefs/aliases 等）：https://www.orca-studio.com/orcanote-docs/interfaces/types_orca.Block.html
  - `PanelProps`（面板渲染器入参，含 viewArgs）：https://www.orca-studio.com/orcanote-docs/types/types_orca.PanelProps.html
- 模板构建：Vite + TypeScript；`peerDependencies` 仅 `react@^18.2.0` / `valtio@^1.13.2`；编译产物 `dist/index.js` + `icon.png` + `package.json` 解压到 `Documents/orca/plugins`。

### 9.2 完整 API 表面（权威，来自 modules.html + orca.d.ts 类型定义）

> 全局 `orca` 对象 = 24 个命名空间 + 1 个根方法 `invokeBackend`。**`orca` 是全局对象（`window.orca`），不是 `load()` 的参数**——插件入口只导出 `load(pluginName)` / `unload()`，在模块内直接引用全局 `orca`。

| 命名空间 | 关键方法（逐字签名见 9.2.1） | 我们能用什么 |
|---|---|---|
| `orca.state` | **只读普通对象，无方法**（无 `getState`/`subscribe`/`refs` 函数）。字段：`locale` / `themeMode`("light"\|"dark") / `repo` / `repoDir` / `dataDir` / `activePanel` / `panels`(RowPanel) / `panelBackHistory` / `panelForwardHistory` / `blocks[id]`(Block) / `plugins[name].settings` / `notifications` / 各类注册表 |`blocks[id]` 含 `refs`/`backRefs`/`aliases`/`children`/`parent`/`text`/`content`（见 9.2.1）；`activePanel`=当前面板 id；设置经 `plugins[name].settings` 读。**响应式靠 `useSnapshot(orca.state)`（window.Valtio），非 state.subscribe()** |
| `orca.commands` | `registerCommand(id, fn, label)` / `registerEditorCommand` / `invokeCommand` / `registerBeforeCommand` / `registerAfterCommand` | 注册命令（含 `/serendipity` 斜杠锚点）；绑定工具栏/斜杠 |
| `orca.slashCommands` | `registerSlashCommand(id, command)` / `unregisterSlashCommand` | 斜杠命令 |
| `orca.toolbar` | `registerToolbarButton(id, button\|button[])` | 工具栏按钮（`{icon,tooltip,command}`） |
| `orca.headbar` | `registerHeadbarButton(id, () => ReactElement)` | 头部栏按钮（返回 React 组件） |
| `orca.renderers` | `registerBlock(type, isEditable, renderer, opts?)` / `registerInline` | 自定义块渲染器（iframe 内嵌引擎 UI 的**备选**路径）；`opts` 含 `assetFields`/`foldInQuery`/`useChildren` |
| `orca.converters` | `registerBlock` / `registerInline` | 块/行内格式转换 |
| `orca.panels` | **`registerPanel(type, renderer)`** / `unregisterPanel` | **注册专属视图面板（D1 薄壳主路径）**；renderer 收 `PanelProps`（含 `viewArgs`，见 9.2.1） |
| `orca.nav` | **`goTo(view, viewArgs?, panelId?)`** / `openInLastPanel` / `replace` / `switchFocusTo` / `findViewPanel(id, panels)` / `goBack` / `goForward` / `close` / `addTo` | **打开面板 / 按 id 跳块 `goTo("block",{blockId})`**；`findViewPanel` 配 `state.panels` 定位已有面板 |
| `orca.themes` | `injectCSS(css, role)` / `injectCSSResource(url, role)` / `register` / `unregister` | **样式注入**（面板内可接管引擎 UI 主题，比纯 iframe 更融合） |
| `orca.plugins` | `setData` / `getData` / `removeData` / `setSettingsSchema` / **`writeFile` / `readFile` / `existsFile` / `listFiles` / `removeFile` / `removeFolder`(均支持 `pluginAsRoot`)** / `clearData` / `load` / `unload` / `enable` / `disable` | 配置持久化（corePath/port/coreManagement）+ 设置 schema；**文件读写意味着插件可把引擎二进制下载进自身数据目录（auto-download 在虎鲸端也技术可行，仅启动仍靠用户）** |
| `orca.notify` | `notify(type, msg, opts?)`（`type`=info/success/warn/error） | 提示（连接失败引导等） |
| `orca.broadcasts` | **`broadcast(type, ...args)`** / `registerHandler(type, handler)` / `unregisterHandler` / `isHandlerRegistered` | **订阅= `registerHandler(type, handler)`**（无 subscribe/publish 命名）。候选 host 事件通道（如导航/活跃块变更）→ **`type` 字符串集待核实** |
| `orca.shortcuts` | **`assign(shortcut, command)`** / `reset(command)` / `reload()` | **无 `registerShortcut`**；绑定热键 = `assign("mod+shift+s", "<plugin>.openPanel")` |
| `orca.blockMenuCommands` | `registerBlockMenuCommand(id, command)` / `unregisterBlockMenuCommand` | 块右键菜单命令（**已确认存在**，此前误判待核实） |
| `orca.tagMenuCommands` | `registerTagMenuCommand(id, command)` / `unregisterTagMenuCommand` | 标签菜单命令（**已确认存在**） |
| `orca.editorSidetools` | `registerEditorSidetool(id, tool)` / `unregisterEditorSidetool` | 编辑器侧栏工具 |
| `orca.contexts` | `BlockEditorContext`({active, editor, panelId, **rootBlockId**}) / `ImageViewerContext` / `ZContext` | **取"当前正在查看的块"的正确方式 = `BlockEditorContext.rootBlockId`**（隐式 touch 信号源，优于轮询 state） |
| `orca.utils` | `showBlockPreview(blockId, refElement?, rect?, interactive?, blockEditorActive?)` / `getAssetPath` / `getCursorDataFromRange` / `setSelectionFromCursorData` | **块预览弹窗**（漫游结果 hover 预览）、光标处理 |
| `orca.components` | 大型 React 组件库（`Block` / `Button` / `ModalOverlay` / …） | 原生面板 UI 构建块（不强制 iframe，可混合原生组件） |
| `orca.ai` | `sendMessage(messages, opts?)` / `sendStreamMessage(messages, controller, opts?)` | **内置 AI 对话 API**（意外收获；可呼应"用户批量喂 AI 判链接"的生态位设想，但非核心需求） |
| `orca.invokeBackend` | **`invokeBackend(type: string, ...args: any[]): Promise<any>`** | **虎鲸自家后端 IPC**（`get-block` 等，`type` 为字符串），非我们的核心、不涉进程拉起；可取当前块引用做上下文，但引擎已从 .db 解析完整图，此项可选 |

> 全局可用：`window.React`（18）、`window.Valtio`（`useSnapshot` / `subscribe`）。生命周期钩子：入口 `export async function load(pluginName: string)`（Enable 阶段）+ `export async function unload()`（Disable 阶段）；另有 Loading 阶段（发现并解析元数据，未启用）。**`orca` 为全局，不在 load 参数里。**

#### 9.2.1 Orca 根接口关键签名（逐字，modules 权威参考）
```typescript
// state：纯数据，无方法
state: {
  locale: string; themeMode: "light" | "dark";
  repo: string; repoDir?: string; dataDir: string;
  activePanel: string;
  panels: RowPanel;                       // 面板树
  panelBackHistory: PanelHistory[]; panelForwardHistory: PanelHistory[];
  blocks: Record<string | DbId, Block | undefined>;   // 含 refs/backRefs/aliases
  plugins: Record<string, Plugin | undefined>;
  notifications: Notification[];
  // …各类命令/渲染器/按钮注册表
};

// panels：仅注册/注销，无 openPanel（打开由 nav 负责）
panels: { registerPanel(type: string, renderer: any): void; unregisterPanel(type: string): void; };

// nav：打开/跳块/聚焦
nav: {
  goTo(view: string, viewArgs?: Record<string, any>, panelId?: string): void;  // 跳块: goTo("block",{blockId})
  openInLastPanel(view: string, viewArgs?: Record<string, any>): void;
  replace(view: string, viewArgs?: Record<string, any>, panelId?: string): void;
  findViewPanel(id: string, panels: RowPanel): ViewPanel;
  switchFocusTo(id: string): void; goBack(opts?): void; goForward(opts?): void;
  close(id: string): void; addTo(id, dir, src?): string;
};

// plugins：持久化 + 文件读写
plugins: {
  setData(name, key, value: string|number|ArrayBuffer): Promise<void>;
  getData(name, key): Promise<any>; removeData(name, key): Promise<void>;
  setSettingsSchema(name, schema: PluginSettingsSchema): Promise<void>;  // 无 getSettingsSchema
  writeFile(name, filePath, data: string|ArrayBuffer, pluginAsRoot?: boolean): Promise<void>;
  readFile(name, filePath, type?: "string"|"buffer", pluginAsRoot?: boolean): Promise<string|ArrayBuffer>;
  existsFile(name, filePath, pluginAsRoot?: boolean): Promise<boolean>;
  listFiles(name, pluginAsRoot?: boolean): Promise<string[]>;
  removeFile / removeFolder(name, path, pluginAsRoot?): Promise<void>;
};

// broadcasts：订阅=registerHandler
broadcasts: { broadcast(type: string, ...args: any[]): void;
  registerHandler(type: string, handler: CommandFn): void;
  unregisterHandler(type: string, handler: CommandFn): void;
  isHandlerRegistered(type: string): boolean; };

// shortcuts：无 registerShortcut
shortcuts: { assign(shortcut: string, command: string): Promise<void>;
  reset(command: string): Promise<void>; reload(): Promise<void>; };

// 根方法
invokeBackend(type: string, ...args: any[]): Promise<any>;
```

#### 9.2.2 Block 数据模型（隐式 touch / 跳回的字段基础）
```typescript
interface Block {
  id: number;
  aliases: string[];                                  // 别名（= 我们引擎的 title 候选）
  refs: BlockRef[];                                   // 出链（= 引擎 edges 来源）
  backRefs: BlockRef[];                               // 入链（backlinks）
  children: number[]; parent?: number; left?: number;
  text?: string; content?: ContentFragment[];         // 纯文本 / 富文本
  created: Date; modified: Date;
  properties: BlockProperty[];
}
```
→ 与引擎 Orca adapter 模型完全对齐（title↔alias[0]、refs↔出链、backRefs↔backlinks）；插件读 `state.blocks[id]` 即可拿实时图，跳回用 `nav.goTo("block",{blockId})`。

### 9.3 关键修正（纠正此前保守判断 + 全站深读结论）
0. **权威来源是 modules.html + orca.d.ts，不是 Quick Start**：Quick Start 仅展示 commands/toolbar/headbar/block-renderer 四类，会让人误以为"虎鲸只有这些"——实际 `panels`/`nav`/`broadcasts`/`state` 全字段都在类型定义里。所有 API 判断以 9.2 为准。
1. **虎鲸有面板 API**：`orca.panels.registerPanel(type, renderer)` + `orca.nav.goTo(type)` 可开专属视图面板——薄壳主路径应是"注册 `serendipity.board` 面板 + 工具栏/斜杠命令 `goTo` 打开"，而非只能把 iframe 塞进块渲染器（块内 iframe 仍是备选）。
2. **跳回块有原生 API**：`orca.nav.goTo("block", { blockId })` 按 id 打开任意块——漫游结果点击回跳的缺口已补上；§六 原"orca.invokeBackend 打开块"写法**错误**，应为 nav.goTo。
3. **invokeBackend 是虎鲸自家后端**，不是我们的核心、也不涉进程拉起——"虎鲸无进程能力"结论不变。
4. **生命周期签名精确化**：入口是 `export async function load(pluginName: string)` / `export async function unload()`，**`orca` 是全局对象（`window.orca`）而非 load 参数**——插件内直接引用全局 `orca` 即可（此前文中 "load(Enable)" 措辞不精确，以此为准）。

### 9.4 生命周期 ↔ 我们的状态机（修正）
虎鲸生命周期 = Loading（已安装未启用）→ Enable（`load` 调用）→ Disable（`unload`）。映射到我们的四态：
- **INSTALLED** = Loading（插件在 `orca/plugins`，未启用）
- **CONFIGURED** = `load` 时读取 `orca.state.plugins[name].settings` 判定 core host/port 已填
- **RUNNING** = `load` 探测 `GET /api/stats` 通 → 注册面板/命令，iframe 加载引擎 UI
- **CORE_STOPPED（插件仍启用，子状态）** = 探测不通 → 面板/命令仍注册，但 iframe 区显示"未检测到引擎"+ 下载链接 + 启动命令；提供"重试连接"命令。用户自行启动核心后重连。
- **DISABLED** = `unload`（用户在宿主内禁用插件）→ 注销全部。

> 对虎鲸，**`coreManagement` 强制 `external`**：`load` 不拉起进程、`unload` 不杀进程（物理不能）。所以"二次确认启动服务"在虎鲸语境 = "启用插件时披露：本插件将连接本机 localhost 服务（该服务读取你的笔记），数据不出本机，可随时禁用"；真正启动核心由用户用 `seren serve` / launchd / 系统服务完成。

### 9.5 分发与生命周期（含 Q1–Q4 结论落定）
- **Q1 多平台**：Go 交叉编译近乎零成本，goreleaser + GitHub Actions 一次出 win-amd64 / mac-amd64 / mac-arm64 / linux-amd64 四 asset（含校验和、自动挂 release）。**linux 不放弃、不作为"自己编译"主路径**；仅非 x86 架构才兜底推荐自编译。
- **Q2 核心分发**：虎鲸 external 模式**只需 host/port、不需要 corePath**（插件不启动它）。分发分两 tier：
  - **v1 主路径**：插件 README + 设置页链 GitHub release，用户自下载自运行（审查友好、零二进制进插件，契合 D4 纯壳）。
  - **v1.x 增强**：设置内"下载核心"按钮——取 latest release → 按平台选 asset → `fetch` 二进制 → `orca.plugins.writeFile(name, "seren", buf, pluginAsRoot)` 落进插件数据目录。**全站深读后确认：连虎鲸端 `plugins.writeFile` 都支持 ArrayBuffer 写入，自动下载在技术上是可行的**（Obsidian 端同理用 `fs`）；唯"启动核心"仍靠用户（`seren serve`/launchd），这是 external 模式不可逾越的物理边界。
- **Q3 依赖体现**：三个独立仓库（`serendipity-engine` / `serendipity-obsidian` / `serendipity-orca`），零构建时依赖；唯一共享物 = `docs/api-contract.md`（插件侧副本 `seren-api.d.ts`）；运行时 `GET /api/stats` 比对 `version`（D6）。GitHub 上引擎 repo 出二进制 release，插件 repo README 首行写 `requires serendipity-engine ≥ vX.Y`。
- **Q4 生命周期**：四态机 + `coreManagement` 双模式（Obsidian=managed 可 spawn；虎鲸=external 只连）；二次确认弹窗披露隐私四点（读笔记 / 本机服务 / 数据不出本机 / 可停用）；停用后前端变原生壳（不发 `/api/*`，配置保留）。

### 9.6 开发难度评估（已并入开发计划）
> 跨平台难度对比表与总判断已并入 [`docs/plugin-dev-plan.md` §三.3](plugin-dev-plan.md)（可执行开发计划文档）。本文保留原始 API 逐字签名（9.2）与待确认点（9.7）作为调研证据，难度评估以计划文档为准。

### 9.7 待确认点（更新，替换 §六 旧清单）
> 全站 modules 深读后，原 §六 多数"是否存在"类疑问已**确认**（panels/nav/broadcasts/state 全字段、blockMenuCommands/tagMenuCommands、shortcuts.assign、invokeBackend 字符串 type 等均在类型定义中）。剩余待核实项如下：

1. **（已可解）隐式 touch 信号源**：优先用 `orca.contexts.BlockEditorContext.rootBlockId`（编辑器内取当前块，最干净）；或用 `useSnapshot(orca.state)` 响应 `state` 变化重渲染。`orca.state` 本身**无 `subscribe()` 方法**，勿误用。
2. **`broadcasts` 的 `type` 字符串集**：订阅 API 已确认为 `registerHandler(type, handler)`，但 Orca 具体广播哪些事件（如导航/活跃块变更/文件增改）未知 → **待核实**（若承载导航事件，隐式 touch 可走更轻的 broadcast 而非轮询 state）。
3. **设置 schema 类型集**：Quick Start 仅示 `boolean`/`string`；`number`（port）、枚举/下拉（`coreManagement`: managed/external）是否支持 → **待核实**（影响设置页实现方式）。
4. **`PanelProps` 精确形状**：文档页未干净提取；已知 `nav.goTo(view, viewArgs?)` 把 `viewArgs` 传入面板渲染器，`registerPanel(type, renderer)` 的 renderer 收 `PanelProps`（含 `viewArgs`/`history` 等）→ **待核实渲染器入参字段名**（决定我们如何把漫游结果 blockId 传进面板）。
5. **`invokeBackend` 支持的 `type` 字符串**（如 `get-block` / `get-blocks-with-tags`）：若要在插件端实时取当前块引用做上下文，需确认可用 type → **待核实**（非核心，引擎已从 .db 解析完整图）。
6. **`shortcuts.assign` 命令格式**：已确认签名 `assign(shortcut, command)`；`command` 是否接受我们 `registerCommand` 的 `id`（含插件名前缀）及快捷键语法（如 `mod+shift+s`）→ **待核实**（影响热键绑定实现）。

## 十、相关文档

- [product-form.md](product-form.md)：产品形态分层（跳回软件 > 插件薄壳 > MCP）——本文是其"② 插件薄壳"的细化与拍板
- [roadmap.md](../roadmap.md)：总路线图（阶段 1 引擎核心+Web UI / 2 插件薄壳 M2）
- [architecture/07-mcp.md](../architecture/07-mcp.md)：MCP 架构研究稿（seren mcp 子命令、只读三件套）
- [architecture/00-overview.md](../architecture/00-overview.md)：架构总览
