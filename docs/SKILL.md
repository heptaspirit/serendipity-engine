---
name: SKILL
description: 使用 serendipity MCP 工具（graph.stats/roam/random/relation/node/similar/community/suggest + seren.touch_digest + seren.touch_stats）并正确解读结果。当可用 serendipity MCP 工具、或用户提到 graph/roam/漫游/图谱/related/similar/community/suggest/touch/digest/serendipity/奇遇 时加载。核心立场：这些工具输出的是"候选/建议"（图结构估算），不是库内既定事实——教 AI 何时调用、如何解读、哪些反模式会把它变成噪声。
version: 0.1.0
---

# Serendipity MCP 使用指南

> 本文是引擎分发的 **skill 资产**。安装：将本文件拷到对应文件夹即可`；与 MCP 工具描述（§3.8 Layer A）和 `seren_orientation` prompt 保持同源同步。

## 定位
偶遇引擎，不是检索器：回答"你被什么结构性地连接"，不是"你说了什么"。
发散（灵感/探索）用它；收敛（精确答案）用搜索。

## 能力边界
引擎另有 CLI / REST / Web UI（含写操作：建图、刷新、起服务），归人/插件用；你只用 MCP **只读**面。MCP 未配置时，引导用户从 Obsidian 插件设置页**一键复制** MCP 配置，不要自行 spawn CLI。

## 工具速查
| 工具 | 何时 | 呈现 |
|---|---|---|
| graph.stats | 首次接触库 | 陈述规模 |
| graph.roam | 开放探索/找灵感 | "你可能对 X 感兴趣" |
| graph.random | 无目标随机 | 明说随机 |
| graph.relation | 问 A-B 有无关系 | "结构上似乎相关，非库内既有边" |
| graph.node | 确认目标笔记 | 确认/否定 |
| graph.similar | 找结构孪生 | "这两篇像一对，值得连一下" |
| graph.community | 主题审计/找空洞 | "这些成簇 / 这片稀疏"（默认只看最大 top10；传 node 查单簇归属） |
| graph.suggest | AI 研判互链补全 | "这对看似相关（共享 X/Y），你判、接受者写回 kind=ai"（带端点标题） |
| seren.touch_digest | 用户问注意力在哪（被动） | "主题在升温"，高计数≠重要（窗口摘要，易为空） |
| seren.touch_stats | 看累计点击热度 | "哪些被点得多"（累计，与窗口 digest 互补） |

全部**只读**：不写 touch、不触发刷新、不改图。

## 价值（为什么值得信）
- **第三种信号**：关键词/语义之外的图结构关联（间接引用、结构孪生、簇与空洞）。
- **候选=可验证假设**：价值在"人看一眼→验证→成知识"的闭环，不在结论本身。
- **结构化偶遇**：没有问题时也给你"值得看"的——解决"你不知道你不知道什么"。
- **场景**：写作启发 / 知识审计 / 回顾元认知 / 建议性关联（确认后才落图）。
- **边界**：不当检索、不当事实、不当权威；"模糊的正确"优于"精确的错误"。

## 反模式（红线）
1. 把 roam/similar/relation 输出说成库内事实
2. 用 touch 计数推导重要性/权威性
3. 主动刷 digest、频繁打扰（digest 被动，等用户问）
4. 用 touch 演化边权/排序
5. 把"建议边"写成"已连接"

## 例子
✅ "笔记 A 和 B 结构上靠得近，要不要连一下？C 那簇有点孤立，可能值得补充。"
❌ "根据图谱，A 与 B 存在关联；touch 显示这是你最重要的主题。"
