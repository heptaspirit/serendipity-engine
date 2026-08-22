// Package roam 封装查询漫游管线（CLI 与 Web 共用）：
// 锚定 → PPR + 激活扩散 → 排除（种子/目录枢纽/结构类型）→ 归一化融合 + 跳数配额 → 降级兜底。
//
// ▍降级搜索词（v0.1.3）
//   Web 端"点击卡片继续漫游"用节点 ID（虎鲸为纯数字块 ID）作查询词；锚定后若
//   邻居稀疏走 ModeSparse 全文兜底时，纯数字全文搜必空（孤立节点点击"没反应"）。
//   修复：查询词为纯数字且唯一锚定时，降级搜索改用锚点 title。
package roam

import (
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
	ID    string `json:"id"`
	Title string `json:"title"`
	Type  string `json:"type"`
	Match int    `json:"match"` // 匹配级别 1-5（like/tag/alias/title/exact），展示筛选用
	Deg   int    `json:"deg"`   // 图度，同级别展示排序的次级权重
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
	for i, m := range matches {
		seeds[i] = m.ID
		if n, ok := g.Node(m.ID); ok {
			out.Anchors = append(out.Anchors, Anchor{
				ID: m.ID, Title: n.Title, Type: n.Type(),
				Match: int(m.Level), Deg: len(g.Neighbors(m.ID)),
			})
		}
	}

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
