// Package adapter 实现设计 §6.8 的 Document API（内核唯一的格式抽象 = VFS）。
// 内核只认识 Document；每个笔记软件一个 adapter 负责"格式翻译"。
// 本文件：统一图格式的 Document 定义（节点粒度 = 原生链接粒度）。
package adapter

import "time"

// Document 是图节点，粒度 = 原生链接目标（Obsidian 页面 / 虎鲸块）。
// 对应设计 §6.8 Document API 定稿。
type Document struct {
	ID      string    // 身份：Obsidian 文件名（不含 .md）；图结构与对账用
	Title   string    // 语义名：按 VaultProfile.TitleKeys 提取；锚定与展示用
	Aliases []string  // frontmatter aliases（锚定层多通道）
	Type    string    // 节点类型：VaultProfile 类型规则推断（人物/设定/线索/章节…）
	Path    string    // 相对 vault 根的路径；对账用
	MTime   time.Time // 对账用
	Size    int64     // 对账用
	Refs    []string  // 双链 → 其他 Document ID（解析后，不含别名 / #锚点）
	Tags    []string  // frontmatter tags（v1 标签锚定通道）
	Text    string    // 正文全文（去 frontmatter）——全文 LIKE 降级兜底用（决策 #10）
}
