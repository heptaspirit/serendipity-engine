# 适配器层 · 格式翻译（internal/adapter）

> 面向未来维护者：adapter 是"每个笔记软件一个翻译器"。**加新数据源 = 加一个
> adapter 文件实现 ParseXXX，内核全不动**（扩展点见 06-maintenance）。
> 安全红线在本文件也最集中——改虎鲸相关代码前必读。

## 1. 职责边界

- **内核只认识 `Document`**（见 01-data-model）；adapter 负责"格式翻译"。
- **通用语法代码固定，语义映射走画像**（哲学 #3）：
  - `obsidian.go`：`[[...]]` 维基链接、标准 markdown 链接（OKF，目标须 .md）、
    frontmatter、H1——四海皆准，代码写死；
  - `profile.go`：title/别名/标签键、类型推断、排除目录——因人而异，YAML 画像。

## 2. Obsidian（`obsidian.go`）

- `ParseVault(root, profile)`：递归扫 `.md`，跳过 `profile.ExcludedDirs`。
- `ParseFile`：frontmatter（title/别名/标签/类型）+ 链接 + OKF 元数据。
- **OKF（Open Knowledge Format，Google Cloud 2026）**：默认解析内置
  `type_field="type"`（frontmatter type 值 = 节点类型）、`description_keys` /
  `resource_keys`（并入正文可全文检索）、markdown 链接入图；`--profile-name okf`
  与 default-obsidian 等价。`index.md/log.md` 不默认结构类型化（真实库可能是正文）。
- **同名文件消歧**：ID 撞车时从第二个改用相对路径 ID（v0.1.3，见 01-data-model）。
- frontmatter 由 yaml.v3 解码（v0.2.2 起，替换手写 mini-YAML）：支持块标量
  （`|`/`>` 多行值）、引号转义、嵌套缩进列表；键字符集与旧版一致
  （`^[A-Za-z_][A-Za-z0-9_]*$`），标量保留源码文本（不转型）。
- 已知限制：wiki 链接大小写敏感。

## 3. 虎鲸 Orca Note（`orca.go`）—— 安全红线集中地

### 数据模型（对真实库实测）

虎鲸无笔记文件，数据在库根 `OrcaNote.db`（SQLite，WAL）。块（Block）是唯一实体，
**"页面块 vs 内容块"的区别在 `content` 列**：

| Block.content | 含义 |
|---|---|
| `NULL` | **页面块**（一个页面 = 一个文档；可嵌套） |
| JSON 数组（`{"t":"t","v":"文本"}` / `{"t":"r","v":块id}`） | **内容块**（一段 = 一个块，从属于页面） |

`text` 列 = 已解析纯文本（内联引用已渲染成 `[目标标题]`）；`left` = 前兄弟指针
（页内顺序链）；`BlockRef(f,t,type,alias)` 三种引用；`BlockAlias(name,block)` = title。

### 聚合策略（v0.1.1-v0.1.3 演化）

图节点粒度 = **页面**（app 里用户看到的就是页面）：

1. **文档根** = 页面块（content NULL）∪ **链顶内容块**（parent 无效且无页面祖先，
   如 epub 剪藏书根——v0.1.2 修深层树碎片：21805→538 文档）；
2. 文档 Text = 自身 + 非文档根后代按 `left` 链拼接；BlockRef 归一化到所属文档根
   → 页面间无向边（页内自环丢弃）；嵌套页面独立文档 + 包含边；
3. **空壳清理（v0.1.3）**：无别名 + 无文本 + 无引用的文档直接不进图（TestOrca
   304 个）；无内容但有结构引用的标 `container` 类型（默认结构类型，漫游/hot 排除）；
   过滤后清理宿主 Refs 中指向空壳的悬空目标。

### 快照（`CopyDBForRead`）——双路径

虎鲸 app 常以**独占锁**打开活库（实测：连 SELECT 都被锁，仅 immutable 可读）：

1. **VACUUM INTO** 优先：一致性快照（含未 checkpoint 的 WAL）。需要 `to_pinyin`
   **确定性函数 stub**（BlockAlias 生成列 `name_p` 校验需要，否则报 malformed）；
2. 锁定时回退**文件拷贝**（先 `.db-wal` 后 `.db`）+ 打开时自动 WAL recovery +
   `PRAGMA integrity_check` 校验（损坏重试一次）。
3. 锁探测（`busy_timeout=0` 的 SELECT 1）决定路径，避免白等。

### 安全红线（不可违反）

- **`Repo` 表含用户凭据（DeepSeek API key、对象存储密钥等）——本适配器绝不查询**；
  插件表同理。图数据只从 Block/BlockRef/BlockAlias 派生。
- 绝不直接读活库（先快照）；快照文件用完即删。
- 用户个人数据（真实 vault 内容）不进 git；`scratch/` 整个 gitignore。

## 4. VaultProfile（`profile.go`）

```go
type VaultProfile struct {
    Name, ExcludedDirs
    TitleKeys, AliasKeys, TagKeys     // frontmatter 键优先级
    TypeByDir/Key/Prefix/Ext []TypeRule // 类型推断规则
    TypeField string                   // OKF：frontmatter 键，值 = 节点类型
    DescriptionKeys, ResourceKeys      // OKF：并入正文
    DefaultType, StructuralTypes       // 结构类型：实体查询默认排除
}
```

- `DefaultObsidianProfile()`：内置默认（含 OKF 字段、structural `container`）。
- `LoadProfile`：缺失字段用默认补齐（防御性校验）。
- `profile-detect`：扫描 vault 普查 frontmatter 键/目录/文件名前缀，产出画像骨架，
  机器产出、人定语义。
- 用户真实画像（novel-wiki.yaml 等）gitignore、本地留存（embed 编译时仍可用）。
