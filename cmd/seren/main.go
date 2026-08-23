// ============================================================================
// 文件：cmd/seren/main.go
// 模块：Serendipity Engine CLI 入口（设计 §6.8 三入口之 CLI）
//
// ▍职责
//
//	把"解析 → 建图 → 漫游 → 持久化 → 对账刷新 → Web"串成一条命令行。子命令：
//	  index          解析 + 建图 + 统计 + 持久化
//	  roam <q>       查询漫游 → top-N 节点簇（CLI 版）
//	  refresh        对账刷新：重解析 + 与上次持久化状态 diff（增/删/改明细）
//	  serve          启动 Web UI（REST / JSON + 节点簇可视化 + /api/refresh）
//	  profile-detect 扫描 vault，产出解析画像 YAML（新库 onboarding）
//	  version        打印版本（与 git tag 同步）
//
// ▍数据源自动识别（parseSource，优先级从高到低）
//  1. --db <file.sqlite>   从持久化存储读图（跳过解析）
//  2. 虎鲸库（扩展名 .db） 先 CopyDBForRead 一致性快照（VACUUM INTO，含 WAL）再解析
//  3. Obsidian vault       按 VaultProfile 解析（见 adapter/obsidian.go、profile.go）
//
// ▍画像（VaultProfile）解析顺序（ResolveProfile）
//
//	显式 --profile 文件 > --profile-name 内置名（default-obsidian / okf /
//	example-wiki）> <vault>/.serendipity/profile.yaml（跟库走）> 通用默认。
//	OKF（Open Knowledge Format）通用格式由默认画像内置（type_field /
//	description_keys / resource_keys / markdown 链接），见 adapter/profile.go。
//
// ▍对账刷新（refresh / serve 的 /api/refresh，v0.1.2）
//
//	全量重解析 + sync.Diff 与上次持久化状态比对（按 ID 对齐，规范化比较字段），
//	报告 新增/更新/删除 明细并写回存储（幂等全量重写）。边际情况与语义
//	见 internal/sync/sync.go 文件头。刷新后 Web 内存图替换，hot/stats/roam 即新图。
//
// ▍安全说明
//
//	本命令只在用户本机运行，无网络出口；不读取也不输出任何凭据类数据；
//	虎鲸库 Repo 表（含 API key）从不解析（adapter/orca.go 红线）。
//
// ▍修改记录
//
//	v0.1.0  初版五子命令。
//	v0.1.1  --profile-name 新增 okf（OKF 通用画像别名）；版本号提升。
//	v0.1.2  新增 refresh 子命令 + serve 注入 /api/refresh 闭包（对账刷新）。
//	v0.1.3  虎鲸空壳页面清理（过滤 + container 类型化，见 adapter/orca.go）。
//	v0.1.4  自动监听（watch 轮询+节流，默认开）、反馈埋点（/api/touch，
//	        仅记录不演化）、虎鲸跳转（--repo → orca-note://）。
//	v0.1.5  改名迁移（修订 #8：Diff 识别 rename + Refs 重定向 + touch 迁移）、
//	        关系查询 /api/relation（权重+路径+证据，为 MCP 铺路）。
//	v0.1.6  打分桶内归一化（修复深跳 score=0 误导）；快照增量解析
//	        （ParseVaultIncremental：mtime/size 复用未变文件，只重解析变更）。
//	v0.1.7  随机漫步（roam --random）：随机 roll 起点 + 它的簇——"节点 + 簇"
//	        一次给出；roll 取舍：质量门槛过滤 + deg^α 加权 + 防重复 + 可复现种子。
//	v0.1.8  serve 安全前置（roadmap M0-0.1）：--token 指定 / 自动生成 + 页面注入；
//	        Host 校验（仅回环）+ API token 鉴权；README 徽章化美化。
//	v0.1.9  MCP server（第四个入口，roadmap M0-0.3）：seren mcp 子命令——stdio
//	        JSON-RPC 2.0 自实现薄协议（零第三方依赖），只读四件套 tools
//	        （stats/roam/random/relation）；只 import 纯库不碰 web/watch。
//
// ============================================================================
package main

import (
	"context"
	cryptorand "crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"math/rand/v2"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"serendipity-engine/internal/adapter"
	"serendipity-engine/internal/graph"
	"serendipity-engine/internal/mcp"
	"serendipity-engine/internal/roam"
	"serendipity-engine/internal/store"
	"serendipity-engine/internal/sync"
	"serendipity-engine/internal/watch"
	"serendipity-engine/internal/web"
)

// version 语义化版本号；发布时同步 git tag（README 徽章版本号也在此次同步）。
const version = "v0.1.9"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}
	switch os.Args[1] {
	case "index":
		cmdIndex(os.Args[2:])
	case "roam":
		cmdRoam(os.Args[2:])
	case "serve":
		cmdServe(os.Args[2:])
	case "refresh":
		cmdRefresh(os.Args[2:])
	case "profile-detect":
		cmdProfileDetect(os.Args[2:])
	case "mcp":
		cmdMCP(os.Args[2:])
	case "version", "--version", "-v":
		fmt.Printf("Serendipity Engine %s\n", version)
	case "help", "-h", "--help":
		usage()
	default:
		usage()
		os.Exit(1)
	}
}

func usage() {
	fmt.Printf(`Serendipity Engine %s
用法:
  seren index <vault|OrcaNote.db> [flags]  解析、建图、统计（.db 自动识别为虎鲸库）
  seren roam <vault|OrcaNote.db> [query] [flags]   查询漫游 → top-N 节点簇（--random 随机漫步）
  seren serve <vault|OrcaNote.db> [--port 8080]    Web UI（REST + 节点簇可视化 + 刷新）
  seren refresh <vault|OrcaNote.db> [flags] 对账刷新：重解析 + 与上次持久化状态 diff
  seren profile-detect <vault>          扫描 vault，提出解析画像 YAML（新库 onboarding）
  seren mcp <vault> [--db <store>]       MCP stdio server（只读四件套，AI 入口）
  seren version                         打印版本
flags:
  --top N        输出条数 (默认 15)
  --lambda X     激活衰减 (默认 0.7)
  --theta Y      激活剪枝阈值 (默认 0.1)
  --hops N       最大跳数 (默认 3)
  --alpha X      结构分权重 (默认 0.5)
  --beta Y       激活分权重 (默认 0.5)
随机漫步 (v0.1.7):
  --random       随机漫步：随机 roll 起点 + 它的簇（可省略 query）
  --seed N       随机种子（默认 0=随机；固定 N 可复现同一漫步，便于分享/测试）
  --rand-alpha X 随机起点度加权指数：0=均匀（惊喜），1=偏丰富簇（默认 0.5）
安全 (v0.1.8):
  --token <t>    serve API 鉴权 token（默认自动生成并打印；页面自动注入）
画像/存储:
  --profile <file.yaml>       显式画像文件
  --profile-name <name>       内置画像名 (default-obsidian / okf / example-wiki)
  --db <file.sqlite>          从持久化存储读图（跳过解析）
  --persist                   解析后持久化到库内 .serendipity/db-<hash>.sqlite
  --store <file.sqlite>       指定持久化路径（覆盖默认）
监听/跳转/埋点:
  --watch-off                 关闭自动监听（默认开：轮询变化→节流合并刷新）
  --watch-interval N          监听轮询间隔秒（默认 10）
  --watch-throttle N          刷新节流秒（默认 60，合并窗口防频繁重解析）
  --repo <name>               虎鲸 repo 名（orca-note:// 跳转；默认取库文件名）
`, version)
}

// parseArgs 解析 CLI 参数：位置参数 + --k=v / --k v 标志。
func parseArgs(args []string) (pos []string, flags map[string]string) {
	flags = map[string]string{}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if strings.HasPrefix(a, "--") {
			kv := strings.SplitN(a[2:], "=", 2)
			if len(kv) == 2 {
				flags[kv[0]] = kv[1]
			} else if i+1 < len(args) && !strings.HasPrefix(args[i+1], "--") {
				flags[kv[0]] = args[i+1]
				i++
			} else {
				// 裸布尔开关（如 --random / --watch-off）：此前被静默忽略
				flags[kv[0]] = "true"
			}
		} else {
			pos = append(pos, a)
		}
	}
	return pos, flags
}

// loadSource 统一加载：显式存储(--db) > 虎鲸库(.db 自动识别，先快照再读) > Obsidian vault。
// 返回 图 + 原始文档 + 源描述。图构建叠加改名重定向（store renames 表，
// v0.1.5 修订 #8：存储存文件真相，图层做身份迁移，见 redirectForGraph）。
func loadSource(vault string, p *adapter.VaultProfile, dbFile, storeFlag string) (*graph.Graph, []*adapter.Document, string) {
	docs, src, err := parseSource(vault, p, dbFile)
	if err != nil {
		fatal("%v", err)
	}
	// 改名映射来源：--db 时 renames 在同一个存储文件里；vault 解析时在
	// storePathFor 对应的默认存储里（无 → 空映射，全新构建无需迁移）。
	var renames map[string]string
	if dbFile != "" {
		renames, _ = store.LoadRenames(dbFile)
	} else {
		renames, _ = store.LoadRenames(storePathFor(vault, storeFlag))
	}
	return graph.Build(redirectForGraph(docs, renames)), docs, src
}

// redirectForGraph 返回 Refs 重定向后的文档副本（用于建图）。不改动原始 docs：
// 存储始终写原始 Refs（文件真相），对账 diff 才能收敛；身份迁移（改名）只
// 在图/展示层生效，由持久化的 renames 表驱动。
func redirectForGraph(docs []*adapter.Document, renames map[string]string) []*adapter.Document {
	out := make([]*adapter.Document, len(docs))
	for i, d := range docs {
		cp := *d
		cp.Refs = append([]string(nil), d.Refs...)
		out[i] = &cp
	}
	sync.ApplyRenames(out, renames)
	return out
}

// parseSource 统一加载（error 版，供 refresh 复用）：返回 Document 列表与源描述。
func parseSource(vault string, p *adapter.VaultProfile, dbFile string) ([]*adapter.Document, string, error) {
	if dbFile != "" {
		docs, err := store.Load(dbFile)
		if err != nil {
			return nil, "", fmt.Errorf("读存储失败: %w", err)
		}
		return docs, "store:" + dbFile, nil
	}
	if adapter.IsOrcaDB(vault) {
		cp, cleanup, err := adapter.CopyDBForRead(vault)
		if err != nil {
			return nil, "", fmt.Errorf("快照虎鲸库失败: %w", err)
		}
		defer cleanup()
		docs, err := adapter.ParseOrcaDB(cp)
		if err != nil {
			return nil, "", fmt.Errorf("解析虎鲸库失败: %w", err)
		}
		return docs, "orca:" + vault, nil
	}
	docs, err := adapter.ParseVault(vault, p)
	if err != nil {
		return nil, "", fmt.Errorf("解析失败: %w", err)
	}
	return docs, "obsidian:" + vault, nil
}

// refreshParse 刷新专用解析（v0.1.6 快照增量优化）：
//   Obsidian 源且已有旧状态 → ParseVaultIncremental（复用 mtime/size 未变文件，
//   只重解析变更/新增；返回 reused 计数供日志）；其余（--db 回读 / 虎鲸 /
//   首次全量）→ parseSource。语义与全量解析等价（见 adapter/obsidian.go）。
func refreshParse(vault string, p *adapter.VaultProfile, flags map[string]string, old []*adapter.Document) (docs []*adapter.Document, reused int, src string, err error) {
	if flags["db"] == "" && !adapter.IsOrcaDB(vault) && len(old) > 0 {
		docs, reused, err = adapter.ParseVaultIncremental(vault, p, old)
		return docs, reused, "obsidian:" + vault, err
	}
	docs, src, err = parseSource(vault, p, flags["db"])
	return docs, 0, src, err
}

// storePathFor 计算默认持久化路径（与 cmdIndex 一致：虎鲸库取库所在目录）。
func storePathFor(vault string, storeFlag string) string {
	if storeFlag != "" {
		return storeFlag
	}
	base := vault
	if adapter.IsOrcaDB(vault) {
		base = filepath.Dir(vault)
	}
	return store.DBPath(base)
}

// cmdRefresh 对账刷新：全量重解析 → 与上次持久化状态 diff → 输出明细 → 写回存储。
func cmdRefresh(args []string) {
	pos, flags := parseArgs(args)
	if len(pos) < 1 {
		fatal("用法: seren refresh <vault> [--store file]")
	}
	vault := pos[0]
	p, err := adapter.ResolveProfile(flags["profile"], flags["profile-name"], vault)
	if err != nil {
		fatal("画像加载失败: %v", err)
	}
	storePath := storePathFor(vault, flags["store"])

	old, err := store.Load(storePath)
	if err != nil {
		fatal("读旧状态失败: %v", err)
	}
	storedRenames, err := store.LoadRenames(storePath)
	if err != nil {
		fatal("读改名映射失败: %v", err)
	}
	docs, reused, src, err := refreshParse(vault, p, flags, old)
	if err != nil {
		fatal("%v", err)
	}
	res := sync.Diff(old, docs)
	// 改名迁移（v0.1.5，修订 #8）：持久化映射合并（含本次新检测）→ 存 renames
	// 表 + touch 迁移。documents 存原始 Refs（文件真相，diff 收敛）；
	// 重定向只在建图时叠加（见 refreshFunc/loadSource 的 redirectForGraph）。
	merged := sync.MergeRenames(storedRenames, renamesMap(res.Renames), docs)
	if err := store.SaveRenames(storePath, merged); err != nil {
		fatal("改名映射持久化失败: %v", err)
	}
	if err := store.RenameTouch(storePath, merged); err != nil {
		fatal("touch 迁移失败: %v", err)
	}
	if err := store.Save(storePath, docs); err != nil {
		fatal("持久化失败: %v", err)
	}

	fmt.Printf("source: %s\n", src)
	fmt.Printf("画像: %s\n", p.Name)
	fmt.Printf("store: %s\n", storePath)
	if flags["db"] == "" && !adapter.IsOrcaDB(vault) && len(old) > 0 {
		fmt.Printf("解析: %d 文档（快照增量：复用 %d / 重解析 %d）\n", len(docs), reused, len(docs)-reused)
	} else {
		fmt.Printf("解析: %d 文档（全量）\n", len(docs))
	}
	fmt.Printf("对账: 新增 %d / 更新 %d / 删除 %d / 改名 %d / 未变 %d  （耗时 %dms）\n",
		res.Added, res.Updated, res.Deleted, res.Renamed, res.Unchanged, res.DurationMS)
	limit := fint(flags, "top", 50)
	show := 0
	next := func() bool {
		if show >= limit {
			fmt.Printf("  … 其余 %d 条略（--top 调整）\n", len(res.Changes)+len(res.Renames)-show)
			return false
		}
		show++
		return true
	}
	for _, r := range res.Renames {
		if !next() {
			break
		}
		fmt.Printf("  ↦ 改名   %-10s → %-10s %s [%s]\n", r.OldID, r.NewID, r.Title, r.Type)
	}
	for _, c := range res.Changes {
		if !next() {
			break
		}
		switch c.Kind {
		case sync.KindAdded:
			fmt.Printf("  + 新增   %-10s %s [%s]\n", c.ID, c.Title, c.Type)
		case sync.KindDeleted:
			fmt.Printf("  - 删除   %-10s %s [%s]\n", c.ID, c.Title, c.Type)
		case sync.KindUpdated:
			fmt.Printf("  ~ 更新   %-10s %s [%s] 字段=%s", c.ID, c.Title, c.Type, strings.Join(c.Fields, ","))
			if len(c.AddedRefs) > 0 || len(c.RemovedRefs) > 0 {
				fmt.Printf(" 引用+%d/-%d", len(c.AddedRefs), len(c.RemovedRefs))
			}
			fmt.Println()
		}
	}
}

func cmdIndex(args []string) {
	pos, flags := parseArgs(args)
	if len(pos) < 1 {
		fatal("用法: seren index <vault>")
	}
	vault := pos[0]
	p, err := adapter.ResolveProfile(flags["profile"], flags["profile-name"], vault)
	if err != nil {
		fatal("画像加载失败: %v", err)
	}
	g, docs, src := loadSource(vault, p, flags["db"], flags["store"])
	fmt.Printf("vault: %s\n", vault)
	fmt.Printf("source: %s\n", src)
	fmt.Printf("画像: %s\n", p.Name)
	fmt.Printf("解析文档: %d\n", len(docs))

	typeCount := map[string]int{}
	for _, d := range docs {
		typeCount[d.Type]++
	}
	fmt.Println("类型分布:")
	var types []string
	for t := range typeCount {
		types = append(types, t)
	}
	sortStrings(types)
	for _, t := range types {
		fmt.Printf("  %-8s %d\n", t, typeCount[t])
	}

	s := g.Stats()
	fmt.Printf("图节点: %d\n", s.Nodes)
	fmt.Printf("链接账目: 总计 %d, 自环 %d, 去重无向边 %d, 重复链接 %d 条\n",
		s.TotalLinks, s.SelfLinks, s.Edges, s.MultiEdge)
	fmt.Printf("悬空链接: %d 种 / %d 条 (指向不存在的文件)\n", s.Dangling, s.DanglingLinks)
	fmt.Printf("孤儿节点: %d (无任何边)\n", s.Orphans)
	fmt.Printf("连通分量: %d\n", s.Components)
	fmt.Println("top 枢纽:")
	for _, h := range s.TopHubs {
		fmt.Printf("  %-28s deg=%-4d type=%s title=%s\n", h.ID, h.Deg, h.Type, h.Title)
	}

	// 持久化（设计 §6.8：SQLite 主存储，库内 .serendipity/db-<hash>.sqlite）
	if flags["persist"] != "" || flags["store"] != "" {
		base := vault
		if adapter.IsOrcaDB(vault) {
			base = filepath.Dir(vault)
		}
		dbPath := flags["store"]
		if dbPath == "" {
			dbPath = store.DBPath(base)
			if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
				fatal("创建 .serendipity 失败: %v", err)
			}
		}
		if err := store.Save(dbPath, docs); err != nil {
			fatal("持久化失败: %v", err)
		}
		fmt.Printf("已持久化: %s\n", dbPath)
	}
}

func cmdRoam(args []string) {
	pos, flags := parseArgs(args)
	random := flags["random"] != ""
	if len(pos) < 1 {
		fatal("用法: seren roam <vault> [query] [flags]（--random 随机漫步时可省略 query）")
	}
	vault := pos[0]
	var query string
	if len(pos) >= 2 {
		query = pos[1]
	}
	if !random && strings.TrimSpace(query) == "" {
		fatal("查询不能为空（或加 --random 随机漫步）")
	}
	top := fint(flags, "top", 15)
	lambda := ffloat(flags, "lambda", 0.7)
	theta := ffloat(flags, "theta", 0.1)
	hops := fint(flags, "hops", 3)
	alpha := ffloat(flags, "alpha", 0.5)
	beta := ffloat(flags, "beta", 0.5)
	seed := fint64(flags, "seed", 0)
	randAlpha := ffloat(flags, "rand-alpha", 0.5)

	p, err := adapter.ResolveProfile(flags["profile"], flags["profile-name"], vault)
	if err != nil {
		fatal("画像加载失败: %v", err)
	}
	g, _, src := loadSource(vault, p, flags["db"], flags["store"])
	opt := roam.Options{
		Top: top, Hops: hops, Lambda: lambda, Theta: theta,
		Alpha: alpha, Beta: beta, FilterStructural: true,
	}

	var out *roam.Outcome
	if random {
		// 随机漫步（v0.1.7）：seed=0 用时间随机；固定 seed 可复现（同一节点同一簇）
		var rng *rand.Rand
		if seed != 0 {
			rng = rand.New(rand.NewPCG(uint64(seed), uint64(seed)>>1^0x9E3779B97F4A7C15))
		} else {
			rng = rand.New(rand.NewPCG(uint64(time.Now().UnixNano()), 0x9E3779B97F4A7C15))
		}
		out = roam.ComputeRandom(g, p, opt, roam.Roll{Rng: rng, Alpha: randAlpha})
	} else {
		out = roam.Compute(g, p, query, opt)
	}

	fmt.Printf("source: %s\n", src)
	if random {
		fmt.Printf("mode: random-walk (🎲 随机起点 + 簇, rand-alpha=%.2f)\n", randAlpha)
	} else {
		fmt.Printf("query: %s\n", query)
	}
	fmt.Printf("画像: %s\n", p.Name)
	switch out.Fallback {
	case roam.ModeNoAnchor:
		fmt.Println("锚定失败（图内无精确ID/title/别名/标签/LIKE）→ 降级：全文检索")
		printTextHits(out.FallbackHits, top)
	case roam.ModeSparse:
		fmt.Println("--- 降级：查询点无/少链接 → 全文 LIKE 兜底（决策 #10）---")
		printTextHits(out.FallbackHits, top)
	default:
		fmt.Printf("anchor: ")
		for i, a := range out.Anchors {
			if i > 0 {
				fmt.Print(" ")
			}
			if a.Random {
				fmt.Print("🎲")
			} else {
				fmt.Printf("%s", a.ID)
			}
		}
		fmt.Println()
		for _, a := range out.Anchors {
			mark := ""
			if a.Random {
				mark = " 🎲 随机起点"
			}
			fmt.Printf("  -> %s (title=%s, type=%s)%s\n", a.ID, a.Title, a.Type, mark)
		}
		fmt.Printf("params: lambda=%.2f theta=%.2f hops=%d alpha=%.2f beta=%.2f top=%d\n",
			lambda, theta, hops, alpha, beta, top)
		fmt.Println("--- top 节点簇 ---")
		if len(out.Results) == 0 {
			fmt.Println("  (该随机节点没有可展示的关联簇——再试一次，或去掉 --random 用查询漫游)")
		}
		for i, r := range out.Results {
			fmt.Printf("%2d. %-28s %-12s %-6s score=%.3f ppr=%.4f act=%.3f %d-hop  %s\n",
				i+1, r.ID, r.Title, nodeType(g, r.ID), r.Score, r.PPR, r.Act, r.Hops, strings.Join(r.Path, " → "))
		}
	}
}

func cmdServe(args []string) {
	pos, flags := parseArgs(args)
	if len(pos) < 1 {
		fatal("用法: seren serve <vault> [--port 8080] [--vault-name 库名] [--repo 虎鲸库名]")
	}
	vault := pos[0]
	port := fint(flags, "port", 8080)
	p, err := adapter.ResolveProfile(flags["profile"], flags["profile-name"], vault)
	if err != nil {
		fatal("画像加载失败: %v", err)
	}
	g, docs, src := loadSource(vault, p, flags["db"], flags["store"])
	isOrca := adapter.IsOrcaDB(vault)
	if flags["db"] != "" {
		// 存储回读：按路径形态推断源（orca 节点 path = block/<id>）
		isOrca = false
		for _, d := range docs {
			if strings.HasPrefix(d.Path, "block/") {
				isOrca = true
				break
			}
		}
	}

	// vault 名（obsidian:// URI 跳转用）：显式 > 路径 basename
	vaultName := flags["vault-name"]
	if vaultName == "" && !isOrca {
		vaultName = filepath.Base(filepath.Clean(vault))
	}
	// 虎鲸 repo 名（orca-note:// URI 跳转用）：显式 --repo > 库文件名（去 .db）
	orcaRepo := ""
	if isOrca {
		orcaRepo = flags["repo"]
		if orcaRepo == "" {
			orcaRepo = strings.TrimSuffix(filepath.Base(vault), ".db")
		}
	}

	storePath := storePathFor(vault, flags["store"])
	refreshFn := refreshFunc(vault, p, flags)
	// 反馈埋点闭包（克制：仅记录，写 store touch 表；失败静默）
	touchFn := func(target, from string) error { return store.AppendTouch(storePath, target, from) }
	srv := web.New(g, p, src, vaultName, version, refreshFn, touchFn)
	srv.OrcaRepo = orcaRepo

	// API 鉴权（v0.1.8 安全前置）：--token 指定；否则自动生成 32 位 hex 并打印。
	// 前端页面由服务端注入 token（外部页面拿不到）；curl 用 X-Seren-Token 头或
	// ?token= 查询参数。重启后 token 变化 → 浏览器重新 GET / 即拿到新 token。
	token := flags["token"]
	if token == "" {
		buf := make([]byte, 16)
		if _, err := cryptorand.Read(buf); err != nil {
			fatal("token 生成失败: %v", err)
		}
		token = hex.EncodeToString(buf)
	}
	srv.Token = token

	// 自动监听（v0.1.4，克制设计见 internal/watch）：默认开启，--watch-off 关闭。
	// 轮询间隔 --watch-interval 秒（默认 10s）；刷新节流 --watch-throttle 秒
	// （默认 60s，合并窗口——连续编辑吸收为窗口内一次刷新）。
	if flags["watch-off"] == "" {
		interval := time.Duration(fint(flags, "watch-interval", 10)) * time.Second
		throttle := time.Duration(fint(flags, "watch-throttle", 60)) * time.Second
		var check func() (bool, error)
		if isOrca {
			check = watch.NewOrcaChecker(vault)
		} else {
			check = watch.NewVaultChecker(vault, p.ExcludedDirs)
		}
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go watch.Run(ctx, interval, throttle, check, func() error {
			res, ng, err := refreshFn()
			if err != nil {
				return err
			}
			srv.ReplaceGraph(ng)
			log.Printf("[watch] 自动刷新完成: 新增 %d / 更新 %d / 删除 %d / 改名 %d（revision=%d）",
				res.Added, res.Updated, res.Deleted, res.Renamed, srv.Revision())
			return nil
		})
		fmt.Printf("自动监听: 开（轮询 %v，刷新节流 %v；--watch-off 关闭）\n", interval, throttle)
	}

	addr := fmt.Sprintf("127.0.0.1:%d", port)
	fmt.Printf("Serendipity Engine %s Web UI: http://%s  (source: %s, 节点 %d)\n",
		version, addr, src, g.Stats().Nodes)
	fmt.Printf("API 鉴权: 开（token=%s；页面已自动注入，curl 用 X-Seren-Token 头或 ?token=；--token 可指定固定值）\n", token)
	switch {
	case orcaRepo != "":
		fmt.Printf("跳转: 虎鲸 repo=%s（卡片上点「打开」会跳到虎鲸对应块）\n", orcaRepo)
	case vaultName != "":
		fmt.Printf("跳转: Obsidian vault 名=%s（卡片上点「打开」会跳到笔记软件）\n", vaultName)
	}
	if n, err := store.TouchCount(storePath); err == nil && n > 0 {
		fmt.Printf("反馈埋点: 已记录 %d 次点击（仅记录不演化边权）\n", n)
	}
	if err := http.ListenAndServe(addr, srv.Handler()); err != nil {
		fatal("服务失败: %v", err)
	}
}

// refreshFunc 构造 Web 端的刷新闭包：重解析 → 对账 diff → 改名迁移（合并持久化
// 映射 + touch 迁移 + renames 落盘）→ 写回存储（原始 Refs）→ 返回 diff 结果与
// 刷新后的新图（建图叠加重定向）。Store 路径与 CLI refresh 一致。
func refreshFunc(vault string, p *adapter.VaultProfile, flags map[string]string) web.RefreshFunc {
	storePath := storePathFor(vault, flags["store"])
	return func() (*sync.Result, *graph.Graph, error) {
		old, err := store.Load(storePath)
		if err != nil {
			return nil, nil, fmt.Errorf("读旧状态失败: %w", err)
		}
		storedRenames, err := store.LoadRenames(storePath)
		if err != nil {
			return nil, nil, fmt.Errorf("读改名映射失败: %w", err)
		}
		docs, _, _, err := refreshParse(vault, p, flags, old)
		if err != nil {
			return nil, nil, err
		}
		res := sync.Diff(old, docs)
		// 改名迁移（v0.1.5，修订 #8）：见 cmdRefresh 同段注释
		merged := sync.MergeRenames(storedRenames, renamesMap(res.Renames), docs)
		if err := store.SaveRenames(storePath, merged); err != nil {
			return nil, nil, fmt.Errorf("改名映射持久化失败: %w", err)
		}
		if err := store.RenameTouch(storePath, merged); err != nil {
			return nil, nil, fmt.Errorf("touch 迁移失败: %w", err)
		}
		if err := store.Save(storePath, docs); err != nil {
			return nil, nil, fmt.Errorf("持久化失败: %w", err)
		}
		return res, graph.Build(redirectForGraph(docs, merged)), nil
	}
}

// renamesMap 从对账结果的改名明细构建 旧ID→新ID 映射（ApplyRenames /
// RenameTouch 的入参形态）。
func renamesMap(rs []sync.Rename) map[string]string {
	m := make(map[string]string, len(rs))
	for _, r := range rs {
		m[r.OldID] = r.NewID
	}
	return m
}

// printTextHits 打印全文检索命中（roam.Compute 已按模式过滤）。
func printTextHits(hits []graph.TextHit, top int) {
	shown := 0
	for _, h := range hits {
		fmt.Printf("%2d. %-28s %-12s %-6s 全文命中 %d 次\n", shown+1, h.ID, h.Title, h.Type, h.Count)
		shown++
		if shown >= top {
			break
		}
	}
	if shown == 0 {
		fmt.Println("  (全文也无命中——库中确实没有相关内容)")
	}
}

func nodeType(g *graph.Graph, id string) string {
	if n, ok := g.Node(id); ok {
		return n.Type()
	}
	return ""
}

func cmdProfileDetect(args []string) {
	pos, _ := parseArgs(args)
	if len(pos) < 1 {
		fatal("用法: seren profile-detect <vault>")
	}
	p, err := adapter.DetectProfile(pos[0])
	if err != nil {
		fatal("探测失败: %v", err)
	}
	out, err := adapter.MarshalProfile(p)
	if err != nil {
		fatal("序列化失败: %v", err)
	}
	fmt.Print(out)
}

// cmdMCP 启动 MCP stdio server（第四个入口，v0.1.9，roadmap M0-0.3）。
// 只读四件套 tools（stats/roam/random/relation）；只 import 纯库，不碰 web/watch。
// stdout 只承载 JSON-RPC 协议；启动提示一律写 stderr（避免污染协议流）。
func cmdMCP(args []string) {
	pos, flags := parseArgs(args)
	if len(pos) < 1 {
		fatal("用法: seren mcp <vault> [--db <store.sqlite>]（库来源同 roam；--db 读持久化存储免重解析）")
	}
	vault := pos[0]
	p, err := adapter.ResolveProfile(flags["profile"], flags["profile-name"], vault)
	if err != nil {
		fatal("画像加载失败: %v", err)
	}
	g, _, src := loadSource(vault, p, flags["db"], flags["store"])
	fmt.Fprintf(os.Stderr, "seren mcp: 已建图（source=%s, 节点 %d）——只读 tools: stats/roam/random/relation（AI 通道）\n",
		src, g.Stats().Nodes)
	srv := mcp.New(g, p, version)
	if err := srv.Serve(os.Stdin, os.Stdout); err != nil {
		fatal("MCP 服务失败: %v", err)
	}
}

func sortStrings(ss []string) {
	for i := 1; i < len(ss); i++ {
		for j := i; j > 0 && ss[j] < ss[j-1]; j-- {
			ss[j], ss[j-1] = ss[j-1], ss[j]
		}
	}
}

func fint(flags map[string]string, k string, def int) int {
	if v, ok := flags[k]; ok {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func fint64(flags map[string]string, k string, def int64) int64 {
	if v, ok := flags[k]; ok {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			return n
		}
	}
	return def
}

func ffloat(flags map[string]string, k string, def float64) float64 {
	if v, ok := flags[k]; ok {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return def
}

func fatal(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "错误: "+format+"\n", a...)
	os.Exit(1)
}
