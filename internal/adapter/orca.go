package adapter

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite" // 纯 Go SQLite，零 CGO（设计 §6.8）
)

// Orca 格式实测（详见 docs/design.md §6.9.1）：
//   Block(id, content JSON, text, modified, parent, left)  — content IS NULL = 文档根节点
//   BlockRef(id, f, t, type, alias)                        — 三种引用（1=内嵌 2=带属性 3=关联）
//   BlockAlias(name, block, pos)                           — title（块可多别名）
//   ⚠️ Repo 表含用户凭据（API key 等）——本适配器绝不读取。

// ParseOrcaDB 从虎鲸库 SQLite 的拷贝解析出 Document 列表（安全红线：先拷贝再读，Repo 表不碰）。
// 映射到统一图格式：
//   节点 = Block（文档根节点 type=doc，内容块 type=block）
//   title = BlockAlias（别名即 title，锚定层直接用）
//   边   = BlockRef(f→t) + 层级边(parent→child，包含关系)
func ParseOrcaDB(dbPath string) ([]*Document, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("打开虎鲸库: %w", err)
	}
	defer db.Close()
	// 只读安全（拷贝已保证不锁活库）
	if _, err := db.Exec("PRAGMA query_only = ON"); err != nil {
		return nil, err
	}

	// 1. blocks
	type blk struct {
		id       int64
		content  sql.NullString
		text     sql.NullString
		modified int64
		parent   sql.NullInt64
	}
	blocks := map[int64]*blk{}
	rows, err := db.Query(`SELECT id, content, text, modified, parent FROM Block`)
	if err != nil {
		return nil, fmt.Errorf("读 Block: %w", err)
	}
	for rows.Next() {
		b := &blk{}
		if err := rows.Scan(&b.id, &b.content, &b.text, &b.modified, &b.parent); err != nil {
			rows.Close()
			return nil, err
		}
		blocks[b.id] = b
	}
	rows.Close()

	// 2. aliases（title）
	aliasMap := map[int64][]string{}
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
		aliasMap[blkID] = append(aliasMap[blkID], name)
	}
	rows.Close()

	// 3. refs（边）
	refMap := map[int64][]string{}
	rows, err = db.Query(`SELECT f, t FROM BlockRef`)
	if err != nil {
		return nil, fmt.Errorf("读 BlockRef: %w", err)
	}
	for rows.Next() {
		var f, t int64
		if err := rows.Scan(&f, &t); err != nil {
			rows.Close()
			return nil, err
		}
		refMap[f] = append(refMap[f], strconv.FormatInt(t, 10))
	}
	rows.Close()

	// 4. 组装 Documents
	docs := make([]*Document, 0, len(blocks))
	for id, b := range blocks {
		docType := "block"
		if !b.content.Valid {
			docType = "doc"
		}
		text := ""
		if b.text.Valid {
			text = b.text.String
		}
		refs := refMap[id]
		// 层级边：块知道自己的父（包含关系 → 图边，v1 按无向关联）
		if b.parent.Valid {
			refs = append(refs, strconv.FormatInt(b.parent.Int64, 10))
		}
		aliases := aliasMap[id]
		title := orcaTitle(id, aliases, text)
		docs = append(docs, &Document{
			ID:      strconv.FormatInt(id, 10),
			Title:   title,
			Aliases: aliases,
			Type:    docType,
			Path:    "block/" + strconv.FormatInt(id, 10),
			MTime:   time.Unix(b.modified, 0),
			Size:    int64(len(text)),
			Refs:    refs,
			Text:    text,
		})
	}
	return docs, nil
}

// orcaTitle：别名优先，其次正文首行（截断），最后兜底块号。
func orcaTitle(id int64, aliases []string, text string) string {
	if len(aliases) > 0 {
		return aliases[0]
	}
	if t := strings.TrimSpace(text); t != "" {
		line := t
		if i := strings.IndexByte(line, '\n'); i > 0 {
			line = line[:i]
		}
		runes := []rune(strings.TrimSpace(line))
		if len(runes) > 30 {
			runes = runes[:30]
		}
		if s := string(runes); s != "" {
			return s
		}
	}
	return fmt.Sprintf("块#%d", id)
}

// CopyDBForRead 把虎鲸库拷贝到安全位置再读（绝不锁活库、绝不在库内写文件）。
// 返回拷贝路径与清理函数；拷贝优先放系统临时目录，失败则退回当前目录。
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
		in, err := os.Open(src)
		if err != nil {
			os.Remove(dst)
			lastErr = err
			continue
		}
		out, err := os.OpenFile(dst, os.O_WRONLY|os.O_TRUNC, 0)
		if err != nil {
			in.Close()
			os.Remove(dst)
			lastErr = err
			continue
		}
		if _, err := out.ReadFrom(in); err != nil {
			in.Close()
			out.Close()
			os.Remove(dst)
			lastErr = err
			continue
		}
		in.Close()
		out.Close()
		return dst, func() { os.Remove(dst) }, nil
	}
	return "", cleanup, lastErr
}

// IsOrcaDB 按扩展名粗判是否为虎鲸库文件。
func IsOrcaDB(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".db"
}
