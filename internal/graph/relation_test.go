package graph

// 关系查询单元测试（v0.1.5）：BFS 最短路径、双向 PPR 非对称性、证据链。
import (
	"testing"

	"serendipity-engine/internal/adapter"
)

// 构造：a-b 直达；a-c-b 二跳；a-c-e-b 三跳（与设计讨论的多路径例同构）。
func testRelGraph() *Graph {
	docs := []*adapter.Document{
		{ID: "a", Title: "A", Type: "note", Path: "a.md", Refs: []string{"b", "c"}, Text: "a"},
		{ID: "b", Title: "B", Type: "note", Path: "b.md", Refs: []string{"a", "c", "e"}, Text: "b"},
		{ID: "c", Title: "C", Type: "note", Path: "c.md", Refs: []string{"a", "b", "e"}, Text: "c"},
		{ID: "e", Title: "E", Type: "note", Path: "e.md", Refs: []string{"c", "b"}, Text: "e"},
	}
	return Build(docs)
}

// a-b 直达：hops=1、direct、激活 λ^1；证据链含文档 a/b。
func TestRelationDirect(t *testing.T) {
	g := testRelGraph()
	rel := g.ComputeRelation("a", "b")
	if rel == nil {
		t.Fatal("nil")
	}
	if rel.Hops != 1 || !rel.Direct {
		t.Fatalf("应直达：hops=%d direct=%v", rel.Hops, rel.Direct)
	}
	if rel.Activation != 0.7 {
		t.Fatalf("激活应 0.7：%v", rel.Activation)
	}
	if len(rel.Evidence) != 1 || len(rel.Evidence[0].Witnesses) == 0 {
		t.Fatalf("证据链缺失：%v", rel.Evidence)
	}
	// 多路径（a-c-b / a-c-e-b）不改变最短路径——只走直达
	if len(rel.Path) != 2 {
		t.Fatalf("最短路径应 [a b]：%v", rel.Path)
	}
}

// 无直达：a-e 最短 2 跳 a-c-e；激活 λ²。
func TestRelationTwoHop(t *testing.T) {
	g := testRelGraph()
	rel := g.ComputeRelation("a", "e")
	if rel.Hops != 2 || rel.Direct {
		t.Fatalf("应 2 跳非直达：hops=%d direct=%v", rel.Hops, rel.Direct)
	}
	if diff := rel.Activation - 0.49; diff > 1e-9 || diff < -1e-9 {
		t.Fatalf("激活应约 0.49：%v", rel.Activation)
	}
	if len(rel.Path) != 3 {
		t.Fatalf("最短路径应为 3 节点（a-?-e，存在 a-c-e 与 a-b-e 两条）：%v", rel.Path)
	}
	if len(rel.Evidence) != 2 {
		t.Fatalf("证据应有 2 条边：%v", rel.Evidence)
	}
}

// PPR 非对称：a 直达 b 但 b 也直达 a（对称图）→ 双向接近；
// 结构不同时（度差异）方向性会体现。此处验证两值都存在且 affinity 为均值。
func TestRelationPPR(t *testing.T) {
	g := testRelGraph()
	rel := g.ComputeRelation("a", "b")
	if rel.PPRFromTo <= 0 || rel.PPRToFrom <= 0 {
		t.Fatalf("PPR 应为正：%v / %v", rel.PPRFromTo, rel.PPRToFrom)
	}
	want := (rel.PPRFromTo + rel.PPRToFrom) / 2
	if rel.Affinity != want {
		t.Fatalf("affinity 应为均值：%v ≠ %v", rel.Affinity, want)
	}
}

// 不存在节点 → nil。
func TestRelationMissingNode(t *testing.T) {
	g := testRelGraph()
	if rel := g.ComputeRelation("a", "不存在"); rel != nil {
		t.Fatal("应 nil")
	}
}

// 自关系：hops=0、激活 1.0。
func TestRelationSelf(t *testing.T) {
	g := testRelGraph()
	rel := g.ComputeRelation("a", "a")
	if rel.Hops != 0 || rel.Activation != 1.0 {
		t.Fatalf("自关系错误：hops=%d act=%v", rel.Hops, rel.Activation)
	}
}

// 可达但无路径（不同连通分量）：hops=-1、activation=-1、无证据。
func TestRelationDisconnected(t *testing.T) {
	docs := []*adapter.Document{
		{ID: "a", Title: "A", Type: "note", Path: "a.md", Refs: []string{"b"}, Text: "a"},
		{ID: "b", Title: "B", Type: "note", Path: "b.md", Refs: []string{"a"}, Text: "b"},
		{ID: "x", Title: "X", Type: "note", Path: "x.md", Text: "孤立"},
		{ID: "y", Title: "Y", Type: "note", Path: "y.md", Refs: []string{"x"}, Text: "y"},
	}
	g := Build(docs)
	rel := g.ComputeRelation("a", "x")
	if rel == nil {
		t.Fatal("nil")
	}
	if rel.Hops != -1 || rel.Activation != -1 {
		t.Fatalf("不可达：hops=%d act=%v", rel.Hops, rel.Activation)
	}
	if rel.Path != nil || len(rel.Evidence) != 0 {
		t.Fatalf("不可达不应有路径/证据：%v / %v", rel.Path, rel.Evidence)
	}
}
