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
//	  GET  /api/similar?id=&k=  结构相似节点（v0.1.11，Jaccard 孪生：
//	       共同邻居多但互不链接，带共享邻居证据；独立入口不并入 roam 红线 1）
//	  GET  /api/node?id=  单节点详情（v0.1.11：L0 Text 摘要 + L1 邻居/被引用）
//	  GET  /api/touch/stats  反馈埋点只读统计（v0.1.11，backlog §3.3；
//	       只读聚合，绝不反馈到排序/hot——红线 2）
//	  GET  /api/config   前端可调参数白名单（top/hops/lambda/theta/alpha/beta
//	       + 范围约束；前端据此渲染设置抽屉，见 tuneParams）
//	  POST /api/refresh  对账刷新（v0.1.2，见下）
//	  GET  /             嵌入的可视化页面（go:embed static/index.html）
//	roam 支持 ?export=1 → Markdown 卡片清单（v0.1.11，backlog §3.2：
//	       导出语义 = 卡片清单而非重新生成笔记；默认路径行为完全不变）
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
//	v0.1.11 GET /api/similar（Jaccard 结构相似，独立入口）+ GET /api/node
//	        （节点详情 L0/L1）+ GET /api/touch/stats（埋点只读统计）+
//	        /api/roam?export=1（Markdown 卡片清单导出）。
//	v0.1.12 GET /api/communities（Leiden 社区发现，诊断层）；/api/stats 加
//	        is_pending（库变化待刷新）+ dangling_refs（悬空链接明细）；touch/stats
//	        targets 过滤幽灵 touch；similar 升级 Adamic-Adar。
//	v0.1.15 无库启动：/api/stats 加 configured；GET/POST /api/vault（配库/换库，
//	        VaultFunc 闭包 + ApplyVaultState + OnVaultApplied 钩子）；路由全量注册
//	        + handler 内闭包 nil 判定（rlockGraph 守卫 G/P）；未配库数据端点 503。
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
	"serendipity-engine/internal/mcp"
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

// TouchStatsFunc 反馈埋点只读统计闭包（由 main 构造，读 store touch 表聚合；
// v0.1.11，backlog §3.3 —— 只读分析，绝不反馈到排序/hot，红线 2）。
// 返回 (total 总点击数, targets 被点击 TopN, sources 点击来源 TopN, err)。
// 用 web 自有类型而非 store.TouchRow——保持 web 不 import store 的边界
// （闭包注入模式：main 负责把 store 结果映射过来）。
type TouchStatsFunc func() (total int, targets, sources []TouchRow, err error)

// TouchRow 埋点聚合行（web 层展示形态）。
type TouchRow struct {
	ID    string `json:"id"`
	Count int    `json:"count"`
}

// DigestTarget digest 聚合行（web 层展示形态：ID + 标题 + 次数）。
type DigestTarget struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Count int    `json:"count"`
}

// Digest digest 内容（web 层展示形态；§3.7，v0.1.14——只读接口，引擎零写 vault）。
type Digest struct {
	ID          string         `json:"id"`           // 唯一 id（unix 纳秒）
	GeneratedAt int64          `json:"generated_at"` // 生成时间（unix 秒）
	WindowStart int64          `json:"window_start"` // 窗口起点（unix 秒）
	Since       string         `json:"since"`        // 窗口起点人读串
	Total       int            `json:"total"`        // 窗口新增 touch 数
	Targets     []DigestTarget `json:"targets"`      // TopN（幽灵过滤 + 标题）
	Sources     []TouchRow     `json:"sources"`      // TopN 来源词
}

// TouchDigestFunc 最新 digest 查询闭包（由 main 构造，读 touch store；nil → 无）。
type TouchDigestFunc func() (*Digest, error)

// TouchAckFunc digest 已读标记闭包（由 main 构造，写 touch store meta）。
type TouchAckFunc func(id string) error

// VaultOpts 配库请求的可选参数（覆盖 serve 启动默认；字段与 CLI flags 对应）。
type VaultOpts struct {
	ProfileName string `json:"profile_name"` // 内置画像名
	Profile     string `json:"profile"`      // 显式画像文件
	Store       string `json:"store"`        // 持久化路径覆盖
	DB          string `json:"db"`           // 从持久化存储读图（跳过解析）
}

// VaultState 配库成功的完整状态（由 main 构造）：新图 + 画像 + 全套闭包。
// web 只应用状态（ReplaceGraph + 字段替换），不感知解析细节（闭包注入边界）。
type VaultState struct {
	G         *graph.Graph
	P         *adapter.VaultProfile
	Source    string
	VaultName string // Obsidian vault 名（跳转）；空 = 不提供
	OrcaRepo  string // 虎鲸 repo 名（跳转）；空 = 不提供
	Refresh   RefreshFunc
	Touch     TouchFunc
	TouchStat TouchStatsFunc
	TouchDg   TouchDigestFunc
	TouchAck  TouchAckFunc
	DigAvail  func() bool
	IsPending func() bool
}

// VaultFunc 配库闭包（由 main 构造）：path + opts → 解析建图 → 完整状态。
// 无库启动（seren serve 不带 vault）时注入；POST /api/vault 调用后应用。
type VaultFunc func(path string, opts VaultOpts) (*VaultState, error)

// Server 持有图与画像，提供 REST 接口。
type Server struct {
	mu        sync.RWMutex // 保护 G 与 revision（刷新替换图，读接口并发安全）
	G         *graph.Graph
	P         *adapter.VaultProfile
	Source    string
	VaultName string // Obsidian vault 名（obsidian:// URI 跳转用）；空 = 不提供跳转
	OrcaRepo  string // 虎鲸 repo 名（orca-note:// URI 跳转用）；空 = 不提供跳转
	Version   string
	Refresh   RefreshFunc    // 非空时注册 POST /api/refresh
	Touch     TouchFunc      // 非空时注册 POST /api/touch（反馈埋点）
	TouchStat TouchStatsFunc // 非空时注册 GET /api/touch/stats（只读统计，v0.1.11）
	TouchDg   TouchDigestFunc // 非空时注册 GET /api/touch/digest（§3.7，v0.1.14）
	TouchAck  TouchAckFunc   // 非空时注册 POST /api/touch/digest/ack（§3.7）
	DigAvail  func() bool    // 非空时 /api/stats 返回 digest_available（§3.7）
	IsPending func() bool    // 非空时 /api/stats 返回 is_pending（v0.1.12，roadmap #14：库有变化待刷新）
	Vault     VaultFunc      // 非空时注册 POST /api/vault（无库启动配库指令，v0.1.15）
	Token     string         // API 鉴权 token（v0.1.8 安全前置）；空 = 未配置鉴权
	revision  int            // 图版本号：每次刷新 +1，前端轮询 stats 对比以提示"库已更新"

	// MCP（v0.2.0，mcp-go / Streamable HTTP）：serve 内嵌 /mcp 端点，前端/插件
	// 可查状态并启停。MCP 非空时注册 /mcp + /api/mcp/*；MCPOn 控制 /mcp 是否可访问。
	MCP  *mcp.Server
	MCPOn bool

	// OnVaultApplied 配库成功应用后的回调（v0.1.15，由 main 注入）：配库闭包
	// 返回状态 → handleVault 替换图/闭包字段后调用——main 在此启动/重建 watch。
	OnVaultApplied func()

	// 随机漫步状态（v0.1.7）：randMu 保护 rng 与 recent（rand.Rand 非并发安全）。
	randMu sync.Mutex
	rng    *rand.Rand // 时间种子随机源（?seed=N 时用独立固定源，不占此锁路径）
	recent []string   // 最近随机起点（防重复 ring，上限 32）
}

// New 创建 Web 服务；refresh/touch/touchStat 为 nil 时不注册对应端点。
func New(g *graph.Graph, p *adapter.VaultProfile, source, vaultName, version string, refresh RefreshFunc, touch TouchFunc) *Server {
	return &Server{
		G: g, P: p, Source: source, VaultName: vaultName, Version: version,
		Refresh: refresh, Touch: touch,
		rng: rand.New(rand.NewPCG(uint64(time.Now().UnixNano()), 0x9E3779B97F4A7C15)),
	}
}

// SetTouchStats 注入埋点只读统计闭包（v0.1.11；New 后调用，避免改签名波及调用方）。
func (s *Server) SetTouchStats(fn TouchStatsFunc) {
	s.TouchStat = fn
}

// SetTouchDigest 注入 digest 查询闭包（§3.7，v0.1.14；New 后调用）。
func (s *Server) SetTouchDigest(fn TouchDigestFunc) {
	s.TouchDg = fn
}

// SetTouchAck 注入 digest 已读闭包（§3.7，v0.1.14；New 后调用）。
func (s *Server) SetTouchAck(fn TouchAckFunc) {
	s.TouchAck = fn
}

// SetDigestAvailable 注入 digest_available 查询（§3.7，v0.1.14；New 后调用）。
func (s *Server) SetDigestAvailable(fn func() bool) {
	s.DigAvail = fn
}

// SetIsPending 注入"有待刷新变化"查询（v0.1.12，roadmap #14）。non-nil 时
// /api/stats 返回 is_pending；手动刷新后由 main 的刷新闭包清 pending。
func (s *Server) SetIsPending(fn func() bool) {
	s.IsPending = fn
}

// SetVault 注入配库闭包（无库启动 v0.1.15）：path + opts → 解析建图 → 完整状态。
// 注入后 POST /api/vault 可用；配库成功 ReplaceGraph 并替换全套闭包字段。
func (s *Server) SetVault(fn VaultFunc) {
	s.Vault = fn
}

// Configured 当前是否已配库（G 非空 = 已配）。无库启动时 false，
// /api/stats 返回 configured:false，前端据此显示选库引导。
func (s *Server) Configured() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.G != nil
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
// v0.1.15 无库启动：路由一律无条件注册，闭包/图是否为 nil 由各 handler 自行
// 判定（返回 503 未配置）——配库（POST /api/vault）成功后闭包从 nil 变非 nil，
// 同一份 Handler 立即生效，无需重建路由。
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/stats", s.handleStats)
	mux.HandleFunc("/api/hot", s.handleHot)
	mux.HandleFunc("/api/roam", s.handleRoam)
	mux.HandleFunc("/api/relation", s.handleRelation)
	mux.HandleFunc("/api/similar", s.handleSimilar)
	mux.HandleFunc("/api/suggest-links", s.handleSuggestLinks)
	mux.HandleFunc("/api/node", s.handleNode)
	mux.HandleFunc("/api/communities", s.handleCommunities)
	mux.HandleFunc("/api/config", s.handleConfig)
	mux.HandleFunc("/api/vault", s.handleVault) // v0.1.15 无库启动配库指令
	mux.HandleFunc("/api/refresh", s.handleRefresh)
	mux.HandleFunc("/api/touch", s.handleTouch)
	mux.HandleFunc("/api/touch/stats", s.handleTouchStats)
	mux.HandleFunc("/api/touch/digest", s.handleTouchDigest)
	mux.HandleFunc("/api/touch/digest/ack", s.handleTouchDigestAck)
	if s.MCP != nil {
		mux.HandleFunc("/mcp", s.handleMCP)
		mux.HandleFunc("/api/mcp/status", s.handleMCPStatus)
		mux.HandleFunc("/api/mcp/enable", s.handleMCPEnable)
		mux.HandleFunc("/api/mcp/disable", s.handleMCPDisable)
	}
	mux.HandleFunc("/", s.handleIndex)
	return s.auth(mux)
}

// vaultStateError 未配库的统一错误响应（无库启动时各数据端点的 503 形态）。
func vaultStateError(w http.ResponseWriter) {
	writeJSON(w, map[string]any{"error": "no vault configured", "configured": false})
}

// rlockGraph 获取图读锁并校验已配库（v0.1.15 无库启动守卫）：
// 已配库（G 与 P 均非 nil，配库时一起设置）→ 持 RLock 返回 true（调用方 defer
// s.mu.RUnlock()）；未配库 → 已写 503 并解锁，返回 false。P 一并检查：handler
// 在锁内访问 s.P（StructuralTypes 等），G 非空但 P 空会在那里 nil panic
// （v0.1.15 实测 handleHot 顺序 bug 的根因——守卫必须在任何 s.P 访问之前）。
func (s *Server) rlockGraph(w http.ResponseWriter) bool {
	s.mu.RLock()
	if s.G == nil || s.P == nil {
		s.mu.RUnlock()
		vaultStateError(w)
		return false
	}
	return true
}

// ApplyVaultState 应用配库状态：替换图/画像/源/全套闭包并递增 revision
// （v0.1.15，handleVault 与 cmd/seren 启动即配库共用）。
func (s *Server) ApplyVaultState(st *VaultState) {
	s.mu.Lock()
	s.G = st.G
	s.P = st.P
	s.Source = st.Source
	s.VaultName = st.VaultName
	s.OrcaRepo = st.OrcaRepo
	s.Refresh = st.Refresh
	s.Touch = st.Touch
	s.TouchStat = st.TouchStat
	s.TouchDg = st.TouchDg
	s.TouchAck = st.TouchAck
	s.DigAvail = st.DigAvail
	s.IsPending = st.IsPending
	s.revision++
	s.mu.Unlock()
}

// handleVault POST /api/vault：无库启动的配库指令（v0.1.15）。
// body {path, profile_name?, profile?, store?, db?} → 调 Vault 闭包（解析建图 +
// 构造全套闭包）→ 成功后 ApplyVaultState + 通知 OnVaultApplied（main 重建 watch）。
// 已配库时再配 = 换库（同幂等语义：重新解析新 path，旧图/闭包整体替换）。
// GET /api/vault：查询当前配置（configured/source/vault/nodes）。
func (s *Server) handleVault(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.mu.RLock()
		defer s.mu.RUnlock()
		resp := map[string]any{"configured": s.G != nil, "source": s.Source, "vault": s.VaultName}
		if s.G != nil {
			st := s.G.Stats()
			resp["nodes"] = st.Nodes
			resp["edges"] = st.Edges
		}
		writeJSON(w, resp)
	case http.MethodPost:
		if s.Vault == nil {
			writeJSON(w, map[string]string{"error": "vault config unavailable"})
			return
		}
		var body struct {
			Path string `json:"path"`
			VaultOpts
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Path == "" {
			writeJSON(w, map[string]string{"error": "path required"})
			return
		}
		st, err := s.Vault(body.Path, body.VaultOpts)
		if err != nil {
			writeJSON(w, map[string]string{"error": err.Error()})
			return
		}
		s.ApplyVaultState(st)
		// 配库成功应用后通知 main（启动/重建 watch 等生命周期动作）
		if s.OnVaultApplied != nil {
			s.OnVaultApplied()
		}
		st2 := st.G.Stats()
		writeJSON(w, map[string]any{
			"ok": "true", "configured": true,
			"source": st.Source, "vault": st.VaultName,
			"nodes": st2.Nodes, "edges": st2.Edges,
		})
	default:
		writeJSON(w, map[string]string{"error": "method not allowed"})
	}
}

// SetMCP 注入 serve 内嵌的 MCP 服务（v0.2.0）：非空时注册 /mcp + /api/mcp/*，
// MCPOn 控制 /mcp 是否可访问（默认开）。GraphProvider 由 main 构造（读当前 VaultState）。
func (s *Server) SetMCP(srv *mcp.Server, on bool) {
	s.MCP = srv
	s.MCPOn = on
}

// MCPGraphProvider 返回供 mcp.Server 用的 live 图快照函数：每次调用持 RLock 读当前
// G/P（refresh/换库后自动用新图，修「子进程快照吃不到中途改动」）。nil,nil = 未配库。
func (s *Server) MCPGraphProvider() func() (*graph.Graph, *adapter.VaultProfile) {
	return func() (*graph.Graph, *adapter.VaultProfile) {
		s.mu.RLock()
		defer s.mu.RUnlock()
		return s.G, s.P
	}
}

// handleMCP 把请求转发给内嵌 MCP 服务（Streamable HTTP，端点 /mcp）。
// 仅在 MCPOn 时可用；关闭时返回 404（不响应协议，客户端重连会看到拒绝）。
func (s *Server) handleMCP(w http.ResponseWriter, r *http.Request) {
	if !s.MCPOn || s.MCP == nil {
		http.NotFound(w, r)
		return
	}
	s.mcpHandler().ServeHTTP(w, r)
}

// mcpHandler 惰性构造内嵌 MCP 的 Streamable HTTP handler（每次构建保持与
// 当前 MCPOn 读取一致；Handler() 注册时一次构造也可，但惰性保证 enable/disable
// 后再次调用取到同一实例）。
func (s *Server) mcpHandler() http.Handler {
	return s.MCP.Handler()
}

// mcpStatus GET /api/mcp/status：MCP 状态（transport/enabled/config/tools）。
// 供前端/插件展示"MCP 已就绪 / 未配库 / 已停用"。
func (s *Server) handleMCPStatus(w http.ResponseWriter, r *http.Request) {
	if s.MCP == nil {
		writeJSON(w, map[string]any{"enabled": false, "configured": false, "tools": 0})
		return
	}
	writeJSON(w, map[string]any{
		"enabled":    s.MCPOn,
		"configured": s.MCP.Configured(),
		"tools":      s.MCP.ToolCount(),
		"transport":  "streamable-http",
		"endpoint":   "/mcp",
	})
}

// handleMCPEnable / disable：切换 /mcp 是否可访问（v0.2.0 启停开关，供前端/插件）。
func (s *Server) handleMCPEnable(w http.ResponseWriter, r *http.Request) {
	if s.MCP == nil {
		writeJSON(w, map[string]any{"error": "mcp unavailable"})
		return
	}
	s.MCPOn = true
	writeJSON(w, map[string]any{"ok": true, "enabled": true})
}

func (s *Server) handleMCPDisable(w http.ResponseWriter, r *http.Request) {
	if s.MCP == nil {
		writeJSON(w, map[string]any{"error": "mcp unavailable"})
		return
	}
	s.MCPOn = false
	writeJSON(w, map[string]any{"ok": true, "enabled": false})
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
	if s.Touch == nil {
		writeJSON(w, map[string]string{"error": "touch unavailable"})
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
	if s.Refresh == nil {
		writeJSON(w, map[string]string{"error": "refresh unavailable"})
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
	if s.G == nil {
		vaultStateError(w)
		return
	}
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

// similarItem 结构相似响应项（web 层补充标题/类型 + 共享邻居证据标题）。
type similarItem struct {
	ID           string   `json:"id"`
	Title        string   `json:"title"`
	Type         string   `json:"type"`
	Score        float64  `json:"score"`
	Shared       []string `json:"shared"`        // 共享邻居 ID（证据）
	SharedTitles []string `json:"shared_titles"` // 共享邻居标题（证据可读）
	URI          string   `json:"uri,omitempty"` // 跳转
}

// handleSimilar GET /api/similar?id=&k=：结构相似节点（v0.1.11，backlog §3.1）。
// id 接受 ID 或标题（Resolve 锚定首个）。输出 Jaccard 相似节点 + 共享邻居证据
// （white-box："因为都链接了 X/Y"）。独立入口——绝不并入 roam 管线（红线 1）。
func (s *Server) handleSimilar(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("id")
	if q == "" {
		writeJSON(w, map[string]string{"error": "id required"})
		return
	}
	k := atoiDefault(r.URL.Query().Get("k"), 10)
	if !s.rlockGraph(w) {
		return
	}
	defer s.mu.RUnlock()
	id := s.resolveID(q)
	if id == "" {
		writeJSON(w, map[string]string{"error": "node not found"})
		return
	}
	structural := map[string]bool{}
	for _, t := range s.P.StructuralTypes {
		structural[t] = true
	}
	sims := s.G.Similar(id, k, structural)
	out := make([]similarItem, 0, len(sims))
	for _, sm := range sims {
		var titles []string
		for _, sid := range sm.Shared {
			if n, ok := s.G.Node(sid); ok && n.Title != "" {
				titles = append(titles, n.Title)
			}
		}
		out = append(out, similarItem{
			ID: sm.ID, Title: sm.Title, Type: sm.Type,
			Score: sm.Score, Shared: sm.Shared, SharedTitles: titles,
			URI: s.uriFor(nodePath(s.G, sm.ID), sm.ID),
		})
	}
	writeJSON(w, map[string]any{"id": id, "results": out})
}

// handleSuggestLinks GET /api/suggest-links?k=：潜在关联待审清单（v0.1.13，
// roadmap #15，backlog §3.6）。引擎从拓扑多算法（AA/Jaccard/RA 原始分加权求和）
// 估算"近似相关"的候选对——有界、标注算法与共享邻居证据、**未落图**。
// 消费方 = 外部 AI / agent 研判（取候选 + 笔记正文判定，
// 接受者写回 kind=ai 边）。只读、无副作用。
func (s *Server) handleSuggestLinks(w http.ResponseWriter, r *http.Request) {
	k := atoiDefault(r.URL.Query().Get("k"), 50)
	if k > 200 {
		k = 200
	}
	if !s.rlockGraph(w) {
		return
	}
	defer s.mu.RUnlock()
	structural := map[string]bool{}
	for _, t := range s.P.StructuralTypes {
		structural[t] = true
	}
	links := s.G.PotentialLinks(2, structural)
	// top-K 节流：per-node K=2 已限总量（≤2N），这里再按请求条数截断
	if len(links) > k {
		links = links[:k]
	}
	out := make([]suggestItem, 0, len(links))
	for _, e := range links {
		out = append(out, suggestItem{
			A: e.A, B: e.B, Score: e.Score, Algorithms: e.Algorithms, Shared: e.Shared,
			ATitle: s.titleOf(e.A), BTitle: s.titleOf(e.B),
			AURI: s.uriFor(nodePath(s.G, e.A), e.A),
			BURI: s.uriFor(nodePath(s.G, e.B), e.B),
		})
	}
	writeJSON(w, map[string]any{"count": len(out), "results": out})
}

// suggestItem 潜在关联响应项（web 层补充端点标题/跳转，证据保持白盒）。
type suggestItem struct {
	A          string   `json:"a"`
	B          string   `json:"b"`
	Score      float64  `json:"score"`      // 聚合分（AA/RA/Jaccard 原始分加权求和）
	Algorithms []string `json:"algorithms"` // 命中算法（aa/jaccard/ra）
	Shared     []string `json:"shared"`     // 共享邻居 ID（证据："都链接了 X/Y"）
	ATitle     string   `json:"a_title"`
	BTitle     string   `json:"b_title"`
	AURI       string   `json:"a_uri,omitempty"`
	BURI       string   `json:"b_uri,omitempty"`
}

// titleOf 节点标题（不存在/空 → 用 ID 兜底，展示不崩）。
func (s *Server) titleOf(id string) string {
	if n, ok := s.G.Node(id); ok && n.Title != "" {
		return n.Title
	}
	return id
}

// handleNode GET /api/node?id=：单节点详情（v0.1.11，roadmap M1 #2 / 前端 #3）。
// id 接受 ID 或标题。输出 L0 摘要 + L1 邻居/被引用（graph.NodeDetail 原样）。
func (s *Server) handleNode(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("id")
	if q == "" {
		writeJSON(w, map[string]string{"error": "id required"})
		return
	}
	if !s.rlockGraph(w) {
		return
	}
	defer s.mu.RUnlock()
	id := s.resolveID(q)
	if id == "" {
		writeJSON(w, map[string]string{"error": "node not found"})
		return
	}
	d := s.G.NodeDetail(id)
	if d == nil {
		writeJSON(w, map[string]string{"error": "node not found"})
		return
	}
	writeJSON(w, d)
}

// handleTouchStats GET /api/touch/stats：反馈埋点只读统计（v0.1.11，backlog §3.3）。
// 只读分析：总点击数 + 被点击 TopN + 来源 TopN。绝不反馈到排序/hot（红线 2：
// 否则等于偷偷启动边权演化）。不进 MCP（隐私敏感，见 backend-backlog §3.3）。
func (s *Server) handleTouchStats(w http.ResponseWriter, r *http.Request) {
	if s.TouchStat == nil {
		writeJSON(w, map[string]string{"error": "touch stats unavailable"})
		return
	}
	limit := atoiDefault(r.URL.Query().Get("n"), 10)
	total, targets, sources, err := s.TouchStat()
	if err != nil {
		writeJSON(w, map[string]string{"error": err.Error()})
		return
	}
	if len(targets) > limit {
		targets = targets[:limit]
	}
	if len(sources) > limit {
		sources = sources[:limit]
	}
	writeJSON(w, map[string]any{
		"total": total, "targets": targets, "sources": sources,
	})
}

// handleTouchDigest GET /api/touch/digest：最新 digest 只读查询（§3.7，v0.1.14）。
// 被动告知（§3.7.2）：仅主动查询才返回；不主动推送、不弹窗。无 digest → 空摘要
// （200 + 零值，不报错——前端轻量状态提醒的轮询友好形态）。
func (s *Server) handleTouchDigest(w http.ResponseWriter, r *http.Request) {
	if s.TouchDg == nil {
		writeJSON(w, map[string]any{"digest": nil, "available": false})
		return
	}
	dig, err := s.TouchDg()
	if err != nil {
		writeJSON(w, map[string]string{"error": err.Error()})
		return
	}
	available := false
	if dig != nil && s.DigAvail != nil {
		available = s.DigAvail()
	}
	if dig == nil {
		writeJSON(w, map[string]any{"digest": nil, "available": false})
		return
	}
	writeJSON(w, map[string]any{"digest": dig, "available": available})
}

// handleTouchDigestAck POST /api/touch/digest/ack：标记 digest 已读（§3.7，v0.1.14）。
// body {id}；只写 touch store meta（last_ack_id），不碰 touch 事件、不反馈排序（红线）。
func (s *Server) handleTouchDigestAck(w http.ResponseWriter, r *http.Request) {
	if s.TouchAck == nil {
		writeJSON(w, map[string]string{"error": "touch digest ack unavailable"})
		return
	}
	if r.Method != http.MethodPost {
		writeJSON(w, map[string]string{"error": "method not allowed"})
		return
	}
	var body struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, map[string]string{"error": "invalid body"})
		return
	}
	if body.ID == "" {
		writeJSON(w, map[string]string{"error": "id required"})
		return
	}
	if err := s.TouchAck(body.ID); err != nil {
		writeJSON(w, map[string]string{"ok": "false"})
		return
	}
	writeJSON(w, map[string]string{"ok": "true"})
}

type statsResp struct {
	Configured      bool                `json:"configured"`       // v0.1.15：是否已配库（无库启动时为 false，前端显示选库引导）
	Nodes           int                 `json:"nodes"`
	Edges           int                 `json:"edges"`
	Version         string              `json:"version"`
	Revision        int                 `json:"revision"`         // 图版本号：自动/手动刷新后 +1（前端轮询提示更新）
	IsPending       bool                `json:"is_pending"`       // v0.1.12：库有变化待刷新（roadmap #14；手动刷新清 pending）
	DigestAvailable bool                `json:"digest_available"` // §3.7：有未读 digest（轻量状态提醒开关；DigAvail nil = false）
	Dangling        int                 `json:"dangling"`         // 悬空目标种数
	DanglingRefs    []graph.DanglingRef `json:"dangling_refs"`    // v0.1.12：悬空链接明细（backlog §四 缺口①，截断上限）
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	if s.G == nil {
		s.mu.RUnlock()
		writeJSON(w, statsResp{Configured: false, Version: s.Version})
		return
	}
	st := s.G.Stats()
	rev := s.revision
	isPending := false
	if s.IsPending != nil {
		isPending = s.IsPending()
	}
	digAvail := false
	if s.DigAvail != nil {
		digAvail = s.DigAvail()
	}
	// 悬空链接明细（v0.1.12，backlog §四 缺口①）：截断上限防 stats 轮询响应膨胀
	danglingRefs := s.G.DanglingRefs()
	if len(danglingRefs) > maxDanglingRefs {
		danglingRefs = danglingRefs[:maxDanglingRefs]
	}
	s.mu.RUnlock()
	writeJSON(w, statsResp{
		Configured: true, Nodes: st.Nodes, Edges: st.Edges, Version: s.Version,
		Revision: rev, IsPending: isPending, DigestAvailable: digAvail,
		Dangling: st.DanglingLinks, DanglingRefs: danglingRefs,
	})
}

// maxDanglingRefs /api/stats 返回的悬空链接明细上限（前端每 30s 轮询 stats，
// 防大库悬空过多时响应膨胀——统计面板展示可截断后"… 共 N 条"）。
const maxDanglingRefs = 50

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
	s.mu.RLock()
	if s.G == nil || s.P == nil {
		s.mu.RUnlock()
		vaultStateError(w)
		return
	}
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

// handleCommunities GET /api/communities：社区发现（v0.1.12，roadmap #10 诊断层）。
// ?resolution= &seed= 可选；返回模块度 + 社区列表（按 Size 降序，含代表标题）+
// Membership（node→comm）。只读、无副作用（Leiden 不动图，仅分簇）。
func (s *Server) handleCommunities(w http.ResponseWriter, r *http.Request) {
	resolution := clampFloat(r.URL.Query(), "resolution", 1.0, 0, 100)
	seed := int64(0)
	if n, err := strconv.ParseInt(r.URL.Query().Get("seed"), 10, 64); err == nil {
		seed = n
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.G == nil {
		vaultStateError(w)
		return
	}
	res, err := s.G.Communities(resolution, seed)
	if err != nil {
		writeJSON(w, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{
		"modularity":      res.Modularity,
		"community_count": res.CommunityCount,
		"membership":      res.Membership,
		"communities":     res.Communities,
	})
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
	export := vals.Get("export") == "1"
	if !s.rlockGraph(w) {
		return
	}
	opt := roam.Options{
		// 可调参数：前端可从 /api/config 取白名单，这里统一钳制到安全范围
		// （高阶内部项——跳数配额/PPR 阻尼/迭代次数——不对外暴露，保持默认）。
		Top:              clampInt(vals, "top", 15, 1, 60),
		Hops:             clampInt(vals, "hops", 3, 1, 5),
		Lambda:           clampFloat(vals, "lambda", 0.7, 0, 1),
		Theta:            clampFloat(vals, "theta", 0.1, 0, 1),
		Alpha:            clampFloat(vals, "alpha", 0.5, 0, 1),
		Beta:             clampFloat(vals, "beta", 0.5, 0, 1),
		FilterStructural: true,
	}

	// 随机漫步（v0.1.7）：?random=1 时忽略 q，roll 随机起点 + 簇。
	if vals.Get("random") == "1" {
		out := s.computeRandom(vals, opt)
		resp := s.roamRespOf(out, "random")
		if export {
			writeMarkdown(w, s.exportMD(resp))
			s.mu.RUnlock()
			return
		}
		writeJSON(w, resp)
		s.mu.RUnlock()
		return
	}

	out := roam.Compute(s.G, s.P, query, opt)
	resp := s.roamRespOf(out, query)
	if export {
		writeMarkdown(w, s.exportMD(resp))
		s.mu.RUnlock()
		return
	}
	writeJSON(w, resp)
	s.mu.RUnlock()
}

// exportMD 把一次漫游结果渲染为 Markdown 卡片清单（v0.1.11，backlog §3.2）。
// 语义：卡片清单（标题 + 类型 + hop + 路径 + 分数），不是重新生成笔记；
// 导出不额外 touch（只读）。锚点、结果、降级命中都带上，供沉淀进笔记。
func (s *Server) exportMD(resp roamResp) string {
	var b strings.Builder
	b.WriteString("# Serendipity 漫游导出\n\n")
	b.WriteString(fmt.Sprintf("- 查询：`%s`（源 %s，vault %s）\n", resp.Query, resp.Source, resp.Vault))
	if len(resp.Anchors) > 0 {
		b.WriteString("\n## 锚点\n\n")
		for _, a := range resp.Anchors {
			mark := ""
			if a.Random {
				mark = " 🎲"
			}
			b.WriteString(fmt.Sprintf("- **%s** `%s` [%s]%s\n", a.Title, a.ID, a.Type, mark))
		}
	}
	if len(resp.Results) > 0 {
		b.WriteString("\n## 相关节点\n\n")
		for _, it := range resp.Results {
			hop := ""
			if it.Hops > 0 {
				hop = fmt.Sprintf(" · %d-hop", it.Hops)
			}
			b.WriteString(fmt.Sprintf("- **%s** `%s` [%s]%s\n", it.Title, it.ID, it.Type, hop))
			b.WriteString(fmt.Sprintf("  score %.3f · ppr %.4f · act %.3f\n", it.Score, it.PPR, it.Act))
			if len(it.Path) > 0 {
				b.WriteString("  路径：`" + strings.Join(it.Path, " → ") + "`\n")
			}
		}
	}
	if len(resp.FallbackHits) > 0 {
		b.WriteString("\n## 全文降级命中\n\n")
		for _, h := range resp.FallbackHits {
			b.WriteString(fmt.Sprintf("- **%s** `%s` [%s] · 命中 %d 次\n", h.Title, h.ID, h.Type, h.Count))
		}
	}
	if len(resp.Results) == 0 && len(resp.FallbackHits) == 0 {
		b.WriteString("\n_（无结果）_\n")
	}
	return b.String()
}

func writeMarkdown(w http.ResponseWriter, s string) {
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.Write([]byte(s))
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
	if s.G == nil {
		s.mu.RUnlock()
		// 未配库：仍返回参数白名单 + configured:false（前端渲染选库引导时
		// 也需要可调参数；nodes/edges 置 0）。
		writeJSON(w, map[string]any{
			"params": tuneParams(), "source": "", "vault": "",
			"version": s.Version, "nodes": 0, "edges": 0, "configured": false,
		})
		return
	}
	st := s.G.Stats()
	s.mu.RUnlock()
	writeJSON(w, map[string]any{
		"params":  tuneParams(),
		"source":  s.Source,
		"vault":   s.VaultName,
		"version": s.Version,
		"nodes":   st.Nodes,
		"edges":   st.Edges,
		"configured": true,
	})
}
