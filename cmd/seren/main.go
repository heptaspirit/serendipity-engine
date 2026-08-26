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
//	         潜在关联 suggest-links 待审清单（#15：2-hop + AA/Jaccard/RA + Borda +
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
	cryptorand "crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"math/rand/v2"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
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
const version = "v0.1.15"

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
		text = "index: 解析 + 建图 + 统计 + 持久化（新库 onboarding）\n" +
			"  seren index <vault|OrcaNote.db> [--profile-name <名>] [--db <store>] [--persist|--store <file>]\n" +
			"  --profile-name  内置画像 (default-obsidian / okf / example-wiki)\n" +
			"  --db            从持久化存储读图（跳过解析）\n" +
			"  --persist       解析后持久化到库内 .serendipity/db-<hash>.bbolt\n" +
			"  --store         指定持久化路径（覆盖默认）"
	case "roam":
		text = "roam: 查询漫游 → top-N 节点簇（--random 随机漫步）\n" +
			"  seren roam <vault|OrcaNote.db> [query] [flags]\n" +
			"  --top N        输出条数 (默认 15)\n" +
			"  --hops N       最大跳数 (默认 3)\n" +
			"  --lambda X     激活衰减 (默认 0.7)\n" +
			"  --theta Y      激活剪枝阈值 (默认 0.1)\n" +
			"  --alpha X      结构分权重 (默认 0.5)\n" +
			"  --beta Y       激活分权重 (默认 0.5)\n" +
			"  --random       随机漫步：随机 roll 起点 + 它的簇（可省略 query）\n" +
			"  --seed N       随机种子（默认 0=随机；固定 N 可复现）\n" +
			"  --rand-alpha X 起点度加权指数（0=均匀，1=偏丰富簇，默认 0.5）\n" +
			"  --json         结构化 JSON 输出（roam.Outcome）"
	case "serve":
		text = "serve: Web UI（REST + 节点簇可视化 + 自动监听 + 刷新）\n" +
			"  seren serve [<vault|OrcaNote.db>] [--port 8080] [flags]\n" +
			"  不带 vault = 无库启动，启动后 POST /api/vault {\"path\":\"<库>\"} 配库（v0.1.15）\n" +
			"  --port N       端口 (默认 8080)\n" +
			"  --vault-name N Obsidian vault 名（obsidian:// 跳转）\n" +
			"  --repo <名>    虎鲸 repo 名（orca-note:// 跳转）\n" +
			"  --token <t>    鉴权 token（默认自动生成）\n" +
			"  --watch-off    关闭自动监听\n" +
			"  --watch-interval N  轮询秒 (默认 10)\n" +
			"  --watch-throttle N  刷新节流秒 (默认 60)"
	case "refresh":
		text = "refresh: 对账刷新（重解析 + 与上次持久化状态 diff 增删改）\n" +
			"  seren refresh <vault|OrcaNote.db> [--profile-name <名>] [--store <file>] [--json]\n" +
			"  --store         存储路径 (覆盖默认)\n" +
			"  --json          结构化 JSON 输出（syncpkg.Result）"
	case "profile-detect":
		text = "profile-detect: 扫描 vault，产出解析画像 YAML（新库 onboarding）\n" +
			"  seren profile-detect <vault>"
	case "mcp":
		text = "mcp: MCP stdio server（只读工具：stats/roam/random/relation/node/similar/community）\n" +
			"  seren mcp <vault|OrcaNote.db> [--db <store>] [--profile-name <名>]\n" +
			"  --db            读持久化存储免重解析"
	default:
		usage()
		return
	}
	fmt.Printf("Serendipity Engine %s\n用法:\n  %s\n", version, text)
}

func usage() {
	fmt.Printf(`Serendipity Engine %s
用法:
  seren index <vault|OrcaNote.db> [flags]  解析、建图、统计（.db 自动识别为虎鲸库）
  seren roam <vault|OrcaNote.db> [query] [flags]   查询漫游 → top-N 节点簇（--random 随机漫步）
  seren serve [<vault|OrcaNote.db>] [--port 8080]    Web UI（REST + 节点簇可视化 + 刷新；不带 vault = 无库启动，POST /api/vault 配库）
  seren refresh <vault|OrcaNote.db> [flags] 对账刷新：重解析 + 与上次持久化状态 diff
  seren profile-detect <vault>          扫描 vault，提出解析画像 YAML（新库 onboarding）
  seren mcp <vault> [--db <store>]       MCP stdio server（只读七件套，AI 入口）
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
  --db <file.bbolt>           从持久化存储读图（跳过解析）
  --persist                   解析后持久化到库内 .serendipity/db-<hash>.bbolt
  --store <file.bbolt>        指定持久化路径（覆盖默认）
监听/跳转/埋点:
  --watch-off                 关闭自动监听（默认开：轮询变化→节流合并刷新）
  --watch-interval N          监听轮询间隔秒（默认 10）
  --watch-throttle N          刷新节流秒（默认 60，合并窗口防频繁重解析）
  --repo <name>               虎鲸 repo 名（orca-note:// 跳转；默认取库文件名）
CLI 三件套 (v0.1.11):
  seren help <subcmd>         子命令级帮助（或 seren <subcmd> -h）
  --json                      roam/index/refresh 结构化 JSON 输出
  退出码: 0 成功 / 2 用法或参数错误 / 1 运行时错误
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
	syncpkg.ApplyRenames(out, renames)
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
//
//	Obsidian 源且已有旧状态 → ParseVaultIncremental（复用 mtime/size 未变文件，
//	只重解析变更/新增；返回 reused 计数供日志）；其余（--db 回读 / 虎鲸 /
//	首次全量）→ parseSource。语义与全量解析等价（见 adapter/obsidian.go）。
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
		usageErr("用法: seren refresh <vault> [--store file]")
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
	res := syncpkg.Diff(old, docs)
	// 改名迁移（v0.1.5，修订 #8）：持久化映射合并（含本次新检测）→ 存 renames
	// 表 + touch 迁移。documents 存原始 Refs（文件真相，diff 收敛）；
	// 重定向只在建图时叠加（见 refreshFunc/loadSource 的 redirectForGraph）。
	merged := syncpkg.MergeRenames(storedRenames, renamesMap(res.Renames), docs)
	if err := store.SaveRenames(storePath, merged); err != nil {
		fatal("改名映射持久化失败: %v", err)
	}
	if err := store.RenameTouch(touchPathFor(vault), merged); err != nil {
		fatal("touch 迁移失败: %v", err)
	}
	if err := store.Save(storePath, docs); err != nil {
		fatal("持久化失败: %v", err)
	}

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
		case syncpkg.KindAdded:
			fmt.Printf("  + 新增   %-10s %s [%s]\n", c.ID, c.Title, c.Type)
		case syncpkg.KindDeleted:
			fmt.Printf("  - 删除   %-10s %s [%s]\n", c.ID, c.Title, c.Type)
		case syncpkg.KindUpdated:
			fmt.Printf("  ~ 更新   %-10s %s [%s] 字段=%s", c.ID, c.Title, c.Type, strings.Join(c.Fields, ","))
			if len(c.AddedRefs) > 0 || len(c.RemovedRefs) > 0 {
				fmt.Printf(" 引用+%d/-%d", len(c.AddedRefs), len(c.RemovedRefs))
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
		usageErr("用法: seren index <vault>")
	}
	vault := pos[0]
	p, err := adapter.ResolveProfile(flags["profile"], flags["profile-name"], vault)
	if err != nil {
		fatal("画像加载失败: %v", err)
	}
	g, docs, src := loadSource(vault, p, flags["db"], flags["store"])
	if flags["json"] != "" {
		// --json：复用 graph.Stats 结构体 + 类型分布，顶层补 source/画像/文档数。
		// 置于人类可读输出之前——--json 是整块序列化，不掺人类文本。
		typeCount := map[string]int{}
		for _, d := range docs {
			typeCount[d.Type]++
		}
		jsonOut(map[string]any{
			"vault": vault, "source": src, "profile": p.Name,
			"documents": len(docs), "types": typeCount, "stats": g.Stats(),
		})
		return 0
	}

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
				fatal("创建 .serendipity 失败: %v", err)
			}
		}
		if err := store.Save(dbPath, docs); err != nil {
			fatal("持久化失败: %v", err)
		}
		fmt.Printf("已持久化: %s\n", dbPath)
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
		usageErr("用法: seren roam <vault> [query] [flags]（--random 随机漫步时可省略 query）")
	}
	vault := pos[0]
	var query string
	if len(pos) >= 2 {
		query = pos[1]
	}
	if !random && strings.TrimSpace(query) == "" {
		usageErr("查询不能为空（或加 --random 随机漫步）")
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
	baseRefresh := refreshFunc(vault, p, flags)
	refreshFn := func() (*syncpkg.Result, *graph.Graph, error) {
		res, ng, err := baseRefresh()
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
		Refresh: refreshFn, Touch: touchFn,
		IsPending: func() bool { return env.pending.Load() },
	}
	// 埋点只读统计（幽灵过滤跨图库，双路径）
	st.TouchStat = func() (int, []web.TouchRow, []web.TouchRow, error) {
		total, targets, sources, err := store.TouchStats(touchPath, storePath, 10)
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
		check = watch.NewVaultChecker(vault, p.ExcludedDirs, p.ExcludedFiles)
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
		log.Printf("[watch] 自动刷新完成: 新增 %d / 更新 %d / 删除 %d / 改名 %d（revision=%d）",
			res.Added, res.Updated, res.Deleted, res.Renamed, srv.Revision())
		return nil
	})
	fmt.Printf("自动监听: 开（轮询 %v，刷新节流 %v；--watch-off 关闭）\n", env.poll, env.throttle)
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
		if _, err := cryptorand.Read(buf); err != nil {
			fatal("token 生成失败: %v", err)
		}
		token = hex.EncodeToString(buf)
	}

	env := &serveEnv{
		flags:    flags,
		watchOn:  flags["watch-off"] == "",
		poll:     time.Duration(fint(flags, "watch-interval", 10)) * time.Second,
		throttle: time.Duration(fint(flags, "watch-throttle", 60)) * time.Second,
	}
	srv := web.New(nil, nil, "", "", version, nil, nil)
	srv.Token = token

	// 配库闭包（POST /api/vault）：opts 覆盖启动 flags 的 profile/store/db
	srv.SetVault(func(path string, opts web.VaultOpts) (*web.VaultState, error) {
		merged := map[string]string{}
		for k, v := range flags {
			merged[k] = v
		}
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
			fatal("画像加载失败: %v", err)
		}
		env.vault, env.p = vault, p
		st, err := buildServeState(env)
		if err != nil {
			fatal("%v", err)
		}
		srv.ApplyVaultState(st)
		env.startWatch(srv)

		fmt.Printf("Serendipity Engine %s Web UI: %s  (source: %s, 节点 %d)\n",
			version, clickableLink("http://"+addr), st.Source, st.G.Stats().Nodes)
		switch {
		case st.OrcaRepo != "":
			fmt.Printf("跳转: 虎鲸 repo=%s（卡片上点「打开」会跳到虎鲸对应块）\n", st.OrcaRepo)
		case st.VaultName != "":
			fmt.Printf("跳转: Obsidian vault 名=%s（卡片上点「打开」会跳到笔记软件）\n", st.VaultName)
		}
		if !adapter.IsOrcaDB(vault) && flags["profile-name"] == "" && flags["profile"] == "" && adapter.DetectLLMWiki(vault) {
			fmt.Printf("提示: 检测到 LLM Wiki 结构（raw/ + wiki/index.md）——如需只扫 wiki/ 实体页并排除 index.md/log.md，加 --profile-name llm-wiki\n")
		}
		if n, err := store.TouchCount(touchPathFor(vault)); err == nil && n > 0 {
			fmt.Printf("反馈埋点: 已记录 %d 次点击（仅记录不演化边权）\n", n)
		}
		if gen, _ := store.MaybeDigest(touchPathFor(vault), storePathFor(vault, flags["store"]), store.LoadTouchConfig(baseOf(vault))); gen {
			fmt.Printf("touch digest: 启动补查生成新 digest（间隔兜底）\n")
		}
	} else {
		// 无库启动：空图起服务，等待 POST /api/vault 配库
		fmt.Printf("Serendipity Engine %s Web UI（无库启动）: %s\n", version, clickableLink("http://"+addr))
		fmt.Printf("  等待配库: POST /api/vault {\"path\":\"<vault 目录或 .db>\"}（页面顶部也有选库入口）\n")
	}

	fmt.Printf("API 鉴权: 开（token=%s；页面已自动注入，curl 用 X-Seren-Token 头或 ?token=；--token 可指定固定值）\n", token)

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
		log.Printf("收到退出信号，正在关闭…")
		env.mu.Lock()
		if env.cancel != nil {
			env.cancel() // 停 watch（若有）
		}
		env.mu.Unlock()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := httpSrv.Shutdown(ctx); err != nil {
			log.Printf("关闭服务超时: %v", err)
		}
	}()
	if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		fatal("服务失败: %v", err)
	}
	log.Printf("引擎已退出")
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

// refreshFunc 构造 Web 端的刷新闭包：重解析 → 对账 diff → 改名迁移（合并持久化
// 映射 + touch 迁移 + renames 落盘）→ 写回存储（原始 Refs）→ 返回 diff 结果与
// 刷新后的新图（建图叠加重定向）。Store 路径与 CLI refresh 一致。
func refreshFunc(vault string, p *adapter.VaultProfile, flags map[string]string) web.RefreshFunc {
	storePath := storePathFor(vault, flags["store"])
	return func() (*syncpkg.Result, *graph.Graph, error) {
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
		res := syncpkg.Diff(old, docs)
		// 改名迁移（v0.1.5，修订 #8）：见 cmdRefresh 同段注释
		merged := syncpkg.MergeRenames(storedRenames, renamesMap(res.Renames), docs)
		if err := store.SaveRenames(storePath, merged); err != nil {
			return nil, nil, fmt.Errorf("改名映射持久化失败: %w", err)
		}
		if err := store.RenameTouch(touchPathFor(vault), merged); err != nil {
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

func cmdProfileDetect(args []string) int {
	pos, flags := parseArgs(args)
	if flags["help"] != "" {
		usageFor("profile-detect")
		return 0
	}
	if len(pos) < 1 {
		usageErr("用法: seren profile-detect <vault>")
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
		usageErr("用法: seren mcp <vault> [--db <store.bbolt>]（库来源同 roam；--db 读持久化存储免重解析）")
	}
	vault := pos[0]
	p, err := adapter.ResolveProfile(flags["profile"], flags["profile-name"], vault)
	if err != nil {
		fatal("画像加载失败: %v", err)
	}
	g, _, src := loadSource(vault, p, flags["db"], flags["store"])
	// 启动横幅仅在交互式终端（stdout 是 TTY）打印——DSH 等 MCP 客户端 spawn 时
	// stdout 是管道，静默（否则每次重连/respawn 都在宿主控制台刷一行）。
	if isTerminal(os.Stdout) {
		fmt.Fprintf(os.Stderr, "seren mcp: 已建图（source=%s, 节点 %d）——只读 tools: stats/roam/random/relation/node/similar/community/touch_digest\n",
			src, g.Stats().Nodes)
	}
	srv := mcp.New(g, p, version)
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
	if err := srv.Serve(os.Stdin, os.Stdout); err != nil {
		fatal("MCP 服务失败: %v", err)
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

// usageErr 用法/参数错误 → 退出码 2（CLI 三件套 #3；agent 可自纠补参重跑）。
// 与 fatal（运行时错误，exit 1）区分——参数错误不是系统故障，重跑即可能成功。
func usageErr(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "用法错误: "+format+"\n", a...)
	os.Exit(2)
}

// jsonOut 结构化输出（CLI 三件套 #2）：整块 JSON 序列化到 stdout。
// err 非 nil 时按运行时错误处理（exit 1）。复用现有结构体（roam.Outcome /
// syncpkg.Result / graph.Stats / adapter.Document），不新增镜像类型。
func jsonOut(v any) {
	if err := json.NewEncoder(os.Stdout).Encode(v); err != nil {
		fatal("JSON 输出失败: %v", err)
	}
}
