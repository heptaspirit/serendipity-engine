package adapter

// 画像模板 + 自动落模板（v0.2.1）：ObsidianProfileTemplate 应为"空"模板（全注释 → 回落
// default-obsidian）；ensureObsidianProfileTemplate 创建但不覆盖已有；ResolveProfile 在
// per-vault 画像缺失时自动落模板。
import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestObsidianProfileTemplateIsEmpty(t *testing.T) {
	p := &VaultProfile{}
	if err := yaml.Unmarshal([]byte(ObsidianProfileTemplate()), p); err != nil {
		t.Fatalf("模板应可解析: %v", err)
	}
	fillDefaults(p)
	if p.DefaultType != "note" {
		t.Fatalf("全注释模板应回落 default-obsidian, got default_type=%q", p.DefaultType)
	}
	if len(p.ExcludedPrefixes) != 0 || len(p.ExcludedFiles) != 0 {
		t.Fatalf("模板应无激活排除规则: prefixes=%v files=%v", p.ExcludedPrefixes, p.ExcludedFiles)
	}
	if len(p.TitleKeys) == 0 {
		t.Fatalf("title_keys 应默认填充")
	}
}

func TestResolveProfileBootstrapsTemplate(t *testing.T) {
	vault := t.TempDir()
	p, err := ResolveProfile("", "", vault)
	if err != nil {
		t.Fatal(err)
	}
	if p.DefaultType != "note" {
		t.Fatalf("未配画像应回落 default-obsidian, got %q", p.DefaultType)
	}
	path := filepath.Join(vault, ".serendipity", "profile.yaml")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("应自动落模板到 %s: %v", path, err)
	}
}

func TestEnsureObsidianProfileTemplateNoOverwrite(t *testing.T) {
	vault := t.TempDir()
	if err := ensureObsidianProfileTemplate(vault); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(vault, ".serendipity", "profile.yaml")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("模板应被创建: %v", err)
	}
	// 用户改过 → 不应覆盖
	orig := "name: custom\n"
	if err := os.WriteFile(path, []byte(orig), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ensureObsidianProfileTemplate(vault); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	if string(b) != orig {
		t.Fatalf("不应覆盖已有配置: %q", string(b))
	}
}
