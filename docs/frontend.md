# Serendipity Engine · 前端计划（Web UI）

> 日期：2026-08-23（由外部审计前端路线图定稿并汇入仓库）
> 来源：与用户讨论（2026-08-23）+ `internal/web/static/index.html` 与 [`docs/history/plugin-evaluation.md`](history/plugin-evaluation.md) 评审
> 性质：**前端做什么**——让 Web UI 从「漫游工具」升级为「阅读 + 漫游工具」，并为 Obsidian 插件薄壳（[`docs/roadmap.md`](roadmap.md) M2；~~虎鲸插件已暂停，2026-08-26~~）铺路。
> 相关：战略定位 [`docs/positioning.md`](positioning.md) · 后端机会 [`docs/backend-backlog.md`](backend-backlog.md) · 前端源码 `internal/web/static/index.html`（单文件，零依赖原生 JS，go:embed 嵌入）。

> 目标：让 Web UI 从「漫游工具」升级为「阅读 + 漫游工具」，并为 Obsidian 插件薄壳铺路（~~虎鲸插件已暂停~~）。

## 一、现状核对（v0.1.12，internal/web/static/index.html）

已有功能：搜索漫游 / 🎲 随机漫步（升级带文字主按钮）/ 关系查询 / **相似查询（Adamic-Adar）** / **节点详情预览** / **漫游导出** / **反馈统计（只读）** / 参数侧滑抽屉（白盒）/ 卡片续漫游 + 历史栈 / 打开跳转（obsidian:// 与 orca-note:// 与 **postMessage 桥**）/ touch 埋点 + 幽灵过滤 / 自动监听提示 + **is_pending 提示条** / 对账刷新 / **紧凑嵌入 `?embed=1`** / **i18n 中英双语全部文案**。
技术形态：零依赖原生 JS，单 HTML，CSS 变量隔离（Tokyo Night 暗色 + light 变量主题跟随），token 服务端注入（iframe 天然兼容）。

## 二、P0 · 插件化前置（进 Obsidian 前必须；~~虎鲸插件暂停~~）—— ✅ v0.1.12 全部落地

| # | 功能 | 说明 | 落点 |
|---|---|---|---|
| 1 | **紧凑嵌入模式** | 当前 920px 全宽 + hero 56vh，窄面板（300-500px）直接崩。`?embed=1` 或 iframe 检测：隐藏 hero、压缩卡片密度、搜索框常驻 | ✅ v0.1.12：`?embed=1` 或 `top!==self` → body.embed（hero/brand/页脚隐藏、搜索 sticky、卡片收紧、.open 常显） |
| 2 | **postMessage 桥** | 插件场景「打开」不跳外链，postMessage 通知宿主就地打开（Obsidian openLinkText / ~~虎鲸 invokeBackend，暂停~~）。`top !== self` 时自动启用 | ✅ v0.1.12：嵌入时 `window.parent.postMessage({type:'open',id})`；宿主注入 `{type:'theme'}/{type:'locale'}/{type:'activeFile'}` |
| 3 | **节点详情预览** | 点卡片先看内容再决定深入。需 `/api/node?id=`（Text 摘要 + 邻居列表）+ 卡片「预览」按钮 | ✅ v0.1.11（`/api/node` + 卡片预览浮层） |
| 4 | **多语言（中英双语，覆盖全部用户可见文案）** | Obsidian 社区是国际平台；前端文案目前全硬编码中文。抽 i18n（文案集中一份文件），早期做比后期便宜。**范围：所有用户可见文案一律中英双语**——按钮/标签/提示条/toast/刷新摘要/空状态/错误提示/加载态，禁止硬编码中文字符串。语言跟随宿主（Obsidian navigator.language / ~~虎鲸 `orca.state.locale`，暂停~~，默认中文，英文兜底） | ✅ v0.1.12：集中 `I18N` 字典（zh/en），`t(key)` 取值；`applyStaticI18n()` 填充 data-i18n；语言跟随 postMessage `{type:'locale'}` > navigator.language（zh→中文，其他→英文兜底） |

## 三、P0.5 · 近期改掉：hero 浮游气泡 —— ✅ v0.1.12 已落地

- 现状：初始页 56vh 随机浮动气泡（/api/hot），动画 + 随机位置——**插件场景是纯粹负面资产**（窄面板装不下、视觉喧宾夺主、信息密度低）。
- 方向：改掉动画随机漂浮，替换为**静态热门节点列表 / 标签云**（信息密度优先，保留"点它开始漫游"的入口）。✅ v0.1.12：hero 改静态热门 tags（气泡 → 静态 `.bubble` 胶囊，移除 floaty 动画），嵌入时直接隐藏 hero-title/sub 聚焦搜索框。

## 四、P1 · 易用性高杠杆（独立使用 + 插件共用）

| # | 功能 | 价值 |
|---|---|---|
| 5 | 命中片段高亮 | 全文降级只显示「命中 N 次」，用户不知道为什么命中。返回 snippet + 高亮命中词 |
| 6 | 卡片操作菜单 | 右键/长按：复制 ID、查看邻居（图中连着什么）、快捷填入关系查询 from/to |
| 7 | 主题跟随宿主 | Obsidian 浅/深色两套，CSS 变量已隔离，加 light 变量即可 |

## 五、P2 · 打磨（可后置）

| # | 功能 | 说明 |
|---|---|---|
| 8 | URL 状态扩展 | 目前只有 `?q=`；随机 seed、参数值不进 URL → 无法分享/收藏特定漫游 |
| 9 | 键盘导航 | `/` 聚焦搜索、`Esc` 关闭面板、方向键选卡片 |
| 10 | 历史栈持久化 | 刷新丢漫游路径（iframe 重载频繁，插件场景尤其痛），存 sessionStorage |

## 六、架构注记

1. **需要动引擎的**：#3 节点详情 API（`/api/node?id=`，登记契约）+ similar/export/touch-stats 三个端点（与 [`docs/backend-backlog.md`](backend-backlog.md) §三 同批登记）。其余纯前端。且天然契合"Web 层留口子"——语义候选将来可出现在节点详情的"相似节点"区（见 [`docs/positioning.md`](positioning.md) §五）。
2. **iframe token 已兼容**：v0.1.8 token 服务端注入页面，iframe 直接 GET / 即得，插件零透传工作。
3. 多语言尽早做（P0）：**全部用户可见文案中英双语**（含提示/错误/摘要/空状态），文案抽离后后续所有新功能直接带 key，避免二次返工。实现参考：~~虎鲸模板插件自带官方 l10n（`setupL10N(orca.state.locale, ...)`）——语言跟随宿主，默认中文、英文兜底；~~Obsidian 侧读 `navigator.language`。
4. **〔2026-08-23 借鉴〕节点详情分级 L0/L1**（OpenViking，见 [`docs/history/agent-memory-research.md`](history/agent-memory-research.md) §4.2）：L0 = summary（Text 截断），L1 = overview（摘要 + 邻居导航）——#3 节点详情 API 天然分两级（默认截断摘要、展开给邻居清单）。同源还有「确定性排序」（稳定采样，印证锚点排序需稳定 = Resolve map 序）与「簇级导航」（按 hop 分组展示 roam 结果，远期可读性方向）。

## 七、后续动作

- [ ] #3 节点详情 API 登记进 [`docs/api-contract.md`](api-contract.md)（新端点）
- [ ] hero 改静态列表（P0.5）与紧凑嵌入模式（P0-1）可在同一批前端改动中完成

## 八、后端能力 → 前端联动映射（2026-08-23 整理）

> 只保留需要前端动作的能力；后端专属（graph.search / CLI 三件套 / renames·WAL 内部修正）前端不跟进，已从本文删除。
> 原则：**能力不落地到 UI = 用户摸不到；但克制优先，能进折叠面板就不挤主界面。**

| 后端能力（来源） | 前端联动 | 状态 |
|---|---|---|
| **节点详情 API**（[`docs/backend-backlog.md`](backend-backlog.md) §六 graph.node，MCP 与 Web 同源） | 卡片「预览」按钮 + 详情浮层（Text 摘要 + 邻居 + 被引用）——**一次实现两端受益** | ✅ v0.1.11 已落地 |
| **export 漫游导出**（[`docs/backend-backlog.md`](backend-backlog.md) §3.2） | 顶栏「导出」按钮 → `?export=1` 拿 Markdown 下载；默认路径零回归 | ✅ v0.1.11 已落地 |
| **similar 结构相似**（[`docs/backend-backlog.md`](backend-backlog.md) §3.1） | 卡片「相似」按钮 + 独立折叠面板（与「关系」面板同级），展示共享邻居证据清单 | ✅ v0.1.11 落地，v0.1.12 升级 Adamic-Adar |
| **touch 统计 API**（[`docs/backend-backlog.md`](backend-backlog.md) §3.3） | 顶栏「统计」折叠面板（哪些节点被反复点击）——**只展示，绝不反馈排序**（红线 2） | ✅ v0.1.11 落地，v0.1.12 加幽灵 touch 过滤 |
| **refresh 的 renamed 字段**（v0.1.5 已有） | 刷新摘要补「改名 N」展示 | ✅ v0.1.11 已落地 |
| **communities 社区发现**（[`docs/backend-backlog.md`](backend-backlog.md) §3.4，v0.1.12） | `/api/communities`（Leiden）——诊断层展示库里主题簇（MCP graph.community + REST 同源）；前端后续可在侧滑抽屉加「社区」入口（未做，M2 酌情） | ✅ 后端 v0.1.12，前端 M2 可选 |
| **MCP 接入配置说明** | serve 页面加「AI 接入」卡片：展示 `seren mcp <vault> --db <store>` 配置 + 一键复制——onboarding AI 消费 | P1（未做） |

**MCP 配置说明（细节）**：注意 MCP 是独立子命令进程（`seren mcp`，stdio），不是 serve 的一部分——前端只能**展示配置模板供复制**，不能"开关"它（无开关意义，入口即开关，v0.1.9 已定）。配置模板由服务端提供（如 `/api/config` 返回 mcp 示例块）或前端按当前 source/vault 拼。

**联动批次**：P0（节点详情 #3 + 导出按钮）→ P1（相似面板 + 统计面板 + 刷新改名摘要 + MCP 配置卡片）。相似/统计面板复用「关系」面板的 toggle 模式。

### 多宿主兼容要求（~~Obsidian / 虎鲸插件技术栈差异的影响~~；虎鲸插件已暂停，保留宿主无关协议供未来扩展）

> 结论：**插件壳的技术栈差异（Obsidian=TS/esbuild；~~虎鲸=React+Valtio，暂停~~）对 Web UI 零影响**——都是 iframe 嵌同一份引擎自服务的 UI（[`docs/history/plugin-evaluation.md`](history/plugin-evaluation.md) D1），壳不碰 UI。真正的要求只有三条：

1. **postMessage 协议必须宿主无关**：前端只发通用消息（如 `{type:'open', id}`），Obsidian 壳用 openLinkText（~~虎鲸壳用 invokeBackend，暂停~~）实现——协议契约定在引擎侧，壳遵守同一份。
2. **窄面板真正自适应**：~~虎鲸 ViewPanel 可能比 Obsidian 侧栏更窄，~~紧凑模式不能是固定宽度，需按 iframe 实际宽度响应（P0-1 强化）。
3. **主题变量化**：宿主各有主题系统，light/dark 两套 CSS 变量是硬要求（P1-7），且深色跟随宿主而非固定 Tokyo Night。

### Web UI 宿主无关性的完整边界（2026-08-24 定稿）

> 移植性保证：**Web UI 与后端核心同等可移植**——换宿主只重写壳（~20 行），UI 零改动。
> 技术事实：iframe 跨源（localhost:端口 vs 宿主 app:// / ~~orca://~~ 协议）**浏览器强制隔离**——Web UI 无法访问父 window 的宿主 API（`app.vault` / ~~`orca.state`~~），物理上不可能绑定宿主。

**Web UI 只认识三样东西**：
1. `/api/*`（引擎 REST 契约）
2. `localStorage`（自身偏好，白名单 key）
3. `postMessage`（与壳的宿主上下文通道）

**postMessage 完整协议（壳 ↔ Web UI）**：

```
壳 → Web UI（宿主上下文注入，UI 永远不需要知道宿主是谁）：
  {type:'theme',   mode:'light'|'dark', colors?:{bg,panel,surface,text,muted,accent,border}} # 主题跟随；colors 可选，覆盖引擎 CSS token
  {type:'locale',  lang:'zh'|'en'}          # 语言跟随（Obsidian 壳读 navigator.language / ~~虎鲸壳读 orca.state.locale，暂停~~）
  {type:'activeFile', id:'xxx'}             # 命令锚点（用户当前在看哪篇）
Web UI → 壳：
  {type:'open', id:'xxx', uri?:'obsidian://…|orca-note://…'} # 打开请求（唯一上行）；uri 供宿主解码 file 路径跳回（更可靠）
```

**规则**：
- Web UI **绝不直接调宿主 API**（跨源也不允许，双保险）；需要宿主信息一律由壳注入
- 壳是**唯一**接触宿主 API 的层（Obsidian openLinkText / ~~虎鲸 invokeBackend，暂停~~），只做翻译不塞逻辑
- **壳设置保持宿主绑定**（seren.exe 路径/端口/自动启动存宿主 settings）——本就该绑，不抽象成通用协议（3 个字段不值得）
- 换宿主 = 壳重写 postMessage 注入部分，UI / 引擎零改动

## 九、UI/UX 打磨规范（美术 + 易用性，详细可执行）

> 基于 index.html（v0.1.10）现状评估。目标：从「能用」到「好用」，同时不破坏克制原则与零依赖形态。
> 每条标注改哪里（文件/类/函数）、怎么改、优先级。

### 9.1 设计原则（贯穿所有改动）

1. **主任务是漫游**——搜索框 + 结果卡片是主区，任何元素不抢它们
2. **次功能折叠**——关系/参数/相似/统计/详情一律折叠或抽屉，不挤主界面
3. **信息分级显示**——标题/路径是「用户看」，ID/分数是「AI/调试看」——后者默认收敛
4. **零依赖不妥协**——所有效果用原生 CSS/JS，不引入任何库
5. **桌面 + 插件窄面板双形态同一份代码**——靠 `?embed=1` + 响应式，不是两份

### 9.2 布局与信息架构

| 区 | 桌面版（920px） | 插件窄面板（~360px） |
|---|---|---|
| 顶栏 | 图标按钮组 + brand + 次功能入口 | brand 隐藏，按钮图标化（brand.tag / .header-right .txt 已 @media 处理） |
| 搜索框 | 全宽、自动聚焦 | 同左 + **sticky 常驻顶部**（滚动时不丢搜索） |
| 结果卡片 | 单列，信息密度高 | 单列，间距收紧，卡片更矮 |
| 次功能面板 | 折叠（主区下方） | **抽屉式叠层**（从右滑出，不占主区） |
| 页脚 | source / revision / 埋点计数 | 同左（小字，弱化） |

**关键改动**：
- `.header-right` 加「关系 / 参数」按钮已是 `ghost sm` ✅ 保持；新增「相似 / 统计」入口同样 `ghost sm`，宽度 `min-width` 不挤压。
- 搜索框 `.search` 在窄面板（`?embed=1`）时 `position: sticky; top: 0; z-index: 5; background: var(--bg); padding: 8px 0`。

### 9.3 控件规范（改哪里 + 怎么改）

**按钮层级（顶栏分组）**
| 组 | 按钮 | 样式 | 说明 |
|---|---|---|---|
| 主操作 | 🎲 随便逛逛 | `.btn.primary`（带文字，不是 icon-btn） | **升级**：从 icon-btn 改为带文字的次要主按钮，新用户第一个入口 |
| 次操作 | ⌂ ← ↻ | `.icon-btn` | 保持图标 |
| 次功能 | 关系 / 参数 / 相似 / 统计 | `.btn.ghost.sm` | 保持 |

**卡片（结果卡片，核心信息单元）**
现状 `.card .top .rank .title .id .type .hop .open` + `.path` + `.scores`。评估与改动：
1. **`.id` 收敛**：常显的纯数字 ID 是噪音——改 `display: none`，ID 放入 `.title` 的 `title` 属性（hover 提示）+ 保留在 data-id 属性（不渲染文本）。`# 卡片 id → 悬停提示，不常显`
2. **`.scores` 默认折叠**：score/ppr/act 是白盒但噪音——卡片下方加一行小字「详情 ▸」点击展开，或卡片 `hover` 时 `.scores` 从 `opacity:0.3` 变 `1`。首选 hover 展开（无点击成本）。`.scores { opacity: .3; transition: opacity .15s } .card:hover .scores { opacity: 1 }`
3. **`.path` 药丸链化**：现在纯文本 `甲 → 乙 → 丙`。改为可点击小胶囊（复用 `.rel-node` 样式，新增 `.path-node`）：点击中间节点 = 从该节点继续漫游（`roam(id, 'push')`）。`cardHTML()` 里 path 字符串拆成数组渲染。窄面板里截断（`max-width` + ellipsis）。
4. **`.rank` 保留**（序号有定位价值）。
5. **`.open` 按钮**：桌面 `hover` 显示（现有）✅；窄面板（`?embed=1`）**常显**（触屏/侧栏无 hover）。`body.embed .card .open { opacity: 1 }`。

**徽章（hop / type / 深跳）**
- 1/2/3-hop 金/蓝/紫保留（`--hop1/2/3`）✅；3-hop 深跳加 `deep` 弱化类（现有），建议深跳用专属弱紫（`--hop3` 降饱和）而非纯灰。
- `type-` 颜色（人物/设定/线索/ADR/doc/block）保留 ✅ 良好。

### 9.4 面板组织（统一抽屉，不堆叠）

> 现状：关系 / 参数两个独立折叠面板（`.panel` + `toggle()`，同时只开一个 ✅）。未来加相似 / 统计 / 详情 = 5 个面板。

**建议：统一「侧滑抽屉」**
- 结构：右侧固定抽屉（`position: fixed; right: 0; top: 0; height: 100%; width: min(420px, 90%); background: var(--panel); transform: translateX(100%); transition: transform .2s`）
- 打开任一入口（关系 / 参数 / 相似 / 统计 / 详情）→ 抽屉滑出，标题栏切换内容，同一时刻只开一个（复用现有 `toggle()` 单开逻辑）
- 关闭：抽屉标题栏 ✕ 或点击主区遮罩（`.overlay`）
- 窄面板（`?embed=1`）：抽屉宽度 `100%`，全屏覆盖

**好处**：5 个面板不堆在主区下方淹没结果卡片（主任务是漫游）；窄面板天然适配。
**节点详情预览（#3）不占抽屉**——它是「点卡片展开的浮层/内联区」，属于主流程（点卡片想看一眼，不想被抽屉打断）。

### 9.5 状态规范（加载 / 空 / 降级 / 死路）

| 状态 | 现状 | 建议改动 |
|---|---|---|
| **加载** | 纯文字「加载中…」（`.loading`） | **骨架屏**：渲染 3 个灰色 `.card` 占位（`background: var(--surface); height: 72px; border-radius: 12px; animation: shimmer 1.2s infinite`），视觉反馈强一档 |
| **空簇**（随机漫步空结果） | 文字「再掷一次」 | **做成按钮**：点一下直接 `roamRandom()`，少一次点击 |
| **死路节点** | `.deadend` 提示 ✅ | 加「查看相似节点」按钮（联动 similar，把死路变成转向） |
| **降级提示** | `.fallback` 黄色提示 ✅ | 保留，加关闭按钮（✕，一次性 dismiss） |
| **JS 错误** | `window.onerror` → meta ✅ | 保留，建议错误同时输出 console.error（开发者可见） |

### 9.6 交互与动效规范

- **hover 效果**：现在卡片 `transform: translateX(3px)`——**改为边框提亮 + 轻微上浮**：`.card:hover { border-color: var(--accent); transform: translateY(-1px); box-shadow: 0 4px 12px rgba(0,0,0,.2) }`。translateX 在窄面板会溢出容器，且视觉像"躲开"。
- **focus 状态**：搜索框/按钮/卡片加 `:focus-visible` 轮廓（`.card:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px }`）——键盘导航（P2-9）的前提。
- **点击反馈**：卡片点击时 `.card:active { transform: scale(.99) }`——确认感。
- **动画克制**：保留 `@keyframes fade`（面板）+ floaty（气泡，P0.5 改掉后移除）；新增动画统一 `prefers-reduced-motion` 降级。
- **键盘**：`/` 聚焦搜索、`Esc` 关闭面板/抽屉、↑↓ 选卡片（P2-9 实现时遵守 9.6 focus 规范）。

### 9.7 插件/嵌入适配（与 §八 多宿主兼容联动）

- `?embed=1`（或 `top !== self` 自动）→ `body` 加 `embed` 类：隐藏 hero / brand / 页脚次要信息，搜索 sticky，卡片间距收紧，`.open` 常显。
- **postMessage 桥**：前端只发 `{type:'open', id}` 消息，宿主实现打开（P0-2）——UI 不感知宿主。
- **主题跟随**：P1-7 加 light 变量，`prefers-color-scheme` 或宿主传参（如 `?theme=light`）切换；深色时跟随宿主而非固定 Tokyo Night。

### 9.8 美术细节（Tokyo Night 保留，微调）

- **颜色语义**：结构类型色保留；「深跳」用弱紫（`--hop3` 50% 饱和）替代纯灰。
- **字体节奏**：标题 15-16px / 正文 13-14px / 元信息 11-12px 层级保留 ✅；卡片标题加粗（`.card .title { font-weight: 600 }`）现有 ✅。
- **留白**：主区 `max-width: 920px` 保持；卡片 `margin-bottom: 8px` 保持；窄面板收紧为 `6px`。
- **radius/shadow**：`--radius: 12px` 保留；shadow 用 `--shadow` 现有，不加深（保持克制）。

### 9.9 可执行改动清单（标优先级，开发 agent 照做）

**P0（随插件化，视觉收敛）—— ✅ v0.1.12 全部落地**
- [x] 9.2 搜索框窄面板 sticky 常驻
- [x] 9.3 卡片 `.id` 收敛为悬停提示；`.scores` 默认折叠（hover 展开）
- [x] 9.3 🎲 升级为带文字的主按钮「随便逛逛」
- [x] 9.6 hover 效果修正（translateX → 边框提亮 + 上浮）
- [x] 9.4 侧滑抽屉统一面板（关系/参数/相似/统计/详情）
- [x] 9.2 source/revision/埋点 移页脚

**P1（易用性）—— ✅ v0.1.12 部分落地**
- [x] 9.3 `.path` 药丸链化（可点击中间节点续漫游）
- [ ] 9.5 加载骨架屏 + 空簇按钮化 + 死路节点转相似按钮（空簇默认按钮化/死路仍提示，骨架屏未做）
- [ ] 9.5 降级提示加关闭按钮（降级提示保留，关闭按钮未做）
- [x] 9.6 focus-visible 轮廓 + active 点击反馈
- [x] 9.7 `?embed=1` 嵌入适配（搜索 sticky / open 常显 / 间距收紧）
- [x] 9.7 主题跟随宿主（light 变量 + postMessage {type:'theme'}）

**P2（打磨）—— ✅ v0.1.12 部分落地**
- [x] 9.8 深跳用弱紫替代纯灰（hop3 降饱和 + deep 弱化）
- [x] 9.6 键盘导航（`/` 聚焦搜索 / Esc 关抽屉 / ↑↓ 选卡片——本版 `/`+Esc 已做，↑↓ 待补）
- [x] 9.7 主题跟随宿主（light 变量）

**改动范围**：全部在 `internal/web/static/index.html`（单文件），不引入依赖；`?embed=1` 逻辑复用现有 `toggle()` / `paramQS()` 模式；相似/统计面板复用 `toggle()` 单开模式。

---

## 附录 · 测试速查与历史交接（由 `docs/history/frontend-issues.md` 折叠）

### A.1 快速上手（起服务）

```powershell
# 虎鲸库（TestOrca，推荐，数据丰富）
seren serve "D:\WorkSpace\NoteLib\TestOrca\TestOrca.db" --port 8910 --store <临时store.bbolt> --repo TestOrca

# Obsidian vault（另备测试库）
seren serve "D:\WorkSpace\WriteLib\Novel_AI_Helper" --port 8901
```

- 浏览器打开 `http://127.0.0.1:8910/`。
- **每次改 `index.html` 后必须硬刷新**（Ctrl+Shift+R）——页面已设 `Cache-Control: no-store`，但浏览器/代理仍有极低概率缓存，硬刷新排除干扰。
- 服务端日志会打印监听/刷新事件；`/api/stats` 返回 revision（图版本号）。
- 后端改动后需重新 `go build` 并重启 serve（同一端口要先杀掉旧进程）。

### A.2 测试方法速查

| 目标 | 方法 |
|---|---|
| 起服务 | `seren serve <库> --port 8910 --store <store> [--repo TestOrca]` |
| 硬刷新 | Ctrl+Shift+R（排除缓存干扰） |
| 看 revision | `GET http://127.0.0.1:8910/api/stats` |
| 漫游 API | `GET /api/roam?q=<词>&top=<N>`（锚点/结果/路径/降级都在 JSON 里） |
| 关系 API | `GET /api/relation?from=<ID或标题>&to=<ID或标题>`（最短路径+PPR+证据） |
| 手动刷新 | `POST /api/refresh?limit=50`（响应含 added/updated/deleted/renamed） |
| 埋点 | `POST /api/touch {"target":"7128","from":"10156"}` → store touch 表 |
| 跳转 URI | roam 结果里每项的 `uri` 字段（orca-note:// 或 obsidian://） |
| 自动监听 | 改库文件 → 等 60s 节流窗口 → revision+1 + 页面提示 |

### A.3 已知环境限制

- **沙箱内 Edge headless dump-dom 不可用**（piped stdio 被沙箱 EPERM 拦截）——前端自动化目前用 **ego browser**（AI 驱动，探索/回归）；**Playwright 留作可选防回归套件**（需用户安装环境后在**无沙箱环境**跑）。
- 虎鲸活库被 App 独占锁，serve 读取走一致性快照（`CopyDBForRead`），测试时改库文件请通过虎鲸 App 操作或改副本。
- 多实例冲突：同一 store 文件别开两个 serve；改代码重启前先杀旧进程。

### A.4 已修复问题（防回归清单，v0.1.3–v0.1.6 修过）

- **"页面#N" 噪声**（v0.1.3）：adapter 过滤虎鲸空壳页面 + `container` 类型排除（538→234）。
- **点击节点没反应**（v0.1.3，三层根因）：① 搜索结果锚点/气泡是纯展示 span → 绑 `data-id` 事件委托；② 孤立节点降级搜纯数字 → 改用锚点 title 全文搜索；③ store 加载"存在但从未写入"报错 → 兼容空库文件。
- **初始页点入无反应**（v0.1.3）：统一 `document` 级事件委托（`.card, .bubble, #anchors .anchor[data-id]`）；孤立节点显示"⚠ 这个节点到头了"横幅；点击瞬间显示"漫游中…"。
- **浏览器缓存旧页面**（v0.1.3）：`/` 响应头 `Cache-Control: no-store`。
- **锚点点击语义**（v0.1.4）：输入框显示节点标题而非纯数字 ID。
- **自动更新提示**（v0.1.4）：每 30s 轮询 `/api/stats`，revision 变化 → "库已自动更新"。
- **深跳 score≤0 展示**（v0.1.6）：后端桶内归一化 + 前端「深跳」标签。
