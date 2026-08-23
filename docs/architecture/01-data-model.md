# 数据模型 · 统一图格式与持久化

> 面向未来维护者：本文描述内核唯一的格式抽象（Document）、内存图结构、
> SQLite 存储布局与对账/埋点的数据落点。改数据结构前先读这里。

## 1. Document（统一图节点，`internal/adapter/document.go`）

内核只认识 `Document`——每个笔记软件一个 adapter 负责把自己的格式翻译成它
（Linux VFS 哲学：内核定义抽象，adapter 实现抽象）。

```go
type Document struct {
    ID      string    // 身份：Obsidian 文件名（不含 .md）/ 虎鲸块 ID（数字字符串）
    Title   string    // 语义名：锚定与展示用（与 ID 分离，纯编号文件名的解法）
    Aliases []string  // 别名（frontmatter aliases / 虎鲸 BlockAlias）
    Type    string    // 节点类型：画像规则推断 / OKF type 字段 / orca 的 doc/block/container
    Path    string    // 相对路径（Obsidian）/ "block/<id>"（虎鲸）
    MTime   time.Time // 对账用
    Size    int64     // 对账用
    Refs    []string  // 双链 → 其他 Document ID（无向边数据源）
    Tags    []string  // 标签（锚定通道）
    Text    string    // 全文（去 frontmatter）——全文 LIKE 兜底用
}
```

### ID 约定与坑

- **Obsidian**：`ID = 文件名（去 .md）`。**不同目录同名文件会撞 ID**——v0.1.3 起
  冲突的（从第二个）改用相对路径作 ID（`源文件归档/设定/剧情总纲`），两个都进图。
  代价：`[[短名]]` 链接只匹配 basename 节点（已知限制）。
- **虎鲸**：`ID = 块 ID（数字字符串）`，稳定不复用。`Path = "block/<id>"`——Web 端
  靠这个前缀识别虎鲸源（--db 回读时）。
- **ID 变化语义（v0.1.5 起支持改名迁移，修订 #8）**：Obsidian 改名在 diff 眼里
  = 删一个 + 加一个；`sync.DetectRenames` 按"内容哈希（Text 相同）+ 路径相似度
  （同目录优先）"把二者配对为 rename，从 deleted+added 中拆出（记 `Renamed`）。
  虎鲸块 ID 数字稳定且改名不换 ID，永不配对（守卫见 `sync.isNumericID`）。
  改名是**持久身份事实**：映射存 `renames` 表（见 §3），每次刷新合并应用。

## 2. 内存图（`internal/graph`）

```go
type Graph struct {
    nodes      map[string]*Node   // ID → Node{ID, Title, Doc}
    adj        map[string][]string // 无向邻接（pairKey 去重）
    dangling   map[string]int     // 解析到但节点不存在的链接目标
    totalLinks / selfLinks / multiedge  // 链接账目
}
```

- **边语义**：v1 全部无向（设计修订 #2；单向信息仅统计保留）。
- **悬空链接**（`dangling`）：`[[不存在]]` 的引用——不建边，仅统计（`Stats.Dangling`）。
  orca 过滤空壳时宿主 Refs 里指向空壳的目标会被一并清理（见 04-sync）。
- **Hub 排除**：`Stats().Nodes / 2` 度以上的节点在漫游中被当"目录枢纽"排除。

## 3. SQLite 存储（`internal/store`）

默认路径 `<vault>/.serendipity/db-<库路径 hash12>.sqlite`（`DBPath`），WAL 模式。

| 表 | 内容 | 写入方式 |
|---|---|---|
| `documents(id PK, title, type, path, mtime, size, tags, aliases, text)` | 图节点 | **Save 全量重写**（DELETE + INSERT 于一个事务，幂等） |
| `links(a, b, weight, PK(a,b))` | **有向引用行**（v0.1.5 修正：a 链接 b，保方向） | 同上 |
| `touch(id AUTOINC, ts, target, src)` | 反馈埋点（点击记录） | **AppendTouch 增量**，Save 不清除；容量上限 5000 条（超限删最旧） |
| `renames(old_id PK, new_id)` | 改名迁移映射（v0.1.5，修订 #8） | **SaveRenames 全量重写**（随每次刷新） |

### 关键语义

- **links 有向（v0.1.5 修正，对账收敛前提）**：`Document.Refs` 有向（本文档链接谁），
  diff 按文档逐一比较 Refs，存储必须保方向。v0.1.5 前用排序 pairKey 去重（无向对），
  Load 只把 b 追加到 a 的 Refs——字典序较大的端点回读后 Refs 为空，每次刷新报
  虚假 "refs +1" 永不收敛。修正后 Save 按精确 (a,b) 去重，Load 原样回读；
  **无向语义只在 graph.Build 层体现**（双方入邻接表）。
- **Save 幂等**：任何一次 refresh 后存储即最新状态，重复刷新无副作用。
- **Load 容错**：文件不存在 → `nil, nil`（首次对账全新增）；文件存在但从未写入
  （无 documents 表）→ 同样视为无旧状态（v0.1.3）。
- **touch 独立**：Save 只 DELETE documents/links，不动 touch——埋点数据跨刷新保留。
  `from` 列名避开了 SQL 保留字（曾用 `from` 导致语法错误，v0.1.4 改 `src`）。
- **documents 存"文件真相"**：Save 写原始 Refs（不做改名重定向），对账 diff 才能
  收敛；身份迁移（改名）在**建图层**叠加（`redirectForGraph`），由 renames 表驱动。

## 4. 对账 diff（`internal/sync`）

`sync.Diff(old, cur []*Document) *Result`：按 ID 对齐新旧集合。

- 旧有新无 → `deleted`；新有旧无 → `added`；两边都有 → 比较内容指纹
  （Title/Type/Path/Tags/Aliases/Text/Refs，**不含 MTime/Size**——内容即真相，
  touch 不改内容不算更新）。
- **改名（v0.1.5）**：deleted × added 按内容哈希 + 路径相似度配对 → `renamed`，
  从 deleted/added 计数中拆出（`Result.Renamed/Renames`）；拿不准（并列候选）
  宁可不判，退回 deleted+added。
- **规范化**：Tags/Aliases/Refs 比较前排序——adapter 输出顺序受 map 遍历/SQL 行序
  影响，不排序会误报 updated。
- 字段级变化明细（`Change.Fields`）+ 引用增减（`AddedRefs/RemovedRefs`）。

## 5. 配置（VaultProfile，`internal/adapter/profile.go`）

见 `02-adapters.md`。要点：YAML 画像声明"语义映射"；缺失字段用通用默认补齐
（`LoadProfile`）；`--profile` 文件 > `--profile-name`（default-obsidian / okf /
example-wiki）> `<vault>/.serendipity/profile.yaml` > 内置默认。
