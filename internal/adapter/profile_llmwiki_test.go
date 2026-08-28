// LLM Wiki 画像 + 文件名级排除 + 结构探测测试（v0.1.12，backlog §3.5）。
package adapter

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

// llm-wiki 画像：excluded_dirs（raw/audit/output/outputs）+ excluded_files
// （index.md/log.md/CLAUDE.md/AGENTS.md）；其余字段继承 default-obsidian。
func TestProfileLLMWiki(t *testing.T) {
	p, ok := ProfileByName("llm-wiki")
	if !ok {
		t.Fatal("ProfileByName(llm-wiki) 应 ok")
	}
	if p.Name != "llm-wiki" {
		t.Fatalf("name: %s", p.Name)
	}
	if !containsStr(p.ExcludedFiles, "index.md") || !containsStr(p.ExcludedFiles, "AGENTS.md") {
		t.Fatalf("ExcludedFiles 应含 index.md/AGENTS.md: %v", p.ExcludedFiles)
	}
	// 继承 default-obsidian 的默认（TitleKeys/AliasKeys/TagKeys/TypeField）
	if len(p.TitleKeys) == 0 || p.AliasKeys == nil || p.TagKeys == nil {
		t.Fatalf("llm-wiki 应继承 default-obsidian 默认: %+v", p)
	}
	if p.TypeField != "type" {
		t.Fatalf("TypeField 应默认 type: %s", p.TypeField)
	}
}

// ParseVault 文件级排除：index.md/log.md/CLAUDE.md 不被解析进图（llm-wiki 画像）。
func TestParseVaultExcludeFiles(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "raw", "article.md"), "# raw 内容\n链接[[A]]\n")
	writeFile(t, filepath.Join(dir, "wiki", "entities", "A.md"), "# A 实体\n")
	writeFile(t, filepath.Join(dir, "wiki", "index.md"), "# 索引\n")
	writeFile(t, filepath.Join(dir, "wiki", "log.md"), "# 日志\n")
	writeFile(t, filepath.Join(dir, "CLAUDE.md"), "# schema\n")
	writeFile(t, filepath.Join(dir, "wiki", "entities", "B.md"), "# B 实体\n")

	p, _ := ProfileByName("llm-wiki")
	docs, err := ParseVault(dir, p)
	if err != nil {
		t.Fatalf("ParseVault: %v", err)
	}
	byID := map[string]bool{}
	for _, d := range docs {
		byID[d.ID] = true
	}
	if byID["A"] != true || byID["B"] != true {
		t.Fatalf("wiki 实体页 A/B 应进图: %v", byID)
	}
	if byID["index"] || byID["log"] || byID["CLAUDE"] {
		t.Fatalf("index.md/log.md/CLAUDE.md 应被排除: %v", byID)
	}
	// raw/ 整体不扫（含其中 .md）——不该出现 article
	if byID["article"] {
		t.Fatalf("raw/ 应整体跳过: %v", byID)
	}
}

// v0.2.1 bugfix：excluded_files 写裸名（log/index 不带 .md）也要能排除 log.md/index.md。
// 此前 ExcludedName 只做全名精确匹配，裸名匹配不上 "log.md"，排除不生效。
func TestExcludedNameBareStem(t *testing.T) {
	p := &VaultProfile{ExcludedFiles: []string{"log", "index", "status"}}
	for _, name := range []string{"log.md", "index.md", "status.md", "LOG.md"} {
		if !p.ExcludedName(name) {
			t.Fatalf("ExcludedName(%q) 应为 true（裸名排除）", name)
		}
	}
	if p.ExcludedName("chapter_001.md") {
		t.Fatal("普通文件不应被排除")
	}
	// 带 .md 的历史写法仍生效
	p2 := &VaultProfile{ExcludedFiles: []string{"log.md", "CLAUDE.md"}}
	if !p2.ExcludedName("log.md") || !p2.ExcludedName("CLAUDE.md") {
		t.Fatal("带 .md 的排除条目应继续生效")
	}
	// 前缀排除不受 .md 影响
	p3 := &VaultProfile{ExcludedPrefixes: []string{".ingest-report-", "health_"}}
	if !p3.ExcludedName(".ingest-report-2024-01-01.md") || !p3.ExcludedName("health_index.md") {
		t.Fatal("前缀排除应生效")
	}
}

// DetectLLMWiki：raw/ + wiki/index.md 组合命中；缺 raw 或 index 不命中。
func TestDetectLLMWiki(t *testing.T) {
	dir := t.TempDir()
	if DetectLLMWiki(dir) {
		t.Fatal("空 vault 不应命中")
	}
	writeFile(t, filepath.Join(dir, "raw", "x.md"), "x")
	writeFile(t, filepath.Join(dir, "wiki", "index.md"), "index")
	if !DetectLLMWiki(dir) {
		t.Fatal("raw/ + wiki/index.md 应命中")
	}
	// 缺 index.md
	dir2 := t.TempDir()
	writeFile(t, filepath.Join(dir2, "raw", "x.md"), "x")
	if DetectLLMWiki(dir2) {
		t.Fatal("缺 wiki/index.md 不应命中")
	}
}
