package graph

// ============================================================================
// 文件：internal/graph/relation.go
// 模块：两节点关系查询（v0.1.5，为 /api/relation 与未来 MCP 暴露铺路）
//
// ▍语义（与漫游引擎同源，white-box）
//   - Hops / Path：BFS 最短路径（无向图）。Direct = 1 跳直达。
//   - PPRFromTo / PPRToFrom：以一端为种子的 Personalized PageRank 得分
//     （非对称——"A 链接 B"与"B 链接 A"的结构地位不同）；多路径隐式累加、
//     每跳按 (1-teleport)/deg 衰减，正是漫游引擎的结构分同源信号。
//   - Affinity：二者算术平均 = 对称关联强度（"这两个节点关联多强"）。
//   - Activation：激活扩散到另一端的值 = λ^hops（λ=0.7 时 1 跳 0.7、
//     2 跳 0.49）；不可达为 -1。
//   - Evidence：最短路径每条边的来源文档标题（white-box：不只给数字，
//     给证据链——"为什么说它们相关"）。
//
// ▍与多路径语义的关系（用户确认的设计）
//   PPR 天然聚合所有路径（无需枚举路径）；激活只认最短路径（可解释的
//   走法）。本查询直接复用这两套信号，不再引入第三套度量。
// ============================================================================

import (
	"math"
	"slices"
	"sort"
)

// pprRelationParams 关系查询的 PPR 参数（与 roam 管线同源：teleport 0.15, 60 迭代）。
const (
	pprRelationTeleport = 0.15
	pprRelationIters    = 60
	relationLambda      = 0.7 // 激活衰减（漫游默认同值；全调用点恒 0.7，不再做参数）
)

// NodeInfo 关系查询中的端点信息。
type NodeInfo struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Type  string `json:"type"`
}

// EdgeEvidence 最短路径上一条边的证据：哪些文档实际包含这条链接（无向）。
type EdgeEvidence struct {
	A         string   `json:"a"`
	B         string   `json:"b"`
	Witnesses []string `json:"witnesses"` // 文档标题（去重，最多 3 个）
}

// Relation 两节点间关系查询结果。
type Relation struct {
	From       NodeInfo       `json:"from"`
	To         NodeInfo       `json:"to"`
	Direct     bool           `json:"direct"`      // 是否有直达边
	Hops       int            `json:"hops"`        // 最短路径长度；-1 = 不可达
	Path       []string       `json:"path"`        // 最短路径节点（含两端）
	Affinity   float64        `json:"affinity"`    // 对称关联强度（PPR 双向算术平均）
	PPRFromTo  float64        `json:"ppr_from_to"` // 非对称：以 from 为种子的 PPR
	PPRToFrom  float64        `json:"ppr_to_from"` // 非对称：以 to 为种子的 PPR
	Activation float64        `json:"activation"`  // 激活扩散值 λ^hops；不可达 -1
	Evidence   []EdgeEvidence `json:"evidence"`
}

// ComputeRelation 计算 from 与 to 的关系。任一节点不存在返回 nil。
func (g *Graph) ComputeRelation(from, to string) *Relation {
	fromNode, ok1 := g.Node(from)
	toNode, ok2 := g.Node(to)
	if !ok1 || !ok2 {
		return nil
	}
	rel := &Relation{
		From: NodeInfo{ID: fromNode.ID, Title: fromNode.Title, Type: fromNode.Type()},
		To:   NodeInfo{ID: toNode.ID, Title: toNode.Title, Type: toNode.Type()},
		Hops: -1,
	}
	if from == to {
		rel.Direct = true
		rel.Hops = 0
		rel.Path = []string{from}
		rel.Activation = 1.0
		rel.PPRFromTo = 1.0 / float64(len(g.nodes))
		rel.PPRToFrom = rel.PPRFromTo
		rel.Affinity = rel.PPRFromTo
		return rel
	}
	if path := g.shortestPath(from, to); path != nil {
		rel.Hops = len(path) - 1
		rel.Path = path
		rel.Direct = rel.Hops == 1
		rel.Activation = math.Pow(relationLambda, float64(rel.Hops))
		rel.Evidence = g.edgeEvidence(path)
	} else {
		rel.Activation = -1
	}
	// PPR 双向（与 roam 同源参数）；PPR 聚合多路径，无需枚举路径
	p1 := g.PPR([]string{from}, pprRelationTeleport, pprRelationIters)
	p2 := g.PPR([]string{to}, pprRelationTeleport, pprRelationIters)
	rel.PPRFromTo = p1[to]
	rel.PPRToFrom = p2[from]
	rel.Affinity = (rel.PPRFromTo + rel.PPRToFrom) / 2
	return rel
}

// shortestPath BFS 求 from→to 最短路径（无向图）；不可达返回 nil。
func (g *Graph) shortestPath(from, to string) []string {
	if from == to {
		return []string{from}
	}
	prev := map[string]string{}
	queue := []string{from}
	visited := map[string]bool{from: true}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, nb := range g.adj[cur] {
			if visited[nb] {
				continue
			}
			visited[nb] = true
			prev[nb] = cur
			if nb == to {
				// 回溯 to→…→from，再反转
				var rev []string
				for x := to; ; x = prev[x] {
					rev = append(rev, x)
					if x == from {
						break
					}
				}
				slices.Reverse(rev)
				return rev
			}
			queue = append(queue, nb)
		}
	}
	return nil
}

// edgeEvidence 最短路径每条边的来源文档（white-box 证据链）。
func (g *Graph) edgeEvidence(path []string) []EdgeEvidence {
	if len(path) < 2 {
		return nil
	}
	out := make([]EdgeEvidence, 0, len(path)-1)
	for i := 0; i+1 < len(path); i++ {
		a, b := path[i], path[i+1]
		out = append(out, EdgeEvidence{A: a, B: b, Witnesses: g.witnesses(a, b)})
	}
	return out
}

// witnesses 返回包含无向边 (u,v) 的文档标题：u 的 Refs 含 v，或 v 的 Refs 含 u
// （重定向后的 Refs——与图一致）。去重、排序、最多 3 个。
func (g *Graph) witnesses(u, v string) []string {
	seen := map[string]bool{}
	var out []string
	add := func(id, target string) {
		if n, ok := g.Node(id); ok && n.Doc != nil {
			for _, ref := range n.Doc.Refs {
				if ref == target {
					if !seen[n.Title] {
						seen[n.Title] = true
						out = append(out, n.Title)
					}
					break
				}
			}
		}
	}
	add(u, v)
	add(v, u)
	sort.Strings(out)
	if len(out) > 3 {
		out = out[:3]
	}
	return out
}
