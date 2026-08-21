// Package web 实现三入口之 Web（设计 §6.8）：REST / JSON + 节点簇可视化页面。
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

	"serendipity-engine/internal/adapter"
	"serendipity-engine/internal/graph"
	"serendipity-engine/internal/roam"
	"serendipity-engine/internal/score"
)

//go:embed static/index.html
var staticFS embed.FS

// Server 持有图与画像，提供 REST 接口。
type Server struct {
	G         *graph.Graph
	P         *adapter.VaultProfile
	Source    string
	VaultName string // Obsidian vault 名（obsidian:// URI 跳转用）；空 = 不提供跳转
	Version   string
}

// New 创建 Web 服务。
func New(g *graph.Graph, p *adapter.VaultProfile, source, vaultName, version string) *Server {
	return &Server{G: g, P: p, Source: source, VaultName: vaultName, Version: version}
}

// Handler 返回路由。
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/stats", s.handleStats)
	mux.HandleFunc("/api/hot", s.handleHot)
	mux.HandleFunc("/api/roam", s.handleRoam)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		b, _ := staticFS.ReadFile("static/index.html")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(b)
	})
	return mux
}

type statsResp struct {
	Nodes   int    `json:"nodes"`
	Edges   int    `json:"edges"`
	Version string `json:"version"`
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	st := s.G.Stats()
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
