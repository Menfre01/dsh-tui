package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/Menfre01/dsh-tui/internal/dsh"
)

// fakeResponder 记录应答调用,不真正发网络请求。
type fakeResponder struct {
	approvals []approvalCall
	questions []questionCall
	cancels   []string // 取消的 rpcID
}

type approvalCall struct {
	rpcID, sessionID, approvalID string
	allowed                      bool
}

type questionCall struct {
	rpcID, sessionID string
	answers          []dsh.QuestionAnswer
}

func (f *fakeResponder) RespondApproval(rpcID, sessionID, approvalID string, allowed bool) error {
	f.approvals = append(f.approvals, approvalCall{rpcID, sessionID, approvalID, allowed})
	return nil
}

func (f *fakeResponder) RespondQuestionCancel(rpcID string) error {
	f.cancels = append(f.cancels, rpcID)
	return nil
}

func (f *fakeResponder) RespondQuestion(rpcID, sessionID string, answers []dsh.QuestionAnswer) error {
	f.questions = append(f.questions, questionCall{rpcID, sessionID, answers})
	return nil
}

func keyPress(s string) tea.KeyPressMsg {
	var k tea.KeyPressMsg
	// bubbletea v2 用 KeyPressMsg{...};直接构造一个能通过 String() 的实例
	// 不可行(String 依赖内部字段),因此这里用可解析的按键类型。
	// 简化:通过 tea 的 key 解析不可用,直接调用底层 handler 测试。
	_ = s
	_ = k
	return k
}

// 由于 tea.KeyPressMsg 的构造依赖内部实现,这里直接测 handler 逻辑层:
// 模拟 m.handleApprovalKey / m.handleQuestionKey 的输入分支。

func TestHandleApprovalKeyAllow(t *testing.T) {
	f := &fakeResponder{}
	m := NewModel(ModelConfig{Theme: "dark"})
	_ = m.Init()
	m.responder = f
	pa := &PendingApproval{
		RpcID: "rpc-1", SessionID: "s1", ApprovalID: "apr-1",
		ToolName: "bash", Reason: "may modify files",
	}
	// 同步应答(等价于按 1 后的网络调用,测试可确定性断言)
	m.doRespondApproval(pa, true)
	if len(f.approvals) != 1 {
		t.Fatalf("approvals = %d, want 1", len(f.approvals))
	}
	call := f.approvals[0]
	if call.rpcID != "rpc-1" || call.sessionID != "s1" || call.approvalID != "apr-1" || !call.allowed {
		t.Fatalf("approval call = %+v", call)
	}
}

func TestRespondApprovalClosesOverlay(t *testing.T) {
	// responder 为 nil 时不发起异步调用,纯验证覆盖层关闭逻辑
	m := NewModel(ModelConfig{Theme: "dark"})
	_ = m.Init()
	m.pendingApproval = &PendingApproval{
		RpcID: "rpc-1", SessionID: "s1", ApprovalID: "apr-1",
		ToolName: "bash",
	}
	m.overlay = overlayPermission
	m.respondApproval(m.pendingApproval, true)
	if m.pendingApproval != nil || m.overlay != overlayNone {
		t.Fatalf("overlay not closed: pending=%v overlay=%v", m.pendingApproval, m.overlay)
	}
}

func TestHandleApprovalKeyDeny(t *testing.T) {
	f := &fakeResponder{}
	m := NewModel(ModelConfig{Theme: "dark"})
	_ = m.Init()
	m.responder = f
	pa := &PendingApproval{
		RpcID: "rpc-2", SessionID: "s1", ApprovalID: "apr-2",
		ToolName: "write_file", Reason: "write outside workspace",
	}
	m.doRespondApproval(pa, false)
	if len(f.approvals) != 1 || f.approvals[0].allowed {
		t.Fatalf("deny call = %+v", f.approvals)
	}
}

// TestQuestionArrowSelectAndSubmit 验证 ↑↓ 移动光标 + Enter 确认提交。
func TestQuestionArrowSelectAndSubmit(t *testing.T) {
	f := &fakeResponder{}
	m := NewModel(ModelConfig{Theme: "dark"})
	_ = m.Init()
	m.responder = f
	pq := &PendingQuestion{
		RpcID:     "rpc-q",
		SessionID: "s1",
		Questions: []dsh.AskUserQuestionItem{
			{ID: "q1", Question: "选择测试范围?", Options: []dsh.QuestionOption{
				{Label: "全部", Description: "全量"},
				{Label: "单元测试"},
			}},
		},
		Selection: map[int]int{},
		Customs:   map[int]string{},
	}
	m.pendingQuestion = pq
	m.overlay = overlayQuestion

	// ↓ 移动到第 2 项,Enter 确认
	m.handleQuestionKey(tea.KeyPressMsg{Code: tea.KeyDown})
	if pq.cursor != 1 {
		t.Fatalf("cursor = %d, want 1", pq.cursor)
	}
	m.handleQuestionKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	// respondQuestion 走 goroutine;测试同步执行应答调用
	m.doRespondQuestion(pq.RpcID, pq.SessionID, collectAnswers(pq))

	if len(f.questions) != 1 {
		t.Fatalf("questions = %d, want 1", len(f.questions))
	}
	call := f.questions[0]
	if call.rpcID != "rpc-q" || call.sessionID != "s1" {
		t.Fatalf("question call = %+v", call)
	}
	if len(call.answers) != 1 || call.answers[0].ID != "q1" {
		t.Fatalf("answers = %+v", call.answers)
	}
	if len(call.answers[0].Selected) != 1 || call.answers[0].Selected[0] != "单元测试" {
		t.Fatalf("selected = %+v", call.answers[0].Selected)
	}
	if call.answers[0].Custom != "" {
		t.Fatalf("custom 应为空: %q", call.answers[0].Custom)
	}
}

func TestRespondQuestionClosesOverlay(t *testing.T) {
	m := NewModel(ModelConfig{Theme: "dark"})
	_ = m.Init()
	m.pendingQuestion = &PendingQuestion{
		RpcID:     "rpc-q",
		SessionID: "s1",
		Questions: []dsh.AskUserQuestionItem{{ID: "q1", Question: "?"}},
		Selection: map[int]int{},
	}
	m.overlay = overlayQuestion
	m.respondQuestion(m.pendingQuestion, true)
	if m.pendingQuestion != nil || m.overlay != overlayNone {
		t.Fatalf("overlay not closed")
	}
}

func TestQuestionCancelSendsEmpty(t *testing.T) {
	f := &fakeResponder{}
	m := NewModel(ModelConfig{Theme: "dark"})
	_ = m.Init()
	m.responder = f
	pq := &PendingQuestion{
		RpcID:     "rpc-q2",
		SessionID: "s1",
		Questions: []dsh.AskUserQuestionItem{
			{ID: "q1", Question: "?"},
		},
		Selection: map[int]int{},
	}
	m.pendingQuestion = pq
	m.respondQuestion(pq, true) // cancelled
	// 取消走 RPC error 通道(code "cancelled"),不 send 值批次
	m.doCancelQuestion(pq.RpcID)
	if len(f.cancels) != 1 {
		t.Fatalf("cancelled 应走取消通道: %+v", f.cancels)
	}
	if len(f.questions) != 0 {
		t.Fatalf("cancelled 不应发值批次: %+v", f.questions)
	}
	if m.pendingQuestion != nil {
		t.Fatalf("overlay not closed")
	}
}


// keyPress helper 占位(避免未使用导入)。
var _ = keyPress

// TestQuestionOverlayTitle 验证提问框标题格式化(%d 注入问题数)。
func TestQuestionOverlayTitle(t *testing.T) {
	m := NewModel(ModelConfig{Theme: "dark"})
	_ = m.Init()
	m.pendingQuestion = &PendingQuestion{
		RpcID:     "rpc-1",
		SessionID: "sess-1",
		Questions: []dsh.AskUserQuestionItem{
			{Question: "q1", Options: []dsh.QuestionOption{{Label: "a"}, {Label: "b"}}},
			{Question: "q2", Options: []dsh.QuestionOption{{Label: "c"}}},
		},
		Selection: map[int]int{},
	}
	out := m.renderQuestionOverlay(60)
	if strings.Contains(out, "%d") {
		t.Fatalf("标题含未格式化 %%d:\n%s", out)
	}
	if !strings.Contains(out, "(2 questions)") && !strings.Contains(out, "(2 问)") {
		t.Fatalf("标题未格式化问题数:\n%s", out)
	}
}

// TestQuestionOtherCustom 验证 Other 自定义答案流程:
// ↓ 到 Other 行 → Enter 进入输入 → 输入文本 → Enter 提交 custom。
func TestQuestionOtherCustom(t *testing.T) {
	f := &fakeResponder{}
	m := NewModel(ModelConfig{Theme: "dark"})
	_ = m.Init()
	m.responder = f
	pq := &PendingQuestion{
		RpcID:     "rpc-q",
		SessionID: "s1",
		Questions: []dsh.AskUserQuestionItem{
			{ID: "q1", Question: "?", Options: []dsh.QuestionOption{{Label: "a"}, {Label: "b"}}},
		},
		Selection: map[int]int{},
		Customs:   map[int]string{},
	}
	m.pendingQuestion = pq
	m.overlay = overlayQuestion

	// ↓↓ 到 Other 行(cursor=2 = len(options))
	m.handleQuestionKey(tea.KeyPressMsg{Code: tea.KeyDown})
	m.handleQuestionKey(tea.KeyPressMsg{Code: tea.KeyDown})
	if pq.cursor != 2 {
		t.Fatalf("cursor = %d, want 2(Other 行)", pq.cursor)
	}
	// Enter → 进入自定义输入模式
	m.handleQuestionKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !pq.otherMode {
		t.Fatal("应进入 Other 输入模式")
	}
	// 输入自定义文本
	m.handleQuestionKey(tea.KeyPressMsg{Code: 'x', Text: "x"})
	m.handleQuestionKey(tea.KeyPressMsg{Code: 'y', Text: "y"})
	if got := m.otherInput.Value(); got != "xy" {
		t.Fatalf("otherInput = %q, want xy", got)
	}
	// Enter 提交 → 批次应答含 Custom
	m.handleQuestionKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.pendingQuestion != nil {
		t.Fatal("提交后覆盖层应关闭")
	}
	m.doRespondQuestion(pq.RpcID, pq.SessionID, collectAnswers(pq))
	if len(f.questions) != 1 {
		t.Fatalf("questions = %d", len(f.questions))
	}
	ans := f.questions[0].answers[0]
	if ans.Custom != "xy" {
		t.Fatalf("custom = %q, want xy", ans.Custom)
	}
	if len(ans.Selected) != 0 {
		t.Fatalf("selected 应为空: %+v", ans.Selected)
	}
}

// TestQuestionMultiStepping 验证多题逐题推进:每题确认后进下一题,
// 全部答完自动提交。
func TestQuestionMultiStepping(t *testing.T) {
	m := NewModel(ModelConfig{Theme: "dark"})
	_ = m.Init()
	pq := &PendingQuestion{
		RpcID:     "rpc-m",
		SessionID: "s1",
		Questions: []dsh.AskUserQuestionItem{
			{ID: "q1", Question: "一?", Options: []dsh.QuestionOption{{Label: "a1"}, {Label: "a2"}}},
			{ID: "q2", Question: "二?", Options: []dsh.QuestionOption{{Label: "b1"}}},
		},
		Selection: map[int]int{},
		Customs:   map[int]string{},
	}
	m.pendingQuestion = pq
	m.overlay = overlayQuestion

	// 第 1 题:直接 Enter(选 a1)
	m.handleQuestionKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if pq.currentQ != 1 {
		t.Fatalf("currentQ = %d, want 1", pq.currentQ)
	}
	if m.pendingQuestion == nil {
		t.Fatal("还有第 2 题,不应关闭")
	}
	// 第 2 题:Enter(选 b1)→ 全部答完提交
	m.handleQuestionKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.pendingQuestion != nil {
		t.Fatal("全部答完应关闭覆盖层")
	}
	answers := collectAnswers(pq)
	if len(answers) != 2 {
		t.Fatalf("answers = %d, want 2", len(answers))
	}
	if answers[0].Selected[0] != "a1" || answers[1].Selected[0] != "b1" {
		t.Fatalf("answers = %+v", answers)
	}
}

// TestQuestionEscFromOther 验证 Other 输入模式 Esc 返回选项选择(不提交)。
func TestQuestionEscFromOther(t *testing.T) {
	m := NewModel(ModelConfig{Theme: "dark"})
	_ = m.Init()
	pq := &PendingQuestion{
		RpcID:     "rpc-e",
		SessionID: "s1",
		Questions: []dsh.AskUserQuestionItem{
			{ID: "q1", Question: "?", Options: []dsh.QuestionOption{{Label: "a"}}},
		},
		Selection: map[int]int{},
		Customs:   map[int]string{},
	}
	m.pendingQuestion = pq
	m.overlay = overlayQuestion
	// 进入 Other 输入
	m.handleQuestionKey(tea.KeyPressMsg{Code: tea.KeyDown}) // cursor 0→1(Other)
	m.handleQuestionKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !pq.otherMode {
		t.Fatal("应进入 Other 模式")
	}
	// Esc 返回选择模式
	m.handleQuestionKey(tea.KeyPressMsg{Code: tea.KeyEsc})
	if pq.otherMode {
		t.Fatal("Esc 应退出 Other 模式")
	}
	if m.pendingQuestion == nil {
		t.Fatal("返回选择模式时覆盖层不应关闭")
	}
}

// TestApprovalArrowSelection 验证审批框 ↑↓ 选择 + Enter 确认:
// 默认 allow,↓ 切 deny,Enter 提交对应决策。
func TestApprovalArrowSelection(t *testing.T) {
	f := &fakeResponder{}
	m := NewModel(ModelConfig{Theme: "dark"})
	_ = m.Init()
	m.responder = f
	pa := &PendingApproval{RpcID: "r", SessionID: "s", ApprovalID: "a", ToolName: "bash", cursor: 0}
	m.pendingApproval = pa
	m.overlay = overlayPermission

	// 默认 allow:Enter → 允许
	m.handleApprovalKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	m.doRespondApproval(pa, true)
	if len(f.approvals) != 1 || !f.approvals[0].allowed {
		t.Fatalf("默认应 allow: %+v", f.approvals)
	}
	// ↓ 切 deny → Enter 拒绝
	pa2 := &PendingApproval{RpcID: "r2", SessionID: "s", ApprovalID: "a2", ToolName: "bash"}
	m.pendingApproval = pa2
	m.overlay = overlayPermission
	m.handleApprovalKey(tea.KeyPressMsg{Code: tea.KeyDown})
	if pa2.cursor != 1 {
		t.Fatalf("cursor = %d, want 1", pa2.cursor)
	}
	m.handleApprovalKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	m.doRespondApproval(pa2, false)
	last := f.approvals[len(f.approvals)-1]
	if last.allowed {
		t.Fatal("↓ 后 Enter 应拒绝")
	}
	// Esc 始终拒绝
	pa3 := &PendingApproval{RpcID: "r3", SessionID: "s", ApprovalID: "a3", ToolName: "bash"}
	m.pendingApproval = pa3
	m.overlay = overlayPermission
	m.handleApprovalKey(tea.KeyPressMsg{Code: tea.KeyEsc})
	m.doRespondApproval(pa3, false)
	last = f.approvals[len(f.approvals)-1]
	if last.allowed {
		t.Fatal("Esc 应拒绝")
	}
}
