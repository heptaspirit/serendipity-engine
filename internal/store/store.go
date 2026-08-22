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
//	  links(a, b, weight, PK(a,b)) —— 无向边，pair 去重（Save 内 seen 集合）
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
			key := pairKey(d.ID, ref)
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

func pairKey(a, b string) string {
	if a < b {
		return a + "\x00" + b
	}
	return b + "\x00" + a
}
