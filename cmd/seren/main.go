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
//  1. --db <file.bbolt>   从持久化存储读图（跳过解析）
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
//	全量重解析 + syncpkg.Diff 与上次持久化状态比对（按 ID 对齐，规范化比较字段），
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
//	v0.1.10 MCP 集成修复：initialize 回显客户端 protocolVersion（修复 SDK 客户端
//	         版本不匹配→断连重连→反复 spawn）；启动横幅仅 TTY 打印（DSH spawn 静默）。
//	v0.1.11 M1 阶段 1 第二批：similar 结构相似（/api/similar + MCP graph.similar）、
//	         graph.node 节点详情（/api/node + MCP graph.node）、/api/roam?export=1
//	         漫游导出、/api/touch/stats 埋点只读统计、Stats 缓存、renames 中间环
//	         清理、WAL autocheckpoint。
//	         附：CLI 三件套（backlog §五）——子命令级帮助（seren help <cmd> /
//	         <cmd> -h）、--json 结构化输出（roam/index/refresh）、退出码语义化
//	         （0 成功 / 2 用法错误 / 1 运行时错误）。
//	v0.1.12 M1 阶段 1 收官 + 前端 P0：similar 评分升级 Jaccard → Adamic-Adar；
//	         is_pending 刷新待办标志（watch 原子标志 + /api/stats + 手动刷新清 pending）；
//	         LLM Wiki 结构探测提示（DetectLLMWiki）；MCP 扩至七工具（graph.community）；
//	         前端 P0（紧凑嵌入 / postMessage 桥 / i18n 双语）——见 internal/web/static。
//	         本版不做 GitHub Actions 自动构建（用户拍板），本地 scratch/seren.exe 仅联调不入库。
//	v0.1.13 M1 收官（#16 + #15）：存储层 SQLite → bbolt（#16，四 bucket 无迁移，
//	         P1 增量写 / P2 mmap+NoSync / P5 幽灵过滤 O(1)，扩展名 .bbolt）；
//	         潜在关联 suggest-links 待审清单（#15：2-hop + AA/Jaccard/RA 加权聚合 +
//	         top-K 节流 → /api/suggest-links，未落图，co-touch 留 M2）。
//	v0.1.14 M2 §3.7 touch 行为信号子系统：touch 拆独立 store touch-<hash>.bbolt
//	         （touch/meta/backups，图库重建不再连坐）；digest 触发（计数≥digest_count
//	         主 / 间隔≥digest_days 兜底 + 启动补查）+ /api/touch/digest + ack +
//	         /api/stats.digest_available + MCP seren.touch_digest（只读）；touch.yaml
//	         配置（YAML 钳制）；引擎零写 vault（digest md 由插件导出）。
//	v0.1.15 serve 无库启动（壳/TUI/Wails 的地基）：seren serve 不带 vault → 空库
//	         启动，POST /api/vault {path,...} 配库/换库（GET /api/vault 查配置）；
//	         web 路由全量注册 + handler 内闭包 nil 判定；/api/stats 加 configured。
//	         前端导出改 fetch+blob（a 标签不带 token 被 auth 拒的 bug 修复）。
//	         优雅退出（SIGINT/SIGTERM → 停 watch → Shutdown）；终端 URL OSC 8 可点击。
//
// ============================================================================
package main

import (
	"context"
	crand "crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"maps"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"serendipity-engine/internal/adapter"
	"serendipity-engine/internal/graph"
	"serendipity-engine/internal/mcp"
	"serendipity-engine/internal/roam"
	"serendipity-engine/internal/store"
	syncpkg "serendipity-engine/internal/sync"
	"serendipity-engine/internal/watch"
	"serendipity-engine/internal/web"
)

// version 语义化版本号；发布时同步 git tag（README 徽章版本号也在此次同步）。
const version = "v0.2.2"

func main() {
	code := run(os.Args[1:])
	os.Exit(code)
}

// ---------- CLI 三件套（v0.1.11，backlog §五）----------
// 退出码语义化：
//
//	0 成功；2 用法/参数错误（agent 可自纠——补参数重跑）；1 运行时错误（解析失败、
//	库不存在、服务失败等）。此前参数错误与运行时错误都 exit 1，agent 无法区分。
func run(args []string) int {
	if len(args) < 1 {
		usage()
		return 2
	}
	switch args[0] {
	case "index":
		return cmdIndex(args[1:])
	case "roam":
		return cmdRoam(args[1:])
	case "serve":
		return cmdServe(args[1:])
	case "refresh":
		return cmdRefresh(args[1:])
	case "profile-detect":
		return cmdProfileDetect(args[1:])
	case "mcp":
		return cmdMCP(args[1:])
	case "version", "--version", "-v":
		fmt.Printf("Serendipity Engine %s\n", version)
		return 0
	case "help", "-h", "--help":
		if len(args) >= 2 {
			usageFor(args[1])
		} else {
			usage()
		}
		return 0
	default:
		usage()
		return 2
	}
}

// usageFor 打印某个子命令的专属帮助（CLI 三件套 #1：seren <cmd> 的 -h / help）。
func usageFor(cmd string) {
	var text string
	switch cmd {
	case "index":
		text = "index: parse + build graph + stats + persist (new-vault onboarding)\n" +
			"  seren index <vault|OrcaNote.db> [--profile-name <name>] [--db <store>] [--persist|--store <file>]\n" +
			"  --profile-name  built-in profile (default-obsidian / okf / example-wiki)\n" +
			"  --db            read graph from persisted store (skip parse)\n" +
			"  --persist       persist to .serendipity/db-<hash>.bbolt\n" +
			"  --store         explicit persist path (overrides default)"
	case "roam":
		text = "roam: query roam -> top-N node cluster (--random random walk)\n" +
			"  seren roam <vault|OrcaNote.db> [query] [flags]\n" +
			"  --top N        output count (default 15)\n" +
			"  --hops N       max hops (default 3)\n" +
			"  --lambda X     activation decay (default 0.7)\n" +
			"  --theta Y      activation pruning threshold (default 0.1)\n" +
			"  --alpha X      structural weight (default 0.5)\n" +
			"  --beta Y       activation weight (default 0.5)\n" +
			"  --random       random walk: random start node + its cluster (query optional)\n" +
			"  --seed N       random seed (default 0=random; fixed N reproducible)\n" +
			"  --rand-alpha X start degree weighting (0=uniform, 1=favor rich, default 0.5)\n" +
			"  --json         structured JSON output (roam.Outcome)"
	case "serve":
		text = "serve: Web UI (REST + node-cluster viz + auto watch + refresh)\n" +
			"  seren serve [<vault|OrcaNote.db>] [--port 8080] [flags]\n" +
			"  no vault = no-vault start, then POST /api/vault {\"path\":\"<vault>\"} to configure (v0.1.15)\n" +
			"  --port N       port (default 8080)\n" +
			"  --vault-name N Obsidian vault name (obsidian:// jump)\n" +
			"  --repo <name>  Orca repo name (orca-note:// jump)\n" +
			"  --token <t>    auth token (default auto-generated)\n" +
			"  --pid-file <p> write own PID to <p> on start, remove on clean exit (managed-launch handle)\n" +
			"  --watch-off    disable auto watch\n" +
			"  --watch-interval N  poll seconds (default 10)\n" +
			"  --watch-throttle N  refresh throttle seconds (default 60)"
	case "refresh":
		text = "refresh: reconcile refresh (reparse + diff against last persisted state)\n" +
			"  seren refresh <vault|OrcaNote.db> [--profile-name <name>] [--store <file>] [--json]\n" +
			"  --store        persist path (overrides default)\n" +
			"  --json         structured JSON output (syncpkg.Result)"
	case "profile-detect":
		text = "profile-detect: scan vault, emit parse profile YAML (new-vault onboarding)\n" +
			"  seren profile-detect <vault>"
	case "mcp":
		text = "mcp: MCP stdio server (read-only tools: stats/roam/random/relation/node/similar/community)\n" +
			"  seren mcp <vault|OrcaNote.db> [--db <store>] [--profile-name <name>]\n" +
			"  --db           read persisted store to skip re-parse"
	default:
		usage()
		return
	}
	fmt.Printf("Serendipity Engine %s\nUsage:\n  %s\n", version, text)
}

func usage() {
	fmt.Printf(`Serendipity Engine %s
Usage:
  seren index <vault|OrcaNote.db> [flags]  parse, build graph, stats (.db auto-detected as Orca vault)
  seren roam <vault|OrcaNote.db> [query] [flags]   query roam -> top-N node cluster (--random random walk)
  seren serve [<vault|OrcaNote.db>] [--port 8080]    Web UI (REST + node-cluster viz + refresh; no vault = no-vault start, POST /api/vault to configure)
  seren refresh <vault|OrcaNote.db> [flags]  reconcile refresh: reparse + diff against last persisted state
  seren profile-detect <vault>           scan vault, emit parse profile YAML (new-vault onboarding)
  seren mcp <vault> [--db <store>]       MCP stdio server (read-only nine tools, AI channel)
  seren version                          print version
Flags:
  --top N        output count (default 15)
  --lambda X     activation decay (default 0.7)
  --theta Y      activation pruning threshold (default 0.1)
  --hops N       max hops (default 3)
  --alpha X      structural weight (default 0.5)
  --beta Y       activation weight (default 0.5)
Random walk (v0.1.7):
  --random       random walk: random start node + its cluster (query optional)
  --seed N       random seed (default 0=random; fixed N reproducible)
  --rand-alpha X random start degree weighting: 0=uniform (surprise), 1=favor rich clusters (default 0.5)
Security (v0.1.8):
  --token <t>    serve API auth token (default auto-generated and printed; page auto-injected)
Profile/store:
  --profile <file.yaml>       explicit profile file
  --profile-name <name>       built-in profile (default-obsidian / okf / example-wiki)
  --db <file.bbolt>           read graph from persisted store (skip parse)
  --persist                   persist to .serendipity/db-<hash>.bbolt
  --store <file.bbolt>        explicit persist path (overrides default)
Watch/jump/touch:
  --watch-off                 disable auto watch (default on: poll changes -> throttle merge refresh)
  --watch-interval N          watch poll interval seconds (default 10)
  --watch-throttle N          refresh throttle seconds (default 60, merge window)
  --repo <name>               Orca repo name (orca-note:// jump; default from filename)
CLI trio (v0.1.11):
  seren help <subcmd>         per-subcommand help (or seren <subcmd> -h)
  --json                      structured JSON output for roam/index/refresh
  exit code: 0 ok / 2 usage or arg error / 1 runtime error
`, version)
}

// parseArgs 解析 CLI 参数：位置参数 + --k=v / --k v 标志。
// -h/--help 也识别为 flags（子命令级帮助，CLI 三件套 #1）——单短横线 -h 不当作位置参数。
func parseArgs(args []string) (pos []string, flags map[string]string) {
	flags = map[string]string{}
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch a {
		case "-h", "--help":
			flags["help"] = "true"
			continue
		}
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
// 解析失败 fatal 退出（CLI 命令批处理语义）。
func loadSource(vault string, p *adapter.VaultProfile, dbFile, storeFlag string) (*graph.Graph, []*adapter.Document, string) {
	g, docs, src, err := loadSourceErr(vault, p, dbFile, storeFlag)
	if err != nil {
		fatal("%v", err)
	}
	return g, docs, src
}

// loadSourceErr loadSource 的 error 返回版（v0.1.15 配库用）：配库（POST /api/vault）
// 解析失败必须返回 error 给前端 JSON，绝不能 fatal 退出——否则路径写错直接杀掉
// 整个 serve 进程（此前 buildServeState 用 loadSource 的 fatal，配库失败即崩溃）。
func loadSourceErr(vault string, p *adapter.VaultProfile, dbFile, storeFlag string) (*graph.Graph, []*adapter.Document, string, error) {
	docs, src, err := parseSource(vault, p, dbFile)
	if err != nil {
		return nil, nil, "", err // 前缀已在 parseSource 内（"解析失败"），不再重复
	}
	// 改名映射来源：--db 时 renames 在同一个存储文件里；vault 解析时在
	// storePathFor 对应的默认存储里（无 → 空映射，全新构建无需迁移）。
	var renames map[string]string
	if dbFile != "" {
		renames, _ = store.LoadRenames(dbFile)
	} else {
		renames, _ = store.LoadRenames(storePathFor(vault, storeFlag))
	}
	return graph.Build(redirectForGraph(docs, renames)), docs, src, nil
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
	syncpkg.ApplyRenames(out, renames)
	return out
}

// parseSource 统一加载（error 版，供 refresh 复用）：返回 Document 列表与源描述。
// v0.2.1：--db 加载前校验 build 是否过期（binary 版本或画像签名变化）——过期 → 强制
// 全量重析，否则升级后的解析规则/画像排除不生效（增量复用 mtime/size 未变文件）。
func parseSource(vault string, p *adapter.VaultProfile, dbFile string) ([]*adapter.Document, string, error) {
	if dbFile != "" && !storeStale(dbFile, p) {
		docs, err := store.Load(dbFile)
		if err != nil {
			return nil, "", fmt.Errorf("read store failed: %w", err)
		}
		return docs, "store:" + dbFile, nil
	}
	if adapter.IsOrcaDB(vault) {
		cp, cleanup, err := adapter.CopyDBForRead(vault)
		if err != nil {
			return nil, "", fmt.Errorf("snapshot Orca DB failed: %w", err)
		}
		defer cleanup()
		docs, err := adapter.ParseOrcaDB(cp)
		if err != nil {
			return nil, "", fmt.Errorf("parse Orca DB failed: %w", err)
		}
		return docs, "orca:" + vault, nil
	}
	docs, err := adapter.ParseVault(vault, p)
	if err != nil {
		return nil, "", fmt.Errorf("parse failed: %w", err)
	}
	return docs, "obsidian:" + vault, nil
}

// profileSignature 对画像做稳定签名（sha256 hex）：画像任一字段变（如新增排除）→ 签名变。
// storeStale 据此在下次加载时强制全量重析，让"改画像"自动生效（无需手动 seren index）。
func profileSignature(p *adapter.VaultProfile) string {
	if p == nil {
		return ""
	}
	s, err := adapter.MarshalProfile(p)
	if err != nil {
		return ""
	}
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// storeStale 判断 store 是否过期：binary 版本或画像签名变化 → 需强制全量重析（否则增量
// 复用 mtime/size 未变的旧文档，升级/改画像后的新规则不生效）。
func storeStale(dbPath string, p *adapter.VaultProfile) bool {
	if store.LoadParserVersion(dbPath) != version {
		return true
	}
	if store.LoadProfileSignature(dbPath) != profileSignature(p) {
		return true
	}
	return false
}

// refreshParse 刷新专用解析（v0.1.6 快照增量优化）：
//
//	Obsidian 源且已有旧状态 → ParseVaultIncremental（复用 mtime/size 未变文件，
//	只重解析变更/新增；返回 reused 计数供日志）；其余（--db 回读 / 虎鲸 /
//	首次全量）→ parseSource。语义与全量解析等价（见 adapter/obsidian.go）。
func refreshParse(vault string, p *adapter.VaultProfile, flags map[string]string, old []*adapter.Document) (docs []*adapter.Document, reused int, src string, err error) {
	// v0.2.1：build 过期（binary 版本或画像签名变化）→ 强制全量重析（否则增量复用旧解析结果，
	// 升级/改画像后的规则不生效——曾导致反斜杠 dangling 残留、新排除不生效）。
	stale := storeStale(storePathFor(vault, flags["store"]), p)
	if !stale && flags["db"] == "" && !adapter.IsOrcaDB(vault) && len(old) > 0 {
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

// writePIDFile 原子写入当前进程 PID 到 path（temp+rename，覆盖陈旧文件），
// 目录不存在则创建。供 managed 启动方（Obsidian 插件）读取/清理。v0.2.1。
func writePIDFile(path string) error {
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(strconv.Itoa(os.Getpid())+"\n"), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path) // 原子替换：Windows 上 Go 的 Rename 覆盖已存在目标
}

// touchPathFor 计算 touch 独立 store 路径（§3.7，v0.1.14）：与图库同源派生，
// 虎鲸库同样取库所在目录（touch 与图库成对）。
func touchPathFor(vault string) string {
	return store.TouchDBPath(baseOf(vault))
}

// baseOf 库根目录：虎鲸库（db 文件）取所在目录，Obsidian vault 即自身。
// 用于 touch 独立库与 touch.yaml 的路径派生（§3.7.1 / §3.7.5）。
func baseOf(vault string) string {
	if adapter.IsOrcaDB(vault) {
		return filepath.Dir(vault)
	}
	return vault
}

// cmdRefresh 对账刷新：全量重解析 → 与上次持久化状态 diff → 输出明细 → 写回存储。
func cmdRefresh(args []string) int {
	pos, flags := parseArgs(args)
	if flags["help"] != "" {
		usageFor("refresh")
		return 0
	}
	if len(pos) < 1 {
		usageErr("usage: seren refresh <vault> [--store file]")
	}
	vault := pos[0]
	p, err := adapter.ResolveProfile(flags["profile"], flags["profile-name"], vault)
	if err != nil {
		fatal("profile load failed: %v", err)
	}
	storePath := storePathFor(vault, flags["store"])

	// 对账刷新（CLI 与 Web 共用同一管线，见 refreshAll）
	o, err := refreshAll(vault, p, flags, false)
	if err != nil {
		fatal("%v", err)
	}
	res := o.res
	docs := o.docs
	src := o.src
	reused := o.reused
	oldN := o.oldN

	if flags["json"] != "" {
		// --json：复用 syncpkg.Result 结构体（含 Changes/Renames 明细）。为可读，
		// 补 src/画像/解析计数作为顶层字段（新匿名结构体，不污染 Result）。
		jsonOut(map[string]any{
			"source": src, "profile": p.Name, "store": storePath,
			"reused": reused, "documents": len(docs),
			"result": res,
		})
		return 0
	}

	fmt.Printf("source: %s\n", src)
	fmt.Printf("profile: %s\n", p.Name)
	fmt.Printf("store: %s\n", storePath)
	if flags["db"] == "" && !adapter.IsOrcaDB(vault) && oldN > 0 {
		fmt.Printf("parsed: %d docs (incremental: reused %d / reparsed %d)\n", len(docs), reused, len(docs)-reused)
	} else {
		fmt.Printf("parsed: %d docs (full)\n", len(docs))
	}
	fmt.Printf("reconcile: +%d / ~%d / -%d / ↦%d / =%d  (%dms)\n",
		res.Added, res.Updated, res.Deleted, res.Renamed, res.Unchanged, res.DurationMS)
	limit := fint(flags, "top", 50)
	show := 0
	next := func() bool {
		if show >= limit {
			fmt.Printf("  … %d more omitted (adjust --top)\n", len(res.Changes)+len(res.Renames)-show)
			return false
		}
		show++
		return true
	}
	for _, r := range res.Renames {
		if !next() {
			break
		}
		fmt.Printf("  ↦ rename  %-10s → %-10s %s [%s]\n", r.OldID, r.NewID, r.Title, r.Type)
	}
	for _, c := range res.Changes {
		if !next() {
			break
		}
		switch c.Kind {
		case syncpkg.KindAdded:
			fmt.Printf("  + added  %-10s %s [%s]\n", c.ID, c.Title, c.Type)
		case syncpkg.KindDeleted:
			fmt.Printf("  - removed %-10s %s [%s]\n", c.ID, c.Title, c.Type)
		case syncpkg.KindUpdated:
			fmt.Printf("  ~ updated %-10s %s [%s] fields=%s", c.ID, c.Title, c.Type, strings.Join(c.Fields, ","))
			if len(c.AddedRefs) > 0 || len(c.RemovedRefs) > 0 {
				fmt.Printf(" refs+%d/-%d", len(c.AddedRefs), len(c.RemovedRefs))
			}
			fmt.Println()
		}
	}
	return 0
}

func cmdIndex(args []string) int {
	pos, flags := parseArgs(args)
	if flags["help"] != "" {
		usageFor("index")
		return 0
	}
	if len(pos) < 1 {
		usageErr("usage: seren index <vault>")
	}
	vault := pos[0]
	p, err := adapter.ResolveProfile(flags["profile"], flags["profile-name"], vault)
	if err != nil {
		fatal("profile load failed: %v", err)
	}
	g, docs, src := loadSource(vault, p, flags["db"], flags["store"])
	// 类型分布（--json 与人类输出共用一次统计）
	typeCount := map[string]int{}
	for _, d := range docs {
		typeCount[d.Type]++
	}
	if flags["json"] != "" {
		// --json：复用 graph.Stats 结构体 + 类型分布，顶层补 source/画像/文档数。
		// 置于人类可读输出之前——--json 是整块序列化，不掺人类文本。
		jsonOut(map[string]any{
			"vault": vault, "source": src, "profile": p.Name,
			"documents": len(docs), "types": typeCount, "stats": g.Stats(),
		})
		return 0
	}

	fmt.Printf("vault: %s\n", vault)
	fmt.Printf("source: %s\n", src)
	fmt.Printf("profile: %s\n", p.Name)
	fmt.Printf("parsed documents: %d\n", len(docs))

	fmt.Println("type distribution:")
	var types []string
	for t := range typeCount {
		types = append(types, t)
	}
	sort.Strings(types)
	for _, t := range types {
		fmt.Printf("  %-8s %d\n", t, typeCount[t])
	}

	s := g.Stats()
	fmt.Printf("graph nodes: %d\n", s.Nodes)
	fmt.Printf("link ledger: total %d, self %d, dedup-edges %d, multi %d\n",
		s.TotalLinks, s.SelfLinks, s.Edges, s.MultiEdge)
	fmt.Printf("dangling links: %d kinds / %d links (pointing to missing files)\n", s.Dangling, s.DanglingLinks)
	fmt.Printf("orphan nodes: %d (no edges)\n", s.Orphans)
	fmt.Printf("connected components: %d\n", s.Components)
	fmt.Println("top hubs:")
	for _, h := range s.TopHubs {
		fmt.Printf("  %-28s deg=%-4d type=%s title=%s\n", h.ID, h.Deg, h.Type, h.Title)
	}

	// 持久化（设计 §6.8：bbolt 主存储 v0.1.13，库内 .serendipity/db-<hash>.bbolt）
	if flags["persist"] != "" || flags["store"] != "" {
		base := vault
		if adapter.IsOrcaDB(vault) {
			base = filepath.Dir(vault)
		}
		dbPath := flags["store"]
		if dbPath == "" {
			dbPath = store.DBPath(base)
			if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
				fatal("create .serendipity failed: %v", err)
			}
		}
		if err := store.Save(dbPath, docs); err != nil {
			fatal("persist failed: %v", err)
		}
		_ = store.SaveParserVersion(dbPath, version)                // 记解析器版本（v0.2.1，升级失效）
		_ = store.SaveProfileSignature(dbPath, profileSignature(p)) // 记画像签名（改画像自动重析）
		fmt.Printf("persisted: %s\n", dbPath)
	}
	return 0
}

func cmdRoam(args []string) int {
	pos, flags := parseArgs(args)
	if flags["help"] != "" {
		usageFor("roam")
		return 0
	}
	random := flags["random"] != ""
	if len(pos) < 1 {
		usageErr("usage: seren roam <vault> [query] [flags] (query optional with --random)")
	}
	vault := pos[0]
	var query string
	if len(pos) >= 2 {
		query = pos[1]
	}
	if !random && strings.TrimSpace(query) == "" {
		usageErr("query cannot be empty (or add --random)")
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
		fatal("profile load failed: %v", err)
	}
	g, _, src := loadSource(vault, p, flags["db"], flags["store"])
	opt := roam.Options{
		Top: top, Hops: hops, Lambda: lambda, Theta: theta,
		Alpha: alpha, Beta: beta,
	}

	var out *roam.Outcome
	if random {
		// 随机漫步（v0.1.7）：seed=0 用时间随机；固定 seed 可复现（同一节点同一簇）
		out = roam.ComputeRandom(g, p, opt, roam.Roll{Rng: roam.SeededRNG(seed), Alpha: randAlpha})
	} else {
		out = roam.Compute(g, p, query, opt)
	}

	if flags["json"] != "" {
		// --json：复用 roam.Outcome 结构体（anchors/results/fallback/fallback_hits），
		// 顶层补 mode/source/画像（新匿名结构体，不污染 Outcome）。
		mode := "query"
		if random {
			mode = "random-walk"
		}
		jsonOut(map[string]any{
			"mode": mode, "query": query, "seed": seed, "rand_alpha": randAlpha,
			"source": src, "profile": p.Name, "outcome": out,
		})
		return 0
	}

	fmt.Printf("source: %s\n", src)
	if random {
		fmt.Printf("mode: random-walk (🎲 random start + cluster, rand-alpha=%.2f)\n", randAlpha)
	} else {
		fmt.Printf("query: %s\n", query)
	}
	fmt.Printf("profile: %s\n", p.Name)
	switch out.Fallback {
	case roam.ModeNoAnchor:
		fmt.Println("anchor failed (no exact ID/title/alias/tag/LIKE in graph) → fallback: full-text search")
		printTextHits(out.FallbackHits, top)
	case roam.ModeSparse:
		fmt.Println("--- fallback: query node has no/few links → full-text LIKE fallback (decision #10) ---")
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
				mark = " 🎲 random start"
			}
			fmt.Printf("  -> %s (title=%s, type=%s)%s\n", a.ID, a.Title, a.Type, mark)
		}
		fmt.Printf("params: lambda=%.2f theta=%.2f hops=%d alpha=%.2f beta=%.2f top=%d\n",
			lambda, theta, hops, alpha, beta, top)
		fmt.Println("--- top node cluster ---")
		if len(out.Results) == 0 {
			fmt.Println("  (this random node has no cluster — roll again, or drop --random to use query roam)")
		}
		for i, r := range out.Results {
			fmt.Printf("%2d. %-28s %-12s %-6s score=%.3f ppr=%.4f act=%.3f %d-hop  %s\n",
				i+1, r.ID, r.Title, nodeType(g, r.ID), r.Score, r.PPR, r.Act, r.Hops, strings.Join(r.Path, " → "))
		}
	}
	return 0
}

// serveEnv 一次 serve 进程的可变运行时状态（v0.1.15 无库启动）：
// 无 vault 启动后经 POST /api/vault 配库/换库；pending 与 watch 随库重建。
type serveEnv struct {
	mu      sync.Mutex
	vault   string                 // 当前已配库路径（空 = 未配库）
	p       *adapter.VaultProfile  // 当前画像
	flags   map[string]string      // 当前生效 flags（启动 flags + 配库 opts 覆盖）
	pending atomic.Bool            // 待刷新标志（配库后重建）
	cancel  context.CancelFunc     // 当前 watch 取消（换库时停旧）
	watchOn bool                   // 是否启用自动监听（--watch-off 关闭）
	poll    time.Duration          // 监听轮询间隔
	throttle time.Duration         // 刷新节流
}

// buildServeState 解析 vault 并构造 Web 全套闭包状态（配库核心，启动即配库与
// POST /api/vault 共用）：loadSource → isOrca/vaultName/orcaRepo → store/touch
// 路径 → refresh/touch/touchStat/digest/ack/available/isPending 闭包。
// env 只读使用（vault/p/flags）；pending 由调用方在 env 上持有。
func buildServeState(env *serveEnv) (*web.VaultState, error) {
	vault := env.vault
	p := env.p
	flags := env.flags
	g, docs, src, err := loadSourceErr(vault, p, flags["db"], flags["store"])
	if err != nil {
		return nil, err // 解析失败返回给调用方（配库走 JSON 报错，不 fatal）
	}
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
	vaultName := flags["vault-name"]
	if vaultName == "" && !isOrca {
		vaultName = filepath.Base(filepath.Clean(vault))
	}
	orcaRepo := ""
	if isOrca {
		orcaRepo = flags["repo"]
		if orcaRepo == "" {
			orcaRepo = strings.TrimSuffix(filepath.Base(vault), ".db")
		}
	}
	storePath := storePathFor(vault, flags["store"])
	// 刷新闭包：重解析 → diff → 改名迁移 → 写回 → 换图（成功清 pending）
	baseRefresh := refreshFunc(vault, p, flags, false)
	refreshFn := func() (*syncpkg.Result, *graph.Graph, error) {
		res, ng, err := baseRefresh()
		if err == nil {
			env.pending.Store(false)
		}
		return res, ng, err
	}
	// 全量重建闭包（v0.2.1，POST /api/rebuild）：忽略增量、重新解析整库
	baseRebuild := refreshFunc(vault, p, flags, true)
	rebuildFn := func() (*syncpkg.Result, *graph.Graph, error) {
		res, ng, err := baseRebuild()
		if err == nil {
			env.pending.Store(false)
		}
		return res, ng, err
	}
	// touch 独立库 + digest 触发（§3.7，v0.1.14）：失败静默不影响埋点
	touchPath := touchPathFor(vault)
	touchCfg := store.LoadTouchConfig(baseOf(vault))
	touchFn := func(target, from string) error {
		if err := store.AppendTouch(touchPath, target, from); err != nil {
			return err
		}
		_, _ = store.MaybeDigest(touchPath, storePath, touchCfg)
		return nil
	}
	st := &web.VaultState{
		G: g, P: p, Source: src, VaultName: vaultName, OrcaRepo: orcaRepo,
		Refresh: refreshFn, Rebuild: rebuildFn, Touch: touchFn,
		IsPending: func() bool { return env.pending.Load() },
	}
	// 埋点只读统计（幽灵过滤跨图库，双路径）
	st.TouchStat = func() (int, []web.TouchRow, []web.TouchRow, error) {
		total, targets, sources, err := store.TouchStats(touchPath, storePath)
		if err != nil {
			return 0, nil, nil, err
		}
		toRows := func(rs []store.TouchRow) []web.TouchRow {
			out := make([]web.TouchRow, 0, len(rs))
			for _, r := range rs {
				out = append(out, web.TouchRow{ID: r.ID, Count: r.Count})
			}
			return out
		}
		return total, toRows(targets), toRows(sources), nil
	}
	// digest 只读接口（查询 / ack / available）
	st.TouchDg = func() (*web.Digest, error) {
		d, err := store.LatestDigest(touchPath)
		if err != nil || d == nil {
			return nil, err
		}
		return &web.Digest{
			ID: d.ID, GeneratedAt: d.GeneratedAt, WindowStart: d.WindowStart,
			Since: d.Since, Total: d.Total,
			Targets: toWebDigestTargets(d.Targets), Sources: toWebRows(d.Sources),
		}, nil
	}
	st.TouchAck = func(id string) error { return store.AckDigest(touchPath, id) }
	st.DigAvail = func() bool { return store.DigestAvailable(touchPath) }
	return st, nil
}

// startWatch 启动/重启自动监听（配库后或启动即配库调用；换库时先停旧）。
// 回调复用 web 已注入的 Refresh 闭包（配库后已指向新库）+ ReplaceGraph 换图。
func (env *serveEnv) startWatch(srv *web.Server) {
	env.mu.Lock()
	if env.cancel != nil {
		env.cancel() // 停旧 watch（换库）
		env.cancel = nil
	}
	watchOn := env.watchOn
	if !watchOn {
		env.mu.Unlock()
		return
	}
	vault := env.vault
	p := env.p
	var check func() (bool, error)
	if adapter.IsOrcaDB(vault) {
		check = watch.NewOrcaChecker(vault)
	} else {
		check = watch.NewVaultChecker(vault, p)
	}
	ctx, cancel := context.WithCancel(context.Background())
	env.cancel = cancel
	env.mu.Unlock()
	go watch.Run(ctx, env.poll, env.throttle, &env.pending, check, func() error {
		res, ng, err := srv.Refresh()
		if err != nil {
			return err
		}
		srv.ReplaceGraph(ng)
		log.Printf("[watch] auto refresh done: +%d / ~%d / -%d / ↦%d (revision=%d)",
			res.Added, res.Updated, res.Deleted, res.Renamed, srv.Revision())
		return nil
	})
	fmt.Printf("Auto watch: on (poll %v, refresh throttle %v; --watch-off to disable)\n", env.poll, env.throttle)
}

// cmdServe 启动 Web UI（REST + 节点簇可视化 + 自动监听 + 刷新）。
// v0.1.15 无库启动：不带 vault 时以空库启动，POST /api/vault 配库/换库
// （壳/TUI/浏览器选库的地基）；带 vault 时行为与旧版一致（启动即配库）。
func cmdServe(args []string) int {
	pos, flags := parseArgs(args)
	if flags["help"] != "" {
		usageFor("serve")
		return 0
	}
	port := fint(flags, "port", 8080)

	// API 鉴权（v0.1.8 安全前置）：--token 指定；否则自动生成 32 位 hex 并打印。
	token := flags["token"]
	if token == "" {
		buf := make([]byte, 16)
		if _, err := crand.Read(buf); err != nil {
			fatal("token generation failed: %v", err)
		}
		token = hex.EncodeToString(buf)
	}

	// v0.2.1：--pid-file 供 managed 启动方（Obsidian 插件）可靠获得/清理本进程。
	// 启动即写（原子替换，覆盖陈旧 pid 文件），优雅退出时删除；硬杀残留的陈旧
	// pid 文件由插件侧自愈读取校验后清理（见 serendipity-obsidian 交接说明）。
	pidFile := flags["pid-file"]
	if pidFile != "" {
		if err := writePIDFile(pidFile); err != nil {
			log.Printf("warning: write pid file %s: %v", pidFile, err)
		}
	}

	env := &serveEnv{
		flags:    flags,
		watchOn:  flags["watch-off"] == "",
		poll:     time.Duration(fint(flags, "watch-interval", 10)) * time.Second,
		throttle: time.Duration(fint(flags, "watch-throttle", 60)) * time.Second,
	}
	srv := web.New(version)
	srv.Token = token

	// 内嵌 MCP（v0.2.0）：serve 内 /mcp 端点（Streamable HTTP），Web+REST+MCP 三合一。
	// GraphProvider 每次调用读当前 VaultState（live 图，修子进程快照吃不到中途改动）；
	// touch_digest 闭包跟随当前库（配库/换库后经闭包解析当前 touch store）。
	mcpSrv := mcp.New(srv.MCPGraphProvider(), version, "streamable-http")
	mcpSrv.SetTouchDigest(func() (any, error) {
		env.mu.Lock()
		vault := env.vault
		env.mu.Unlock()
		if vault == "" {
			return map[string]any{"digest": nil, "available": false, "total": 0, "targets": []store.TouchRow{}, "sources": []store.TouchRow{}}, nil
		}
		touchPath := touchPathFor(vault)
		storePath := storePathFor(vault, flags["store"])
		d, err := store.LatestDigest(touchPath)
		if err != nil {
			return nil, err
		}
		// v0.2.1 反馈 #1：digest（窗口摘要）与累计统计(ycle 口径)一起返回——
		// AI 不会只看到 null，总有累计上下文；窗口未触发时 digest 为 null 但 total/targets/sources 有值。
		total, targets, sources, serr := store.TouchStats(touchPath, storePath)
		if serr != nil {
			return nil, serr
		}
		return map[string]any{
			"digest": d, "available": d != nil && store.DigestAvailable(touchPath),
			"total": total, "targets": targets, "sources": sources,
		}, nil
	})
	// 累计点击统计（v0.2.1，反馈 #1）：等价 REST /api/touch/stats，补 MCP 侧的"非空"视角。
	mcpSrv.SetTouchStats(func() (any, error) {
		env.mu.Lock()
		vault := env.vault
		env.mu.Unlock()
		if vault == "" {
			return map[string]any{"total": 0, "targets": []store.TouchRow{}, "sources": []store.TouchRow{}}, nil
		}
		total, targets, sources, err := store.TouchStats(touchPathFor(vault), storePathFor(vault, flags["store"]))
		if err != nil {
			return nil, err
		}
		return map[string]any{"total": total, "targets": targets, "sources": sources}, nil
	})
	srv.SetMCP(mcpSrv, true)

	// 配库闭包（POST /api/vault）：opts 覆盖启动 flags 的 profile/store/db
	srv.SetVault(func(path string, opts web.VaultOpts) (*web.VaultState, error) {
		merged := maps.Clone(flags)
		if opts.ProfileName != "" {
			merged["profile-name"] = opts.ProfileName
		}
		if opts.Profile != "" {
			merged["profile"] = opts.Profile
		}
		if opts.Store != "" {
			merged["store"] = opts.Store
		}
		if opts.DB != "" {
			merged["db"] = opts.DB
		}
		p, err := adapter.ResolveProfile(merged["profile"], merged["profile-name"], path)
		if err != nil {
			return nil, fmt.Errorf("画像加载失败: %w", err)
		}
		env.mu.Lock()
		env.vault, env.p, env.flags = path, p, merged
		env.pending.Store(false)
		env.mu.Unlock()
		st, err := buildServeState(env)
		if err != nil {
			return nil, err
		}
		// 配库后启动/重建 watch（web 应用状态后经 OnVaultApplied 触发，
		// 见下——先返回状态，watch 由应用成功后的回调启动）
		return st, nil
	})
	srv.OnVaultApplied = func() { env.startWatch(srv) }

	addr := fmt.Sprintf("127.0.0.1:%d", port)

	// 有 vault → 启动即配库（行为与旧版一致）
	if len(pos) >= 1 {
		vault := pos[0]
		p, err := adapter.ResolveProfile(flags["profile"], flags["profile-name"], vault)
		if err != nil {
			fatal("profile load failed: %v", err)
		}
		env.vault, env.p = vault, p
		st, err := buildServeState(env)
		if err != nil {
			fatal("%v", err)
		}
		srv.ApplyVaultState(st)
		env.startWatch(srv)

		fmt.Printf("Serendipity Engine %s Web UI: %s  (source: %s, nodes: %d)\n",
			version, ansi(1, clickableLink("http://"+addr)), ansi(36, st.Source), st.G.Stats().Nodes)
		switch {
		case st.OrcaRepo != "":
			fmt.Printf("Jump: Orca repo=%s (click „Open“ on a card to jump to the corresponding block)\n", st.OrcaRepo)
		case st.VaultName != "":
			fmt.Printf("Jump: Obsidian vault name=%s (click „Open“ on a card to open the note app)\n", st.VaultName)
		}
		if !adapter.IsOrcaDB(vault) && flags["profile-name"] == "" && flags["profile"] == "" && adapter.DetectLLMWiki(vault) {
			fmt.Printf("Hint: LLM Wiki structure detected (raw/ + wiki/index.md) — to scan only wiki/ entity pages and exclude index.md/log.md, add --profile-name llm-wiki\n")
		}
		if n, err := store.TouchCount(touchPathFor(vault)); err == nil && n > 0 {
			fmt.Printf("Click telemetry: %d hits recorded (recorded only, no edge-weight evolution)\n", n)
		}
		if gen, _ := store.MaybeDigest(touchPathFor(vault), storePathFor(vault, flags["store"]), store.LoadTouchConfig(baseOf(vault))); gen {
			fmt.Printf("touch digest: startup check generated a new digest (interval fallback)\n")
		}
	} else {
		// 无库启动：空图起服务，等待 POST /api/vault 配库
		fmt.Printf("Serendipity Engine %s Web UI (no-vault start): %s\n", version, clickableLink("http://"+addr))
		fmt.Printf("  Waiting for configure: POST /api/vault {\"path\":\"<vault dir or .db>\"} (or use the picker at the top of the page)\n")
	}

	fmt.Printf("API auth: on (token=%s; auto-injected into the page; curl with X-Seren-Token header or ?token=; --token sets a fixed value)\n", ansi(33, token))

	// 优雅退出（v0.1.15）：SIGINT/SIGTERM → 停 watch → HTTP Shutdown（等当前请求完成）。
	// 此前 serve 无信号处理，Ctrl+C 直接硬杀；无库启动 + 自动监听挂后台后需要干净退出。
	// store 每次操作开闭 bbolt（非长驻连接），硬杀不损坏数据，但优雅退出收尾更稳妥
	// （停监听循环、打印退出日志）。Web 端不做关闭入口——生命周期归"拉起它的人"
	// （终端 Ctrl+C / 服务管理器 / 未来壳的进程面板），Web 是消费端无杀服务权限。
	httpSrv := &http.Server{Addr: addr, Handler: srv.Handler()}
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sig
		log.Printf("shutdown signal received, closing…")
		env.mu.Lock()
		if env.cancel != nil {
			env.cancel() // 停 watch（若有）
		}
		env.mu.Unlock()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := httpSrv.Shutdown(ctx); err != nil {
			log.Printf("shutdown timed out: %v", err)
		}
		if pidFile != "" {
			if err := os.Remove(pidFile); err != nil && !os.IsNotExist(err) {
				log.Printf("warning: remove pid file: %v", err)
			}
		}
	}()
	if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		fatal("serve failed: %v", err)
	}
	log.Printf("engine exited")
	return 0
}

// toWebRows / toWebDigestTargets：store 聚合形态 → web 展示形态（闭包注入边界：
// web 不 import store，main 负责映射，与 SetTouchStats 的 toRows 同模式）。
func toWebRows(rs []store.TouchRow) []web.TouchRow {
	out := make([]web.TouchRow, 0, len(rs))
	for _, r := range rs {
		out = append(out, web.TouchRow{ID: r.ID, Count: r.Count})
	}
	return out
}

func toWebDigestTargets(ts []store.DigestTarget) []web.DigestTarget {
	out := make([]web.DigestTarget, 0, len(ts))
	for _, t := range ts {
		out = append(out, web.DigestTarget{ID: t.ID, Title: t.Title, Count: t.Count})
	}
	return out
}

// refreshOut 一次对账刷新的完整产出（CLI 展示与 Web 换图各取所需）。
type refreshOut struct {
	res    *syncpkg.Result
	ng     *graph.Graph // 新图（Web refresh/rebuild 换图用；CLI 忽略）
	docs   []*adapter.Document
	oldN   int // 旧状态文档数（CLI 判断"增量/全量"展示口径）
	src    string
	reused int
}

// refreshAll 执行一次完整对账刷新（CLI `seren refresh` 与 Web refresh/rebuild 共用
// 同一管线，docs/architecture/04-sync.md 契约）：Load 旧态 → 重解析（Obsidian
// 增量复用 / --db 回读 / forceFull 全量重析）→ 对账 diff → 改名迁移（合并持久化
// 映射 + touch 迁移 + renames 落盘）→ 写回存储（原始 Refs）→ 记解析器版本/画像
// 签名。forceFull=true（POST /api/rebuild 用）：忽略增量复用、重新解析整库。
func refreshAll(vault string, p *adapter.VaultProfile, flags map[string]string, forceFull bool) (*refreshOut, error) {
	storePath := storePathFor(vault, flags["store"])
	old, err := store.Load(storePath)
	if err != nil {
		return nil, fmt.Errorf("read old state failed: %w", err)
	}
	storedRenames, err := store.LoadRenames(storePath)
	if err != nil {
		return nil, fmt.Errorf("read rename map failed: %w", err)
	}
	var docs []*adapter.Document
	var src string
	var reused int
	if forceFull {
		docs, src, err = parseSource(vault, p, "") // 全量重建：从库重析，无视增量
	} else {
		docs, reused, src, err = refreshParse(vault, p, flags, old)
	}
	if err != nil {
		return nil, err
	}
	res := syncpkg.Diff(old, docs)
	// 改名迁移（v0.1.5，修订 #8）：持久化映射合并（含本次新检测）→ 存 renames 表 +
	// touch 迁移。documents 存原始 Refs（文件真相，diff 收敛）；重定向只在建图时叠加。
	merged := syncpkg.MergeRenames(storedRenames, renamesMap(res.Renames), docs)
	if err := store.SaveRenames(storePath, merged); err != nil {
		return nil, fmt.Errorf("rename map persist failed: %w", err)
	}
	if err := store.RenameTouch(touchPathFor(vault), merged); err != nil {
		return nil, fmt.Errorf("touch migration failed: %w", err)
	}
	if err := store.Save(storePath, docs); err != nil {
		return nil, fmt.Errorf("persist failed: %w", err)
	}
	_ = store.SaveParserVersion(storePath, version)                // 记解析器版本（v0.2.1，升级失效）
	_ = store.SaveProfileSignature(storePath, profileSignature(p)) // 记画像签名（改画像自动重析）
	return &refreshOut{res: res, ng: graph.Build(redirectForGraph(docs, merged)),
		docs: docs, oldN: len(old), src: src, reused: reused}, nil
}

// refreshFunc 构造 Web 端的刷新/重建闭包（refreshAll 的薄封装：Web 只要 diff 与新图）。
func refreshFunc(vault string, p *adapter.VaultProfile, flags map[string]string, forceFull bool) web.RefreshFunc {
	return func() (*syncpkg.Result, *graph.Graph, error) {
		o, err := refreshAll(vault, p, flags, forceFull)
		if err != nil {
			return nil, nil, err
		}
		return o.res, o.ng, nil
	}
}

// renamesMap 从对账结果的改名明细构建 旧ID→新ID 映射（ApplyRenames /
// RenameTouch 的入参形态）。
func renamesMap(rs []syncpkg.Rename) map[string]string {
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
		fmt.Printf("%2d. %-28s %-12s %-6s full-text hits: %d\n", shown+1, h.ID, h.Title, h.Type, h.Count)
		shown++
		if shown >= top {
			break
		}
	}
	if shown == 0 {
		fmt.Println("  (no full-text hit either — nothing relevant in the library)")
	}
}

func nodeType(g *graph.Graph, id string) string {
	if n, ok := g.Node(id); ok {
		return n.Type()
	}
	return ""
}

func cmdProfileDetect(args []string) int {
	pos, flags := parseArgs(args)
	if flags["help"] != "" {
		usageFor("profile-detect")
		return 0
	}
	if len(pos) < 1 {
		usageErr("usage: seren profile-detect <vault>")
	}
	p, err := adapter.DetectProfile(pos[0])
	if err != nil {
		fatal("profile-detect failed: %v", err)
	}
	out, err := adapter.MarshalProfile(p)
	if err != nil {
		fatal("serialize failed: %v", err)
	}
	fmt.Print(out)
	return 0
}

// cmdMCP 启动 MCP stdio server（第四个入口，v0.1.9，roadmap M0-0.3）。
// 只读工具（stats/roam/random/relation/node/similar/community，v0.1.12 扩至七个）：
// 只 import 纯库，不碰 web/watch。stdout 只承载 JSON-RPC 协议；启动提示写 stderr。
func cmdMCP(args []string) int {
	pos, flags := parseArgs(args)
	if flags["help"] != "" {
		usageFor("mcp")
		return 0
	}
	if len(pos) < 1 {
		usageErr("usage: seren mcp <vault> [--db <store.bbolt>] (source same as roam; --db reads persisted store to skip re-parse)")
	}
	vault := pos[0]
	p, err := adapter.ResolveProfile(flags["profile"], flags["profile-name"], vault)
	if err != nil {
		fatal("profile load failed: %v", err)
	}
	g, _, src := loadSource(vault, p, flags["db"], flags["store"])
	// 启动横幅仅在交互式终端（stdout 是 TTY）打印——DSH 等 MCP 客户端 spawn 时
	// stdout 是管道，静默（否则每次重连/respawn 都在宿主控制台刷一行）。
	if isTerminal(os.Stdout) {
		fmt.Fprintf(os.Stderr, "seren mcp: graph built (source=%s, nodes %d) — read-only tools: stats/roam/random/relation/node/similar/community/touch_digest\n",
			src, g.Stats().Nodes)
	}
	srv := mcp.New(func() (*graph.Graph, *adapter.VaultProfile) { return g, p }, version, "stdio")
	// §3.7（v0.1.14）：seren.touch_digest 只读闭包——MCP 保持只 import 纯库边界，
	// touch store 访问经闭包隔离；digest 映射为 MCP 可见形态。
	srv.SetTouchDigest(func() (any, error) {
		touchPath := touchPathFor(vault)
		d, err := store.LatestDigest(touchPath)
		if err != nil || d == nil {
			return d, err
		}
		// 附 available（未读开关），供 AI 判断是否有新 digest
		return map[string]any{
			"digest": d, "available": store.DigestAvailable(touchPath),
		}, nil
	})
	if err := srv.ServeStdio(os.Stdin, os.Stdout); err != nil {
		fatal("MCP serve failed: %v", err)
	}
	return 0
}

// isTerminal 判断 f 是否为交互式终端（字符设备）。MCP 客户端 spawn 时 stdout 是
// 管道/重定向，非字符设备 → 静默；手动在终端跑 seren mcp 才打印横幅。
func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

// ansi 给文本加 ANSI 颜色/样式——仅在 stdout 是 TTY 时生效（管道/重定向不加，
// 避免污染结构化输出与日志）。c 为 ANSI SGR 码（1=bold, 36=cyan, 33=yellow, 31=red...）。
func ansi(c int, s string) string {
	if !isTerminal(os.Stdout) {
		return s
	}
	return "\x1b[" + strconv.Itoa(c) + "m" + s + "\x1b[0m"
}

// clickableLink 把 URL 渲染为终端可点击链接（v0.1.15）：TTY 下用 OSC 8 超链接
// 转义（\x1b]8;;<url>\x1b\\<text>\x1b]8;;\x1b\\，Windows Terminal / iTerm2 /
// VSCode 终端等支持，Ctrl+点击/单击即开）；非 TTY（管道/重定向/嵌入 spawn）
// 退化为纯文本，避免输出污染。提示"如何开启前端界面"的统一入口。
func clickableLink(url string) string {
	if !isTerminal(os.Stdout) {
		return url
	}
	return "\x1b]8;;" + url + "\x1b\\" + url + "\x1b]8;;\x1b\\"
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
	fmt.Fprintf(os.Stderr, "error: "+format+"\n", a...)
	os.Exit(1)
}

// usageErr 用法/参数错误 → 退出码 2（CLI 三件套 #3；agent 可自纠补参重跑）。
// 与 fatal（运行时错误，exit 1）区分——参数错误不是系统故障，重跑即可能成功。
func usageErr(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "usage error: "+format+"\n", a...)
	os.Exit(2)
}

// jsonOut 结构化输出（CLI 三件套 #2）：整块 JSON 序列化到 stdout。
// err 非 nil 时按运行时错误处理（exit 1）。复用现有结构体（roam.Outcome /
// syncpkg.Result / graph.Stats / adapter.Document），不新增镜像类型。
func jsonOut(v any) {
	if err := json.NewEncoder(os.Stdout).Encode(v); err != nil {
		fatal("JSON output failed: %v", err)
	}
}
