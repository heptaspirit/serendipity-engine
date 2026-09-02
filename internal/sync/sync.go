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
//     删除再建同名 = deleted + added
//  5. 大库性能：TestOrca 21976 块全量解析 ≈ 0.45s，diff O(n) 微不足道
//
// ▍改名迁移（v0.1.5，设计修订 #8）
//   Obsidian 文件名即 ID：改名在 Diff 眼里 = deleted（旧名）+ added（新名）。
//   修订 #8 要求把这对操作识别为 rename——否则被引用节点的链接会悬空、
//   touch 埋点数据断在旧 ID 上。判定 = 内容哈希（Text 相同）+ 路径相似度
//   （同目录优先 + basename 公共前缀）双信号；拿不准（候选并列）宁可不判，
//   退回 deleted+added 老行为。识别出的 rename 由调用方 ApplyRenames（Refs
//   重定向）与 store.RenameTouch（埋点迁移）落地。虎鲸数字 ID 永不配对（稳定）。
package sync

import (
	"crypto/sha256"
	"encoding/hex"
	"path"
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

// Rename 一条改名迁移（修订 #8）：旧 ID → 新 ID。
// 语义：同一内容在库中的身份更换（Obsidian 文件名变更）；被引用链接与
// touch 埋点应由调用方按此迁移，避免悬空与数据断裂。
type Rename struct {
	OldID string `json:"old_id"`
	NewID string `json:"new_id"`
	Title string `json:"title"`
	Type  string `json:"type"`
}

// Result 一次刷新的对账结果。
type Result struct {
	Added      int      `json:"added"`
	Updated    int      `json:"updated"`
	Deleted    int      `json:"deleted"`
	Renamed    int      `json:"renamed"` // v0.1.5：改名迁移（修订 #8），从 deleted+added 中拆出
	Unchanged  int      `json:"unchanged"`
	Changes    []Change `json:"changes"` // 明细（调用方按需截断）
	Renames    []Rename `json:"renames"` // 改名明细（旧→新）；deleted/added 已扣除配对数
	DurationMS int64    `json:"duration_ms"`
}

// Diff 对账：old 为上次持久化状态，cur 为本次全量解析结果。
// 返回按 ID 对齐的增/删/改统计与明细；改名（v0.1.5，修订 #8）从
// deleted+added 中识别拆出，计入 Renamed/Renames。
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

	// 改名检测：deleted × added 按内容哈希 + 路径相似度配对（见 detectRenames）。
	renames := detectRenames(oldByID, curByID)
	renamedNew := map[string]bool{} // 反查：新 ID 集合（added 循环排除用）
	for oid, nid := range renames {
		renamedNew[nid] = true
		res.Renames = append(res.Renames, Rename{
			OldID: oid, NewID: nid,
			Title: curByID[nid].Title, Type: curByID[nid].Type,
		})
	}
	res.Renamed = len(renames)

	// deleted：旧有新无（扣除改名配对的旧侧）
	for id, od := range oldByID {
		if _, ok := curByID[id]; ok {
			continue
		}
		if _, renamed := renames[id]; renamed {
			continue
		}
		res.Deleted++
		res.Changes = append(res.Changes, Change{ID: id, Title: od.Title, Kind: KindDeleted, Type: od.Type})
	}
	// added / updated：新集合为主遍历（改名配对的新侧不计 added）
	for id, cd := range curByID {
		od, ok := oldByID[id]
		if !ok {
			if renamedNew[id] {
				continue // 改名配对的新侧：不计 added
			}
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

// ---- 改名迁移（v0.1.5，修订 #8）----

// DetectRenames 识别改名：旧有新无 × 新有旧无 配对。
// 判定双信号：内容哈希（Text 相同——改名通常不动正文；Refs/Path 不参与，
// 引用可能随改名更新、路径必然变化）+ 路径相似度（同目录优先，basename
// 公共前缀作次级区分）。拿不准（并列候选）宁可不判，退回 deleted+added。
// 返回 map[旧ID]新ID；虎鲸数字 ID 稳定，天然不配对。
func DetectRenames(old, cur []*adapter.Document) map[string]string {
	oldByID := map[string]*adapter.Document{}
	for _, d := range old {
		oldByID[d.ID] = d
	}
	curByID := map[string]*adapter.Document{}
	for _, d := range cur {
		curByID[d.ID] = d
	}
	return detectRenames(oldByID, curByID)
}

func detectRenames(oldByID, curByID map[string]*adapter.Document) map[string]string {
	deleted := map[string]*adapter.Document{} // 旧有新无
	for id, od := range oldByID {
		if _, ok := curByID[id]; !ok {
			deleted[id] = od
		}
	}
	added := map[string]*adapter.Document{} // 新有旧无
	for id, cd := range curByID {
		if _, ok := oldByID[id]; !ok {
			added[id] = cd
		}
	}
	if len(deleted) == 0 || len(added) == 0 {
		return nil
	}

	// deleted 按内容哈希分组
	byHash := map[string][]string{}
	for id, d := range deleted {
		h := contentHash(d)
		byHash[h] = append(byHash[h], id)
	}

	renames := map[string]string{}
	claimed := map[string]bool{} // 已被认领的旧 ID
	for nid, cd := range added {
		cands := byHash[contentHash(cd)]
		if len(cands) == 0 {
			continue
		}
		// 虎鲸守卫：块 ID 为纯数字且稳定（改名不换 ID），删+增是"真删除 +
		// 新块"，不是改名；且所有块共享虚拟目录 block/，内容相同必误配。
		// 两侧 ID 均纯数字 → 跳过配对（Obsidian 纯数字文件名改名也接受此保守）。
		if isNumericID(nid) {
			allNumeric := true
			for _, oid := range cands {
				if !isNumericID(oid) {
					allNumeric = false
					break
				}
			}
			if allNumeric {
				continue
			}
		}
		// 取路径相似度最高的未认领候选；并列（拿不准）跳过
		best, bestScore, tie := "", -1.0, false
		for _, oid := range cands {
			if claimed[oid] {
				continue
			}
			s := pathSimilarity(deleted[oid].Path, cd.Path)
			if s > bestScore {
				best, bestScore, tie = oid, s, false
			} else if s == bestScore {
				tie = true
			}
		}
		if best != "" && !tie && bestScore > 0 {
			renames[best] = nid
			claimed[best] = true
		}
	}
	return renames
}

// isNumericID 判断 ID 是否为纯数字（虎鲸块 ID 形态）。
func isNumericID(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// contentHash 内容指纹：仅正文（Text）。Title 随文件名兜底会变、Path 必变、
// Refs 可能随 Obsidian 自动更新引用而变——都不参与，避免改名误判为内容变化。
func contentHash(d *adapter.Document) string {
	h := sha256.Sum256([]byte(d.Text))
	return hex.EncodeToString(h[:])
}

// pathSimilarity 路径相似度（0~1）：
//  同目录 = 0.9 基准；再按 basename 公共前缀长度加权（0~0.1）。
//  目录不同 = 0（改名通常不动目录；跨目录移动不判改名，保守）。
func pathSimilarity(a, b string) float64 {
	da, db := path.Dir(a), path.Dir(b)
	if da != db {
		return 0
	}
	ba, bb := path.Base(a), path.Base(b)
	common := 0
	for i := 0; i < len(ba) && i < len(bb); i++ {
		if ba[i] != bb[i] {
			break
		}
		common++
	}
	return 0.9 + 0.1*float64(common)/float64(max(len(ba), len(bb)))
}

// MergeRenames 合并持久化映射与本次新检测到的改名：
//   - fresh（本次 DetectRenames 结果）覆盖/新增；
//   - 持久化条目在"旧 ID 重现于当前批次"时失效删除（旧名回到库 = 新文档，
//     不再是改名目标）；目标消失但旧名也未重现 → 保留（链式改名的中间环，
//     由 ApplyRenames 传递解析到最终目标）。
//   - v0.1.11 链式折叠（backlog §四，renames 表无上限风险修复）：合并后只保留
//     "链头 → 最终目标"直达映射，丢弃被其他映射覆盖的中间环（A→B、B→C 存在时
//     A→B 可删）——条目数从"历史改名总次数"收敛为"仍存活的链头数"（有界）。
func MergeRenames(stored, fresh map[string]string, cur []*adapter.Document) map[string]string {
	curIDs := map[string]bool{}
	for _, d := range cur {
		curIDs[d.ID] = true
	}
	out := map[string]string{}
	for o, n := range stored {
		if curIDs[o] {
			continue // 旧名重现：该映射失效
		}
		out[o] = n
	}
	for o, n := range fresh {
		out[o] = n
	}
	return collapseChains(out)
}

// collapseChains 链式折叠：每条改名链只留"链头→最终目标"。
// 链头 = 不作为任何条目目标的 key（入度 0）；中间环（目标本身也是 key）被链头
// 的直达映射覆盖，丢弃。环（异常）防御：解析回自身的条目删除。
// 语义权衡（backlog §四）：中间名（B/C）是改名过程的短暂状态，Obsidian 内改名
// 会自动更新引用、文件系统手动改名时引用仍指向原始名（链头）——中间名引用
// 在实践中不存在或极罕见，丢弃换取有界增长。
func collapseChains(m map[string]string) map[string]string {
	if len(m) < 2 {
		return m
	}
	inDeg := map[string]int{}
	for _, n := range m {
		inDeg[n]++
	}
	out := map[string]string{}
	for o := range m {
		if inDeg[o] > 0 {
			continue // 中间节点：被链头覆盖
		}
		final := resolveChain(m, o)
		if final != o {
			out[o] = final
		}
	}
	return out
}

// resolveChain 沿映射解析到最终目标；环/缺失返回自身（保守）。
func resolveChain(m map[string]string, start string) string {
	seen := map[string]bool{start: true}
	cur := start
	for {
		next, ok := m[cur]
		if !ok {
			return cur
		}
		if seen[next] {
			return start // 环：返回原始
		}
		seen[next] = true
		cur = next
	}
}

// ApplyRenames 把 docs 的 Refs 里指向旧 ID 的链接重定向到新 ID（就地修改）。
// 传递解析：链式改名（旧→中→新）一路解到最终目标；循环防御（正常不会出现）。
// 场景：Obsidian 内改名会自动更新引用（无需重定向）；文件系统/手动改名时
// 其他文档的 [[旧名]] 会悬空——重定向后进图即无悬空。重定向后去重保持集合语义。
// 注意：documents 存储仍写原始 Refs（文件真相），本函数只用于建图与展示。
func ApplyRenames(docs []*adapter.Document, renames map[string]string) {
	if len(renames) == 0 {
		return
	}
	resolve := func(id string) string {
		seen := map[string]bool{id: true}
		for {
			nid, ok := renames[id]
			if !ok {
				return id
			}
			if seen[nid] {
				return id // 环：返回原始
			}
			seen[nid] = true
			id = nid
		}
	}
	for _, d := range docs {
		changed := false
		for i, r := range d.Refs {
			if nid := resolve(r); nid != r {
				d.Refs[i] = nid
				changed = true
			}
		}
		if !changed {
			continue
		}
		seen := map[string]bool{}
		out := d.Refs[:0]
		for _, r := range d.Refs {
			if !seen[r] {
				seen[r] = true
				out = append(out, r)
			}
		}
		d.Refs = out
	}
}
