// ============================================================================
// 文件：internal/store/touch.go
// 模块：touch 行为信号子系统（backlog §3.7，M2，2026-08-26 定稿）
//
// ▍职责
//
//	touch 从图库 db-<hash>.bbolt 拆为独立 store touch-<hash>.bbolt（§3.7.1：
//	修复原 bug——touch 是不可从 vault 派生的原始行为信号，不该被"源数据权威
//	原则"允许删除的可重建派生快照连坐）。本文件 = 独立 store + digest 全部逻辑。
//
// ▍存储布局（三个 bucket）
//
//	touch    seq(8B BE uint64) → {ts,target,src} JSON（自增事件流，上限 5000 截断）
//	meta     last_seq / last_digest_ts / last_digest_id / last_digest / last_ack_id
//	backups  <digest id> → 聚合快照 JSON（算法长期记忆，TopN，backup_max 轮转）
//
// ▍digest（§3.7.2–3.7.4）
//
//	触发：计数优先（自上次 digest 起累计 ≥ digest_count）+ 间隔兜底
//	     （距上次 ≥ digest_days，仅上次存在时参与）+ serve 启动补查。
//	     digest 在 touch 截断之前生成——通知先于淘汰（§3.7.2 顺序铁律）。
//	出口：REST GET /api/touch/digest + MCP seren_touch_digest（只读、被动查询）；
//	     **引擎零写 vault**（§3.7.3 定稿）——digest md 由前端插件导出。
//	备份：每次 digest 滚一份 TopN 聚合快照入 backups，backup_max 轮转（§3.7.4）。
//
// ▍红线：touch 只读、不演化边权；digest 仅只读接口暴露，绝不反馈排序/hot。
//
// ▍修改记录
//
//	v0.1.4  AppendTouch/TouchCount（反馈埋点，独立 bucket + 容量上限）。
//	v0.1.5  RenameTouch（改名迁移，修订 #8）。
//	v0.1.14 M2 §3.7：touch 拆独立 store + digest 子系统（触发/生成/备份/ack/配置）。
//
// ============================================================================
package store

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"

	bolt "go.etcd.io/bbolt"
	"gopkg.in/yaml.v3"
)

// touchMax 反馈埋点（touch）bucket 容量上限：保留最近 N 条，防无限增长
// （克制设计 v0.1.4：埋点只记录不演化，杜绝"点击→边权变→结果变→再点击"正反馈）。
const touchMax = 5000

// touch 独立 store 的 bucket 名（§3.7.1）
var (
	bTouchB    = []byte("touch")   // 事件流
	bTouchMeta = []byte("meta")    // digest/ack 状态
	bBackups   = []byte("backups") // 聚合快照（轮转）
)

// touchEntry 是 touch bucket 的值（ts 秒级；src 可为空）。
type touchEntry struct {
	TS     int64  `json:"ts"`
	Target string `json:"target"`
	Src    string `json:"src"`
}

// TouchDBPath 返回 touch 独立 store 路径：<vault>/.serendipity/touch-<hash>.bbolt
// （与图库 DBPath 同 hash 派生——同一 vault 的 touch 与图库成对；虎鲸库由调用方
// 传库所在目录，见 cmd/seren storePathFor 同逻辑）。
func TouchDBPath(vault string) string {
	h := sha256.Sum256([]byte(filepath.Clean(vault)))
	hash := hex.EncodeToString(h[:])[:12]
	dir := filepath.Join(vault, ".serendipity")
	return filepath.Join(dir, "touch-"+hash+".bbolt")
}

// AppendTouch 记录一次节点点击（反馈埋点，独立 store——图库重建不清除）。
// 容量上限：超出 touchMax 时删除最旧记录（seq 大端键天然有序，从 First 删）。
// 失败静默由调用方决定（埋点不影响主流程）。
func AppendTouch(dbPath, target, from string) error {
	db, err := open(dbPath)
	if err != nil {
		return err
	}
	defer db.Close()
	return db.Update(func(tx *bolt.Tx) error {
		tb, err := tx.CreateBucketIfNotExists(bTouchB)
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
		tb := tx.Bucket(bTouchB)
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
// §3.7.1：touch 独立 store 后幽灵过滤跨库——graphPath（图库）docs 桶查存在性 O(1)。
func TouchStats(touchPath, graphPath string, limit int) (total int, targets, sources []TouchRow, err error) {
	if limit <= 0 {
		limit = 10
	}
	if _, err := os.Stat(touchPath); os.IsNotExist(err) {
		return 0, nil, nil, nil // 无库 = 全零
	}
	db, err := open(touchPath)
	if err != nil {
		return 0, nil, nil, err
	}
	defer db.Close()
	graphHasDoc := docExistence(graphPath)
	err = db.View(func(tx *bolt.Tx) error {
		tb := tx.Bucket(bTouchB)
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
		targets = topTouchRows(targetCnt, limit, graphHasDoc)
		sources = topTouchRows(srcCnt, limit, nil) // src 自由文本，不过滤
		return nil
	})
	return total, targets, sources, err
}

// docExistence 返回"图库 docs 桶中是否存在该 ID"的判定函数（幽灵过滤）。
// 图库缺失 / 无 docs 桶 → 一律视为不存在（targets 保守为空——避免已删节点上榜）。
// 注：每次调用打开一次图库读，TouchStats/digest 低频场景可接受。
func docExistence(graphPath string) func(string) bool {
	return func(id string) bool {
		if graphPath == "" {
			return false
		}
		if _, err := os.Stat(graphPath); os.IsNotExist(err) {
			return false
		}
		db, err := open(graphPath)
		if err != nil {
			return false
		}
		defer db.Close()
		ok := false
		_ = db.View(func(tx *bolt.Tx) error {
			dbB := tx.Bucket(bDocs)
			if dbB != nil {
				ok = dbB.Get([]byte(id)) != nil
			}
			return nil
		})
		return ok
	}
}

// docTitle 从图库读节点标题（digest 聚合展示用）。图库缺失/无该节点 → 空串。
func docTitle(graphPath, id string) string {
	if graphPath == "" {
		return ""
	}
	if _, err := os.Stat(graphPath); os.IsNotExist(err) {
		return ""
	}
	db, err := open(graphPath)
	if err != nil {
		return ""
	}
	defer db.Close()
	title := ""
	_ = db.View(func(tx *bolt.Tx) error {
		dbB := tx.Bucket(bDocs)
		if dbB == nil {
			return nil
		}
		v := dbB.Get([]byte(id))
		if v == nil {
			return nil
		}
		var r docRow
		if err := json.Unmarshal(v, &r); err == nil {
			title = r.Title
		}
		return nil
	})
	return title
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
		tb := tx.Bucket(bTouchB)
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

// ============================================================================
// digest 子系统（§3.7.2–3.7.5）
// ============================================================================

// meta 键名（§3.7.1：meta bucket 存 digest/ack 状态）
var (
	mLastSeq      = []byte("last_seq")        // 上次 digest 时的 touch seq（窗口起点）
	mLastDigestTS = []byte("last_digest_ts")  // 上次 digest 生成时间（unix 秒）
	mLastDigestID = []byte("last_digest_id")  // 上次 digest 唯一 id
	mLastDigest   = []byte("last_digest")     // 上次 digest 完整内容（JSON）
	mLastAckID    = []byte("last_ack_id")     // 已读 digest id（ack）
)

// TouchConfig touch 行为信号参数（§3.7.5，touch.yaml，与 profile.yaml 同 convention）。
type TouchConfig struct {
	DigestCount int `yaml:"digest_count"` // 计数触发阈值（主），[200,600]
	DigestDays  int `yaml:"digest_days"`  // 间隔触发阈值，天（兜底），[1,5]
	BackupMax   int `yaml:"backup_max"`   // 聚合快照保留上限，[1,20]
}

// DefaultTouchConfig 缺省参数（§3.7.5）。
func DefaultTouchConfig() TouchConfig {
	return TouchConfig{DigestCount: 500, DigestDays: 3, BackupMax: 5}
}

// LoadTouchConfig 从 <vault>/.serendipity/touch.yaml 加载并钳制到区间；
// 文件缺失 → 默认；越界字段钳制（与 profile.yaml fillDefaults 同精神）。
func LoadTouchConfig(vault string) TouchConfig {
	cfg := DefaultTouchConfig()
	b, err := os.ReadFile(filepath.Join(vault, ".serendipity", "touch.yaml"))
	if err != nil {
		return cfg // 无配置文件 = 默认
	}
	var c struct {
		DigestCount *int `yaml:"digest_count"`
		DigestDays  *int `yaml:"digest_days"`
		BackupMax   *int `yaml:"backup_max"`
	}
	if err := yaml.Unmarshal(b, &c); err != nil {
		return cfg // 解析失败 = 默认（配置损坏不阻断 serve）
	}
	if c.DigestCount != nil {
		cfg.DigestCount = clamp(*c.DigestCount, 200, 600)
	}
	if c.DigestDays != nil {
		cfg.DigestDays = clamp(*c.DigestDays, 1, 5)
	}
	if c.BackupMax != nil {
		cfg.BackupMax = clamp(*c.BackupMax, 1, 20)
	}
	return cfg
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// DigestTarget digest 聚合行（带标题，供展示"X/Y/Z 聚成簇"）。
type DigestTarget struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Count int    `json:"count"`
}

// Digest digest 内容（§3.7.2：窗口聚合 TopN + 时间跨度 + 新增总数）。
type Digest struct {
	ID          string         `json:"id"`           // 唯一 id（unix 纳秒）
	GeneratedAt int64          `json:"generated_at"` // 生成时间（unix 秒）
	WindowStart int64          `json:"window_start"` // 窗口起点（上次 digest 时间，unix 秒）
	Since       string         `json:"since"`        // 窗口起点人读串
	Total       int            `json:"total"`        // 窗口新增 touch 数
	Targets     []DigestTarget `json:"targets"`      // TopN（幽灵过滤 + 标题）
	Sources     []TouchRow     `json:"sources"`      // TopN 来源词
}

// MaybeDigest 检查阈值，达标则生成 digest + 备份（计数优先，间隔兜底）。
// 返回是否生成了 digest。touch 库不存在（从未埋点）→ 不生成。
// 顺序铁律（§3.7.2）：digest 生成发生在 touch 截断之前——AppendTouch 写满即
// 截断，而计数阈值 500 << touchMax 5000，窗口事件绝无被截断的可能，铁律天然满足。
func MaybeDigest(touchPath, graphPath string, cfg TouchConfig) (bool, error) {
	if _, err := os.Stat(touchPath); os.IsNotExist(err) {
		return false, nil
	}
	db, err := open(touchPath)
	if err != nil {
		return false, err
	}
	defer db.Close()

	var curSeq uint64
	var lastSeq uint64
	var lastTS int64
	windowStart := time.Now().Unix()
	err = db.View(func(tx *bolt.Tx) error {
		tb := tx.Bucket(bTouchB)
		if tb == nil {
			return nil // 无事件 → 无 digest
		}
		curSeq = tb.Sequence()
		mb := tx.Bucket(bTouchMeta)
		if mb != nil {
			lastSeq = beUint(mb.Get(mLastSeq))
			if v := mb.Get(mLastDigestTS); len(v) > 0 {
				lastTS = atoi64(string(v))
				windowStart = lastTS
			}
		}
		return nil
	})
	if err != nil {
		return false, err
	}
	if curSeq == 0 {
		return false, nil // 从未有事件
	}
	// 计数触发（主）：自上次 digest 起累计 ≥ digest_count
	countSince := int(curSeq - lastSeq)
	intervalDue := lastTS != 0 && time.Since(time.Unix(lastTS, 0)) >= time.Duration(cfg.DigestDays)*24*time.Hour
	if countSince < cfg.DigestCount && !intervalDue {
		return false, nil
	}

	// 生成 digest：遍历窗口事件聚合（seq > lastSeq）
	var entries []touchEntry
	err = db.View(func(tx *bolt.Tx) error {
		tb := tx.Bucket(bTouchB)
		if tb == nil {
			return nil
		}
		startKey := make([]byte, 8)
		binary.BigEndian.PutUint64(startKey, lastSeq+1)
		c := tb.Cursor()
		for k, v := c.Seek(startKey); k != nil; k, v = c.Next() {
			var e touchEntry
			if err := json.Unmarshal(v, &e); err != nil {
				return err
			}
			entries = append(entries, e)
		}
		return nil
	})
	if err != nil {
		return false, err
	}
	dig := buildDigest(entries, windowStart, graphPath)
	dig.ID = fmt.Sprintf("%d", time.Now().UnixNano())
	dig.GeneratedAt = time.Now().Unix()

	// 写 meta + 备份（单事务）
	err = db.Update(func(tx *bolt.Tx) error {
		mb, err := tx.CreateBucketIfNotExists(bTouchMeta)
		if err != nil {
			return err
		}
		seqKey := make([]byte, 8)
		binary.BigEndian.PutUint64(seqKey, curSeq)
		if err := mb.Put(mLastSeq, seqKey); err != nil {
			return err
		}
		if err := mb.Put(mLastDigestTS, []byte(strconv.FormatInt(dig.GeneratedAt, 10))); err != nil {
			return err
		}
		if err := mb.Put(mLastDigestID, []byte(dig.ID)); err != nil {
			return err
		}
		dbJSON, err := json.Marshal(dig)
		if err != nil {
			return err
		}
		if err := mb.Put(mLastDigest, dbJSON); err != nil {
			return err
		}
		// 备份（§3.7.4）：TopN 快照入 backups，backup_max 轮转
		bb, err := tx.CreateBucketIfNotExists(bBackups)
		if err != nil {
			return err
		}
		snap, err := json.Marshal(dig.Targets)
		if err != nil {
			return err
		}
		if err := bb.Put([]byte(dig.ID), snap); err != nil {
			return err
		}
		return rotateBucket(bb, cfg.BackupMax)
	})
	if err != nil {
		return false, err
	}
	return true, nil
}

// buildDigest 从窗口事件聚合 digest 内容（幽灵过滤 + 标题解析跨图库）。
func buildDigest(entries []touchEntry, windowStart int64, graphPath string) *Digest {
	dig := &Digest{WindowStart: windowStart, Since: time.Unix(windowStart, 0).Format("2006-01-02 15:04")}
	dig.Total = len(entries)
	hasDoc := docExistence(graphPath)
	targetCnt := map[string]int{}
	srcCnt := map[string]int{}
	for _, e := range entries {
		if e.Target != "" {
			targetCnt[e.Target]++
		}
		if e.Src != "" {
			srcCnt[e.Src]++
		}
	}
	for _, r := range topTouchRows(targetCnt, 10, hasDoc) {
		dig.Targets = append(dig.Targets, DigestTarget{ID: r.ID, Title: docTitle(graphPath, r.ID), Count: r.Count})
	}
	dig.Sources = topTouchRows(srcCnt, 10, nil)
	return dig
}

// rotateBucket 保留最近 maxN 条键（键按字典序=时间序），超则删最旧。
func rotateBucket(b *bolt.Bucket, maxN int) error {
	if maxN <= 0 {
		return nil
	}
	var keys [][]byte
	c := b.Cursor()
	for k, _ := c.First(); k != nil; k, _ = c.Next() {
		keys = append(keys, append([]byte(nil), k...))
	}
	for i := 0; i < len(keys)-maxN; i++ {
		if err := b.Delete(keys[i]); err != nil {
			return err
		}
	}
	return nil
}

// LatestDigest 返回最新 digest（无 → nil, nil）。
func LatestDigest(touchPath string) (*Digest, error) {
	if _, err := os.Stat(touchPath); os.IsNotExist(err) {
		return nil, nil
	}
	db, err := open(touchPath)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	var dig *Digest
	err = db.View(func(tx *bolt.Tx) error {
		mb := tx.Bucket(bTouchMeta)
		if mb == nil {
			return nil
		}
		v := mb.Get(mLastDigest)
		if v == nil {
			return nil
		}
		var d Digest
		if err := json.Unmarshal(v, &d); err != nil {
			return err
		}
		dig = &d
		return nil
	})
	return dig, err
}

// DigestAvailable 是否有未被 ack 的新 digest（/api/stats.digest_available 的开关）。
func DigestAvailable(touchPath string) bool {
	dig, err := LatestDigest(touchPath)
	if err != nil || dig == nil {
		return false
	}
	db, err := open(touchPath)
	if err != nil {
		return false
	}
	defer db.Close()
	acked := false
	_ = db.View(func(tx *bolt.Tx) error {
		mb := tx.Bucket(bTouchMeta)
		if mb == nil {
			return nil
		}
		acked = string(mb.Get(mLastAckID)) == dig.ID
		return nil
	})
	return !acked
}

// AckDigest 标记 digest 已读（只写 meta，不碰 touch 事件、不反馈排序）。
func AckDigest(touchPath, id string) error {
	db, err := open(touchPath)
	if err != nil {
		return err
	}
	defer db.Close()
	return db.Update(func(tx *bolt.Tx) error {
		mb, err := tx.CreateBucketIfNotExists(bTouchMeta)
		if err != nil {
			return err
		}
		return mb.Put(mLastAckID, []byte(id))
	})
}

// ---- 小工具 ----

func beUint(b []byte) uint64 {
	if len(b) != 8 {
		return 0
	}
	return binary.BigEndian.Uint64(b)
}

func atoi64(s string) int64 {
	n, _ := strconv.ParseInt(s, 10, 64)
	return n
}
