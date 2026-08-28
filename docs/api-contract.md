# API 契约（REST /api/* + 鉴权）

> 维护者/插件仓库与引擎之间的**唯一共享物**——改 API 必须同步本文（维护指南 §5）。
> 版本随引擎走：本文描述 v0.2.0 的行为；字段改动要在此登记。
> base：`http://127.0.0.1:<port>`（serve 默认 8910，始终绑定 127.0.0.1）。

## 0. 鉴权（v0.1.8 起）

所有 `/api/*` 请求必须带 token，两种方式任一：

- 请求头：`X-Seren-Token: <token>`
- 查询参数：`?token=<token>`

token 由 `seren serve` 启动时打印（或 `--token` 指定）；前端页面由服务端注入
（`__SEREN_TOKEN__` 占位符），浏览器打开 `/` 即自动携带，无感。**GET /（页面本体）
不需要 token。** 非法/缺失 token 或非回环 Host → `403 {"error":...}`。

---

## 1. `GET /api/stats` · 库规模

**响应**：

| 字段 | 类型 | 说明 |
|---|---|---|
| `configured` | bool | 是否已配库（v0.1.15 无库启动）：`seren serve` 不带 vault 启动时 → false（空库，前端显示选库引导）；配库（POST /api/vault）后 → true |
| `nodes` | int | 图节点数 |
| `edges` | int | 去重无向边数 |
| `version` | string | 引擎版本（如 `v0.1.12`） |
| `revision` | int | 图版本号：自动/手动刷新后 +1（前端轮询对比以提示"库已更新"） |
| `is_pending` | bool | 库有变化待刷新（v0.1.12）：watch 检测到但节流窗口未到 → true；自动/手动刷新成功 → false。前端据以显示"库有变化，将自动刷新 · 立即刷新"提示条 |
| `digest_available` | bool | 有未读 touch digest（v0.1.14，§3.7）：引擎生成新 digest 且未被 ack → true。插件据以显示"有新的 digest 可供查看"轻量状态提醒（被动、非弹窗） |
| `dangling` | int | 悬空链接总条数（指向不存在文件的链接） |
| `dangling_refs[]` | array | 悬空链接明细 `{source,target}`（v0.1.12，截断上限 50；`source`=引用方节点 ID，`target`=悬空目标） |

```json
{"configured":true,"nodes":235,"edges":318,"version":"v0.2.0","revision":3,"is_pending":false,"digest_available":true,"dangling":4,"dangling_refs":[{"source":"a","target":"ghost1"}]}
```

**未配库形态**（v0.1.15 无库启动）：`{"configured":false,"nodes":0,"edges":0,"version":"v0.2.0","revision":0,"is_pending":false,"digest_available":false,"dangling":0,"dangling_refs":null}`

## 2. `GET /api/hot?n=20` · 热门节点（初始页气泡）

按图度降序，跳过结构类型与目录枢纽。`n` 最大条数（默认 20）。

**响应**：`[{id,title,type,deg}]`

| 字段 | 类型 | 说明 |
|---|---|---|
| `id` | string | 节点 ID |
| `title` | string | 标题 |
| `type` | string | 节点类型 |
| `deg` | int | 图度 |

## 3. `GET /api/roam` · 漫游（查询 / 随机）

**查询参数**：

| 参数 | 说明 | 默认 |
|---|---|---|
| `q` | 查询词（笔记名/标签/别名/ID/任意子串） | — |
| `random=1` | 随机漫步（忽略 q，roll 随机起点 + 簇，v0.1.7） | off |
| `top` | 输出条数（1-60） | 15 |
| `hops` | 激活扩散最大跳数（1-5） | 3 |
| `lambda` | 激活衰减（0-1） | 0.7 |
| `theta` | 激活剪枝阈值（0-1） | 0.1 |
| `alpha` | 结构分（PPR）融合权重（0-1） | 0.5 |
| `beta` | 激活分融合权重（0-1） | 0.5 |
| `rand_alpha` | 随机漫步起点度加权指数（0=均匀，1=偏丰富簇） | 0.5 |
| `seed` | 随机漫步种子（0=随机；固定值可复现同一节点同一簇） | 0 |
| `export=1` | 输出 Markdown 卡片清单（`Content-Type: text/markdown`，v0.1.11） | off |

> `random=1` 且给 `seed` 时跳过防重复 ring；不给 seed 走服务端随机源 + 最近
> 32 个起点 ring（防连续撞车）。`top/hops/lambda/theta/alpha/beta` 由服务端钳制到范围。

**响应**（`fallback` 0=正常簇，1=无锚点全文降级，2=簇空全文降级）：

```json
{
  "query": "Alpha", "source": "orca:...", "vault": "TestOrca",
  "anchors": [{"id":"a","title":"Alpha","type":"note","match":5,"deg":2,"random":false}],
  "results": [{"id":"b","title":"Beta","type":"note","score":0.5,"ppr":0.46,"act":0.7,"hops":1,"path":["a","b"],"uri":"orca-note://TestOrca/block?blockId=b"}],
  "fallback": 0,
  "fallback_hits": []
}
```

- **anchors**：命中锚点按 match 级别/度展示（`match` 1-5：like/tag/alias/title/exact）。
  随机漫步恰一个锚点，`random:true`（前端显示 🎲 随机起点）。
- **results**：`roamItem = {id,title,type,score,ppr,act,hops,path,uri}`（`uri` 为空表示该
  源不提供跳转）。**`path`** 是白盒激活路径（`A → B → C`），前端可点。
- **fallback_hits**：全文降级命中 `{id,title,type,count,uri}`（`count` 命中次数）。

## 4. `GET /api/relation?from=&to=` · 两节点关系（v0.1.5）

`from/to` 接受节点 ID 或标题（经 Resolve 锚定，取首个命中）。任一无法锚定 →
`{"error":"node not found"}`。

**响应** = `graph.Relation`（内嵌）+ `path_nodes`：

| 字段 | 类型 | 说明 |
|---|---|---|
| `from` / `to` | `{id,title,type}` | 端点信息 |
| `direct` | bool | 是否有直达边 |
| `hops` | int | 最短路径长度；-1 = 不可达 |
| `path` | string[] | 最短路径节点（含两端） |
| `affinity` | float | 对称关联强度（双向 PPR 算术平均） |
| `ppr_from_to` / `ppr_to_from` | float | 非对称 PPR（"A 链接 B"骨架地位） |
| `activation` | float | 激活扩散值 λ^hops；不可达 -1 |
| `evidence` | array | 最短路径每条边的来源文档标题（`{a,b,witnesses[]}`，最多 3） |
| `path_nodes` | array | 路径节点 `{id,title}`（前端可读路径链） |

## 5. `GET /api/config` · 前端可调参数白名单 + 源信息

**响应**：

```json
{
  "params":[{"key":"top","label":"结果条数","type":"int","min":1,"max":60,"step":1,"default":15,"group":"基础","hint":"..."}],
  "source":"orca:...", "vault":"TestOrca", "version":"v0.1.11", "nodes":235, "edges":318
}
```

- `params`：前端设置抽屉滑块白名单（`TuneParam`），范围/步长/默认来自服务端，
  前端只做展示与提交，边界由服务端 `clampInt/clampFloat` 强制。
- keys：`top/hops/lambda/theta/alpha/beta/rand_alpha`（v0.1.7 加 `rand_alpha`）。

## 6. `POST /api/refresh?limit=50` · 对账刷新

调用刷新闭包（重解析 → diff → 改名迁移 → 写回存储）→ 替换内存图。**鉴权必带。**

> **v0.2.1 新增 `POST /api/rebuild?limit=50`（全量重建）**：与 refresh 同构，但**忽略增量复用、
> 强制重新解析整库**（相当于 CLI `seren index`）。GUI/插件"重建索引"按钮用它（加画像排除等后
> 强制重建、清存量）。响应形状与 refresh 相同（added/updated/deleted/renamed/unchanged/
> duration_ms/nodes/changes/renames）。鉴权必带。

**响应**：

| 字段 | 类型 | 说明 |
|---|---|---|
| `added/updated/deleted/renamed/unchanged` | int | 对账计数（renamed 从 deleted+added 拆出，v0.1.5） |
| `duration_ms` | int64 | 耗时 |
| `nodes` | int | 刷新后节点数 |
| `changes` | array | 明细（截断到 limit）：`{id,title,kind,type,fields?,added_refs?,removed_refs?}` |
| `renames` | array | 改名明细（截断到 limit）：`{old_id,new_id,title,type}` |

`kind` ∈ `added / updated / deleted`。失败 → `{"error":"..."}`。

## 7. `POST /api/touch` · 反馈埋点

请求体：`{"target":"<节点ID>","from":"<来源节点ID>"}`。
响应：`{"ok":true}`（埋点失败也返回 `{"ok":false}`，不影响主流程——克制设计，
仅记录，不演化边权）。

## 8. `GET /api/similar?id=&k=` · 结构相似节点（v0.1.11）

`id` 接受 ID 或标题（Resolve 锚定首个）。`k` 最大条数（默认 10）。
**独立入口——绝不并入 roam 管线**（红线 1：roam=相关，similar=说同一件事）。

**响应**：

| 字段 | 类型 | 说明 |
|---|---|---|
| `id` | string | 锚定后的节点 ID |
| `results[]` | array | 相似候选（降序） |
| `results[].id/title/type` | string | 候选节点 |
| `results[].score` | float | **Adamic-Adar** 相似度 `Σ_{w∈N(u)∩N(v)} 1/log(deg(w))`（v0.1.12 升级：共同邻居按度倒数加权，比 Jaccard 更抗枢纽偏置、对"专属关联"更敏感） |
| `results[].shared` | string[] | 共享邻居 ID（证据） |
| `results[].shared_titles` | string[] | 共享邻居标题（证据可读，最多 4） |
| `results[].uri` | string | 跳转（obsidian:// / orca-note://） |

排除：自身、直接邻居（已链接=相关非相似）、目录枢纽（deg≥N/2）、结构类型、
空标题、孤立。任一无法锚定 → `{"error":"node not found"}`。

## 9. `GET /api/node?id=` · 单节点详情（v0.1.11）

`id` 接受 ID 或标题。输出 `graph.NodeDetail`（L0 摘要 + L1 邻居导航）：

| 字段 | 类型 | 说明 |
|---|---|---|
| `id/title/type` | string | 节点本体 |
| `aliases/tags` | string[] | 别名/标签（可选） |
| `text` | string | 正文摘要（截断到 200 字符，发现层不读全文） |
| `deg` | int | 图度 |
| `neighbors[]` | array | 无向邻接：链接到谁 `{id,title,type}` |
| `backlinks[]` | array | 有向入边：谁链接我 `{id,title,type}` |

无法锚定 → `{"error":"node not found"}`。

## 10. `GET /api/touch/stats?n=10` · 反馈埋点只读统计（v0.1.11）

只读聚合：总点击数 + 被点击 TopN + 点击来源 TopN。**绝不反馈到排序/hot**
（红线 2：否则等于偷偷启动边权演化）。**不进 MCP**（隐私敏感）。

**响应**：

| 字段 | 类型 | 说明 |
|---|---|---|
| `total` | int | 累计点击数 |
| `targets[]` | array | 被反复点击 `{id,count}`（降序，n 截断） |
| `sources[]` | array | 点击来源 `{id,count}`（降序，n 截断；空 src 排除） |

无埋点表（从未埋点/旧库）→ 全零（不报错，展示友好）。targets 关联 documents 表
过滤**幽灵 touch**（v0.1.12：点击过但已删的节点不再进热度榜）；sources 是自由
文本查询词（非节点 ID），不过滤。

## 11. `GET /api/communities?resolution=&seed=` · 社区发现（v0.1.12，诊断层）

Leiden 社区检测（`github.com/vsuryav/leiden-go`，MIT，vendor）——把图拆成主题簇，
回答「库里有哪些主题簇、哪些区域互不相连」，诊断层定位知识缺口。只读、无副作用。

**响应**：

| 字段 | 类型 | 说明 |
|---|---|---|
| `modularity` | float | 模块度质量分（-1~1，越高社区越清晰；Leiden 自查） |
| `community_count` | int | 社区数 |
| `membership` | object | `nodeID → 社区ID`（不含孤立节点——孤立节点由 stats.orphans 承接，不单独成社区） |
| `communities[]` | array | 社区列表（按 Size 降序）：`{id,size,nodes[],titles[]}`（titles 为度 TOP8 代表标题） |

参数：`resolution`（默认 1.0，越大社区越碎）、`seed`（0=随机；固定值可复现）。
孤立节点（度=0）在检测前过滤。失败 → `{"error":"..."}`。

## 12. `GET /api/suggest-links?k=50` · 潜在关联待审清单（v0.1.13）

引擎从拓扑多算法估算"近似相关"的候选对（roadmap #15，backlog §3.6）——**有界、
标注算法与共享邻居证据、未落图**（kind=approx 的候选形态，不是真实边）。消费方 =
外部 AI / agent 研判（取候选 + 笔记正文判定，接受者写回 kind=ai 边）。只读、无副作用。

`k` 最大条数（默认 50，上限 200）。

**响应**：

| 字段 | 类型 | 说明 |
|---|---|---|
| `count` | int | 实际返回条数 |
| `results[]` | array | 候选对（聚合分降序——AA/RA/Jaccard 原始分加权求和） |
| `results[].a/b` | string | 端点 ID（a < b，无向对规范化） |
| `results[].a_title/b_title` | string | 端点标题 |
| `results[].score` | float | **聚合分** = AA·1.0 + Jaccard·0.5 + RA·1.0（原始分直接求和；弃用 Borda 名次分——跨锚点不可比/非确定） |
| `results[].algorithms` | string[] | 命中算法（`aa` / `jaccard` / `ra`） |
| `results[].shared` | string[] | 共享邻居 ID（证据："都链接了 X/Y"） |
| `results[].a_uri/b_uri` | string | 端点跳转（obsidian:// / orca-note://） |

候选生成有界：只对每节点 2-hop 邻域打分（图稀疏 O(N·d²)）；top-K 节流
（每节点 K=2，全局上限 ≤ 2N）。排除口径同 similar：自身/直接邻居/目录枢纽/
结构类型/空标题/孤立。co-touch 行为信号需插件 L1 经 touch 通道喂入，v0.1.13
暂为纯拓扑。

## 13. touch digest（v0.1.14，backlog §3.7）—— 行为信号子系统只读出口

> 引擎零写 vault：digest 内容经本组只读接口暴露，`serendipity-digest-*.md`
> 由前端插件在用户主动导出时写入（引擎保持"源数据权威原则"下零写 vault 边界）。

### 13.1 `GET /api/touch/digest` · 最新 digest 查询

只读、被动（仅主动查询返回；不主动推送、不弹窗）。无 digest → `{"digest":null,"available":false}`。

**响应**：

| 字段 | 类型 | 说明 |
|---|---|---|
| `digest` | object\|null | 最新 digest（无 → null） |
| `digest.id` | string | 唯一 id（unix 纳秒） |
| `digest.generated_at` | int | 生成时间（unix 秒） |
| `digest.window_start` | int | 窗口起点（上次 digest 时间，unix 秒） |
| `digest.since` | string | 窗口起点人读串 |
| `digest.total` | int | 窗口新增 touch 数 |
| `digest.targets[]` | array | TopN `{id,title,count}`（幽灵过滤 + 标题解析，降序） |
| `digest.sources[]` | array | TopN 来源词 `{id,count}`（降序） |
| `available` | bool | 有未读 digest（`digest_available` 的独立查询形态） |

### 13.2 `POST /api/touch/digest/ack` · 标记已读

请求体：`{"id":"<digest.id>"}`。响应：`{"ok":true}`。只写 touch store meta
（`last_ack_id`），不碰 touch 事件、不反馈排序（红线）。`available` / `digest_available`
随 ack 转 false。

---

## 14. 无库启动与配库（v0.1.15，壳/TUI/浏览器选库的地基）

> `seren serve` 不带 vault 时以**空库启动**：不解析、不建图，全部数据端点返回
> `{"error":"no vault configured","configured":false}`；`/api/stats` 返回
> `configured:false`。经本端点配库/换库后立即恢复全功能（换图 + 全套闭包
> 重建 + watch 重启，同一次 serve 进程内完成，无需重启）。

### 14.1 `GET /api/vault` · 查询当前配置

响应：`{"configured":bool, "source":"obsidian:…|orca:…|store:…", "vault":"<名>"[, "nodes":N, "edges":M]}`

- 未配库：`{"configured":false,"source":"","vault":""}`
- 已配库：追加 `nodes`/`edges`

### 14.2 `POST /api/vault` · 配库 / 换库

请求体：

```json
{"path":"<vault 目录 | OrcaNote.db 路径>", "profile_name":"<可选：内置画像>", "profile":"<可选：画像文件>", "store":"<可选：持久化路径覆盖>", "db":"<可选：从持久化存储读图>"}
```

- `path` 必填；其余可选（对应 CLI `--profile-name` / `--profile` / `--store` / `--db`，覆盖 serve 启动默认）。
- 语义：解析建图（Obsidian 目录 / 虎鲸 .db 自动识别，同 `seren index`）→ 替换内存图 + 重建全套闭包（refresh/touch/stats/digest/ack/available/is_pending）→ 重启 watch（如开启）→ revision +1。
- 已配库时再调用 = **换库**（旧图/闭包/watch 整体替换，幂等）。
- 成功：`{"ok":true,"configured":true,"source":"…","vault":"…","nodes":N,"edges":M}`
- 失败（路径不存在 / 解析失败）：`{"error":"<原因>"}`

---

## 15. MCP（v0.2.0 · serve 内嵌 /mcp 端点 + 状态/启停）

> serve 内嵌 MCP 服务（Streamable HTTP，端点 `/mcp`），Web+REST+MCP 三合一。前端/插件
> 经下面 REST 端点查询状态、启停 MCP。`/mcp` 本身走 Streamable HTTP 协议（POST 会话，
> 本地消费，除 Host 校验外不强制 token）；`/api/mcp/*` 走 token 鉴权（同 `/api/*`）。

### 15.1 `GET /api/mcp/status` · 查询 MCP 状态

响应：

| 字段 | 类型 | 说明 |
|---|---|---|
| `enabled` | bool | `/mcp` 是否可访问（启停开关） |
| `configured` | bool | 是否已配库（未配库时 MCP 工具给引导） |
| `tools` | int | 工具数（九件套） |
| `transport` | string | `streamable-http` |
| `endpoint` | string | `/mcp` |

未注册 MCP（旧服务）→ 端点不存在（404）。

### 15.2 `POST /api/mcp/enable` · 启用 `/mcp` 端点

响应：`{"ok":true,"enabled":true}`

### 15.3 `POST /api/mcp/disable` · 停用 `/mcp` 端点

响应：`{"ok":true,"enabled":false}`（停用后 `/mcp` 返回 404）

---

## 变更登记

| 版本 | 变更 |
|---|---|
| v0.1.0 | 初版 stats/hot/roam/页面 |
| v0.1.1 | 新增 hot/config；roam 加 match 级别 |
| v0.1.2 | 新增 /api/refresh |
| v0.1.4 | 新增 /api/touch；stats 加 revision |
| v0.1.5 | 新增 /api/relation；refresh 加 renamed/renames |
| v0.1.6 | /api/config + roam 可调参数（top/hops/lambda/theta/alpha/beta clamp）；relation 加 path_nodes |
| v0.1.7 | roam 加 random/seed/rand_alpha；config 加 rand_alpha |
| v0.1.8 | **全 API 加鉴权**（X-Seren-Token 头 / ?token=）+ Host 校验 |
| v0.1.11 | 新增 /api/similar（Jaccard）、/api/node（详情）、/api/touch/stats（只读统计）；roam 加 `?export=1`（text/markdown 卡片清单） |
| v0.1.12 | similar 评分升级 **Jaccard → Adamic-Adar**（度加权，抗枢纽偏置）；stats 加 `is_pending`（库变化待刷新）+ `dangling_refs`（悬空明细）；touch/stats targets 过滤幽灵 touch；**新增 /api/communities**（Leiden 社区发现，诊断层）；MCP tools 扩至 7（+graph.community） |
| v0.1.13 | **新增 /api/suggest-links**（潜在关联待审清单：2-hop + AA/Jaccard/RA 聚合 + top-K 节流，未落图）；存储层 SQLite → bbolt（扩展名 .bbolt，无迁移） |
| v0.1.14 | **新增 touch digest 子系统（§3.7）**：/api/touch/digest（只读查询）+ /api/touch/digest/ack（已读）+ stats 加 `digest_available`；**MCP tools 扩至 8（+seren.touch_digest）**；touch 拆独立 store（`touch-<hash>.bbolt`，touch/meta/backups） |
| v0.1.15 | **无库启动 + 配库**：`seren serve` 无 vault 空库启动；**新增 /api/vault**（GET 查配置 / POST 配库换库，换图 + 闭包重建 + watch 重启）；stats 加 `configured`；数据端点未配库时返回 `configured:false`；前端导出改 fetch+blob（a 标签不带 token 被 auth 拒的修复） |
| v0.2.0 | **MCP 迁移 mcp-go + Streamable HTTP**：serve 内嵌 `/mcp` 端点（Web+REST+MCP 三合一，live 图）；**新增 /api/mcp/status + /api/mcp/enable + /api/mcp/disable**（前端/插件查状态与启停）；MCP tools 扩至 9（+seren.state）；`seren_orientation` prompt；suggest-links 聚合改原始分（修 Borda 跨锚点不可比/非确定）；CLI 用户可见输出英文化 |
| v0.2.1 | **mcp**：MCP tools 扩至 11（**+graph.suggest**、**+seren.touch_stats**）；graph.community 加 `top`（默认 10 裁剪社区 + membership）与 `node`（单节点归属）；graph.node 详情加 `text_len`/`text_truncated`；graph.suggest 输出补 `a_title`/`b_title`（反馈 #5）；graph.stats 附带 `dangling_refs` 明细（反馈 #8）；**/mcp 会话兼容**（反馈 #9：宽松 SessionIdManager，未回传/错传 `Mcp-Session-Id` 不再 404 "Invalid session ID"，修 .NET HttpClient 类客户端）。**graph**：Build 加 title/alias 重定向（非精确 ID 的 `[[名字]]` 落到拥有该名节点，不再丢成 dangling）。**adapter**：parseLinks 剔代码块/行内代码、处理表格反斜杠、过滤占位符；VaultProfile 加 `excluded_prefixes`（文件名前缀级排除，贯穿 watch）；**画像 onboarding**：ResolveProfile 在 per-vault `.serendipity/profile.yaml` 缺失且 vault 为 Obsidian 目录时，自动落一个全注释的 llm-wiki 模板（`ObsidianProfileTemplate`），用户按需取消注释启用。**store**：**解析失效**（`SaveParserVersion`/`LoadParserVersion` 记 meta bucket + `SaveProfileSignature`/`LoadProfileSignature` 记画像签名）——loadSource/refreshParse 检测**binary 版本或画像签名**变化即强制全量重析，否则增量复用 mtime/size 未变文件会让升级后的解析规则/画像排除不生效（曾导致反斜杠 dangling 残留、新增 log/index 排除后图不更新）；改画像后引擎自动重析，无需手动 seren index。**web/touch**：**结构性文件 touch 跳过**——handleTouch 记录前对照画像排除（文件名精确/前缀，basename 与带 .md 两种形态），log/index/.ingest-report-/health_ 等工具文件不再污染 touch 权重（反馈：log 权重异常大）；与图解析排除同源（画像当唯一真相源）。**web/rebuild**：**新增 `POST /api/rebuild`**（全量重建，忽略增量强制重析整库，等价 CLI `seren index`）——GUI/插件"重建索引"按钮用；Web UI 设置抽屉已加该按钮。**adapter/bugfix**：`ExcludedName` 对 `.md` 后缀免疫（画像里写裸名 `log`/`index` 也能排除 `log.md`/`index.md`，此前只做全名精确匹配，裸名排除不生效）。**serve**：新增 `--pid-file <path>`——启动原子写入自身 PID（覆盖陈旧文件），优雅退出（SIGINT/SIGTERM）时删除；供 managed 启动方（Obsidian 插件）可靠获得/清理进程句柄（配合插件侧自愈，治"关宿主不杀进程/孤儿占端口连到旧引擎"）。 |

> 参考：/api/roam 的 random 走的是 `roam.ComputeRandom`（随机层），其它查询走
> `roam.Compute`；两者共用同一簇管线（clusterFromSeeds）。内核语义见
> `docs/architecture/03-engine.md`。
