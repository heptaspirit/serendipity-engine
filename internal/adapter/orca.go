package adapter

// ============================================================================
// 文件：internal/adapter/orca.go
// 模块：虎鲸（Orca Note）笔记库适配器 —— 把虎鲸 SQLite 库翻译成内核统一 Document 列表
//
// ▍职责
//   内核只认识 Document（设计 §6.8 Document API，VFS 哲学）；每个笔记软件一个
//   adapter 负责"格式翻译"。本文件 = 虎鲸的翻译器，其余代码不感知虎鲸的任何细节。
//
// ▍虎鲸数据模型（对真实库实测，见 docs/design.md §6.9.1）
//   虎鲸没有笔记文件，数据全在库根 OrcaNote.db（SQLite，WAL 模式）。块（Block）是
//   唯一实体，**"页面块 vs 内容块"的区别就在 content 列**：
//     content IS NULL         → 页面块（一个页面 = 一个文档；页面可以嵌套）
//     content 为 JSON 数组    → 内容块（一段 = 一个块，从属于某个页面）
//   content JSON 段两种：{"t":"t","v":"文本"} 文本段；{"t":"r","v":<块id>} 引用段。
//   另有：
//     text 列  = 已解析纯文本——内联引用已渲染成 [目标标题]，可直接用于检索；
//     left 列  = 前兄弟指针（块在页面内的顺序链，页内排序用）；
//     BlockRef(f,t,type[,alias]) 三种引用（1=正文内嵌 2=带别名/属性 3=无别名关联）；
//     BlockAlias(name,block)     = 页面/块的 title（别名即 title）。
//
// ▍安全红线（不可违反）
//   1. 绝不直接读用户正在使用的活库（WAL 可能被占用、可能写坏）：先
//      CopyDBForRead 拷贝到安全位置再读，读时再加 PRAGMA query_only 双保险，
//      绝不写回、绝不锁活库。
//   2. Repo / Plugin / PluginStorage 表含用户凭据（API key、对象存储密钥等）与
//      私有数据——本适配器不查询这些表；图数据只从 Block/BlockRef/BlockAlias 派生。
//
// ▍聚合策略（v0.1.1 起，修"每块一节点"的纯数字噪声；v0.1.2 补虚拟文档根）
//   旧版把每个 Block 都做成一个 Document（真实库 4919 节点），其中 1042 个页面块
//   大多无别名、无文本，标题兜底成"块#N"——输出一片纯数字，没有任何具体信息。
//   v0.1.1 把图节点粒度收敛到**页面**；v0.1.2 处理深层树（TestOrca 大库实测）：
//     - 页面块 → 一个 Document(type=doc)；**链顶内容块**（parent 无效且无页面
//       祖先，如剪藏书根）→ 一个 Document(type=block，虚拟文档根)；
//     - 文档根自身 text + 全部非文档根后代按 left 顺序链拼接的 text，构成文档
//       全文（全文 LIKE 兜底直接可用）；
//     - BlockRef 的 f/t 都先映射到"所属文档根"，边 = 文档根间无向边；映射后自环
//       （同文档内部引用）丢弃；引用别名（type2）暂存备用；
//     - 嵌套页面（content NULL 且 parent 非空）仍是独立 Document，并向其宿主
//       文档根加一条"包含"边（保留层级信息，防碎块）；
//     - 标题兜底：别名 > 文档根文本首行（去行尾 " #标签"）> 首个后代块文本首行
//       > "页面#N" / "块#N"（已极少）。
//
// ▍修改记录
//   v0.1.0  初版：每 Block 一节点 → 页面块全成"块#N"，纯数字噪声（已弃用）。
//   v0.1.1  页面/内容块聚合重写；引用归一化到所属页面；left 排序链；标题兜底改善。
//   v0.1.2  pageOf 链顶虚拟文档根（TestOrca 深层树重复/碎片修复：21805→538 文档）；
//          CopyDBForRead 双路径快照（VACUUM INTO 优先 + 文件拷贝/WAL recovery/
//          integrity_check 兜底，兼容虎鲸 app 独占锁）；to_pinyin 确定性 stub。
// ============================================================================

import (
	"database/sql"
	"database/sql/driver"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	sqlite "modernc.org/sqlite" // 纯 Go SQLite，零 CGO（设计 §6.8）；命名 import 以注册 stub 函数
)

// init 注册虎鲸库缺失的自定义函数 stub。
// 实测：虎鲸 schema 里 BlockAlias 有生成列 name_p = to_pinyin(name)（拼音搜索）。
// CopyDBForRead 用 VACUUM INTO 做一致性快照时需复制 schema —— SQLite 要求生成列
// 里的函数必须是**确定性**函数（否则 schema 校验报 malformed），且函数必须存在
// （否则报 no such function）。stub 返回输入原样即可：我们从不读 name_p 列，
// 只要 schema 复制能通过。注册是进程级（modernc driver），本地工具可接受。
func init() {
	sqlite.RegisterDeterministicScalarFunction("to_pinyin", 1, func(ctx *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
		if len(args) == 0 || args[0] == nil {
			return nil, nil
		}
		return args[0], nil
	})
}

// orcaBlock 一行 Block 表的原始数据。
type orcaBlock struct {
	id       int64
	content  sql.NullString // NULL = 页面块；否则为 JSON 段数组（文本段/引用段）
	text     sql.NullString // 已解析纯文本（内联引用渲染成 [目标标题]）
	modified int64          // Unix 秒
	parent   sql.NullInt64  // 父块（页面/内容块）
	left     sql.NullInt64  // 前兄弟指针（页内顺序链）
}

// orcaRef 一条 BlockRef：引用目标 + 引用别名（type=2 带属性引用时的标签）。
type orcaRef struct {
	t     int64
	alias string
}

// orcaDB 解析上下文：块表 + 子块索引 + 别名 + 引用 + 所属页面缓存。
type orcaDB struct {
	blocks map[int64]*orcaBlock
	kids   map[int64][]int64   // parent id → 子块 id（已按 left 链排序）
	alias  map[int64][]string  // block id → 别名（title）
	refs   map[int64][]orcaRef // f → 引用列表
	owner  map[int64]int64     // block id → 所属页面块 id；-1 = 游离块（缓存）
}

// ParseOrcaDB 从虎鲸库 SQLite 的拷贝解析出 Document 列表。
// 安全红线：先拷贝再读（CopyDBForRead），Repo 等凭据表绝不查询。
// 图节点粒度 = 页面（详见文件头"聚合策略"）。
func ParseOrcaDB(dbPath string) ([]*Document, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("打开虎鲸库: %w", err)
	}
	defer db.Close()
	// 只读安全（拷贝已保证不锁活库，query_only 双保险）
	if _, err := db.Exec("PRAGMA query_only = ON"); err != nil {
		return nil, err
	}

	o := &orcaDB{
		blocks: map[int64]*orcaBlock{},
		kids:   map[int64][]int64{},
		alias:  map[int64][]string{},
		refs:   map[int64][]orcaRef{},
		owner:  map[int64]int64{},
	}

	// 1. blocks（页面/内容块判定 + 父子 + 顺序指针）
	rows, err := db.Query(`SELECT id, content, text, modified, parent, left FROM Block`)
	if err != nil {
		return nil, fmt.Errorf("读 Block: %w", err)
	}
	for rows.Next() {
		b := &orcaBlock{}
		if err := rows.Scan(&b.id, &b.content, &b.text, &b.modified, &b.parent, &b.left); err != nil {
			rows.Close()
			return nil, err
		}
		o.blocks[b.id] = b
		if b.parent.Valid {
			o.kids[b.parent.Int64] = append(o.kids[b.parent.Int64], b.id)
		}
	}
	rows.Close()

	// 子块按 left 链排序：left = 前兄弟 id，升序 ≈ 文档顺序（left 为空的最前）
	for p, ids := range o.kids {
		sort.Slice(ids, func(i, j int) bool {
			vi, vj := o.leftOf(ids[i]), o.leftOf(ids[j])
			if vi != vj {
				return vi < vj
			}
			return ids[i] < ids[j]
		})
		o.kids[p] = ids
	}

	// 2. 别名（title）
	rows, err = db.Query(`SELECT name, block FROM BlockAlias ORDER BY block, pos`)
	if err != nil {
		return nil, fmt.Errorf("读 BlockAlias: %w", err)
	}
	for rows.Next() {
		var name string
		var blkID int64
		if err := rows.Scan(&name, &blkID); err != nil {
			rows.Close()
			return nil, err
		}
		o.alias[blkID] = append(o.alias[blkID], name)
	}
	rows.Close()

	// 3. 引用（边）；alias 列暂存，未来可作边标签展示
	rows, err = db.Query(`SELECT f, t, alias FROM BlockRef`)
	if err != nil {
		return nil, fmt.Errorf("读 BlockRef: %w", err)
	}
	for rows.Next() {
		var f, t int64
		var a sql.NullString
		if err := rows.Scan(&f, &t, &a); err != nil {
			rows.Close()
			return nil, err
		}
		o.refs[f] = append(o.refs[f], orcaRef{t: t, alias: a.String})
	}
	rows.Close()

	return o.documents(), nil
}

// leftOf 取块的 left 值（无则为 0，排最前）。
func (o *orcaDB) leftOf(id int64) int64 {
	if b, ok := o.blocks[id]; ok && b.left.Valid {
		return b.left.Int64
	}
	return 0
}

// pageOf 返回块所属的"文档根"块 id：
//   - 内容块向上找最近的页面（content NULL）祖先 → 页面即文档根；
//   - 若无页面祖先，则链条顶端（parent 无效/缺失）的块是**虚拟文档根**
//     （如剪藏书：epub 导入的书根是内容块而非页面，整棵子树挂在它下面）。
//
// 文档根是唯一的：页面块属于自己；每个块恰好归一个文档根。
// TestOrca 大库实测：深层树（书→章节→段落）若按"无页面祖先即游离"处理，
// 链上每个块都会独立成文档且被父块 walk 折叠——内容重复、碎片化。
func (o *orcaDB) pageOf(id int64) (int64, bool) {
	if v, ok := o.owner[id]; ok {
		return v, v >= 0
	}
	b, ok := o.blocks[id]
	if !ok {
		o.owner[id] = -1
		return 0, false
	}
	if !b.content.Valid { // 页面块属于自己
		o.owner[id] = id
		return id, true
	}
	// 内容块：向上找最近页面祖先；无则链顶块为虚拟文档根
	top := id
	cur := b
	for cur.parent.Valid {
		parent, ok := o.blocks[cur.parent.Int64]
		if !ok {
			break // 父缺失：cur 即链顶
		}
		if !parent.content.Valid { // 最近页面祖先
			o.owner[id] = parent.id
			return parent.id, true
		}
		top = parent.id
		cur = parent
	}
	o.owner[id] = top
	return top, true
}

// isPage 判断块是否为页面块（content 列为 NULL）。
func isPage(b *orcaBlock) bool { return b != nil && !b.content.Valid }

// documents 组装 Document 列表（聚合策略见文件头）。
func (o *orcaDB) documents() []*Document {
	docs := map[int64]*Document{} // 文档根块 id → Document

	// 1. 建壳：每个"文档根"一个 Document。
	//    文档根 = 页面块（content NULL）∪ 链顶内容块（虚拟文档根，如剪藏书根）。
	for id, b := range o.blocks {
		switch {
		case isPage(b):
			docs[id] = &Document{
				ID:    strconv.FormatInt(id, 10),
				Type:  "doc",
				Path:  "block/" + strconv.FormatInt(id, 10),
				MTime: time.Unix(b.modified, 0),
			}
		default:
			root, _ := o.pageOf(id)
			if root == id { // 自己是链顶内容块 → 虚拟文档根
				docs[id] = &Document{
					ID:    strconv.FormatInt(id, 10),
					Type:  "block",
					Path:  "block/" + strconv.FormatInt(id, 10),
					MTime: time.Unix(b.modified, 0),
				}
			}
		}
	}

	// 2. 填充：文本（自身 + 非文档根后代按序拼接）+ 引用（归一化到所属文档根）+ 包含边
	for id, d := range docs {
		refSet := map[string]bool{}
		addRef := func(t int64) {
			pt, ok := o.pageOf(t)
			if !ok {
				// 目标是游离块：它自己是文档
				if _, isDoc := docs[t]; isDoc {
					pt, ok = t, true
				}
			}
			if !ok || pt == id {
				return // 悬空 / 页内自环
			}
			tid := strconv.FormatInt(pt, 10)
			if !refSet[tid] {
				refSet[tid] = true
				d.Refs = append(d.Refs, tid)
			}
		}

		if b := o.blocks[id]; b != nil && b.text.Valid && strings.TrimSpace(b.text.String) != "" {
			d.Text += b.text.String
		}
		for _, r := range o.refs[id] {
			addRef(r.t)
		}
		seen := map[int64]bool{id: true}
		var walk func(pid int64)
		walk = func(pid int64) {
			for _, kid := range o.kids[pid] {
				if seen[kid] {
					continue
				}
				seen[kid] = true
				kb := o.blocks[kid]
				if isPage(kb) {
					addRef(kid) // 嵌套页面 → 包含边（pageOf(kid)=kid）
					continue    // 嵌套页面的内容归它自己，不并进父页
				}
				if kb != nil && kb.text.Valid && strings.TrimSpace(kb.text.String) != "" {
					d.Text += kb.text.String
				}
				for _, r := range o.refs[kid] {
					addRef(r.t)
				}
				walk(kid)
			}
		}
		walk(id)

		d.Text = strings.TrimSpace(d.Text)
		d.Size = int64(len(d.Text))
		d.Aliases = o.alias[id]
		d.Title = o.docTitle(id, o.blocks[id])
	}

	// 3. 按块 id 稳定排序输出（确定性）
	ids := make([]int64, 0, len(docs))
	for id := range docs {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	out := make([]*Document, 0, len(ids))
	for _, id := range ids {
		out = append(out, docs[id])
	}
	return out
}

// docTitle 标题兜底链：别名 > 自身文本首行 > 首个内容块文本首行 > "页面#N"/"块#N"。
func (o *orcaDB) docTitle(id int64, b *orcaBlock) string {
	if as := o.alias[id]; len(as) > 0 {
		if t := strings.TrimSpace(as[0]); t != "" {
			return t
		}
	}
	if b != nil && b.text.Valid {
		if t := orcaTitleFromText(b.text.String); t != "" {
			return t
		}
	}
	for _, kid := range o.kids[id] {
		kb := o.blocks[kid]
		if kb == nil || isPage(kb) || !kb.text.Valid {
			continue
		}
		if t := orcaTitleFromText(kb.text.String); t != "" {
			return t
		}
	}
	if isPage(b) {
		return "页面#" + strconv.FormatInt(id, 10)
	}
	return "块#" + strconv.FormatInt(id, 10)
}

// orcaTitleFromText 从文本提取标题：首行、去行尾 " #标签"、截断 30 字符。
// 虎鲸页面文本常见形态："标题 #标签"（如 "十一个时区之旅 #书籍"）。
func orcaTitleFromText(t string) string {
	t = strings.TrimSpace(t)
	if t == "" {
		return ""
	}
	if i := strings.IndexByte(t, '\n'); i > 0 {
		t = t[:i]
	}
	t = strings.TrimSpace(t)
	if i := strings.Index(t, " #"); i > 0 {
		t = t[:i]
	}
	runes := []rune(strings.TrimSpace(t))
	if len(runes) > 30 {
		runes = runes[:30]
	}
	return strings.TrimSpace(string(runes))
}

// CopyDBForRead 把虎鲸库做成一致性快照再读（绝不锁活库、绝不在库内写文件）。
//
// 双路径（实测 TestOrca 大库，虎鲸 app 常以独占锁打开活库）：
//
//	A. VACUUM INTO：等价 backup API，输出含未 checkpoint WAL 事务的一致性副本，
//	   最干净。但 app 持 EXCLUSIVE 锁时拿不到读锁（busy_timeout 后 SQLITE_BUSY）。
//	B. 文件级拷贝（.db + .db-wal，先 wal 后 db）：Windows 文件共享允许文件拷贝，
//	   不经过 SQLite 锁协议。拷贝打开时 SQLite 自动做 WAL recovery（wal 与 db 的
//	   salt 匹配才合并，最坏丢未 checkpoint 数据、绝不损坏）；打开后跑
//	   integrity_check 校验（拷贝中 app 写入导致中间态 → 检测到则重试一次）。
//
// 返回快照路径与清理函数；快照优先放系统临时目录，失败则退回当前目录。
func CopyDBForRead(src string) (string, func(), error) {
	cleanup := func() {}
	dirs := []string{os.TempDir(), "."}
	var lastErr error

	for _, dir := range dirs {
		f, err := os.CreateTemp(dir, "seren-orca-*.db")
		if err != nil {
			lastErr = err
			continue
		}
		dst := f.Name()
		f.Close()

		// 探测：app 是否独占锁（busy_timeout=0 立即失败）→ 决定走哪条路径
		locked := probeLocked(src)
		var snapErr error
		if !locked {
			if err := vacuumInto(src, dst); err == nil {
				return dst, func() { os.Remove(dst) }, nil
			} else {
				snapErr = err
			}
		}
		if err := fileCopySnapshot(src, dst); err == nil {
			return dst, func() { os.Remove(dst); os.Remove(dst + "-wal") }, nil
		} else if snapErr == nil {
			snapErr = err
		}
		lastErr = snapErr
		os.Remove(dst)
		os.Remove(dst + "-wal")
	}
	return "", cleanup, lastErr
}

// probeLocked 探测源库当前是否被独占（busy_timeout=0，立即返回）。
func probeLocked(src string) bool {
	db, err := sql.Open("sqlite", src)
	if err != nil {
		return true // 打不开按锁处理，走文件拷贝兜底
	}
	defer db.Close()
	db.Exec("PRAGMA busy_timeout = 0")
	var one int
	if err := db.QueryRow("SELECT 1").Scan(&one); err != nil {
		return true
	}
	return false
}

// vacuumInto 路径 A：VACUUM INTO 一致性快照。
func vacuumInto(src, dst string) error {
	db, err := sql.Open("sqlite", src)
	if err != nil {
		return err
	}
	defer db.Close()
	// 活库可能正被虎鲸 app 占用：等待锁释放（默认 busy_timeout=0 会立刻失败）
	db.Exec("PRAGMA busy_timeout = 3000")
	// VACUUM INTO 的文件名是 SQL 字符串字面量，不支持参数绑定 → 转义单引号
	_, err = db.Exec("VACUUM INTO '" + strings.ReplaceAll(dst, "'", "''") + "'")
	if err != nil {
		return fmt.Errorf("VACUUM INTO: %w", err)
	}
	return nil
}

// fileCopySnapshot 路径 B：文件级拷贝 .db + .db-wal（先 wal 后 db），
// 打开时自动 WAL recovery，再 integrity_check 校验。
func fileCopySnapshot(src, dst string) error {
	// 先 wal 后 db：checkpoint 间隙若 db 已更新，旧 wal salt 不匹配会被忽略，
	// 不会丢已 checkpoint 的数据
	if err := copyFile(src+"-wal", dst+"-wal"); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := copyFile(src, dst); err != nil {
		return err
	}
	db, err := sql.Open("sqlite", dst)
	if err != nil {
		return err
	}
	defer db.Close()
	db.Exec("PRAGMA busy_timeout = 3000")
	var ok string
	if err := db.QueryRow("PRAGMA integrity_check").Scan(&ok); err != nil {
		return err
	}
	if ok != "ok" {
		return fmt.Errorf("快照完整性校验失败: %s", ok)
	}
	return nil
}

// copyFile 简单的文件复制（无缓冲优化——个人库规模足够）。
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := out.ReadFrom(in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// IsOrcaDB 按扩展名粗判是否为虎鲸库文件。
func IsOrcaDB(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".db"
}
