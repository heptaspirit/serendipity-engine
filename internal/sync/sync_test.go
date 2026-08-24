// Package sync 对账同步的单元测试：改名迁移（v0.1.5，修订 #8）。
// 覆盖：内容哈希配对、同目录优先、并列拿不准跳过、Refs 重定向去重、
// Diff 统计拆分（deleted+added → renamed）、虎鲸数字 ID 不误判。
package sync

import (
	"reflect"
	"testing"

	"serendipity-engine/internal/adapter"
)

func doc(id, path, text string, refs ...string) *adapter.Document {
	return &adapter.Document{ID: id, Path: path, Text: text, Refs: refs}
}

// 基础改名：同目录、内容不变 → 识别为 rename。
func TestDetectRenamesBasic(t *testing.T) {
	old := []*adapter.Document{
		doc("旧名", "笔记/旧名.md", "正文内容"),
		doc("甲", "笔记/甲.md", "甲的内容"),
	}
	cur := []*adapter.Document{
		doc("新名", "笔记/新名.md", "正文内容"),
		doc("甲", "笔记/甲.md", "甲的内容"),
	}
	rm := DetectRenames(old, cur)
	if len(rm) != 1 || rm["旧名"] != "新名" {
		t.Fatalf("期望 {旧名→新名}，得到 %v", rm)
	}
}

// 改名 + 内容微调 → 内容哈希不同，不判改名（保守退回 deleted+added）。
func TestDetectRenamesContentChanged(t *testing.T) {
	old := []*adapter.Document{doc("旧名", "笔记/旧名.md", "原内容")}
	cur := []*adapter.Document{doc("新名", "笔记/新名.md", "内容改了")}
	rm := DetectRenames(old, cur)
	if len(rm) != 0 {
		t.Fatalf("内容已变不应判改名，得到 %v", rm)
	}
}

// 同目录两个文件同时改名且内容相同 → 并列拿不准，跳过（不误配）。
func TestDetectRenamesTieSkip(t *testing.T) {
	old := []*adapter.Document{
		doc("a", "x/a.md", "相同正文"),
		doc("b", "x/b.md", "相同正文"),
	}
	cur := []*adapter.Document{
		doc("c", "x/c.md", "相同正文"),
		doc("d", "x/d.md", "相同正文"),
	}
	rm := DetectRenames(old, cur)
	if len(rm) != 0 {
		t.Fatalf("并列候选不应猜配，得到 %v", rm)
	}
}

// 目录不同（跨目录移动）→ 不判改名（保守）。
func TestDetectRenamesMovedDir(t *testing.T) {
	old := []*adapter.Document{doc("旧名", "a/旧名.md", "正文")}
	cur := []*adapter.Document{doc("新名", "b/新名.md", "正文")}
	rm := DetectRenames(old, cur)
	if len(rm) != 0 {
		t.Fatalf("跨目录不应判改名，得到 %v", rm)
	}
}

// 虎鲸：数字 ID 稳定，删/增不配对。
func TestDetectRenamesOrcaStable(t *testing.T) {
	old := []*adapter.Document{doc("1001", "block/1001", "正文")}
	cur := []*adapter.Document{doc("2002", "block/2002", "正文")}
	rm := DetectRenames(old, cur)
	if len(rm) != 0 {
		t.Fatalf("虎鲸数字 ID 不应误判改名，得到 %v", rm)
	}
}

// ApplyRenames：他人 Refs 重定向 + 去重。
func TestApplyRenames(t *testing.T) {
	docs := []*adapter.Document{
		doc("新名", "笔记/新名.md", "正文"),
		doc("引用者", "笔记/引用者.md", "x", "旧名", "旧名", "其他"),
	}
	ApplyRenames(docs, map[string]string{"旧名": "新名"})
	want := []string{"新名", "其他"}
	if !reflect.DeepEqual(docs[1].Refs, want) {
		t.Fatalf("Refs 重定向+去重失败：%v ≠ %v", docs[1].Refs, want)
	}
}

// Diff 集成：改名从 deleted+added 拆出为 renamed。
func TestDiffRenamed(t *testing.T) {
	old := []*adapter.Document{
		doc("旧名", "笔记/旧名.md", "正文"),
		doc("删除的", "笔记/删除的.md", "删"),
	}
	cur := []*adapter.Document{
		doc("新名", "笔记/新名.md", "正文"),
		doc("新增的", "笔记/新增的.md", "增"),
	}
	res := Diff(old, cur)
	if res.Renamed != 1 || res.Deleted != 1 || res.Added != 1 {
		t.Fatalf("统计错误：renamed=%d deleted=%d added=%d", res.Renamed, res.Deleted, res.Added)
	}
	if len(res.Renames) != 1 || res.Renames[0].OldID != "旧名" || res.Renames[0].NewID != "新名" {
		t.Fatalf("Renames 明细错误：%v", res.Renames)
	}
}

// MergeRenames：持久化条目在旧名重现时失效；本次新检测合并进来。
func TestMergeRenames(t *testing.T) {
	stored := map[string]string{"旧A": "新A", "旧B": "新B"}
	cur := []*adapter.Document{
		doc("新A", "笔记/新A.md", "x"),
		doc("旧B", "笔记/旧B.md", "y"), // 旧名重现（被重新创建）
		doc("新C", "笔记/新C.md", "z"),
	}
	fresh := map[string]string{"旧C": "新C"}
	out := MergeRenames(stored, fresh, cur)
	if _, ok := out["旧A"]; !ok {
		t.Fatalf("旧A→新A 应保留：%v", out)
	}
	if _, ok := out["旧B"]; ok {
		t.Fatalf("旧B 重现应失效：%v", out)
	}
	if out["旧C"] != "新C" {
		t.Fatalf("新检测应合并：%v", out)
	}
}

// ApplyRenames 链式解析：旧→中→新 一路解到最终目标。
func TestApplyRenamesChain(t *testing.T) {
	docs := []*adapter.Document{
		doc("最终", "笔记/最终.md", "x"),
		doc("引用者", "笔记/引用者.md", "x", "旧"),
	}
	ApplyRenames(docs, map[string]string{"旧": "中", "中": "最终"})
	want := []string{"最终"}
	if !reflect.DeepEqual(docs[1].Refs, want) {
		t.Fatalf("链式重定向失败：%v ≠ %v", docs[1].Refs, want)
	}
}

// MergeRenames 链式折叠（v0.1.11，backlog §四）：A→B、B→C 只留 A→C（链头→最终），
// 丢弃中间环，条目数从"历史改名总次数"收敛为"存活链头数"。
func TestMergeRenamesCollapseChain(t *testing.T) {
	stored := map[string]string{"A": "B", "B": "C"}
	fresh := map[string]string{}
	cur := []*adapter.Document{doc("C", "笔记/C.md", "x")}
	out := MergeRenames(stored, fresh, cur)
	if out["A"] != "C" {
		t.Fatalf("A 应解到最终 C：%v", out)
	}
	if _, ok := out["B"]; ok {
		t.Fatalf("中间环 B 应被丢弃：%v", out)
	}
	if len(out) != 1 {
		t.Fatalf("应收敛为 1 条：%v", out)
	}
}

// 多个独立链头：A→B、X→Y 互不影响，各保留链头→目标。
func TestMergeRenamesCollapseMultiChain(t *testing.T) {
	stored := map[string]string{"A": "B", "X": "Y"}
	cur := []*adapter.Document{doc("B", "B.md", "1"), doc("Y", "Y.md", "2")}
	out := MergeRenames(stored, nil, cur)
	if out["A"] != "B" || out["X"] != "Y" || len(out) != 2 {
		t.Fatalf("多链应各自保留：%v", out)
	}
}

// 环防御：A→B、B→A 异常态 → 折叠后丢弃（无链头）。
func TestMergeRenamesCollapseCycle(t *testing.T) {
	stored := map[string]string{"A": "B", "B": "A"}
	cur := []*adapter.Document{doc("A", "A.md", "1"), doc("B", "B.md", "2")}
	out := MergeRenames(stored, nil, cur)
	if len(out) != 0 {
		t.Fatalf("环应被折叠丢弃：%v", out)
	}
}

// 单条（无链）：A→B 保持不变。
func TestMergeRenamesCollapseSingle(t *testing.T) {
	stored := map[string]string{"A": "B"}
	cur := []*adapter.Document{doc("B", "B.md", "1")}
	out := MergeRenames(stored, nil, cur)
	if out["A"] != "B" || len(out) != 1 {
		t.Fatalf("单条应保持：%v", out)
	}
}
