// ============================================================================
// 文件：internal/web/server.go
// 模块：Web 入口（设计 §6.8 三入口之 Web）—— REST / JSON + 节点簇可视化页面
//
// ▍职责
//
//	把内存图（graph.Graph）暴露成 REST 接口并托管可视化页面：
//	  GET  /api/stats    规模统计（节点/边/版本）
//	  GET  /api/hot      热门节点（初始页漂浮气泡池，跳过结构类型与目录枢纽）
//	  GET  /api/roam?q=  查询漫游（与 CLI 同一 roam.Compute 管线）
//	  GET  /api/relation?from=&to=  两节点关系查询（v0.1.5：最短路径+双向
//	       PPR 强度+证据链，white-box；from/to 接受 ID 或标题）
//	  POST /api/refresh  对账刷新（v0.1.2，见下）
//	  GET  /             嵌入的可视化页面（go:embed static/index.html）
//
// ▍对账刷新（POST /api/refresh，v0.1.2）
//
//	使用者增删改笔记后，浏览器点「刷新」→ 服务端调用 main 注入的 RefreshFunc
//	（重解析 → sync.Diff 与上次持久化状态比对 → 写回存储）→ 返回 diff 摘要
//	（新增/更新/删除计数 + 明细，limit 截断防大库刷爆响应）。
//	刷新成功后内存图整体替换：hot/stats/roam 立即反映新图。
//	RWMutex 保护 G：读接口（stats/hot/roam）持 RLock，刷新替换持 Lock，
//	本地单用户并发安全。
//
// ▍跳转（v0.1.4 虎鲸跳转落地）
//
//	Obsidian 源且已知 vault 名 → 卡片「打开」生成 obsidian://open URI（去 .md，
//	URL 编码）；虎鲸源（OrcaRepo 非空）→ 生成 orca-note://<repo>/block?blockId=<id>
//	（协议见 orcanote-agent-skills / orcanote-markdown skill）。
//
// ▍反馈埋点（POST /api/touch，v0.1.4）
//
//	Web 端点击节点时上报 {target, from}，写入 store 的 touch 表（独立存储、
//	容量上限，见 internal/store）。克制设计：仅记录不演化边权，杜绝
//	"点击→边权变→结果变→再点击"的正反馈跑飞；写失败静默不影响主流程。
//
// ▍自动监听集成（v0.1.4）
//
//	watch 包轮询检测到变化 → 调用 main 注入的 refresh 闭包 → ReplaceGraph
//	替换内存图并递增 revision；前端轮询 /api/stats 对比 revision 提示"库已更新"。
//
// ▍修改记录
//
//	v0.1.0  初版 REST + 页面。
//	v0.1.1  页面/画像改动无涉本文件。
//	v0.1.2  POST /api/refresh 对账刷新；RWMutex 图保护；刷新闭包注入。
//	v0.1.3  / 响应 Cache-Control: no-store（防浏览器缓存旧页面）。
//	v0.1.4  虎鲸跳转 orca-note://；POST /api/touch；ReplaceGraph + revision。
//	v0.1.5  GET /api/relation 关系查询（最短路径+双向 PPR+证据链）；
//	        /api/refresh 响应补充 renamed/renames（改名迁移，修订 #8）。
//
// ============================================================================
package web

import (
	"embed"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"

	"serendipity-engine/internal/adapter"
	"serendipity-engine/internal/graph"
	"serendipity-engine/internal/roam"
	"serendipity-engine/internal/score"
	syncpkg "serendipity-engine/internal/sync"
)

//go:embed static/index.html
var staticFS embed.FS

// RefreshFunc 对账刷新闭包（由 main 构造）：重解析 → diff → 写回存储，
// 返回 diff 结果与刷新后的新图（供内存替换）。
type RefreshFunc func() (*syncpkg.Result, *graph.Graph, error)

// TouchFunc 反馈埋点闭包（由 main 构造，写 store 的 touch 表）。
type TouchFunc func(target, from string) error

// Server 持有图与画像，提供 REST 接口。
type Server struct {
	mu        sync.RWMutex // 保护 G 与 revision（刷新替换图，读接口并发安全）
	G         *graph.Graph
	P         *adapter.VaultProfile
	Source    string
	VaultName string // Obsidian vault 名（obsidian:// URI 跳转用）；空 = 不提供跳转
	OrcaRepo  string // 虎鲸 repo 名（orca-note:// URI 跳转用）；空 = 不提供跳转
	Version   string
	Refresh   RefreshFunc // 非空时注册 POST /api/refresh
	Touch     TouchFunc   // 非空时注册 POST /api/touch（反馈埋点）
	revision  int         // 图版本号：每次刷新 +1，前端轮询 stats 对比以提示"库已更新"
}

// New 创建 Web 服务；refresh/touch 为 nil 时不注册对应端点。
func New(g *graph.Graph, p *adapter.VaultProfile, source, vaultName, version string, refresh RefreshFunc, touch TouchFunc) *Server {
	return &Server{G: g, P: p, Source: source, VaultName: vaultName, Version: version, Refresh: refresh, Touch: touch}
}

// ReplaceGraph 用新图整体替换内存图并递增 revision（手动 /api/refresh 与
// 自动监听触发共用）。调用方须持有新图所有权。
func (s *Server) ReplaceGraph(g *graph.Graph) {
	s.mu.Lock()
	s.G = g
	s.revision++
	s.mu.Unlock()
}

// Revision 当前图版本号（/api/stats 返回，前端轮询提示"库已更新"）。
func (s *Server) Revision() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.revision
}

// Handler 返回路由。
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/stats", s.handleStats)
	mux.HandleFunc("/api/hot", s.handleHot)
	mux.HandleFunc("/api/roam", s.handleRoam)
	mux.HandleFunc("/api/relation", s.handleRelation)
	if s.Refresh != nil {
		mux.HandleFunc("/api/refresh", s.handleRefresh)
	}
	if s.Touch != nil {
		mux.HandleFunc("/api/touch", s.handleTouch)
	}
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		// 禁用缓存：前端迭代频繁，避免浏览器缓存旧页面导致"点了没反应"
		w.Header().Set("Cache-Control", "no-store")
		b, _ := staticFS.ReadFile("static/index.html")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(b)
	})
	return mux
}

// handleTouch POST /api/touch：反馈埋点（点击节点 = touch）。
// 克制设计：仅记录，不演化边权；写失败静默（埋点不影响主流程）。
func (s *Server) handleTouch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, map[string]string{"error": "method not allowed"})
		return
	}
	var body struct {
		Target string `json:"target"`
		From   string `json:"from"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Target == "" {
		writeJSON(w, map[string]string{"ok": "false"})
		return
	}
	if err := s.Touch(body.Target, body.From); err != nil {
		// 埋点失败不影响主流程（克制：静默）
		writeJSON(w, map[string]string{"ok": "false"})
		return
	}
	writeJSON(w, map[string]string{"ok": "true"})
}

// refreshResp 对账刷新 REST 响应（明细截断到 limit，防大库刷爆响应）。
type refreshResp struct {
	Added      int              `json:"added"`
	Updated    int              `json:"updated"`
	Deleted    int              `json:"deleted"`
	Renamed    int              `json:"renamed"` // v0.1.5：改名迁移（修订 #8）
	Unchanged  int              `json:"unchanged"`
	DurationMS int64            `json:"duration_ms"`
	Nodes      int              `json:"nodes"`
	Changes    []syncpkg.Change `json:"changes"`
	Renames    []syncpkg.Rename `json:"renames"`
}

// handleRefresh POST /api/refresh：调用刷新闭包 → 替换内存图 → 返回 diff 摘要。
func (s *Server) handleRefresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, map[string]string{"error": "method not allowed"})
		return
	}
	limit := atoiDefault(r.URL.Query().Get("limit"), 50)
	res, g, err := s.Refresh()
	if err != nil {
		writeJSON(w, map[string]string{"error": err.Error()})
		return
	}
	s.ReplaceGraph(g)

	changes := res.Changes
	if len(changes) > limit {
		changes = changes[:limit]
	}
	renames := res.Renames
	if len(renames) > limit {
		renames = renames[:limit]
	}
	writeJSON(w, refreshResp{
		Added: res.Added, Updated: res.Updated, Deleted: res.Deleted,
		Renamed: res.Renamed, Unchanged: res.Unchanged, DurationMS: res.DurationMS,
		Nodes: g.Stats().Nodes, Changes: changes, Renames: renames,
	})
}

// handleRelation GET /api/relation?from=&to=：两节点关系查询（v0.1.5）。
// from/to 接受节点 ID 或标题（经 Resolve 锚定，取首个命中）。输出 white-box
// 关系：最短路径 + 双向 PPR 强度（对称 affinity）+ 激活值 + 证据链。
// 为未来 MCP 暴露（graph.relation）铺路——AI 可据此判断两实体关联强度。
func (s *Server) handleRelation(w http.ResponseWriter, r *http.Request) {
	fromQ := r.URL.Query().Get("from")
	toQ := r.URL.Query().Get("to")
	if fromQ == "" || toQ == "" {
		writeJSON(w, map[string]string{"error": "from/to required"})
		return
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	from := s.resolveID(fromQ)
	to := s.resolveID(toQ)
	if from == "" || to == "" {
		writeJSON(w, map[string]string{"error": "node not found"})
		return
	}
	rel := s.G.ComputeRelation(from, to, 0.7)
	if rel == nil {
		writeJSON(w, map[string]string{"error": "node not found"})
		return
	}
	writeJSON(w, rel)
}

// resolveID 把查询词解析为节点 ID：精确 ID > title > 别名 > 标签 > LIKE
// （Resolve 按级别降序返回，取首个）。
func (s *Server) resolveID(q string) string {
	ms := s.G.Resolve(q)
	if len(ms) == 0 {
		return ""
	}
	return ms[0].ID
}

type statsResp struct {
	Nodes    int    `json:"nodes"`
	Edges    int    `json:"edges"`
	Version  string `json:"version"`
	Revision int    `json:"revision"` // 图版本号：自动/手动刷新后 +1（前端轮询提示更新）
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	st := s.G.Stats()
	rev := s.revision
	s.mu.RUnlock()
	writeJSON(w, statsResp{Nodes: st.Nodes, Edges: st.Edges, Version: s.Version, Revision: rev})
}

// hotNode 初始页漂浮气泡节点。
type hotNode struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Type  string `json:"type"`
	Deg   int    `json:"deg"`
}

// handleHot 返回热门节点（按图度降序，跳过结构类型与目录枢纽）。
// 初始页用它生成漂浮气泡池，前端随机采样展示。
func (s *Server) handleHot(w http.ResponseWriter, r *http.Request) {
	n := atoiDefault(r.URL.Query().Get("n"), 20)
	structural := map[string]bool{}
	for _, t := range s.P.StructuralTypes {
		structural[t] = true
	}
	s.mu.RLock()
	hubThresh := s.G.Stats().Nodes / 2

	type hub struct {
		id  string
		deg int
	}
	var hubs []hub
	for id, ns := range s.G.NeighborsOfAll() {
		hubs = append(hubs, hub{id, len(ns)})
	}
	sort.Slice(hubs, func(i, j int) bool { return hubs[i].deg > hubs[j].deg })

	out := make([]hotNode, 0, n)
	for _, h := range hubs {
		if len(out) >= n {
			break
		}
		if h.deg >= hubThresh {
			continue // 目录枢纽（index 类）
		}
		node, ok := s.G.Node(h.id)
		if !ok || structural[node.Type()] {
			continue
		}
		out = append(out, hotNode{ID: node.ID, Title: node.Title, Type: node.Type(), Deg: h.deg})
	}
	s.mu.RUnlock()
	writeJSON(w, out)
}

// roamResp REST 响应。
type roamResp struct {
	Query        string        `json:"query"`
	Source       string        `json:"source"`
	Vault        string        `json:"vault"`
	Anchors      []roam.Anchor `json:"anchors"`
	Results      []roamItem    `json:"results"`
	Fallback     int           `json:"fallback"`
	FallbackHits []roamItem    `json:"fallback_hits"`
}

// roamItem 带跳转 URI 的节点项。
type roamItem struct {
	ID    string   `json:"id"`
	Title string   `json:"title"`
	Type  string   `json:"type"`
	Score float64  `json:"score,omitempty"`
	PPR   float64  `json:"ppr,omitempty"`
	Act   float64  `json:"act,omitempty"`
	Hops  int      `json:"hops,omitempty"`
	Path  []string `json:"path,omitempty"`
	Count int      `json:"count,omitempty"` // 全文命中次数（降级）
	URI   string   `json:"uri,omitempty"`   // obsidian:// 或 orca-note:// 跳转（对应源）
}

// obsidianURI 拼 obsidian://open?vault=<名>&file=<相对路径>（去 .md，URL 编码）。
func (s *Server) obsidianURI(path string) string {
	if s.VaultName == "" || path == "" {
		return ""
	}
	file := strings.TrimSuffix(path, ".md")
	return "obsidian://open?vault=" + url.QueryEscape(s.VaultName) + "&file=" + url.QueryEscape(file)
}

// orcaURI 拼 orca-note://<repo>/block?blockId=<id>（虎鲸跳转，v0.1.4；
// 协议见 orcanote-agent-skills 的 orcanote-markdown skill：块链接用块 ID）。
func (s *Server) orcaURI(id string) string {
	if s.OrcaRepo == "" || id == "" {
		return ""
	}
	return "orca-note://" + url.QueryEscape(s.OrcaRepo) + "/block?blockId=" + url.QueryEscape(id)
}

// uriFor 按源类型生成跳转 URI：虎鲸（OrcaRepo 非空）用块 ID；否则 Obsidian 路径。
func (s *Server) uriFor(path, id string) string {
	if s.OrcaRepo != "" {
		return s.orcaURI(id)
	}
	return s.obsidianURI(path)
}

func (s *Server) toItems(results []score.Result) []roamItem {
	out := make([]roamItem, 0, len(results))
	for _, r := range results {
		out = append(out, roamItem{
			ID: r.ID, Title: r.Title, Type: r.Type,
			Score: r.Score, PPR: r.PPR, Act: r.Act, Hops: r.Hops, Path: r.Path,
			URI: s.uriFor(nodePath(s.G, r.ID), r.ID),
		})
	}
	return out
}

func (s *Server) toHitItems(hits []graph.TextHit) []roamItem {
	out := make([]roamItem, 0, len(hits))
	for _, h := range hits {
		out = append(out, roamItem{
			ID: h.ID, Title: h.Title, Type: h.Type, Count: h.Count,
			URI: s.uriFor(nodePath(s.G, h.ID), h.ID),
		})
	}
	return out
}

// nodePath 取节点相对路径（跳转用）。
func nodePath(g *graph.Graph, id string) string {
	if n, ok := g.Node(id); ok && n.Doc != nil {
		return n.Doc.Path
	}
	return ""
}

func (s *Server) handleRoam(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	top := atoiDefault(r.URL.Query().Get("top"), 15)
	s.mu.RLock()
	out := roam.Compute(s.G, s.P, q, roam.Options{
		Top: top, Hops: 3, Lambda: 0.7, Theta: 0.1,
		Alpha: 0.5, Beta: 0.5, FilterStructural: true,
	})
	resp := roamResp{
		Query: q, Source: s.Source, Vault: s.VaultName,
		Anchors: out.Anchors, Fallback: int(out.Fallback),
	}
	if out.Fallback == 0 {
		resp.Results = s.toItems(out.Results)
	} else {
		resp.FallbackHits = s.toHitItems(out.FallbackHits)
	}
	s.mu.RUnlock()
	writeJSON(w, resp)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		fmt.Fprintln(w, `{"error":"encode"}`)
	}
}

func atoiDefault(s string, def int) int {
	if n, err := strconv.Atoi(s); err == nil && n > 0 {
		return n
	}
	return def
}
