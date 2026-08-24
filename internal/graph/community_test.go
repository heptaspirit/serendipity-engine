// 社区发现（Leiden）单元测试（v0.1.12，roadmap #10）。
// 用两组分明 cluster 验证社区划分质量与模块度；孤立节点不成社区。
package graph

import (
	"testing"

	"serendipity-engine/internal/adapter"
)

func cliqueDocs(ids []string) []*adapter.Document {
	var docs []*adapter.Document
	for _, id := range ids {
		refs := []string{}
		for _, o := range ids {
			if o != id {
				refs = append(refs, o)
			}
		}
		docs = append(docs, &adapter.Document{ID: id, Title: id, Type: "note", Refs: refs})
	}
	return docs
}

// 两团：A1-A4 团 + B1-B4 团，各团内全连接，仅 A4-B1 一座桥。
// Leiden 应拆成 ≥2 社区、模块度 > 0；孤立节点（orphan）不成社区。
func TestCommunitiesTwoClusters(t *testing.T) {
	ads := cliqueDocs([]string{"A1", "A2", "A3", "A4"})
	bds := cliqueDocs([]string{"B1", "B2", "B3", "B4"})
	// 加桥：A4 ↔ B1
	for _, d := range ads {
		if d.ID == "A4" {
			d.Refs = append(d.Refs, "B1")
		}
	}
	for _, d := range bds {
		if d.ID == "B1" {
			d.Refs = append(d.Refs, "A4")
		}
	}
	docs := append(ads, bds...)
	docs = append(docs, &adapter.Document{ID: "orphan", Title: "orphan", Type: "note"}) // 孤立
	g := Build(docs)

	res, err := g.Communities(1.0, 42)
	if err != nil {
		t.Fatalf("Communities: %v", err)
	}
	if res.CommunityCount < 2 {
		t.Fatalf("两团应 ≥2 社区，got %d", res.CommunityCount)
	}
	if res.Modularity <= 0 {
		t.Fatalf("模块度应 > 0，got %f", res.Modularity)
	}
	// 孤立节点不进 Membership（职责分离）
	if _, ok := res.Membership["orphan"]; ok {
		t.Fatal("孤立节点不应进 Membership")
	}
	// A 团与 B 团应分属不同社区（Membership 应有 A1 与 B1 且不同）
	a1, a1ok := res.Membership["A1"]
	b1, b1ok := res.Membership["B1"]
	if !a1ok || !b1ok || a1 == b1 {
		t.Fatalf("A1/B1 应分属不同社区：Membership=%v", res.Membership)
	}
	// 社区列表按 Size 降序
	if len(res.Communities) < 2 {
		t.Fatalf("应至少 2 个社区：%v", res.Communities)
	}
	for i := 1; i < len(res.Communities); i++ {
		if res.Communities[i-1].Size < res.Communities[i].Size {
			t.Fatalf("社区应按 Size 降序：%v", res.Communities)
		}
	}
}

// 全孤立（无任何边）→ 空结果（不报错）。
func TestCommunitiesEmpty(t *testing.T) {
	docs := []*adapter.Document{
		{ID: "a", Title: "A", Type: "note"},
		{ID: "b", Title: "B", Type: "note"},
	}
	g := Build(docs)
	res, err := g.Communities(1.0, 1)
	if err != nil {
		t.Fatalf("空图不应报错：%v", err)
	}
	if res.CommunityCount != 0 {
		t.Fatalf("全孤立应 0 社区，got %d", res.CommunityCount)
	}
}
