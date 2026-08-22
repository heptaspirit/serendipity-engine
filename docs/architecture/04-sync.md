# 同步层 · 对账刷新 / 自动监听 / 反馈埋点

> 面向未来维护者：这三块是 v0.1.2-v0.1.4 陆续加入的"保持图与笔记软件一致"的机制。
> **克制设计是硬约束**（用户拍板）：任何改动不得引入正反馈循环或资源耗尽。

## 1. 对账刷新（`internal/sync` + `cmd refresh` + `/api/refresh`）

### 为什么是全量 diff 而不是增量

- 虎鲸 Block 表**没有删除墓碑**——删除只能靠"全量重解析 + 与旧状态比对"发现；
  modified 是秒级时间戳，同秒多次修改会漏报——全量 diff 免疫时间戳精度问题。
- Obsidian vault 解析毫秒~百毫秒级，全量足够便宜。
- 结论：v1 统一"全量解析 → 规范化 diff → 幂等全量写回"；mtime 快照对账（只重解析
  变更文件）是 v1.5 的增量优化，不改语义。

### 流程（`refreshFunc`，CLI 与 serve 共用）

```
store.Load(storePath)     旧状态（无 → 空，等价首次全新增）
→ parseSource(...)         全量重解析（虎鲸：快照 → ParseOrcaDB）
→ sync.Diff(old, cur)      按 ID 对齐，规范化比较
→ store.Save(storePath)    幂等全量重写
→（serve）ReplaceGraph     内存图替换 + revision++
```

### 边际情况（已实测，见 design.md §6.8 落地表）

首次全新增 / 增删改字段级明细 / 引用增减（页内自环丢弃）/ 归属变化（双文档
updated）/ 删除被引用块（deleted + 引用方 refs-1 + 子块变链顶 added）/ 幂等。

## 2. 自动监听（`internal/watch`，v0.1.4）

**克制设计（防正反馈，用户明确要求）：**

| 手段 | 说明 |
|---|---|
| 轮询而非事件监听 | Obsidian 逐 .md 比对 (mtime,size) 快照（含增删检测）；虎鲸 stat 库文件。频率完全可控，无 fsnotify 事件风暴 |
| 刷新节流合并 | 检测到变化进"待刷新"；距上次实际刷新 < 节流窗口（默认 60s）则合并等待——连续编辑吸收为每分钟至多一次全量刷新 |
| **排除自身产物** | 扫描跳过 `.serendipity/.git/.obsidian/.trash/.dsh/.agents`——store 写入 .serendipity 不会自触发"变化→刷新→写入→再变化"无限循环（正反馈核心防线） |
| 失败节流重试 | 刷新失败仅记日志、保留待刷新，下一轮（节流后）重试，不轰炸 |

控制：`--watch-off` / `--watch-interval`（轮询秒，默认 10）/ `--watch-throttle`
（节流秒，默认 60）。serve 默认开启。触发后 `ReplaceGraph` + revision+1，
前端 30s 轮询 stats 提示"库已自动更新"。

## 3. 反馈埋点（touch，v0.1.4）

- Web 端点击节点 → `POST /api/touch {target, from}` → `store.AppendTouch`。
- **独立表 touch**：Save 全量重写不清除；容量上限 5000 条（超限删最旧）。
- **v1 不演化边权**：埋点只记录——"点击→边权变→结果变→再点击"的跑飞在源头切断。
  将来若做演化，须先出观察报告（哪些边被点、频次），且仍须保留节流与上限。
- 写失败静默（埋点不影响主流程）。

## 4. 与旧机制的关系

- 手动 `↻`（/api/refresh）、CLI `refresh`、自动 watch **三路共用同一 refreshFunc**，
  语义一致（幂等），无分叉。
- 快照/锁的处理见 `02-adapters.md`（VACUUM INTO + 文件拷贝双路径、to_pinyin stub、
  锁探测）——对账正确性的前提。
