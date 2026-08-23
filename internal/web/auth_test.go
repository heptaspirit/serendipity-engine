// 安全前置测试（v0.1.8）：Host 校验 + token 鉴权 + 页面注入。
package web

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"serendipity-engine/internal/adapter"
	"serendipity-engine/internal/graph"
)

const testToken = "test-token-123"

func authTestServer(t *testing.T) *Server {
	t.Helper()
	docs := []*adapter.Document{
		{ID: "a", Title: "Alpha", Type: "note", Refs: []string{"b"}},
		{ID: "b", Title: "Beta", Type: "note", Refs: []string{"a"}},
	}
	s := New(graph.Build(docs), &adapter.VaultProfile{}, "test", "TestVault", "v0.1.8", nil, nil)
	s.Token = testToken
	return s
}

// 页面本身不需要 token，且注入的 HTML 包含真实 token（前端 fetch 包装的种子）。
func TestAuthPageInjectsToken(t *testing.T) {
	s := authTestServer(t)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()
	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET / 应 200, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), testToken) {
		t.Fatalf("页面应注入真实 token")
	}
	if strings.Contains(string(body), "__SEREN_TOKEN__") {
		t.Fatalf("占位符应被替换")
	}
}

// API 需要 token：无 token / 错 token → 403；Header 与 ?token= 均可用。
func TestAuthAPINeedsToken(t *testing.T) {
	s := authTestServer(t)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	// 无 token
	resp, _ := http.Get(ts.URL + "/api/stats")
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("无 token 应 403, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// 错 token
	req, _ := http.NewRequest("GET", ts.URL+"/api/stats", nil)
	req.Header.Set(tokenHeader, "wrong-token")
	resp, _ = http.DefaultClient.Do(req)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("错 token 应 403, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Header 正确
	req, _ = http.NewRequest("GET", ts.URL+"/api/stats", nil)
	req.Header.Set(tokenHeader, testToken)
	resp, _ = http.DefaultClient.Do(req)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("正确 token 应 200, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// ?token= 查询参数
	resp, _ = http.Get(ts.URL + "/api/stats?token=" + testToken)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("?token= 应 200, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

// 非回环 Host → 403（即便 token 正确）：防 DNS rebinding / Host 欺骗。
func TestAuthBadHost(t *testing.T) {
	s := authTestServer(t)
	h := s.Handler()

	req := httptest.NewRequest("GET", "/api/stats", nil)
	req.Host = "evil.example.com"
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("恶意 Host 应 403, got %d", rr.Code)
	}

	// 带正确 token 也一样拒
	req2 := httptest.NewRequest("GET", "/api/stats", nil)
	req2.Host = "evil.example.com"
	req2.Header.Set(tokenHeader, testToken)
	rr2 := httptest.NewRecorder()
	h.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusForbidden {
		t.Fatalf("恶意 Host + 正确 token 也应 403, got %d", rr2.Code)
	}
}

// 回环 Host 形式白名单：127.0.0.1 / localhost / ::1（带不带端口）。
func TestIsLoopbackHost(t *testing.T) {
	ok := []string{
		"127.0.0.1", "127.0.0.1:8910",
		"localhost", "localhost:8080",
		"::1", "[::1]:8910",
		"0:0:0:0:0:0:0:1",
	}
	for _, h := range ok {
		if !isLoopbackHost(h) {
			t.Fatalf("%s 应判为回环", h)
		}
	}
	bad := []string{"evil.example.com", "192.168.1.5:8080", "10.0.0.1", "127.0.0.2:80"}
	for _, h := range bad {
		if isLoopbackHost(h) {
			t.Fatalf("%s 不应判为回环", h)
		}
	}
}
