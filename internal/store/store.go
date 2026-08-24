// ============================================================================
// 文件：internal/store/store.go
// 模块：SQLite 持久化（设计 §6.8 主存储）
//
// ▍职责
//
//	Document 列表 ⇄ SQLite 的存取。v1 写语义 = 全量重写（幂等，DELETE + INSERT 于
//	一个事务）；对账刷新（internal/sync + cmd refresh）依赖本模块作为"上次状态"。
//
// ▍存储布局
//
//	默认路径 <vault>/.serendipity/db-<库路径 hash12>.sqlite（DBPath），多库各一文件，
//	便携闭环（库在哪图在哪，见设计 §6.8）。两张表：
//	  documents(id PK, title, type, path, mtime, size, tags, aliases, text)
//	  links(a, b, weight, PK(a,b)) —— 有向引用行（v0.1.5 修正）
//
// ▍links 的方向性（v0.1.5 修正，对账收敛的前提）
//   Document.Refs 是有向的（本文档链接谁）；对账 diff 按 ID 逐文档比较 Refs，
//   因此存储必须保方向。v0.1.5 前 Save 用排序 pairKey 去重（无向对，如
//   (张三,李四) 恒按字典序），Load 只把 b 追加到 a 的 Refs——字典序较大的
//   端点回读后 Refs 为空，每次刷新都报虚假 "refs +1"，永不收敛。修正后：
//   Save 按精确 (a,b) 去重（每文档自己的 Refs 无重复），Load 原样回读；
//   无向语义只在 graph.Build 层体现（双方入邻接表）。
//
// ▍与对账的关系（v0.1.2）
//   - Load 对不存在的文件返回空列表（nil, nil）：对账刷新"首次"场景等价全新增；
//   - Save 全量重写是幂等的：任何一次 refresh 后存储即最新状态，重复刷新无副作用；
//   - 增量写（WAL 单行 UPDATE）是 v1.5 优化，不改本模块语义。
//
// ▍反馈埋点（v0.1.4）
//   touch 表独立于 documents/links：Save 全量重写只 DELETE 后两张表，touch 保留
//   （增量写入）。容量上限 touchMax=5000，超限删最旧——克制设计防无限增长；
//   埋点只记录不演化边权（杜绝"点击→边权变→结果变→再点击"正反馈跑飞）。
//
// ▍修改记录
//
//	v0.1.0  初版 Save/Load 全量重写。
//	v0.1.2  Load 支持不存在的存储文件（首次对账全新增）；补充模块头注释。
//	v0.1.3  Load 兼容"存在但从未写入"的空库文件（无 documents 表 → 无旧状态）。
//	v0.1.4  AppendTouch/TouchCount（反馈埋点，独立表 + 容量上限）。
//	v0.1.5  RenameTouch（改名迁移，修订 #8：touch 旧 ID → 新 ID，两阶段占位防链式）；
//	        links 改有向引用行（修正回读丢方向的虚假 refs+1 对账不收敛）。
//
// ============================================================================
package store

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
	"serendipity-engine/internal/adapter"
)

// touchMax 反馈埋点（touch）表容量上限：保留最近 N 条，防无限增长
// （克制设计 v0.1.4：埋点只记录不演化，杜绝"点击→边权变→结果变→再点击"正反馈）。
const touchMax = 5000

// AppendTouch 记录一次节点点击（反馈埋点，独立表——Save 全量重写不清除）。
// 容量上限：超出 touchMax 时删除最旧记录（按 id 单调递增裁剪）。
func AppendTouch(dbPath, target, from string) error {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return err
	}
	defer db.Close()
	if _, err := db.Exec(`PRAGMA journal_mode = WAL`); err != nil {
		return err
	}
	// v0.1.11（backlog §四）：WAL 自动 checkpoint 上限 1000 页，长跑 + 频繁
	// touch 时防 WAL 缓慢增长（此前未设，靠连接关闭隐式 checkpoint）。
	if _, err := db.Exec(`PRAGMA wal_autocheckpoint = 1000`); err != nil {
		return err
	}
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS touch (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			ts INTEGER NOT NULL,
			target TEXT NOT NULL,
			src TEXT
		)`); err != nil {
		return err
	}
	// 注意：src = 来源节点（from 是 SQL 保留字，不能用做列名）
	if _, err := db.Exec(`INSERT INTO touch (ts, target, src) VALUES (?,?,?)`,
		time.Now().Unix(), target, from); err != nil {
		return err
	}
	_, err = db.Exec(`DELETE FROM touch WHERE id <= (SELECT MAX(id) FROM touch) - ?`, touchMax)
	return err
}

// TouchCount 返回已记录的点击数（serve 启动/刷新日志用）。
func TouchCount(dbPath string) (int, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return 0, err
	}
	defer db.Close()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM touch`).Scan(&n); err != nil {
		return 0, err // 表不存在 = 0
	}
	return n, nil
}

// TouchRow 一条 touch 聚合行（目标/来源 + 点击数）。
type TouchRow struct {
	ID    string `json:"id"`
	Count int    `json:"count"`
}

// TouchStats 反馈埋点只读统计（v0.1.11，backlog §3.3 —— 反馈闭环只读第一步）。
// 只读 SQL 聚合，绝不写库、绝不反馈到排序/hot（红线 2：埋点只记录不演化）。
// touch 表不存在（从未埋点/旧库）→ 全零统计（不报错，展示友好）。
func TouchStats(dbPath string, limit int) (total int, targets, sources []TouchRow, err error) {
	if limit <= 0 {
		limit = 10
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return 0, nil, nil, err
	}
	defer db.Close()
	var tbl string
	if err := db.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name='touch'`).Scan(&tbl); err != nil {
		return 0, nil, nil, nil // 无埋点表 = 全零
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM touch`).Scan(&total); err != nil {
		return 0, nil, nil, err
	}
	targets, err = touchGroup(db, `target`, limit)
	if err != nil {
		return 0, nil, nil, err
	}
	// src 列可能为 NULL/空（早期埋点未传 from）——排除空值
	sources, err = touchGroup(db, `src`, limit)
	if err != nil {
		return 0, nil, nil, err
	}
	return total, targets, sources, nil
}

// touchGroup 按列分组计数（target/src 通用），返回 TopN（降序，并列按 ID 稳定）。
func touchGroup(db *sql.DB, col string, limit int) ([]TouchRow, error) {
	rows, err := db.Query(`SELECT `+col+` AS k, COUNT(*) AS c FROM touch
		WHERE k IS NOT NULL AND k != '' GROUP BY k ORDER BY c DESC, k ASC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TouchRow
	for rows.Next() {
		var r TouchRow
		if err := rows.Scan(&r.ID, &r.Count); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// LoadRenames 读回改名迁移映射（v0.1.5，修订 #8）：renames 表 = 持久化的
// 身份迁移层（old_id → new_id）。documents/links 存文件真相（原始 Refs），
// 图构建时叠加本映射重定向——改名是持久身份事实，不能只在下一次刷新生效。
// 表不存在（旧库/从未改名）→ 返回空映射。
func LoadRenames(dbPath string) (map[string]string, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	var tbl string
	if err := db.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name='renames'`).Scan(&tbl); err != nil {
		return map[string]string{}, nil
	}
	rows, err := db.Query(`SELECT old_id, new_id FROM renames`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var o, n string
		if err := rows.Scan(&o, &n); err != nil {
			return nil, err
		}
		out[o] = n
	}
	return out, rows.Err()
}

// SaveRenames 全量重写 renames 表（幂等；每次刷新后与 documents 一起落盘）。
func SaveRenames(dbPath string, renames map[string]string) error {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return err
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS renames (
		old_id TEXT PRIMARY KEY, new_id TEXT NOT NULL)`); err != nil {
		return err
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM renames`); err != nil {
		return err
	}
	ins, err := tx.Prepare(`INSERT OR IGNORE INTO renames (old_id, new_id) VALUES (?,?)`)
	if err != nil {
		return err
	}
	defer ins.Close()
	for o, n := range renames {
		if _, err := ins.Exec(o, n); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// RenameTouch 把 touch 表里 target/src 为旧 ID 的埋点迁移到新 ID（v0.1.5，
// 改名迁移，设计修订 #8）。两阶段占位（旧ID+"\x00"）防链式改名互踩；
// 映射先做传递解析（旧→中→新 解到最终目标，与 ApplyRenames 语义一致）；
// touch 表不存在（从未埋点）→ 静默跳过。documents/links 由 Save 全量
// 重写重建，无需迁移。
func RenameTouch(dbPath string, renames map[string]string) error {
	if len(renames) == 0 {
		return nil
	}
	// 传递解析：链式改名解到最终目标（持久化映射跨刷新累积成链）
	resolved := map[string]string{}
	for o := range renames {
		seen := map[string]bool{o: true}
		cur := o
		for {
			nid, ok := renames[cur]
			if !ok {
				break
			}
			if seen[nid] {
				cur = o // 环防御：回到原始
				break
			}
			seen[nid] = true
			cur = nid
		}
		if cur != o {
			resolved[o] = cur
		}
	}
	if len(resolved) == 0 {
		return nil
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return err
	}
	defer db.Close()
	var tbl string
	if err := db.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name='touch'`).Scan(&tbl); err != nil {
		return nil // 无埋点数据
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// 阶段 1：旧 ID → 占位（\x00 不出现在真实 ID 中）
	for oldID := range resolved {
		ph := oldID + "\x00"
		if _, err := tx.Exec(`UPDATE touch SET target=? WHERE target=?`, ph, oldID); err != nil {
			return err
		}
		if _, err := tx.Exec(`UPDATE touch SET src=? WHERE src=?`, ph, oldID); err != nil {
			return err
		}
	}
	// 阶段 2：占位 → 新 ID
	for oldID, newID := range resolved {
		ph := oldID + "\x00"
		if _, err := tx.Exec(`UPDATE touch SET target=? WHERE target=?`, newID, ph); err != nil {
			return err
		}
		if _, err := tx.Exec(`UPDATE touch SET src=? WHERE src=?`, newID, ph); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// DBPath 返回库的默认存储路径：<vault>/.serendipity/db-<路径hash>.sqlite
// （设计 §6.8 多库：每库一 DB，便携闭环）。
func DBPath(vault string) string {
	h := sha256.Sum256([]byte(filepath.Clean(vault)))
	hash := hex.EncodeToString(h[:])[:12]
	dir := filepath.Join(vault, ".serendipity")
	return filepath.Join(dir, "db-"+hash+".sqlite")
}

// Save 全量写图：documents + links（无向边，pair 去重）。WAL 模式。
func Save(dbPath string, docs []*adapter.Document) error {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return err
	}
	defer db.Close()
	if _, err := db.Exec(`PRAGMA journal_mode = WAL`); err != nil {
		return err
	}
	// v0.1.11（backlog §四）：WAL autocheckpoint 防长跑增长
	if _, err := db.Exec(`PRAGMA wal_autocheckpoint = 1000`); err != nil {
		return err
	}
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS documents (
			id TEXT PRIMARY KEY, title TEXT, type TEXT, path TEXT,
			mtime INTEGER, size INTEGER, tags TEXT, aliases TEXT, text TEXT
		);
		CREATE TABLE IF NOT EXISTS links (
			a TEXT NOT NULL, b TEXT NOT NULL, weight REAL NOT NULL DEFAULT 1.0,
			PRIMARY KEY (a, b)
		);`); err != nil {
		return err
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM documents; DELETE FROM links;`); err != nil {
		return err
	}
	insDoc, err := tx.Prepare(`INSERT INTO documents
		(id, title, type, path, mtime, size, tags, aliases, text) VALUES (?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer insDoc.Close()
	insLink, err := tx.Prepare(`INSERT OR IGNORE INTO links (a, b, weight) VALUES (?,?,1.0)`)
	if err != nil {
		return err
	}
	defer insLink.Close()

	// 有向引用行：d.Refs = 本文档链接谁；精确 (a,b) 去重（每文档 Refs 本身无重复，
	// seen 仅防御性）。v0.1.5 修正：此前排序 pairKey 去重会丢方向（见文件头）。
	seen := map[string]bool{}
	for _, d := range docs {
		tags, _ := json.Marshal(d.Tags)
		aliases, _ := json.Marshal(d.Aliases)
		if _, err := insDoc.Exec(d.ID, d.Title, d.Type, d.Path,
			d.MTime.Unix(), d.Size, string(tags), string(aliases), d.Text); err != nil {
			return err
		}
		for _, ref := range d.Refs {
			if ref == d.ID {
				continue
			}
			key := d.ID + "\x00" + ref
			if seen[key] {
				continue
			}
			seen[key] = true
			if _, err := insLink.Exec(d.ID, ref); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

// Load 从存储读回 Document 列表（含 Refs，可直接 graph.Build）。
// 存储文件不存在 → 返回空列表（对账刷新"首次"场景：等价全新增）。
// 文件存在但从未写入（空库/无 documents 表）→ 同样视为无旧状态（v0.1.3）。
func Load(dbPath string) ([]*adapter.Document, error) {
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return nil, nil
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	var tbl string
	if err := db.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name='documents'`).Scan(&tbl); err != nil {
		return nil, nil // 空库/半成品 → 无旧状态
	}

	rows, err := db.Query(`SELECT id, title, type, path, mtime, size, tags, aliases, text FROM documents`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	docs := []*adapter.Document{}
	byID := map[string]*adapter.Document{}
	for rows.Next() {
		var d adapter.Document
		var mtime int64
		var tagsJSON, aliasesJSON string
		if err := rows.Scan(&d.ID, &d.Title, &d.Type, &d.Path, &mtime, &d.Size,
			&tagsJSON, &aliasesJSON, &d.Text); err != nil {
			return nil, err
		}
		d.MTime = time.Unix(mtime, 0)
		json.Unmarshal([]byte(tagsJSON), &d.Tags)
		json.Unmarshal([]byte(aliasesJSON), &d.Aliases)
		docs = append(docs, &d)
		byID[d.ID] = &d
	}
	rows.Close()

	// 有向引用行回读：a 链接 b → byID[a].Refs += b（v0.1.5 起 links 保方向，
	// 无向语义由 graph.Build 层补全）
	lrows, err := db.Query(`SELECT a, b FROM links`)
	if err != nil {
		return nil, err
	}
	defer lrows.Close()
	for lrows.Next() {
		var a, b string
		if err := lrows.Scan(&a, &b); err != nil {
			return nil, err
		}
		if d, ok := byID[a]; ok {
			d.Refs = append(d.Refs, b)
		}
	}
	return docs, nil
}
