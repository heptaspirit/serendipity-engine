package graph

import "sort"

// ---- 查询锚定 PPR（修订 #1：结构分 = 查询锚定的 Personalized PageRank）----

// PPR 计算以 seeds 为种子的 Personalized PageRank 平稳分布。
// teleport: 跳回种子的概率（默认 0.15）；iters: 幂迭代次数。
func (g *Graph) PPR(seeds []string, teleport float64, iters int) map[string]float64 {
	n := len(g.nodes)
	if n == 0 {
		return nil
	}
	// 种子分布
	seed := make(map[string]float64, len(seeds))
	for _, s := range seeds {
		seed[s] += 1.0 / float64(len(seeds))
	}
	p := make(map[string]float64, n)
	for id := range g.nodes {
		p[id] = 1.0 / float64(n)
	}
	for it := 0; it < iters; it++ {
		next := make(map[string]float64, n)
		for id, ns := range g.adj {
			deg := float64(len(ns))
			if deg == 0 {
				// 悬空节点：把概率按种子分布回退（等价于均匀回跳的简化）
				for s, w := range seed {
					next[s] += (1-teleport) * p[id] * w
				}
				continue
			}
			share := (1 - teleport) * p[id] / deg
			for _, nb := range ns {
				next[nb] += share
			}
		}
		for id := range g.nodes {
			next[id] += teleport * seed[id]
		}
		p = next
	}
	return p
}

// ---- 激活扩散（§3.1：λ 衰减 + θ 剪枝 + maxHops）----

// ActivationResult 是激活扩散的输出：分数 + 可解释路径。
type ActivationResult struct {
	ID    string
	Score float64 // λ^hops（边权 v1 全 1.0）
	Hops  int
	Path  []string // seed → ... → 该节点（首达 = 最短）
}

// Activate 从 seeds 做 BFS 激活扩散。
// lambda: 每跳衰减；theta: 低于 θ 剪枝（不收录也不再传播）；maxHops: 最大跳数。
func (g *Graph) Activate(seeds []string, lambda, theta float64, maxHops int) []ActivationResult {
	best := map[string]*ActivationResult{}
	type item struct {
		id   string
		act  float64
		path []string
	}
	queue := []item{}
	for _, s := range seeds {
		if _, ok := g.nodes[s]; !ok {
			continue
		}
		queue = append(queue, item{id: s, act: 1.0, path: []string{s}})
	}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		// 首达即最短路径；但更远的路径激活更低，无需再比较
		if _, seen := best[cur.id]; !seen {
			best[cur.id] = &ActivationResult{
				ID: cur.id, Score: cur.act, Hops: len(cur.path) - 1, Path: cur.path,
			}
		}
		if cur.act < theta || len(cur.path)-1 >= maxHops {
			continue // 剪枝：不传播
		}
		for _, nb := range g.adj[cur.id] {
			if _, seen := best[nb]; seen {
				continue // 已收录（首达已最短），不重复
			}
			nextAct := cur.act * lambda
			if nextAct < theta {
				continue
			}
			np := make([]string, len(cur.path)+1)
			copy(np, cur.path)
			np[len(cur.path)] = nb
			queue = append(queue, item{id: nb, act: nextAct, path: np})
		}
	}
	out := make([]ActivationResult, 0, len(best))
	for _, r := range best {
		out = append(out, *r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	return out
}
