// Package roam 封装查询漫游管线（CLI 与 Web 共用）：
// 锚定 → PPR + 激活扩散 → 排除（种子/目录枢纽/结构类型）→ 归一化融合 + 跳数配额 → 降级兜底。
//
// ▍随机漫步（v0.1.7）
//   查询漫游要求用户先给意图（打字或点卡片）；随机漫步（ComputeRandom）反过来：
//   随机 roll 一个候选节点作起点，然后走与查询完全相同的簇管线——"节点 + 它的簇"
//   一次给出。roll 取舍见 rollSeed（质量门槛过滤 + deg^α 加权 + 防重复 + 可复现种子）。
//
// ▍降级搜索词（v0.1.3）
//   Web 端"点击卡片继续漫游"用节点 ID（虎鲸为纯数字块 ID）作查询词；锚定后若
//   邻居稀疏走 ModeSparse 全文兜底时，纯数字全文搜必空（孤立节点点击"没反应"）。
//   修复：查询词为纯数字且唯一锚定时，降级搜索改用锚点 title。
package roam

import (
	"math"
	"math/rand/v2"
	"strings"

	"serendipity-engine/internal/adapter"
	"serendipity-engine/internal/graph"
	"serendipity-engine/internal/score"
)

// Options 漫游参数。
type Options struct {
	Top              int
	Hops             int
	Lambda, Theta    float64
	Alpha, Beta      float64
	FilterStructural bool // 簇输出是否排除结构类型（实体查询 true；文本搜索式降级 false）
}

// Anchor 锚点信息。
type Anchor struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Type   string `json:"type"`
	Match  int    `json:"match"`            // 匹配级别 1-5（like/tag/alias/title/exact），展示筛选用
	Deg    int    `json:"deg"`              // 图度，同级别展示排序的次级权重
	Random bool   `json:"random,omitempty"` // 随机漫步起点（v0.1.7，🎲 展示标记）
}

// FallbackMode 降级模式。
type FallbackMode int

const (
	ModeNormal   FallbackMode = iota
	ModeNoAnchor              // 无锚点 → 全文检索（不过滤结构）
	ModeSparse                // 锚点命中但邻居稀疏 → 全文检索（过滤结构）
)

// Outcome 漫游结果。
type Outcome struct {
	Anchors      []Anchor        `json:"anchors"`
	Results      []score.Result  `json:"results"`
	Fallback     FallbackMode    `json:"fallback"`
	FallbackHits []graph.TextHit `json:"fallback_hits"`
}

// Compute 执行一次查询漫游。
func Compute(g *graph.Graph, p *adapter.VaultProfile, query string, opt Options) *Outcome {
	// 空查询：不漫游（Resolve('') 的 Contains 恒真会把全图当锚点）
	if strings.TrimSpace(query) == "" {
		return &Outcome{}
	}
	matches := g.Resolve(query)
	if len(matches) == 0 {
		// 无锚点：全文检索兜底（搜索式交互，不过滤结构类型）
		return &Outcome{
			Fallback:     ModeNoAnchor,
			FallbackHits: g.TextSearch(query, opt.Top*2),
		}
	}

	out := &Outcome{}
	seeds := make([]string, len(matches))
	anchors := make([]Anchor, 0, len(matches))
	for i, m := range matches {
		seeds[i] = m.ID
		if n, ok := g.Node(m.ID); ok {
			anchors = append(anchors, Anchor{
				ID: m.ID, Title: n.Title, Type: n.Type(),
				Match: int(m.Level), Deg: len(g.Neighbors(m.ID)),
			})
		}
	}

	out = clusterFromSeeds(g, p, seeds, anchors, opt)
	if len(out.Results) == 0 {
		// 锚点命中但邻居稀疏 → 全文 LIKE 兜底（保留结构过滤）。
		// v0.1.3 降级搜索词优化：Web 端"点击卡片继续漫游"用节点 ID 作查询词，
		// 纯数字全文搜必空——孤立节点点击会"没反应"。改用锚点 title 搜索，
		// 让孤立节点也能找到正文提到它的内容（fallback 不排除种子，自身可见）。
		searchQ := query
		if isNumeric(query) && len(matches) == 1 {
			if n, ok := g.Node(matches[0].ID); ok && n.Title != "" {
				searchQ = n.Title
			}
		}
		hits := g.TextSearch(searchQ, opt.Top*2)
		filtered := hits[:0]
		structural := map[string]bool{}
		for _, t := range p.StructuralTypes {
			structural[t] = true
		}
		for _, h := range hits {
			if opt.FilterStructural && structural[h.Type] {
				continue
			}
			filtered = append(filtered, h)
		}
		out.Fallback = ModeSparse
		out.FallbackHits = filtered
	}
	return out
}

// clusterFromSeeds 从种子集计算漫游簇：PPR + 激活扩散 → 排除（种子/目录枢纽/
// 结构类型）→ 归一化融合 + 跳数配额。查询锚定与随机漫步共用同一管线。
func clusterFromSeeds(g *graph.Graph, p *adapter.VaultProfile, seeds []string, anchors []Anchor, opt Options) *Outcome {
	out := &Outcome{Anchors: anchors}

	ppr := g.PPR(seeds, 0.15, 60)
	acts := g.Activate(seeds, opt.Lambda, opt.Theta, opt.Hops)
	actMap := make(map[string]graph.ActivationResult, len(acts))
	for _, r := range acts {
		actMap[r.ID] = r
	}

	// 排除：种子 + 目录枢纽（度 ≥ 半数节点）+ 结构类型
	exclude := map[string]bool{}
	for _, s := range seeds {
		exclude[s] = true
	}
	st := g.Stats()
	hubThresh := st.Nodes / 2
	for id, ns := range g.NeighborsOfAll() {
		if len(ns) >= hubThresh {
			exclude[id] = true
		}
	}
	if opt.FilterStructural {
		structural := map[string]bool{}
		for _, t := range p.StructuralTypes {
			structural[t] = true
		}
		for id := range actMap {
			if n, ok := g.Node(id); ok && structural[n.Type()] {
				exclude[id] = true
			}
		}
	}

	out.Results = score.Rank(g, actMap, ppr, score.RankOpts{
		Alpha: opt.Alpha, Beta: opt.Beta, Gamma: 0, Delta: 0,
		TopN:     opt.Top,
		Exclude:  exclude,
		HopQuota: [3]float64{0.5, 0.3, 0.2},
	})
	return out
}

// Roll 随机漫步的 roll 参数（v0.1.7）。
type Roll struct {
	Rng   *rand.Rand // 随机源；固定种子 = 可复现（同一节点同一簇，可分享）
	Alpha float64    // 度加权指数：0=均匀（最大惊喜），1=度加权（偏丰富簇）；默认 0.5
	Avoid []string   // 最近已展示的节点 ID（防连续漫步撞车，服务端 ring）
}

// ComputeRandom 随机漫步：随机 roll 一个候选节点作为起点，然后走与查询漫游
// 完全相同的簇管线（clusterFromSeeds）。与恐龙工具箱 SRS 的块级 BFS roam
// 区别：簇按相关性排序（PPR+激活，不是引用展开顺序），且起点是 roll 出来的。
// 候选池为空（图太小或全被排除）时返回空 Outcome，调用方提示用户。
func ComputeRandom(g *graph.Graph, p *adapter.VaultProfile, opt Options, roll Roll) *Outcome {
	alpha := roll.Alpha
	if alpha < 0 || alpha > 1 {
		alpha = 0.5
	}
	id := rollSeed(g, p, roll.Rng, roll.Avoid, alpha)
	if id == "" {
		return &Outcome{}
	}
	n, _ := g.Node(id)
	anchor := Anchor{ID: id, Title: n.Title, Type: n.Type(),
		Match: 5, Deg: len(g.Neighbors(id)), Random: true}
	return clusterFromSeeds(g, p, []string{id}, []Anchor{anchor}, opt)
}

// rollSeed 从候选池加权采样一个随机起点。
// 候选池过滤（与簇输出同一口径，避免"滚出没法展示/没意义的节点"）：
//   - 结构类型（目录/机器节点，簇输出同样排除）
//   - 空标题（Orca 大量块无标题，没法当卡片展示）
//   - 孤立（度=0：簇必空，滚出来只有一张空卡片）
//   - 目录枢纽（度 ≥ 半数节点：不作为起点——枢纽会以簇成员身份自然出现在结果里，
//     反复拿枢纽当起点只会腻）
//   - avoid（最近已展示，防重复）
//
// 采样权重 = deg^alpha：alpha=0 均匀（最大惊喜），1 度加权（偏丰富簇），
// 默认 0.5 折中"惊喜度"与"首屏丰富度"。
func rollSeed(g *graph.Graph, p *adapter.VaultProfile, rng *rand.Rand, avoid []string, alpha float64) string {
	if rng == nil {
		return ""
	}
	structural := map[string]bool{}
	for _, t := range p.StructuralTypes {
		structural[t] = true
	}
	avoidSet := map[string]bool{}
	for _, id := range avoid {
		avoidSet[id] = true
	}
	hubThresh := g.Stats().Nodes / 2

	type cand struct {
		id string
		w  float64
	}
	var cands []cand
	total := 0.0
	for _, n := range g.Nodes() {
		if n.Title == "" || structural[n.Type()] || avoidSet[n.ID] {
			continue
		}
		deg := len(g.Neighbors(n.ID))
		if deg == 0 || deg >= hubThresh {
			continue
		}
		w := math.Pow(float64(deg), alpha)
		cands = append(cands, cand{n.ID, w})
		total += w
	}
	if len(cands) == 0 {
		return ""
	}
	r := rng.Float64() * total
	for _, c := range cands {
		r -= c.w
		if r <= 0 {
			return c.id
		}
	}
	return cands[len(cands)-1].id
}

// isNumeric 判断查询词是否为纯数字（虎鲸块 ID 形态；Web 点击卡片漫游的查询词）。
func isNumeric(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
