// Package sync 实现"使用者增删后数据库刷新"（对账同步，设计 §6.8 启动对账 v1 落地）。
//
// ▍为什么是全量 diff 而不是增量
//   - 虎鲸（SQLite 权威）：Block 表没有删除墓碑（实测无 valid_until/deleted 列），
//     删除只能通过"全量重解析 + 与旧状态比对"发现；modified 是秒级时间戳，
//     同秒多次修改会漏报——全量 diff 天然免疫时间戳精度问题。
//   - Obsidian（文件系统权威）：vault 级解析毫秒~百毫秒，全量 diff 足够便宜。
//   - 结论：v1 统一"全量解析 → 规范化 diff → 全量持久化（幂等）"；
//     mtime 快照对账（只重解析变更文件）是 v1.5 的增量优化，不改变本语义。
//
// ▍Diff 语义（按 ID 对齐新旧 Document 集合）
//   - 旧有新无                        → deleted（删除）
//   - 新有旧无                        → added（新增）
//   - 两边都有：比较内容指纹（规范化后）→ 变化则 updated（列出变化字段 + 引用增减）
//   - 内容指纹 = Title / Type / Path / Tags / Aliases / Text / Refs
//     （不含 MTime/Size：内容即真相，touch 不改内容不算更新）
//
// ▍规范化（diff 稳定性的前提）
//
//	Tags / Aliases / Refs 在比较前排序——adapter 输出顺序可能受 map 遍历 /
//	SQL 行序影响，不排序会误报 updated。
//
// ▍边际情况（实测/设计确认）
//  1. 首次刷新（无旧存储）→ 全部 added（等价 index）
//  2. 删除被引用节点 → 该节点 deleted；引用它的节点 Refs 变化 → updated；
//     新图悬空链接由 graph.Build 统计（不进 diff，见 Stats.Dangling）
//  3. 归属变化（块移页）→ 两个文档都 updated（Text/Refs 变化）
//  4. ID 复用：虎鲸 autoincrement 不复用（删除即消失）；Obsidian 文件名即 ID，
//     删除再建同名 = deleted + added（v1 不做改名迁移，评审 #8 后置）
//  5. 大库性能：TestOrca 21976 块全量解析 ≈ 0.45s，diff O(n) 微不足道
package sync

import (
	"sort"
	"time"

	"serendipity-engine/internal/adapter"
)

// Kind 变更类型。
type Kind string

const (
	KindAdded   Kind = "added"   // 新增
	KindUpdated Kind = "updated" // 更新（字段级明细）
	KindDeleted Kind = "deleted" // 删除
)

// Change 一条变更明细。
type Change struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Kind  Kind   `json:"kind"`
	Type  string `json:"type,omitempty"`

	// 仅 updated：变化的字段名（title/type/path/tags/aliases/text/refs）
	Fields []string `json:"fields,omitempty"`
	// 仅 updated：引用集合的增减（去重后）
	AddedRefs   []string `json:"added_refs,omitempty"`
	RemovedRefs []string `json:"removed_refs,omitempty"`
}

// Result 一次刷新的对账结果。
type Result struct {
	Added      int      `json:"added"`
	Updated    int      `json:"updated"`
	Deleted    int      `json:"deleted"`
	Unchanged  int      `json:"unchanged"`
	Changes    []Change `json:"changes"` // 明细（调用方按需截断）
	DurationMS int64    `json:"duration_ms"`
}

// Diff 对账：old 为上次持久化状态，cur 为本次全量解析结果。
// 返回按 ID 对齐的增/删/改统计与明细。
func Diff(old, cur []*adapter.Document) *Result {
	start := time.Now()
	res := &Result{Changes: []Change{}}

	oldByID := map[string]*adapter.Document{}
	for _, d := range old {
		oldByID[d.ID] = d
	}
	curByID := map[string]*adapter.Document{}
	for _, d := range cur {
		curByID[d.ID] = d
	}

	// deleted：旧有新无
	for id, od := range oldByID {
		if _, ok := curByID[id]; !ok {
			res.Deleted++
			res.Changes = append(res.Changes, Change{ID: id, Title: od.Title, Kind: KindDeleted, Type: od.Type})
		}
	}
	// added / updated：新集合为主遍历
	for id, cd := range curByID {
		od, ok := oldByID[id]
		if !ok {
			res.Added++
			res.Changes = append(res.Changes, Change{ID: id, Title: cd.Title, Kind: KindAdded, Type: cd.Type})
			continue
		}
		fields, addRefs, rmRefs := diffFields(od, cd)
		if len(fields) == 0 && len(addRefs) == 0 && len(rmRefs) == 0 {
			res.Unchanged++
			continue
		}
		res.Updated++
		res.Changes = append(res.Changes, Change{
			ID: id, Title: cd.Title, Kind: KindUpdated, Type: cd.Type,
			Fields: fields, AddedRefs: addRefs, RemovedRefs: rmRefs,
		})
	}

	res.DurationMS = time.Since(start).Milliseconds()
	return res
}

// diffFields 比较两个同 ID 文档的内容指纹，返回变化的字段名与引用增减。
func diffFields(old, cur *adapter.Document) (fields []string, addRefs, rmRefs []string) {
	if old.Title != cur.Title {
		fields = append(fields, "title")
	}
	if old.Type != cur.Type {
		fields = append(fields, "type")
	}
	if old.Path != cur.Path {
		fields = append(fields, "path")
	}
	if old.Text != cur.Text {
		fields = append(fields, "text")
	}
	if !eqStrings(old.Tags, cur.Tags) {
		fields = append(fields, "tags")
	}
	if !eqStrings(old.Aliases, cur.Aliases) {
		fields = append(fields, "aliases")
	}
	if !eqStrings(old.Refs, cur.Refs) {
		fields = append(fields, "refs")
		addRefs, rmRefs = setDiff(cur.Refs, old.Refs)
	}
	return fields, addRefs, rmRefs
}

// eqStrings 无序集合比较（先拷贝排序，不修改原切片）。
func eqStrings(a, b []string) bool {
	sa, sb := sortedCopy(a), sortedCopy(b)
	if len(sa) != len(sb) {
		return false
	}
	for i := range sa {
		if sa[i] != sb[i] {
			return false
		}
	}
	return true
}

func sortedCopy(ss []string) []string {
	out := make([]string, len(ss))
	copy(out, ss)
	sort.Strings(out)
	return out
}

// setDiff 返回 a−b（在 a 不在 b）的有序列表。
func setDiff(a, b []string) ([]string, []string) {
	bset := map[string]bool{}
	for _, s := range b {
		bset[s] = true
	}
	onlyA := []string{}
	for _, s := range sortedCopy(a) {
		if !bset[s] {
			onlyA = append(onlyA, s)
		}
	}
	aset := map[string]bool{}
	for _, s := range a {
		aset[s] = true
	}
	onlyB := []string{}
	for _, s := range sortedCopy(b) {
		if !aset[s] {
			onlyB = append(onlyB, s)
		}
	}
	return onlyA, onlyB
}
