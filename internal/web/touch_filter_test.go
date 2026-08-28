package web

// 结构性文件 touch 跳过（v0.2.1，反馈：log/index 等工具文件 touch 权重异常大）：
// handleTouch 在记录前对照画像排除（文件名精确 / 前缀）丢弃。
import (
	"bytes"
	"net/http"
	"testing"

	"serendipity-engine/internal/adapter"
)

func TestTouchSkipsStructuralFiles(t *testing.T) {
	s := contractServer(t, "")
	// 画像排除：文件名精确 log + 前缀 .ingest-report-；touch 记录器统计被调用次数
	s.P = &adapter.VaultProfile{
		ExcludedFiles:    []string{"log"},
		ExcludedPrefixes: []string{".ingest-report-"},
	}
	calls := map[string]int{}
	s.Touch = func(target, from string) error { calls[target]++; return nil }
	ts := newAuthServer(t, s)
	defer ts.Close()

	post := func(target string) {
		req, _ := http.NewRequest("POST", ts.URL+"/api/touch", bytes.NewBufferString(`{"target":"`+target+`","from":"x"}`))
		req.Header.Set(tokenHeader, testToken)
		req.Header.Set("Content-Type", "application/json")
		_, _ = http.DefaultClient.Do(req)
	}

	post("log")                  // excluded_files 精确（basename）→ 不记录
	post("log.md")               // 带 .md 同命中 → 不记录
	post(".ingest-report-CH020") // 前缀命中 → 不记录
	post("人物_012")               // 非排除 → 记录
	post("index")                // 不在排除列表 → 记录（说明非排除的照常）

	if calls["log"] != 0 || calls["log.md"] != 0 || calls[".ingest-report-CH020"] != 0 {
		t.Fatalf("结构性文件不应记录: %v", calls)
	}
	if calls["人物_012"] != 1 || calls["index"] != 1 {
		t.Fatalf("非排除文件应记录: %v", calls)
	}
}
