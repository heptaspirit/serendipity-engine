// Package mcp 实现 seren mcp（第四个入口，v0.1.9）：
//   供 AI（agent，含 dsh-mneme 类）经 MCP stdio 调用引擎能力。
//   只读四件套：graph.stats / graph.roam / graph.random / graph.relation。
//
// 边界（design §6.10 / docs/architecture/07-mcp.md）：
//   - 只 import internal/{graph,roam,adapter}（纯库、无副作用）；
//     绝不 import internal/web（Web 是消费者不是内核）、internal/watch（监听是 serve 的事）。
//   - 只读：不写 touch、不触发 refresh、不读凭据表——AI 会话不能改动本地状态
//     （库存数据 / touch 埋点），与全项目"克制 + 安全红线"一致。
//   - 自实现薄 MCP 协议（零第三方依赖）：起步只需 initialize / tools/list /
//     tools/call（+ ping）；stdio 换行分隔 JSON-RPC 2.0（每行一个消息）。
package mcp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"math/rand/v2"
	"time"

	"serendipity-engine/internal/adapter"
	"serendipity-engine/internal/graph"
	"serendipity-engine/internal/roam"
)

// Server 持有图与画像，提供只读 MCP 服务。
type Server struct {
	g       *graph.Graph
	p       *adapter.VaultProfile
	version string
}

// New 创建 MCP 服务（图与画像由调用方从 --db / vault 加载，见 cmd/seren main）。
func New(g *graph.Graph, p *adapter.VaultProfile, version string) *Server {
	return &Server{g: g, p: p, version: version}
}

// ---- 最小 JSON-RPC 2.0 结构（零第三方依赖） ----

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

// Serve 处理 stdio 请求流：换行分隔 JSON-RPC 请求 → 换行分隔响应。
// stdout 仅承载协议；任何启动提示必须写 stderr（main 里做），否则污染协议流。
func (s *Server) Serve(in io.Reader, out io.Writer) error {
	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	enc := json.NewEncoder(out)
	for sc.Scan() {
		line := sc.Bytes()
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var req rpcRequest
		if err := json.Unmarshal(line, &req); err != nil {
			// 解析错误：-32700（无有效 ID 回 null）
			_ = enc.Encode(rpcResponse{JSONRPC: "2.0", Error: &rpcError{Code: -32700, Message: "parse error"}})
			continue
		}
		if len(req.ID) == 0 { // 通知（无 id）不响应
			continue
		}
		_ = enc.Encode(s.handle(req))
	}
	return sc.Err()
}

func (s *Server) handle(req rpcRequest) rpcResponse {
	switch req.Method {
	case "initialize":
		return okResp(req.ID, map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": "serendipity-engine", "version": s.version},
		})
	case "ping":
		return okResp(req.ID, map[string]any{})
	case "tools/list":
		return okResp(req.ID, map[string]any{"tools": toolDefs()})
	case "tools/call":
		return s.callTool(req)
	default:
		return errResp(req.ID, -32601, "method not found: "+req.Method)
	}
}

// ---- tools/list ----

type toolDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

func toolDefs() []toolDef {
	schema := func(required []string, props map[string]any) map[string]any {
		return map[string]any{"type": "object", "properties": props, "required": required, "additionalProperties": false}
	}
	num := func(desc string) map[string]any { return map[string]any{"type": "number", "description": desc} }
	snum := func(desc string) map[string]any { return map[string]any{"type": "integer", "description": desc} }
	str := func(desc string) map[string]any { return map[string]any{"type": "string", "description": desc} }

	return []toolDef{
		{
			Name: "graph.stats", Description: "库规模统计：节点/边/链接账目/孤立/连通分量/枢纽——AI 先摸库。无参数。",
			InputSchema: schema(nil, map[string]any{}),
		},
		{
			Name: "graph.roam", Description: "查询漫游：输入笔记名/标签/任意词 → 筛选、排序、可解释的相关节点簇（带激活路径）。q 必填。",
			InputSchema: schema([]string{"q"}, map[string]any{
				"q":      str("查询词（笔记名/标签/别名/ID/任意子串）"),
				"top":    snum("输出条数（1-60，默认 15）"),
				"hops":   snum("激活扩散最大跳数（1-5，默认 3）"),
				"lambda": num("激活衰减（0-1，默认 0.7）"),
				"theta":  num("激活剪枝阈值（0-1，默认 0.1）"),
				"alpha":  num("结构分（PPR）融合权重（0-1，默认 0.5）"),
				"beta":   num("激活分融合权重（0-1，默认 0.5）"),
			}),
		},
		{
			Name: "graph.random", Description: "🎲 随机漫步：无明确目标时的『随便逛逛』——随机起点 + 它的簇一次给出。seed 固定可复现。",
			InputSchema: schema(nil, map[string]any{
				"top":       snum("输出条数（1-60，默认 15）"),
				"seed":      snum("随机种子（0=随机；固定值可复现同一漫步）"),
				"rand_alpha": num("起点度加权指数（0=均匀惊喜，1=偏丰富簇，默认 0.5）"),
			}),
		},
		{
			Name: "graph.relation", Description: "两节点关系：最短路径 + 双向 PPR 强度（对称 affinity）+ 激活值 + 证据链。from/to 接受 ID 或标题。",
			InputSchema: schema([]string{"from", "to"}, map[string]any{
				"from": str("节点 A（名称/ID）"),
				"to":   str("节点 B（名称/ID）"),
			}),
		},
	}
}

// ---- tools/call ----

func (s *Server) callTool(req rpcRequest) rpcResponse {
	var p struct {
		Name string          `json:"name"`
		Args json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(req.Params, &p); err != nil {
		return errResp(req.ID, -32602, "invalid params")
	}

	var payload any
	switch p.Name {
	case "graph.stats":
		payload = s.g.Stats()
	case "graph.roam":
		payload = s.roam(p.Args)
	case "graph.random":
		payload = s.randomWalk(p.Args)
	case "graph.relation":
		rel := s.relation(p.Args)
		if rel == nil {
			return errResp(req.ID, -32602, "node not found: from/to 无法锚定")
		}
		payload = rel
	default:
		return errResp(req.ID, -32602, "unknown tool: "+p.Name)
	}

	if payload == nil {
		payload = map[string]any{}
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return errResp(req.ID, -32602, "encode failed: "+err.Error())
	}
	return okResp(req.ID, map[string]any{
		"content": []any{map[string]any{"type": "text", "text": string(data)}},
	})
}

// roam 查询漫游（复用 roam.Compute，与 CLI/Web 同一管线）。
func (s *Server) roam(raw json.RawMessage) *roam.Outcome {
	var a struct {
		Q      string  `json:"q"`
		Top    int     `json:"top"`
		Hops   int     `json:"hops"`
		Lambda float64 `json:"lambda"`
		Theta  float64 `json:"theta"`
		Alpha  float64 `json:"alpha"`
		Beta   float64 `json:"beta"`
	}
	_ = json.Unmarshal(raw, &a)
	opt := roam.Options{
		Top:    clamp(a.Top, 15, 1, 60),
		Hops:   clamp(a.Hops, 3, 1, 5),
		Lambda: clampF(a.Lambda, 0.7, 0, 1),
		Theta:  clampF(a.Theta, 0.1, 0, 1),
		Alpha:  clampF(a.Alpha, 0.5, 0, 1),
		Beta:   clampF(a.Beta, 0.5, 0, 1),
		FilterStructural: true,
	}
	return roam.Compute(s.g, s.p, a.Q, opt)
}

// randomWalk 随机漫步（复用 roam.ComputeRandom，与 CLI/Web 同一管线）。
func (s *Server) randomWalk(raw json.RawMessage) *roam.Outcome {
	var a struct {
		Top       int     `json:"top"`
		Seed      int64   `json:"seed"`
		RandAlpha float64 `json:"rand_alpha"`
	}
	_ = json.Unmarshal(raw, &a)
	var rng *rand.Rand
	if a.Seed != 0 {
		rng = rand.New(rand.NewPCG(uint64(a.Seed), uint64(a.Seed)>>1^0x9E3779B97F4A7C15))
	} else {
		rng = rand.New(rand.NewPCG(uint64(time.Now().UnixNano()), 0x9E3779B97F4A7C15))
	}
	opt := roam.Options{Top: clamp(a.Top, 15, 1, 60), Hops: 3, Lambda: 0.7,
		Theta: 0.1, Alpha: 0.5, Beta: 0.5, FilterStructural: true}
	return roam.ComputeRandom(s.g, s.p, opt, roam.Roll{Rng: rng, Alpha: clampF(a.RandAlpha, 0.5, 0, 1)})
}

// relation 两节点关系（复用 graph.ComputeRelation，与 REST /api/relation 同一白盒输出）。
func (s *Server) relation(raw json.RawMessage) *graph.Relation {
	var m struct {
		From string `json:"from"`
		To   string `json:"to"`
	}
	_ = json.Unmarshal(raw, &m)
	if m.From == "" || m.To == "" {
		return nil
	}
	from := s.resolveID(m.From)
	to := s.resolveID(m.To)
	if from == "" || to == "" {
		return nil
	}
	return s.g.ComputeRelation(from, to, 0.7)
}

func (s *Server) resolveID(q string) string {
	ms := s.g.Resolve(q)
	if len(ms) == 0 {
		return ""
	}
	return ms[0].ID
}

// ---- 小工具 ----

func okResp(id json.RawMessage, result any) rpcResponse {
	return rpcResponse{JSONRPC: "2.0", ID: id, Result: result}
}
func errResp(id json.RawMessage, code int, msg string) rpcResponse {
	return rpcResponse{JSONRPC: "2.0", ID: id, Error: &rpcError{Code: code, Message: msg}}
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
