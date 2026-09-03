// 随机漫步（v0.1.7）与查询管线回归测试。
package roam

import (
	"math"
	"math/rand/v2"
	"testing"

	"serendipity-engine/internal/adapter"
	"serendipity-engine/internal/graph"
)

// testGraph 构造小图：a-e 是普通节点；s1 结构类型 / s2 空标题 / s3 孤立
// 应被 rollSeed 候选池排除（s1/s2/s3 都有边或度，唯独质量不合格）。
// 无向度：a=2 (b,c)，b=3 (a,c,d)，c=3 (a,b,e)，d=1 (b)，e=1 (c)。
func testGraph(t *testing.T) (*graph.Graph, *adapter.VaultProfile) {
	t.Helper()
	docs := []*adapter.Document{
		{ID: "a", Title: "Alpha", Type: "note", Refs: []string{"b", "c"}},
		{ID: "b", Title: "Beta", Type: "note", Refs: []string{"c", "d"}},
		{ID: "c", Title: "Gamma", Type: "note", Refs: []string{"e"}},
		{ID: "d", Title: "Delta", Type: "note", Refs: []string{}},
		{ID: "e", Title: "Epsilon", Type: "note", Refs: []string{"a"}},
		{ID: "s1", Title: "目录", Type: "toc", Refs: []string{"a"}},
		{ID: "s2", Title: "", Type: "note", Refs: []string{"a"}},
		{ID: "s3", Title: "孤立", Type: "note", Refs: []string{}},
	}
	p := &adapter.VaultProfile{StructuralTypes: []string{"toc"}}
	return graph.Build(docs), p
}

func testOpt() Options {
	return Options{Top: 10, Hops: 3, Lambda: 0.7, Theta: 0.1,
		Alpha: 0.5, Beta: 0.5}
}

// 候选池过滤：结构类型 / 空标题 / 孤立 永不被滚出。
func TestRollSeedFilters(t *testing.T) {
	g, p := testGraph(t)
	valid := map[string]bool{"a": true, "b": true, "c": true, "d": true, "e": true}
	for i := 0; i < 300; i++ {
		rng := rand.New(rand.NewPCG(uint64(i), 1))
		id := rollSeed(g, p, rng, nil, 0.5)
		if id == "" {
			t.Fatalf("候选池意外为空")
		}
		if !valid[id] {
			t.Fatalf("滚到了应排除节点 %s", id)
		}
	}
}

// 防重复：avoid 列表内的节点永不被滚出。
func TestRollSeedAvoid(t *testing.T) {
	g, p := testGraph(t)
	avoid := []string{"a", "b", "c"}
	for i := 0; i < 200; i++ {
		rng := rand.New(rand.NewPCG(uint64(i), 2))
		id := rollSeed(g, p, rng, avoid, 0.5)
		if id == "a" || id == "b" || id == "c" {
			t.Fatalf("avoid 节点 %s 仍被滚出", id)
		}
		if id == "" {
			t.Fatalf("候选池意外为空")
		}
	}
}

// 加权方向：alpha 越大，高度节点（b/c，度 3）占比越高；
// alpha=0（均匀）时高低度占比接近（容差 5%）。
func TestRollSeedWeighting(t *testing.T) {
	g, p := testGraph(t)
	weighted := map[string]int{}
	for i := 0; i < 5000; i++ {
		rng := rand.New(rand.NewPCG(uint64(i), 3))
		weighted[rollSeed(g, p, rng, nil, 1.0)]++
	}
	wHigh := weighted["b"] + weighted["c"] // 度 3
	wLow := weighted["d"] + weighted["e"]  // 度 1
	if wHigh <= wLow {
		t.Fatalf("度加权下高度节点占比应显著更高: b+c=%d, d+e=%d", wHigh, wLow)
	}

	uniform := map[string]int{}
	for i := 0; i < 5000; i++ {
		rng := rand.New(rand.NewPCG(uint64(i), 4))
		uniform[rollSeed(g, p, rng, nil, 0.0)]++
	}
	uHigh := uniform["b"] + uniform["c"]
	uLow := uniform["d"] + uniform["e"]
	if math.Abs(float64(uHigh-uLow)) > 500 {
		t.Fatalf("均匀采样下高低度占比应接近: b+c=%d, d+e=%d", uHigh, uLow)
	}
}

// 可复现：同种子 → 同一起点 + 同一簇。
func TestComputeRandomDeterministic(t *testing.T) {
	g, p := testGraph(t)
	opt := testOpt()
	o1 := ComputeRandom(g, p, opt, Roll{Rng: rand.New(rand.NewPCG(123, 5)), Alpha: 0.5})
	o2 := ComputeRandom(g, p, opt, Roll{Rng: rand.New(rand.NewPCG(123, 5)), Alpha: 0.5})
	if len(o1.Anchors) != 1 {
		t.Fatalf("随机漫步应恰好一个锚点, got %d", len(o1.Anchors))
	}
	if o1.Anchors[0].ID != o2.Anchors[0].ID {
		t.Fatalf("同种子应同一起点: %s vs %s", o1.Anchors[0].ID, o2.Anchors[0].ID)
	}
	if len(o1.Results) != len(o2.Results) {
		t.Fatalf("同种子应同一簇规模")
	}
	for i := range o1.Results {
		if o1.Results[i].ID != o2.Results[i].ID {
			t.Fatalf("同种子簇不同: %s vs %s", o1.Results[i].ID, o2.Results[i].ID)
		}
	}
}

// 起点本身绝不出现在簇结果里（与查询漫游的种子排除同一口径）。
func TestComputeRandomSeedExcludedFromResults(t *testing.T) {
	g, p := testGraph(t)
	opt := testOpt()
	for i := 0; i < 40; i++ {
		o := ComputeRandom(g, p, opt, Roll{Rng: rand.New(rand.NewPCG(uint64(i), 6)), Alpha: 0.5})
		if len(o.Anchors) != 1 {
			t.Fatalf("随机漫步应恰好一个锚点")
		}
		seed := o.Anchors[0].ID
		for _, r := range o.Results {
			if r.ID == seed {
				t.Fatalf("种子 %s 不应出现在簇结果里", seed)
			}
		}
	}
}

// 起点锚点带 Random 标记（前端 🎲 展示）。
func TestComputeRandomAnchorFlag(t *testing.T) {
	g, p := testGraph(t)
	o := ComputeRandom(g, p, testOpt(), Roll{Rng: rand.New(rand.NewPCG(9, 7)), Alpha: 0.5})
	if len(o.Anchors) != 1 || !o.Anchors[0].Random {
		t.Fatalf("随机漫步锚点应带 Random 标记")
	}
}

// 防重复联动：avoid 只剩一个候选时，必滚出它。
func TestComputeRandomAvoidLeavesRemaining(t *testing.T) {
	g, p := testGraph(t)
	o := ComputeRandom(g, p, testOpt(), Roll{
		Rng: rand.New(rand.NewPCG(3, 8)), Alpha: 0.5,
		Avoid: []string{"a", "b", "c", "d"},
	})
	if len(o.Anchors) != 1 || o.Anchors[0].ID != "e" {
		t.Fatalf("只剩 e 可滚: got %v", o.Anchors)
	}
}

// 无随机源（rng=nil）→ 空结果不 panic。
func TestComputeRandomNilRng(t *testing.T) {
	g, p := testGraph(t)
	o := ComputeRandom(g, p, testOpt(), Roll{Alpha: 0.5})
	if len(o.Anchors) != 0 {
		t.Fatalf("无随机源应返回空 Outcome")
	}
}

// 查询路径回归：拆出 clusterFromSeeds 后行为不变（a 的簇非空且不含种子）。
func TestComputeQueryPathUnchanged(t *testing.T) {
	g, p := testGraph(t)
	out := Compute(g, p, "Alpha", testOpt())
	if len(out.Anchors) != 1 || out.Anchors[0].ID != "a" {
		t.Fatalf("查询 Alpha 应锚定 a, got %v", out.Anchors)
	}
	if out.Fallback != ModeNormal {
		t.Fatalf("不应降级, fallback=%d", out.Fallback)
	}
	if len(out.Results) == 0 {
		t.Fatalf("a 的簇不应为空")
	}
	for _, r := range out.Results {
		if r.ID == "a" {
			t.Fatalf("种子 a 不应出现在簇里")
		}
	}
}
