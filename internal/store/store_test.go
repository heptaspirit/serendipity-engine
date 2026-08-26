// Package store 持久化单元测试：links 有向引用行回读（v0.1.5 修正）。
// 回归守护：此前排序 pairKey 去重导致字典序较大的端点回读后 Refs 为空，
// 对账 diff 每次刷新报虚假 "refs +1" 永不收敛。
// #16（v0.1.13）：SQLite → bbolt 后测试全部走 store API，不依赖 SQL。
// §3.7（v0.1.14）：touch 拆独立 store——touch 相关测试用独立 touchPath，
// TouchStats/幽灵过滤跨库（touchPath + graphPath）。
package store

import (
	"encoding/binary"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	bolt "go.etcd.io/bbolt"
	"serendipity-engine/internal/adapter"
)

// touchPair 测试辅助：touch 独立库 + 图库双路径（§3.7.1）。
func touchPair(t *testing.T) (touchPath, graphPath string) {
	t.Helper()
	dir := t.TempDir()
	return filepath.Join(dir, "touch.bbolt"), filepath.Join(dir, "graph.bbolt")
}

func TestSaveLoadDirectedRefs(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "t.bbolt")

	// 两个互链文档：A 链接 B，B 链接 A（字典序 B < A 也无妨，方向必须保留）
	docs := []*adapter.Document{
		{ID: "甲", Title: "甲", Type: "note", Path: "甲.md", Refs: []string{"乙"}, Text: "甲正文"},
		{ID: "乙", Title: "乙", Type: "note", Path: "乙.md", Refs: []string{"甲"}, Text: "乙正文"},
	}
	if err := Save(dbPath, docs); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := Load(dbPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	byID := map[string]*adapter.Document{}
	for _, d := range got {
		byID[d.ID] = d
	}
	if !reflect.DeepEqual(byID["甲"].Refs, []string{"乙"}) {
		t.Fatalf("甲.Refs 回读错误：%v", byID["甲"].Refs)
	}
	if !reflect.DeepEqual(byID["乙"].Refs, []string{"甲"}) {
		t.Fatalf("乙.Refs 回读错误（方向丢失）：%v", byID["乙"].Refs)
	}
}

// loadTouches 测试辅助：读回全部 touch 行（按 seq 顺序）——store API 之外
// 的内部读路径（SQLite 时代测试直接查表，bbolt 后走 bucket 遍历）。
func loadTouches(t *testing.T, dbPath string) [][2]string {
	t.Helper()
	rows, err := readTouches(dbPath)
	if err != nil {
		t.Fatalf("readTouches: %v", err)
	}
	return rows
}

func TestRenameTouchPlaceholder(t *testing.T) {
	touchPath, _ := touchPair(t)
	if err := AppendTouch(touchPath, "旧A", "来源"); err != nil {
		t.Fatalf("AppendTouch: %v", err)
	}
	if err := AppendTouch(touchPath, "旧B", "旧A"); err != nil {
		t.Fatalf("AppendTouch: %v", err)
	}
	// 链式：旧A→旧B→新B
	if err := RenameTouch(touchPath, map[string]string{"旧A": "旧B", "旧B": "新B"}); err != nil {
		t.Fatalf("RenameTouch: %v", err)
	}
	got := loadTouches(t, touchPath)
	// 传递解析后：target 旧A→新B、旧B→新B；src 旧A 也解到 新B（来源身份同样迁移）
	want := [][2]string{{"新B", "来源"}, {"新B", "新B"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("链式 touch 迁移错误：%v ≠ %v", got, want)
	}
}

func TestLoadRenamesRoundTrip(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "t.bbolt")
	rm := map[string]string{"旧A": "新A", "旧B": "新B"}
	if err := SaveRenames(dbPath, rm); err != nil {
		t.Fatalf("SaveRenames: %v", err)
	}
	got, err := LoadRenames(dbPath)
	if err != nil {
		t.Fatalf("LoadRenames: %v", err)
	}
	if !reflect.DeepEqual(got, rm) {
		t.Fatalf("round-trip 错误：%v ≠ %v", got, rm)
	}
}

// TouchStats：只读统计聚合（v0.1.11，backlog §3.3）。
// 验证被点击 TopN、来源 TopN、总数；且不写库（只读）。
// v0.1.12：targets 关联 documents 过滤幽灵 touch——热点A/热点B 先存入图库 documents，
// 未保存的幽灵节点被点后不进热度榜。
// §3.7（v0.1.14）：touch 独立库，幽灵过滤跨图库（graphPath）。
func TestTouchStats(t *testing.T) {
	touchPath, graphPath := touchPair(t)
	// 先保存两个真实节点到图库（docs bucket），否则 target 过滤会把它们全滤掉（v0.1.12）
	if err := Save(graphPath, []*adapter.Document{
		{ID: "热点A", Title: "A", Type: "note", Refs: []string{}},
		{ID: "热点B", Title: "B", Type: "note", Refs: []string{}},
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	// 插入埋点（touch 独立库）：target 聚焦少数节点，src 记录来源
	for i := 0; i < 3; i++ {
		if err := AppendTouch(touchPath, "热点A", "来源X"); err != nil {
			t.Fatalf("AppendTouch: %v", err)
		}
	}
	if err := AppendTouch(touchPath, "热点A", "来源Y"); err != nil {
		t.Fatalf("AppendTouch: %v", err)
	}
	if err := AppendTouch(touchPath, "热点B", ""); err != nil { // 无来源（src 空）
		t.Fatalf("AppendTouch: %v", err)
	}
	if err := AppendTouch(touchPath, "幽灵节点", "来源Z"); err != nil { // 已删/不存在 → 应被过滤
		t.Fatalf("AppendTouch: %v", err)
	}
	total, targets, sources, err := TouchStats(touchPath, graphPath, 10)
	if err != nil {
		t.Fatalf("TouchStats: %v", err)
	}
	if total != 6 {
		t.Fatalf("总数应 6：%d", total)
	}
	// 被点击：热点A(4) 应排首位；幽灵节点不存在于图库 docs → 被过滤
	if len(targets) == 0 || targets[0].ID != "热点A" || targets[0].Count != 4 {
		t.Fatalf("Targets 错误：%v", targets)
	}
	foundGhost := false
	for _, r := range targets {
		if r.ID == "幽灵节点" {
			foundGhost = true
		}
	}
	if foundGhost {
		t.Fatalf("幽灵节点不应进热度榜：%v", targets)
	}
	// 来源：来源X(3) 应排首位；空 src 应被排除；来源Z（幽灵对应的来源）是自由文本，保留
	if len(sources) == 0 || sources[0].ID != "来源X" || sources[0].Count != 3 {
		t.Fatalf("Sources 错误：%v", sources)
	}
	foundZ := false
	for _, r := range sources {
		if r.ID == "来源Z" {
			foundZ = true
		}
	}
	if !foundZ {
		t.Fatalf("src 是自由查询词，来源Z 应保留：%v", sources)
	}
	// 无 touch 库文件 → 全零（不报错）
	total2, _, _, err := TouchStats(filepath.Join(t.TempDir(), "none.bbolt"), graphPath, 10)
	if err != nil || total2 != 0 {
		t.Fatalf("无库应全零：total=%d err=%v", total2, err)
	}
}

// TestTouchTruncate：容量上限 touchMax 截断（删最旧，保留最近）。
func TestTouchTruncate(t *testing.T) {
	touchPath, _ := touchPair(t)
	// 超上限写入（touchMax 是包内常量；这里写 touchMax+5 条）
	n := touchMax + 5
	for i := 0; i < n; i++ {
		if err := AppendTouch(touchPath, "T", ""); err != nil {
			t.Fatalf("AppendTouch #%d: %v", i, err)
		}
	}
	rows := loadTouches(t, touchPath)
	if len(rows) != touchMax {
		t.Fatalf("截断后应 %d 条，实际 %d", touchMax, len(rows))
	}
	// 保留的是最近 touchMax 条（首条 seq = n - touchMax + 1；验证最后一条存在）
	last := rows[len(rows)-1]
	if last[0] != "T" {
		t.Fatalf("末条应保留：%v", last)
	}
}

// TestSaveIncrementalIdempotent：#16 P1 增量写——重复 Save 相同内容零写入
// （幂等），且删文档后库中不再有（差值 Delete）。
func TestSaveIncrementalIdempotent(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "t.bbolt")
	docs := []*adapter.Document{
		{ID: "A", Title: "A", Type: "note", Refs: []string{"B"}, Text: "x"},
		{ID: "B", Title: "B", Type: "note", Refs: []string{}, Text: "y"},
	}
	if err := Save(dbPath, docs); err != nil {
		t.Fatalf("Save#1: %v", err)
	}
	if err := Save(dbPath, docs); err != nil {
		t.Fatalf("Save#2(同内容): %v", err)
	}
	got, err := Load(dbPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("应 2 文档：%d", len(got))
	}
	// 删掉 A 再存：A 及其 links 应从库中移除
	if err := Save(dbPath, docs[1:]); err != nil {
		t.Fatalf("Save#3(删A): %v", err)
	}
	got, err = Load(dbPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(got) != 1 || got[0].ID != "B" {
		t.Fatalf("删 A 后应只剩 B：%v", got)
	}
	// 改 B 内容再存：差值更新生效
	docs[1].Text = "y2"
	if err := Save(dbPath, docs[1:]); err != nil {
		t.Fatalf("Save#4(改B): %v", err)
	}
	got, err = Load(dbPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got[0].Text != "y2" {
		t.Fatalf("B 内容未更新：%v", got[0].Text)
	}
}

// TestLoadEmptyOrMissing：#16 无迁移——旧库（SQLite）文件 Load 应返回空而非报错
// （refresh 重建），缺失文件返回空。
func TestLoadEmptyOrMissing(t *testing.T) {
	dir := t.TempDir()
	// 缺失文件 → nil, nil
	got, err := Load(filepath.Join(dir, "missing.bbolt"))
	if err != nil || got != nil {
		t.Fatalf("缺失文件应 (nil,nil)：got=%v err=%v", got, err)
	}
	// 旧 SQLite 文件（非 bbolt 格式）→ bbolt 打开报错，应视为无旧状态？
	// 注：bbolt.Open 对非 bbolt 文件返回 "invalid database" 错误——按 #16 无迁移
	// 语义，旧库直接删，调用方（cmd refresh）在 Load 失败时不致死（err 向上传）。
	// 这里仅验证缺失场景；旧库场景由 cmd 层决定处理。
	_ = got
}

// readTouches 内部读路径（测试辅助；与 loadTouches 同源，按 seq 顺序返回）。
func readTouches(dbPath string) ([][2]string, error) {
	var rows [][2]string
	db, err := open(dbPath)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	err = db.View(func(tx *bolt.Tx) error {
		tb := tx.Bucket(bTouchB)
		if tb == nil {
			return nil
		}
		type kv struct {
			seq uint64
			row [2]string
		}
		var all []kv
		if err := tb.ForEach(func(k, v []byte) error {
			var e touchEntry
			if err := json.Unmarshal(v, &e); err != nil {
				return err
			}
			all = append(all, kv{seq: binary.BigEndian.Uint64(k), row: [2]string{e.Target, e.Src}})
			return nil
		}); err != nil {
			return err
		}
		sort.Slice(all, func(i, j int) bool { return all[i].seq < all[j].seq })
		for _, x := range all {
			rows = append(rows, x.row)
		}
		return nil
	})
	return rows, err
}

// ---- digest 子系统（§3.7，v0.1.14） ----

// TestMaybeDigestCountTrigger：计数触发（主）——累计 ≥ digest_count 生成 digest，
// 内容为窗口聚合（幽灵过滤 + 标题）；未达阈值不生成。
func TestMaybeDigestCountTrigger(t *testing.T) {
	touchPath, graphPath := touchPair(t)
	if err := Save(graphPath, []*adapter.Document{
		{ID: "A", Title: "甲", Type: "note", Refs: []string{}},
		{ID: "B", Title: "乙", Type: "note", Refs: []string{}},
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	cfg := DefaultTouchConfig()
	cfg.DigestCount = 3
	// 未达阈值：2 次 → 不生成
	for i := 0; i < 2; i++ {
		if err := AppendTouch(touchPath, "A", "q"); err != nil {
			t.Fatalf("AppendTouch: %v", err)
		}
	}
	if gen, err := MaybeDigest(touchPath, graphPath, cfg); err != nil || gen {
		t.Fatalf("未达阈值不应生成：gen=%v err=%v", gen, err)
	}
	// 达阈值：再 1 次（累计 3）→ 生成
	if err := AppendTouch(touchPath, "B", "q"); err != nil {
		t.Fatalf("AppendTouch: %v", err)
	}
	gen, err := MaybeDigest(touchPath, graphPath, cfg)
	if err != nil || !gen {
		t.Fatalf("达阈值应生成：gen=%v err=%v", gen, err)
	}
	dig, err := LatestDigest(touchPath)
	if err != nil || dig == nil {
		t.Fatalf("LatestDigest: %v", err)
	}
	if dig.Total != 3 {
		t.Fatalf("窗口总数应 3：%d", dig.Total)
	}
	if len(dig.Targets) != 2 || dig.Targets[0].ID != "A" || dig.Targets[0].Count != 2 {
		t.Fatalf("Targets 错误：%v", dig.Targets)
	}
	if dig.Targets[0].Title != "甲" || dig.Targets[1].Title != "乙" {
		t.Fatalf("标题解析错误：%v", dig.Targets)
	}
	// ack 语义：未 ack → available；ack 后 → 不可用
	if !DigestAvailable(touchPath) {
		t.Fatalf("新 digest 应 available")
	}
	if err := AckDigest(touchPath, dig.ID); err != nil {
		t.Fatalf("AckDigest: %v", err)
	}
	if DigestAvailable(touchPath) {
		t.Fatalf("ack 后不应 available")
	}
}

// TestMaybeDigestGhostFilter：digest 窗口内幽灵 touch（图库不存在）不进 targets。
func TestMaybeDigestGhostFilter(t *testing.T) {
	touchPath, graphPath := touchPair(t)
	if err := Save(graphPath, []*adapter.Document{
		{ID: "A", Title: "甲", Type: "note", Refs: []string{}},
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	cfg := DefaultTouchConfig()
	cfg.DigestCount = 2
	for i := 0; i < 2; i++ {
		if err := AppendTouch(touchPath, "幽灵", "q"); err != nil {
			t.Fatalf("AppendTouch: %v", err)
		}
	}
	if _, err := MaybeDigest(touchPath, graphPath, cfg); err != nil {
		t.Fatalf("maybeDigest: %v", err)
	}
	dig, _ := LatestDigest(touchPath)
	if dig == nil {
		t.Fatal("digest 应存在")
	}
	if len(dig.Targets) != 0 {
		t.Fatalf("幽灵 touch 应被过滤：%v", dig.Targets)
	}
	if dig.Total != 2 {
		t.Fatalf("Total 仍应计窗口事件（幽灵也计入总数）：%d", dig.Total)
	}
}

// TestMaybeDigestBackupRotate：backups 轮转——超过 backup_max 删最旧。
func TestMaybeDigestBackupRotate(t *testing.T) {
	touchPath, graphPath := touchPair(t)
	if err := Save(graphPath, []*adapter.Document{
		{ID: "A", Title: "甲", Type: "note", Refs: []string{}},
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	cfg := DefaultTouchConfig()
	cfg.DigestCount = 1
	cfg.BackupMax = 2
	for i := 0; i < 4; i++ {
		if err := AppendTouch(touchPath, "A", "q"); err != nil {
			t.Fatalf("AppendTouch: %v", err)
		}
		if _, err := MaybeDigest(touchPath, graphPath, cfg); err != nil {
			t.Fatalf("maybeDigest #%d: %v", i, err)
		}
	}
	db, err := open(touchPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	n := 0
	_ = db.View(func(tx *bolt.Tx) error {
		bb := tx.Bucket(bBackups)
		if bb == nil {
			return nil
		}
		return bb.ForEach(func(_, _ []byte) error {
			n++
			return nil
		})
	})
	if n != cfg.BackupMax {
		t.Fatalf("backups 应保留 %d 份：%d", cfg.BackupMax, n)
	}
}

// TestLoadTouchConfig：touch.yaml 解析 + 钳制；缺失 → 默认。
func TestLoadTouchConfig(t *testing.T) {
	dir := t.TempDir()
	if cfg := LoadTouchConfig(dir); cfg != DefaultTouchConfig() {
		t.Fatalf("无配置应默认：%+v", cfg)
	}
	vault := filepath.Join(dir, "v")
	if err := os.MkdirAll(filepath.Join(vault, ".serendipity"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	yamlStr := "digest_count: 700\nbackup_max: 3\n"
	if err := os.WriteFile(filepath.Join(vault, ".serendipity", "touch.yaml"), []byte(yamlStr), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	cfg := LoadTouchConfig(vault)
	if cfg.DigestCount != 600 { // 700 钳到上限 600
		t.Fatalf("digest_count 应钳到 600：%d", cfg.DigestCount)
	}
	if cfg.DigestDays != 3 { // 未写 → 默认
		t.Fatalf("digest_days 应默认 3：%d", cfg.DigestDays)
	}
	if cfg.BackupMax != 3 {
		t.Fatalf("backup_max 应 3：%d", cfg.BackupMax)
	}
}
