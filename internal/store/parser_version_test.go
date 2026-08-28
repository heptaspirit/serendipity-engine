package store

// parser 版本读写（v0.2.1）：SaveParserVersion/LoadParserVersion roundtrip；
// 文件不存在时应返回 ""（不创建文件，避免版本检查的副作用）。
import (
	"os"
	"path/filepath"
	"testing"
)

func TestParserVersionRoundtrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "db.bbolt")
	if got := LoadParserVersion(p); got != "" {
		t.Fatalf("不存在文件应返回空, got %q", got)
	}
	if err := SaveParserVersion(p, "v0.2.1"); err != nil {
		t.Fatal(err)
	}
	if got := LoadParserVersion(p); got != "v0.2.1" {
		t.Fatalf("roundtrip 失败: got %q", got)
	}
}

// 版本检查不应在文件不存在时创建文件（refreshParse/parseSource 会先查版本）。
func TestParserVersionNoCreateOnMissing(t *testing.T) {
	p := filepath.Join(t.TempDir(), "never.bbolt")
	_ = LoadParserVersion(p)
	_ = LoadProfileSignature(p)
	if _, err := os.Stat(p); err == nil {
		t.Fatalf("LoadParserVersion/LoadProfileSignature 不应创建文件")
	}
}

func TestProfileSignatureRoundtrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "db.bbolt")
	if got := LoadProfileSignature(p); got != "" {
		t.Fatalf("不存在文件应返回空, got %q", got)
	}
	if err := SaveProfileSignature(p, "sig-v1"); err != nil {
		t.Fatal(err)
	}
	if got := LoadProfileSignature(p); got != "sig-v1" {
		t.Fatalf("profile 签名 roundtrip 失败: got %q", got)
	}
}
