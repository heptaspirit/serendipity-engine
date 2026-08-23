package score

// 归一化融合单元测试（v0.1.6 桶内归一化）：深跳桶不再整体趋 0，
// 且 min-max 单调不改变桶内排序（输出序列与全局归一化一致）。
import (
	"testing"

	"serendipity-engine/internal/adapter"
	"serendipity-engine/internal/graph"
)

// 小图：s(种子) → h1a/h1b（1 跳），经 m 到 h2a/h2b（2 跳）。
func testGraph() *graph.Graph {
	docs := []*adapter.Document{
		{ID: "s", Title: "S", Type: "note", Path: "s.md", Refs: []string{"h1a", "h1b", "m"}},
		{ID: "h1a", Title: "H1A", Type: "note", Path: "h1a.md", Refs: []string{"s"}},
		{ID: "h1b", Title: "H1B", Type: "note", Path: "h1b.md", Refs: []string{"s"}},
		{ID: "m", Title: "M", Type: "note", Path: "m.md", Refs: []string{"s", "h2a", "h2b"}},
		{ID: "h2a", Title: "H2A", Type: "note", Path: "h2a.md", Refs: []string{"m"}},
		{ID: "h2b", Title: "H2B", Type: "note", Path: "h2b.md", Refs: []string{"m"}},
	}
	return graph.Build(docs)
}

func testMaps() (map[string]graph.ActivationResult, map[string]float64) {
	actMap := map[string]graph.ActivationResult{
		"h1a": {ID: "h1a", Score: 0.7, Hops: 1, Path: []string{"s", "h1a"}},
		"h1b": {ID: "h1b", Score: 0.7, Hops: 1, Path: []string{"s", "h1b"}},
		"h2a": {ID: "h2a", Score: 0.49, Hops: 2, Path: []string{"s", "m", "h2a"}},
		"h2b": {ID: "h2b", Score: 0.49, Hops: 2, Path: []string{"s", "m", "h2b"}},
	}
	// PPR：1 跳候选显著高于 2 跳（模拟真实结构分——深跳 ppr 趋 0）
	pprMap := map[string]float64{
		"h1a": 0.02, "h1b": 0.018, "h2a": 0.0002, "h2b": 0.0001,
	}
	return actMap, pprMap
}

func rank4(t *testing.T) []Result {
	t.Helper()
	g := testGraph()
	actMap, pprMap := testMaps()
	out := Rank(g, actMap, pprMap, RankOpts{
		Alpha: 0.5, Beta: 0.5, TopN: 4, HopQuota: [3]float64{0.5, 0.3, 0.2},
	})
	return out
}

// 桶内归一化：2 跳桶内最好的节点（h2a）应有正区分度且高于 h2b（不再整体 score=0）。
func TestRankBucketNormalization(t *testing.T) {
	out := rank4(t)
	var h2a, h2b float64
	for _, r := range out {
		switch r.ID {
		case "h2a":
			h2a = r.Score
		case "h2b":
			h2b = r.Score
		}
	}
	if h2a <= 0 {
		t.Fatalf("桶内归一化后 h2a 应有正区分度（v0.1.6 修复 score=0）：%v", h2a)
	}
	if h2a <= h2b {
		t.Fatalf("h2a(ppr 更高) 应高于 h2b：h2a=%v h2b=%v", h2a, h2b)
	}
}

// 单调性：1 跳桶内 h1a(ppr 更高) 仍排在 h1b 前（输出序列不被归一化破坏）。
func TestRankBucketOrderPreserved(t *testing.T) {
	out := rank4(t)
	var first string
	for _, r := range out {
		if r.ID == "h1a" || r.ID == "h1b" {
			if first == "" {
				first = r.ID
			}
		}
	}
	if first != "h1a" {
		t.Fatalf("桶内排序被破坏：1 跳桶首个应为 h1a，得到 %s", first)
	}
}
