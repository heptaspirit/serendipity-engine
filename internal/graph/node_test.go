package graph

// 单节点详情 text 截断标注（v0.1.13，反馈 #8）：text 超 textSummaryMax 时截断并标注
// text_len=全文长度、text_truncated=true——AI 不会误当全文。
import (
	"strings"
	"testing"
	"unicode/utf8"

	"serendipity-engine/internal/adapter"
)

func TestNodeDetailTextAnnotation(t *testing.T) {
	long := strings.Repeat("字", 250)
	g := Build([]*adapter.Document{{ID: "n", Title: "N", Type: "note", Text: long}})
	d := g.NodeDetail("n")
	if d == nil {
		t.Fatal("NodeDetail 应为非 nil")
	}
	if d.TextLen != 250 {
		t.Fatalf("TextLen=%d 应为 250", d.TextLen)
	}
	if !d.TextTrunc {
		t.Fatalf("超长 text 应标记 text_truncated=true")
	}
	// truncateRunes 保留前 200 rune + "…"（共 201）
	if utf8.RuneCountInString(d.Text) != textSummaryMax+1 {
		t.Fatalf("截断后 rune 数=%d 应为 %d", utf8.RuneCountInString(d.Text), textSummaryMax+1)
	}

	// 短正文不截断、不标记
	short := Build([]*adapter.Document{{ID: "s", Title: "S", Type: "note", Text: "很短"}})
	ds := short.NodeDetail("s")
	if ds.TextLen != 2 || ds.TextTrunc {
		t.Fatalf("短正文应不截断: TextLen=%d trunc=%v", ds.TextLen, ds.TextTrunc)
	}
}
