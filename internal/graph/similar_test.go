package graph

// 结构相似（Jaccard 孪生）与节点详情单元测试（v0.1.11）。
// similar：白盒结构相似（共同邻居多但互不链接），度偏置防御；
// node：L0 摘要 + L1 邻居/被引用；
// stats 缓存：Graph 不可变，首次计算后缓存。

import (
	"reflect"
	"strings"
	"testing"

	"serendipity-engine/internal/adapter"
)

func testSimilarGraph() *Graph {
	// A/B/C 共享 X、Y（但 A/B/C 互不链接 → 说"同一件事"）；D 链接 A（无 X/Y 共同
	// 邻居 + 是 A 直接邻居 → 不相似/排除）；hub（目录类型）链接 X 但结构类型应排除。
	docs := []*adapter.Document{
		{ID: "A", Title: "A", Type: "人物", Path: "A.md", Refs: []string{"X", "Y"}, Text: "a"},
		{ID: "B", Title: "B", Type: "人物", Path: "B.md", Refs: []string{"X", "Y"}, Text: "b"},
		{ID: "C", Title: "C", Type: "人物", Path: "C.md", Refs: []string{"X"}, Text: "c"},
		{ID: "D", Title: "D", Type: "人物", Path: "D.md", Refs: []string{"A"}, Text: "d"},
		{ID: "X", Title: "X", Type: "设定", Path: "X.md", Refs: []string{"A", "B", "C"}, Text: "x"},
		{ID: "Y", Title: "Y", Type: "设定", Path: "Y.md", Refs: []string{"A", "B"}, Text: "y"},
		{ID: "hub", Title: "hub", Type: "目录", Path: "hub.md", Refs: []string{"X"}, Text: "h"},
	}
	return Build(docs)
}

// A 与 B 共享 {X,Y}：Jaccard = 2/2 = 1；A 与 C 共享 {X}：Jaccard = 1/(2+1-1)=0.5。
func TestSimilarJaccard(t *testing.T) {
	g := testSimilarGraph()
	structural := map[string]bool{"目录": true}
	sim := g.Similar("A", 10, structural)
	byID := map[string]float64{}
	shared := map[string][]string{}
	for _, s := range sim {
		byID[s.ID] = s.Score
		shared[s.ID] = s.Shared
	}
	if byID["B"] == 0 {
		t.Fatalf("A 应相似 B：%v", byID)
	}
	if shared["B"] == nil {
		t.Fatalf("B 应带共享邻居证据：%v", shared)
	}
	// D 是 A 的直接邻居（已链接=相关非相似）且无共同邻居 → 排除
	if _, ok := byID["D"]; ok {
		t.Fatalf("直接邻居 D 不应被当相似：%v", byID)
	}
	// hub 是结构类型 → 排除
	if _, ok := byID["hub"]; ok {
		t.Fatalf("结构类型 hub 不应出现在相似列：%v", byID)
	}
}

// 相似度：B（2 共享）应在 C（1 共享）前。
func TestSimilarOrdering(t *testing.T) {
	g := testSimilarGraph()
	structural := map[string]bool{"目录": true}
	sim := g.Similar("A", 10, structural)
	if len(sim) < 2 {
		t.Fatalf("至少 2 个候选：%v", sim)
	}
	if sim[0].ID != "B" {
		t.Fatalf("B 应排首位：%v", sim[0].ID)
	}
	if sim[0].Score < sim[1].Score {
		t.Fatalf("应按相似度降序：%v", sim)
	}
}

// 不存在节点 → nil；孤立节点无相似。
func TestSimilarMissingOrIsolated(t *testing.T) {
	g := testSimilarGraph()
	if sim := g.Similar("不存在", 10, nil); sim != nil {
		t.Fatalf("不存在应返回 nil：%v", sim)
	}
	docs := []*adapter.Document{
		{ID: "A", Title: "A", Type: "人物", Path: "A.md", Refs: []string{"B"}, Text: "a"},
		{ID: "B", Title: "B", Type: "人物", Path: "B.md", Refs: []string{"A"}, Text: "b"},
		{ID: "iso", Title: "iso", Type: "人物", Path: "iso.md", Text: "孤立"},
	}
	g2 := Build(docs)
	if sim := g2.Similar("iso", 10, nil); sim != nil {
		t.Fatalf("孤立节点应无相似：%v", sim)
	}
}

// NodeDetail：L0 摘要截断 + L1 邻居/被引用。
func TestNodeDetail(t *testing.T) {
	docs := []*adapter.Document{
		{ID: "a", Title: "A", Type: "note", Path: "a.md", Refs: []string{"b"}, Text: "a正文"},
		{ID: "b", Title: "B", Type: "note", Path: "b.md", Refs: []string{"a"}, Text: strings.Repeat("正文", 200)},
	}
	g := Build(docs)
	d := g.NodeDetail("a")
	if d == nil {
		t.Fatal("nil")
	}
	if d.Deg != 1 {
		t.Fatalf("deg 应 1：%v", d.Deg)
	}
	if len(d.Neighbors) != 1 || d.Neighbors[0].ID != "b" {
		t.Fatalf("邻居应 [b]：%v", d.Neighbors)
	}
	if len(d.Backlinks) != 1 || d.Backlinks[0].ID != "b" {
		t.Fatalf("被引用应 [b]：%v", d.Backlinks)
	}
	db := g.NodeDetail("b")
	if db.Text == "" || len([]rune(db.Text)) > textSummaryMax+1 {
		t.Fatalf("正文应截断：len=%d", len([]rune(db.Text)))
	}
	if g.NodeDetail("不存在") != nil {
		t.Fatal("不存在应 nil")
	}
}

// Stats 缓存：Graph 不可变，连续调用结果稳定。
func TestStatsCached(t *testing.T) {
	g := testSimilarGraph()
	s1 := g.Stats()
	s2 := g.Stats()
	if !reflect.DeepEqual(s1, s2) {
		t.Fatalf("Stats 应缓存稳定：%v ≠ %v", s1, s2)
	}
	if s1.Nodes != 7 {
		t.Fatalf("节点应 7：%v", s1.Nodes)
	}
	if s1.TopHubs[0].Deg == 0 {
		t.Fatal("TopHubs[0].Deg 不应 0")
	}
}
