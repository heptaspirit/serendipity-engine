# Web 层 · REST 服务与前端

> 面向未来维护者：`internal/web` 是产品的人机界面（REST + 嵌入的单页前端）。
> 前端改动注意：页面已禁用缓存（`Cache-Control: no-store`），但仍需硬刷新验证。

## 1. REST API

| 端点 | 方法 | 说明 |
|---|---|---|
| `/api/stats` | GET | 节点/边/版本/**revision**（图版本号，自动/手动刷新 +1，前端轮询提示更新）/**configured**（v0.1.15 无库启动：是否已配库） |
| `/api/vault` | GET/POST | **无库启动配库/换库（v0.1.15）**：GET 查配置（configured/source/vault）；POST `{path,...}` 解析建图 → 替换图 + 重建闭包 + 重启 watch（`VaultFunc` 注入时注册） |
| `/api/hot?n=` | GET | 热门节点（按度降序，跳过结构类型 + 目录枢纽），初始页气泡池 |
| `/api/roam?q=&top=` | GET | 查询漫游（与 CLI 同一 `roam.Compute`）；`?random=1` 随机漫步（v0.1.7：roll 随机起点 + 簇；`?seed=N` 固定种子可复现、跳过防重复；`?rand_alpha=` 度加权指数；服务端内置 32 个"最近起点"ring 防连续撞车） |
| `/api/relation?from=&to=` | GET | 两节点关系查询（v0.1.5）：BFS 最短路径 + 双向 PPR 强度（对称 affinity）+ 激活值 + 证据链；from/to 接受 ID 或标题（`resolveID` 锚定）。white-box 输出，为未来 MCP 暴露（`graph.relation`）铺路 |
| `/api/refresh` | POST | 对账刷新（`RefreshFunc` 注入时注册；v0.1.15 起路由无条件注册、handler 内 nil 判定）→ diff 摘要（limit 截断；含 renamed/renames 明细，v0.1.5） |
| `/api/touch` | POST | 反馈埋点 `{target, from}`（`TouchFunc` 注入时注册） |
| `/` | GET | 嵌入页面（`go:embed static/index.html`，`no-store`） |

### 并发模型

- `Server.mu sync.RWMutex` 保护 `G` 与 `revision`：读接口（stats/hot/roam）持 RLock；
  刷新（手动/自动）持 Lock 整体替换（`ReplaceGraph`）。本地单用户，够用。
- 随机漫步状态（v0.1.7）：`Server.randMu` 保护 `rng` 与 `recent`（rand.Rand 非并发安全）。

### 安全前置（v0.1.8，roadmap M0-0.1，`auth.go`）

- **Host 校验**：只接受回环地址（127.0.0.1 / localhost / ::1，带不带端口均可），
  其余 403——防 DNS rebinding / Host 头欺骗。
- **API token 鉴权**：所有 `/api/*` 必须带 `X-Seren-Token` 头或 `?token=` 查询参数，
  与 `Server.Token` 常量时间比较（crypto/subtle）。token 由 `handleIndex` 注入页面
  （`__SEREN_TOKEN__` 占位符替换），前端 fetch 包装自动携带；外部页面受同源策略
  限制读不到 → 防 CSRF 调写接口（refresh/touch）与读取本地笔记。
- 来源：`--token` 指定，否则 `cmd/seren` 自动生成 32 位 hex 并打印；重启后 token
  变化，浏览器重新 GET / 即拿到新 token。`Server.Token` 为空 = 未配置鉴权（嵌入式
  用法），cmd/seren 永远生成非空。
- curl 用法：`curl -H "X-Seren-Token: <token>" http://127.0.0.1:8910/api/stats`。

### 跳转 URI

- Obsidian：`obsidian://open?vault=<名>&file=<相对路径>`（`VaultName` 非空）。
- 虎鲸：`orca-note://<repo>/block?blockId=<id>`（`OrcaRepo` 非空，`uriFor` 按源选择）。
  协议来自 orcanote-agent-skills 的 orcanote-markdown skill。

## 2. 前端（`static/index.html`，零依赖原生 JS）

### 页面结构

- **初始页（hero）**：热门节点漂浮气泡（`/api/hot` 随机采样 10 个），点击即漫游。
- **漫游结果**：锚点标签（前 5 个"重的"，可点击）+ 结果卡片（rank/标题/类型/hop/
  路径/score + 「打开 ↗」跳转）+ 全文降级卡片。
- **历史栈**：与浏览器 history 同步（`?q=` URL、后退/前进可用，`goHome` 重置）。

### 交互关键点（v0.1.3-v0.1.4 加固）

- **事件委托**：卡片/气泡/锚点统一由 document 级 click 处理（`closest('.card,
  .bubble, #anchors .anchor[data-id]')`）——任何时刻渲染的元素都可点，杜绝漏绑定；
  「打开 ↗」链接（`a`）排除。
- **点击反馈**：点击瞬间 meta 区显示"漫游中: <节点> …"；请求失败显示错误；
  `window.onerror` 显示页面 JS 错误（不静默）。
- **到头了**：`fallback=2` 且全文也无命中 → 醒目黄色横幅"⚠ 这个节点到头了"。
- **埋点**：点击时 `POST /api/touch`（fire-and-forget，不阻塞漫游）。
- **自动更新提示**：每 30s 轮询 `/api/stats`，revision 变化 → "库已自动更新"。
- 点击卡片输入框显示节点**标题**（而非 ID）——查询词仍是 ID（精确锚定）。

### 嵌入契约（iframe 壳 / postMessage 桥，v0.1.12 前端 P0）

> 前端被 Obsidian 插件（~~虎鲸插件暂停，2026-08-26~~）嵌进面板时（`?embed=1` 或 `window.top !== window.self`），
> 进入 **紧凑嵌入模式**（`body.embed`）：隐藏 hero/brand/hint、收窄 padding、
> 顶栏按钮文字仅窄屏（<560px）才隐藏（纯文字按钮不因藏 `.txt` 变空壳）。

**宿主 → 页面 `postMessage`**：

| 类型 | 载荷 | 说明 |
|---|---|---|
| `theme` | `{mode:'light'\|'dark', colors?}` | `mode` 设 `data-theme`；`colors` 可选，覆盖 `--bg/--panel/--surface/--text/--muted/--accent/--primary/--border/--border-strong` 等 token（Obsidian 插件注入其主题配色，使界面颜色跟随宿主） |
| `locale` | `{lang:'zh'\|'en'}` | 覆盖语言（默认 `navigator.language`） |
| `activeFile` | `{id}` | 宿主告知当前笔记路径（记录到 `window.__activeFile`，预留命令锚点） |

**页面 → 宿主 `postMessage`**：嵌入态点「打开 ↗」→ `{type:'open', id, uri}`；
宿主**优先用 `uri` 解码出的 file 路径**跳回（llm-wiki/路径化 id 下更可靠），失败退回 `id`。
非嵌入态则 `window.open(uri)`（外链）。

### serve 组装（cmd/seren）

```
loadSource → g, docs, src
isOrca（.db 或 store 回读 Path 前缀 block/）
vaultName（Obsidian） / orcaRepo（--repo 或库文件名去 .db）
refreshFunc（重解析+diff+Save） / touchFunc（AppendTouch）
web.New(...) + srv.OrcaRepo = orcaRepo
watch goroutine（默认开）→ refreshFunc → ReplaceGraph
http.ListenAndServe
```

## 4. 前端改动的验证方法（沙箱环境）

- JS 语法：提取 `<script>` 内容 `node --check`。
- 接口：`Invoke-RestMethod` 逐端点验证。
- 页面版本：curl 检查关键标记（如 `data-title`、`deadend`、`漫游中`）确认 go:embed
  已更新（重建 + 重启服务后）。
