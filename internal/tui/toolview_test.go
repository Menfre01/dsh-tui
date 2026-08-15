package tui

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Menfre01/dsh-tui/internal/dsh"
)

// diffResultViewJSON 构造宿主 diff result view(真实结构)。
func diffResultViewJSON(callID, path, oldText, newText string) json.RawMessage {
	return json.RawMessage(`{"for":"result","view":{"card":"diff","title":"Edit ` + path + `","diffs":[{"path":"` + path + `","oldText":` + jsonString(oldText) + `,"newText":` + jsonString(newText) + `}]}}`)
}

func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// TestProjectorDiffViewResult 验证 tool/result 的 diff view 填充段落 DiffHunks。
func TestProjectorDiffViewResult(t *testing.T) {
	m := NewModel(ModelConfig{Theme: "dark"})
	_ = m.Init()
	p := NewProjector(m)
	m.AttachProjector(p)

	const callID = "call_00_diffview"
	p.ReplayEventWithView(toolCallEv(callID, "edit"),
		json.RawMessage(`{"for":"call","view":{"card":"diff","title":"Edit /x/y.go","diffs":[{"path":"/x/y.go","oldText":"a\nb\n","newText":"a\nb2\n"}]}}`))
	if len(m.paras) != 1 || m.paras[0].DiffHunks == nil {
		t.Fatalf("call view 未填充 DiffHunks: paras=%d hunks=%v", len(m.paras), m.paras[0].DiffHunks)
	}

	// result 视图(应用后的实际改动)替换 call 视图
	oldText := "a\nb\n"
	newText := "a\nb2\nc\n"
	p.ReplayEventWithView(toolResultEvToolCallID(callID, "", false), diffResultViewJSON(callID, "/x/y.go", oldText, newText))

	para := m.paras[0]
	if para.State != stateCollapsed {
		t.Fatalf("state = %v, want collapsed", para.State)
	}
	if len(para.DiffHunks) != 1 {
		t.Fatalf("DiffHunks = %d, want 1", len(para.DiffHunks))
	}
	if add, del := para.DiffHunks[0].Stats(); add != 2 || del != 1 {
		t.Fatalf("stats = %d/%d, want 2/1 (b→b2 改 + c 增)", add, del)
	}
}

// TestProjectorReadView 验证 tool/result 的 read view 填充结构化行号。
func TestProjectorReadView(t *testing.T) {
	m := NewModel(ModelConfig{Theme: "dark"})
	_ = m.Init()
	p := NewProjector(m)
	m.AttachProjector(p)

	const callID = "call_00_readview"
	p.ReplayEvent(toolCallEv(callID, "read"))
	raw := json.RawMessage(`{"for":"result","view":{"card":"read","path":"/x/y.go","offset":525,"lines":[{"number":525,"text":"\tti.Focus()"},{"number":526,"text":"\t"}]}}`)
	p.ReplayEventWithView(toolResultEvToolCallID(callID, "", false), raw)

	para := m.paras[0]
	if para.ReadPath != "/x/y.go" || para.ReadLines == nil {
		t.Fatalf("read view 未填充: path=%q lines=%v", para.ReadPath, para.ReadLines)
	}
	if len(para.ReadLines) != 2 || para.ReadLines[0].Number != 525 || para.ReadLines[0].Text != "\tti.Focus()" {
		t.Fatalf("ReadLines = %+v", para.ReadLines)
	}
}

// TestProjectorTerminalViewFallback 验证 terminal view 在无 text 块时兜底 output。
func TestProjectorTerminalViewFallback(t *testing.T) {
	m := NewModel(ModelConfig{Theme: "dark"})
	_ = m.Init()
	p := NewProjector(m)
	m.AttachProjector(p)

	const callID = "call_00_termview"
	p.ReplayEvent(toolCallEv(callID, "bash"))
	raw := json.RawMessage(`{"for":"result","view":{"card":"terminal","title":"ls","output":"hello\nworld\n","exitCode":0}}`)
	p.ReplayEventWithView(toolResultEvToolCallID(callID, "", false), raw)

	if m.paras[0].ToolResult != "hello\nworld" {
		t.Fatalf("ToolResult = %q, want output 兜底", m.paras[0].ToolResult)
	}
}

// TestProjectorApprovalDiff 验证审批框拿到 call 视图的 diff 预览
// (approval/requested 帧本身不带 view,按 callId 反查)。
func TestProjectorApprovalDiff(t *testing.T) {
	m := NewModel(ModelConfig{Theme: "dark"})
	_ = m.Init()
	p := NewProjector(m)
	m.AttachProjector(p)

	const callID = "call_00_approve"
	p.ReplayEventWithView(toolCallEv(callID, "edit"),
		json.RawMessage(`{"for":"call","view":{"card":"diff","title":"Edit /x/y.go","diffs":[{"path":"/x/y.go","oldText":"a\n","newText":"b\n"}]}}`))

	p.onApprovalRequested("rpc-1", dsh.MuxFrame{
		SessionID:  "sess-1",
		ApprovalID: "appr-1",
		ToolName:   "edit",
		CallID:     callID,
		Reason:     "needs approval",
	})
	pa := m.pendingApproval
	if pa == nil {
		t.Fatal("pendingApproval 未弹出")
	}
	if len(pa.Diffs) != 1 {
		t.Fatalf("Diffs = %d, want 1 (call 视图缓存)", len(pa.Diffs))
	}
	if pa.Diffs[0].FilePath != "/x/y.go" {
		t.Fatalf("diff path = %q", pa.Diffs[0].FilePath)
	}

	// result 到达后缓存清理,不再泄露
	p.ReplayEventWithView(toolResultEvToolCallID(callID, "", false),
		diffResultViewJSON(callID, "/x/y.go", "a\n", "b\n"))
	if len(p.callDiffs) != 0 {
		t.Fatalf("callDiffs 未清理: %d", len(p.callDiffs))
	}
}

// TestProjectorViewWithoutView 验证无 view 时行为不变(纯文本回退)。
func TestProjectorViewWithoutView(t *testing.T) {
	m := NewModel(ModelConfig{Theme: "dark"})
	_ = m.Init()
	p := NewProjector(m)
	m.AttachProjector(p)

	const callID = "call_00_noview"
	p.ReplayEvent(toolCallEv(callID, "bash"))
	p.ReplayEvent(toolResultEvToolCallID(callID, "plain output", false))
	para := m.paras[0]
	if para.DiffHunks != nil || para.ReadLines != nil {
		t.Fatalf("无 view 不应有结构化字段: %+v", para)
	}
	if para.ToolResult != "plain output" {
		t.Fatalf("ToolResult = %q", para.ToolResult)
	}
}

// TestTurnEndFinalizesStreamingParas 验证 turn/end 收尾泄漏的流式段落
// (中断/异常时 assistant/message、tool/result 缺失 → 前缀 spinner 永转)。
func TestTurnEndFinalizesStreamingParas(t *testing.T) {
	m := NewModel(ModelConfig{Theme: "dark"})
	_ = m.Init()
	p := NewProjector(m)
	m.AttachProjector(p)

	// 构造泄漏场景:thought 流式中断 + tool 执行中断
	chunkData, _ := json.Marshal(map[string]any{
		"chunk": map[string]any{"type": "reasoning-delta", "text": "thinking about something long"},
	})
	p.ReplayEvent(&dsh.SessionEvent{Type: dsh.EvAssistantChunk, Data: chunkData})
	p.ReplayEvent(todoCallEv("call_00_leak", "bash", `{"command":"sleep"}`))
	if p.thoughtIdx < 0 || p.thoughtPara().State != stateStreaming {
		t.Fatal("thought 应为 streaming")
	}
	streamingTool := false
	for _, para := range m.paras {
		if para.Type == paraTool && para.State == stateStreaming {
			streamingTool = true
		}
	}
	if !streamingTool {
		t.Fatal("tool 应为 streaming")
	}

	// turn/end(aborted) → 全部收尾
	turnEndData, _ := json.Marshal(map[string]any{
		"turn":   1,
		"reason": map[string]any{"kind": "aborted"},
	})
	p.ReplayEvent(&dsh.SessionEvent{Type: dsh.EvTurnEnd, Data: turnEndData})

	for i := range m.paras {
		para := &m.paras[i]
		if para.Type == paraThought && para.State != stateCollapsed {
			t.Fatalf("thought 段落 state = %v, want collapsed(收尾)", para.State)
		}
		if para.Type == paraTool && para.State != stateDone {
			t.Fatalf("tool 段落 state = %v, want done(收尾)", para.State)
		}
	}
	if len(p.toolIdx) != 0 || len(p.toolStart) != 0 {
		t.Fatalf("toolIdx/toolStart 未清理: %d/%d", len(p.toolIdx), len(p.toolStart))
	}
}

// TestTodoPanelClearedOnTurnStart 验证 turn/start 清空 todo 面板
// (宿主投影语义:每回合独立 todo 列表,全 completed 面板随新回合消失)。
func TestTodoPanelClearedOnTurnStart(t *testing.T) {
	m := NewModel(ModelConfig{Theme: "dark"})
	_ = m.Init()
	p := NewProjector(m)
	m.AttachProjector(p)

	// 回合内 todo_write 填充面板(全 completed,宿主不主动清空)
	todoData, _ := json.Marshal(map[string]any{
		"todos": []map[string]string{
			{"content": "task A", "status": "completed"},
			{"content": "task B", "status": "completed"},
		},
	})
	p.ReplayEvent(&dsh.SessionEvent{Type: dsh.EvTodoWrite, Data: todoData})
	if len(m.todos) != 2 {
		t.Fatalf("todos = %d, want 2", len(m.todos))
	}

	// 下一回合开始:面板清空
	p.ReplayEvent(&dsh.SessionEvent{Type: dsh.EvTurnStart, Data: json.RawMessage(`{"turn":2}`)})
	if len(m.todos) != 0 {
		t.Fatalf("turn/start 后 todos = %d, want 0(面板应消失)", len(m.todos))
	}

	// 新回合重新 todo_write → 面板恢复
	p.ReplayEvent(&dsh.SessionEvent{Type: dsh.EvTodoWrite, Data: todoData})
	if len(m.todos) != 2 {
		t.Fatalf("新回合 todo_write 后 todos = %d, want 2", len(m.todos))
	}
}

// todoCallEv 构造 tool/call 事件(callId + name + arguments)。
func todoCallEv(callID, name, args string) *dsh.SessionEvent {
	data, _ := json.Marshal(map[string]any{
		"callId":    callID,
		"name":      name,
		"arguments": args,
	})
	return &dsh.SessionEvent{Type: dsh.EvToolCall, Data: data, Time: 1000}
}

// todoResultEv 构造 tool/result 事件(带时间戳)。
func todoResultEv(callID string, time int64) *dsh.SessionEvent {
	data, _ := json.Marshal(map[string]any{
		"message": map[string]any{
			"content": []any{
				map[string]any{
					"type":       "tool-result",
					"toolCallId": callID,
					"content":    []any{map[string]any{"type": "text", "text": "ok"}},
				},
			},
		},
	})
	return &dsh.SessionEvent{Type: dsh.EvToolResult, Data: data, Time: time}
}

// TestTodoWriteStyledPara 验证 todo_write 段落专用样式:
// 不显示参数 JSON,suffix 带计数摘要。
func TestTodoWriteStyledPara(t *testing.T) {
	m := NewModel(ModelConfig{Theme: "dark"})
	_ = m.Init()
	p := NewProjector(m)
	m.AttachProjector(p)

	const callID = "call_00_todo"
	p.ReplayEvent(todoCallEv(callID, "todo_write", `{"todos":[...]}`))
	para := m.paras[0]
	if !para.ToolTodo {
		t.Fatal("todo_write 段落应标记 ToolTodo")
	}
	if para.ToolArgs != "" {
		t.Fatalf("ToolArgs 应清空(不显示 todo 列表 JSON), got %q", para.ToolArgs)
	}

	// todo/write 副作用先于 result 更新面板 → result 时摘要 = "已完成 0/2"
	todoData, _ := json.Marshal(map[string]any{
		"todos": []map[string]string{
			{"content": "a", "status": "pending"},
			{"content": "b", "status": "pending"},
		},
	})
	p.ReplayEvent(&dsh.SessionEvent{Type: dsh.EvTodoWrite, Data: todoData})
	p.ReplayEvent(todoResultEv(callID, 1500))

	para = m.paras[0]
	if para.ToolDurMs != 500 {
		t.Fatalf("ToolDurMs = %d, want 500 (call 1000 → result 1500)", para.ToolDurMs)
	}
	if para.ToolTodoSummary != "0/2" {
		t.Fatalf("summary = %q, want 0/2", para.ToolTodoSummary)
	}
	if got := toolSuffix(&m.paras[0], m.msg()); got != "(0/2)" {
		t.Fatalf("suffix = %q, want (0/2)", got)
	}

	// in_progress 状态 → 摘要切换为进行中
	todoData2, _ := json.Marshal(map[string]any{
		"todos": []map[string]string{
			{"content": "a", "status": "in_progress"},
			{"content": "b", "status": "pending"},
		},
	})
	p.ReplayEvent(&dsh.SessionEvent{Type: dsh.EvTodoWrite, Data: todoData2})
	if got := m.paras[0].ToolTodoSummary; got != "0/2" {
		t.Fatalf("summary = %q, want 0/2", got)
	}
	if got := m.paras[0].ToolArgs; got != "a" {
		t.Fatalf("ToolArgs(任务内容) = %q, want a", got)
	}
}

// TestBashExitCodeFromView 验证 bash 退出码来自宿主 terminal result view,
// 耗时来自事件时间差。
func TestBashExitCodeFromView(t *testing.T) {
	m := NewModel(ModelConfig{Theme: "dark"})
	_ = m.Init()
	p := NewProjector(m)
	m.AttachProjector(p)

	const callID = "call_00_bash"
	p.ReplayEvent(todoCallEv(callID, "bash", `{"command":"false"}`))
	raw := json.RawMessage(`{"for":"result","view":{"card":"terminal","title":"false","output":"","exitCode":1}}`)
	p.ReplayEventWithView(todoResultEv(callID, 2300), raw)

	para := m.paras[0]
	if para.ToolExitCode != 1 {
		t.Fatalf("ToolExitCode = %d, want 1", para.ToolExitCode)
	}
	if para.ToolDurMs != 1300 {
		t.Fatalf("ToolDurMs = %d, want 1300", para.ToolDurMs)
	}
	// 非零退出判为错误:红色 + 错误 suffix
	if para.State != stateError || !para.ToolFatal {
		t.Fatalf("exit 1 应为红色错误: state=%v fatal=%v", para.State, para.ToolFatal)
	}
	if got := toolSuffix(&m.paras[0], m.msg()); got != "(exit=1)" {
		t.Fatalf("suffix = %q, want (exit=1)", got)
	}
}

// TestBashSignalFromView 验证信号终止的 suffix。
func TestBashSignalFromView(t *testing.T) {
	m := NewModel(ModelConfig{Theme: "dark"})
	_ = m.Init()
	p := NewProjector(m)
	m.AttachProjector(p)

	const callID = "call_00_sig"
	p.ReplayEvent(todoCallEv(callID, "bash", `{"command":"kill"}`))
	raw := json.RawMessage(`{"for":"result","view":{"card":"terminal","title":"kill","output":"","signal":"SIGKILL"}}`)
	p.ReplayEventWithView(todoResultEv(callID, 1000), raw)

	para := m.paras[0]
	if para.ToolSignal != "SIGKILL" {
		t.Fatalf("ToolSignal = %q", para.ToolSignal)
	}
	if got := toolSuffix(&m.paras[0], m.msg()); got != "(killed: SIGKILL, 0ms)" {
		t.Fatalf("suffix = %q, want (killed: SIGKILL, 0ms)", got)
	}
}

// TestAskUserQuestionSuffix 验证 ask_user_question 段落 suffix 显示问题数
// (从原始 arguments 解析,而非格式化摘要)。
func TestAskUserQuestionSuffix(t *testing.T) {
	m := NewModel(ModelConfig{Theme: "dark"})
	_ = m.Init()
	p := NewProjector(m)
	m.AttachProjector(p)

	args := `{"questions":[{"question":"q1","options":[{"label":"a"},{"label":"b"}]},{"question":"q2","options":[{"label":"c"}]}]}`
	p.ReplayEvent(todoCallEv("call_00_ask", "ask_user_question", args))
	para := m.paras[0]
	if para.ToolQuestionCount != 2 {
		t.Fatalf("ToolQuestionCount = %d, want 2", para.ToolQuestionCount)
	}
	// result 到达(提问框已应答)
	p.ReplayEvent(todoResultEv("call_00_ask", 1200))
	if got := toolSuffix(&m.paras[0], m.msg()); got != "(2 questions)" && got != "(2 问)" {
		t.Fatalf("suffix = %q, want (2 questions)", got)
	}
}

// TestEditArgsDisplay 验证 edit 参数摘要对齐 dsh 协议
// ({file_path,old_string,new_string} → 显示文件路径,而非 waveloom 的
// hunk patch 计数)。
func TestEditArgsDisplay(t *testing.T) {
	args := `{"file_path":"/work/dsh-tui/internal/tui/focus.go","old_string":"a","new_string":"b"}`
	got := formatToolArgs("edit", args, "/work/dsh-tui")
	if got != "internal/tui/focus.go" {
		t.Fatalf("edit args = %q, want internal/tui/focus.go", got)
	}
	if strings.Contains(got, "hunk") {
		t.Fatalf("不应包含 hunk 计数: %q", got)
	}
	// write 保持文件路径显示
	args2 := `{"file_path":"/work/dsh-tui/internal/tui/new.go"}`
	if got := formatToolArgs("write", args2, "/work/dsh-tui"); got != "internal/tui/new.go" {
		t.Fatalf("write args = %q", got)
	}
}

// toolResultErrEv 构造带顶层 error 的失败 tool/result。
func toolResultErrEv(callID string, code, text string, time int64) *dsh.SessionEvent {
	data, _ := json.Marshal(map[string]any{
		"message": map[string]any{
			"content": []any{
				map[string]any{
					"type":       "tool-result",
					"toolCallId": callID,
					"isError":    true,
					"content":    []any{map[string]any{"type": "text", "text": text}},
				},
			},
		},
		"error": map[string]any{"name": "FsError", "code": code},
	})
	return &dsh.SessionEvent{Type: dsh.EvToolResult, Data: data, Time: time}
}

// TestEditToolErrorState 验证 edit 失败:stateError、ToolErrorKind=code、
// suffix 显示分类、折叠预览优先错误信息而非 diff。
func TestEditToolErrorState(t *testing.T) {
	m := NewModel(ModelConfig{Theme: "dark"})
	_ = m.Init()
	p := NewProjector(m)
	m.AttachProjector(p)

	const callID = "call_00_edit_err"
	// call 视图带 diff(call 时将要发生的改动)
	p.ReplayEventWithView(todoCallEv(callID, "edit", `{"file_path":"/x/f.go","old_string":"a","new_string":"b"}`),
		json.RawMessage(`{"for":"call","view":{"card":"diff","title":"Edit /x/f.go","diffs":[{"path":"/x/f.go","oldText":"a\n","newText":"b\n"}]}}`))
	if m.paras[0].DiffHunks == nil {
		t.Fatal("call view 应填充 DiffHunks")
	}
	// result:失败
	p.ReplayEvent(toolResultErrEv(callID, "FS_NOT_FOUND", "Error: The path does not exist.", 1500))

	para := m.paras[0]
	if para.State != stateError {
		t.Fatalf("state = %v, want stateError", para.State)
	}
	if para.ToolErrorKind != "FS_NOT_FOUND" {
		t.Fatalf("ToolErrorKind = %q, want FS_NOT_FOUND", para.ToolErrorKind)
	}
	if got := toolSuffix(&m.paras[0], m.msg()); got != "(FS_NOT_FOUND)" {
		t.Fatalf("suffix = %q, want (FS_NOT_FOUND)", got)
	}

	// 折叠预览:错误信息优先,不显示 diff 行
	var sb strings.Builder
	renderToolPara(&sb, &m.paras[0], ViewportCtx{Width: 60, LC: m.msg(), Tool: m.spTool})
	out := sb.String()
	if !strings.Contains(out, "Error: The path does not exist.") {
		t.Fatalf("错误信息未显示:\n%s", out)
	}
	if strings.Contains(out, "diff") && strings.Contains(out, "── ") {
		t.Fatalf("错误态不应显示 diff 预览:\n%s", out)
	}
}

// TestToolErrorKindSet 验证通用错误路径设置 ToolErrorKind(bash 等)。
func TestToolErrorKindSet(t *testing.T) {
	m := NewModel(ModelConfig{Theme: "dark"})
	_ = m.Init()
	p := NewProjector(m)
	m.AttachProjector(p)

	const callID = "call_00_bash_err"
	p.ReplayEvent(todoCallEv(callID, "bash", `{"command":"false"}`))
	p.ReplayEvent(toolResultErrEv(callID, "ABORTED", "Error: tool call aborted", 2000))
	para := m.paras[0]
	if para.ToolErrorKind != "ABORTED" {
		t.Fatalf("ToolErrorKind = %q, want ABORTED", para.ToolErrorKind)
	}
	if got := toolSuffix(&m.paras[0], m.msg()); got != "(ABORTED)" {
		t.Fatalf("suffix = %q, want (ABORTED)", got)
	}
}

// TestIsErrorWithoutTopError 验证 isError=true 但无顶层 error 时
// 兜底分类 failed,suffix 不为空括号。
func TestIsErrorWithoutTopError(t *testing.T) {
	m := NewModel(ModelConfig{Theme: "dark"})
	_ = m.Init()
	p := NewProjector(m)
	m.AttachProjector(p)

	const callID = "call_00_noerr"
	// 纯 isError,无 error 字段
	data, _ := json.Marshal(map[string]any{
		"message": map[string]any{
			"content": []any{
				map[string]any{
					"type":       "tool-result",
					"toolCallId": callID,
					"isError":    true,
					"content":    []any{map[string]any{"type": "text", "text": "Error: boom"}},
				},
			},
		},
	})
	p.ReplayEvent(todoCallEv(callID, "grep", `{"pattern":"x"}`))
	p.ReplayEvent(&dsh.SessionEvent{Type: dsh.EvToolResult, Data: data, Time: 1000})

	para := m.paras[0]
	if para.State != stateError {
		t.Fatalf("state = %v, want error", para.State)
	}
	if para.ToolErrorKind != "failed" {
		t.Fatalf("ToolErrorKind = %q, want failed", para.ToolErrorKind)
	}
	if got := toolSuffix(&m.paras[0], m.msg()); got != "(failed)" {
		t.Fatalf("suffix = %q, want (failed)", got)
	}
}

// TestTurnEndAbortedFormat 验证中断通知的 %s 被耗时格式化(非原样输出),
// 耗时来自 turn/start 与 turn/end 的事件时间戳差。
func TestTurnEndAbortedFormat(t *testing.T) {
	m := NewModel(ModelConfig{Theme: "dark"})
	_ = m.Init()
	p := NewProjector(m)
	m.AttachProjector(p)

	// turn/start(time=1000) → 中断 turn/end(time=2500) → 1.5s(实时路径)
	p.replayEvent(&dsh.SessionEvent{Type: dsh.EvTurnStart, Data: json.RawMessage(`{"turn":1}`), Time: 1000}, nil, false)
	turnEndData, _ := json.Marshal(map[string]any{
		"turn":   1,
		"reason": map[string]any{"kind": "aborted", "reason": map[string]any{"kind": "user"}},
	})
	p.replayEvent(&dsh.SessionEvent{Type: dsh.EvTurnEnd, Data: turnEndData, Time: 2500}, nil, false)

	if len(m.paras) == 0 || m.paras[len(m.paras)-1].Type != paraSystem {
		t.Fatalf("应追加系统通知段落")
	}
	text := m.paras[len(m.paras)-1].Text
	if strings.Contains(text, "%s") {
		t.Fatalf("通知含未格式化 %%s: %q", text)
	}
	if !strings.Contains(text, "1.5s") {
		t.Fatalf("通知应含耗时 1.5s: %q", text)
	}
	if m.hudLatMs != 1500 {
		t.Fatalf("hudLatMs = %d, want 1500", m.hudLatMs)
	}
}

// TestBashNonZeroExitRed 验证 bash 非零退出码(isError=false)判为错误:
// stateError + 红色(fatal) + suffix (exit=N)。
func TestBashNonZeroExitRed(t *testing.T) {
	m := NewModel(ModelConfig{Theme: "dark"})
	_ = m.Init()
	p := NewProjector(m)
	m.AttachProjector(p)

	const callID = "call_00_bash127"
	p.ReplayEvent(todoCallEv(callID, "bash", `{"command":"nope"}`))
	// 宿主:isError=false + terminal view exitCode=127
	data, _ := json.Marshal(map[string]any{
		"message": map[string]any{
			"content": []any{
				map[string]any{
					"type":       "tool-result",
					"toolCallId": callID,
					"isError":    false,
					"content":    []any{map[string]any{"type": "text", "text": "[stderr]\ncommand not found\n[exit code: 127]"}},
				},
			},
		},
	})
	raw := json.RawMessage(`{"for":"result","view":{"card":"terminal","title":"nope","output":"[stderr]\ncommand not found\n[exit code: 127]","exitCode":127}}`)
	p.ReplayEventWithView(&dsh.SessionEvent{Type: dsh.EvToolResult, Data: data, Time: 1000}, raw)

	para := m.paras[0]
	if para.State != stateError {
		t.Fatalf("state = %v, want stateError(非零退出)", para.State)
	}
	if !para.ToolFatal {
		t.Fatal("非零退出应 fatal(红色)")
	}
	if got := toolSuffix(&m.paras[0], m.msg()); got != "(exit=127)" {
		t.Fatalf("suffix = %q, want (exit=127)", got)
	}
	// exit 0 不误判:仍是 done
	const callID2 = "call_00_bash0"
	p.ReplayEvent(todoCallEv(callID2, "bash", `{"command":"true"}`))
	data2, _ := json.Marshal(map[string]any{
		"message": map[string]any{
			"content": []any{
				map[string]any{
					"type":       "tool-result",
					"toolCallId": callID2,
					"content":    []any{map[string]any{"type": "text", "text": "ok"}},
				},
			},
		},
	})
	raw2 := json.RawMessage(`{"for":"result","view":{"card":"terminal","title":"true","output":"ok","exitCode":0}}`)
	p.ReplayEventWithView(&dsh.SessionEvent{Type: dsh.EvToolResult, Data: data2, Time: 1500}, raw2)
	para2 := m.paras[1]
	if para2.State != stateCollapsed || para2.ToolExitCode != 0 {
		t.Fatalf("exit 0 不应判错: state=%v exit=%d", para2.State, para2.ToolExitCode)
	}
}

// TestSearchViewMatches 验证 grep 的 search 卡片(matches shape):
// 折叠预览 path:line、展开态文件头+行号、truncated 提示。
func TestSearchViewMatches(t *testing.T) {
	m := NewModel(ModelConfig{Theme: "dark"})
	_ = m.Init()
	p := NewProjector(m)
	m.AttachProjector(p)

	const callID = "call_00_grep"
	p.ReplayEvent(todoCallEv(callID, "grep", `{"pattern":"x"}`))
	raw := json.RawMessage(`{"for":"result","view":{"card":"search","shape":"matches","title":"grep x","truncated":true,"total":5,"files":[{"path":"/w/a.go","matches":[{"lineNumber":12,"line":"func a() {"},{"lineNumber":45,"line":"func b() {"}]},{"path":"/w/b.go","matches":[{"lineNumber":3,"line":"x = 1"}]}]}}`)
	p.ReplayEventWithView(todoResultEv(callID, 1000), raw)

	para := m.paras[0]
	if len(para.SearchGroups) != 2 {
		t.Fatalf("SearchGroups = %d, want 2", len(para.SearchGroups))
	}
	if !para.SearchTruncated || para.SearchTotal != 5 {
		t.Fatalf("truncated = %v total = %d", para.SearchTruncated, para.SearchTotal)
	}
	// 折叠预览:path:line 格式
	var sb strings.Builder
	renderSearchPreview(&sb, &m.paras[0], 60, "", ViewportCtx{LC: m.msg()})
	out := sb.String()
	if !strings.Contains(out, "a.go:12") || !strings.Contains(out, "func a() {") {
		t.Fatalf("预览缺失:\n%s", out)
	}
	// 展开态:文件头 + 行号
	sb.Reset()
	renderSearchView(&sb, &m.paras[0], 60, "", ViewportCtx{LC: m.msg()})
	out = sb.String()
	if !strings.Contains(out, "/w/a.go") || !strings.Contains(out, "/w/b.go") {
		t.Fatalf("文件头缺失:\n%s", out)
	}
	if !strings.Contains(out, "共 5 条") && !strings.Contains(out, "5 total") {
		t.Fatalf("truncated 提示缺失:\n%s", out)
	}
}

// TestSearchViewPaths 验证 glob 的 search 卡片(paths shape)。
func TestSearchViewPaths(t *testing.T) {
	m := NewModel(ModelConfig{Theme: "dark"})
	_ = m.Init()
	p := NewProjector(m)
	m.AttachProjector(p)

	const callID = "call_00_glob"
	p.ReplayEvent(todoCallEv(callID, "glob", `{"pattern":"**/*.go"}`))
	raw := json.RawMessage(`{"for":"result","view":{"card":"search","shape":"paths","title":"Glob **/*.go","truncated":false,"total":2,"paths":["/w/a.go","/w/b.go"]}}`)
	p.ReplayEventWithView(todoResultEv(callID, 1000), raw)

	para := m.paras[0]
	if len(para.SearchPaths) != 2 || para.SearchPaths[0] != "/w/a.go" {
		t.Fatalf("SearchPaths = %+v", para.SearchPaths)
	}
	if para.SearchGroups != nil {
		t.Fatal("paths shape 不应有 Groups")
	}
	// isExpandable:glob 结构化结果可展开
	if !isExpandable(&m.paras[0], 60) {
		t.Fatal("glob search 应可展开")
	}
	var sb strings.Builder
	renderSearchView(&sb, &m.paras[0], 60, "", ViewportCtx{LC: m.msg()})
	if !strings.Contains(sb.String(), "a.go") {
		t.Fatalf("展开缺失:\n%s", sb.String())
	}
}

// TestGrepGlobArgsDisplay 验证 grep/glob 摘要行展示 pattern(非原始 JSON)。
func TestGrepGlobArgsDisplay(t *testing.T) {
	grepArgs := `{"pattern":"func render","path":"/w","include":"*.go"}`
	if got := formatToolArgs("grep", grepArgs, "/w"); got != "func render" {
		t.Fatalf("grep args = %q, want pattern", got)
	}
	globArgs := `{"pattern":"**/*_test.go"}`
	if got := formatToolArgs("glob", globArgs, "/w"); got != "**/*_test.go" {
		t.Fatalf("glob args = %q", got)
	}
	if strings.Contains(formatToolArgs("grep", grepArgs, "/w"), "{") {
		t.Fatal("grep 摘要不应含原始 JSON")
	}
}

// TestGrepGlobSuffix 验证 grep/glob suffix 统计:
// grep → (N matches, 耗时),glob → (N paths, 耗时)。
func TestGrepGlobSuffix(t *testing.T) {
	m := NewModel(ModelConfig{Theme: "dark"})
	_ = m.Init()
	p := NewProjector(m)
	m.AttachProjector(p)

	// grep:文本 "Found 4 matches"
	p.ReplayEvent(todoCallEv("c1", "grep", `{"pattern":"x"}`))
	resultData, _ := json.Marshal(map[string]any{
		"message": map[string]any{
			"content": []any{
				map[string]any{
					"type":       "tool-result",
					"toolCallId": "c1",
					"content":    []any{map[string]any{"type": "text", "text": "Found 4 matches\n\na.go\nLine 1: x"}},
				},
			},
		},
	})
	p.ReplayEvent(&dsh.SessionEvent{Type: dsh.EvToolResult, Data: resultData, Time: 1200})
	if got := toolSuffix(&m.paras[0], m.msg()); got != "(4 matches, 200ms)" {
		t.Fatalf("grep suffix = %q, want (4 matches, 200ms)", got)
	}

	// glob:路径列表 3 行
	p.ReplayEvent(todoCallEv("c2", "glob", `{"pattern":"*.go"}`))
	globData, _ := json.Marshal(map[string]any{
		"message": map[string]any{
			"content": []any{
				map[string]any{
					"type":       "tool-result",
					"toolCallId": "c2",
					"content":    []any{map[string]any{"type": "text", "text": "a.go\nb.go\nc.go"}},
				},
			},
		},
	})
	p.ReplayEvent(&dsh.SessionEvent{Type: dsh.EvToolResult, Data: globData, Time: 2000})
	if got := toolSuffix(&m.paras[1], m.msg()); got != "(3 paths, 1.0s)" {
		t.Fatalf("glob suffix = %q, want (3 paths, 1.0s)", got)
	}
}

// TestCountGrepMatches 验证 "Found N matches" 解析。
func TestCountGrepMatches(t *testing.T) {
	if got := countGrepMatches("Found 4 matches\n\na.go"); got != 4 {
		t.Fatalf("count = %d, want 4", got)
	}
	if got := countGrepMatches("no matches found"); got != 0 {
		t.Fatalf("count = %d, want 0", got)
	}
}

// TestTurnEndLoopTokens 验证 Done 通知显示"本轮" token(waveloom 语义),
// 而非会话累计值。
func TestTurnEndLoopTokens(t *testing.T) {
	m := NewModel(ModelConfig{Theme: "dark"})
	_ = m.Init()
	p := NewProjector(m)
	m.AttachProjector(p)

	// 回合 1:turn/start(重置) → assistant/message usage 100/200
	p.replayEvent(&dsh.SessionEvent{Type: dsh.EvTurnStart, Data: json.RawMessage(`{"turn":1}`), Time: 1000}, nil, false)
	msgData1, _ := json.Marshal(map[string]any{
		"message": map[string]any{"content": []any{map[string]any{"type": "text", "text": "hi"}}},
		"usage":   map[string]any{"inputTokens": 100, "outputTokens": 200},
	})
	p.replayEvent(&dsh.SessionEvent{Type: dsh.EvAssistantMsg, Data: msgData1, Time: 1500}, nil, false)
	turnEnd1, _ := json.Marshal(map[string]any{"turn": 1, "reason": map[string]any{"kind": "completed"}})
	p.replayEvent(&dsh.SessionEvent{Type: dsh.EvTurnEnd, Data: turnEnd1, Time: 2000}, nil, false)

	// 回合 2:turn/start 重置 → usage 300/400 → Done 应显示 300/400 而非累计 400/600
	p.replayEvent(&dsh.SessionEvent{Type: dsh.EvTurnStart, Data: json.RawMessage(`{"turn":2}`), Time: 3000}, nil, false)
	msgData2, _ := json.Marshal(map[string]any{
		"message": map[string]any{"content": []any{map[string]any{"type": "text", "text": "yo"}}},
		"usage":   map[string]any{"inputTokens": 300, "outputTokens": 400},
	})
	p.replayEvent(&dsh.SessionEvent{Type: dsh.EvAssistantMsg, Data: msgData2, Time: 3500}, nil, false)
	turnEnd2, _ := json.Marshal(map[string]any{"turn": 2, "reason": map[string]any{"kind": "completed"}})
	p.replayEvent(&dsh.SessionEvent{Type: dsh.EvTurnEnd, Data: turnEnd2, Time: 4000}, nil, false)

	// 最后一条通知是回合 2 的 Done
	var last string
	for i := range m.paras {
		if m.paras[i].Type == paraSystem {
			last = m.paras[i].Text
		}
	}
	if !strings.Contains(last, "↑300") || !strings.Contains(last, "↓400") {
		t.Fatalf("Done 应显示本轮 token 300/400: %q", last)
	}
	if strings.Contains(last, "↑400") {
		t.Fatalf("Done 不应显示累计 token: %q", last)
	}
	// HUD 累计保持 400/600
	if m.hudPromptTokens != 400 || m.hudComplTokens != 600 {
		t.Fatalf("hud 累计 = %d/%d, want 400/600", m.hudPromptTokens, m.hudComplTokens)
	}
}

// TestHudTurnsAndMessages 验证 HUD 回合/消息计数来源:
// turn/end 设置回合数,user/assistant 消息递增消息数(回放也恢复)。
func TestHudTurnsAndMessages(t *testing.T) {
	m := NewModel(ModelConfig{Theme: "dark"})
	_ = m.Init()
	p := NewProjector(m)
	m.AttachProjector(p)

	// 回放路径(模拟 resume 恢复):user + assistant + turn/end
	userData, _ := json.Marshal(map[string]any{
		"message": map[string]any{"content": []any{map[string]any{"type": "text", "text": "hi"}}},
	})
	p.ReplayEvent(&dsh.SessionEvent{Type: dsh.EvUserMessage, Data: userData, Time: 1000})
	asstData, _ := json.Marshal(map[string]any{
		"message": map[string]any{"content": []any{map[string]any{"type": "text", "text": "yo"}}},
	})
	p.ReplayEvent(&dsh.SessionEvent{Type: dsh.EvAssistantMsg, Data: asstData, Time: 1500})
	turnEnd, _ := json.Marshal(map[string]any{"turn": 115, "reason": map[string]any{"kind": "completed"}})
	p.ReplayEvent(&dsh.SessionEvent{Type: dsh.EvTurnEnd, Data: turnEnd, Time: 2000})

	if m.hudTurns != 115 {
		t.Fatalf("hudTurns = %d, want 115", m.hudTurns)
	}
	if m.hudMessages != 2 {
		t.Fatalf("hudMessages = %d, want 2", m.hudMessages)
	}
}

// TestContextPressureProjection 验证 ctx 进度条数据来自宿主投影
// (contextPressure.pressureTokens/contextWindow)。
func TestContextPressureProjection(t *testing.T) {
	m := NewModel(ModelConfig{Theme: "dark"})
	_ = m.Init()
	p := NewProjector(m)
	m.AttachProjector(p)

	// 初始:无数据 → ctx --
	if got := m.renderCtxBarCompact(); !strings.Contains(got, "--") {
		t.Fatalf("初始 ctx 应为 --: %q", got)
	}
	// projection 帧(contextPressure)
	p.ReplayFrame(dsh.ServerRequest{
		Method: "session/projection",
		Payload: mustJSON(map[string]any{
			"type":  "session/projection",
			"key":   "contextPressure",
			"value": map[string]any{"pressureTokens": 58030, "projectedTokens": 58155, "contextWindow": 1000000},
		}),
	})
	if m.lastPromptTokens != 58030 {
		t.Fatalf("lastPromptTokens = %d, want 58030", m.lastPromptTokens)
	}
	if m.projectedTokens != 58155 {
		t.Fatalf("projectedTokens = %d, want 58155", m.projectedTokens)
	}
	if m.contextLimit != 1000000 {
		t.Fatalf("contextLimit = %d, want 1000000", m.contextLimit)
	}
	got := m.renderCtxBarCompact()
	if strings.Contains(got, "--") {
		t.Fatalf("ctx 进度条不应为空: %q", got)
	}
	if !strings.Contains(got, "58.2K") || !strings.Contains(got, "1.0M") {
		t.Fatalf("ctx 进度条内容(应优先 projectedTokens): %q", got)
	}
	// sessionStats 投影 → HUD 回合数
	p.ReplayFrame(dsh.ServerRequest{
		Method: "session/projection",
		Payload: mustJSON(map[string]any{
			"type":  "session/projection",
			"key":   "sessionStats",
			"value": map[string]any{"turns": 119, "steps": 400, "llmMs": 53400},
		}),
	})
	if m.hudTurns != 119 {
		t.Fatalf("hudTurns = %d, want 119", m.hudTurns)
	}
}

func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}

// TestTokenUsageProjection 验证 tokenUsage 投影覆盖 cache/tok 显示值。
func TestTokenUsageProjection(t *testing.T) {
	m := NewModel(ModelConfig{Theme: "dark"})
	_ = m.Init()
	p := NewProjector(m)
	m.AttachProjector(p)
	// 本地累计先设一些值
	m.hudCacheHit = 10
	m.hudCacheMiss = 90

	p.ReplayFrame(dsh.ServerRequest{
		Method: "session/projection",
		Payload: mustJSON(map[string]any{
			"type":  "session/projection",
			"key":   "tokenUsage",
			"value": map[string]any{"uncachedInputTokens": 47256, "outputTokens": 17523, "cacheReadTokens": 1499392, "cacheWriteTokens": 0},
		}),
	})
	if m.hudCacheHit != 1499392 || m.hudCacheMiss != 0 {
		t.Fatalf("cache = %d/%d, want 1499392/0", m.hudCacheHit, m.hudCacheMiss)
	}
	if m.hudPromptTokens != 1499392+47256 {
		t.Fatalf("prompt = %d, want 1546648", m.hudPromptTokens)
	}
	if m.hudComplTokens != 17523 {
		t.Fatalf("compl = %d, want 17523", m.hudComplTokens)
	}
}

// TestCtxBarDisplay 验证 ctx 进度条显示 token 文本(572K/1.0M),
// 不显示百分比数字(格数即视觉近似)。
func TestCtxBarDisplay(t *testing.T) {
	m := NewModel(ModelConfig{Theme: "dark"})
	_ = m.Init()
	m.lastPromptTokens = 572000
	m.contextLimit = 1000000
	got := m.renderCtxBarCompact()
	if !strings.Contains(got, "572K/1.0M") {
		t.Fatalf("ctx 应显示 token 文本: %q", got)
	}
	if strings.Contains(got, "%") {
		t.Fatalf("ctx 不应显示百分比数字: %q", got)
	}
}

// TestSessionTitleProjection 验证 title 投影显示在 header(优先于 session id)。
func TestSessionTitleProjection(t *testing.T) {
	m := NewModel(ModelConfig{Theme: "dark"})
	_ = m.Init()
	p := NewProjector(m)
	m.AttachProjector(p)
	m.sessionID = "session-ba61dc5b-96de-4601-8cd5-c9975cb70a9f"
	m.width = 80

	// 无 title:显示 session id
	out := m.renderHeader()
	if !strings.Contains(out, "ba61dc5b") {
		t.Fatalf("无 title 时应显示 session id: %q", out)
	}
	// title 投影到达(JSON 字符串)
	p.ReplayFrame(dsh.ServerRequest{
		Method: "session/projection",
		Payload: mustJSON(map[string]any{
			"type":  "session/projection",
			"key":   "title",
			"value": "你是谁",
		}),
	})
	if m.sessionTitle != "你是谁" {
		t.Fatalf("sessionTitle = %q", m.sessionTitle)
	}
	out = m.renderHeader()
	if !strings.Contains(out, "你是谁") {
		t.Fatalf("title 应显示在 header: %q", out)
	}
	// 短 id 显示(前 8 + … + 后 4),不含完整 id
	if !strings.Contains(out, "ba61dc5b…0a9f") {
		t.Fatalf("应显示短 session id: %q", out)
	}
	if strings.Contains(out, "96de-4601") {
		t.Fatalf("不应显示完整 session id: %q", out)
	}
}

// TestElapSemantics 验证 elap 语义:本轮耗时(实时 turn/start 启动计时,
// 事件时间差结束),不被 sessionStats.llmMs 累计值覆盖。
func TestElapSemantics(t *testing.T) {
	m := NewModel(ModelConfig{Theme: "dark"})
	_ = m.Init()
	p := NewProjector(m)
	m.AttachProjector(p)

	// sessionStats 投影(llmMs 累计)不应污染 elap
	p.ReplayFrame(dsh.ServerRequest{
		Method: "session/projection",
		Payload: mustJSON(map[string]any{
			"type":  "session/projection",
			"key":   "sessionStats",
			"value": map[string]any{"turns": 131, "steps": 805, "llmMs": 5305313},
		}),
	})
	if m.hudTurns != 131 {
		t.Fatalf("hudTurns = %d, want 131", m.hudTurns)
	}
	if m.hudLatMs != 0 {
		t.Fatalf("llmMs 不应覆盖 hudLatMs: %d", m.hudLatMs)
	}
	// 实时回合:turn/start 启动计时 → turn/end 事件时间差(1.5s)
	p.replayEvent(&dsh.SessionEvent{Type: dsh.EvTurnStart, Data: json.RawMessage(`{"turn":132}`), Time: 1000}, nil, false)
	if m.turnStartTime.IsZero() {
		t.Fatal("实时 turn/start 应设置 turnStartTime")
	}
	turnEnd, _ := json.Marshal(map[string]any{"turn": 132, "reason": map[string]any{"kind": "completed"}})
	p.replayEvent(&dsh.SessionEvent{Type: dsh.EvTurnEnd, Data: turnEnd, Time: 2500}, nil, false)
	if m.hudLatMs != 1500 {
		t.Fatalf("hudLatMs = %d, want 1500(事件时间差)", m.hudLatMs)
	}
	if got := m.renderLatency(); !strings.Contains(got, "1.5s") {
		t.Fatalf("elap 应显示本轮耗时: %q", got)
	}
}

// TestThinkingEffortDisplay 验证 effort 显示在 HUD 模型名后。
func TestThinkingEffortDisplay(t *testing.T) {
	m := NewModel(ModelConfig{Theme: "dark"})
	_ = m.Init()
	m.hudModel = "deepseek-v4-flash"
	m.width = 80

	// 无 effort:不显示
	if out := m.renderFooter(); strings.Contains(out, "effort") {
		t.Fatalf("无 effort 不应显示: %q", out)
	}
	// 设置 effort
	m.SetThinkingEffort("high")
	out := m.renderFooter()
	if !strings.Contains(out, "effort") || !strings.Contains(out, "high") {
		t.Fatalf("应显示 effort: %q", out)
	}
	m.SetThinkingEffort("max")
	if out := m.renderFooter(); !strings.Contains(out, "effort") || !strings.Contains(out, "max") {
		t.Fatalf("max effort 应显示: %q", out)
	}
}

// TestReplayFrameSessionFilter 验证 mux 帧按当前会话过滤:
// 其他会话的事件/投影不会串进当前视图。
func TestReplayFrameSessionFilter(t *testing.T) {
	m := NewModel(ModelConfig{Theme: "dark"})
	_ = m.Init()
	p := NewProjector(m)
	m.AttachProjector(p)
	p.SessionID = "sess-current"

	// 其他会话的事件:忽略
	otherCall := dsh.ServerRequest{
		Method: "session/event",
		Payload: mustJSON(map[string]any{
			"type": "session/event", "sessionId": "sess-other",
			"event": map[string]any{"type": "user/message", "seq": 1, "data": map[string]any{
				"content": []any{map[string]any{"type": "text", "text": "other session"}},
			}},
		}),
	}
	p.ReplayFrame(otherCall)
	if len(m.paras) != 0 {
		t.Fatalf("其他会话事件不应投影: %d paras", len(m.paras))
	}
	// 当前会话的事件:正常投影
	ownCall := dsh.ServerRequest{
		Method: "session/event",
		Payload: mustJSON(map[string]any{
			"type": "session/event", "sessionId": "sess-current",
			"event": map[string]any{"type": "user/message", "seq": 2, "data": map[string]any{
				"content": []any{map[string]any{"type": "text", "text": "mine"}},
			}},
		}),
	}
	p.ReplayFrame(ownCall)
	if len(m.paras) != 1 {
		t.Fatalf("当前会话事件应投影: %d paras", len(m.paras))
	}
}

// TestJobToolsDisplay 验证 job 工具渲染:
// 参数摘要显示 job_id(wait 标记)、suffix 状态解析。
func TestJobToolsDisplay(t *testing.T) {
	// 参数摘要
	if got := formatToolArgs("job_output", `{"job_id":"job-abc"}`, ""); got != "job-abc" {
		t.Fatalf("job_output args = %q", got)
	}
	if got := formatToolArgs("job_output", `{"job_id":"job-abc","wait":true}`, ""); got != "job-abc (wait)" {
		t.Fatalf("job_output wait args = %q", got)
	}
	if got := formatToolArgs("job_kill", `{"job_id":"job-abc","reason":"stale"}`, ""); got != "job-abc" {
		t.Fatalf("job_kill args = %q", got)
	}
	if got := formatToolArgs("job_list", `{}`, ""); got != "" {
		t.Fatalf("job_list args = %q", got)
	}
	// 状态解析
	if got := parseJobStatus("hello\n[status: done]"); got != "done" {
		t.Fatalf("status = %q", got)
	}
	if got := parseJobStatus("running output"); got != "" {
		t.Fatalf("status = %q, want 空", got)
	}
	// suffix
	m := NewModel(ModelConfig{Theme: "dark"})
	_ = m.Init()
	p := NewProjector(m)
	m.AttachProjector(p)
	p.ReplayEvent(todoCallEv("c1", "job_output", `{"job_id":"job-abc"}`))
	data, _ := json.Marshal(map[string]any{
		"message": map[string]any{
			"content": []any{
				map[string]any{
					"type":       "tool-result",
					"toolCallId": "c1",
					"content":    []any{map[string]any{"type": "text", "text": "output line\n[status: done]"}},
				},
			},
		},
	})
	p.ReplayEvent(&dsh.SessionEvent{Type: dsh.EvToolResult, Data: data, Time: 1500})
	if got := toolSuffix(&m.paras[0], m.msg()); got != "(done)" {
		t.Fatalf("job_output suffix = %q, want (done)", got)
	}
}

// TestRemainingToolArgs 验证其余宿主工具的摘要行(不再显示原始 JSON)。
func TestRemainingToolArgs(t *testing.T) {
	cases := []struct {
		name string
		args string
		want string
	}{
		{"read_image", `{"file_path":"/w/a.png"}`, "a.png"},
		{"pwsh", `{"command":"Get-Process"}`, "Get-Process"},
		{"ralph", `{"objective":"fix the bug"}`, "fix the bug"},
		{"create_goal", `{"objective":"long objective here"}`, "long objective here"},
		{"get_goal", `{}`, ""},
		{"update_goal", `{"goal_id":"g1","action":"pause"}`, "g1 pause"},
		{"interrupt_agent", `{"agent_id":"a1"}`, "a1"},
		{"send_message", `{"agent_id":"a1","message":"hello there"}`, "a1 hello there"},
		{"report", `{"content":"done"}`, ""},
		{"str_replace_editor", `{"command":"view","file_path":"/w/f.go"}`, "view f.go"},
	}
	for _, c := range cases {
		got := formatToolArgs(c.name, c.args, "/w")
		if got != c.want {
			t.Fatalf("%s args = %q, want %q", c.name, got, c.want)
		}
		if strings.Contains(got, "{") {
			t.Fatalf("%s 不应含原始 JSON: %q", c.name, got)
		}
	}
}
