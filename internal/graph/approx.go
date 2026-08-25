// ============================================================================
// 文件：internal/graph/approx.go
// 模块：潜在关联候选（kind=approx 近似边，roadmap #15，backlog §3.6）
//
// ▍定位（克制 + AI 补位，2026-08-25 重新定调）
//
//	取代原「mentions API（虚拟引用 / AC 文本扫描）」方案（已否决：暴力枚举成倍
//	加边、中文子串边界无解、虎鲸无模糊提及通道）。新方案 = 引擎从拓扑多算法
//	估算"近似相关"的候选对，产物是**有界、明确标注、带算法溯源**的待审清单
//	（suggest-links），不是落图的边。AI 生态位（语义研判）留给插件层：
//	插件 AI 取本清单 + 笔记正文做判定，接受者升级为 kind=ai 边回流引擎
//	（见 plugin-ai-cooperation.md Flow 1）。
//
// ▍算法（§3.6 #1–#4，复用 #12 similar 底座）
//
//	1. 候选生成有界：只对每节点 2-hop 邻域打分（图稀疏 O(N·d²)），不枚举全图。
//	2. 拓扑指数（取公共邻居，三算法同一输入）：
//	    AA  Adamic-Adar    Σ_{w∈∩} 1/log(deg(w))   —— 度加权（低度节点专属关联强）
//	    Jaccard |∩|/|∪|                            —— 比例（抗大分母？不，见 #12 教训）
//	    RA  Resource-Alloc Σ_{w∈∩} 1/deg(w)         —— 度倒数（更强枢纽惩罚）
//	3. Borda 聚合：三算法各自排序 → 名次分加总（Borda count，抗量纲差异）。
//	4. top-K 节流（防爆核心）：每节点取 K 个候选（K=2~3），全局上限 K×N——
//	    与图密度无关，区别于暴力文本扫描的"每匹配都成边"。
//
// ▍与 similar 的区别（§3.6.0 命名澄清）
//
//	similar（#12）= 查询时实时算、**不落图**、带证据的 ranked 列表（"X 跟谁像"）；
//	本模块 = 候选 pass、**可被消费的待审清单**（"这些对看似相关，你来判"）。
//	两者共用 Adamic-Adar 底座（commonNeighbors），一个是查看器、一个是原料清单。
//
// ▍co-touch 行为信号（§3.6 #3 关键澄清）
//
//	"同窗口先后打开"的共现只有插件 L1 知道（adapter 是静态摄入翻译器，
//	Document 无访问时间）；引擎 touch 表当前只有 target/src 点击记录，无共现
//	数据。故引擎层本模块**只做拓扑**；co-touch 信号留待 M2 插件经 touch 通道
//	喂入后再并入聚合（结构已在 Evidence 预留思路，见 ApproxEdge）。
//
// ▍修改记录
//
//	v0.1.13 初版（roadmap 阶段 1 #15）。
//
// ============================================================================
package graph

import (
	"math"
	"sort"
)

// ApproxEdge 一条潜在关联候选（待审清单条目；kind=approx 的低权边形态，
// 未落图——插件 AI 判定接受后才由插件层写回 kind=ai 边）。
type ApproxEdge struct {
	A          string   `json:"a"`          // 端点（A < B，无向对规范化）
	B          string   `json:"b"`          // 端点
	Score      float64  `json:"score"`      // Borda 聚合分（名次分之和，越高越相关）
	Algorithms []string `json:"algorithms"` // 命中的算法名（aa / jaccard / ra）
	Shared     []string `json:"shared"`     // 共同邻居 ID（证据："都链接了 X/Y"）
}

// PotentialLinks 计算潜在关联候选（待审清单）。
// perNodeK：每节点保留的候选数（backlog K=2~3；默认 2）。structuralTypes：
// 结构类型名集合（与 roam/Similar 同口径，目录/机器节点不做候选端点）。
// 返回全局去重（无向对）后按 Borda 分降序的清单（并列按对稳定）。
// 排除口径复用 Similar（#12）：自身/直接邻居/空标题/结构类型/孤立/目录枢纽。
func (g *Graph) PotentialLinks(perNodeK int, structuralTypes map[string]bool) []ApproxEdge {
	if perNodeK <= 0 {
		perNodeK = 2
	}
	hubThresh := g.Stats().Nodes / 2

	// 每节点的 2-hop 候选 → 三算法分 → Borda 名次分 → top-K
	all := map[string]ApproxEdge{} // "a\x00b" → 候选（全局去重）
	for id, n := range g.nodes {
		if n.Title == "" || structuralTypes[n.Type()] {
			continue
		}
		deg := len(g.adj[id])
		if deg == 0 || deg >= hubThresh {
			continue
		}
		cands := g.approxCandidates2Hop(id, structuralTypes, hubThresh)
		if len(cands) == 0 {
			continue
		}
		// Borda：三算法各自排序取名次分
		ranked := bordaRank(cands)
		// top-K 节流：该节点保留前 K 个
		if len(ranked) > perNodeK {
			ranked = ranked[:perNodeK]
		}
		for _, r := range ranked {
			a, b := id, r.id
			if a > b {
				a, b = b, a
			}
			key := a + "\x00" + b
			if _, dup := all[key]; dup {
				continue
			}
			all[key] = ApproxEdge{A: a, B: b, Score: r.borda, Algorithms: r.algs, Shared: r.shared}
		}
	}

	out := make([]ApproxEdge, 0, len(all))
	for _, e := range all {
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		if out[i].A != out[j].A {
			return out[i].A < out[j].A
		}
		return out[i].B < out[j].B
	})
	return out
}

// approxCand 一条 2-hop 候选（三算法分 + 证据）。
type approxCand struct {
	id     string
	aa     float64 // Adamic-Adar
	jc     float64 // Jaccard
	ra     float64 // Resource-Allocation
	shared []string
	borda  float64 // Borda 名次分（排序后回填）
	algs   []string
}

// approxCandidates2Hop 计算 id 的 2-hop 邻域候选（排除自身/直接邻居/结构/孤立/枢纽）。
// 图稀疏时 O(Σ deg(邻居))——有界，不枚举全图（§3.6 #1）。
func (g *Graph) approxCandidates2Hop(id string, structuralTypes map[string]bool, hubThresh int) []approxCand {
	nbSet := map[string]bool{}
	for _, nb := range g.adj[id] {
		nbSet[nb] = true
	}
	if len(nbSet) == 0 {
		return nil
	}
	// 2-hop：邻居的邻居（∪ 去重）作为候选池
	candSet := map[string]bool{}
	for nb := range nbSet {
		for _, nn := range g.adj[nb] {
			if nn == id || nbSet[nn] {
				continue // 自身 + 直接邻居（已链接 = 相关，非潜在）
			}
			candSet[nn] = true
		}
	}
	var out []approxCand
	for cid := range candSet {
		cn, ok := g.nodes[cid]
		if !ok {
			continue
		}
		if cn.Title == "" || structuralTypes[cn.Type()] {
			continue
		}
		cdeg := len(g.adj[cid])
		if cdeg == 0 || cdeg >= hubThresh {
			continue
		}
		// 公共邻居（∩）+ 三指数（同一输入）
		var shared []string
		aa, ra := 0.0, 0.0
		for _, nb := range g.adj[cid] {
			if !nbSet[nb] {
				continue
			}
			shared = append(shared, nb)
			d := math.Max(2, float64(len(g.adj[nb])))
			aa += 1.0 / math.Log(d)
			ra += 1.0 / d
		}
		if len(shared) == 0 {
			continue
		}
		sort.Strings(shared)
		union := len(nbSet) + cdeg - len(shared)
		jc := float64(len(shared)) / float64(union)
		out = append(out, approxCand{id: cid, aa: aa, jc: jc, ra: ra, shared: shared})
	}
	return out
}

// bordaRank 对候选做 Borda 聚合：AA/Jaccard/RA 三算法各自降序排名，
// 名次分 = 候选数 - 名次（第 1 名得 n-1 分，末名 0），加总回填 cand.borda。
// 各算法只记录"命中"（得分 > 0 才参与排名——RA/AA 恒正，Jaccard 恒正，
// 有公共邻居即全命中；此处防御性判断）。
// 返回按 borda 降序（并列按 id 稳定）的候选切片。
func bordaRank(cands []approxCand) []approxCand {
	n := len(cands)
	// 每算法排名 → 名次分
	scoreOf := func(get func(*approxCand) float64) map[string]float64 {
		sorted := append([]approxCand(nil), cands...)
		sort.Slice(sorted, func(i, j int) bool {
			if get(&sorted[i]) != get(&sorted[j]) {
				return get(&sorted[i]) > get(&sorted[j])
			}
			return sorted[i].id < sorted[j].id
		})
		m := map[string]float64{}
		for i, c := range sorted {
			m[c.id] = float64(n - 1 - i)
		}
		return m
	}
	aaM := scoreOf(func(c *approxCand) float64 { return c.aa })
	jcM := scoreOf(func(c *approxCand) float64 { return c.jc })
	raM := scoreOf(func(c *approxCand) float64 { return c.ra })
	for i := range cands {
		c := &cands[i]
		// 基础分 = 命中算法数（三算法全命中 = 更强信号），+ Borda 名次分。
		// 基础分解决 Borda 的固有问题：候选池只有 1 个时名次分恒 0，
		// 无法体现"有候选且全算法命中"；跨节点全局排序时也避免
		// 候选池大小不同导致的名次分不可比。
		c.algs = []string{"aa", "jaccard", "ra"} // 三算法全参与（拓扑 pass，无行为信号）
		c.borda = float64(len(c.algs)) + aaM[c.id] + jcM[c.id] + raM[c.id]
	}
	sort.Slice(cands, func(i, j int) bool {
		if cands[i].borda != cands[j].borda {
			return cands[i].borda > cands[j].borda
		}
		return cands[i].id < cands[j].id
	})
	return cands
}
