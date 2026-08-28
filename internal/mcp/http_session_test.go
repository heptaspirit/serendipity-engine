package mcp

// HTTP 会话集成测试（v0.2.1，反馈 #9）：Streamable HTTP /mcp 端点的会话处理。
// .NET HttpClient 客户端带 Mcp-Session-Id 时曾报 "Invalid session ID"（404），而 curl
// 同流程正常——本测试用 httptest 复现 .NET 客户端流程（initialize → 带 session id
// 的 notifications/tools/call），确保服务端不因会话校验拒绝。
import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// doPost 向 /mcp 发一个 POST，返回 (status, response header, body)。
func doPost(t *testing.T, ts *httptest.Server, sessionID string, body string) (int, http.Header, string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, ts.URL+"/mcp", bytes.NewBufferString(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if sessionID != "" {
		req.Header.Set("Mcp-Session-Id", sessionID)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, resp.Header, string(b)
}

// TestStreamableHTTPSessionNetClient 复现 .NET 客户端会话流程。
func TestStreamableHTTPSessionNetClient(t *testing.T) {
	srv := testServer(t) // graph 8 节点, MCP 十工具
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	// 1) initialize → 应 200 且返回 Mcp-Session-Id
	initBody := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"net-client","version":"0.0.1"}}}`
	status, hdr, body := doPost(t, ts, "", initBody)
	if status != http.StatusOK {
		t.Fatalf("initialize 应 200, got %d: %s", status, body)
	}
	sid := hdr.Get("Mcp-Session-Id")
	if sid == "" {
		t.Fatalf("initialize 响应应带 Mcp-Session-Id")
	}

	// 2) notifications/initialized（带 session id）→ 应 202 不报错
	status, _, body = doPost(t, ts, sid, `{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	if status != http.StatusAccepted && status != http.StatusOK {
		t.Fatalf("notifications/initialized 应 202/200, got %d: %s", status, body)
	}

	// 3) tools/call（带 session id）→ 应成功
	status, _, body = doPost(t, ts, sid, `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"seren.state","arguments":{}}}`)
	if strings.Contains(body, "Invalid session ID") || status == http.StatusNotFound {
		t.Fatalf("tools/call 不应被会话校验拒绝, got status=%d body=%s", status, body)
	}
	if status != http.StatusOK {
		t.Fatalf("tools/call 应 200, got %d: %s", status, body)
	}

	var r struct {
		Result json.RawMessage           `json:"result"`
		Error  *struct{ Message string } `json:"error"`
	}
	// 流式/JSON 两种可能；这里只要求非 error
	if err := json.Unmarshal([]byte(body), &r); err != nil {
		t.Fatalf("tools/call 响应非 JSON: %v (%s)", err, body)
	}
	if r.Error != nil {
		t.Fatalf("tools/call 应无 error, got %s", r.Error.Message)
	}
}

// TestStreamableHTTPSessionNoSessionID 客户端带 session id 但未走 initialize、或回传
// 一个对方不认识的 id，都不应被服务端硬性拒绝（只读工具无会话内状态）——v0.2.1 反馈 #9。
func TestStreamableHTTPSessionNoSessionID(t *testing.T) {
	srv := testServer(t)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	for _, sid := range []string{"", "bogus-session-id"} {
		status, _, body := doPost(t, ts, sid, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"seren.state","arguments":{}}}`)
		if strings.Contains(body, "Invalid session ID") || status == http.StatusNotFound {
			t.Fatalf("不应因会话校验拒绝: sid=%q status=%d body=%s", sid, status, body)
		}
	}
	// 无 session id 也应能正常处理（200 + 结果）
	status, _, body := doPost(t, ts, "", `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"seren.state","arguments":{}}}`)
	if status != http.StatusOK {
		t.Fatalf("无 session id tools/call 应 200, got %d: %s", status, body)
	}
}
