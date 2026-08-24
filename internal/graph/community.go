// ============================================================================
// 文件：internal/graph/community.go
// 模块：社区发现（Leiden，v0.1.12，roadmap 阶段 1 #10 · 诊断层）
//
// ▍定位（backlog §3.4 + agent-memory-research 附录 D.4）
//
//	"激活层"之外第二种 agent 价值：诊断层——不用遍历全库就能定位「有哪些主题簇、
//	哪个区域互不相连、哪些是知识缺口」。算法 = Leiden（Louvain 官方改进版，保证
//	well-connected 社区，支持分辨率参数 γ 缓解模块度分辨率极限）。
//
// ▍选型（已定 2026-08-24）：github.com/vsuryav/leiden-go（MIT、string 节点直通、
//
//	零新增依赖树、自带 Modularity 质量分）。本文件只是适配层（~80 行），符合
//	§一.5 开发纪律（算法/模块 = 包级可复用函数，为 MCP graph.community 直供）。
//	go.sum 以 pseudo-version 锁定版本（MIT 保留 LICENSE，README 标一行 attribution）。
//
// ▍孤立节点处理（已拍板 2026-08-24）
//
//	度 = 0 的节点在社区检测前过滤——不把孤立节点当社区：① 社区相似性需要社区间
//	结构信号（inter-community 边 / PPR 激活 / 邻居集合），孤立节点三者全失效；
//	② 其诊断信号已由 Stats().Orphans 承接（职责分离：Orphans 统计管孤立，社区
//	检测只管有内部结构的簇）。因此 Membership 不含孤立节点。
//
// ▍合规（MIT）：go.sum 锁版本（LICENSE 由 go.sum 记录）；README 标一行 attribution。
//
// ▍修改记录
//
//	v0.1.12 初版（roadmap 阶段 1 #10）。
//
// ============================================================================
package graph

import (
	"sort"
	"time"

	"github.com/vsuryav/leiden-go"
)

// Community 一个社区（簇）。
type Community struct {
	ID     int      `json:"id"`
	Size   int      `json:"size"`
	Nodes  []string `json:"nodes"`  // 全部节点 ID
	Titles []string `json:"titles"` // 代表标题（按度降序取前 8，展示用）
}

// CommunityResult 社区发现结果。
type CommunityResult struct {
	Modularity     float64        `json:"modularity"`      // Leiden 自带模块度质量分（-1~1，越高社区越清晰）
	CommunityCount int            `json:"community_count"` // 社区数
	Membership     map[string]int `json:"membership"`      // nodeID → 社区 ID（不含孤立节点）
	Communities    []Community    `json:"communities"`     // 按 Size 降序的社区列表
}

// Communities 用 Leiden 做无向无权社区检测（v1 边权全 1.0）。
// resolution：分辨率参数（默认 1.0，越大社区越碎）；seed：随机种子（默认时间，
// 固定值可复现同一划分）。无边/无节点图 → 空结果（不报错）。
// 返回 Membership（node→comm）+ 社区列表（Size 降序，并列按 ID 稳定）。
func (g *Graph) Communities(resolution float64, seed int64) (*CommunityResult, error) {
	if resolution <= 0 {
		resolution = 1.0
	}
	if seed == 0 {
		seed = time.Now().UnixNano()
	}
	// 无权无向 → leiden 加权图（边权全 1.0）；过滤孤立节点（度=0）
	wadj := make(map[string]map[string]float64, len(g.adj))
	for id, nbs := range g.adj {
		if len(nbs) == 0 {
			continue
		}
		m := make(map[string]float64, len(nbs))
		for _, nb := range nbs {
			m[nb] = 1.0
		}
		wadj[id] = m
	}
	if len(wadj) == 0 {
		return &CommunityResult{}, nil
	}
	// 用 DefaultConfig 作底（MaxIterations/MinModularityGain 有默认值），只覆盖
	// resolution + seed。注意：不能只用 &Config{Resolution, RandomSeed}——那会让
	// MaxIterations=0、MinModularityGain 为 0，Leiden 循环不跑、全部节点各成一簇。
	cfg := leiden.DefaultConfig()
	cfg.Resolution = resolution
	cfg.RandomSeed = seed
	res, err := leiden.Leiden(leiden.NewGraph(wadj), cfg)
	if err != nil {
		return nil, err
	}
	membership := res.Partition.Membership()
	commNodes := res.Partition.Communities()

	// 构建社区列表：按 Size 降序，并列按 ID 稳定；Titles = 按度降序取前 8
	type sizeRank struct {
		id   int
		size int
	}
	var ranks []sizeRank
	for id := range commNodes {
		ranks = append(ranks, sizeRank{id, len(commNodes[id])})
	}
	sort.Slice(ranks, func(i, j int) bool {
		if ranks[i].size != ranks[j].size {
			return ranks[i].size > ranks[j].size
		}
		return ranks[i].id < ranks[j].id
	})
	comms := make([]Community, 0, len(ranks))
	for _, rk := range ranks {
		nodes := commNodes[rk.id]
		sort.Strings(nodes)
		titles := g.topTitles(nodes, 8)
		comms = append(comms, Community{ID: rk.id, Size: rk.size, Nodes: nodes, Titles: titles})
	}
	return &CommunityResult{
		Modularity:     res.Modularity,
		CommunityCount: res.CommunityCount,
		Membership:     membership,
		Communities:    comms,
	}, nil
}

// topTitles 取 nodes 中度最高的前 n 个标题（代表节点，展示用）。
func (g *Graph) topTitles(nodes []string, n int) []string {
	type t struct {
		id  string
		deg int
	}
	var ss []t
	for _, id := range nodes {
		if _, ok := g.nodes[id]; ok {
			ss = append(ss, t{id, len(g.adj[id])})
		}
	}
	sort.Slice(ss, func(i, j int) bool {
		if ss[i].deg != ss[j].deg {
			return ss[i].deg > ss[j].deg
		}
		return ss[i].id < ss[j].id
	})
	if len(ss) > n {
		ss = ss[:n]
	}
	out := make([]string, 0, len(ss))
	for _, s := range ss {
		if nd := g.nodes[s.id]; nd != nil && nd.Title != "" {
			out = append(out, nd.Title)
		} else {
			out = append(out, s.id)
		}
	}
	return out
}
