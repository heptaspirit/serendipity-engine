package graph

// 建图链路解析测试（v0.1.13，反馈 #2 引擎侧）：非精确 ID 的 [[名字]] 应重定向到
// 拥有该 title/alias 的节点，不再被丢成 dangling；精确 ID 行为保持不变。
import (
	"os"
	"path/filepath"
	"testing"

	"serendipity-engine/internal/adapter"
)

// title 重定向：被引用的 "周真" 非任何节点 ID，但人物_012 的 title 是"周真" → 连过去。
func TestBuildRedirectByTitle(t *testing.T) {
	docs := []*adapter.Document{
		{ID: "a", Title: "Anchor", Type: "note", Refs: []string{"周真"}},
		{ID: "人物_012", Title: "周真", Type: "人物"},
	}
	g := Build(docs)
	if len(g.dangling) != 0 {
		t.Fatalf("不应有 dangling: %v", g.dangling)
	}
	nb := g.adj["a"]
	if len(nb) != 1 || nb[0] != "人物_012" {
		t.Fatalf("a 应重定向连到 人物_012, got %v", nb)
	}
}

// alias 重定向：title 不同、但 aliases 含"周真" → 连到 alias 主节点。
func TestBuildRedirectByAlias(t *testing.T) {
	docs := []*adapter.Document{
		{ID: "a", Title: "Anchor", Type: "note", Refs: []string{"周真"}},
		{ID: "人物_012", Title: "周玄", Type: "人物", Aliases: []string{"周真"}},
	}
	g := Build(docs)
	if len(g.dangling) != 0 {
		t.Fatalf("不应有 dangling: %v", g.dangling)
	}
	nb := g.adj["a"]
	if len(nb) != 1 || nb[0] != "人物_012" {
		t.Fatalf("a 应连到 alias 主节点 人物_012, got %v", nb)
	}
}

// 精确 ID 优先保持不变：存在 ID=周真 的节点（归档文件）时仍连它，不重定向到别名节点。
func TestBuildExactIDWins(t *testing.T) {
	docs := []*adapter.Document{
		{ID: "a", Title: "Anchor", Type: "note", Refs: []string{"周真"}},
		{ID: "周真", Title: "周真", Type: "其他"}, // 归档文件，精确 ID
		{ID: "人物_012", Title: "周玄", Type: "人物", Aliases: []string{"周真"}},
	}
	g := Build(docs)
	if len(g.dangling) != 0 {
		t.Fatalf("不应有 dangling: %v", g.dangling)
	}
	if nb := g.adj["a"]; len(nb) != 1 || nb[0] != "周真" {
		t.Fatalf("精确 ID 应优先连 周真 归档节点, got %v", nb)
	}
}

// 重定向后折叠成自环：节点 C 的 ref 重定向回自身 → 不计边、不计 dangling。
func TestBuildRedirectToSelf(t *testing.T) {
	docs := []*adapter.Document{
		{ID: "人物_012", Title: "周真", Type: "人物", Refs: []string{"周真"}},
	}
	g := Build(docs)
	if len(g.dangling) != 0 {
		t.Fatalf("折叠为自环不应 dangling: %v", g.dangling)
	}
	if len(g.adj["人物_012"]) != 0 {
		t.Fatalf("自环不计边: got %v", g.adj["人物_012"])
	}
}

// 无 title/alias 命中 → 仍按悬空记录（真死链/格式噪声，交由上层报告）。
func TestBuildNonRedirectableStaysDangling(t *testing.T) {
	docs := []*adapter.Document{
		{ID: "a", Title: "Anchor", Type: "note", Refs: []string{"不存在的目标"}},
	}
	g := Build(docs)
	if g.dangling["不存在的目标"] != 1 {
		t.Fatalf("应记 dangling: %v", g.dangling)
	}
	if len(g.adj["a"]) != 0 {
		t.Fatalf("无命中不应连边: got %v", g.adj["a"])
	}
}

// 反斜杠解析（反馈 #2 全链路验证）：[[人物_001\]] 经 ParseFile 剥成 人物_001 后
// 应连到真实节点，不产生带 \ 的 dangling。证明重解析即可消除旧图里的该类噪声。
func TestBuildBackslashRefResolves(t *testing.T) {
	dir := t.TempDir()
	// 设定_人物称呼 用表格反斜杠形式引用人物
	if err := os.WriteFile(filepath.Join(dir, "设定_人物称呼.md"), []byte("# 设定_人物称呼\n\n| 角色 | 称呼 |\n|---|---|\n| [[人物_001\\]] | a |\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "人物_001.md"), []byte("# 人物一\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	profile := adapter.DefaultObsidianProfile()
	doc1, err := adapter.ParseFile(filepath.Join(dir, "设定_人物称呼.md"), dir, profile)
	if err != nil {
		t.Fatal(err)
	}
	doc2, err := adapter.ParseFile(filepath.Join(dir, "人物_001.md"), dir, profile)
	if err != nil {
		t.Fatal(err)
	}
	g := Build([]*adapter.Document{doc1, doc2})
	if len(g.dangling) != 0 {
		t.Fatalf("剥反斜杠后不应有 dangling: %v", g.dangling)
	}
	// 设定_人物称呼 通过 ref 人物_001 连到 人物_001
	found := false
	for _, x := range g.adj["人物_001"] {
		if x == "设定_人物称呼" {
			found = true
		}
	}
	if !found {
		t.Fatalf("人物_001 应连到 设定_人物称呼: %v", g.adj["人物_001"])
	}
}
