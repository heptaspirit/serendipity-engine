# 前端问题记录与交接（Web UI 测试）

> ⚠️ **历史记录（已归档）**：本文记录了 v0.1.3–v0.1.6 前端 Web UI 的历史问题、根因、修复与验证，以及遗留待办。文档主体已迁移为 [`docs/frontend.md`](../frontend.md)（前瞻计划 + 测试速查），测试方法速查表与已知环境限制已在其中折叠保留。

> 面向后续**前端专项 session**（可能是新开的独立会话，无本会话上下文）：
> 本文件记录 Web UI 的历史问题、根因、修复与验证状态，以及遗留待办与测试方法。
> 配合 [`docs/architecture/05-web.md`](../architecture/05-web.md) 阅读——那是架构视角，
> 本文是**问题 / 测试视角**。前端源码：`internal/web/static/index.html`（单文件，
> 零依赖原生 JS，go:embed 嵌入）。

## 0. 快速上手（起服务）

```powershell
# 虎鲸库（TestOrca，推荐，数据丰富）
seren serve "D:\WorkSpace\NoteLib\TestOrca\TestOrca.db" --port 8910 --store <临时store.sqlite> --repo TestOrca

# Obsidian vault（另备测试库）
seren serve "D:\WorkSpace\WriteLib\Novel_AI_Helper" --port 8901
```

- 浏览器打开 `http://127.0.0.1:8910/`。
- **每次改 `index.html` 后必须硬刷新**（Ctrl+Shift+R）——页面已设
  `Cache-Control: no-store`，但浏览器/代理仍有极低概率缓存，硬刷新排除干扰。
- 服务端日志会打印监听/刷新事件；`/api/stats` 返回 revision（图版本号）。
- 后端改动后需重新 `go build` 并重启 serve（同一端口要先杀掉旧进程）。

## 1. 已修复问题（防回归清单）

以下问题均在 v0.1.3-v0.1.4 修过；**回归测试请重点覆盖**：

### 1.1 "页面#N" 噪声（v0.1.3 修复）
- **现象**：虎鲸库扫出大量 `页面#N` 空壳页面，对用户无意义。
- **根因**：虎鲸页面块聚合后产生无别名、无正文、零引用的空壳文档。
- **修复**：adapter 层过滤空壳（TestOrca 538→234）+ 空壳容器标 `container`
  类型并加入 `structural_types` 排除（图结构保留、漫游/气泡不显示）。
- **验证**：8910 全库 0 个 `页面#N`；hot 气泡全为真实书名/章节。

### 1.2 点击节点"没反应"（v0.1.3 补丁修复，三层根因）
- **现象**：点击搜索结果/气泡里的节点无响应。
- **根因 1（主因）**：搜索结果的"锚: 征引文献"是**纯展示 span，没绑点击事件**。
- **修复 1**：锚点加 `data-id` + 事件委托，点击即继续漫游。
- **根因 2**：孤立节点（无邻居）→ 降级 ModeSparse 全文搜**纯数字 ID** → 必空，
  表现像"没反应"。
- **修复 2**：`roam.go` ModeSparse 降级搜索词优化——查询词为纯数字且唯一锚定时
  改用锚点 title 全文搜索（孤立节点也能找到正文提到它的内容）。
- **根因 3**：store.Load 遇"存在但从未写入"的存储文件报错（服务起不来）。
- **修复 3**：Load 兼容空库文件（无 documents 表 → 视为无旧状态）。
- **验证**：征引文献 → 锚定 + 3 结果；孤立节点 card 点击 → 降级 title 搜索命中。

### 1.3 从初始界面点进去无反应（v0.1.3 修复）
- **现象**：初始页漂浮气泡点击后无反馈。
- **根因**：气泡也是纯展示元素；且孤立节点无下文时没有任何提示。
- **修复**：document 级**事件委托**统一处理 `.card, .bubble, #anchors .anchor[data-id]`
  （任何时刻渲染的元素都可点，杜绝漏绑定；`a` 链接排除）；孤立节点（fallback=2
  且全文无命中）显示醒目横幅 **"⚠ 这个节点到头了"**；点击瞬间 meta 区显示
  "漫游中: <节点> …" 即时反馈；`window.onerror` 把 JS 错误显示在页面上（不静默）。

### 1.4 浏览器缓存旧页面（v0.1.3 修复）
- **现象**：改了 index.html，浏览器还显示旧版（"点了没反应"假象）。
- **根因**：无 Cache-Control，浏览器缓存页面。
- **修复**：`/` 响应头 `Cache-Control: no-store`。
- **验证**：改页面后普通刷新即可见新版（仍建议硬刷新）。

### 1.5 锚点点击语义（v0.1.4 加固）
- **现象**：锚点点击后输入框显示的是节点 ID（纯数字），人看不懂。
- **修复**：输入框显示节点**标题**（查询词仍是 ID，保证精确锚定）。
- **说明**：锚点点击 = 用该节点 ID 继续漫游；标题显示只是展示层。

### 1.6 自动更新提示（v0.1.4）
- 前端每 30s 轮询 `/api/stats`，revision 变化 → 提示"库已自动更新"。
- **验证**：改库文件后（Obsidian 改笔记 / 虎鲸改库）等监听节流窗口（默认 60s）
  触发自动刷新，前端应出现提示。

## 2. 遗留待办（后续 session 的活）

### 2.1 Playwright 前端自动化测试（用户将安装环境）
- 目标：主动回归 1.x 全部修复项 + 漫游/刷新/跳转交互，替代人工点测。
- 建议用例：
  1. 初始页气泡点击 → 漫游结果渲染；
  2. 锚点点击 → 继续漫游；
  3. 孤立节点点击 → "⚠ 这个节点到头了"横幅；
  4. 卡片「打开 ↗」→ 跳转 URI（虎鲸 `orca-note://`）；
  5. 手动 `↻` 刷新 → diff 摘要显示 + revision+1；
  6. 自动监听：改库文件 → 60s 内 revision+1 且页面出现"库已自动更新"；
  7. 浏览器后退/前进（history 栈）与 `?q=` URL 同步。

### 2.2 obsidian:// 真机跳转验证 ✅（2026-08-23 用户手动确认）
- `obsidian://open?vault=<名>&file=<相对路径>` 由用户在手头 Obsidian 实测跳转
  **正常**，此项从待办划掉。后续前端自动化回归仍可覆盖（验证卡片「打开」链接
  生成的 URI 格式，见 §3 跳转 URI 行）。

### 2.3 score≤0 节点的展示策略（v0.1.6 已修后端，前端待回归）
- **现象**：深跳配额强制纳入的节点常 score=0（min-max 归一化后远锚点分数趋 0），
  卡片上显示 0 分难看且误导。
- **已修（v0.1.6）**：后端打分改**桶内归一化**（按跳数分桶各自 min-max；单调
  变换，不改桶内排序 → 输出序列不变），深跳节点分数有桶内区分度，不再整体为 0。
- **前端待办**：回归验证卡片分数展示（旧 store 需重跑一次 refresh 才有新分数）；
  若仍想突出 serendipity 节点，可后续加"惊喜"徽标（用结果里的 `hops` 字段）。
- 注意：深跳节点是 serendipity 机制的设计目标，**不能砍掉**，只修展示。

### 2.4 测试文档污染 2-3 跳槽位（v0.1.5 定性验证发现）
- **现象**："今天进行一个测试文档"链反复占 马背上的朝廷 系查询的 2/3 跳位。
- **性质**：数据问题（测试文档造出假枢纽链），非算法 bug；可提示用户清理
  TestOrca 里的测试文档，或前端对特定类型/路径做展示过滤（需先决策）。

## 3. 测试方法速查

| 目标 | 方法 |
|---|---|
| 起服务 | `seren serve <库> --port 8910 --store <store> [--repo TestOrca]` |
| 硬刷新 | Ctrl+Shift+R（排除缓存干扰） |
| 看 revision | `GET http://127.0.0.1:8910/api/stats` |
| 漫游 API | `GET /api/roam?q=<词>&top=<N>`（锚点/结果/路径/降级都在 JSON 里） |
| 关系 API | `GET /api/relation?from=<ID或标题>&to=<ID或标题>`（最短路径+PPR+证据） |
| 手动刷新 | `POST /api/refresh?limit=50`（响应含 added/updated/deleted/renamed） |
| 埋点 | `POST /api/touch {"target":"7128","from":"10156"}` → store touch 表 |
| 跳转 URI | roam 结果里每项的 `uri` 字段（orca-note:// 或 obsidian://） |
| 自动监听 | 改库文件 → 等 60s 节流窗口 → revision+1 + 页面提示 |

## 4. 已知环境限制

- **沙箱内 Edge headless dump-dom 不可用**（piped stdio 被沙箱 EPERM 拦截）——
  前端自动化需用户安装 Playwright 后在**无沙箱环境**跑。
- 虎鲸活库被 App 独占锁，serve 读取走一致性快照（`CopyDBForRead`），
  测试时改库文件请通过虎鲸 App 操作或改副本。
- 多实例冲突：同一 store 文件别开两个 serve；改代码重启前先杀旧进程。

## 5. 修改记录

- 2026-08-23 初版：整理 v0.1.3-v0.1.5 前端问题史 + 遗留待办（交接给前端专项 session）。
- 2026-08-23 v0.1.6 前端专项（交接给下一前端 session，追加记录）：
  - **换肤**：采用 dsh web 同款 design language（Tokyo Night 暗色；`internal/web/static/index.html`）。
  - **可调参数白盒**：`GET /api/config` 返回 top/hops/lambda/theta/alpha/beta 白名单（含范围/
    步长/默认/hint）；`GET /api/roam` 接受这些参数并按 clampInt/clampFloat 钳制安全边界。
    前端新增「⚙ 参数」设置抽屉（分组滑块 + 重置默认 + localStorage 记忆，改动后自动重查）。
  - **关系查询 UI**：新增「关系」面板，接入 `GET /api/relation`；`handleRelation` 补充
    `path_nodes`（路径节点 ID→标题），前端渲染可点击路径药丸链 + affinity/激活/ppr + 证据链。
  - **深跳展示**：前端对 score≤0 节点不再显示 "score=0.000"，改用「深跳」标签（serendipity
    机制保留）；与后端桶内归一化（另一 session v0.1.6）互补。已用 ego browser 回归验证。
  - **回归验证**：已用 ego browser（http://127.0.0.1:8915，TestOrca）验证换肤初始页/
    设置抽屉/参数改后重查/重置默认/关系路径/卡片续漫游+历史栈/刷新摘要，全部通过。
