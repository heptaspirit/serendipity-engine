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
//	   touch   seq(8B BE uint64) → {ts,target,src} JSON（自增事件流，上限 5000 截断）
//	   renames old → new（改名迁移映射，v0.1.5 修订 #8）
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
// ▍反馈埋点（v0.1.4）
//
//	touch bucket 独立于 docs/links：Save 全量重写只清后两个 bucket，touch 保留
//	（增量写入）。容量上限 touchMax=5000，超限删最旧——克制设计防无限增长；
//	埋点只记录不演化边权（杜绝"点击→边权变→结果变→再点击"正反馈跑飞）。
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
//	v0.1.4  AppendTouch/TouchCount（反馈埋点，独立表 + 容量上限）。
//	v0.1.5  RenameTouch（改名迁移，修订 #8：touch 旧 ID → 新 ID，两阶段占位防链式）；
//	        links 改有向引用行（修正回读丢方向的虚假 refs+1 对账不收敛）。
//	v0.1.13 #16：SQLite → bbolt（四 bucket，签名不变，调用点零改动）；Save 增量写
//	        （P1）；幽灵 touch 过滤改 O(1) bucket.Has（P5）；DBPath 扩展名 .bbolt。
//
// ============================================================================
package store

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"time"

	bolt "go.etcd.io/bbolt"
	"serendipity-engine/internal/adapter"
)

// touchMax 反馈埋点（touch）bucket 容量上限：保留最近 N 条，防无限增长
// （克制设计 v0.1.4：埋点只记录不演化，杜绝"点击→边权变→结果变→再点击"正反馈）。
const touchMax = 5000

// bucket 名（#16：四表 → 四 bucket 一一映射）
var (
	bDocs    = []byte("docs")
	bLinks   = []byte("links")
	bTouch   = []byte("touch")
	bRenames = []byte("renames")
)

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

// touchEntry 是 touch bucket 的值（ts 秒级；src 可为空）。
type touchEntry struct {
	TS     int64  `json:"ts"`
	Target string `json:"target"`
	Src    string `json:"src"`
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

// ensureBuckets 建四个 bucket（幂等；首次 Save/SaveRenames 时调用）。
func ensureBuckets(db *bolt.DB) error {
	return db.Update(func(tx *bolt.Tx) error {
		for _, name := range [][]byte{bDocs, bLinks, bTouch, bRenames} {
			if _, err := tx.CreateBucketIfNotExists(name); err != nil {
				return err
			}
		}
		return nil
	})
}

// AppendTouch 记录一次节点点击（反馈埋点，独立 bucket——Save 全量重写不清除）。
// 容量上限：超出 touchMax 时删除最旧记录（seq 大端键天然有序，从 First 删）。
func AppendTouch(dbPath, target, from string) error {
	db, err := open(dbPath)
	if err != nil {
		return err
	}
	defer db.Close()
	return db.Update(func(tx *bolt.Tx) error {
		tb, err := tx.CreateBucketIfNotExists(bTouch)
		if err != nil {
			return err
		}
		seq, err := tb.NextSequence()
		if err != nil {
			return err
		}
		key := make([]byte, 8)
		binary.BigEndian.PutUint64(key, seq)
		val, err := json.Marshal(touchEntry{TS: time.Now().Unix(), Target: target, Src: from})
		if err != nil {
			return err
		}
		if err := tb.Put(key, val); err != nil {
			return err
		}
		return truncateTouch(tb)
	})
}

// truncateTouch 超限删最旧（seq 有序键 → 从头删到 <= touchMax）。
// 注：不用 Bucket.Stats().KeyN 短路——写事务内统计是 stale 的（实测 5005 插
// 只删到 5001），必须真实遍历。NoSync 下每次事务微秒级，遍历 5000 条可忽略。
func truncateTouch(tb *bolt.Bucket) error {
	var keys [][]byte
	c := tb.Cursor()
	for k, _ := c.First(); k != nil; k, _ = c.Next() {
		keys = append(keys, append([]byte(nil), k...))
	}
	for i := 0; i < len(keys)-touchMax; i++ {
		if err := tb.Delete(keys[i]); err != nil {
			return err
		}
	}
	return nil
}

// TouchCount 返回已记录的点击数（serve 启动/刷新日志用）。库/桶不存在 = 0。
func TouchCount(dbPath string) (int, error) {
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return 0, nil
	}
	db, err := open(dbPath)
	if err != nil {
		return 0, err
	}
	defer db.Close()
	n := 0
	err = db.View(func(tx *bolt.Tx) error {
		tb := tx.Bucket(bTouch)
		if tb == nil {
			return nil
		}
		return tb.ForEach(func(_, _ []byte) error {
			n++
			return nil
		})
	})
	return n, err
}

// TouchRow 一条 touch 聚合行（目标/来源 + 点击数）。
type TouchRow struct {
	ID    string `json:"id"`
	Count int    `json:"count"`
}

// TouchStats 反馈埋点只读统计（v0.1.11，backlog §3.3 —— 反馈闭环只读第一步）。
// 只读聚合，绝不写库、绝不反馈到排序/hot（红线 2：埋点只记录不演化）。
// 库/桶不存在（从未埋点/旧库）→ 全零统计（不报错，展示友好）。
// v0.1.12（backlog §四 缺口②）：targets 关联 documents 过滤幽灵 touch——
// 点击过但已删的节点不再进热度榜；sources 是自由文本查询词（非节点 ID），不过滤。
// #16 P5：幽灵过滤从 SQL 子查询 join 改为 docs bucket 存在性 O(1)。
func TouchStats(dbPath string, limit int) (total int, targets, sources []TouchRow, err error) {
	if limit <= 0 {
		limit = 10
	}
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return 0, nil, nil, nil // 无库 = 全零
	}
	db, err := open(dbPath)
	if err != nil {
		return 0, nil, nil, err
	}
	defer db.Close()
	err = db.View(func(tx *bolt.Tx) error {
		tb := tx.Bucket(bTouch)
		if tb == nil {
			return nil // 无埋点桶 = 全零
		}
		targetCnt := map[string]int{}
		srcCnt := map[string]int{}
		err := tb.ForEach(func(_, v []byte) error {
			var e touchEntry
			if err := json.Unmarshal(v, &e); err != nil {
				return err
			}
			total++
			if e.Target != "" {
				targetCnt[e.Target]++
			}
			if e.Src != "" {
				srcCnt[e.Src]++
			}
			return nil
		})
		if err != nil {
			return err
		}
		// 幽灵过滤（#16 P5）：docs bucket 存在性 O(1)，替代 SQL join
		hasDoc := func(id string) bool {
			db := tx.Bucket(bDocs)
			return db != nil && db.Get([]byte(id)) != nil
		}
		targets = topTouchRows(targetCnt, limit, hasDoc)
		sources = topTouchRows(srcCnt, limit, nil) // src 自由文本，不过滤
		return nil
	})
	return total, targets, sources, err
}

// topTouchRows 按列分组计数 → TopN（降序，并列按 ID 稳定）。filter 非 nil 时
// 只保留仍存在的节点 ID（幽灵 touch 过滤）；documents 桶不存在 → 结果为空。
func topTouchRows(cnt map[string]int, limit int, filter func(string) bool) []TouchRow {
	out := make([]TouchRow, 0, len(cnt))
	for id, c := range cnt {
		if filter != nil && !filter(id) {
			continue
		}
		out = append(out, TouchRow{ID: id, Count: c})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].ID < out[j].ID
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out
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

// RenameTouch 把 touch 里 target/src 为旧 ID 的埋点迁移到新 ID（v0.1.5，
// 改名迁移，设计修订 #8）。映射先做传递解析（旧→中→新 解到最终目标，与
// ApplyRenames 语义一致）；touch 桶不存在（从未埋点）→ 静默跳过。
// 实现注：bbolt 单写事务内先收集再写，不存在 SQL 逐行 UPDATE 的中间态互踩，
// 无需旧版两阶段占位（占位法保留在 v0.1.5 提交历史中）。
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
	db, err := open(dbPath)
	if err != nil {
		return err
	}
	defer db.Close()
	return db.Update(func(tx *bolt.Tx) error {
		tb := tx.Bucket(bTouch)
		if tb == nil {
			return nil // 无埋点数据
		}
		type upd struct {
			key []byte
			val []byte
		}
		var updates []upd
		c := tb.Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			var e touchEntry
			if err := json.Unmarshal(v, &e); err != nil {
				return err
			}
			changed := false
			if nid, ok := resolved[e.Target]; ok {
				e.Target = nid
				changed = true
			}
			if nid, ok := resolved[e.Src]; ok {
				e.Src = nid
				changed = true
			}
			if changed {
				nv, err := json.Marshal(e)
				if err != nil {
					return err
				}
				updates = append(updates, upd{append([]byte(nil), k...), nv})
			}
		}
		for _, u := range updates {
			if err := tb.Put(u.key, u.val); err != nil {
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
