# Web 层 · REST 服务与前端

> 面向未来维护者：`internal/web` 是产品的人机界面（REST + 嵌入的单页前端）。
> 前端改动注意：页面已禁用缓存（`Cache-Control: no-store`），但仍需硬刷新验证。

## 1. REST API

| 端点 | 方法 | 说明 |
|---|---|---|
| `/api/stats` | GET | 节点/边/版本/**revision**（图版本号，自动/手动刷新 +1，前端轮询提示更新） |
| `/api/hot?n=` | GET | 热门节点（按度降序，跳过结构类型 + 目录枢纽），初始页气泡池 |
| `/api/roam?q=&top=` | GET | 查询漫游（与 CLI 同一 `roam.Compute`）；`?random=1` 随机漫步（v0.1.7：roll 随机起点 + 簇；`?seed=N` 固定种子可复现、跳过防重复；`?rand_alpha=` 度加权指数；服务端内置 32 个"最近起点"ring 防连续撞车） |
| `/api/relation?from=&to=` | GET | 两节点关系查询（v0.1.5）：BFS 最短路径 + 双向 PPR 强度（对称 affinity）+ 激活值 + 证据链；from/to 接受 ID 或标题（`resolveID` 锚定）。white-box 输出，为未来 MCP 暴露（`graph.relation`）铺路 |
| `/api/refresh` | POST | 对账刷新（`RefreshFunc` 非空时注册）→ diff 摘要（limit 截断；含 renamed/renames 明细，v0.1.5） |
| `/api/touch` | POST | 反馈埋点 `{target, from}`（`TouchFunc` 非空时注册） |
| `/` | GET | 嵌入页面（`go:embed static/index.html`，`no-store`） |

### 并发模型

- `Server.mu sync.RWMutex` 保护 `G` 与 `revision`：读接口（stats/hot/roam）持 RLock；
  刷新（手动/自动）持 Lock 整体替换（`ReplaceGraph`）。本地单用户，够用。

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

## 3. serve 组装（cmd/seren）

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
- 用户机器可装 Playwright 后做真实点击回归（计划中）。
