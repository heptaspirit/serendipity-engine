package adapter

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// DetectProfile 扫描一个 Obsidian vault，自动提出画像 YAML 骨架。
// 这是"适配新库"的 onboarding 路径：机器产出骨架，用户按库微调类型名后保存。
// 只做统计推断，不猜语义——类型名默认用目录/前缀原名，交给用户改。
func DetectProfile(vault string) (*VaultProfile, error) {
	p := DefaultObsidianProfile()
	p.Name = filepath.Base(strings.TrimRight(vault, `/\`))

	// 1. 目录与文件普查
	type dirStat struct {
		name  string
		count int
	}
	dirCount := map[string]int{}
	prefixCount := map[string]int{}
	dotPrefix := map[string]int{}
	excalidraw := 0
	keyCount := map[string]int{}
	fileCount := 0

	excluded := map[string]bool{".obsidian": true, ".trash": true, ".git": true}
	// 先收集全部工具点目录
	filepath.WalkDir(vault, func(path string, d os.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return nil
		}
		name := d.Name()
		if name != "." && strings.HasPrefix(name, ".") && !excluded[name] {
			excluded[name] = true // .agents/.dsh 等
		}
		return nil
	})
	for e := range excluded {
		p.ExcludedDirs = append(p.ExcludedDirs, e)
	}
	// 去重
	seenDir := map[string]bool{}
	dedup := p.ExcludedDirs[:0]
	for _, e := range p.ExcludedDirs {
		if !seenDir[e] {
			seenDir[e] = true
			dedup = append(dedup, e)
		}
	}
	p.ExcludedDirs = dedup
	sort.Strings(p.ExcludedDirs)

	filepath.WalkDir(vault, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if d.Name() != "." && excluded[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(d.Name()), ".md") {
			return nil
		}
		fileCount++
		rel, _ := filepath.Rel(vault, path)
		rel = filepath.ToSlash(rel)
		// 目录统计（一级）
		if i := strings.IndexByte(rel, '/'); i > 0 {
			dirCount[rel[:i]]++
		}
		// 文件名前缀统计（跳过点文件——点文件单独按结构处理）
		base := strings.TrimSuffix(filepath.Base(rel), ".md")
		if strings.Contains(base, ".excalidraw") {
			excalidraw++
		}
		if strings.HasPrefix(base, ".") {
			// 点前缀文件（如 .ingest-* 报告类）→ 前缀 ".ingest"
			if i := strings.IndexAny(base, "-_"); i > 0 {
				dotPrefix[base[:i]]++
			}
		} else if i := strings.IndexByte(base, '_'); i > 0 {
			prefixCount[base[:i]]++
		}
		// frontmatter 键统计
		if b, err := os.ReadFile(path); err == nil {
			if meta, _ := parseFrontmatter(string(b)); meta != nil {
				for k := range meta {
					keyCount[k]++
				}
			}
		}
		return nil
	})

	// 2. 目录 → 类型规则（去掉明显工具目录）
	var dirs []string
	for name := range dirCount {
		dirs = append(dirs, name)
	}
	sort.Strings(dirs)
	for _, name := range dirs {
		if excluded[name] || dirCount[name] < 2 {
			continue
		}
		p.TypeByDir = append(p.TypeByDir, TypeRule{Pattern: name, Type: name})
	}

	// 3. 文件名前缀 → 类型规则（count ≥ 3 且非工具模式）
	var prefixes []string
	for pr := range prefixCount {
		prefixes = append(prefixes, pr)
	}
	sort.Strings(prefixes)
	for _, pr := range prefixes {
		if prefixCount[pr] < 3 {
			continue
		}
		p.TypeByPrefix = append(p.TypeByPrefix, TypeRule{Pattern: pr + "_", Type: pr})
	}
	// 点前缀文件（.ingest-report-* 等）→ 报告/结构类
	var dotPrefixes []string
	for pr := range dotPrefix {
		dotPrefixes = append(dotPrefixes, pr)
	}
	sort.Strings(dotPrefixes)
	for _, pr := range dotPrefixes {
		p.TypeByPrefix = append(p.TypeByPrefix, TypeRule{Pattern: pr, Type: "报告"})
	}
	if excalidraw > 0 {
		p.TypeByExt = append(p.TypeByExt, TypeRule{Pattern: ".excalidraw", Type: "画布"})
	}

	// 4. title/别名/标签键建议
	p.TitleKeys = nil
	for _, k := range []string{"title", "name"} {
		if keyCount[k] > 0 {
			p.TitleKeys = append(p.TitleKeys, k)
		}
	}
	if len(p.TitleKeys) == 0 {
		p.TitleKeys = []string{"title"}
	}
	for k := range keyCount {
		if keyCount[k] >= 3 && (strings.HasSuffix(k, "_name") || strings.HasSuffix(k, "_title")) && k != "name" && k != "title" {
			p.TitleKeys = append(p.TitleKeys, k)
		}
	}
	if keyCount["aliases"] > 0 {
		p.AliasKeys = []string{"aliases"}
	}
	if keyCount["tags"] > 0 {
		p.TagKeys = []string{"tags"}
	}

	// 5. 结构类型建议：目录/前缀名含结构词，或点前缀/画布
	structuralWords := []string{"章节", "大纲", "报告", "仪表盘", "dashboard", "chapter", "outline", "report", "log", "state", "index", "ingest"}
	seenStruct := map[string]bool{}
	for _, r := range p.TypeByDir {
		if containsAny(r.Type, structuralWords) {
			seenStruct[r.Type] = true
		}
	}
	for _, r := range p.TypeByPrefix {
		if containsAny(r.Type, structuralWords) {
			seenStruct[r.Type] = true
		}
	}
	if excalidraw > 0 {
		seenStruct["画布"] = true
	}
	if keyCount["chapter_id"] > 0 {
		seenStruct["章节"] = true
	}
	for t := range seenStruct {
		p.StructuralTypes = append(p.StructuralTypes, t)
	}
	sort.Strings(p.StructuralTypes)

	return p, nil
}

func containsAny(s string, words []string) bool {
	ls := strings.ToLower(s)
	for _, w := range words {
		if strings.Contains(ls, w) {
			return true
		}
	}
	return false
}
