package adapter

// 快照增量解析单元测试（v0.1.6）：增量结果必须与全量解析完全一致
// （含同名消歧、ID 分配）；mtime/size 未变才复用。
import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"
)

// parseLinks 鲁棒性（v0.1.13，反馈 #1）：剔代码块/行内代码、处理表格反斜杠、过滤占位符。
// 返回按序去重后的链接目标列表（parseLinks 内部已按出现顺序去重，这里有序化便于比较）。
func TestParseLinksRobustness(t *testing.T) {
	inline := "`[[wikilink]]`"
	fence := "```"
	text := "# 测试\n" +
		"正文真链接 [[周真]] 和 [[人物_012]]。\n" +
		"行内代码 " + inline + " 不是链接。\n" +
		fence + "\n代码块里的 [[人物_xxx]] 和 [[章节_XXX]] 也不是。\n" + fence + "\n" +
		"列表 [[人物_xxx|别名]]、[[章节_XXX]]、[[...]]、[[wikilink]] 都是模板。\n" +
		"表格带反斜杠：| [[人物_003\\]] | 结尾反斜杠。\n" +
		"md 链接 [正文](文档乙.md) 应保留；[外链](https://x.com) 忽略。"

	got := parseLinks(text)
	// 期望：真链接 + md 链接；无代码块/行内代码/占位符/反斜杠噪音。
	want := []string{"周真", "人物_012", "人物_003", "文档乙"}
	sort.Strings(got)
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseLinks 鲁棒性失败:\n got=%v\nwant=%v", got, want)
	}
}

// 表格反斜杠单独核验（人物_003\ → 人物_003，去尾部 \）。
func TestParseLinksTableBackslash(t *testing.T) {
	got := parseLinks("| 列 | 值 |\n|---|---|\n| 人物 | [[人物_003\\]] |\n")
	if len(got) != 1 || got[0] != "人物_003" {
		t.Fatalf("表格反斜杠未归一: got=%v", got)
	}
}

// 代码块/行内代码里的链接应被剔除（`[[x]]` 与 ```...``` 内）。
func TestParseLinksStripsCode(t *testing.T) {
	text := "真 [[a]]。\n`inline [[b]]` 不算。\n```\nblock [[c]]\n```"
	got := parseLinks(text)
	if len(got) != 1 || got[0] != "a" {
		t.Fatalf("代码区链接未剔除: got=%v", got)
	}
}

// 前缀级排除（v0.1.13，反馈 #3）：.ingest-report- / health_ 等自动生成文件不进图。
func TestParseVaultPrefixExclude(t *testing.T) {
	root := t.TempDir()
	writeTestNote(t, filepath.Join(root, "人物_012.md"), "# 周真\n\n内容。")
	writeTestNote(t, filepath.Join(root, ".ingest-report-CH020.md"), "# 报告\n\n[[人物_xxx]]。")
	writeTestNote(t, filepath.Join(root, "health_2026-08-01.md"), "# 健康\n\n[[人物_012]]。")
	p := &VaultProfile{Name: "test", ExcludedPrefixes: []string{".ingest-report-", "health_"}}
	docs, err := ParseVault(root, p)
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 1 || docs[0].ID != "人物_012" {
		t.Fatalf("前缀排除应只剩 人物_012, got %v", docs)
	}
}

// 批量表格反斜杠（反馈 #2）：设定_人物称呼 的 [[人物_001\]]..[[人物_026\]] + log 的
// [[实体\]]——反斜杠应全部剔除，不再产生带 \ 的悬空链接。
func TestParseLinksBackslashBatch(t *testing.T) {
	var b strings.Builder
	b.WriteString("# 设定_人物称呼\n\n| 角色 | 称呼 |\n|---|---|\n")
	for i := 1; i <= 26; i++ {
		b.WriteString(fmt.Sprintf("| [[人物_%03d\\]] | 称呼%d |\n", i, i))
	}
	b.WriteString("\nlog: [[实体\\]]\n")
	got := parseLinks(b.String())
	if len(got) != 27 {
		t.Fatalf("应 27 个链接(人物_001..026 + 实体), got %d: %v", len(got), got)
	}
	for _, g := range got {
		if strings.HasSuffix(g, "\\") {
			t.Fatalf("链接仍带反斜杠: %q", g)
		}
	}
}

func writeTestNote(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// 建一个含同名冲突的 vault：a/甲.md + b/甲.md（消歧后后者 ID = b/甲）。
func buildIncrVault(t *testing.T) string {
	root := t.TempDir()
	writeTestNote(t, filepath.Join(root, "a", "甲.md"), "# 甲A\n\n内容[[乙]]。")
	writeTestNote(t, filepath.Join(root, "b", "甲.md"), "# 甲B\n\n同名文件。")
	writeTestNote(t, filepath.Join(root, "乙.md"), "# 乙\n\n内容[[甲]]。")
	return root
}

func docsKey(docs []*Document) map[string]string {
	out := map[string]string{}
	for _, d := range docs {
		out[d.ID] = d.Path
	}
	return out
}

// 增量 ≡ 全量：同一 vault，全量解析与"以全量结果为旧状态"的增量解析
// 产生完全相同的 ID→Path 映射（消歧一致）。
func TestIncrementalEqualsFull(t *testing.T) {
	root := buildIncrVault(t)
	p := &VaultProfile{Name: "test", ExcludedDirs: nil}

	full, err := ParseVault(root, p)
	if err != nil {
		t.Fatal(err)
	}
	inc, reused, err := ParseVaultIncremental(root, p, full)
	if err != nil {
		t.Fatal(err)
	}
	if reused != len(full) {
		t.Fatalf("无变化应全复用：reused=%d total=%d", reused, len(full))
	}
	// 至少出现一个同名消歧（a/甲 保留 basename，b/甲 用相对路径）
	hasDisambig := false
	for _, d := range inc {
		if d.ID == "b/甲" {
			hasDisambig = true
		}
	}
	if !hasDisambig {
		t.Fatalf("同名消歧未生效：%v", docsKey(inc))
	}
	if !reflect.DeepEqual(docsKey(full), docsKey(inc)) {
		t.Fatalf("增量 ≠ 全量：\n全量=%v\n增量=%v", docsKey(full), docsKey(inc))
	}
}

// 修改一个文件后增量：只重解析变更文件，且结果与全量一致。
func TestIncrementalAfterChange(t *testing.T) {
	root := buildIncrVault(t)
	p := &VaultProfile{Name: "test"}

	full, err := ParseVault(root, p)
	if err != nil {
		t.Fatal(err)
	}
	// 改乙.md 并确保 mtime 变化（不同秒）
	time.Sleep(1100 * time.Millisecond)
	writeTestNote(t, filepath.Join(root, "乙.md"), "# 乙\n\n内容改了[[甲]]。")

	inc, reused, err := ParseVaultIncremental(root, p, full)
	if err != nil {
		t.Fatal(err)
	}
	if reused != len(full)-1 {
		t.Fatalf("只应重解析乙：reused=%d（期望 %d）", reused, len(full)-1)
	}
	full2, err := ParseVault(root, p)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(docsKey(full2), docsKey(inc)) {
		t.Fatalf("增量 ≠ 全量（变更后）：\n全量=%v\n增量=%v", docsKey(full2), docsKey(inc))
	}
	// 变更文件的 Text 应为新内容（重解析了）
	for _, d := range inc {
		if d.ID == "乙" && !strings.Contains(d.Text, "内容改了") {
			t.Fatalf("乙未重解析：text=%q", d.Text)
		}
	}
}

// 删除检测：文件消失 → 增量结果不含它（diff 层报 deleted）。
func TestIncrementalAfterDelete(t *testing.T) {
	root := buildIncrVault(t)
	p := &VaultProfile{Name: "test"}

	full, err := ParseVault(root, p)
	if err != nil {
		t.Fatal(err)
	}
	os.Remove(filepath.Join(root, "乙.md"))
	inc, _, err := ParseVaultIncremental(root, p, full)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range inc {
		if d.ID == "乙" {
			t.Fatal("删除的文件不应出现在增量结果里")
		}
	}
}
