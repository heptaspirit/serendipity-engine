// ============================================================================
// 文件：internal/graph/node.go
// 模块：单节点详情（v0.1.11，roadmap M1 · graph.node / 前端 #3）
//
// ▍语义（backlog §六 graph.node + frontend #3，一次实现两端受益）
//   AI（MCP graph.node）漫游到节点后需要"确认这是不是我要的"——现在缺这个
//   基础动作；Web（/api/node + 卡片「预览」）同源。两级（OpenViking L0/L1
//   借鉴，见 agent-memory-research §4.2）：
//     L0 = Text 摘要截断（发现层不读全文，正文由宿主负责——克制红线）；
//     L1 = 邻居导航（链接到谁 + 谁链接我，可点击继续漫游）。
//
// ▍与 roam/similar 的关系
//   - 邻居（Neighbors）= 无向邻接（与 roam 扩散同图）；
//   - 被引用（Backlinks）= 有向入边（谁链接我，Obsidian/虎鲸语义）。
//   - 不参与打分，纯详情展示——"确认"动作，不是"推荐"动作。
//
// ▍修改记录
//   v0.1.11 初版（roadmap M1 #2）。
// ============================================================================
package graph

import (
	"sort"
)

// textSummaryMax 详情里正文摘要的最大字符数（发现层不读全文，克制边界；
// 正文由 Obsidian/虎鲸宿主负责，见 docs/history/product-form.md）。
const textSummaryMax = 200

// NodeRef 邻居/被引用节点的展示信息。
type NodeRef struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Type  string `json:"type"`
}

// NodeDetail 单节点详情（L0 摘要 + L1 邻居导航）。
type NodeDetail struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Type      string    `json:"type"`
	Aliases   []string  `json:"aliases,omitempty"`
	Tags      []string  `json:"tags,omitempty"`
	Text      string    `json:"text"` // 摘要截断（L0）
	Deg       int       `json:"deg"`
	Neighbors []NodeRef `json:"neighbors"` // 无向邻接：链接到谁
	Backlinks []NodeRef `json:"backlinks"` // 有向入边：谁链接我
}

// NodeDetail 返回 id 的详情；节点不存在返回 nil。
// 邻居排序：按 Title 稳定（确定性，v0.1.7 同款）；被引用取引用方按 Title 排序。
func (g *Graph) NodeDetail(id string) *NodeDetail {
	n, ok := g.nodes[id]
	if !ok {
		return nil
	}
	d := &NodeDetail{
		ID: n.ID, Title: n.Title, Type: n.Type(),
		Deg: len(g.adj[id]),
	}
	if n.Doc != nil {
		d.Aliases = append([]string(nil), n.Doc.Aliases...)
		d.Tags = append([]string(nil), n.Doc.Tags...)
		d.Text = truncateRunes(n.Doc.Text, textSummaryMax)
	}
	// 邻居（无向邻接）
	seen := map[string]bool{}
	for _, nb := range g.adj[id] {
		if seen[nb] {
			continue
		}
		seen[nb] = true
		if nn, ok2 := g.nodes[nb]; ok2 {
			d.Neighbors = append(d.Neighbors, NodeRef{ID: nn.ID, Title: nn.Title, Type: nn.Type()})
		}
	}
	sort.Slice(d.Neighbors, func(i, j int) bool { return d.Neighbors[i].Title < d.Neighbors[j].Title })
	// 被引用（有向入边：谁的 Refs 含 id）
	bl := map[string]bool{}
	for _, cand := range g.nodes {
		if cand.Doc == nil {
			continue
		}
		for _, ref := range cand.Doc.Refs {
			if ref == id {
				if !bl[cand.ID] {
					bl[cand.ID] = true
					d.Backlinks = append(d.Backlinks, NodeRef{ID: cand.ID, Title: cand.Title, Type: cand.Type()})
				}
				break
			}
		}
	}
	sort.Slice(d.Backlinks, func(i, j int) bool { return d.Backlinks[i].Title < d.Backlinks[j].Title })
	return d
}

// truncateRunes 按 rune 截断（中文安全），超长加省略号。
func truncateRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}
