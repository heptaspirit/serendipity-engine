# 插件开发计划（M2）

> **〔2026-08-26 停止开发〕虎鲸版本插件不开发（由于架构限制决定暂停开发）**。虎鲸为纯前端壳、无进程能力（external 模式强制）、且无插件市场分发；**内核已直读虎鲸库**（`seren index/roam/serve <库.db>` 照常可用）——虎鲸用户直接用内核即可完成同等功能，无需插件。M2 插件薄壳收敛为 **Obsidian 单壳**；本文虎鲸相关小节均已注释/标注停止，保留作调研溯源（完整调研见 plugin-evaluation.md，已归档）。

> 本文是插件开发的**执行计划**，承接 plugin-evaluation 的调研与拍板（已归档），把已定方向、平台能力、分发与生命周期汇成一份可执行的开发计划。
> **说明**：本文是计划文档，已确定的事项只记「决定 + 理由」，深入 API 细节见 plugin-evaluation（已归档，含逐字签名与全站文档链接）。开发时仍会有进一步研究（尤其 Obsidian 进程管理），故**不追求细节完备**。
> 关联：[`roadmap.md`](roadmap.md)（总路线图 M2）· plugin-evaluation（决策与平台调研全记录，已归档）· [`api-contract.md`](api-contract.md)（引擎 REST 契约）

---

## 一、总体定位

- **形态 = 插件薄壳**（D1）：面板 iframe 引擎自服务的 Web UI；引擎零代码改动，插件只做「发现引擎 → iframe → 跳回/刷新补丁」。
- **不做 TS/WASM 移植**（D2）：拒绝双重维护，引擎核心（Go）是唯一事实源。
- **移动端不做**（D3）：`isDesktopOnly: true` + README 坦诚声明，欢迎 fork 移植。
- **时机 = M2**：引擎核心（roadmap M0/M1）完成后启动——本文先定方向，避免届时重新调研。

---

## 二、已确定决策（简化：决定 + 理由）

| # | 决定 | 理由 | 深入位置 |
|---|---|---|---|
| D1 | 插件薄壳（iframe 引擎 UI） | 零引擎改动、UI 永与引擎版本一致、消灭前端双重维护 | plugin-evaluation §三/§五 |
| D2 | 不做 TS/WASM 移植 | 双重维护风险 > 收益 | plugin-evaluation §四 |
| D3 | 移动端不支持 | 无 sidecar 进程，唯一解是 TS 移植（已否决） | plugin-evaluation §四/§七 |
| D4 | 独立仓库，零构建时依赖（`serendipity-engine` / `serendipity-obsidian`） | 引擎/插件只运行时契约引用，不 submodule/vendor | plugin-evaluation §五 |
| D5 | 唯一共享物 = API 契约 | 引擎 `api-contract.md` + 插件侧副本 `seren-api.d.ts` | plugin-evaluation §五 |
| D6 | 运行时版本契约 | 连接时 `/api/stats` 比对 version，不匹配弹升级提示，无版本协商协议 | plugin-evaluation §五 |
| D7 | 插件化是 M2 动作 | 先引擎核心，后插件 | plugin-evaluation §一 |
| D8 | MCP 提前 | 与引擎安全前置（token 鉴权）同批 | plugin-evaluation §五 |
| Q1 | 后端三平台全出包 | Go 交叉编译近乎零成本，放弃 linux 反而让自动下载失败 | §四 |
| Q2 | 核心分发两 tier | v1 链 release 自下载（审查友好）；v1.x 加插件内下载按钮 | §四 |
| Q3 | 依赖=运行时契约引用 | 仓库独立，GitHub 上引擎出二进制、插件 README 写 requires | §五 |
| Q4 | 四态机 + coreManagement（Obsidian=managed） | Obsidian 可 spawn（managed） | §六 |

---

## 三、目标平台与开发预期

### 3.1 Obsidian（建议先做）

- **能力**（官方文档已确认）：桌面 Electron 有 Node API，可 `child_process` 拉起 seren（**managed 模式**）；`ItemView` 注册侧栏/标签页面板；`workspace.openLinkText()` 就地跳回笔记；`workspace.on(...)` / `vault.on(...)` 事件做隐式 touch 与 refresh 联动。
- **分发**（官方文档已确认）：
  - 仓库根需 `README.md` + `LICENSE` + `manifest.json`；GitHub release 的 **tag 必须与 manifest 的 version 一致**，release 上传 `main.js` / `manifest.json` / `styles.css`；经 community.obsidian.md 提交，自动化审核。
  - **manifest 必填字段**（已查官方 Manifest 规范）：`id` / `name` / `author` / `version` / `minAppVersion` / `description` / `isDesktopOnly`（**均必填**）。`id` 规则：仅小写字母+连字符、不能以 `plugin` 结尾、不能含 `obsidian`。我们的 `isDesktopOnly` 必为 `true`。
- **生命周期要点**（已查官方 Manage plugin lifecycle 指南，直接关系进程管理）：
  - `onload()` 配置资源，`onunload()` **必须释放**——官方明确列出「External connections（网络/WebSocket/子进程）须在 unload 清理」。→ 我们 spawn 的 seren 子进程**必须在 onunload 杀掉**（或借 `registerEvent` 自动清理），否则留孤儿进程。
  - 事件监听用 `registerEvent` / `registerInterval` / `registerDomEvent` 自动随卸载解绑。
- **难度**：中–高（进程管理 + 社区审核 + 侧栏视图），但能力最自然。

### 3.2 开发难度评估（Obsidian，深读结论）

| 维度 | Obsidian 插件 |
|---|---|
| 进程管理 | ✅ spawn seren（managed） |
| UI 呈现 | ItemView 侧栏/标签 + iframe |
| 跳回 | `openLinkText` |
| 隐式 touch | `workspace.on(...)` |
| 配置 | 宿主 settings |
| 分发 | 社区目录（审核） |
| 代码量/难度 | 中–高（API 面大、需管进程） |

**总判断**：插件工作约 **80% 胶水、20% 平台特化**（引擎核心 + Web UI + REST 契约共享）；Obsidian 难度最高（进程管理 + 审核 + 侧栏视图），但能力最自然，先做。

---

## 四、核心引擎分发（Q1–Q2）

- **Q1 多平台打包**：Go 交叉编译近乎零成本，`goreleaser` + GitHub Actions 一次出 **win-amd64 / mac-amd64 / mac-arm64 / linux-amd64** 四 asset（含校验和、自动挂 release）。**linux 不放弃**，仅非 x86 架构才兜底推荐自编译。
- **Q2 核心分发**：
  - **v1 主路径**：插件 README + 设置页链 GitHub release，用户自下载自运行（审查友好、零二进制进插件，契合 D4 纯壳）。
  - **v1.x 增强**：设置内「下载核心」按钮——取 latest release → 按平台选 asset → 下载落盘。Obsidian 端用 `fs` 技术可行；唯「启动核心」仍靠用户。

---

## 五、仓库结构与依赖（Q3）

- **两个独立仓库**：`serendipity-engine`（Go 核心 + Web UI + release 二进制）/ `serendipity-obsidian`；**零构建时依赖**（不 submodule、不 vendor）。
- **唯一共享物 = `docs/api-contract.md`**（7 端点 + version/revision）；插件侧放手写副本 `seren-api.d.ts`（文件头注明以契约为准）。
- **运行时版本契约**：插件连接时 `GET /api/stats` 比对 `version`，不匹配弹「请升级引擎到 vX.Y.Z」（D6，无版本协商协议）。
- **GitHub 体现**：引擎 repo 出二进制 release；插件 repo README 首行写 `requires serendipity-engine ≥ vX.Y`。依赖是**运行时契约引用，不是 git 关系**。

---

## 六、生命周期与状态机（Q4）

```
INSTALLED ──(获取内核: 下载/链接 + 填路径端口)──▶ CONFIGURED
CONFIGURED ──(二次确认"启动服务"弹窗)──▶ RUNNING ⇄ CORE_STOPPED
RUNNING ──(用户点"停用内核")──▶ DISABLED ──(重新启用)──▶ RUNNING
```

- **coreManagement**：**Obsidian = managed**——插件 spawn 核心进程、随宿主启停；onunload 杀进程（见 §3.1 生命周期要点）。
- **二次确认弹窗**（隐私红线，合规必需）：披露① 本地读取笔记 ② 本机 127.0.0.1 起服务 ③ 数据不出本机 ④ 随时可停用。
- **停用后**：前端变原生壳（不发任何 `/api/*`，配置保留），可随时恢复。

---

## 七、开发顺序与里程碑

- **M2-1 先 Obsidian**：能力最自然但最难，先啃硬骨头——打通 managed 进程 + ItemView + 跳回 + 隐式 touch + 社区提交流程。
- **预期**：插件工作约 **80% 胶水 / 20% 平台特化**（单端共享引擎核心 + Web UI + 契约）。

---

## 八、已知待确认点（开发时再查，不影响定调）

- **Obsidian**：
  - spawn 子进程最佳实践（PID 管理 / 宿主崩溃孤儿进程 / 启动时先探活复用而非重复拉起）。
  - 社区审核对「spawn 外部进程 + 监听端口」的具体反馈（Local REST API 已过审，可作参考先例）。
- **引擎前置**（M0 已列入）：serve token 鉴权 + Host 头校验；`api-contract.md` 完整化（含 touch/stats/refresh/roam/relation/mentions）。

---

## 九、引擎与 AI 的协作边界（简化）

> 已定：引擎是纯本地算法核心，零 AI 依赖（绝不调用任何模型）；AI 能力全部在**引擎之外**。本节是协作边界的**执行级简化**；引擎接口、端点与 MCP 工具面详见 [`api-contract.md`](api-contract.md) 与引擎 docs（MCP 暴露设计）。

- **三层模型**：宿主笔记软件（Layer 0）→ 插件薄壳（Layer 1：iframe + 隐式 touch + digest 导出，**不内置 AI**）→ 引擎纯算法核心（Layer 2：只读 REST / MCP）。外部 AI/agent 作为独立消费者经只读 MCP 读取引擎产出，不进入插件与引擎内部。
- **协作三原则**：① 引擎供给事实（roam / similar / communities / suggest-links），外部 AI 在其上推理；② AI 经「typed-edge + provenance」回流建议，由外部系统持久化，引擎只存数据不推断；③ 引擎 = 广而便宜的拓扑网（潜在关联候选），外部 AI = 语义过滤 / 升级器。
- **MCP 暴露（非插件内置）**：引擎以只读 MCP 工具面（graph.roam / graph.random / graph.relation / graph.stats 等）暴露给外部 AI/agent；插件不内嵌任何 AI 代码路径，AI 集成由外部 agent 自行完成。
- **红线对账**：引擎无 AI 依赖 ✓（白盒铁律：无任何 AI 代码路径、不调用任何模型）；AI 边非真实链接（kind=ai + 强制溯源 + 可撤销，由外部系统管理）✓；节点内容真实（语义检索通道 ✅，LLM 生成节点 ❌）✓；离线可用（AI 是可选外部富集）✓。
- **开发落点**：先引擎端点与 MCP 暴露（向前兼容）→ 插件薄壳（iframe + touch + digest 导出）。AI 集成不占用插件排期，由外部 agent/AI 经 MCP 自行完成。

## 十、参考文档链接

### Obsidian（官方开发者文档，2026-08-25 深读确认）
- Home：https://docs.obsidian.md/Home
- Build a plugin：https://docs.obsidian.md/Plugins/Getting+started/Build+a+plugin
- Manifest 规范：https://docs.obsidian.md/Reference/Manifest
- Submit your plugin：https://docs.obsidian.md/Plugins/Releasing/Submit+your+plugin
- Manage plugin lifecycle：https://docs.obsidian.md/Plugins/Guides/Manage+plugin+lifecycle
- Anatomy of a plugin：https://docs.obsidian.md/Plugins/Getting+started/Anatomy+of+a+plugin
- Developer policies：https://docs.obsidian.md/Developer+policies
- obsidian-releases（社区插件拉取机制）：https://github.com/obsidianmd/obsidian-releases/blob/master/README.md
- 官方插件模板：https://github.com/obsidianmd/obsidian-sample-plugin

