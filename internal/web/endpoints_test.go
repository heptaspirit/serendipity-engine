// 新增端点测试（v0.1.11）：/api/similar、/api/node、/api/roam?export=1、/api/touch/stats。
package web

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"serendipity-engine/internal/adapter"
	"serendipity-engine/internal/graph"
)

func newAuthServer(t *testing.T, s *Server) *httptest.Server {
	t.Helper()
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	return ts
}

func doAuthGet(t *testing.T, ts *httptest.Server, path string) *http.Response {
	t.Helper()
	req, err := http.NewRequest("GET", ts.URL+path, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set(tokenHeader, testToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	return resp
}

func readAll(body io.Reader, sb *strings.Builder) {
	_, _ = io.Copy(sb, body)
}

// 构造：a-b-c 链 + 一个独立节点 x。
// a 与 c 共享 b（a-b、c-b）→ similar 应出 c；b 是中间桥。
func endpointGraph() *graph.Graph {
	docs := []*adapter.Document{
		{ID: "a", Title: "Alpha", Type: "人物", Refs: []string{"b"}},
		{ID: "b", Title: "Beta", Type: "人物", Refs: []string{"a", "c"}},
		{ID: "c", Title: "Gamma", Type: "人物", Refs: []string{"b"}},
		{ID: "x", Title: "独立", Type: "其他", Refs: []string{}},
	}
	return graph.Build(docs)
}

// GET /api/similar?id=&k=：结构相似节点 + 共享邻居证据。
func TestSimilarEndpoint(t *testing.T) {
	s := New(endpointGraph(), &adapter.VaultProfile{}, "test", "V", "v0.1.11", nil, nil)
	s.Token = testToken
	ts := newAuthServer(t, s)
	resp := doAuthGet(t, ts, "/api/similar?id=Alpha&k=10")
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("similar 应 200, got %d", resp.StatusCode)
	}
	var out struct {
		ID      string `json:"id"`
		Results []struct {
			ID           string   `json:"id"`
			SharedTitles []string `json:"shared_titles"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.ID != "a" {
		t.Fatalf("应锚定 a, got %s", out.ID)
	}
	found := false
	for _, r := range out.Results {
		if r.ID == "c" {
			found = true
			if len(r.SharedTitles) == 0 || r.SharedTitles[0] != "Beta" {
				t.Fatalf("c 应带共享邻居 Beta: %v", r.SharedTitles)
			}
		}
	}
	if !found {
		t.Fatalf("Alpha 应相似 Gamma(c), got %+v", out.Results)
	}
	// 缺失 id → error
	resp2 := doAuthGet(t, ts, "/api/similar")
	defer resp2.Body.Close()
	if resp2.StatusCode != 200 {
		t.Fatalf("missing id 应 200（返回 error json）")
	}
}

// GET /api/node?id=：节点详情（L0 摘要 + L1 邻居/被引用）。
func TestNodeEndpoint(t *testing.T) {
	s := New(endpointGraph(), &adapter.VaultProfile{}, "test", "V", "v0.1.11", nil, nil)
	s.Token = testToken
	ts := newAuthServer(t, s)
	resp := doAuthGet(t, ts, "/api/node?id=Beta")
	defer resp.Body.Close()
	var d struct {
		ID        string                `json:"id"`
		Title     string                `json:"title"`
		Neighbors []struct{ ID string } `json:"neighbors"`
		Backlinks []struct{ ID string } `json:"backlinks"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&d); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if d.Title != "Beta" || len(d.Neighbors) != 2 {
		t.Fatalf("Beta 详情错误: %+v", d)
	}
	// Beta 被 a、c 引用（backlinks）
	bl := map[string]bool{}
	for _, b := range d.Backlinks {
		bl[b.ID] = true
	}
	if !bl["a"] || !bl["c"] {
		t.Fatalf("Beta backlinks 应含 a,c: %+v", d.Backlinks)
	}
}

// GET /api/roam?q=&export=1 → text/markdown 卡片清单。
func TestRoamExportEndpoint(t *testing.T) {
	s := New(endpointGraph(), &adapter.VaultProfile{}, "test", "V", "v0.1.11", nil, nil)
	s.Token = testToken
	ts := newAuthServer(t, s)
	resp := doAuthGet(t, ts, "/api/roam?q=Alpha&export=1")
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("export 应 200, got %d", resp.StatusCode)
	}
	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "text/markdown") {
		t.Fatalf("export Content-Type 应 markdown, got %s", ct)
	}
	var sb strings.Builder
	readAll(resp.Body, &sb)
	body := sb.String()
	if !strings.Contains(body, "# Serendipity 漫游导出") {
		t.Fatalf("导出应含标题: %s", body)
	}
	if !strings.Contains(body, "Alpha") {
		t.Fatalf("导出应含查询词/结果: %s", body)
	}
	// 导出不追加 touch（只读）——无副作用，默认路径不受影响
}

// GET /api/touch/stats：埋点只读统计（注入 TouchStat 闭包）。
func TestTouchStatsEndpoint(t *testing.T) {
	s := New(endpointGraph(), &adapter.VaultProfile{}, "test", "V", "v0.1.11", nil, nil)
	s.Token = testToken
	s.SetTouchStats(func() (int, []TouchRow, []TouchRow, error) {
		return 5, []TouchRow{{ID: "热点", Count: 3}}, []TouchRow{{ID: "来源", Count: 2}}, nil
	})
	ts := newAuthServer(t, s)
	resp := doAuthGet(t, ts, "/api/touch/stats")
	defer resp.Body.Close()
	var out struct {
		Total   int        `json:"total"`
		Targets []TouchRow `json:"targets"`
		Sources []TouchRow `json:"sources"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Total != 5 || len(out.Targets) != 1 || out.Targets[0].Count != 3 {
		t.Fatalf("touch stats 错误: %+v", out)
	}
}

// GET /api/communities：社区发现（v0.1.12，roadmap #10）。
// a-b-c 链 + 孤立 x；Leiden 应产出 ≥1 社区，membership 含 a/b/c、不含孤立 x。
func TestCommunitiesEndpoint(t *testing.T) {
	s := New(endpointGraph(), &adapter.VaultProfile{}, "test", "V", "v0.1.12", nil, nil)
	s.Token = testToken
	ts := newAuthServer(t, s)
	resp := doAuthGet(t, ts, "/api/communities?seed=42")
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("communities 应 200, got %d", resp.StatusCode)
	}
	var out struct {
		CommunityCount int            `json:"community_count"`
		Membership     map[string]int `json:"membership"`
		Communities    []struct {
			Size int `json:"size"`
		} `json:"communities"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.CommunityCount < 1 {
		t.Fatalf("应有 ≥1 社区: %+v", out)
	}
	if _, ok := out.Membership["a"]; !ok {
		t.Fatalf("membership 应含 a: %+v", out.Membership)
	}
	if _, ok := out.Membership["x"]; ok {
		t.Fatalf("孤立 x 不应进 membership: %+v", out.Membership)
	}
	if len(out.Communities) != out.CommunityCount {
		t.Fatalf("Communities 数应 = CommunityCount: %+v", out)
	}
}
