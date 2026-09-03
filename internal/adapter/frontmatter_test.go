package adapter

// frontmatter → yaml.v3 专项测试（v0.2.2）：锁定新解析器与旧手写版对齐的口径，
// 以及旧版缺失（块标量/引号转义/嵌套缩进列表）的修复面——此前没有 fixture 网，
// 这是 yaml 化改造的安全网。

import (
	"reflect"
	"strings"
	"testing"
)

// 旧口径回归：字符串值原文、内联/缩进列表、空值键、数字/布尔保留源码文本。
func TestParseFrontmatterLegacyCompat(t *testing.T) {
	text := "---\n" +
		"title: 周真\n" +
		"type: 人物\n" +
		"aliases: [阿真, 真哥]\n" +
		"tags:\n" +
		"  - 主角\n" +
		"  - 将门\n" +
		"empty:\n" +
		"version: 1.50\n" +
		"flag: true\n" +
		"---\n# 正文\n[[人物_012]]\n"
	meta, lists := parseFrontmatter(text)
	if meta["title"] != "周真" || meta["type"] != "人物" {
		t.Fatalf("标量值错误: %v", meta)
	}
	if meta["version"] != "1.50" {
		t.Fatalf("数字应保留源码文本(不转型): %q", meta["version"])
	}
	if meta["flag"] != "true" {
		t.Fatalf("布尔应保留源码文本(不转型): %q", meta["flag"])
	}
	if v, ok := meta["empty"]; !ok || v != "" {
		t.Fatalf("空值键应存空串: %v", meta)
	}
	// 列表键：meta 空串标记（键统计口径）+ lists 取值
	if v, ok := meta["aliases"]; !ok || v != "" {
		t.Fatalf("列表键应有空串标记: %v", meta)
	}
	if !reflect.DeepEqual(lists["aliases"], []string{"阿真", "真哥"}) {
		t.Fatalf("内联列表错误: %v", lists["aliases"])
	}
	if !reflect.DeepEqual(lists["tags"], []string{"主角", "将门"}) {
		t.Fatalf("缩进 - 列表错误: %v", lists["tags"])
	}
}

// 旧版缺失面：块标量（| >）、引号内 #/转义、嵌套映射忽略。
func TestParseFrontmatterYAMLFeatures(t *testing.T) {
	text := "---\n" +
		"title: \"He said \\\"hi\\\" # not comment\"\n" +
		"description: >-\n" +
		"  第一行\n" +
		"  第二行\n" +
		"meta:\n" +
		"  source: x\n" + // 嵌套映射 → 仅键标记
		"---\n# 正文\n"
	meta, _ := parseFrontmatter(text)
	if meta["title"] != `He said "hi" # not comment` {
		t.Fatalf("引号/转义解码错误: %q", meta["title"])
	}
	if !strings.Contains(meta["description"], "第一行") || !strings.Contains(meta["description"], "第二行") {
		t.Fatalf("块标量应合并为多行文本: %q", meta["description"])
	}
	if v, ok := meta["meta"]; !ok || v != "" {
		t.Fatalf("嵌套映射应仅留键标记: %v", meta)
	}
}

// 键口径：非字符串键 / 非旧版字符集键不进 meta。
func TestParseFrontmatterKeyFilter(t *testing.T) {
	text := "---\n" +
		"123: x\n" + // 数字键
		"kebab-key: y\n" + // 短横键（旧版字符集外）
		"valid: ok\n" +
		"---\n"
	meta, _ := parseFrontmatter(text)
	if len(meta) != 1 || meta["valid"] != "ok" {
		t.Fatalf("键过滤应只剩 valid: %v", meta)
	}
}

// 重复键后者覆盖（yaml Node 层天然允许重复键，迭代即后者生效）。
func TestParseFrontmatterDupKeyLastWins(t *testing.T) {
	text := "---\ntitle: 旧\ntitle: 新\n---\n"
	meta, _ := parseFrontmatter(text)
	if meta["title"] != "新" {
		t.Fatalf("重复键应后者覆盖: %q", meta["title"])
	}
}

// 整块 YAML 非法 → 空 meta/lists（解析不崩，当无 frontmatter 处理）。
func TestParseFrontmatterMalformedTolerant(t *testing.T) {
	text := "---\ntitle: \"未闭合\n---\n"
	meta, lists := parseFrontmatter(text)
	if len(meta) != 0 || len(lists) != 0 {
		t.Fatalf("非法 YAML 应整体为空（容错优先）: meta=%v lists=%v", meta, lists)
	}
}

// 无 frontmatter / 未闭合 → 空（与 stripFrontmatter 同边界）。
func TestParseFrontmatterAbsent(t *testing.T) {
	meta, lists := parseFrontmatter("# 只有正文\n[[a]]\n")
	if len(meta) != 0 || len(lists) != 0 {
		t.Fatalf("无 frontmatter 应为空: %v %v", meta, lists)
	}
	meta, _ = parseFrontmatter("---\ntitle: x\n") // 没有闭合 ---
	if len(meta) != 0 {
		t.Fatalf("未闭合 frontmatter 应为空: %v", meta)
	}
}
