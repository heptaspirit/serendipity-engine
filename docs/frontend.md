# Serendipity Engine · 前端计划（Web UI）

> 性质：**前端做什么**——Web UI 从「漫游工具」升级为「阅读 + 漫游工具」，并为 Obsidian 插件薄壳（[`docs/roadmap.md`](roadmap.md) M2）铺路。
> 源码：`internal/web/static/index.html`（单文件，零依赖原生 JS，go:embed 嵌入）。
> 相关：战略定位 [`docs/positioning.md`](positioning.md) · 后端 [`docs/backend-backlog.md`](backend-backlog.md)。

## 一、已落地（✅ v0.1.11–v0.1.12，不保留叙事）

- **P0 插件化前置**：紧凑嵌入 `?embed=1`（隐藏 hero/brand、搜索 sticky、卡片收紧、`.open` 常显）+ postMessage 桥（`{type:'open',id}` / 宿主注入 theme/locale/activeFile）+ i18n 中英双语全部文案（`I18N` 字典 + `t(key)`，语言跟随宿主 > navigator.language）。
- **P0.5 hero 改静态热门列表**（移除随机浮动气泡——插件场景纯负面资产）。
- **易用性**：节点详情预览（`/api/node` + 卡片预览浮层）· 🎲 主按钮带文字 · 侧滑抽屉统一面板（关系/参数/相似/统计/详情）· 卡片 `.id` 收敛为 hover 提示、`.scores` 默认折叠 · `.path` 药丸链化（点击中间节点续漫游）· focus-visible + active 反馈 · 主题跟随宿主（light 变量 + postMessage theme）。

## 二、当前积压（P1/P2）

| # | 功能 | 说明 |
|---|---|---|
| P1 | 命中片段高亮 | 全文降级只显示「命中 N 次」，用户不知道为什么命中 → 返回 snippet + 高亮命中词 |
| P1 | 卡片操作菜单 | 右键/长按：复制 ID、查看邻居（图中连着什么）、快捷填入关系查询 from/to |
| P1 | 状态规范 9.5 | 加载骨架屏（3 个灰色卡片占位 shimmer）· 空簇「再掷一次」做成按钮 · 死路节点加「查看相似节点」· 降级提示加 ✕ 关闭 |
| P1 | 前端「社区」入口（M2 酌情） | `/api/communities` 后端已就绪，前端侧滑抽屉可加「社区」视图 |
| P1 | serve 页面「AI 接入」卡片 | 展示 `seren mcp <vault> --db <store>` 配置 + 一键复制（MCP 是独立子命令，前端只能展示模板，不能"开关"） |
| P2 | URL 状态扩展 | 随机 seed、参数值进 URL，可分享/收藏特定漫游 |
| P2 | 键盘导航 | `/` 聚焦搜索、Esc 关面板、↑↓ 选卡片（↑↓ 待补） |
| P2 | 历史栈持久化 | iframe 重载频繁（插件场景尤其），漫游路径存 sessionStorage |

## 三、宿主无关性（postMessage 协议，壳 ↔ Web UI）

Web UI 只认识三样东西：`/api/*`（引擎 REST 契约）、`localStorage`（自身偏好，白名单 key）、`postMessage`（与壳的宿主上下文通道）。iframe 跨源（localhost 端口 vs 宿主 app://）浏览器强制隔离——Web UI 物理上无法访问宿主 API，壳是**唯一**接触宿主 API 的层，只做翻译不塞逻辑。

```
壳 → Web UI（宿主上下文注入，UI 永远不需要知道宿主是谁）：
  {type:'theme',   mode:'light'|'dark', colors?:{bg,panel,surface,text,muted,accent,border}}  # colors 可选，覆盖引擎 CSS token
  {type:'locale',  lang:'zh'|'en'}
  {type:'activeFile', id:'xxx'}             # 命令锚点（用户当前在看哪篇）
Web UI → 壳：
  {type:'open', id:'xxx', uri?:'obsidian://…'}  # 打开请求（唯一上行）；uri 供宿主解码 file 路径跳回
```

规则：Web UI 绝不直接调宿主 API（跨源也不允许，双保险）；壳设置保持宿主绑定（seren.exe 路径/端口存宿主 settings，3 个字段不值得抽象成通用协议）；换宿主 = 壳重写 postMessage 注入部分，UI / 引擎零改动。

## 四、UI/UX 规范（只留未完成项；已落地不重复）

- **9.2 布局**：桌面 920px / 插件窄面板 ~360px（`?embed=1`）双形态同一份代码；窄面板搜索框 sticky 常驻 ✅。
- **9.6 交互**：↑↓ 选卡片（键盘导航前提）；新增动画统一 `prefers-reduced-motion` 降级。
- **9.8 美术**：深跳用弱紫替代纯灰 ✅；结构类型色保留；radius/shadow 保持克制。

## 附录 · 测试速查

### A.1 起服务

```powershell
# 虎鲸库（TestOrca，数据丰富）
seren serve "D:\WorkSpace\NoteLib\TestOrca\TestOrca.db" --port 8910 --store <临时store.bbolt> --repo TestOrca
# Obsidian vault
seren serve "D:\WorkSpace\WriteLib\Novel_AI_Helper" --port 8901
```

- 浏览器打开 `http://127.0.0.1:8910/`；**每次改 index.html 后必须硬刷新**（Ctrl+Shift+R，排除缓存干扰）。
- 后端改动后重新 `go build` 并重启 serve（同一端口先杀旧进程）。

### A.2 测试方法

| 目标 | 方法 |
|---|---|
| 漫游 API | `GET /api/roam?q=<词>&top=<N>`（锚点/结果/路径/降级都在 JSON 里） |
| 关系 API | `GET /api/relation?from=<ID或标题>&to=<ID或标题>`（最短路径+PPR+证据） |
| 手动刷新 | `POST /api/refresh?limit=50`（响应含 added/updated/deleted/renamed） |
| 全量重建 | `POST /api/rebuild?limit=50`（v0.2.1：忽略增量强制重析整库；设置抽屉"重建索引"按钮） |
| 埋点 | `POST /api/touch {"target":"7128","from":"10156"}` |
| 自动监听 | 改库文件 → 等 60s 节流窗口 → revision+1 + 页面提示 |

### A.3 环境限制

- 沙箱内 Edge headless 不可用（piped stdio 被沙箱 EPERM 拦截）——前端自动化用 **ego browser**（AI 驱动，探索/回归）；**Playwright 留作可选防回归套件**（需无沙箱环境跑）。
- 虎鲸活库被 App 独占锁，serve 读取走一致性快照（`CopyDBForRead`），测试改库走虎鲸 App 操作或改副本。
- 同一 store 文件别开两个 serve；重启前先杀旧进程。
