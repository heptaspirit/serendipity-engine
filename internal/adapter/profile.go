package adapter

// ============================================================================
// 文件：internal/adapter/profile.go
// 模块：VaultProfile（库画像）—— 解析规则的"语义映射层"配置
//
// ▍职责
//   通用 Obsidian 语法（[[...]] 链接、markdown 链接、frontmatter、H1）由代码固定
//   （obsidian.go）——这些是四海皆准的；但"语义映射规则"（title 键、别名键、标签键、
//   排除目录、类型推断）因人而异——不同人的库约定不同 → 改 YAML，不改代码。
//   对应设计 §6.8 配置分层"vault 级 config.yaml（跟库走）"与 §6.9 VaultProfile 实测。
//
// ▍画像字段与 OKF（Open Knowledge Format）通用格式（v0.1.1 起）
//   Google OKF v0.1 = "markdown 目录 + YAML frontmatter" 的可移植知识格式，
//   frontmatter 只约定六个可查询字段：type / title / description / resource /
//   tags / timestamp。默认画像（default-obsidian）据此内置：
//     type_field = "type"（frontmatter 的 type 值即节点类型）
//     description_keys = ["description"]、resource_keys = ["resource"]
//     （并入正文，全文检索可命中结构化元数据）
//   `--profile-name okf` 与 default-obsidian 等价（OKF 是通用格式的超集约定）。
//   index.md / log.md 是 OKF 保留文件名（导航/变更历史）——是否结构类型化由各库
//   画像自定（structural_types），默认不排除：真实库里 index.md 可能是正文页面。
//
// ▍修改记录
//   v0.1.0  初版：title/alias/tag 键 + 类型规则 + 排除目录 + 默认画像。
//   v0.1.1  OKF 落地：新增 type_field / description_keys / resource_keys；
//           okf 内置画像别名。
// ============================================================================

import (
	"embed"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

//go:embed profiles/*.yaml
var profileFS embed.FS

// VaultProfile 描述一个库的解析约定（"库画像"）。
//
// 设计定位：通用 Obsidian 语法（[[...]] 链接、frontmatter 块、H1）由代码固定——
// 这些是四海皆准的。本结构只声明"语义映射规则"（title 键、别名键、标签键、
// 排除目录、类型推断），不同人的库约定不同 → 改 YAML，不改代码。
// 对应设计 §6.8 配置分层"vault 级 config.yaml（跟库走）"。
type VaultProfile struct {
	Name            string     `yaml:"name"`
	ExcludedDirs    []string   `yaml:"excluded_dirs"`
	TitleKeys       []string   `yaml:"title_keys"` // frontmatter 键优先级；兜底: 任意 *_name 键 > H1 > 文件名
	AliasKeys       []string   `yaml:"alias_keys"`
	TagKeys         []string   `yaml:"tag_keys"`
	TypeByDir       []TypeRule `yaml:"type_by_dir"`      // 相对路径目录前缀 → 类型
	TypeByKey       []TypeRule `yaml:"type_by_key"`      // frontmatter 键前缀 → 类型
	TypeByPrefix    []TypeRule `yaml:"type_by_prefix"`   // 文件名前缀 → 类型
	TypeByExt       []TypeRule `yaml:"type_by_ext"`      // 文件名包含 → 类型（如 .excalidraw）
	TypeField       string     `yaml:"type_field"`       // frontmatter 键，其值 = 节点类型（OKF `type`，默认 "type"）
	DescriptionKeys []string   `yaml:"description_keys"` // frontmatter 键，并入正文（OKF `description`）
	ResourceKeys    []string   `yaml:"resource_keys"`    // frontmatter 键，并入正文（OKF `resource`）
	DefaultType     string     `yaml:"default_type"`
	StructuralTypes []string   `yaml:"structural_types"` // 结构/机器节点类型：实体查询默认从簇输出排除
}

// TypeRule 一条类型推断规则。
type TypeRule struct {
	Pattern string `yaml:"pattern"`
	Type    string `yaml:"type"`
}

// DefaultObsidianProfile 通用默认画像：只认 Obsidian 最普适的约定 + OKF 通用格式。
// OKF 落地（v0.1.1）：type 字段作节点类型；description/resource 并入正文；md 链接
// 由通用语法层支持（obsidian.go）；index/log 不默认结构类型化（真实库可能是正文）。
func DefaultObsidianProfile() *VaultProfile {
	return &VaultProfile{
		Name:            "default-obsidian",
		ExcludedDirs:    []string{".obsidian", ".trash", ".git"},
		TitleKeys:       []string{"title"},
		AliasKeys:       []string{"aliases"},
		TagKeys:         []string{"tags"},
		TypeField:       "type",
		DescriptionKeys: []string{"description"},
		ResourceKeys:    []string{"resource"},
		DefaultType:     "note",
	}
}

// ProfileByName 按名字取内置画像。
// 内置画像 = 内嵌 YAML（profiles/*.yaml，单一事实源）；default / okf 走代码默认。
func ProfileByName(name string) (*VaultProfile, bool) {
	switch name {
	case "default-obsidian", "default", "okf":
		return DefaultObsidianProfile(), true
	}
	b, err := profileFS.ReadFile("profiles/" + name + ".yaml")
	if err != nil {
		return nil, false
	}
	p := &VaultProfile{}
	if err := yaml.Unmarshal(b, p); err != nil {
		return nil, false
	}
	return p, true
}

// LoadProfile 从 YAML 文件加载画像；缺失字段用通用默认补齐（防御性校验，设计 §6.8）。
func LoadProfile(path string) (*VaultProfile, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	p := &VaultProfile{}
	if err := yaml.Unmarshal(b, p); err != nil {
		return nil, err
	}
	d := DefaultObsidianProfile()
	if p.Name == "" {
		p.Name = d.Name
	}
	if len(p.ExcludedDirs) == 0 {
		p.ExcludedDirs = d.ExcludedDirs
	}
	if len(p.TitleKeys) == 0 {
		p.TitleKeys = d.TitleKeys
	}
	if len(p.AliasKeys) == 0 {
		p.AliasKeys = d.AliasKeys
	}
	if len(p.TagKeys) == 0 {
		p.TagKeys = d.TagKeys
	}
	if p.TypeField == "" {
		p.TypeField = d.TypeField
	}
	if len(p.DescriptionKeys) == 0 {
		p.DescriptionKeys = d.DescriptionKeys
	}
	if len(p.ResourceKeys) == 0 {
		p.ResourceKeys = d.ResourceKeys
	}
	if p.DefaultType == "" {
		p.DefaultType = d.DefaultType
	}
	return p, nil
}

// ResolveProfile 按 CLI 参数 + vault 本地约定取画像：
// 显式文件 > 显式名字 > <vault>/.serendipity/profile.yaml > 通用默认。
func ResolveProfile(file, name, vault string) (*VaultProfile, error) {
	if file != "" {
		return LoadProfile(file)
	}
	if name != "" {
		p, ok := ProfileByName(name)
		if !ok {
			return nil, os.ErrNotExist
		}
		return p, nil
	}
	if vault != "" {
		p, err := LoadProfile(strings.TrimRight(vault, `/\`) + "/.serendipity/profile.yaml")
		if err == nil {
			return p, nil
		}
	}
	return DefaultObsidianProfile(), nil
}

// SaveProfile 把画像写为 YAML。
func SaveProfile(path string, p *VaultProfile) error {
	b, err := yaml.Marshal(p)
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

// MarshalProfile 序列化为 YAML 文本。
func MarshalProfile(p *VaultProfile) (string, error) {
	b, err := yaml.Marshal(p)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
