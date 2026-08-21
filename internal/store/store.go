// Package store 实现设计 §6.8 的 SQLite 持久化（主存储）。
// v1 写语义：全量重写（幂等）；对账/增量同步是 v1.5（启动对账章节）。
package store

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
	"serendipity-engine/internal/adapter"
)

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
func Load(dbPath string) ([]*adapter.Document, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}
	defer db.Close()

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
