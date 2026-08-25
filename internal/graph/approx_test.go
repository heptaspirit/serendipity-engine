// Package graph 潜在关联（近似边）候选单元测试（roadmap #15，backlog §3.6）。
// 验证：2-hop 候选生成、三算法 Borda 聚合排序、top-K 节流、全局去重、排除口径。
package graph

import (
	"reflect"
	"testing"

	"serendipity-engine/internal/adapter"
)

// TestPotentialLinksBasic：三角形图（A-B-C，A-C 无直接边）→ 应产出 A-C 一对，
// 且 A、C 是彼此的 2-hop 候选（共同邻居 B）。
// 注：加隔离节点 D/E 撑大 Nodes——hubThresh = Nodes/2，小图会把低度端点误伤
// （与 Similar #12 同口径；真实库数百节点无此问题）。
func TestPotentialLinksBasic(t *testing.T) {
	docs := []*adapter.Document{
		{ID: "A", Title: "A", Type: "note", Refs: []string{"B"}},
		{ID: "B", Title: "B", Type: "note", Refs: []string{"A", "C"}},
		{ID: "C", Title: "C", Type: "note", Refs: []string{"B"}},
		{ID: "D", Title: "D", Type: "note", Refs: []string{}}, // 隔离撑大 Nodes
		{ID: "E", Title: "E", Type: "note", Refs: []string{}},
	}
	g := Build(docs)
	out := g.PotentialLinks(2, nil)
	// 应至少有一对 A-C（唯一无直接边但有共同邻居的对）
	found := false
	for _, e := range out {
		if (e.A == "A" && e.B == "C") || (e.A == "C" && e.B == "A") {
			found = true
			if len(e.Shared) != 1 || e.Shared[0] != "B" {
				t.Fatalf("A-C 共同邻居应 [B]：%v", e.Shared)
			}
			if len(e.Algorithms) != 3 {
				t.Fatalf("应三算法命中：%v", e.Algorithms)
			}
			if e.Score <= 0 {
				t.Fatalf("Borda 分应 > 0：%v", e.Score)
			}
		}
	}
	if !found {
		t.Fatalf("应产出 A-C 潜在关联：%+v", out)
	}
}

// TestPotentialLinksNoDirectNeighbor：直接邻居（已链接）绝不进潜在关联。
// A-B 直连 → 即使有公共邻居也不应产出 A-B。
func TestPotentialLinksNoDirectNeighbor(t *testing.T) {
	docs := []*adapter.Document{
		{ID: "A", Title: "A", Type: "note", Refs: []string{"B", "X"}},
		{ID: "B", Title: "B", Type: "note", Refs: []string{"A", "X"}},
		{ID: "X", Title: "X", Type: "note", Refs: []string{"A", "B"}},
	}
	g := Build(docs)
	out := g.PotentialLinks(2, nil)
	for _, e := range out {
		if (e.A == "A" && e.B == "B") || (e.A == "B" && e.B == "A") {
			t.Fatalf("直接邻居 A-B 不应进潜在关联：%+v", e)
		}
	}
}

// TestPotentialLinksDedup：无向对规范化（A<B），同一对不重复。
func TestPotentialLinksDedup(t *testing.T) {
	docs := []*adapter.Document{
		{ID: "A", Title: "A", Type: "note", Refs: []string{"B", "X"}},
		{ID: "B", Title: "B", Type: "note", Refs: []string{"A", "X"}},
		{ID: "X", Title: "X", Type: "note", Refs: []string{"A", "B", "C"}},
		{ID: "C", Title: "C", Type: "note", Refs: []string{"X", "B"}},
	}
	g := Build(docs)
	out := g.PotentialLinks(2, nil)
	seen := map[string]bool{}
	for _, e := range out {
		if e.A >= e.B {
			t.Fatalf("未规范化（应 A<B）：%s >= %s", e.A, e.B)
		}
		key := e.A + "\x00" + e.B
		if seen[key] {
			t.Fatalf("重复对：%v", e)
		}
		seen[key] = true
	}
}

// TestPotentialLinksStructuralExcluded：结构类型节点不做候选端点（与 Similar 同口径）。
func TestPotentialLinksStructuralExcluded(t *testing.T) {
	docs := []*adapter.Document{
		{ID: "A", Title: "A", Type: "note", Refs: []string{"B"}},
		{ID: "B", Title: "B", Type: "note", Refs: []string{"A", "C"}},
		{ID: "C", Title: "C", Type: "note", Refs: []string{"B"}},
		{ID: "目录", Title: "目录", Type: "dir", Refs: []string{"A", "C"}}, // 结构类型
	}
	g := Build(docs)
	// 排除 dir 后：A-C 仍有共同邻居 B，应正常产出；"目录" 不应作为端点出现
	out := g.PotentialLinks(2, map[string]bool{"dir": true})
	for _, e := range out {
		if e.A == "目录" || e.B == "目录" {
			t.Fatalf("结构类型不应作端点：%+v", e)
		}
	}
}

// TestPotentialLinksTopKThrottle：top-K 节流——perNodeK=1 时每端点最多 1 条。
// 星形中心 hub（deg 3 < 半数 2? 构造使 hub 度 ≥ 半数排除）——这里用简单
// 菱形图验证节流数量界。
func TestPotentialLinksTopKThrottle(t *testing.T) {
	// 5 节点：中心 H 连接 4 叶（H 度 4 ≥ 半数 2.5 → 排除 hub），叶之间无直连。
	// 每叶的 2-hop 候选 = 其他叶（经 H）。perNodeK=1 → 全局至多 4 对。
	docs := []*adapter.Document{
		{ID: "H", Title: "H", Type: "note", Refs: []string{"L1", "L2", "L3", "L4"}},
		{ID: "L1", Title: "L1", Type: "note", Refs: []string{"H"}},
		{ID: "L2", Title: "L2", Type: "note", Refs: []string{"H"}},
		{ID: "L3", Title: "L3", Type: "note", Refs: []string{"H"}},
		{ID: "L4", Title: "L4", Type: "note", Refs: []string{"H"}},
	}
	g := Build(docs)
	out := g.PotentialLinks(1, nil)
	if len(out) > 4 {
		t.Fatalf("perNodeK=1 全局应 ≤ 4 对：%d", len(out))
	}
	for _, e := range out {
		if e.A == "H" || e.B == "H" {
			t.Fatalf("hub 不应作端点：%+v", e)
		}
	}
}

// TestPotentialLinksBordaOrdering：Borda 聚合排序——共享邻居更多（且邻居度更低）
// 的对分数更高。A-B 经 X+Y 共享，A-C 只经 X 共享——前者应排前。
// 隔离节点撑大 Nodes 防 hubThresh 误伤（同 Basic）。
func TestPotentialLinksBordaOrdering(t *testing.T) {
	docs := []*adapter.Document{
		{ID: "A", Title: "A", Type: "note", Refs: []string{"X", "Y"}},
		{ID: "B", Title: "B", Type: "note", Refs: []string{"X", "Y"}}, // 与 A 共享 X+Y
		{ID: "C", Title: "C", Type: "note", Refs: []string{"X"}},     // 与 A 只共享 X
		{ID: "X", Title: "X", Type: "note", Refs: []string{"A", "B", "C"}},
		{ID: "Y", Title: "Y", Type: "note", Refs: []string{"A", "B"}},
		{ID: "D", Title: "D", Type: "note", Refs: []string{}}, // 隔离撑大 Nodes
		{ID: "E", Title: "E", Type: "note", Refs: []string{}},
		{ID: "F", Title: "F", Type: "note", Refs: []string{}},
	}
	g := Build(docs)
	out := g.PotentialLinks(2, nil)
	scoreOf := func(a, b string) float64 {
		for _, e := range out {
			if (e.A == a && e.B == b) || (e.A == b && e.B == a) {
				return e.Score
			}
		}
		return -1
	}
	sAB := scoreOf("A", "B") // 共享 X+Y
	sAC := scoreOf("A", "C") // 只共享 X
	if sAB < 0 || sAC < 0 {
		t.Fatalf("缺少候选：A-B=%v A-C=%v（全部：%+v）", sAB, sAC, out)
	}
	if sAB <= sAC {
		t.Fatalf("A-B（共享 X+Y）应比 A-C（只共享 X）分高：%v vs %v", sAB, sAC)
	}
}

// TestPotentialLinksEmpty：空图/无边图 → 空清单（不报错）。
func TestPotentialLinksEmpty(t *testing.T) {
	g := Build([]*adapter.Document{
		{ID: "A", Title: "A", Type: "note", Refs: []string{}},
	})
	if out := g.PotentialLinks(2, nil); len(out) != 0 {
		t.Fatalf("孤立节点图应空：%+v", out)
	}
}

// TestPotentialLinksDeterministic：相同输入两次调用结果一致（稳定排序）。
func TestPotentialLinksDeterministic(t *testing.T) {
	docs := []*adapter.Document{
		{ID: "A", Title: "A", Type: "note", Refs: []string{"B", "X"}},
		{ID: "B", Title: "B", Type: "note", Refs: []string{"A", "X", "C"}},
		{ID: "C", Title: "C", Type: "note", Refs: []string{"B", "X"}},
		{ID: "X", Title: "X", Type: "note", Refs: []string{"A", "B", "C"}},
	}
	g := Build(docs)
	o1 := g.PotentialLinks(2, nil)
	o2 := g.PotentialLinks(2, nil)
	if !reflect.DeepEqual(o1, o2) {
		t.Fatalf("应确定性：%+v ≠ %+v", o1, o2)
	}
}
