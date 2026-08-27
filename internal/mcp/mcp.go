// Package mcp 实现 Serendipity 的 MCP 服务（第四个入口，v0.1.9；迁移 mcp-go v0.2.x）。
//
//	供 AI（agent，含 dsh-mneme 类）调用引擎能力——只读工具九件套：
//	graph.stats / graph.roam / graph.random / graph.relation / graph.node /
//	graph.similar / graph.community / seren.touch_digest / seren.state（v0.2.x）。
//
// 边界（design §6.10 / docs/architecture/07-mcp.md）：
//   - 只 import internal/{graph,roam,adapter}（纯库、无副作用）；绝不 import
//     internal/web（Web 是消费者不是内核）、internal/watch（监听是 serve 的事）。
//   - 只读：不写 touch、不触发 refresh、不读凭据表——AI 会话不能改动本地状态。
//   - 图与画像不内嵌：经 GraphProvider 每次调用取 live 图（v0.2.x，serve /mcp
//     修「子进程快照吃不到中途刷新改动」）；stdio 场景 provider 返回构建时的图即可。
//
// 传输（v0.2.x，mcp-go）：服务端同一套工具注册，暴露两个入口——
//   - Handler() http.Handler      → serve 挂 /mcp（Streamable HTTP，Web+REST+MCP 三合一）
//   - ServeStdio(in, out)         → 独立 `seren mcp`（stdio，Claude Desktop 兜底）
package mcp

import (
	"context"
	"encoding/json"
	"io"
	"math/rand/v2"
	"net/http"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"serendipity-engine/internal/adapter"
	"serendipity-engine/internal/graph"
	"serendipity-engine/internal/roam"
)

// GraphProvider 返回当前图与画像；nil,nil = 未配库（无库启动）。
// 由调用方注入：serve /mcp 读 web.Server 当前状态（RLock）；stdio 返回构建时的图。
type GraphProvider func() (*graph.Graph, *adapter.VaultProfile)

// Server 持有图提供器与画像，提供只读 MCP 服务（transport-agnostic）。
type Server struct {
	provider   GraphProvider     // 每次调用取 live 图（nil,nil = 未配库）
	version    string
	transport  string            // "streamable-http" | "stdio"
	touchDigFn func() (any, error) // §3.7 只读 digest 查询（nil = 不提供）
	tools      []mcp.Tool        // 工具定义（供 seren.state 计数）
	srv        *server.MCPServer // mcp-go 服务端（持工具注册）
}

// New 创建 MCP 服务（图与画像经 provider 注入；transport 描述当前形态）。
func New(provider GraphProvider, version, transport string) *Server {
	s := &Server{provider: provider, version: version, transport: transport}
	s.tools = toolDefs()
	s.srv = server.NewMCPServer("serendipity-engine", version)
	s.registerHandlers()
	s.registerPrompts()
	return s
}

// SetTouchDigest 注入只读 digest 查询闭包（§3.7，v0.1.14）。读 touch store，
// MCP 保持「只 import 纯库」边界；返回 (nil,nil) 表示无 digest。
func (s *Server) SetTouchDigest(fn func() (any, error)) {
	s.touchDigFn = fn
}

// registerPrompts 注册 MCP prompts（§3.8 Layer B）：一份 `seren_orientation`——
// 客户端（Claude Code 等）以斜杠命令触发，把"serendipity 使用说明"注入下文。
// prompt 按需触发（说明书），与常驻 Skill（SKILL.md 行为准则）互补。内容英文（与
// 工具描述/CLI 一致，跨 AI 客户端兼容）。
func (s *Server) registerPrompts() {
	// 说明文本：定位（是什么）/ 能力（能干什么）/ 工具与入参 / 反模式（与 docs/SKILL.md 同源）。
	const orientation = `Serendipity is a graph-roam engine over your notes: it returns explainable clusters of related notes from the real link structure, not search hits. Use it to explore/diverge ("what am I connected to"); use search to find an exact answer.

Read-only MCP tools (all take JSON object arguments — arguments are optional unless marked required):

  graph.stats         — library scale. No args.
  graph.roam          — roam from a query. args: q (required), top (1-60), hops (1-5), lambda (0-1), theta (0-1), alpha (0-1), beta (0-1).
  graph.random        — random wander. args: top, seed (fixed=reproducible), rand_alpha (0-1).
  graph.relation      — relation between two nodes. args: from (required), to (required) — accept ID or title.
  graph.node          — node detail. args: id (required) — ID or title.
  graph.similar       — structurally similar nodes. args: id (required), k (1-60).
  graph.community     — topic clusters (Leiden). args: resolution (default 1.0), seed (fixed=reproducible).
  seren.touch_digest  — behavioral digest of click activity. No args. Passive: only call when the user asks where attention is.
  seren.state         — session state (vault configured? transport? tools). No args. Always available.

Interpretation: these tools output candidates/hypotheses (structural estimates), not library facts. Treat as "worth a look" to confirm with the user — never present them as established edges or facts. Don't infer importance from touch counts; don't poll the digest proactively.`

	s.srv.AddPrompt(
		mcp.NewPrompt("seren_orientation",
			mcp.WithPromptDescription("What Serendipity does, the read-only MCP tools and their arguments, and how to interpret results."),
			mcp.WithPromptTitle("Serendipity orientation"),
		),
		func(ctx context.Context, _ mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
			return &mcp.GetPromptResult{
				Description: "Serendipity MCP usage guide",
				Messages: []mcp.PromptMessage{
					mcp.NewPromptMessage(mcp.RoleUser, mcp.NewTextContent(orientation)),
				},
			}, nil
		},
	)
}

// registerHandlers 把所有工具的调用分派绑定到 s.callTool（live 图每次取）。
func (s *Server) registerHandlers() {
	for _, t := range s.tools {
		name := t.Name
		s.srv.AddTool(t, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return s.callTool(name, req)
		})
	}
}

// Handler 返回 Streamable HTTP 服务（挂到 serve 的 /mcp）。mcp-go 的
// NewStreamableHTTPServer 返回的即 http.Handler，端点路径由 WithEndpointPath("/mcp") 指定。
func (s *Server) Handler() http.Handler {
	return server.NewStreamableHTTPServer(s.srv, server.WithEndpointPath("/mcp"))
}

// ServeStdio 处理 stdio 请求流（独立 `seren mcp` 子进程；Claude Desktop 兜底）。
// stdout 仅承载协议；启动提示必须写 stderr（main 里做），否则污染协议流。
// mcp-go 的 StdioServer.Listen(ctx, stdin, stdout) 支持注入输入输出（便于测试）。
func (s *Server) ServeStdio(in io.Reader, out io.Writer) error {
	if in == nil || out == nil {
		// 默认 os.Stdin/Stdout（正式 `seren mcp` 子进程）
		return server.ServeStdio(s.srv)
	}
	ss := server.NewStdioServer(s.srv)
	return ss.Listen(context.Background(), in, out)
}

// ---- tools/list ----

// toolDefs 返回九只读工具（v0.2.x：+seren.state）。描述为英文（跨 AI 客户端兼容），
// 全部标注只读语义（§3.8 Layer A：readOnlyHint/destructiveHint/idempotentHint——
// 协议层结构化提示，AI 读到即知只读、不破坏、幂等）。
func toolDefs() []mcp.Tool {
	tools := []mcp.Tool{
		mcp.NewTool("graph.stats",
			mcp.WithDescription("Library scale stats: nodes / edges / link ledger / orphans / connected components / hubs — an AI's first touch to see the graph. No params."),
			readOnly(),
		),
		mcp.NewTool("graph.roam",
			mcp.WithDescription("Query roam: anchor on a note name / tag / alias / ID / any substring, then return a score-ordered, explainable cluster of related nodes (with activation path). q is required."),
			readOnly(),
			mcp.WithString("q", mcp.Description("query term (note name / tag / alias / ID / any substring)"), mcp.Required()),
			mcp.WithInteger("top", mcp.Description("output count (1-60, default 15)")),
			mcp.WithInteger("hops", mcp.Description("activation max hops (1-5, default 3)")),
			mcp.WithNumber("lambda", mcp.Description("activation decay (0-1, default 0.7)")),
			mcp.WithNumber("theta", mcp.Description("activation pruning threshold (0-1, default 0.1)")),
			mcp.WithNumber("alpha", mcp.Description("structural (PPR) fusion weight (0-1, default 0.5)")),
			mcp.WithNumber("beta", mcp.Description("activation fusion weight (0-1, default 0.5)")),
		),
		mcp.NewTool("graph.random",
			mcp.WithDescription("Random walk: with no clear goal, 'just wander' — a random start node plus its cluster. Fixed seed is reproducible."),
			readOnly(),
			mcp.WithInteger("top", mcp.Description("output count (1-60, default 15)")),
			mcp.WithInteger("seed", mcp.Description("random seed (0=random; fixed reproduces same walk)")),
			mcp.WithNumber("rand_alpha", mcp.Description("start degree weighting exponent (0=uniform surprise, 1=favor rich clusters, default 0.5)")),
		),
		mcp.NewTool("graph.relation",
			mcp.WithDescription("Relationship between two nodes: shortest path + bidirectional PPR strength (symmetric affinity) + activation value + evidence chain. from/to accept ID or title."),
			readOnly(),
			mcp.WithString("from", mcp.Description("node A (name/id)"), mcp.Required()),
			mcp.WithString("to", mcp.Description("node B (name/id)"), mcp.Required()),
		),
		mcp.NewTool("graph.node",
			mcp.WithDescription("Single-node detail: text summary (L0) + neighbors + backlinks (L1) — confirm 'is this what I'm looking for'. id accepts ID or title."),
			readOnly(),
			mcp.WithString("id", mcp.Description("node ID or title"), mcp.Required()),
		),
		mcp.NewTool("graph.similar",
			mcp.WithDescription("Structurally similar nodes (Adamic-Adar twins): pairs with many shared neighbors but no direct link, with shared-neighbor evidence — 'which notes say the same thing' (pure structural alternative to an embedding axis). id accepts ID or title."),
			readOnly(),
			mcp.WithString("id", mcp.Description("node ID or title"), mcp.Required()),
			mcp.WithInteger("k", mcp.Description("output count (1-60, default 10)")),
		),
		mcp.NewTool("graph.community",
			mcp.WithDescription("Community detection (Leiden): break the graph into topic clusters — an AI can locate 'which topic clusters exist / which areas are disconnected' without walking the whole library (diagnostic layer: knowledge gaps). Larger resolution = more fragmented."),
			readOnly(),
			mcp.WithNumber("resolution", mcp.Description("resolution parameter (default 1.0; larger = more fragmented)")),
			mcp.WithInteger("seed", mcp.Description("random seed (0=random; fixed reproduces same partition)")),
		),
		mcp.NewTool("seren.touch_digest",
			mcp.WithDescription("Behavioral-signal digest (§3.7): top clicked nodes in the window (ghost-filtered + title) + top sources + timespan + new total — an AI can spot 'which topics are heating up, likely worth linking'. Read-only, passive (empty digest when none)."),
			readOnly(),
		),
		mcp.NewTool("seren.state",
			mcp.WithDescription("Report the engine/session state: whether a vault is configured, transport, tool count. Always available (even before configure) — use this first to know if the engine is ready, or to see the hint when not configured."),
			readOnly(),
		),
	}
	return tools
}

// readOnly 返回工具级只读语义注解（§3.8 Layer A）：readOnly + 非破坏 + 幂等。
func readOnly() mcp.ToolOption {
	return mcp.WithToolAnnotation(mcp.ToolAnnotation{
		ReadOnlyHint:    mcp.ToBoolPtr(true),
		DestructiveHint: mcp.ToBoolPtr(false),
		IdempotentHint:  mcp.ToBoolPtr(true),
	})
}

// ---- tools/call ----

// callTool 分派到各工具（live 图经 provider 取；未配库返回一致引导错误）。
func (s *Server) callTool(name string, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// seren.state 永远可用（不需要图）
	if name == "seren.state" {
		return okText(s.stateResult())
	}
	g, p := s.graph()
	if g == nil {
		return errText("no vault configured: engine is running but no vault is set. Configure it (e.g. `seren serve` without a vault then POST /api/vault, or start with a vault argument) before using this tool.")
	}
	args := argsOf(req)
	switch name {
	case "graph.stats":
		return okText(g.Stats())
	case "graph.roam":
		return s.roam(g, p, args)
	case "graph.random":
		return s.randomWalk(g, p, args)
	case "graph.relation":
		rel := s.relation(g, p, args)
		if rel == nil {
			return errText("node not found: from/to could not be anchored")
		}
		return okText(rel)
	case "graph.node":
		d := s.nodeDetail(g, p, args)
		if d == nil {
			return errText("node not found: id could not be anchored")
		}
		return okText(d)
	case "graph.similar":
		return s.similar(g, p, args)
	case "graph.community":
		return okText(s.community(g, p, args))
	case "seren.touch_digest":
		return s.touchDigest()
	default:
		return errText("unknown tool: " + name)
	}
}

// argsOf 提取工具调用参数为 map（mcp-go Arguments 是 any，实际为 map[string]any）。
func argsOf(req mcp.CallToolRequest) map[string]any {
	if req.Params.Arguments == nil {
		return nil
	}
	if m, ok := req.Params.Arguments.(map[string]any); ok {
		return m
	}
	return nil
}

// graph 取当前图与画像（provider 注入；nil,nil = 未配库）。
func (s *Server) graph() (*graph.Graph, *adapter.VaultProfile) {
	if s.provider == nil {
		return nil, nil
	}
	return s.provider()
}

// stateResult 返回 seren.state 的结果（不依赖图；未配库时给出引导）。
func (s *Server) stateResult() map[string]any {
	cfg := s.Configured()
	out := map[string]any{
		"configured": cfg,
		"transport":  s.transport,
		"endpoint":   map[string]string{"streamable-http": "/mcp", "stdio": ""}[s.transport],
		"version":    s.version,
		"tools":      len(s.tools),
	}
	if cfg {
		g, p := s.graph()
		if g != nil {
			st := g.Stats()
			out["nodes"] = st.Nodes
			out["edges"] = st.Edges
		}
		if p != nil {
			if p.Name != "" {
				out["vault"] = p.Name
			}
		}
	} else {
		out["hint"] = "No vault configured. Start `seren serve` with a vault (or POST /api/vault to set one), then reconnect."
	}
	return out
}

// Configured 当前是否已配库（provider 已给出非 nil 图）。
func (s *Server) Configured() bool {
	g, _ := s.graph()
	return g != nil
}

// ToolCount 返回注册的工具数（供 /api/mcp/status 展示）。
func (s *Server) ToolCount() int {
	return len(s.tools)
}

// ---- 各工具实现（复用内核函数，与 REST/CLI 同源） ----

func (s *Server) roam(g *graph.Graph, p *adapter.VaultProfile, raw map[string]any) (*mcp.CallToolResult, error) {
	var a struct {
		Q      string  `json:"q"`
		Top    int     `json:"top"`
		Hops   int     `json:"hops"`
		Lambda float64 `json:"lambda"`
		Theta  float64 `json:"theta"`
		Alpha  float64 `json:"alpha"`
		Beta   float64 `json:"beta"`
	}
	remarshal(raw, &a)
	opt := roam.Options{
		Top:              clamp(a.Top, 15, 1, 60),
		Hops:             clamp(a.Hops, 3, 1, 5),
		Lambda:           clampF(a.Lambda, 0.7, 0, 1),
		Theta:            clampF(a.Theta, 0.1, 0, 1),
		Alpha:            clampF(a.Alpha, 0.5, 0, 1),
		Beta:             clampF(a.Beta, 0.5, 0, 1),
		FilterStructural: true,
	}
	if a.Q == "" {
		return errText("roam: q is required")
	}
	return okText(roam.Compute(g, p, a.Q, opt))
}

func (s *Server) randomWalk(g *graph.Graph, p *adapter.VaultProfile, raw map[string]any) (*mcp.CallToolResult, error) {
	var a struct {
		Top       int     `json:"top"`
		Seed      int64   `json:"seed"`
		RandAlpha float64 `json:"rand_alpha"`
	}
	remarshal(raw, &a)
	var rng *rand.Rand
	if a.Seed != 0 {
		rng = rand.New(rand.NewPCG(uint64(a.Seed), uint64(a.Seed)>>1^0x9E3779B97F4A7C15))
	} else {
		rng = rand.New(rand.NewPCG(uint64(time.Now().UnixNano()), 0x9E3779B97F4A7C15))
	}
	opt := roam.Options{Top: clamp(a.Top, 15, 1, 60), Hops: 3, Lambda: 0.7,
		Theta: 0.1, Alpha: 0.5, Beta: 0.5, FilterStructural: true}
	return okText(roam.ComputeRandom(g, p, opt, roam.Roll{Rng: rng, Alpha: clampF(a.RandAlpha, 0.5, 0, 1)}))
}

func (s *Server) relation(g *graph.Graph, p *adapter.VaultProfile, raw map[string]any) *graph.Relation {
	var m struct {
		From string `json:"from"`
		To   string `json:"to"`
	}
	remarshal(raw, &m)
	if m.From == "" || m.To == "" {
		return nil
	}
	from := s.resolveID(g, m.From)
	to := s.resolveID(g, m.To)
	if from == "" || to == "" {
		return nil
	}
	return g.ComputeRelation(from, to, 0.7)
}

func (s *Server) nodeDetail(g *graph.Graph, p *adapter.VaultProfile, raw map[string]any) *graph.NodeDetail {
	var m struct {
		ID string `json:"id"`
	}
	remarshal(raw, &m)
	if m.ID == "" {
		return nil
	}
	id := s.resolveID(g, m.ID)
	if id == "" {
		return nil
	}
	return g.NodeDetail(id)
}

func (s *Server) similar(g *graph.Graph, p *adapter.VaultProfile, raw map[string]any) (*mcp.CallToolResult, error) {
	var m struct {
		ID string `json:"id"`
		K  int    `json:"k"`
	}
	remarshal(raw, &m)
	if m.ID == "" {
		return errText("similar: id is required")
	}
	id := s.resolveID(g, m.ID)
	if id == "" {
		return errText("similar: id could not be anchored")
	}
	structural := map[string]bool{}
	for _, t := range p.StructuralTypes {
		structural[t] = true
	}
	return okText(g.Similar(id, clamp(m.K, 10, 1, 60), structural))
}

func (s *Server) community(g *graph.Graph, p *adapter.VaultProfile, raw map[string]any) *graph.CommunityResult {
	var m struct {
		Resolution float64 `json:"resolution"`
		Seed       int64   `json:"seed"`
	}
	remarshal(raw, &m)
	res, err := g.Communities(clampF(m.Resolution, 1.0, 0, 100), m.Seed)
	if err != nil {
		return &graph.CommunityResult{}
	}
	return res
}

func (s *Server) touchDigest() (*mcp.CallToolResult, error) {
	if s.touchDigFn == nil {
		return errText("touch digest unavailable (no touch store configured)")
	}
	d, err := s.touchDigFn()
	if err != nil {
		return errText("touch digest: " + err.Error())
	}
	if d == nil {
		return okText(map[string]any{"digest": nil, "available": false})
	}
	return okText(d)
}

func (s *Server) resolveID(g *graph.Graph, q string) string {
	ms := g.Resolve(q)
	if len(ms) == 0 {
		return ""
	}
	return ms[0].ID
}

// ---- 小工具 ----

// okText 把任意可 JSON 化的结果包成 MCP text content（白盒 JSON，AI 直接消费）。
func okText(v any) (*mcp.CallToolResult, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return mcp.NewToolResultError("encode failed: " + err.Error()), nil
	}
	return mcp.NewToolResultText(string(data)), nil
}

// errText 返回 MCP 错误（IsError 标记，客户端识别为失败而非内容）。
func errText(msg string) (*mcp.CallToolResult, error) {
	return mcp.NewToolResultError(msg), nil
}

// remarshal 把 map[string]any 参数解码到结构体（mcp-go Arguments 即 map）。
// 忽略错误（字段缺失回零值，钳制函数兜底）。
func remarshal(raw map[string]any, out any) {
	if raw == nil {
		return
	}
	b, err := json.Marshal(raw)
	if err != nil {
		return
	}
	_ = json.Unmarshal(b, out)
}

func clamp(v, def, lo, hi int) int {
	if v == 0 {
		return def
	}
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
func clampF(v, def, lo, hi float64) float64 {
	if v == 0 {
		return def
	}
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
