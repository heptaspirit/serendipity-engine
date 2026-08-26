// // #8 前端 JSON 契约测试（v0.1.12）：验证 /api/* 端点返回的 JSON 形状与
// api-contract.md 一致——前端据此渲染，契约漂移即测试失败。
// 覆盖全部只读端点 + 手动刷新 + 埋点上报。每个端点断言响应能解码 + 必含契约字段。
package web

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"serendipity-engine/internal/adapter"
	"serendipity-engine/internal/graph"
	syncpkg "serendipity-engine/internal/sync"
)

// contractServer 构造带全端点（含 refresh/touch/touchStat）的契约测试服务。
func contractServer(t *testing.T, storePath string) *Server {
	t.Helper()
	s := New(endpointGraph(), &adapter.VaultProfile{}, "obsidian:test", "V", "v0.1.12", nil, nil)
	s.Token = testToken
	s.OrcaRepo = ""
	s.SetTouchStats(func() (int, []TouchRow, []TouchRow, error) {
		return 7, []TouchRow{{ID: "b", Count: 4}}, []TouchRow{{ID: "Alpha", Count: 2}}, nil
	})
	s.SetTouchDigest(func() (*Digest, error) {
		return &Digest{
			ID: "d1", GeneratedAt: 1700000000, WindowStart: 1699990000,
			Since: "2026-08-25 10:00", Total: 7,
			Targets: []DigestTarget{{ID: "b", Title: "Beta", Count: 4}},
			Sources: []TouchRow{{ID: "Alpha", Count: 2}},
		}, nil
	})
	s.SetTouchAck(func(id string) error { return nil })
	s.SetDigestAvailable(func() bool { return true })
	s.SetIsPending(func() bool { return true })
	// refresh 闭包：返回一个合成 diff + 同一图（端点只验证 JSON 形状，不做真刷新）
	s.Refresh = func() (*syncpkg.Result, *graph.Graph, error) {
		res := &syncpkg.Result{Added: 1, Updated: 2, Deleted: 0, Renamed: 1, Unchanged: 1, DurationMS: 3}
		res.Changes = []syncpkg.Change{{ID: "n1", Title: "新节点", Kind: syncpkg.KindAdded, Type: "note"}}
		res.Renames = []syncpkg.Rename{{OldID: "old", NewID: "new", Title: "改名", Type: "note"}}
		return res, s.G, nil
	}
	return s
}

// decodeUntilEOF 把整个响应体读成任意 JSON（map[string]any / []any）。
func decodeRaw(t *testing.T, body io.Reader) any {
	t.Helper()
	var v any
	if err := json.NewDecoder(body).Decode(&v); err != nil {
		t.Fatalf("响应非 JSON: %v", err)
	}
	return v
}

func mustKeys(t *testing.T, m map[string]any, keys ...string) {
	t.Helper()
	for _, k := range keys {
		if _, ok := m[k]; !ok {
			t.Fatalf("契约缺字段 %s：%v", k, m)
		}
	}
}

// TestEndpointJSONContract 走全端点并校验契约字段。
func TestEndpointJSONContract(t *testing.T) {
	s := contractServer(t, "")
	ts := newAuthServer(t, s)

	cases := []struct {
		method, path string
		keys         []string
	}{
		{"GET", "/api/stats", []string{"configured", "nodes", "edges", "version", "revision", "is_pending", "digest_available", "dangling", "dangling_refs"}},
		{"GET", "/api/config", []string{"params", "source", "vault", "version", "nodes", "edges"}},
		{"GET", "/api/roam?q=Alpha", []string{"query", "source", "vault", "anchors", "results", "fallback", "fallback_hits"}},
		{"GET", "/api/roam?random=1&seed=42", []string{"query", "source", "vault", "anchors", "results", "fallback", "fallback_hits"}},
		{"GET", "/api/relation?from=Alpha&to=Beta", []string{"path", "path_nodes", "affinity"}},
		{"GET", "/api/similar?id=Alpha", []string{"id", "results"}},
		{"GET", "/api/suggest-links?k=10", []string{"count", "results"}},
		{"GET", "/api/node?id=Beta", []string{"id", "title", "type", "text", "deg", "neighbors", "backlinks"}},
		{"GET", "/api/communities?seed=42", []string{"modularity", "community_count", "membership", "communities"}},
		{"GET", "/api/hot?n=10", nil},
	}
	for _, c := range cases {
		resp := doAuthGet(t, ts, c.path)
		if resp.StatusCode != 200 {
			t.Fatalf("%s 应 200, got %d", c.path, resp.StatusCode)
		}
		body := decodeRaw(t, resp.Body)
		resp.Body.Close()
		if m, ok := body.(map[string]any); ok {
			mustKeys(t, m, c.keys...)
		} else if c.keys != nil { // /api/hot 是数组，keys 应为 nil
			t.Fatalf("%s 应为对象：%v", c.path, body)
		}
	}
}

// POST /api/refresh 契约：diff 摘要 + 明细 + 改名。
func TestRefreshEndpointJSONContract(t *testing.T) {
	s := contractServer(t, "")
	ts := newAuthServer(t, s)
	req, _ := http.NewRequest("POST", ts.URL+"/api/refresh", nil)
	req.Header.Set(tokenHeader, testToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	defer resp.Body.Close()
	body := decodeRaw(t, resp.Body)
	m, ok := body.(map[string]any)
	if !ok {
		t.Fatalf("refresh 应为对象：%v", body)
	}
	mustKeys(t, m, "added", "updated", "deleted", "renamed", "unchanged", "duration_ms", "nodes", "changes", "renames")
}

// POST /api/touch 契约：埋点上报成功（写失败静默，仍返回 ok）。
func TestTouchEndpointJSONContract(t *testing.T) {
	s := contractServer(t, "")
	s.Touch = func(target, from string) error { return nil }
	ts := newAuthServer(t, s)
	req, _ := http.NewRequest("POST", ts.URL+"/api/touch", bytes.NewBufferString(`{"target":"a","from":"Alpha"}`))
	req.Header.Set(tokenHeader, testToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("touch: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("touch 应 200, got %d", resp.StatusCode)
	}
	body := decodeRaw(t, resp.Body)
	if m, ok := body.(map[string]any); !ok || m["ok"] != "true" {
		t.Fatalf("touch 应返回 ok: %v", body)
	}
}

// /api/stats 的 is_pending：注入 true 时应反射在响应里（roadmap #14）。
func TestStatsIsPendingReflected(t *testing.T) {
	s := contractServer(t, "")
	ts := newAuthServer(t, s)
	resp := doAuthGet(t, ts, "/api/stats")
	defer resp.Body.Close()
	body := decodeRaw(t, resp.Body)
	m := body.(map[string]any)
	if m["is_pending"] != true {
		t.Fatalf("is_pending 应为 true: %v", m)
	}
	if m["digest_available"] != true {
		t.Fatalf("digest_available 应为 true: %v", m)
	}
}

// GET /api/touch/digest 契约：digest 内容 + available（§3.7）。
func TestTouchDigestEndpointJSONContract(t *testing.T) {
	s := contractServer(t, "")
	ts := newAuthServer(t, s)
	resp := doAuthGet(t, ts, "/api/touch/digest")
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("digest 应 200, got %d", resp.StatusCode)
	}
	body := decodeRaw(t, resp.Body)
	m, ok := body.(map[string]any)
	if !ok {
		t.Fatalf("digest 应为对象：%v", body)
	}
	mustKeys(t, m, "digest", "available")
	dm, ok := m["digest"].(map[string]any)
	if !ok {
		t.Fatalf("digest.digest 应为对象：%v", m["digest"])
	}
	mustKeys(t, dm, "id", "generated_at", "window_start", "since", "total", "targets", "sources")
}

// POST /api/touch/digest/ack 契约：{id} → ok（§3.7）。
func TestTouchDigestAckEndpointJSONContract(t *testing.T) {
	s := contractServer(t, "")
	ts := newAuthServer(t, s)
	req, _ := http.NewRequest("POST", ts.URL+"/api/touch/digest/ack", bytes.NewBufferString(`{"id":"d1"}`))
	req.Header.Set(tokenHeader, testToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("ack: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("ack 应 200, got %d", resp.StatusCode)
	}
	body := decodeRaw(t, resp.Body)
	if m, ok := body.(map[string]any); !ok || m["ok"] != "true" {
		t.Fatalf("ack 应返回 ok: %v", body)
	}
}

// GET /api/vault 契约（v0.1.15 无库启动）：configured/source/vault。
func TestVaultGetEndpointJSONContract(t *testing.T) {
	s := contractServer(t, "")
	ts := newAuthServer(t, s)
	resp := doAuthGet(t, ts, "/api/vault")
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("vault GET 应 200, got %d", resp.StatusCode)
	}
	m, ok := decodeRaw(t, resp.Body).(map[string]any)
	if !ok {
		t.Fatalf("vault GET 应为对象：%v", m)
	}
	mustKeys(t, m, "configured", "source", "vault")
	if m["configured"] != true {
		t.Fatalf("已配库的 contractServer configured 应为 true: %v", m)
	}
}

// POST /api/vault 契约（v0.1.15）：未注入 Vault 闭包 → 不可用；
// 注入后 {path} → ok + configured（换图 + 全套闭包替换生效）。
func TestVaultPostEndpointJSONContract(t *testing.T) {
	s := contractServer(t, "")
	ts := newAuthServer(t, s)

	// 未注入 Vault 闭包：vault config unavailable
	req, _ := http.NewRequest("POST", ts.URL+"/api/vault", bytes.NewBufferString(`{"path":"/x"}`))
	req.Header.Set(tokenHeader, testToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("vault POST: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("vault POST 不可用时也应 200+error, got %d", resp.StatusCode)
	}

	// 注入 Vault 闭包：返回新图 + 全套闭包 → 应用后 configured=true
	newGraph := endpointGraph() // 复用同一合成图，验证状态应用路径
	applied := 0
	s.SetVault(func(path string, opts VaultOpts) (*VaultState, error) {
		return &VaultState{
			G: newGraph, P: &adapter.VaultProfile{},
			Source: "obsidian:" + path, VaultName: "NewVault",
			Refresh: s.Refresh, Touch: func(t, f string) error { return nil },
			IsPending: func() bool { return false },
		}, nil
	})
	// 配库成功后应用回调（模拟 main 的 watch 重启钩子）
	s.OnVaultApplied = func() { applied++ }

	req, _ = http.NewRequest("POST", ts.URL+"/api/vault", bytes.NewBufferString(`{"path":"/new/vault"}`))
	req.Header.Set(tokenHeader, testToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("vault POST: %v", err)
	}
	defer resp.Body.Close()
	m, ok := decodeRaw(t, resp.Body).(map[string]any)
	if !ok {
		t.Fatalf("vault POST 应为对象：%v", m)
	}
	mustKeys(t, m, "ok", "configured", "source", "vault", "nodes", "edges")
	if m["configured"] != true || m["source"] != "obsidian:/new/vault" || m["vault"] != "NewVault" {
		t.Fatalf("vault 应用后状态不符: %v", m)
	}
	if applied != 1 {
		t.Fatalf("OnVaultApplied 应触发 1 次, got %d", applied)
	}

	// 应用后 /api/stats configured=true 且 Source 已换
	resp2 := doAuthGet(t, ts, "/api/stats")
	defer resp2.Body.Close()
	m2, ok := decodeRaw(t, resp2.Body).(map[string]any)
	if !ok {
		t.Fatalf("stats 应为对象：%v", m2)
	}
	if m2["configured"] != true {
		t.Fatalf("配库后 stats.configured 应为 true: %v", m2)
	}
}

// 未配库（G==nil）时数据端点返回 configured:false（v0.1.15 无库启动守卫）。
func TestVaultStateUnconfigured(t *testing.T) {
	s := New(nil, nil, "", "", "v0.1.15", nil, nil) // 空库启动：G==nil
	s.Token = testToken
	ts := newAuthServer(t, s)

	resp := doAuthGet(t, ts, "/api/stats")
	defer resp.Body.Close()
	m, ok := decodeRaw(t, resp.Body).(map[string]any)
	if !ok {
		t.Fatalf("stats 应为对象：%v", m)
	}
	if m["configured"] != false {
		t.Fatalf("空库 stats.configured 应为 false: %v", m)
	}
	mustKeys(t, m, "version")

	// 数据端点统一 503 形态：{error, configured:false}
	resp2 := doAuthGet(t, ts, "/api/roam?q=x")
	defer resp2.Body.Close()
	m2, ok := decodeRaw(t, resp2.Body).(map[string]any)
	if !ok {
		t.Fatalf("roam 应为对象：%v", m2)
	}
	if m2["configured"] != false {
		t.Fatalf("空库 roam 应返回 configured:false: %v", m2)
	}

	// 未配置服务端点（touch/refresh）→ unavailable 错误而非 panic
	req, _ := http.NewRequest("POST", ts.URL+"/api/refresh", nil)
	req.Header.Set(tokenHeader, testToken)
	resp3, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	m3, _ := decodeRaw(t, resp3.Body).(map[string]any)
	resp3.Body.Close()
	if m3 == nil || m3["error"] == "" {
		t.Fatalf("空库 refresh 应返回 error: %v", m3)
	}
}
