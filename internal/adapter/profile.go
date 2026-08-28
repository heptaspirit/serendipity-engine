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
//   v0.1.3  默认 structural_types 加 container（虎鲸空壳容器排除）。
// ============================================================================

import (
	"embed"
	"os"
	"path/filepath"
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
	Name             string     `yaml:"name"`
	ExcludedDirs     []string   `yaml:"excluded_dirs"`
	ExcludedFiles    []string   `yaml:"excluded_files"`    // v0.1.12：文件名级排除（LLM Wiki 的 index.md/log.md）
	ExcludedPrefixes []string   `yaml:"excluded_prefixes"` // v0.1.13：文件名前缀排除（.ingest-report-、health_ 等自动生成/工具文件）
	TitleKeys        []string   `yaml:"title_keys"`        // frontmatter 键优先级；兜底: 任意 *_name 键 > H1 > 文件名
	AliasKeys        []string   `yaml:"alias_keys"`
	TagKeys          []string   `yaml:"tag_keys"`
	TypeByDir        []TypeRule `yaml:"type_by_dir"`      // 相对路径目录前缀 → 类型
	TypeByKey        []TypeRule `yaml:"type_by_key"`      // frontmatter 键前缀 → 类型
	TypeByPrefix     []TypeRule `yaml:"type_by_prefix"`   // 文件名前缀 → 类型
	TypeByExt        []TypeRule `yaml:"type_by_ext"`      // 文件名包含 → 类型（如 .excalidraw）
	TypeField        string     `yaml:"type_field"`       // frontmatter 键，其值 = 节点类型（OKF `type`，默认 "type"）
	DescriptionKeys  []string   `yaml:"description_keys"` // frontmatter 键，并入正文（OKF `description`）
	ResourceKeys     []string   `yaml:"resource_keys"`    // frontmatter 键，并入正文（OKF `resource`）
	DefaultType      string     `yaml:"default_type"`
	StructuralTypes  []string   `yaml:"structural_types"` // 结构/机器节点类型：实体查询默认从簇输出排除
}

// TypeRule 一条类型推断规则。
type TypeRule struct {
	Pattern string `yaml:"pattern"`
	Type    string `yaml:"type"`
}

// DefaultObsidianProfile 通用默认画像：只认 Obsidian 最普适的约定 + OKF 通用格式。
// OKF 落地（v0.1.1）：type 字段作节点类型；description/resource 并入正文；md 链接
// 由通用语法层支持（obsidian.go）；index/log 不默认结构类型化（真实库可能是正文）。
// v0.1.3：structural_types 含 container——虎鲸 adapter 把"空壳容器"（无内容但有
// 结构引用的嵌套页面宿主）标为 container，实体漫游/hot 默认排除；Obsidian 库无
// container 类型故不受影响。
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
		StructuralTypes: []string{"container"},
	}
}

// ProfileByName 按名字取内置画像。
// 内置画像 = 内嵌 YAML（profiles/*.yaml，单一事实源）；default / okf 走代码默认。
// v0.1.12：其余内置画像用 default-obsidian 的默认填充缺省字段（继承），
// 使 llm-wiki 等画像只声明覆盖项（excluded_dirs/excluded_files）即可。
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
	fillDefaults(p)
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
	fillDefaults(p)
	return p, nil
}

// ExcludedName 判断文件名是否被画像排除：ExcludedFiles 精确匹配，或 ExcludedPrefixes
// 前缀匹配（v0.1.13，反馈 #3——.ingest-report-、health_ 等自动生成文件不进图）。
//
// v0.2.1 bugfix：对 .md 后缀免疫——画像里写 "log" 或 "log.md" 都应排除 log.md。
// 此前 ExcludedName 只做全名精确匹配，裸名（排除 log.md 但写 `excluded_files: [log]`）
// 匹配不上 "log.md"，解析器实际不排除（曾导致日志/索引页照进图）。
func (p *VaultProfile) ExcludedName(name string) bool {
	if containsStr(p.ExcludedFiles, name) {
		return true
	}
	// 裸名/大小写不敏感匹配：画像写 "log" 排除 "log.md"/"LOG.md"（Obsidian 常跑在
	// 大小写不敏感的 Windows 文件系统）。Windows 下 "Log.md" 与 "log.md" 是同一文件。
	stem := strings.TrimSuffix(strings.TrimSuffix(name, ".md"), ".MD")
	for _, f := range p.ExcludedFiles {
		if strings.EqualFold(stem, f) || strings.EqualFold(name, f) {
			return true
		}
	}
	for _, pre := range p.ExcludedPrefixes {
		if pre != "" && (strings.HasPrefix(name, pre) || strings.HasPrefix(stem, pre)) {
			return true
		}
	}
	return false
}

// fillDefaults 用 default-obsidian 的默认填充画像中缺失的字段（防御性校验）。
func fillDefaults(p *VaultProfile) {
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
}

// ResolveProfile 按 CLI 参数 + vault 本地约定取画像：
// 显式文件 > 显式名字 > <vault>/.serendipity/profile.yaml > 通用默认。
// v0.2.1：既无显式文件/名字、per-vault 画像又缺失时，若 vault 是 Obsidian 目录，先落一个
// 全注释的模板（见 ObsidianProfileTemplate）到 <vault>/.serendipity/profile.yaml——
// 让用户/agent 有个明确的配置起点，引擎行为仍回落 default-obsidian。best-effort 不阻断。
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
		trimmed := strings.TrimRight(vault, `/\`)
		p, err := LoadProfile(trimmed + "/.serendipity/profile.yaml")
		if err == nil {
			return p, nil
		}
		_ = ensureObsidianProfileTemplate(trimmed) // onboarding 模板，best-effort
	}
	return DefaultObsidianProfile(), nil
}

// ObsidianProfileTemplate 返回一个"空"模板 YAML：所有规则都被注释（不生效），
// 用户按需求取消注释启用。v0.2.1——Obsidian 库 onboarding 的默认配置起点。
// 全注释的 YAML 反序列化为空 VaultProfile，fillDefaults 后即通用 default-obsidian 行为。
func ObsidianProfileTemplate() string {
	return `# ============================================================================
# Serendipity Engine · 库画像（VaultProfile）
# 本文件由引擎自动生成（v0.2.1）——默认"空"模板：下方规则全部被注释，不生效；
# 引擎会回落到通用 default-obsidian（title=title 键、type=type 字段、排除 .obsidian/.trash/.git）。
# 按你的库需要取消注释、改值即可；保存后下次 serve/roam/mcp 自动读取。
# 重新生成：删除本文件，引擎下次运行会重建本模板。
# ============================================================================
#
# ---- 基础 ----
# name: llm-wiki                     # 库画像名（标识用）
#
# ---- 扫描排除（这些文件/目录不进图）----
# excluded_dirs:
#   - raw
#   - audit
#   - output
#   - outputs
# excluded_files:
#   - index.md
#   - log.md
#   - CLAUDE.md
#   - AGENTS.md
# excluded_prefixes:                 # 文件名前缀级排除（.ingest-report- / health_ 等自动生成文件）
#   - ".ingest-report-"
#   - "health_"
#
# ---- 语义映射（frontmatter → 节点字段）----
# title_keys: [title, name]
# alias_keys: [aliases]
# tag_keys: [tags]
#
# ---- 类型推断 ----
# type_by_dir:
#   - {pattern: "wiki", type: 概念}
# type_by_prefix:
#   - {pattern: "人物_", type: 人物}
# default_type: note
#
# ---- 结构类型（实体查询默认排除；目录/机器节点）----
# structural_types:
#   - container
`
}

// ensureObsidianProfileTemplate 若 vault 是 Obsidian 目录且 <vault>/.serendipity/profile.yaml
// 不存在，则写入一个空模板。best-effort；虎鲸 .db / 非目录 / 已存在 → 直接返回。
func ensureObsidianProfileTemplate(vault string) error {
	if vault == "" || IsOrcaDB(vault) {
		return nil
	}
	fi, err := os.Stat(vault)
	if err != nil || !fi.IsDir() {
		return nil
	}
	dir := filepath.Join(vault, ".serendipity")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(dir, "profile.yaml")
	if _, err := os.Stat(path); err == nil {
		return nil // 已存在，不覆盖用户配置
	}
	return os.WriteFile(path, []byte(ObsidianProfileTemplate()), 0o644)
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
