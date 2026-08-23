// ============================================================================
// 文件：internal/web/server.go
// 模块：Web 入口（设计 §6.8 三入口之 Web）—— REST / JSON + 节点簇可视化页面
//
// ▍职责
//
//	把内存图（graph.Graph）暴露成 REST 接口并托管可视化页面：
//	  GET  /api/stats    规模统计（节点/边/版本）
//	  GET  /api/hot      热门节点（初始页漂浮气泡池，跳过结构类型与目录枢纽）
//	  GET  /api/roam?q=  查询漫游（与 CLI 同一 roam.Compute 管线）；
//	       /api/roam?random=1 随机漫步（v0.1.7：roll 随机起点 + 簇，
//	       ?seed=N 可复现，?rand_alpha= 度加权指数，内置防重复 ring）
//	  GET  /api/relation?from=&to=  两节点关系查询（v0.1.5：最短路径+双向
//	       PPR 强度+证据链，white-box；from/to 接受 ID 或标题）
//	  GET  /api/config   前端可调参数白名单（top/hops/lambda/theta/alpha/beta
//	       + 范围约束；前端据此渲染设置抽屉，见 tuneParams）
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
//	v0.1.7  随机漫步 GET /api/roam?random=1（服务端 roll 随机起点 + 簇；
//	        ?seed=N 固定种子可复现、跳过防重复；?rand_alpha= 度加权指数；
//	        内置 32 个"最近起点"ring 防连续撞车）。
//	v0.1.8  安全前置（roadmap M0-0.1）：Handler 包 auth 中间件——Host 校验
//	        （仅回环地址）+ API token 鉴权（X-Seren-Token 头 / ?token=，
//	        常量时间比较）；页面注入 token（__SEREN_TOKEN__ 占位符）。
//
// ============================================================================
package web

import (
	"embed"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

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
	Token     string      // API 鉴权 token（v0.1.8 安全前置）；空 = 未配置鉴权
	revision  int         // 图版本号：每次刷新 +1，前端轮询 stats 对比以提示"库已更新"

	// 随机漫步状态（v0.1.7）：randMu 保护 rng 与 recent（rand.Rand 非并发安全）。
	randMu sync.Mutex
	rng    *rand.Rand // 时间种子随机源（?seed=N 时用独立固定源，不占此锁路径）
	recent []string   // 最近随机起点（防重复 ring，上限 32）
}

// New 创建 Web 服务；refresh/touch 为 nil 时不注册对应端点。
func New(g *graph.Graph, p *adapter.VaultProfile, source, vaultName, version string, refresh RefreshFunc, touch TouchFunc) *Server {
	return &Server{
		G: g, P: p, Source: source, VaultName: vaultName, Version: version,
		Refresh: refresh, Touch: touch,
		rng: rand.New(rand.NewPCG(uint64(time.Now().UnixNano()), 0x9E3779B97F4A7C15)),
	}
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

// Handler 返回路由（v0.1.8 安全前置：整体包 auth 中间件——Host 校验 + API
// token 鉴权，见 auth.go；页面响应注入 token 供前端 fetch 携带）。
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/stats", s.handleStats)
	mux.HandleFunc("/api/hot", s.handleHot)
	mux.HandleFunc("/api/roam", s.handleRoam)
	mux.HandleFunc("/api/relation", s.handleRelation)
	mux.HandleFunc("/api/config", s.handleConfig)
	if s.Refresh != nil {
		mux.HandleFunc("/api/refresh", s.handleRefresh)
	}
	if s.Touch != nil {
		mux.HandleFunc("/api/touch", s.handleTouch)
	}
	mux.HandleFunc("/", s.handleIndex)
	return s.auth(mux)
}

// handleIndex 返回嵌入页面；把 API token 注入 __SEREN_TOKEN__ 占位符
// （index.html 的 const __API_TOKEN__，fetch 包装自动携带）。
// 禁用缓存：前端迭代频繁，避免浏览器缓存旧页面导致"点了没反应"。
func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	b, _ := staticFS.ReadFile("static/index.html")
	out := strings.ReplaceAll(string(b), "__SEREN_TOKEN__", s.Token)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(out))
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

// relationNode 关系路径上单个节点的展示信息（web 层补充标题，供前端绘制可读路径）。
type relationNode struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// relationResp 在 graph.Relation 基础上补充 path_nodes（路径节点 ID→标题），
// 让前端不依赖图内部就能画出可读的最短路径链。
type relationResp struct {
	*graph.Relation
	PathNodes []relationNode `json:"path_nodes"`
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
	// 路径节点 ID→标题（white-box：路径可读可点击）
	pathNodes := make([]relationNode, 0, len(rel.Path))
	for _, id := range rel.Path {
		t := id
		if n, ok := s.G.Node(id); ok && n.Title != "" {
			t = n.Title
		}
		pathNodes = append(pathNodes, relationNode{ID: id, Title: t})
	}
	writeJSON(w, relationResp{Relation: rel, PathNodes: pathNodes})
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
	query := r.URL.Query().Get("q")
	vals := r.URL.Query()
	s.mu.RLock()
	opt := roam.Options{
		// 可调参数：前端可从 /api/config 取白名单，这里统一钳制到安全范围
		// （高阶内部项——跳数配额/PPR 阻尼/迭代次数——不对外暴露，保持默认）。
		Top:    clampInt(vals, "top", 15, 1, 60),
		Hops:   clampInt(vals, "hops", 3, 1, 5),
		Lambda: clampFloat(vals, "lambda", 0.7, 0, 1),
		Theta:  clampFloat(vals, "theta", 0.1, 0, 1),
		Alpha:  clampFloat(vals, "alpha", 0.5, 0, 1),
		Beta:   clampFloat(vals, "beta", 0.5, 0, 1),
		FilterStructural: true,
	}

	// 随机漫步（v0.1.7）：?random=1 时忽略 q，roll 随机起点 + 簇。
	if vals.Get("random") == "1" {
		out := s.computeRandom(vals, opt)
		writeJSON(w, s.roamRespOf(out, "random"))
		s.mu.RUnlock()
		return
	}

	out := roam.Compute(s.G, s.P, query, opt)
	writeJSON(w, s.roamRespOf(out, query))
	s.mu.RUnlock()
}

// computeRandom 执行一次随机漫步（?random=1，v0.1.7）。
// ?seed=N 固定种子 → 完全确定（同一 URL 每次同一节点同一簇，可分享），跳过防重复；
// 否则用服务端时间种子 rng + 最近起点 ring（防连续撞车，上限 32）。
func (s *Server) computeRandom(vals url.Values, opt roam.Options) *roam.Outcome {
	alpha := clampFloat(vals, "rand_alpha", 0.5, 0, 1)
	if n, err := strconv.ParseInt(vals.Get("seed"), 10, 64); err == nil {
		rng := rand.New(rand.NewPCG(uint64(n), uint64(n)>>1^0x9E3779B97F4A7C15))
		return roam.ComputeRandom(s.G, s.P, opt, roam.Roll{Rng: rng, Alpha: alpha})
	}
	s.randMu.Lock()
	out := roam.ComputeRandom(s.G, s.P, opt, roam.Roll{Rng: s.rng, Alpha: alpha, Avoid: s.recent})
	if len(out.Anchors) > 0 {
		s.recent = append(s.recent, out.Anchors[0].ID)
		if len(s.recent) > 32 {
			s.recent = s.recent[len(s.recent)-32:]
		}
	}
	s.randMu.Unlock()
	return out
}

// roamRespOf 把漫游结果包装成 REST 响应（查询与随机共用）。
func (s *Server) roamRespOf(out *roam.Outcome, query string) roamResp {
	resp := roamResp{
		Query: query, Source: s.Source, Vault: s.VaultName,
		Anchors: out.Anchors, Fallback: int(out.Fallback),
	}
	if out.Fallback == 0 {
		resp.Results = s.toItems(out.Results)
	} else {
		resp.FallbackHits = s.toHitItems(out.FallbackHits)
	}
	return resp
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

// clampInt 解析可选整型参数并钳制到 [min,max]；非法/缺省用 def。
// 前端可调参数的安全边界在服务端强制（白盒：即便恶意传值也翻不出范围）。
func clampInt(v url.Values, key string, def, min, max int) int {
	n, err := strconv.Atoi(v.Get(key))
	if err != nil {
		return def
	}
	if n < min {
		n = min
	}
	if n > max {
		n = max
	}
	return n
}

// clampFloat 解析可选浮点参数并钳制到 [min,max]；非法/缺省用 def。
func clampFloat(v url.Values, key string, def, min, max float64) float64 {
	f, err := strconv.ParseFloat(v.Get(key), 64)
	if err != nil {
		return def
	}
	if f < min {
		f = min
	}
	if f > max {
		f = max
	}
	return f
}

// TuneParam 描述一个可在前端调整的漫游参数（白盒：只暴露安全子集）。
// 范围约束由 clampInt/clampFloat 在服务端强制，前端只做展示与提交，
// 避免"高阶内部参数"（如跳数配额/PPR 阻尼/迭代次数）被用户误改成像风险。
type TuneParam struct {
	Key     string  `json:"key"`
	Label   string  `json:"label"`
	Type    string  `json:"type"` // int | float
	Min     float64 `json:"min"`
	Max     float64 `json:"max"`
	Step    float64 `json:"step"`
	Default float64 `json:"default"`
	Group   string  `json:"group"`
	Hint    string  `json:"hint"`
}

// tuneParams 前端可调参数白名单（与 CLI flags 一致的安全子集）。
func tuneParams() []TuneParam {
	return []TuneParam{
		{Key: "top", Label: "结果条数", Type: "int", Min: 1, Max: 60, Step: 1, Default: 15, Group: "基础", Hint: "每屏展示的节点数（1-60）"},
		{Key: "hops", Label: "最大跳数", Type: "int", Min: 1, Max: 5, Step: 1, Default: 3, Group: "基础", Hint: "激活扩散的最大跳数（1-5）"},
		{Key: "lambda", Label: "激活衰减", Type: "float", Min: 0, Max: 1, Step: 0.05, Default: 0.7, Group: "算法", Hint: "激活值随跳数的衰减系数（0-1），越大扩散越浅"},
		{Key: "theta", Label: "剪枝阈值", Type: "float", Min: 0, Max: 1, Step: 0.01, Default: 0.1, Group: "算法", Hint: "低于该值的激活路径被剪枝（0-1），越大结果越少"},
		{Key: "alpha", Label: "结构分权重", Type: "float", Min: 0, Max: 1, Step: 0.05, Default: 0.5, Group: "融合", Hint: "PPR 结构分在融合中的权重（0-1）"},
		{Key: "beta", Label: "激活分权重", Type: "float", Min: 0, Max: 1, Step: 0.05, Default: 0.5, Group: "融合", Hint: "激活分在融合中的权重（0-1）"},
		{Key: "rand_alpha", Label: "随机漫步·度加权", Type: "float", Min: 0, Max: 1, Step: 0.05, Default: 0.5, Group: "随机", Hint: "🎲 随机起点的度加权指数：0=均匀（惊喜），1=偏丰富簇；只影响随机漫步"},
	}
}

// handleConfig GET /api/config：返回前端可调参数白名单 + 源信息。
// 前端据此渲染设置抽屉（滑块/输入框），范围/步长/默认值来自服务端，
// 保证前后端对"可调什么、边界在哪"保持一致。
func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	st := s.G.Stats()
	s.mu.RUnlock()
	writeJSON(w, map[string]any{
		"params":  tuneParams(),
		"source":  s.Source,
		"vault":   s.VaultName,
		"version": s.Version,
		"nodes":   st.Nodes,
		"edges":   st.Edges,
	})
}
