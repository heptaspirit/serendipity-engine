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
// ▍跳转
//
//	Obsidian 源且已知 vault 名 → 卡片「打开」生成 obsidian://open URI（去 .md，
//	URL 编码）；虎鲸源（Path 前缀 block/）无 URI 跳转（v1 未做，见 product-form.md）。
//
// ▍修改记录
//
//	v0.1.0  初版 REST + 页面。
//	v0.1.1  页面/画像改动无涉本文件。
//	v0.1.2  POST /api/refresh 对账刷新；RWMutex 图保护；刷新闭包注入。
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

// Server 持有图与画像，提供 REST 接口。
type Server struct {
	mu        sync.RWMutex // 保护 G（/api/refresh 替换图，读接口并发安全）
	G         *graph.Graph
	P         *adapter.VaultProfile
	Source    string
	VaultName string // Obsidian vault 名（obsidian:// URI 跳转用）；空 = 不提供跳转
	Version   string
	Refresh   RefreshFunc // 非空时注册 POST /api/refresh
}

// New 创建 Web 服务；refresh 为 nil 时不注册刷新端点。
func New(g *graph.Graph, p *adapter.VaultProfile, source, vaultName, version string, refresh RefreshFunc) *Server {
	return &Server{G: g, P: p, Source: source, VaultName: vaultName, Version: version, Refresh: refresh}
}

// Handler 返回路由。
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/stats", s.handleStats)
	mux.HandleFunc("/api/hot", s.handleHot)
	mux.HandleFunc("/api/roam", s.handleRoam)
	if s.Refresh != nil {
		mux.HandleFunc("/api/refresh", s.handleRefresh)
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

// refreshResp 对账刷新 REST 响应（明细截断到 limit，防大库刷爆响应）。
type refreshResp struct {
	Added      int              `json:"added"`
	Updated    int              `json:"updated"`
	Deleted    int              `json:"deleted"`
	Unchanged  int              `json:"unchanged"`
	DurationMS int64            `json:"duration_ms"`
	Nodes      int              `json:"nodes"`
	Changes    []syncpkg.Change `json:"changes"`
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
	s.mu.Lock()
	s.G = g
	s.mu.Unlock()

	changes := res.Changes
	if len(changes) > limit {
		changes = changes[:limit]
	}
	writeJSON(w, refreshResp{
		Added: res.Added, Updated: res.Updated, Deleted: res.Deleted,
		Unchanged: res.Unchanged, DurationMS: res.DurationMS,
		Nodes: g.Stats().Nodes, Changes: changes,
	})
}

type statsResp struct {
	Nodes   int    `json:"nodes"`
	Edges   int    `json:"edges"`
	Version string `json:"version"`
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	st := s.G.Stats()
	s.mu.RUnlock()
	writeJSON(w, statsResp{Nodes: st.Nodes, Edges: st.Edges, Version: s.Version})
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
	URI   string   `json:"uri,omitempty"`   // obsidian:// 跳转（仅 Obsidian 源且有 VaultName）
}

// obsidianURI 拼 obsidian://open?vault=<名>&file=<相对路径>（去 .md，URL 编码）。
func (s *Server) obsidianURI(path string) string {
	if s.VaultName == "" || path == "" {
		return ""
	}
	file := strings.TrimSuffix(path, ".md")
	return "obsidian://open?vault=" + url.QueryEscape(s.VaultName) + "&file=" + url.QueryEscape(file)
}

func (s *Server) toItems(results []score.Result) []roamItem {
	out := make([]roamItem, 0, len(results))
	for _, r := range results {
		out = append(out, roamItem{
			ID: r.ID, Title: r.Title, Type: r.Type,
			Score: r.Score, PPR: r.PPR, Act: r.Act, Hops: r.Hops, Path: r.Path,
			URI: s.obsidianURI(nodePath(s.G, r.ID)),
		})
	}
	return out
}

func (s *Server) toHitItems(hits []graph.TextHit) []roamItem {
	out := make([]roamItem, 0, len(hits))
	for _, h := range hits {
		out = append(out, roamItem{
			ID: h.ID, Title: h.Title, Type: h.Type, Count: h.Count,
			URI: s.obsidianURI(nodePath(s.G, h.ID)),
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
