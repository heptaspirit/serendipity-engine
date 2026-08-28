package main

// CLI 三件套测试（v0.1.11，backlog §五）：
//   parseArgs 识别 -h/--help/裸布尔/位置参数；
//   usageFor 各子命令有专属帮助文本（非空 + 含子命令使用样式）。
// 退出码与 --json 通过真实二进制行为验证（go test 不便 mock os.Exit，
// 见 CLI 专项手动验证，不在此单测）。

import (
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// capture 把 fn 写入 os.Stdout 的内容捕获到 builder（测试 usageFor 等纯打印函数）。
func capture(b *strings.Builder, fn func()) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	fn()
	w.Close()
	os.Stdout = old
	if _, err := io.Copy(b, r); err != nil {
		b.WriteString(err.Error())
	}
	r.Close()
}

// parseArgs：-h/--help 识别为 help 标志（单短横线 -h 不被当位置参数）。
func TestParseArgsHelp(t *testing.T) {
	pos, flags := parseArgs([]string{"vault", "-h"})
	if flags["help"] == "" {
		t.Fatalf("-h 应识别 help：flags=%v", flags)
	}
	if len(pos) != 1 || pos[0] != "vault" {
		t.Fatalf("-h 不应进位置参数：pos=%v", pos)
	}
	pos2, flags2 := parseArgs([]string{"--help"})
	if flags2["help"] == "" {
		t.Fatalf("--help 应识别：%v", flags2)
	}
	if len(pos2) != 0 {
		t.Fatalf("--help 不应进位置参数：%v", pos2)
	}
}

// parseArgs：--h=v / 裸布尔 / --k v 三种形态。
func TestParseArgsForms(t *testing.T) {
	pos, flags := parseArgs([]string{"vault", "q", "--top=5", "--random", "--hops", "3"})
	if len(pos) != 2 || pos[0] != "vault" || pos[1] != "q" {
		t.Fatalf("位置参数错误：%v", pos)
	}
	if flags["top"] != "5" || flags["random"] != "true" || flags["hops"] != "3" {
		t.Fatalf("flags 解析错误：%v", flags)
	}
}

// usageFor：每个子命令都有专属帮助文本（非空、含该子命令用法）。未知名 → 回退全局 usage。
func TestUsageForAllSubcommands(t *testing.T) {
	for _, cmd := range []string{"index", "roam", "serve", "refresh", "profile-detect", "mcp"} {
		var b strings.Builder
		capture(&b, func() { usageFor(cmd) })
		s := b.String()
		if s == "" {
			t.Fatalf("%s 帮助应为空", cmd)
		}
		if !strings.Contains(s, cmd) {
			t.Fatalf("%s 帮助应含子命令名：%q", cmd, s)
		}
	}
}

// writePIDFile：原子写入自身 PID,覆盖陈旧文件;目录不存在自动创建(v0.2.1 managed 句柄)。
func TestWritePIDFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "seren.pid")
	if err := writePIDFile(path); err != nil {
		t.Fatalf("writePIDFile: %v", err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read pid: %v", err)
	}
	if got := strings.TrimSpace(string(b)); got != strconv.Itoa(os.Getpid()) {
		t.Fatalf("pid mismatch: got %q want %d", got, os.Getpid())
	}
	// 覆盖陈旧内容(模拟 stale pid 文件)
	if err := os.WriteFile(path, []byte("999999\n"), 0o644); err != nil {
		t.Fatalf("seed stale: %v", err)
	}
	if err := writePIDFile(path); err != nil {
		t.Fatalf("rewrite pid: %v", err)
	}
	b2, _ := os.ReadFile(path)
	if got := strings.TrimSpace(string(b2)); got != strconv.Itoa(os.Getpid()) {
		t.Fatalf("rewrite pid mismatch: got %q", got)
	}
}
