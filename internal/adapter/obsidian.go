package adapter

// ============================================================================
// 文件：internal/adapter/obsidian.go
// 模块：Obsidian（通用 Markdown）库适配器 —— 把 vault 里的 .md 解析成 Document 列表
//
// ▍职责
//   内核只认识 Document（设计 §6.8 Document API）；本文件 = Obsidian/通用 Markdown
//   的翻译器。图节点粒度 = 页面（Obsidian 链接目标天然是页面，图自洽性原则 §6.9）。
//
// ▍解析分层（VaultProfile 哲学，见 profile.go）
//   通用语法（四海皆准，由代码固定，本文件）：
//     - [[...]] 维基双链（含 frontmatter 里的 [[...]]）
//     - 标准 Markdown 链接 [文本](目标) —— Google Open Knowledge Format (OKF)
//       的通用格式（v0.1.1 起）：OKF 的知识图谱完全由 markdown 链接构成，因此
//       默认解析必须认识它；只认指向 .md 笔记的链接，附件/目录/外链一律忽略。
//     - 开头 --- frontmatter 块、首个 H1
//   语义映射（因人而异，走画像 YAML）：title/别名/标签键、类型推断、排除目录。
//
// ▍OKF（Open Knowledge Format, Google Cloud 2026 发布）在默认解析中的落地
//   OKF v0.1 = "一目录 markdown 文件 + YAML frontmatter" 的可移植知识格式：
//   每个概念一个文件，frontmatter 只约定六个可查询字段——type / title /
//   description / resource / tags / timestamp；概念间用普通 markdown 链接连成图；
//   index.md / log.md 为保留文件名（导航与变更历史，由画像决定是否结构类型化）。
//   默认画像（default-obsidian）因此内置：
//     type_field = "type"         → frontmatter 的 type 值即节点类型
//     description_keys = ["description"] → 描述并入正文（全文检索可命中元数据）
//     resource_keys = ["resource"]       → 外部资源地址并入正文
//     title_keys = ["title"]、tag_keys = ["tags"]（OKF 同名字段）
//   OKF 的 markdown 链接由通用语法层直接支持（见 parseLinks）。
//
// ▍修改记录
//   v0.1.0  初版：[[...]] + frontmatter + H1 + 画像语义映射。
//   v0.1.1  OKF 通用格式落地：markdown 链接入图、type 字段作节点类型、
//           description/resource 并入正文。
//   v0.1.2  同名文件 ID 消歧（重名文档改用相对路径 ID，防持久化 UNIQUE 冲突）。
// ============================================================================

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var (
	linkRe       = regexp.MustCompile(`\[\[([^\]]+)\]\]`)
	mdLinkRe     = regexp.MustCompile(`\[[^\]]*\]\(([^)\s]+)\)`)
	schemeRe     = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9+.\-]*:`)
	h1Re         = regexp.MustCompile(`(?m)^#\s+(.+?)\s*$`)
	tagRe        = regexp.MustCompile(`(?m)^\s*-\s*(.+?)\s*$`)
	kvRe         = regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_]*)\s*:\s*(.*)$`)
	inlineListRe = regexp.MustCompile(`^\s*\[(.*)\]\s*$`)
)

// ParseVault 递归扫描 vault 根下所有 .md，按 VaultProfile 解析为 Document 列表。
// 通用语法（[[...]] / markdown 链接 / frontmatter / H1）是固定的；语义映射走画像。
// 同名文件消歧（v0.1.2）：ID = 文件名，不同目录同名文件会撞 ID（Obsidian 自身
// 也不允许重名——冲突通常是归档/副本），从第二个起改用相对路径作 ID，两者都保留。
func ParseVault(root string, p *VaultProfile) ([]*Document, error) {
	var docs []*Document
	seen := map[string]string{} // id → 已占用的相对路径（冲突检测）
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() != "." && containsStr(p.ExcludedDirs, d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(d.Name()), ".md") {
			return nil
		}
		doc, err := ParseFile(path, root, p)
		if err != nil {
			return err
		}
		if prev, dup := seen[doc.ID]; dup {
			// 同名冲突：保留前者 basename ID（[[链接]] 走最短名），
			// 后者改用相对路径 ID（去 .md、/ 分隔），二者都进图
			doc.ID = strings.TrimSuffix(doc.Path, ".md")
			_ = prev
		}
		seen[doc.ID] = doc.Path
		docs = append(docs, doc)
		return nil
	})
	return docs, err
}

// ParseVaultIncremental 增量解析（v0.1.6，快照增量优化）：复用 mtime/size
// 未变的旧文档，只对变更/新增文件调 ParseFile。
//
// ▍语义与全量一致
//   返回的 Document 列表与 ParseVault 等价（含同名消歧、ID 分配）——复用只是
//   跳过"未变文件"的读取 + 正则解析（Obsidian 解析的主要开销）；消歧仍在
//   扫描循环里统一跑（复用的旧文档 ID 先重置为 basename，还原未消歧形态）。
//   删除检测天然成立：文件系统里消失的文件不会被扫描到，自然不在返回列表里
//   （diff 报 deleted）。
//
// ▍已知限制（v1.5 接受）
//   mtime/size 相同但内容被改回（秒级精度 + 同字节数）会漏检——概率极低；
//   文件系统 touch（改 mtime 不改内容）只会多触发一次重解析，无害。
//
// ▍返回值
//   (docs, reused, err)：reused = 复用的旧文档数（调用方可用于日志/统计）。
func ParseVaultIncremental(root string, p *VaultProfile, old []*Document) ([]*Document, int, error) {
	oldByPath := map[string]*Document{}
	for _, d := range old {
		oldByPath[d.Path] = d
	}
	var docs []*Document
	reused := 0
	seen := map[string]string{}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() != "." && containsStr(p.ExcludedDirs, d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(d.Name()), ".md") {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)

		var doc *Document
		if od, ok := oldByPath[rel]; ok {
			// mtime/size 快照命中 → 复用。注意：存储只保存秒级 mtime
			// （Save 用 d.MTime.Unix()），而内存解析的 MTime 带纳秒（os.Stat），
			// 文件系统 ModTime 也可能带纳秒（NTFS 100ns）——比较前双方都
			// 截断到秒，否则永远不匹配（v0.1.6 实测抓出）。
			if fi, err := d.Info(); err == nil &&
				fi.ModTime().Truncate(time.Second).Equal(od.MTime.Truncate(time.Second)) &&
				fi.Size() == od.Size {
				// 文件未变 → 复用旧解析结果；ID 重置为 basename（还原未消歧
				// 形态，统一跑消歧保证与全量一致）
				cp := *od
				cp.ID = strings.TrimSuffix(filepath.Base(rel), ".md")
				doc = &cp
				reused++
			}
		}
		if doc == nil {
			if doc, err = ParseFile(path, root, p); err != nil {
				return err
			}
		}
		if prev, dup := seen[doc.ID]; dup {
			doc.ID = strings.TrimSuffix(doc.Path, ".md")
			_ = prev
		}
		seen[doc.ID] = doc.Path
		docs = append(docs, doc)
		return nil
	})
	return docs, reused, err
}

// ParseFile 解析单篇 markdown：frontmatter（title/别名/标签/类型）+ 双链 + OKF 元数据。
func ParseFile(path, root string, p *VaultProfile) (*Document, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	text := string(b)

	rel, _ := filepath.Rel(root, path)
	rel = filepath.ToSlash(rel)
	id := strings.TrimSuffix(filepath.Base(rel), ".md")

	meta, lists := parseFrontmatter(text)
	body := stripFrontmatter(text)

	doc := &Document{
		ID:   id,
		Path: rel,
		// 链接含 frontmatter（bind_* / aliases 等字段里也有 [[...]]）与正文
		Refs: parseLinks(text),
		Tags: frontmatterStrings(meta, lists, p.TagKeys),
		Type: inferType(rel, meta, p),
		Text: body, // 全文（去 frontmatter）——降级兜底
	}
	// OKF 通用字段：description/resource 并入正文，全文检索可命中结构化元数据
	if extras := okfMetaText(meta, lists, p); extras != "" {
		doc.Text = extras + "\n\n" + body
	}
	if st, err := os.Stat(path); err == nil {
		doc.MTime = st.ModTime()
		doc.Size = st.Size()
	}
	doc.Title = extractTitle(meta, body, id, p)
	doc.Aliases = frontmatterStrings(meta, lists, p.AliasKeys)
	return doc, nil
}

// okfMetaText 把画像声明为"描述/资源"的 frontmatter 字段并成一段（OKF 通用格式）。
// description 直接并入；resource 加前缀以区分（外部资源地址，可被全文检索命中）。
func okfMetaText(meta map[string]string, lists map[string][]string, p *VaultProfile) string {
	var parts []string
	for _, d := range frontmatterStrings(meta, lists, p.DescriptionKeys) {
		if strings.TrimSpace(d) != "" {
			parts = append(parts, d)
		}
	}
	for _, r := range frontmatterStrings(meta, lists, p.ResourceKeys) {
		if strings.TrimSpace(r) != "" {
			parts = append(parts, "资源: "+r)
		}
	}
	return strings.Join(parts, "\n")
}

// ---- frontmatter ----

// parseFrontmatter 解析开头的 --- 块，返回 kv 与列表型字段。
// 已知限制：不支持 YAML 块标量（| > 多行值）——description 建议单行（OKF 规范同）。
func parseFrontmatter(text string) (map[string]string, map[string][]string) {
	meta := map[string]string{}
	lists := map[string][]string{}
	if !strings.HasPrefix(text, "---") {
		return meta, lists
	}
	rest := text[3:]
	idx := strings.Index(rest, "\n---")
	if idx < 0 {
		return meta, lists
	}
	block := rest[:idx]
	var lastKey string
	for _, line := range strings.Split(block, "\n") {
		line = strings.TrimRight(line, "\r")
		if m := kvRe.FindStringSubmatch(line); m != nil {
			lastKey = m[1]
			val := strings.TrimSpace(m[2])
			meta[lastKey] = strings.Trim(val, `"'`)
		} else if m := tagRe.FindStringSubmatch(line); m != nil && lastKey != "" {
			lists[lastKey] = append(lists[lastKey], strings.Trim(m[1], `"'`))
		}
	}
	return meta, lists
}

func stripFrontmatter(text string) string {
	if !strings.HasPrefix(text, "---") {
		return text
	}
	rest := text[3:]
	idx := strings.Index(rest, "\n---")
	if idx < 0 {
		return text
	}
	return rest[idx+4:]
}

// frontmatterStrings 按画像键取 frontmatter 字符串列表（列表型或内联 [a, b]）。
func frontmatterStrings(meta map[string]string, lists map[string][]string, keys []string) []string {
	for _, k := range keys {
		if v, ok := lists[k]; ok && len(v) > 0 {
			return v
		}
		if raw, ok := meta[k]; ok {
			if m := inlineListRe.FindStringSubmatch(raw); m != nil {
				var out []string
				for _, s := range strings.Split(m[1], ",") {
					if s = strings.TrimSpace(s); s != "" {
						out = append(out, s)
					}
				}
				return out
			}
			if raw != "" {
				return []string{raw}
			}
		}
	}
	return nil
}

// extractTitle：画像 TitleKeys 优先级 > 任意 *_name 键（vault schema 兜底）> 首个 H1 > 文件名。
func extractTitle(meta map[string]string, body, id string, p *VaultProfile) string {
	for _, k := range p.TitleKeys {
		if v, ok := meta[k]; ok && v != "" {
			return v
		}
	}
	for k, v := range meta {
		if strings.HasSuffix(k, "_name") && v != "" {
			return v
		}
	}
	if m := h1Re.FindStringSubmatch(body); m != nil {
		return strings.TrimSpace(m[1])
	}
	return id
}

// inferType 按画像规则推断节点类型：
// 键规则 > OKF type 字段 > 文件名前缀 > 文件名包含 > 目录前缀 > 默认。
// OKF type 字段（type_field，默认 "type"）的值即节点类型；放在键规则之后——
// 库专属的键规则更具体，理应优先；type 字段是通用约定，优于文件名启发式。
func inferType(rel string, meta map[string]string, p *VaultProfile) string {
	base := filepath.Base(rel)
	for _, r := range p.TypeByKey {
		for k := range meta {
			if strings.HasPrefix(k, r.Pattern) {
				return r.Type
			}
		}
	}
	if p.TypeField != "" {
		if v, ok := meta[p.TypeField]; ok {
			if v = strings.TrimSpace(v); v != "" {
				return v
			}
		}
	}
	for _, r := range p.TypeByPrefix {
		if strings.HasPrefix(base, r.Pattern) {
			return r.Type
		}
	}
	for _, r := range p.TypeByExt {
		if strings.Contains(base, r.Pattern) {
			return r.Type
		}
	}
	for _, r := range p.TypeByDir {
		if strings.HasPrefix(rel, r.Pattern) {
			return r.Type
		}
	}
	return p.DefaultType
}

// ---- 链接 ----

// parseLinks 提取全部内链：[[...]] 维基双链 + 标准 Markdown 链接（OKF 通用格式）。
// 统一归一：去别名（|）、去 #锚点、去 .md 后缀、取文件名（相对/绝对路径均按 basename）。
// Markdown 链接只认指向 .md 的（OKF 约定：概念间以 .md 链接相连）；带协议的外链
// （http/mailto/obsidian:// 等）、附件、目录、纯锚点一律不进图。
func parseLinks(text string) []string {
	seen := map[string]bool{}
	var out []string
	add := func(id string) {
		if id == "" || seen[id] {
			return
		}
		seen[id] = true
		out = append(out, id)
	}
	for _, m := range linkRe.FindAllStringSubmatch(text, -1) {
		raw := m[1]
		if i := strings.IndexByte(raw, '|'); i >= 0 {
			raw = raw[:i]
		}
		if i := strings.IndexByte(raw, '#'); i >= 0 {
			raw = raw[:i]
		}
		raw = strings.TrimSpace(raw)
		raw = strings.TrimSuffix(raw, ".md")
		if raw != "" {
			add(raw)
		}
	}
	for _, m := range mdLinkRe.FindAllStringSubmatch(text, -1) {
		raw := strings.TrimSpace(m[1])
		if i := strings.IndexAny(raw, "#?"); i >= 0 {
			raw = raw[:i]
		}
		raw = strings.TrimRight(raw, "/")
		if raw == "" || schemeRe.MatchString(raw) {
			continue // 外链
		}
		if !strings.HasSuffix(strings.ToLower(raw), ".md") {
			continue // 只认 .md 笔记
		}
		raw = filepath.Base(strings.TrimSuffix(raw, ".md"))
		add(raw)
	}
	return out
}

func containsStr(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}
