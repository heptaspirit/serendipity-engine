package adapter

// 快照增量解析单元测试（v0.1.6）：增量结果必须与全量解析完全一致
// （含同名消歧、ID 分配）；mtime/size 未变才复用。
import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func writeTestNote(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// 建一个含同名冲突的 vault：a/甲.md + b/甲.md（消歧后后者 ID = b/甲）。
func buildIncrVault(t *testing.T) string {
	root := t.TempDir()
	writeTestNote(t, filepath.Join(root, "a", "甲.md"), "# 甲A\n\n内容[[乙]]。")
	writeTestNote(t, filepath.Join(root, "b", "甲.md"), "# 甲B\n\n同名文件。")
	writeTestNote(t, filepath.Join(root, "乙.md"), "# 乙\n\n内容[[甲]]。")
	return root
}

func docsKey(docs []*Document) map[string]string {
	out := map[string]string{}
	for _, d := range docs {
		out[d.ID] = d.Path
	}
	return out
}

// 增量 ≡ 全量：同一 vault，全量解析与"以全量结果为旧状态"的增量解析
// 产生完全相同的 ID→Path 映射（消歧一致）。
func TestIncrementalEqualsFull(t *testing.T) {
	root := buildIncrVault(t)
	p := &VaultProfile{Name: "test", ExcludedDirs: nil}

	full, err := ParseVault(root, p)
	if err != nil {
		t.Fatal(err)
	}
	inc, reused, err := ParseVaultIncremental(root, p, full)
	if err != nil {
		t.Fatal(err)
	}
	if reused != len(full) {
		t.Fatalf("无变化应全复用：reused=%d total=%d", reused, len(full))
	}
	// 至少出现一个同名消歧（a/甲 保留 basename，b/甲 用相对路径）
	hasDisambig := false
	for _, d := range inc {
		if d.ID == "b/甲" {
			hasDisambig = true
		}
	}
	if !hasDisambig {
		t.Fatalf("同名消歧未生效：%v", docsKey(inc))
	}
	if !reflect.DeepEqual(docsKey(full), docsKey(inc)) {
		t.Fatalf("增量 ≠ 全量：\n全量=%v\n增量=%v", docsKey(full), docsKey(inc))
	}
}

// 修改一个文件后增量：只重解析变更文件，且结果与全量一致。
func TestIncrementalAfterChange(t *testing.T) {
	root := buildIncrVault(t)
	p := &VaultProfile{Name: "test"}

	full, err := ParseVault(root, p)
	if err != nil {
		t.Fatal(err)
	}
	// 改乙.md 并确保 mtime 变化（不同秒）
	time.Sleep(1100 * time.Millisecond)
	writeTestNote(t, filepath.Join(root, "乙.md"), "# 乙\n\n内容改了[[甲]]。")

	inc, reused, err := ParseVaultIncremental(root, p, full)
	if err != nil {
		t.Fatal(err)
	}
	if reused != len(full)-1 {
		t.Fatalf("只应重解析乙：reused=%d（期望 %d）", reused, len(full)-1)
	}
	full2, err := ParseVault(root, p)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(docsKey(full2), docsKey(inc)) {
		t.Fatalf("增量 ≠ 全量（变更后）：\n全量=%v\n增量=%v", docsKey(full2), docsKey(inc))
	}
	// 变更文件的 Text 应为新内容（重解析了）
	for _, d := range inc {
		if d.ID == "乙" && !strings.Contains(d.Text, "内容改了") {
			t.Fatalf("乙未重解析：text=%q", d.Text)
		}
	}
}

// 删除检测：文件消失 → 增量结果不含它（diff 层报 deleted）。
func TestIncrementalAfterDelete(t *testing.T) {
	root := buildIncrVault(t)
	p := &VaultProfile{Name: "test"}

	full, err := ParseVault(root, p)
	if err != nil {
		t.Fatal(err)
	}
	os.Remove(filepath.Join(root, "乙.md"))
	inc, _, err := ParseVaultIncremental(root, p, full)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range inc {
		if d.ID == "乙" {
			t.Fatal("删除的文件不应出现在增量结果里")
		}
	}
}
