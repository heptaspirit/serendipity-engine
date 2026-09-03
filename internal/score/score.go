// Package score 实现四维打分与归一化融合（设计 §3.2 + 修订 #3/#4/#13）。
// 融合权重：α=β=0.5 默认（γ/δ 维度及公式项已于 2026-09 ponytail C/D 删除——
// 恒为 0，无 heat/依赖分混入导航排名）；每维先归一化。
// spike 迭代：新增 种子/目录节点排除 + 跳数配额混合（serendipity 机制，参数实测来源已归档）。
// v0.1.6：归一化改为**桶内**（按跳数分桶各自 min-max）——配额机制下候选是
// "桶内排序、跨桶配额轮转"，全局归一化会让深跳桶整体趋 0（score=0 误导）；
// min-max 单调不改桶内排序，输出序列不变。
package score

import (
	"math"
	"sort"

	"serendipity-engine/internal/graph"
)

// Result 是融合排序后的输出项（含可解释路径）。
type Result struct {
	ID    string   `json:"id"`
	Title string   `json:"title"`
	Type  string   `json:"type"`
	Score float64  `json:"score"`
	PPR   float64  `json:"ppr"`
	Act   float64  `json:"act"`
	Hops  int      `json:"hops"`
	Path  []string `json:"path"`
}

// RankOpts 融合排序参数。
type RankOpts struct {
	Alpha, Beta float64 // 结构分（PPR）与激活分权重（γ/δ 维度已于 2026-09 删除——恒为 0）
	TopN        int
	Exclude     map[string]bool // 种子 + 目录节点等（spike 发现）
	HopQuota    [3]float64      // 1/2/3-hop 配额比例（spike 发现：纯相关度排不出深跳）
}

// Rank 把激活候选与 PPR 合并，min-max 归一化后线性融合，再按跳数配额混合取 topN。
func Rank(g *graph.Graph, actMap map[string]graph.ActivationResult, pprMap map[string]float64, opts RankOpts) []Result {

	// 1. 归一化 + 融合（桶内归一化 v0.1.6）：先按跳数分桶，每桶独立 min-max。
	//    跳数配额机制下候选是"桶内排序、跨桶配额轮转"——跨桶分数本就无可比性；
	//    全局归一化会让远锚点桶（深跳）整体趋 0（score=0 误导，v0.1.5 定性验证
	//    发现：崖门 2 跳项 score=0）。min-max 单调，不改变桶内排序 → 输出序列
	//    不变，只让分数在桶内有区分度（serendipity 深跳节点不被"标零"）。
	type cand struct {
		id       string
		score    float64
		ppr, act float64
	}
	buckets := map[int][]cand{1: {}, 2: {}, 3: {}}
	for id, r := range actMap {
		if opts.Exclude != nil && opts.Exclude[id] {
			continue
		}
		h := clampHop(r.Hops)
		buckets[h] = append(buckets[h], cand{id: id, ppr: pprMap[id], act: r.Score})
	}
	for h := 1; h <= 3; h++ {
		actVals := make([]float64, 0, len(buckets[h]))
		pprVals := make([]float64, 0, len(buckets[h]))
		for _, c := range buckets[h] {
			actVals = append(actVals, c.act)
			pprVals = append(pprVals, c.ppr)
		}
		minAct, maxAct := minMax(actVals)
		minPPR, maxPPR := minMax(pprVals)
		for i := range buckets[h] {
			nAct := norm(buckets[h][i].act, minAct, maxAct)
			nPPR := norm(buckets[h][i].ppr, minPPR, maxPPR)
			buckets[h][i].score = opts.Alpha*nPPR + opts.Beta*nAct
		}
		// 并列分按 ID 稳定破序（v0.1.7）：Rank 入参来自 map 迭代，sort.Slice 不稳定
		// 会让并列项顺序随运行变化——同 seed 随机漫步"同一簇"就不成立。分数单调性
		// 不受影响（只改并列顺序），查询漫游输出也更稳定。
		sort.Slice(buckets[h], func(i, j int) bool {
			if buckets[h][i].score != buckets[h][j].score {
				return buckets[h][i].score > buckets[h][j].score
			}
			return buckets[h][i].id < buckets[h][j].id
		})
	}

	// 2. 跳数配额混合（serendipity）：hop 越深越少，但保证出现
	quota := opts.HopQuota
	if quota == [3]float64{} {
		quota = [3]float64{0.5, 0.3, 0.2}
	}

	need := opts.TopN
	plan := []int{int(math.Round(quota[0] * float64(need))),
		int(math.Round(quota[1] * float64(need))),
		int(math.Round(quota[2] * float64(need)))}
	// 配额超额时补回不足的桶
	for {
		sum := plan[0] + plan[1] + plan[2]
		if sum == need {
			break
		}
		if sum < need {
			// 找还有余量的桶补
			added := false
			for i := 0; i < 3; i++ {
				if plan[i] < len(buckets[i+1]) {
					plan[i]++
					added = true
					break
				}
			}
			if !added {
				break
			}
		} else {
			// 超额：从最深的桶减（深桶往往最缺候选）
			removed := false
			for i := 2; i >= 0; i-- {
				if plan[i] > 0 && plan[i] > len(buckets[i+1]) {
					plan[i]--
					removed = true
					break
				}
			}
			if !removed {
				// 全满则整体截断
				for i := 2; i >= 0; i-- {
					if plan[i] > 0 {
						plan[i]--
						break
					}
				}
			}
		}
	}

	var out []Result
	shown := map[int]int{}
	for i := 0; i < need; i++ {
		// 轮转取桶：先 1 后 2 后 3，各按配额
		picked := false
		for pass := 0; pass < 3 && !picked; pass++ {
			h := ((i + pass) % 3) + 1
			if shown[h] >= plan[h-1] {
				continue
			}
			if shown[h] >= len(buckets[h]) {
				continue
			}
			c := buckets[h][shown[h]]
			shown[h]++
			act := actMap[c.id]
			title := ""
			typ := ""
			if n, ok := g.Node(c.id); ok {
				title = n.Title
				typ = n.Type()
			}
			out = append(out, Result{
				ID: c.id, Title: title, Type: typ, Score: c.score,
				PPR: c.ppr, Act: c.act, Hops: act.Hops, Path: act.Path,
			})
			picked = true
		}
		if !picked {
			break
		}
	}
	return out
}

func minMax(vals []float64) (float64, float64) {
	if len(vals) == 0 {
		return 0, 1
	}
	mn, mx := vals[0], vals[0]
	for _, v := range vals {
		mn = math.Min(mn, v)
		mx = math.Max(mx, v)
	}
	return mn, mx
}

// norm min-max 归一化；单值域退化时返回 0.5（无区分度）。
func norm(v, mn, mx float64) float64 {
	if mx == mn {
		return 0.5
	}
	return (v - mn) / (mx - mn)
}

// clampHop 跳数归一到 1..3（分桶键：<1 视作 1 跳锚点、>3 并入 3 跳桶）。
func clampHop(h int) int {
	if h > 3 {
		return 3
	}
	if h < 1 {
		return 1
	}
	return h
}
