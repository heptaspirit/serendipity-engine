package adapter

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	linkRe  = regexp.MustCompile(`\[\[([^\]]+)\]\]`)
	h1Re    = regexp.MustCompile(`(?m)^#\s+(.+?)\s*$`)
	tagRe   = regexp.MustCompile(`(?m)^\s*-\s*(.+?)\s*$`)
	kvRe    = regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_]*)\s*:\s*(.*)$`)
	inlineListRe = regexp.MustCompile(`^\s*\[(.*)\]\s*$`)
)

// ParseVault 递归扫描 vault 根下所有 .md，按 VaultProfile 解析为 Document 列表。
// 通用语法（[[...]] / frontmatter / H1）是固定的；语义映射（title/别名/标签/类型）走画像。
func ParseVault(root string, p *VaultProfile) ([]*Document, error) {
	var docs []*Document
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
		docs = append(docs, doc)
		return nil
	})
	return docs, err
}

// ParseFile 解析单篇 markdown：frontmatter（title/aliases/tags/类型）+ [[链接]]。
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
	if st, err := os.Stat(path); err == nil {
		doc.MTime = st.ModTime()
		doc.Size = st.Size()
	}
	doc.Title = extractTitle(meta, body, id, p)
	doc.Aliases = frontmatterStrings(meta, lists, p.AliasKeys)
	return doc, nil
}

// ---- frontmatter ----

// parseFrontmatter 解析开头的 --- 块，返回 kv 与列表型字段。
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

// inferType 按画像规则推断节点类型：键规则 > 文件名前缀 > 文件名包含 > 目录前缀 > 默认。
func inferType(rel string, meta map[string]string, p *VaultProfile) string {
	base := filepath.Base(rel)
	for _, r := range p.TypeByKey {
		for k := range meta {
			if strings.HasPrefix(k, r.Pattern) {
				return r.Type
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

// parseLinks 提取 [[...]]：去别名（|）、去 #锚点、去 .md 后缀、去空白。
func parseLinks(text string) []string {
	var out []string
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
		if raw == "" {
			continue
		}
		out = append(out, raw)
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
