# 同步层 · 对账刷新 / 自动监听 / 反馈埋点 / 改名迁移

> 面向未来维护者：这些是 v0.1.2-v0.1.5 陆续加入的"保持图与笔记软件一致"的机制。
> **克制设计是硬约束**（用户拍板）：任何改动不得引入正反馈循环或资源耗尽。

## 1. 对账刷新（`internal/sync` + `cmd refresh` + `/api/refresh`）

### 为什么是全量 diff 而不是增量

- 虎鲸 Block 表**没有删除墓碑**——删除只能靠"全量重解析 + 与旧状态比对"发现；
  modified 是秒级时间戳，同秒多次修改会漏报——全量 diff 免疫时间戳精度问题。
- Obsidian vault 解析毫秒~百毫秒级，全量足够便宜。
- 结论：v1 统一"全量解析 → 规范化 diff → 幂等全量写回"；**v0.1.6 起 Obsidian
  源用快照增量解析**（`adapter.ParseVaultIncremental`：mtime/size 未变的文件复用
  旧解析结果，只重解析变更/新增；语义与全量等价，含同名消歧——复用只是跳过
  I/O+正则，消歧统一跑）。虎鲸源仍是全量（单 SQLite 文件无 per-file mtime）。

### 快照增量解析（v0.1.6）细节

- **命中条件**：`mtime(截断到秒) 相同 && size 相同`。存储只保存秒级 mtime
  （`MTime.Unix()`），文件系统 ModTime 带纳秒（NTFS 100ns）——**比较前双方都
  截断到秒**，否则永远不匹配（实测抓出的坑）。
- **ID 一致性**：复用文档先重置 ID 为 basename（还原未消歧形态），扫描循环里
  统一跑同名消歧——保证与全量解析结果逐文档一致（有单测锁定：`增量 ≡ 全量`）。
- **删除检测天然成立**：文件消失 → 扫描不到 → 不在返回列表 → diff 报 deleted。
- **已知限制**：秒级 mtime + 同 size 但内容被改回 → 漏检（概率极低，接受）；
  touch（改 mtime 不改内容）只会多重解析一次，无害。
- **入口**：`refreshParse`（cmd/seren）——Obsidian 源且旧状态非空走增量，
  其余（--db / 虎鲸 / 首次）全量；CLI 输出"快照增量：复用 X / 重解析 Y"。

### 流程（`refreshFunc`，CLI 与 serve 共用）

```
store.Load(storePath)        旧状态（无 → 空，等价首次全新增）
→ store.LoadRenames(storePath)  持久化改名映射（v0.1.5，修订 #8）
→ parseSource(...)           全量重解析（虎鲸：快照 → ParseOrcaDB）
→ sync.Diff(old, cur)        按 ID 对齐，规范化比较（含改名配对）
→ sync.MergeRenames(stored, fresh, cur)  合并映射：旧名重现即失效
→ store.SaveRenames + RenameTouch        持久化映射 + 迁移埋点旧 ID
→ store.Save(storePath)      幂等全量重写（写原始 Refs = 文件真相）
→（serve）ReplaceGraph       内存图替换（建图叠加重定向）+ revision++
```

### 边际情况（已实测，见 design.md §6.8 落地表）

首次全新增 / 增删改字段级明细 / 引用增减（页内自环丢弃）/ 归属变化（双文档
updated）/ 删除被引用块（deleted + 引用方 refs-1 + 子块变链顶 added）/ 幂等。

## 1.5 改名迁移（修订 #8，v0.1.5）

**动机**：Obsidian 文件名即 ID，改名 = 删一个 + 加一个——被引用节点的链接会悬空、
touch 埋点断在旧 ID 上。

**判定**（`sync.DetectRenames`）：旧有新无 × 新有旧无 配对，双信号——
内容哈希（Text 相同；Refs/Path/Title 不参与，改名时它们可能变）+ 路径相似度
（同目录优先，basename 公共前缀次级）。**并列候选宁可不判**（退回 deleted+added）。
虎鲸守卫：两侧 ID 均纯数字 → 跳过（块 ID 稳定，删+增是"真删除 + 新块"）。

**落地**（三层，缺一不可）：

| 层 | 机制 | 说明 |
|---|---|---|
| 持久化 | `renames` 表（`store.LoadRenames/SaveRenames`） | 改名是持久身份事实，映射跨刷新保留；`MergeRenames` 在"旧名重现于当前批次"时失效该条 |
| 建图 | `redirectForGraph`（`sync.ApplyRenames`，传递解析链式改名） | documents 存原始 Refs（文件真相，diff 收敛）；图/展示层叠加重定向——他人 `[[旧名]]` 不悬空 |
| 埋点 | `store.RenameTouch` | touch 表 target/src 旧 ID → 新 ID；两阶段占位防链式互踩，先传递解析 |

**验证**：单测（内容变不判、并列跳过、跨目录不判、虎鲸不误判、链式传递、merge
失效、touch 迁移）；E2E（index → 改名 → refresh → refresh 幂等收敛；悬空链接 0；
touch target/src 双列迁移成功）。

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
