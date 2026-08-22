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
//
// ============================================================================
package main

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"serendipity-engine/internal/adapter"
	"serendipity-engine/internal/graph"
	"serendipity-engine/internal/roam"
	"serendipity-engine/internal/store"
	"serendipity-engine/internal/sync"
	"serendipity-engine/internal/web"
)

// version 语义化版本号；发布时同步 git tag。
const version = "v0.1.2"

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
  seren roam <vault|OrcaNote.db> <query> [flags]   查询漫游 → top-N 节点簇
  seren serve <vault|OrcaNote.db> [--port 8080]    Web UI（REST + 节点簇可视化 + 刷新）
  seren refresh <vault|OrcaNote.db> [flags] 对账刷新：重解析 + 与上次持久化状态 diff
  seren profile-detect <vault>          扫描 vault，提出解析画像 YAML（新库 onboarding）
  seren version                         打印版本
flags:
  --top N        输出条数 (默认 15)
  --lambda X     激活衰减 (默认 0.7)
  --theta Y      激活剪枝阈值 (默认 0.1)
  --hops N       最大跳数 (默认 3)
  --alpha X      结构分权重 (默认 0.5)
  --beta Y       激活分权重 (默认 0.5)
画像/存储:
  --profile <file.yaml>       显式画像文件
  --profile-name <name>       内置画像名 (default-obsidian / okf / example-wiki)
  --db <file.sqlite>          从持久化存储读图（跳过解析）
  --persist                   解析后持久化到库内 .serendipity/db-<hash>.sqlite
  --store <file.sqlite>       指定持久化路径（覆盖默认）
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
			}
		} else {
			pos = append(pos, a)
		}
	}
	return pos, flags
}

// loadSource 统一加载：显式存储(--db) > 虎鲸库(.db 自动识别，先拷贝再读) > Obsidian vault。
// loadSource 统一加载（fatal 版）：显式存储(--db) > 虎鲸库(.db 自动识别，先快照再读) > Obsidian vault。
func loadSource(vault string, p *adapter.VaultProfile, dbFile string) (*graph.Graph, []*adapter.Document, string) {
	docs, src, err := parseSource(vault, p, dbFile)
	if err != nil {
		fatal("%v", err)
	}
	return graph.Build(docs), docs, src
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
	docs, src, err := parseSource(vault, p, flags["db"])
	if err != nil {
		fatal("%v", err)
	}
	res := sync.Diff(old, docs)
	if err := store.Save(storePath, docs); err != nil {
		fatal("持久化失败: %v", err)
	}

	fmt.Printf("source: %s\n", src)
	fmt.Printf("画像: %s\n", p.Name)
	fmt.Printf("store: %s\n", storePath)
	fmt.Printf("对账: 新增 %d / 更新 %d / 删除 %d / 未变 %d  （耗时 %dms）\n",
		res.Added, res.Updated, res.Deleted, res.Unchanged, res.DurationMS)
	limit := fint(flags, "top", 50)
	show := 0
	for _, c := range res.Changes {
		if show >= limit {
			fmt.Printf("  … 其余 %d 条略（--top 调整）\n", len(res.Changes)-show)
			break
		}
		show++
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
	g, docs, src := loadSource(vault, p, flags["db"])
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
	if len(pos) < 2 {
		fatal("用法: seren roam <vault> <query> [flags]")
	}
	vault, query := pos[0], pos[1]
	if strings.TrimSpace(query) == "" {
		fatal("查询不能为空")
	}
	top := fint(flags, "top", 15)
	lambda := ffloat(flags, "lambda", 0.7)
	theta := ffloat(flags, "theta", 0.1)
	hops := fint(flags, "hops", 3)
	alpha := ffloat(flags, "alpha", 0.5)
	beta := ffloat(flags, "beta", 0.5)

	p, err := adapter.ResolveProfile(flags["profile"], flags["profile-name"], vault)
	if err != nil {
		fatal("画像加载失败: %v", err)
	}
	g, _, src := loadSource(vault, p, flags["db"])
	out := roam.Compute(g, p, query, roam.Options{
		Top: top, Hops: hops, Lambda: lambda, Theta: theta,
		Alpha: alpha, Beta: beta, FilterStructural: true,
	})

	fmt.Printf("source: %s\n", src)
	fmt.Printf("query: %s\n", query)
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
			fmt.Printf("%s", a.ID)
		}
		fmt.Println()
		for _, a := range out.Anchors {
			fmt.Printf("  -> %s (title=%s, type=%s)\n", a.ID, a.Title, a.Type)
		}
		fmt.Printf("params: lambda=%.2f theta=%.2f hops=%d alpha=%.2f beta=%.2f top=%d\n",
			lambda, theta, hops, alpha, beta, top)
		fmt.Println("--- top 节点簇 ---")
		for i, r := range out.Results {
			fmt.Printf("%2d. %-28s %-12s %-6s score=%.3f ppr=%.4f act=%.3f %d-hop  %s\n",
				i+1, r.ID, r.Title, nodeType(g, r.ID), r.Score, r.PPR, r.Act, r.Hops, strings.Join(r.Path, " → "))
		}
	}
}

func cmdServe(args []string) {
	pos, flags := parseArgs(args)
	if len(pos) < 1 {
		fatal("用法: seren serve <vault> [--port 8080] [--vault-name 库名]")
	}
	vault := pos[0]
	port := fint(flags, "port", 8080)
	p, err := adapter.ResolveProfile(flags["profile"], flags["profile-name"], vault)
	if err != nil {
		fatal("画像加载失败: %v", err)
	}
	g, docs, src := loadSource(vault, p, flags["db"])

	// vault 名（obsidian:// URI 跳转用）：显式 > 路径 basename；虎鲸库无跳转
	vaultName := flags["vault-name"]
	if vaultName == "" && !adapter.IsOrcaDB(vault) {
		vaultName = filepath.Base(filepath.Clean(vault))
	}
	if flags["db"] != "" {
		// 存储回读：按路径形态推断源（orca 节点 path = block/<id>）
		for _, d := range docs {
			if strings.HasPrefix(d.Path, "block/") {
				vaultName = "" // 虎鲸无 URI 跳转
				break
			}
		}
	}

	srv := web.New(g, p, src, vaultName, version, refreshFunc(vault, p, flags))
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	fmt.Printf("Serendipity Engine %s Web UI: http://%s  (source: %s, 节点 %d)\n",
		version, addr, src, g.Stats().Nodes)
	if vaultName != "" {
		fmt.Printf("跳转: Obsidian vault 名=%s（卡片上点「打开」会跳到笔记软件）\n", vaultName)
	}
	if err := http.ListenAndServe(addr, srv.Handler()); err != nil {
		fatal("服务失败: %v", err)
	}
}

// refreshFunc 构造 Web 端的刷新闭包：重解析 → 对账 diff → 写回存储 → 返回
// diff 结果与刷新后的新图（供 /api/refresh 替换内存图）。Store 路径与 CLI refresh 一致。
func refreshFunc(vault string, p *adapter.VaultProfile, flags map[string]string) web.RefreshFunc {
	storePath := storePathFor(vault, flags["store"])
	return func() (*sync.Result, *graph.Graph, error) {
		old, err := store.Load(storePath)
		if err != nil {
			return nil, nil, fmt.Errorf("读旧状态失败: %w", err)
		}
		docs, _, err := parseSource(vault, p, flags["db"])
		if err != nil {
			return nil, nil, err
		}
		res := sync.Diff(old, docs)
		if err := store.Save(storePath, docs); err != nil {
			return nil, nil, fmt.Errorf("持久化失败: %w", err)
		}
		return res, graph.Build(docs), nil
	}
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
