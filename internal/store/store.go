// ============================================================================
// 文件：internal/store/store.go
// 模块：bbolt 持久化（设计 §6.8 主存储；#16 从 SQLite 迁移，2026-08-25）
//
// ▍职责
//
//	Document 列表 ⇄ bbolt 的存取。v1 写语义 = 幂等全量重写（对比差值，只写
//	变更——#16 P1 增量写，成本随变化量）；对账刷新（internal/sync + cmd
//	refresh）依赖本模块作为"上次状态"。
//
// ▍存储布局（#16：四表 → 四 bucket，一一映射）
//
//	 默认路径 <vault>/.serendipity/db-<库路径 hash12>.bbolt（DBPath），多库各一文件，
//	 便携闭环（库在哪图在哪，见设计 §6.8）。四个 bucket：
//	   docs    id → docRow JSON（Document 去 Refs 序列化；Refs 单独存 links）
//	   links   a\x00b → 1.0（有向引用行，v0.1.5 修正保方向）
//	   renames old → new（改名迁移映射，v0.1.5 修订 #8）
//	 注：touch 自 v0.1.14（§3.7）拆为独立 store touch-<hash>.bbolt，见 touch.go。
//
// ▍links 的方向性（v0.1.5 修正，对账收敛的前提）
//
//	Document.Refs 是有向的（本文档链接谁）；对账 diff 按 ID 逐文档比较 Refs，
//	因此存储必须保方向。Save 按精确 (a,b) 去重（每文档自己的 Refs 无重复），
//	Load 原样回读；无向语义只在 graph.Build 层体现（双方入邻接表）。
//
// ▍与对账的关系（v0.1.2）
//   - Load 对不存在的文件返回空列表（nil, nil）：对账刷新"首次"场景等价全新增；
//   - Save 幂等：任何一次 refresh 后存储即最新状态，重复刷新无副作用；
//   - #16 P1：Save 读库内旧值做差值，只 Put/Delete 变更 key——多次 Save 相同
//     内容零写入。
//
// ▍无迁移（#16 红利）
//
//	bbolt 存的是派生快照（源数据 = vault，源数据权威原则）：换 bbolt 后旧
//	.sqlite 直接删，下次 refresh 重建。扩展名 .sqlite → .bbolt（见 DBPath）。
//
// ▍修改记录
//
//	v0.1.0  初版 Save/Load 全量重写（SQLite）。
//	v0.1.2  Load 支持不存在的存储文件（首次对账全新增）。
//	v0.1.3  Load 兼容"存在但从未写入"的空库文件（无 documents 表 → 无旧状态）。
//	v0.1.13 #16：SQLite → bbolt（四 bucket，签名不变，调用点零改动）；Save 增量写
//	        （P1）；DBPath 扩展名 .bbolt。
//	v0.1.14 M2 §3.7：touch 迁出至独立 store（touch.go），图库 bucket 收敛为三个。
//
// ============================================================================
package store

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	bolt "go.etcd.io/bbolt"
	"serendipity-engine/internal/adapter"
)

// bucket 名（#16：四表 → 四 bucket 一一映射；touch 于 v0.1.14 迁出独立 store）
var (
	bDocs    = []byte("docs")
	bLinks   = []byte("links")
	bRenames = []byte("renames")
	bMeta    = []byte("meta")
)

// metaParserVersion meta 桶里的解析器版本键（v0.2.1）：存"最后一次写此 store 的
// seren 版本"。loadSource/refreshParse 据此判断是否过期——任一解析规则/算法变化
// 都会 bump 版本，从而自动触发全量重析（否则增量复用 mtime/size 未变的旧文档，
// 升级后旧解析结果不失效，曾导致反斜杠 dangling 残留）。
const metaParserVersion = "parser_version"

// metaProfileSignature meta 桶里的画像签名键（v0.2.1）：存"最后一次建此 store 的画像"
// 签名（hash）。用户改画像（如加 log/index 排除）后签名变化 → 自动全量重析，
// 否则增量会复用未变文件的旧文档，新增排除不生效（log 权重/touch 残留等）。
const metaProfileSignature = "profile_signature"

// SaveParserVersion 记录写入该 store 的解析器版本。
func SaveParserVersion(dbPath, ver string) error {
	db, err := open(dbPath)
	if err != nil {
		return err
	}
	defer db.Close()
	return db.Update(func(tx *bolt.Tx) error {
		mb, err := tx.CreateBucketIfNotExists(bMeta)
		if err != nil {
			return err
		}
		return mb.Put([]byte(metaParserVersion), []byte(ver))
	})
}

// LoadParserVersion 读取该 store 记录的解析器版本；无 meta/键 → ""（旧库/未记录）。
// 文件不存在 → ""（不创建文件，避免版本检查的副作用）。
func LoadParserVersion(dbPath string) string {
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return ""
	}
	db, err := open(dbPath)
	if err != nil {
		return ""
	}
	defer db.Close()
	var ver string
	_ = db.View(func(tx *bolt.Tx) error {
		if mb := tx.Bucket(bMeta); mb != nil {
			ver = string(mb.Get([]byte(metaParserVersion)))
		}
		return nil
	})
	return ver
}

// SaveProfileSignature 记录建当前 store 所用的画像签名。
func SaveProfileSignature(dbPath, sig string) error {
	db, err := open(dbPath)
	if err != nil {
		return err
	}
	defer db.Close()
	return db.Update(func(tx *bolt.Tx) error {
		mb, err := tx.CreateBucketIfNotExists(bMeta)
		if err != nil {
			return err
		}
		return mb.Put([]byte(metaProfileSignature), []byte(sig))
	})
}

// LoadProfileSignature 读取建 store 用的画像签名；无 meta/键 → ""（旧库/未记录）。
// 文件不存在 → ""（不创建文件）。
func LoadProfileSignature(dbPath string) string {
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return ""
	}
	db, err := open(dbPath)
	if err != nil {
		return ""
	}
	defer db.Close()
	var sig string
	_ = db.View(func(tx *bolt.Tx) error {
		if mb := tx.Bucket(bMeta); mb != nil {
			sig = string(mb.Get([]byte(metaProfileSignature)))
		}
		return nil
	})
	return sig
}

// docRow 是 docs bucket 的持久化形态：Document 去 Refs（Refs 单独存 links
// bucket，Load 时回读拼接——与 SQLite 时代 documents 表不含引用的结构一致）。
type docRow struct {
	ID      string
	Title   string
	Type    string
	Path    string
	MTime   int64
	Size    int64
	Tags    []string
	Aliases []string
	Text    string
}

func rowOf(d *adapter.Document) *docRow {
	return &docRow{ID: d.ID, Title: d.Title, Type: d.Type, Path: d.Path,
		MTime: d.MTime.Unix(), Size: d.Size, Tags: d.Tags, Aliases: d.Aliases, Text: d.Text}
}

func (r *docRow) doc() *adapter.Document {
	return &adapter.Document{ID: r.ID, Title: r.Title, Type: r.Type, Path: r.Path,
		MTime: time.Unix(r.MTime, 0), Size: r.Size, Tags: r.Tags, Aliases: r.Aliases, Text: r.Text}
}

// open 打开（或创建）库文件。bbolt.Open 对不存在文件自动创建；调用方如需
// "不存在 = 空"语义（Load/TouchStats/LoadRenames）须先 os.Stat 判断。
// #16 P2（顺手）：NoSync —— 每事务跳过 fsync，AppendTouch 高频写入从
// 毫秒级事务变微秒级；本地持久化 + 单进程，崩溃最多丢最近一次事务
// （touch 埋点属可重建反馈数据，非源数据权威，风险可接受）。
func open(dbPath string) (*bolt.DB, error) {
	// bbolt.Open 会创建文件但不会创建父目录（<vault>/.serendipity/）；serve 的
	// /api/refresh 首次写库即在此失败（"cannot find path specified"）。此处建父目录。
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, err
	}
	return bolt.Open(dbPath, 0o600, &bolt.Options{NoSync: true})
}

// ensureBuckets 建图库的三个 bucket（幂等；首次 Save/SaveRenames 时调用）。
// 注：touch 于 v0.1.14 迁出独立 store（touch.go），图库不再持有 touch bucket。
func ensureBuckets(db *bolt.DB) error {
	return db.Update(func(tx *bolt.Tx) error {
		for _, name := range [][]byte{bDocs, bLinks, bRenames} {
			if _, err := tx.CreateBucketIfNotExists(name); err != nil {
				return err
			}
		}
		return nil
	})
}

// LoadRenames 读回改名迁移映射（v0.1.5，修订 #8）：renames bucket = 持久化的
// 身份迁移层（old_id → new_id）。documents/links 存文件真相（原始 Refs），
// 图构建时叠加本映射重定向——改名是持久身份事实，不能只在下一次刷新生效。
// 库/桶不存在（旧库/从未改名）→ 返回空映射。
func LoadRenames(dbPath string) (map[string]string, error) {
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return map[string]string{}, nil
	}
	db, err := open(dbPath)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	out := map[string]string{}
	err = db.View(func(tx *bolt.Tx) error {
		tb := tx.Bucket(bRenames)
		if tb == nil {
			return nil
		}
		return tb.ForEach(func(k, v []byte) error {
			out[string(k)] = string(v)
			return nil
		})
	})
	return out, err
}

// SaveRenames 全量重写 renames bucket（幂等；每次刷新后与 documents 一起落盘）。
func SaveRenames(dbPath string, renames map[string]string) error {
	db, err := open(dbPath)
	if err != nil {
		return err
	}
	defer db.Close()
	return db.Update(func(tx *bolt.Tx) error {
		tb, err := tx.CreateBucketIfNotExists(bRenames)
		if err != nil {
			return err
		}
		if err := tb.ForEach(func(k, _ []byte) error { return tb.Delete(k) }); err != nil {
			return err
		}
		for o, n := range renames {
			if err := tb.Put([]byte(o), []byte(n)); err != nil {
				return err
			}
		}
		return nil
	})
}

// DBPath 返回库的默认存储路径：<vault>/.serendipity/db-<路径hash>.bbolt
// （设计 §6.8 多库：每库一 DB，便携闭环）。#16：扩展名 .sqlite → .bbolt
// （无迁移——旧文件直接删，refresh 重建）。
func DBPath(vault string) string {
	h := sha256.Sum256([]byte(filepath.Clean(vault)))
	hash := hex.EncodeToString(h[:])[:12]
	dir := filepath.Join(vault, ".serendipity")
	return filepath.Join(dir, "db-"+hash+".bbolt")
}

// Save 写图：documents + links（有向边）。#16 P1 增量写：读库内旧值做差值，
// 只 Put/Delete 变更 key——成本随变化量，重复 Save 相同内容零写入。
func Save(dbPath string, docs []*adapter.Document) error {
	db, err := open(dbPath)
	if err != nil {
		return err
	}
	defer db.Close()
	return db.Update(func(tx *bolt.Tx) error {
		dbB, err := tx.CreateBucketIfNotExists(bDocs)
		if err != nil {
			return err
		}
		lb, err := tx.CreateBucketIfNotExists(bLinks)
		if err != nil {
			return err
		}
		// 新状态（按 id 收集；links 精确 (a,b) 去重，v0.1.5 有向语义）
		newDocs := map[string][]byte{}
		newLinks := map[string][]byte{}
		for _, d := range docs {
			b, err := json.Marshal(rowOf(d))
			if err != nil {
				return err
			}
			newDocs[d.ID] = b
			seen := map[string]bool{}
			for _, ref := range d.Refs {
				if ref == d.ID {
					continue
				}
				key := d.ID + "\x00" + ref
				if seen[key] {
					continue
				}
				seen[key] = true
				newLinks[key] = []byte{1}
			}
		}
		// 差值写（P1）：删除库里已消失的，Put 新增/变更的
		if err := dbB.ForEach(func(k, old []byte) error {
			if _, ok := newDocs[string(k)]; !ok {
				return dbB.Delete(k)
			}
			return nil
		}); err != nil {
			return err
		}
		for id, b := range newDocs {
			if old := dbB.Get([]byte(id)); !bytes.Equal(old, b) {
				if err := dbB.Put([]byte(id), b); err != nil {
					return err
				}
			}
		}
		if err := lb.ForEach(func(k, _ []byte) error {
			if _, ok := newLinks[string(k)]; !ok {
				return lb.Delete(k)
			}
			return nil
		}); err != nil {
			return err
		}
		for k := range newLinks {
			if lb.Get([]byte(k)) == nil {
				if err := lb.Put([]byte(k), newLinks[k]); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

// Load 从存储读回 Document 列表（含 Refs，可直接 graph.Build）。
// 存储文件不存在 → 返回空列表（对账刷新"首次"场景：等价全新增）。
// 文件存在但从未写入（空库/无 docs 桶）→ 同样视为无旧状态（v0.1.3）。
func Load(dbPath string) ([]*adapter.Document, error) {
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return nil, nil
	}
	db, err := open(dbPath)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	docs := []*adapter.Document{}
	err = db.View(func(tx *bolt.Tx) error {
		dbB := tx.Bucket(bDocs)
		if dbB == nil {
			return nil // 空库/半成品 → 无旧状态
		}
		byID := map[string]*adapter.Document{}
		if err := dbB.ForEach(func(k, v []byte) error {
			var r docRow
			if err := json.Unmarshal(v, &r); err != nil {
				return err
			}
			d := r.doc()
			docs = append(docs, d)
			byID[string(k)] = d
			return nil
		}); err != nil {
			return err
		}
		// 有向引用行回读：a 链接 b → byID[a].Refs += b（v0.1.5 起 links 保方向）
		lb := tx.Bucket(bLinks)
		if lb == nil {
			return nil
		}
		return lb.ForEach(func(k, _ []byte) error {
			a, b, ok := bytes.Cut(k, []byte{0})
			if !ok {
				return nil // 防御：畸形键跳过
			}
			if d, ok := byID[string(a)]; ok {
				d.Refs = append(d.Refs, string(b))
			}
			return nil
		})
	})
	return docs, err
}
