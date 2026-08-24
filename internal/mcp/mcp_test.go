// MCP stdio 服务测试（v0.1.9）：initialize / tools/list / tools/call / 错误路径。
package mcp

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"serendipity-engine/internal/adapter"
	"serendipity-engine/internal/graph"
)

func testServer(t *testing.T) *Server {
	t.Helper()
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
	p := &adapter.VaultProfile{StructuralTypes: []string{"toc"}}
	return New(graph.Build(docs), p, "v0.1.9")
}

type line struct {
	ID     json.RawMessage `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// runServe 把请求行喂给 Serve，返回响应行（每行一个响应）。
func runServe(t *testing.T, reqs string) []line {
	t.Helper()
	var out bytes.Buffer
	if err := testServer(t).Serve(strings.NewReader(reqs), &out); err != nil {
		t.Fatalf("Serve: %v", err)
	}
	var lines []line
	for _, ln := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		if ln == "" {
			continue
		}
		var l line
		if err := json.Unmarshal([]byte(ln), &l); err != nil {
			t.Fatalf("响应行非 JSON: %q (%v)", ln, err)
		}
		lines = append(lines, l)
	}
	return lines
}

// 主流程：初始化 / 列工具 / 七只读 tool / 错误路径 / 通知不响应。
func TestMCPLifecycle(t *testing.T) {
	reqs := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"graph.stats","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"graph.roam","arguments":{"q":"Alpha","top":5}}}`,
		`{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"graph.random","arguments":{"seed":42,"top":5}}}`,
		`{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"graph.relation","arguments":{"from":"Alpha","to":"Beta"}}}`,
		`{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"graph.node","arguments":{"id":"Alpha"}}}`,
		`{"jsonrpc":"2.0","id":8,"method":"tools/call","params":{"name":"graph.similar","arguments":{"id":"Alpha","k":5}}}`,
		`{"jsonrpc":"2.0","id":9,"method":"tools/call","params":{"name":"graph.community","arguments":{"seed":42}}}`,
		`{"jsonrpc":"2.0","id":10,"method":"tools/call","params":{"name":"graph.nope","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":11,"method":"nope","params":{}}`,
		`{"jsonrpc":"2.0","id":12,"method":"ping"}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized","params":{}}`, // 无 id → 不响应
		`not json`,
	}, "\n")
	resps := runServe(t, reqs)
	if len(resps) != 13 {
		t.Fatalf("应有 13 个响应（12 请求 + 1 解析错误；通知不响应），got %d", len(resps))
	}

	// id=1 initialize
	if resps[0].Error != nil || resps[0].Result == nil {
		t.Fatalf("initialize 应返回 result")
	}
	var init map[string]any
	json.Unmarshal(resps[0].Result, &init)
	if init["serverInfo"].(map[string]any)["name"] != "serendipity-engine" {
		t.Fatalf("initialize serverInfo 错误: %v", init["serverInfo"])
	}
	// 客户端未带 protocolVersion → 用兜底 2024-11-05
	if init["protocolVersion"] != "2024-11-05" {
		t.Fatalf("initialize 兜底 protocolVersion 应为 2024-11-05, got %v", init["protocolVersion"])
	}

	// id=2 tools/list → 7 个 tools，含 graph.random/node/similar/community
	var tl struct{ Tools []struct{ Name string } }
	json.Unmarshal(resps[1].Result, &tl)
	if len(tl.Tools) != 7 {
		t.Fatalf("应有 7 个 tools, got %d", len(tl.Tools))
	}
	names := map[string]bool{}
	for _, x := range tl.Tools {
		names[x.Name] = true
	}
	for _, want := range []string{"graph.stats", "graph.roam", "graph.random", "graph.relation", "graph.node", "graph.similar", "graph.community"} {
		if !names[want] {
			t.Fatalf("tools 缺 %s", want)
		}
	}

	// id=3 graph.stats → content 文本是含 nodes 的 JSON（全图 8 文档）
	var stats struct{ Content []struct{ Text string } }
	json.Unmarshal(resps[2].Result, &stats)
	if len(stats.Content) != 1 {
		t.Fatalf("stats content 应 1 项")
	}
	var st graph.Stats
	json.Unmarshal([]byte(stats.Content[0].Text), &st)
	if st.Nodes != 8 {
		t.Fatalf("stats nodes 应为 8（全图），got %d", st.Nodes)
	}

	// id=4 graph.roam → anchors + results
	var roam struct{ Content []struct{ Text string } }
	json.Unmarshal(resps[3].Result, &roam)
	var ro struct {
		Anchors []struct{ ID, Title string }
		Results []struct{ ID string }
	}
	json.Unmarshal([]byte(roam.Content[0].Text), &ro)
	if len(ro.Anchors) != 1 || ro.Anchors[0].ID != "a" {
		t.Fatalf("roam Alpha 应锚定 a, got %+v", ro.Anchors)
	}

	// id=5 graph.random → 锚点带 random 标记，同 seed 确定性
	var rnd struct{ Content []struct{ Text string } }
	json.Unmarshal(resps[4].Result, &rnd)
	var rd struct {
		Anchors []struct {
			ID     string `json:"id"`
			Random bool   `json:"random"`
		} `json:"anchors"`
	}
	json.Unmarshal([]byte(rnd.Content[0].Text), &rd)
	if len(rd.Anchors) != 1 || !rd.Anchors[0].Random {
		t.Fatalf("random 应恰一个随机起点锚点, got %+v", rd.Anchors)
	}
	// 再次调用同 seed → 同一起点（可复现）
	resps2 := runServe(t, `{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"graph.random","arguments":{"seed":42}}}`)
	var rd2 struct {
		Anchors []struct {
			ID string `json:"id"`
		} `json:"anchors"`
	}
	var c2 struct{ Content []struct{ Text string } }
	json.Unmarshal(resps2[0].Result, &c2)
	json.Unmarshal([]byte(c2.Content[0].Text), &rd2)
	if rd2.Anchors[0].ID != rd.Anchors[0].ID {
		t.Fatalf("同 seed 应同一起点: %s vs %s", rd2.Anchors[0].ID, rd.Anchors[0].ID)
	}

	// id=6 graph.relation → 路径
	var rel struct{ Content []struct{ Text string } }
	json.Unmarshal(resps[5].Result, &rel)
	var rl struct {
		Affinity float64  `json:"affinity"`
		Path     []string `json:"path"`
	}
	json.Unmarshal([]byte(rel.Content[0].Text), &rl)
	if len(rl.Path) == 0 {
		t.Fatalf("relation 应有路径, got %+v", rl)
	}

	// id=7 graph.node → 节点详情（L0 摘要 + L1 邻居/被引用）
	var nd struct{ Content []struct{ Text string } }
	json.Unmarshal(resps[6].Result, &nd)
	var ndInfo struct {
		ID        string                `json:"id"`
		Title     string                `json:"title"`
		Neighbors []struct{ ID string } `json:"neighbors"`
	}
	json.Unmarshal([]byte(nd.Content[0].Text), &ndInfo)
	if ndInfo.Title != "Alpha" || len(ndInfo.Neighbors) < 2 {
		t.Fatalf("node Alpha 详情错误: %+v", ndInfo)
	}
	// a 的邻居应含 b、c（a 链接它们 + 被引用它们的反边；s1/s2 也引用 a）
	nbSet := map[string]bool{}
	for _, nb := range ndInfo.Neighbors {
		nbSet[nb.ID] = true
	}
	if !nbSet["b"] || !nbSet["c"] {
		t.Fatalf("node Alpha 邻居应含 b,c: %+v", ndInfo.Neighbors)
	}

	// id=8 graph.similar → 相似节点带共享邻居
	var sm struct{ Content []struct{ Text string } }
	json.Unmarshal(resps[7].Result, &sm)
	var smList []struct {
		ID     string   `json:"id"`
		Shared []string `json:"shared"`
	}
	json.Unmarshal([]byte(sm.Content[0].Text), &smList)
	if len(smList) == 0 || len(smList[0].Shared) == 0 {
		t.Fatalf("similar 应带共享邻居: %+v", smList)
	}

	// id=9 graph.community → 社区结果（模块度 + 社区数 + membership）
	var cm struct{ Content []struct{ Text string } }
	json.Unmarshal(resps[8].Result, &cm)
	var cmRes struct {
		Modularity     float64           `json:"modularity"`
		CommunityCount int               `json:"community_count"`
		Membership     map[string]int    `json:"membership"`
		Communities    []graph.Community `json:"communities"`
	}
	json.Unmarshal([]byte(cm.Content[0].Text), &cmRes)
	if cmRes.CommunityCount < 1 || cmRes.Modularity == 0 && cmRes.CommunityCount == 0 {
		t.Fatalf("community 应返回有效社区: %+v", cmRes)
	}

	// id=10 未知工具 → error -32602
	if resps[9].Error == nil || resps[9].Error.Code != -32602 {
		t.Fatalf("未知工具应 -32602, got %+v", resps[9].Error)
	}
	// id=11 未知方法 → error -32601
	if resps[10].Error == nil || resps[10].Error.Code != -32601 {
		t.Fatalf("未知方法应 -32601, got %+v", resps[10].Error)
	}
	// id=12 ping → result {}
	if string(resps[11].Result) != "{}" {
		t.Fatalf("ping 应返回 {}, got %s", resps[11].Result)
	}
	// 解析错误 → -32700
	if resps[12].Error == nil || resps[12].Error.Code != -32700 {
		t.Fatalf("解析错误应 -32700, got %+v", resps[12].Error)
	}
}

// initialize 回显客户端请求的 protocolVersion —— 修复 SDK 客户端版本不匹配导致
// 断连→重连→反复 spawn（DSH 控制台反复刷横幅的根因）。
func TestInitializeEchoesProtocolVersion(t *testing.T) {
	resps := runServe(t, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"x","version":"1"}}}`)
	if len(resps) != 1 {
		t.Fatalf("应 1 个响应, got %d", len(resps))
	}
	var init map[string]any
	json.Unmarshal(resps[0].Result, &init)
	if init["protocolVersion"] != "2025-06-18" {
		t.Fatalf("应回显客户端 protocolVersion 2025-06-18, got %v", init["protocolVersion"])
	}
}
