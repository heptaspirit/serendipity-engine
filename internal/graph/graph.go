// Package graph 实现图引擎（设计 §6.8：内存邻接表）。
// v1 边语义：默认无向关联（修订 #2）；有向信息仅统计保留。
package graph

import (
	"sort"
	"strings"

	"serendipity-engine/internal/adapter"
)

type Node struct {
	ID    string
	Title string
	Doc   *adapter.Document
}

// Edge 是去重后的无向边（v1 关联语义）。
type Edge struct {
	A, B string
}

type Graph struct {
	nodes       map[string]*Node
	adj         map[string][]string // 无向邻接（去重）
	dangling    map[string]int      // 解析到但文件不存在的链接目标 → 计数
	totalLinks  int                 // 全部 [[链接]] 数（含重复/悬空/自环）
	selfLinks   int
	multiedge   int // 已见面对之间的重复链接数
}

func Build(docs []*adapter.Document) *Graph {
	g := &Graph{
		nodes:    make(map[string]*Node, len(docs)),
		adj:      make(map[string][]string),
		dangling: map[string]int{},
	}
	for _, d := range docs {
		g.nodes[d.ID] = &Node{ID: d.ID, Title: d.Title, Doc: d}
	}
	seen := map[string]bool{} // "a\x00b" 去重
	for _, d := range docs {
		for _, ref := range d.Refs {
			g.totalLinks++
			if ref == d.ID {
				g.selfLinks++
				continue // 自环
			}
			if _, ok := g.nodes[ref]; !ok {
				g.dangling[ref]++
				continue
			}
			key := pairKey(d.ID, ref)
			if seen[key] {
				g.multiedge++
				continue
			}
			seen[key] = true
			g.adj[d.ID] = append(g.adj[d.ID], ref)
			g.adj[ref] = append(g.adj[ref], d.ID)
		}
	}
	return g
}

func pairKey(a, b string) string {
	if a < b {
		return a + "\x00" + b
	}
	return b + "\x00" + a
}

func (g *Graph) Nodes() []*Node {
	out := make([]*Node, 0, len(g.nodes))
	for _, n := range g.nodes {
		out = append(out, n)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (g *Graph) Node(id string) (*Node, bool) {
	n, ok := g.nodes[id]
	return n, ok
}

func (g *Graph) Neighbors(id string) []string {
	return g.adj[id]
}

// NeighborsOfAll 返回完整无向邻接表（供目录枢纽检测等用）。
func (g *Graph) NeighborsOfAll() map[string][]string {
	return g.adj
}

// Stats 图统计：规模 / 链接账目 / 孤儿 / 连通分量 / 枢纽。
type Stats struct {
	Nodes         int
	Edges         int // 去重无向边
	TotalLinks    int
	SelfLinks     int
	MultiEdge     int // 已见面对之间的重复链接数
	Dangling      int // 悬空目标种数
	DanglingLinks int // 悬空链接总数
	Orphans       int
	Components    int
	TopHubs       []Hub
}

type Hub struct {
	ID    string
	Title string
	Type  string
	Deg   int
}

func (g *Graph) Stats() Stats {
	s := Stats{
		Nodes:         len(g.nodes),
		TotalLinks:    g.totalLinks,
		SelfLinks:     g.selfLinks,
		MultiEdge:     g.multiedge,
		Dangling:      len(g.dangling),
		DanglingLinks: 0,
	}
	totalAdj := 0
	for _, ns := range g.adj {
		totalAdj += len(ns)
	}
	s.Edges = totalAdj / 2
	for _, c := range g.dangling {
		s.DanglingLinks += c
	}
	// 孤儿：无任何边
	for id := range g.nodes {
		if len(g.adj[id]) == 0 {
			s.Orphans++
		}
	}
	// 连通分量（并查集）
	parent := map[string]string{}
	var find func(string) string
	find = func(x string) string {
		if parent[x] == "" || parent[x] == x {
			parent[x] = x
			return x
		}
		parent[x] = find(parent[x])
		return parent[x]
	}
	for id := range g.nodes {
		find(id)
	}
	for id, ns := range g.adj {
		for _, n := range ns {
			if find(id) != find(n) {
				parent[find(id)] = find(n)
			}
		}
	}
	comp := map[string]bool{}
	for id := range g.nodes {
		comp[find(id)] = true
	}
	s.Components = len(comp)
	// 枢纽
	var hubs []Hub
	for id, ns := range g.adj {
		hubs = append(hubs, Hub{ID: id, Deg: len(ns)})
	}
	sort.Slice(hubs, func(i, j int) bool { return hubs[i].Deg > hubs[j].Deg })
	if len(hubs) > 10 {
		hubs = hubs[:10]
	}
	for i := range hubs {
		if n, ok := g.nodes[hubs[i].ID]; ok {
			hubs[i].Title = n.Title
			hubs[i].Type = n.Type()
		}
	}
	s.TopHubs = hubs
	return s
}

// MatchLevel 锚点匹配级别（越高越"重"，用于展示筛选）。
type MatchLevel int

const (
	MatchLike  MatchLevel = iota + 1 // 子串命中（ID/title 包含）
	MatchTag                          // 标签精确
	MatchAlias                        // 别名精确
	MatchTitle                        // title 精确
	MatchExact                        // ID 精确
)

// Match 锚点命中项。
type Match struct {
	ID    string
	Level MatchLevel
}

// Resolve 锚定查询词：返回全部命中（起点是集合），带匹配级别。
// 所有命中都参与激活扩散；级别只用于展示筛选（"只显示重的几个锚点"）。
func (g *Graph) Resolve(q string) []Match {
	q = strings.TrimSpace(q)
	var out []Match
	for id, n := range g.nodes {
		switch {
		case id == q:
			out = append(out, Match{ID: id, Level: MatchExact})
		case n.Title == q:
			out = append(out, Match{ID: id, Level: MatchTitle})
		case contains(n.Aliases(), q):
			out = append(out, Match{ID: id, Level: MatchAlias})
		case contains(n.Doc.Tags, q):
			out = append(out, Match{ID: id, Level: MatchTag})
		case strings.Contains(id, q) || strings.Contains(n.Title, q):
			out = append(out, Match{ID: id, Level: MatchLike})
		}
	}
	// 按级别降序（同级别顺序不保证——展示层再按度排序）
	sort.SliceStable(out, func(i, j int) bool { return out[i].Level > out[j].Level })
	return out
}

func contains(ss []string, q string) bool {
	for _, s := range ss {
		if s == q {
			return true
		}
	}
	return false
}

// TextHit 全文 LIKE 命中（降级策略输出，决策 #10）。
type TextHit struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Type  string `json:"type"`
	Count int    `json:"count"` // 命中次数
}

// TextSearch 全文 LIKE 兜底：正文包含查询词（大小写不敏感）的节点，按命中次数排序。
func (g *Graph) TextSearch(q string, limit int) []TextHit {
	q = strings.ToLower(q)
	var hits []TextHit
	for id, n := range g.nodes {
		if n.Doc == nil {
			continue
		}
		t := strings.ToLower(n.Doc.Text)
		if c := strings.Count(t, q); c > 0 {
			hits = append(hits, TextHit{ID: id, Title: n.Title, Type: n.Type(), Count: c})
		}
	}
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].Count != hits[j].Count {
			return hits[i].Count > hits[j].Count
		}
		return hits[i].ID < hits[j].ID
	})
	if len(hits) > limit {
		hits = hits[:limit]
	}
	return hits
}

func (n *Node) Aliases() []string {
	if n.Doc == nil {
		return nil
	}
	return n.Doc.Aliases
}

// Type 返回节点类型（画像推断，用于结构节点过滤等）。
func (n *Node) Type() string {
	if n.Doc == nil {
		return ""
	}
	return n.Doc.Type
}
