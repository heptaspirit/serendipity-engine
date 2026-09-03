// ============================================================================
// 文件：internal/graph/similar.go
// 模块：结构相似节点（v0.1.11 初版 Jaccard；v0.1.12 升级 Adamic-Adar）
//
// ▍概念（backlog §3.1，embedding 语义轴的纯结构替代）
//
//	"共同邻居多但互不链接"的节点对——两篇笔记都关联同一批人物但彼此无链接
//	→ 大概率主题相近。白盒、零依赖、证据可解释（"因为都链接了 人物B/C"）。
//
// ▍评分：Jaccard → Adamic-Adar（v0.1.12，roadmap 阶段 1 #12，用户实测驱动）
//
//	实测反馈（2026-08-24）：Jaccard 比例错位（惩罚大分母——一边接 20 个纯数字
//	hub、一边接 2 个真概念的节点，会被大分母拖低）+ 共享邻居不加权。Adamic-Adar
//	（graphwizard similarity 包，agent-memory-research 附录 E.2#1 学习参考）修正
//	两点：① 只数共同邻居，不看总度（比例退化为交集计数）② 共同邻居按度倒数加权
//	`Σ 1/log(deg)`——度越小越能证明"两者专属关联"（高介数 hub 对所有节点都常见，
//	区分度低）。~20 行改动；证据/排除/排序逻辑全复用，风险低。
//
// ▍与 roam 的语义区分（红线 1：绝不并入 roam 管线）
//   - roam  = 相关（锚点激活扩散，PPR+Act 融合）：从锚点"发散出去"的东西；
//   - similar = 相似（共同邻居结构）：与锚点"说同一件事"但没直接链接的东西。
//     入口独立（/api/similar + MCP graph.similar），输出带共享邻居证据。
//
// ▍度偏置防御（复用 rollSeed 排除口径）
//
//	枢纽（deg ≥ 半数节点）天然像所有人；空标题/孤立节点无法展示。排除：
//	自身 / 直接邻居（已链接 = "相关"而非"相似"）/ 目录枢纽 / 结构类型 /
//	空标题 / 孤立。阈值：得分 > 0（至少 1 个共享邻居）且非直接邻居。
//
// ▍复杂度
//
//	局部按需计算 O(邻居²)，不预计算全图（数千节点库毫秒级）。
//
// ▍修改记录
//
//	v0.1.11 初版（roadmap M1 #1，Jaccard）。
//	v0.1.12 评分升级 Jaccard → Adamic-Adar（roadmap 阶段 1 #12）。
//
// ============================================================================
package graph

import (
	"math"
	"sort"
)

// SimilarResult 一个结构相似候选（带共享邻居证据，white-box）。
type SimilarResult struct {
	ID     string   `json:"id"`
	Title  string   `json:"title"`
	Type   string   `json:"type"`
	Score  float64  `json:"score"`  // Adamic-Adar 相似度 Σ_{w∈N(u)∩N(v)} 1/log(deg(w))
	Shared []string `json:"shared"` // 共同邻居 ID（证据："都链接了 X/Y"）
}

// commonNeighbors 计算候选节点 cid 与目标邻居集 nbSet 的共同邻居（证据）及同一
// shared 集合的 Adamic-Adar（Σ1/log(deg)）/ Resource-Allocation（Σ1/deg）加权和。
// similar（查看器，只要 AA 分）与 approx（候选清单，AA+RA+Jaccard）共用的底座。
// shared 排序后返回（输出确定性）。nb 同时邻接目标与候选（两者互异）故 deg ≥ 2，
// log/deg 无除零；度越大权重越低（高介数节点对所有都常见，区分度低）。
func (g *Graph) commonNeighbors(cid string, nbSet map[string]bool) (shared []string, aa, ra float64) {
	for _, nb := range g.adj[cid] {
		if !nbSet[nb] {
			continue
		}
		shared = append(shared, nb)
		d := float64(max(2, len(g.adj[nb])))
		aa += 1.0 / math.Log(d)
		ra += 1.0 / d
	}
	sort.Strings(shared)
	return shared, aa, ra
}

// Similar 返回与 id 结构相似的节点（Adamic-Adar，top k，降序）。
// structuralTypes：结构类型名集合（画像 StructuralTypes，调用方传入——
// 与 roam 同口径，目录/机器节点不参与）。
// 排除：自身、直接邻居（互链 = 相关不是相似）、目录枢纽（deg ≥ 半数）、
// 结构类型、空标题、孤立（无邻居，AA 恒 0）。
// 得分 > 0（至少 1 个共享邻居）才进入候选；并列按 ID 稳定排序（v0.1.7 同款）。
func (g *Graph) Similar(id string, k int, structuralTypes map[string]bool) []SimilarResult {
	if k <= 0 {
		k = 10
	}
	if _, ok := g.nodes[id]; !ok {
		return nil
	}
	// 目标自身的邻居集合
	nbSet := map[string]bool{}
	for _, nb := range g.adj[id] {
		nbSet[nb] = true
	}
	if len(nbSet) == 0 {
		return nil // 孤立节点：无共同邻居可言
	}
	hubThresh := g.Stats().Nodes / 2

	type cand struct {
		id     string
		shared []string
		score  float64
	}
	var cands []cand
	for cid, cn := range g.nodes {
		if cid == id || nbSet[cid] {
			continue // 自身 + 直接邻居
		}
		if cn.Title == "" {
			continue
		}
		if structuralTypes[cn.Type()] {
			continue
		}
		deg := len(g.adj[cid])
		if deg == 0 || deg >= hubThresh {
			continue
		}
		// 共同邻居（证据）+ Adamic-Adar 度加权（v0.1.12）——approx 共用同底座
		shared, score, _ := g.commonNeighbors(cid, nbSet)
		if len(shared) == 0 || score == 0 {
			continue
		}
		cands = append(cands, cand{cid, shared, score})
	}
	sort.Slice(cands, func(i, j int) bool {
		if cands[i].score != cands[j].score {
			return cands[i].score > cands[j].score
		}
		return cands[i].id < cands[j].id
	})
	if len(cands) > k {
		cands = cands[:k]
	}
	out := make([]SimilarResult, 0, len(cands))
	for _, c := range cands {
		cn := g.nodes[c.id]
		out = append(out, SimilarResult{
			ID: cn.ID, Title: cn.Title, Type: cn.Type(),
			Score: c.score, Shared: c.shared,
		})
	}
	return out
}
