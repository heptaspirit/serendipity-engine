# 产品形态决策：跳回软件 vs 插件（调研记录）

> 日期：2026-08-21
> 起因：Web UI 目前是"一直跳"的独立页面，用户问：能否从界面跳回笔记软件？还是做成软件插件？
> 结论：**分层做，不二选一**——① 跳转链接（今天已做）→ ② 插件薄壳（中期）→ ③ MCP server（v3，与虎鲸 MCP 互操作）。

---

## 一、核心判断

**图谱漫游是"发现层"，不是"阅读层"**——发现之后的实际阅读必须发生在原笔记软件里（大纲层级、编辑、上下文都在那里）。所以"跳回去"是闭环必需品，不是可选项。

## 二、机制盘点（实测/调研）

| 软件 | 跳回机制 | 现状 | 插件生态 |
|---|---|---|---|
| Obsidian | `obsidian://open?vault=<名>&file=<相对路径>` URI 协议 | ✅ **已接入**（Web 卡片"打开 ↗"）；vault 名=库目录名，可用 `--vault-name` 覆盖 | 成熟（TS + manifest，Webview 面板可嵌我们的 Web UI） |
| 虎鲸（Orca Note） | 无公开 `orca://` URI；官方有 **CLI + MCP server**（`sethyuan/orca-note-cli`，Streamable HTTP + Bearer token） | ⏳ 跳转未接（无 URI）；但 MCP 工具齐全（get_page/get_blocks_text/query_blocks…） | TS/Vite 插件（orca-simple-task 模板），可做面板 |

**意外发现**：虎鲸**已内置 MCP server**——我们的 v3"AI 通道用 MCP"与它天然互操作；且虎鲸 adapter 未来可加"运行中走 MCP、离线走 SQLite 直读"双通道（v1.5 候选）。

## 三、分层路线（成本递增）

1. **① 跳转链接（2026-08-21 已实现）**：Obsidian 卡片加 `obsidian://` URI——零插件，浏览器点击即跳。虎鲸待官方 URI 或插件补充。
2. **② 插件薄壳（中期候选）**：Obsidian 侧边栏插件用 Webview 指向本地 REST，等于把现有 Web UI 搬进软件——**核心不变，插件只是客户端**（设计 §6.8 边界：插件只作可选分发形态，绝不把架构锁进插件）。参考同赛道：Juggl（交互图谱）、Smart Connections（embedding 相似面板）。
3. **③ MCP server（v3）**：把 `(节点, 分数, 路径)` 暴露给 AI（设计"双消费者"通道 B）；与虎鲸 MCP 互操作。

## 四、调研局限

- Obsidian URI 官方页抓取失败，语法基于多篇社区/论坛佐证——**待用户本地实测一次**。
- 虎鲸"跳到指定块界面"能力未确认存在。

## 五、已实现细节

- `internal/web/server.go`：`obsidianURI(path)` 拼 URI（去 .md、URL 编码）；`roamItem.uri` 字段（obsidian 源且有 vault 名才输出）。
- `cmd/seren serve`：`--vault-name` 覆盖默认（库目录名）；存储回读按路径形态（`block/` 前缀）识别虎鲸、禁用跳转。
- 验证：8899（Obsidian）卡片带 URI；8900（虎鲸）无 URI 正常漫游。
