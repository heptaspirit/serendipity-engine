# 引擎设计路线图（Serendipity Engine）

> 建立：2026-08-23。面向未来维护者；取代 README「状态」里零散的"下一步"。
> 原则（用户拍板 2026-08-23）：**先完善引擎核心；插件化是远期可选分发形态，不锁架构**（design §6.8）。
> 决策背景见 [plugin-evaluation.md](plugin-evaluation.md)。

---

## 里程碑总览

| 里程碑 | 内容 | 状态 |
|---|---|---|
| **M0** | 插件前置（serve 安全）+ API 契约 + **MCP v3 提前** | 下一步，一起做 |
| **M1** | 引擎核心完善（测试 / 反馈观察 / 性能） | M0 之后 |
| **M2** | 插件薄壳（两个独立仓库，远期） | 核心完善后 |

## M0：插件前置 + MCP（2026-08-23 拍板提前）

> 缘由：插件薄壳已拍板（远期），但它的上线前提——serve 鉴权、API 契约——与 MCP v3 都属于"引擎侧自己的事"，提前做掉，互不阻塞、也为远期插件铺路。

### 0.1 serve 安全前置（薄壳上线前提，也提升 Web 安全性）

- [ ] **token 鉴权**：serve 启动生成/持久化随机 token（本地文件）；前端页面注入；API 校验（Header 或查询参数）。现状：无鉴权。
- [ ] **Host 头校验**（防 DNS rebinding）。
- [x] localhost 绑定已就绪：`cmd/seren/main.go` 固定 `127.0.0.1:<port>`（L516）。

### 0.2 API 契约文档

- [ ] `docs/api-contract.md`：现有 7 端点（stats / hot / roam / relation / config / refresh / touch）的请求/响应结构 + `version`/`revision` 字段说明。**这是插件仓库与引擎的唯一共享物**；改 API 必须同步本文（列入维护指南）。

### 0.3 MCP server v3（研究稿 `docs/architecture/07-mcp.md` 已就绪）

- [ ] 最小 stdio JSON-RPC：`initialize` / `tools/list` / `tools/call`（倾向自实现薄协议，保持零第三方依赖；若 SDK 生态明显成熟再权衡）
- [ ] `seren mcp` 子命令：`--db <store.sqlite>` 启动建图；只 import `internal/{graph,roam,adapter,store,score,sync}` 纯库，**不碰** `internal/web` / `internal/watch`（不影响本体）
- [ ] 只读三件套 tools：`graph.stats` / `graph.roam` / `graph.relation`（白盒输出，全部只读，不写 touch、不触发 refresh）
- [ ] dsh 联调：MCP 配置指向 `seren mcp --db <store>`，验证 `graph.roam` / `graph.relation` 返回可读
- [ ] 文档与发布：07-mcp.md 更新为"已落地"，补 README 入口 + 版本记录

## M1：引擎核心完善（M0 之后）

- [ ] Playwright 前端自动化测试（原 README「下一步」）
- [ ] JSON 契约测试（曾因 Go 结构体缺 json tag 导致 Web 全 undefined——大小写问题）
- [ ] 反馈闭环观察（touch 已埋点；"越用越准"是否演化边权——继续观察，不承诺，克制原则优先）
- [ ] 性能/规模（v1.5 关注）：数万节点（虎鲸块级）加载时间与 PPR 迭代耗时验证；SQLite 全量写放大 → 启动对账 + 增量写
- [ ] 更多查询集定性记录（决策 #6，真实工作流 10 个查询累积）
- [ ] 锚点同级别排序稳定（Resolve 同 level 依赖 map 遍历序，低优先级）

## M2：插件薄壳（远期，核心完善后；方案与工作清单见 plugin-evaluation.md）

- [ ] `serendipity-obsidian` 独立仓库：ItemView iframe + 探测/拉起 seren + `openLinkText` 跳回 + 事件节流刷新 + `isDesktopOnly: true`；tag + Actions 发布
- [ ] `serendipity-orca` 独立仓库：ViewPanel iframe + `invokeBackend` 跳回；zip 发布
- [ ] 两个仓库与引擎关系：**运行时契约引用、零构建时依赖**；唯一共享物 = `docs/api-contract.md`（插件内 `seren-api.d.ts` 手写副本注明以契约为准）

## 明确不做（坦诚声明）

- **TS 移植 / Go→WASM**：双重维护风险 > 收益，否决（2026-08-23）
- **移动端（iOS/Android）**：无 sidecar 环境；唯一干净解是 TS 移植（已否决）。插件 `isDesktopOnly` + README 声明，欢迎 fork 移植
- **远程/云模式**：踩"个人数据不出本机"安全红线，除非用户显式要求

---

## 版本记录

| 日期 | 变更 |
|---|---|
| 2026-08-23 | 建立；M0（安全前置 + 契约 + MCP 提前）、M1（核心完善）、M2（插件薄壳）；决策背景见 plugin-evaluation.md |
