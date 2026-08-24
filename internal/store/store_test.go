// Package store 持久化单元测试：links 有向引用行回读（v0.1.5 修正）。
// 回归守护：此前排序 pairKey 去重导致字典序较大的端点回读后 Refs 为空，
// 对账 diff 每次刷新报虚假 "refs +1" 永不收敛。
package store

import (
	"database/sql"
	"path/filepath"
	"reflect"
	"testing"

	_ "modernc.org/sqlite"
	"serendipity-engine/internal/adapter"
)

func TestSaveLoadDirectedRefs(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "t.sqlite")

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

func TestRenameTouchPlaceholder(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "t.sqlite")
	if err := AppendTouch(dbPath, "旧A", "来源"); err != nil {
		t.Fatalf("AppendTouch: %v", err)
	}
	if err := AppendTouch(dbPath, "旧B", "旧A"); err != nil {
		t.Fatalf("AppendTouch: %v", err)
	}
	// 链式：旧A→旧B→新B
	if err := RenameTouch(dbPath, map[string]string{"旧A": "旧B", "旧B": "新B"}); err != nil {
		t.Fatalf("RenameTouch: %v", err)
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	rows, err := db.Query(`SELECT target, src FROM touch ORDER BY id`)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	defer rows.Close()
	var got [][2]string
	for rows.Next() {
		var tgt, src string
		if err := rows.Scan(&tgt, &src); err != nil {
			t.Fatalf("scan: %v", err)
		}
		got = append(got, [2]string{tgt, src})
	}
	// 传递解析后：target 旧A→新B、旧B→新B；src 旧A 也解到 新B（来源身份同样迁移）
	want := [][2]string{{"新B", "来源"}, {"新B", "新B"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("链式 touch 迁移错误：%v ≠ %v", got, want)
	}
}

func TestLoadRenamesRoundTrip(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "t.sqlite")
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
// v0.1.12：targets 关联 documents 过滤幽灵 touch——热点A/热点B 先存入 documents 表，
// 未保存的幽灵节点被点后不进热度榜。
func TestTouchStats(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "t.sqlite")
	// 先保存两个真实节点（documents 表），否则 target 过滤会把它们全滤掉（v0.1.12）
	if err := Save(dbPath, []*adapter.Document{
		{ID: "热点A", Title: "A", Type: "note", Refs: []string{}},
		{ID: "热点B", Title: "B", Type: "note", Refs: []string{}},
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	// 插入埋点：target 聚焦少数节点，src 记录来源
	for i := 0; i < 3; i++ {
		if err := AppendTouch(dbPath, "热点A", "来源X"); err != nil {
			t.Fatalf("AppendTouch: %v", err)
		}
	}
	if err := AppendTouch(dbPath, "热点A", "来源Y"); err != nil {
		t.Fatalf("AppendTouch: %v", err)
	}
	if err := AppendTouch(dbPath, "热点B", ""); err != nil { // 无来源（src=NULL/空）
		t.Fatalf("AppendTouch: %v", err)
	}
	if err := AppendTouch(dbPath, "幽灵节点", "来源Z"); err != nil { // 已删/不存在 → 应被过滤
		t.Fatalf("AppendTouch: %v", err)
	}
	total, targets, sources, err := TouchStats(dbPath, 10)
	if err != nil {
		t.Fatalf("TouchStats: %v", err)
	}
	if total != 6 {
		t.Fatalf("总数应 6：%d", total)
	}
	// 被点击：热点A(4) 应排首位；幽灵节点不存在于 documents → 被过滤
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
	// 无埋点表 → 全零（不报错）
	total2, _, _, err := TouchStats(filepath.Join(dir, "none.sqlite"), 10)
	if err != nil || total2 != 0 {
		t.Fatalf("无表应全零：total=%d err=%v", total2, err)
	}
}
