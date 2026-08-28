// MCP stdio 服务测试（v0.2.x，mcp-go）：initialize / tools/list / tools/call /
// seren.state / 未配库引导 / 错误路径。走 ServeStdio（可注入 in/out）。
// 注意：mcp-go stdio 并发处理请求（响应乱序），故按 id 匹配而非按位置断言。
package mcp

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"serendipity-engine/internal/adapter"
	"serendipity-engine/internal/graph"
)

func buildGraph() *graph.Graph {
	// 8 文档：枢纽阈值 = Nodes/2 = 4。a 度 4 被剔除为枢纽；候选池 {b(3),c(2),d(2),e(1)}。
	// s1 结构类型 / s2 空标题 / f 孤立 → 被候选池过滤。
	docs := []*adapter.Document{
		{ID: "a", Title: "Alpha", Type: "note", Refs: []string{"b", "c"}},
		{ID: "b", Title: "Beta", Type: "note", Refs: []string{"c", "d"}},
		{ID: "c", Title: "Gamma", Type: "note", Refs: []string{"a"}},
		{ID: "d", Title: "Delta", Type: "note", Refs: []string{"e"}},
		{ID: "e", Title: "Epsilon", Type: "note", Refs: []string{"d"}},
		{ID: "f", Title: "孤独", Type: "note", Refs: []string{}},
		{ID: "s1", Title: "目录", Type: "toc", Refs: []string{"a"}},
		{ID: "s2", Title: "", Type: "note", Refs: []string{"a"}},
	}
	return graph.Build(docs)
}

func testServer(t *testing.T) *Server {
	t.Helper()
	p := &adapter.VaultProfile{StructuralTypes: []string{"toc"}}
	g := buildGraph()
	return New(func() (*graph.Graph, *adapter.VaultProfile) { return g, p }, "v0.2.0", "stdio")
}

type line struct {
	ID     json.RawMessage `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// callTool 单次调用：并发 stdio 下响应乱序且 mcp-go 对未注册工具不回复，
// 故每个工具单独发一次、单独收一次（串行，绝对确定）。返回该 id 的响应行。
func callTool(t *testing.T, srv *Server, req string) line {
	t.Helper()
	// 请求末尾补换行（mcp-go readNextLine 按 \n 切分，防丢最后一条）
	var out bytes.Buffer
	if err := srv.ServeStdio(strings.NewReader(req+"\n"), &out); err != nil {
		t.Fatalf("ServeStdio: %v", err)
	}
	var l line
	if err := json.Unmarshal(bytes.TrimSpace(out.Bytes()), &l); err != nil {
		t.Fatalf("响应非 JSON: %q (%v)", out.String(), err)
	}
	return l
}

// 主流程：初始化 / 列工具 / 各只读 tool / state / 错误路径。
func TestMCPLifecycle(t *testing.T) {
	srv := testServer(t)

	// initialize
	r := callTool(t, srv, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`)
	if r.Error != nil || r.Result == nil {
		t.Fatalf("initialize 应返回 result")
	}
	var init map[string]any
	json.Unmarshal(r.Result, &init)
	if init["serverInfo"].(map[string]any)["name"] != "serendipity-engine" {
		t.Fatalf("initialize serverInfo 错误: %v", init["serverInfo"])
	}

	// tools/list → 11 工具 + readOnlyHint + roam.q required
	r = callTool(t, srv, `{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`)
	var tl struct {
		Tools []struct {
			Name        string `json:"name"`
			Annotations struct {
				ReadOnlyHint bool `json:"readOnlyHint"`
			} `json:"annotations"`
			InputSchema struct {
				Required []string `json:"required"`
			} `json:"inputSchema"`
		} `json:"tools"`
	}
	json.Unmarshal(r.Result, &tl)
	if len(tl.Tools) != 11 {
		t.Fatalf("应有 11 个 tools, got %d", len(tl.Tools))
	}
	names := map[string]bool{}
	for _, x := range tl.Tools {
		names[x.Name] = true
		if !x.Annotations.ReadOnlyHint {
			t.Fatalf("tool %s 应 readOnlyHint=true", x.Name)
		}
		if x.Name == "graph.roam" {
			found := false
			for _, rq := range x.InputSchema.Required {
				if rq == "q" {
					found = true
				}
			}
			if !found {
				t.Fatalf("graph.roam 的 q 应 required")
			}
		}
	}
	for _, want := range []string{"graph.stats", "graph.roam", "graph.random", "graph.relation", "graph.node", "graph.similar", "graph.community", "graph.suggest", "seren.touch_digest", "seren.touch_stats", "seren.state"} {
		if !names[want] {
			t.Fatalf("tools 缺 %s", want)
		}
	}

	// graph.stats → nodes=8
	r = callTool(t, srv, `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"graph.stats","arguments":{}}}`)
	var stats struct{ Content []struct{ Text string } }
	json.Unmarshal(r.Result, &stats)
	var st graph.Stats
	json.Unmarshal([]byte(stats.Content[0].Text), &st)
	if st.Nodes != 8 {
		t.Fatalf("stats nodes 应为 8，got %d", st.Nodes)
	}

	// graph.roam → 锚定 a
	r = callTool(t, srv, `{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"graph.roam","arguments":{"q":"Alpha","top":5}}}`)
	var roam struct{ Content []struct{ Text string } }
	json.Unmarshal(r.Result, &roam)
	var ro struct {
		Anchors []struct{ ID, Title string }
	}
	json.Unmarshal([]byte(roam.Content[0].Text), &ro)
	if len(ro.Anchors) != 1 || ro.Anchors[0].ID != "a" {
		t.Fatalf("roam Alpha 应锚定 a, got %+v", ro.Anchors)
	}

	// graph.relation → 有路径
	r = callTool(t, srv, `{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"graph.relation","arguments":{"from":"Alpha","to":"Beta"}}}`)
	var rel struct{ Content []struct{ Text string } }
	json.Unmarshal(r.Result, &rel)
	var rl struct {
		Path []string `json:"path"`
	}
	json.Unmarshal([]byte(rel.Content[0].Text), &rl)
	if len(rl.Path) == 0 {
		t.Fatalf("relation 应有路径, got %+v", rl)
	}

	// graph.node → Alpha 邻居含 b,c
	r = callTool(t, srv, `{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"graph.node","arguments":{"id":"Alpha"}}}`)
	var nd struct{ Content []struct{ Text string } }
	json.Unmarshal(r.Result, &nd)
	var ndInfo struct {
		Title     string                `json:"title"`
		Neighbors []struct{ ID string } `json:"neighbors"`
	}
	json.Unmarshal([]byte(nd.Content[0].Text), &ndInfo)
	nbSet := map[string]bool{}
	for _, nb := range ndInfo.Neighbors {
		nbSet[nb.ID] = true
	}
	if ndInfo.Title != "Alpha" || !nbSet["b"] || !nbSet["c"] {
		t.Fatalf("node Alpha 详情错误: %+v", ndInfo)
	}

	// graph.similar → 共享邻居
	r = callTool(t, srv, `{"jsonrpc":"2.0","id":8,"method":"tools/call","params":{"name":"graph.similar","arguments":{"id":"Alpha","k":5}}}`)
	var sm struct{ Content []struct{ Text string } }
	json.Unmarshal(r.Result, &sm)
	var smList []struct {
		Shared []string `json:"shared"`
	}
	json.Unmarshal([]byte(sm.Content[0].Text), &smList)
	if len(smList) == 0 || len(smList[0].Shared) == 0 {
		t.Fatalf("similar 应带共享邻居: %+v", smList)
	}

	// graph.community → 社区
	r = callTool(t, srv, `{"jsonrpc":"2.0","id":9,"method":"tools/call","params":{"name":"graph.community","arguments":{"seed":42}}}`)
	var cm struct{ Content []struct{ Text string } }
	json.Unmarshal(r.Result, &cm)
	var cmRes struct {
		CommunityCount int `json:"community_count"`
	}
	json.Unmarshal([]byte(cm.Content[0].Text), &cmRes)
	if cmRes.CommunityCount < 1 {
		t.Fatalf("community 应返回有效社区: %+v", cmRes)
	}

	// graph.suggest → 潜在关联候选（带共享邻居证据 + 端点标题,反馈 #5）
	r = callTool(t, srv, `{"jsonrpc":"2.0","id":10,"method":"tools/call","params":{"name":"graph.suggest","arguments":{"k":5}}}`)
	var sg struct{ Content []struct{ Text string } }
	json.Unmarshal(r.Result, &sg)
	var sgOut struct {
		Count   int `json:"count"`
		Results []struct {
			A          string   `json:"a"`
			B          string   `json:"b"`
			ATitle     string   `json:"a_title"`
			BTitle     string   `json:"b_title"`
			Algorithms []string `json:"algorithms"`
			Shared     []string `json:"shared"`
		} `json:"results"`
	}
	json.Unmarshal([]byte(sg.Content[0].Text), &sgOut)
	if len(sgOut.Results) == 0 || len(sgOut.Results[0].Shared) == 0 {
		t.Fatalf("suggest 应返回带共享邻居的候选: %+v", sgOut)
	}
	if sgOut.Results[0].ATitle == "" || sgOut.Results[0].BTitle == "" {
		t.Fatalf("suggest 候选应带 a_title/b_title: %+v", sgOut.Results[0])
	}
	if len(sgOut.Results[0].Algorithms) == 0 {
		t.Fatalf("suggest 候选 algorithms 应为数组(反馈 #10 观察): %+v", sgOut.Results[0].Algorithms)
	}

	// seren.state → configured:true, tools=11
	r = callTool(t, srv, `{"jsonrpc":"2.0","id":11,"method":"tools/call","params":{"name":"seren.state","arguments":{}}}`)
	var stRes struct{ Content []struct{ Text string } }
	json.Unmarshal(r.Result, &stRes)
	var stateOut map[string]any
	json.Unmarshal([]byte(stRes.Content[0].Text), &stateOut)
	if stateOut["configured"] != true || stateOut["tools"].(float64) != 11 {
		t.Fatalf("state 应 configured=true, tools=11, got %+v", stateOut)
	}
}

// seren_orientation prompt（§3.8 Layer B）：prompts/list 出现 + prompts/get 返回英文说明。
func TestMCPPrompt(t *testing.T) {
	srv := testServer(t)

	// prompts/list → 含 seren_orientation
	r := callTool(t, srv, `{"jsonrpc":"2.0","id":1,"method":"prompts/list","params":{}}`)
	var pl struct {
		Prompts []struct{ Name, Description string }
	}
	json.Unmarshal(r.Result, &pl)
	found := false
	for _, p := range pl.Prompts {
		if p.Name == "seren_orientation" {
			found = true
			if p.Description == "" {
				t.Fatalf("seren_orientation 应带 description")
			}
		}
	}
	if !found {
		t.Fatalf("prompts 缺 seren_orientation: %+v", pl.Prompts)
	}

	// prompts/get → 返回英文 messages
	r2 := callTool(t, srv, `{"jsonrpc":"2.0","id":2,"method":"prompts/get","params":{"name":"seren_orientation"}}`)
	var pg struct {
		Messages []struct {
			Role    string `json:"role"`
			Content struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"messages"`
	}
	json.Unmarshal(r2.Result, &pg)
	if len(pg.Messages) == 0 {
		t.Fatalf("prompts/get 应返回 messages")
	}
	text := pg.Messages[0].Content.Text
	if text == "" {
		t.Fatalf("prompts/get 应返回非空 text")
	}
	if containsChinese(text) {
		t.Fatalf("seren_orientation prompt 应全英文，发现中文: %.80s", text)
	}
}

func containsChinese(s string) bool {
	for _, r := range s {
		if r >= 0x4e00 && r <= 0x9fff {
			return true
		}
	}
	return false
}

// 未配库：provider 返 nil → seren.state 报 configured:false + hint；其它工具回引导错误。
func TestMCPServerNotConfigured(t *testing.T) {
	srv := New(func() (*graph.Graph, *adapter.VaultProfile) { return nil, nil }, "v0.2.0", "streamable-http")

	// state → configured:false + hint
	r := callTool(t, srv, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"seren.state","arguments":{}}}`)
	var stRes struct{ Content []struct{ Text string } }
	json.Unmarshal(r.Result, &stRes)
	var out map[string]any
	json.Unmarshal([]byte(stRes.Content[0].Text), &out)
	if out["configured"] != false || out["hint"] == "" {
		t.Fatalf("未配库 state 应 configured=false + hint, got %+v", out)
	}

	// graph.stats → isError（引导错误）
	r = callTool(t, srv, `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"graph.stats","arguments":{}}}`)
	var er struct {
		IsError bool `json:"isError"`
	}
	json.Unmarshal(r.Result, &er)
	if !er.IsError {
		t.Fatalf("未配库调用 graph.stats 应 isError, got %+v", r.Result)
	}
}

// seren.touch_stats（v0.2.1 反馈 #1）→ 累计点击统计（total/targets/sources）。
func TestMCPServerTouchStats(t *testing.T) {
	srv := testServer(t)
	srv.SetTouchStats(func() (any, error) {
		return map[string]any{
			"total":   22,
			"targets": []map[string]any{{"id": "人物_015", "count": 5}},
			"sources": []map[string]any{{"id": "query", "count": 3}},
		}, nil
	})
	r := callTool(t, srv, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"seren.touch_stats","arguments":{}}}`)
	var ts struct{ Content []struct{ Text string } }
	json.Unmarshal(r.Result, &ts)
	var out struct {
		Total   int              `json:"total"`
		Targets []map[string]any `json:"targets"`
	}
	json.Unmarshal([]byte(ts.Content[0].Text), &out)
	if out.Total != 22 || len(out.Targets) != 1 || out.Targets[0]["id"] != "人物_015" {
		t.Fatalf("touch_stats 应返回累计统计: %+v", out)
	}
}
