package tui

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// TestBuildDiffHunksBasicEdit 验证典型 edit:中间插入一行,
// 前后缀为上下文行,行号正确(sp.Style 行在两侧都存在 → 上下文)。
func TestBuildDiffHunksBasicEdit(t *testing.T) {
	oldText := "\t// spinners(HUD 与段落流式前缀)\n\tsp := spinner.New()\n\tsp.Style = lipgloss.NewStyle().Foreground(colorOK)\n\tm.spinner = sp\n"
	newText := "\t// spinners(HUD 与段落流式前缀)\n\tsp := spinner.New()\n\tsp.Spinner = spinner.Dot\n\tsp.Style = lipgloss.NewStyle().Foreground(colorOK)\n\tm.spinner = sp\n"
	hunks := buildDiffHunks(fileDiff{Path: "/x/view.go", OldText: &oldText, NewText: newText})
	if len(hunks) != 1 {
		t.Fatalf("hunks = %d, want 1", len(hunks))
	}
	h := hunks[0]
	if h.FilePath != "/x/view.go" {
		t.Fatalf("FilePath = %q", h.FilePath)
	}
	if h.OldStart != 1 || h.NewStart != 1 {
		t.Fatalf("start = %d/%d, want 1/1", h.OldStart, h.NewStart)
	}
	if h.OldCount != 4 || h.NewCount != 5 {
		t.Fatalf("count = %d/%d, want 4/5", h.OldCount, h.NewCount)
	}
	// 行序列:2 ctx + 1 add + 2 ctx(纯插入,无删除行)
	kinds := []DiffLineKind{}
	for _, l := range h.Lines {
		kinds = append(kinds, l.Kind)
	}
	want := []DiffLineKind{DiffCtx, DiffCtx, DiffAdd, DiffCtx, DiffCtx}
	if fmt.Sprint(kinds) != fmt.Sprint(want) {
		t.Fatalf("kinds = %v, want %v", kinds, want)
	}
	// 新增行带新行号,上下文两号都有
	if h.Lines[2].Kind != DiffAdd || h.Lines[2].NewNum != 3 || h.Lines[2].OldNum != 0 {
		t.Fatalf("add line = %+v", h.Lines[2])
	}
	if h.Lines[3].Kind != DiffCtx || h.Lines[3].OldNum != 3 || h.Lines[3].NewNum != 4 {
		t.Fatalf("tail ctx = %+v", h.Lines[3])
	}
	if add, del := h.Stats(); add != 1 || del != 0 {
		t.Fatalf("stats add/del = %d/%d, want 1/0", add, del)
	}
}

// TestBuildDiffHunksLineChanged 验证行内容修改:del + add 成对出现。
func TestBuildDiffHunksLineChanged(t *testing.T) {
	oldText := "a\nb\nc\n"
	newText := "a\nB\nc\n"
	h := buildDiffHunks(fileDiff{Path: "/x/f.go", OldText: &oldText, NewText: newText})[0]
	kinds := []DiffLineKind{}
	for _, l := range h.Lines {
		kinds = append(kinds, l.Kind)
	}
	want := []DiffLineKind{DiffCtx, DiffDel, DiffAdd, DiffCtx}
	if fmt.Sprint(kinds) != fmt.Sprint(want) {
		t.Fatalf("kinds = %v, want %v", kinds, want)
	}
	if add, del := h.Stats(); add != 1 || del != 1 {
		t.Fatalf("stats = %d/%d, want 1/1", add, del)
	}
	if h.Lines[1].OldNum != 2 || h.Lines[2].NewNum != 2 {
		t.Fatalf("line numbers wrong: %+v", h.Lines)
	}
}

// TestBuildDiffHunksNewFile 验证新文件创建(oldText nil):全部为新增行。
func TestBuildDiffHunksNewFile(t *testing.T) {
	hunks := buildDiffHunks(fileDiff{Path: "/x/new.go", OldText: nil, NewText: "a\nb\n"})
	if len(hunks) != 1 {
		t.Fatalf("hunks = %d", len(hunks))
	}
	h := hunks[0]
	if len(h.Lines) != 2 {
		t.Fatalf("lines = %d, want 2", len(h.Lines))
	}
	for i, l := range h.Lines {
		if l.Kind != DiffAdd || l.NewNum != i+1 || l.OldNum != 0 {
			t.Fatalf("line %d = %+v, want add with NewNum=%d", i, l, i+1)
		}
	}
}

// TestBuildDiffHunksWholeFile 验证整文件替换(无公共行):全删全增回退。
func TestBuildDiffHunksWholeFile(t *testing.T) {
	oldText := "one\ntwo\n"
	newText := "alpha\nbeta\ngamma\n"
	hunks := buildDiffHunks(fileDiff{Path: "/x/f.go", OldText: &oldText, NewText: newText})
	h := hunks[0]
	if len(h.Lines) != 5 {
		t.Fatalf("lines = %d, want 5", len(h.Lines))
	}
	for i, l := range h.Lines {
		if i < 2 && l.Kind != DiffDel {
			t.Fatalf("line %d = %v, want del", i, l.Kind)
		}
		if i >= 2 && l.Kind != DiffAdd {
			t.Fatalf("line %d = %v, want add", i, l.Kind)
		}
	}
}

// TestBuildDiffHunksAppendOnly 验证纯追加(前缀全匹配)。
func TestBuildDiffHunksAppendOnly(t *testing.T) {
	oldText := "a\nb\n"
	newText := "a\nb\nc\n"
	h := buildDiffHunks(fileDiff{Path: "/x/f.go", OldText: &oldText, NewText: newText})[0]
	kinds := []DiffLineKind{}
	for _, l := range h.Lines {
		kinds = append(kinds, l.Kind)
	}
	want := []DiffLineKind{DiffCtx, DiffCtx, DiffAdd}
	if fmt.Sprint(kinds) != fmt.Sprint(want) {
		t.Fatalf("kinds = %v, want %v", kinds, want)
	}
}

// TestDiffViewRender 验证转换后的 hunks 能渲染出 +/- 行(展开态冒烟)。
func TestDiffViewRender(t *testing.T) {
	oldText := "line1\nline2\n"
	newText := "line1\nline2 changed\n"
	hunks := buildDiffHunks(fileDiff{Path: "/x/f.go", OldText: &oldText, NewText: newText})
	var sb strings.Builder
	renderDiffView(&sb, hunks, 56, "", ViewportCtx{})
	out := sb.String()
	if !strings.Contains(out, "-line2") || !strings.Contains(out, "+line2 changed") {
		t.Fatalf("render missing +/- lines:\n%s", out)
	}
}

// TestParseToolEventView 验证 view 信封解析与 diff 卡片提取。
func TestParseToolEventView(t *testing.T) {
	raw := json.RawMessage(`{"for":"result","view":{"card":"diff","title":"Edit /x/y.go","diffs":[{"path":"/x/y.go","oldText":"a\n","newText":"b\n"}]}}`)
	tv, ok := parseToolEventView(raw)
	if !ok || tv.For != "result" {
		t.Fatalf("parse failed: %+v", tv)
	}
	title, diffs, ok := parseDiffCard(tv.View)
	if !ok || title != "Edit /x/y.go" || len(diffs) != 1 || diffs[0].Path != "/x/y.go" {
		t.Fatalf("diff card = %q %+v %v", title, diffs, ok)
	}
	if diffs[0].OldText == nil || *diffs[0].OldText != "a\n" || diffs[0].NewText != "b\n" {
		t.Fatalf("fileDiff = %+v", diffs[0])
	}
}

// ---------------------------------------------------------------------------
// 宿主真实样本对齐测试(diffview.go 转换器 + 渲染层)
// 样本取自本仓库会话历史中的真实 tool view。
// ---------------------------------------------------------------------------

// TestRealEditCallViewFragment 宿主 edit 工具 CALL 视图:变更片段(无上下文)。
func TestRealEditCallViewFragment(t *testing.T) {
	oldText := "\t// callId → 进行中的工具段落\n\ttoolParas map[string]*Paragraph"
	newText := "\t// callId → 进行中的工具段落\n\ttoolParas map[string]*Paragraph\n\n\t// callId → diff hunks\n\tcallDiffs map[string][]DiffHunk"
	h := buildDiffHunks(fileDiff{Path: "/x/project.go", OldText: &oldText, NewText: newText})[0]
	if add, del := h.Stats(); add != 3 || del != 0 {
		t.Fatalf("stats = %d/%d, want 3/0 (片段内纯插入)", add, del)
	}
	if h.OldCount != 2 || h.NewCount != 5 {
		t.Fatalf("counts = %d/%d, want 2/5", h.OldCount, h.NewCount)
	}
}

// TestRealEditResultViewContext 宿主 edit 工具 RESULT 视图:
// 带上下文的 hunk,行号从片段首行起(相对行号,协议不含文件行号)。
func TestRealEditResultViewContext(t *testing.T) {
	oldText := "\t// name+args 匹配)\n\ttoolParas map[string]*Paragraph\n\n\t// 进行中的 assistant\n\tassistantPara *Paragraph"
	newText := "\t// name+args 匹配)\n\ttoolParas map[string]*Paragraph\n\n\t// callId → diff hunks\n\tcallDiffs map[string][]DiffHunk\n\n\t// 进行中的 assistant\n\tassistantPara *Paragraph"
	h := buildDiffHunks(fileDiff{Path: "/x/project.go", OldText: &oldText, NewText: newText})[0]
	if add, del := h.Stats(); add != 3 || del != 0 {
		t.Fatalf("stats = %d/%d, want 3/0", add, del)
	}
	// 上下文行跨片段首尾
	if len(h.Lines) != 8 {
		t.Fatalf("lines = %d, want 8", len(h.Lines))
	}
	if h.Lines[0].Kind != DiffCtx || h.Lines[7].Kind != DiffCtx {
		t.Fatalf("首尾应为上下文: %+v", h.Lines)
	}
}

// TestRealWriteWholeFile 宿主 write 工具(oldText=null):
// 整个新文件为全 Add,渲染为 @@ -1,0 +1,N @@ 头 + 全绿行。
func TestRealWriteWholeFile(t *testing.T) {
	newText := "package x\n\nfunc main() {\n\tprintln(\"hi\")\n}\n"
	h := buildDiffHunks(fileDiff{Path: "/x/new.go", OldText: nil, NewText: newText})[0]
	if h.OldCount != 0 || h.NewCount != 5 {
		t.Fatalf("counts = %d/%d, want 0/5", h.OldCount, h.NewCount)
	}
	if add, del := h.Stats(); add != 5 || del != 0 {
		t.Fatalf("stats = %d/%d, want 5/0", add, del)
	}
	var sb strings.Builder
	renderDiffView(&sb, []DiffHunk{h}, 60, "", ViewportCtx{})
	out := sb.String()
	if !strings.Contains(out, "@@ -1,0 +1,5 @@") {
		t.Fatalf("hunk header 缺失:\n%s", out)
	}
	if !strings.Contains(out, "── "+stripCWDPrefix("/x/new.go", "")+" ──") {
		t.Fatalf("文件头缺失:\n%s", out)
	}
}

// TestRealEditResultNoTailNewline 宿主 edit RESULT 视图片段不以换行结尾。
func TestRealEditResultNoTailNewline(t *testing.T) {
	oldText := "\t\tif x {\n\t\t\ty\n\t\t}"
	newText := "\t\tif x {\n\t\t\tz\n\t\t}"
	h := buildDiffHunks(fileDiff{Path: "/x/view.go", OldText: &oldText, NewText: newText})[0]
	if add, del := h.Stats(); add != 1 || del != 1 {
		t.Fatalf("stats = %d/%d, want 1/1", add, del)
	}
	if h.Lines[0].OldNum != 1 || h.Lines[2].NewNum != 2 {
		t.Fatalf("行号错位: %+v", h.Lines)
	}
}

// TestRealDiffRenderEndToEnd 真实 edit 样本端到端:
// 投影(带 view) → 段落 DiffHunks → 渲染输出含文件头与 +/- 行。
func TestRealDiffRenderEndToEnd(t *testing.T) {
	m := NewModel(ModelConfig{Theme: "dark"})
	_ = m.Init()
	p := NewProjector(m)
	m.AttachProjector(p)

	const callID = "call_00_real_edit"
	callView := json.RawMessage(`{"for":"call","view":{"card":"diff","title":"Edit /x/app.go","diffs":[{"path":"/x/app.go","oldText":"// ReplayHistory 回放一段历史事件。\nfunc (m *model) ReplayHistory(events []HistoryEvent) {","newText":"// ReplayHistory 回放一段历史事件。\nfunc (m *model) ReplayHistory(events []HistoryEvent) {\n\tif len(ev.View) > 0 {"}]}}`)
	resultView := json.RawMessage(`{"for":"result","view":{"card":"diff","title":"Edit /x/app.go","diffs":[{"path":"/x/app.go","oldText":"// ReplayHistory 回放一段历史事件。\nfunc (m *model) ReplayHistory(events []HistoryEvent) {\n\tif m.projector == nil {\n\t\treturn\n\t}","newText":"// ReplayHistory 回放一段历史事件。\nfunc (m *model) ReplayHistory(events []HistoryEvent) {\n\tif len(ev.View) > 0 {\n\t\tm.projector.ReplayEventWithView(&ev.Event, ev.View)\n\t} else {\n\t\tm.projector.ReplayEvent(&ev.Event)\n\t}\n\tm.scrollToBottom()\n}"}]}}`)

	p.ReplayEventWithView(todoCallEv(callID, "edit", `{"file_path":"/x/app.go"}`), callView)
	if m.paras[0].DiffHunks == nil {
		t.Fatal("call view 未填充 DiffHunks")
	}
	p.ReplayEventWithView(todoResultEv(callID, 1600), resultView)

	var sb strings.Builder
	renderDiffView(&sb, m.paras[0].DiffHunks, 70, "", ViewportCtx{})
	out := sb.String()
	for _, want := range []string{"── /x/app.go ──", "+", "-"} {
		if !strings.Contains(out, want) {
			t.Fatalf("渲染缺失 %q:\n%s", want, out)
		}
	}
	if m.paras[0].ToolDurMs != 600 {
		t.Fatalf("ToolDurMs = %d, want 600 (call 1000 → result 1600)", m.paras[0].ToolDurMs)
	}
}
