# 引擎层 · 图 / 打分 / 漫游管线

> 面向未来维护者：漫游管线是产品的核心（`internal/roam`），图与打分是它的零件。
> 改算法前先读本文件 + `docs/spike-report.md`（参数实测来源）。

## 1. 漫游管线（`roam.Compute`）—— 全流程

CLI 与 Web 共用同一管线（`roam.Options{Top, Hops, Lambda, Theta, Alpha, Beta, FilterStructural}`）：

```
锚定（graph.Resolve）
  → PPR（结构分）+ Activate（激活分）双通道并行
  → 排除：种子 + 目录枢纽（度 ≥ 半数节点）+ 结构类型（FilterStructural）
  → score.Rank：min-max 归一化 → 线性融合（0.5·PPR + 0.5·Act）→ 跳数配额混合
  → 降级兜底（见 §4）
```

### 锚定（`graph.Resolve`）—— 五级匹配

按级别降序返回全部命中（级别用于展示筛选"只显示重的几个锚点"）：

| 级别 | 匹配方式 |
|---|---|
| 5 MatchExact | ID 精确 |
| 4 MatchTitle | title 精确 |
| 3 MatchAlias | 别名精确 |
| 2 MatchTag | 标签精确 |
| 1 MatchLike | ID/title 子串 |

注意：短 ID（虎鲸数字，如 "1"）会子串命中大量节点——多锚点漫游是预期行为。

### 结构分（`graph.PPR`）—— 查询锚定的 Personalized PageRank

- teleport 0.15，60 次迭代；种子概率按匹配权重分布。
- 悬空节点把概率按种子分布回退（简化均匀回跳）。

### 激活分（`graph.Activate`）—— 激活扩散

- λ=0.7（衰减，spike 实测 0.6~0.7）、θ=0.1（剪枝阈值）、maxHops=3。
- 首次到达的最短路径记录为激活路径（白盒展示用）。

### 融合 + serendipity（`score.Rank`）

- 每维独立 min-max 归一化后再融合（避免量纲失衡）。
- **跳数配额**（serendipity 旋钮）：1:2:3-hop = 0.5/0.3/0.2，桶内 round-robin
  交错——保证"我没想到但确实相关"的深跳惊喜稳定出现。
- 依赖分 δ 与热度分 γ 独立于本排名（设计修订：δ=0 不参与）。

## 2. 排除规则（`roam.Compute` 内）

- **种子**：锚点自身不进簇（降级模式除外）。
- **目录枢纽**：`deg ≥ Nodes/2` 的节点（如 Obsidian 的 index 汇总页）。
- **结构类型**：`profile.StructuralTypes`（Obsidian 的章节/大纲/索引…；虎鲸的
  container）。仅 `FilterStructural=true` 时（实体查询）。

## 3. 降级兜底（决策 #10，实测刚需）

| 模式 | 触发 | 行为 |
|---|---|---|
| ModeNoAnchor (1) | 无锚点 | 全文 LIKE 搜索（不过滤结构，搜索式交互） |
| ModeSparse (2) | 锚定但邻居稀疏/簇为空 | 全文 LIKE 兜底（过滤结构） |

**v0.1.3 细节**：Web 点击卡片漫游的查询词是节点 ID（虎鲸为纯数字）——ModeSparse
时全文搜数字必空（"点了没反应"）。修复：查询词为纯数字且唯一锚定时，降级搜索改用
**锚点 title**（`isNumeric` 判断）。

## 4. 统计（`graph.Stats`）

节点/边/链接账目（含自环/重复/悬空）/孤儿/连通分量（并查集）/top 枢纽（按度）。
`index` 与 `/api/stats` 共用。**注意**：`Edges = Σdeg/2`（曾误算成节点数，已修）。

## 5. 参数速查（默认值，均可用 CLI flag 覆盖）

| 参数 | 默认 | 含义 |
|---|---|---|
| --lambda | 0.7 | 激活衰减 |
| --theta | 0.1 | 激活剪枝阈值 |
| --hops | 3 | 最大跳数 |
| --alpha / --beta | 0.5 / 0.5 | 结构分 / 激活分融合权重 |
| --top | 15 | 输出条数 |
| PPR teleport | 0.15 | 回跳概率（代码常量） |
| PPR iters | 60 | 迭代次数（代码常量） |
| 跳数配额 | 0.5/0.3/0.2 | 1:2:3-hop（代码常量） |
