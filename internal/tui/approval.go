package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/Menfre01/dsh-tui/internal/dsh"
)

// ---------------------------------------------------------------------------
// approval.go — dsh 审批/提问应答 UI(阶段 2)
//
// 对应 waveloom 的 tui_permission.go,但驱动源换成 dsh 的
// approval/requested 与 question/requested 下行帧:
//
//	approval/requested → 阻断式确认框 → RespondApproval(allowed-once|rejected)
//	question/requested → 阻断式选择题 → RespondQuestion(answers 批次)
//
// 应答经 Responder 接口回调到 main(实际调用 /api/respond)。
// ---------------------------------------------------------------------------

// Responder 由 main 注入,负责把用户决策发往 dsh host。
type Responder interface {
	RespondApproval(rpcID, sessionID, approvalID string, allowed bool) error
	RespondQuestion(rpcID, sessionID string, answers []dsh.QuestionAnswer) error
	// RespondQuestionCancel 取消提问(dsh 协议:respond 返回 code "cancelled",
	// 宿主据此 claim cancelled 收尾;空 answers 批次会被拒绝)
	RespondQuestionCancel(rpcID string) error
}

// PendingApproval 待用户决策的审批请求。
type PendingApproval struct {
	RpcID      string
	SessionID  string
	ApprovalID string
	ToolName   string
	CallID     string
	Reason     string
	// cursor 审批选项光标:0 = 允许一次,1 = 拒绝(↑↓ 选择)
	cursor int
	// Diffs 是该 call 的 tool/call 视图导出的 diff 预览(edit/write 等
	// 改动文件的工具;非 diff 卡片为空)。审批帧本身不带 view,
	// 由投影器按 callId 从 call 视图缓存反查。
	Diffs []DiffHunk
}

// PendingQuestion 待用户回答的问题批次(一次 ask = 多题,一个批次应答)。
// 交互对齐 waveloom:逐题 ↑↓ 选择 + Enter 确认,末尾 Other 输入自定义答案。
type PendingQuestion struct {
	RpcID     string
	SessionID string
	Questions []dsh.AskUserQuestionItem
	// Selection[questionIdx] = optionIdx(单选;v1 不支持多选勾选)
	Selection map[int]int
	// Customs[questionIdx] = Other 自定义答案
	Customs map[int]string
	// 逐题交互状态
	currentQ  int // 当前答题题号
	cursor    int // 当前题内选项光标(0..len(Options),len = Other 行)
	otherMode bool // 正在输入自定义答案
}

// renderApprovalOverlay 渲染审批确认框。
func (m *model) renderApprovalOverlay(boxWidth int) string {
	pa := m.pendingApproval
	if pa == nil {
		return ""
	}
	lc := m.msg()
	var lines []string
	lines = append(lines, lipgloss.NewStyle().Bold(true).Foreground(colorAccentGold).Render("🔒 "+lc.PermRequired))
	lines = append(lines, "")
	lines = append(lines, styleOverlayBody.Render(
		lipgloss.NewStyle().Bold(true).Render(pa.ToolName)+" "+
			lipgloss.NewStyle().Foreground(colorMuted).Render(pa.CallID)))
	if pa.Reason != "" {
		lines = append(lines, styleOverlayBody.Render(pa.Reason))
	}
	// 待审批改动预览(diff 卡片):复用 diff 折叠渲染,限制在框内宽度
	if len(pa.Diffs) > 0 {
		innerWidth := boxWidth - 6 // border(2) + padding(4)
		if innerWidth < 20 {
			innerWidth = 20
		}
		var db strings.Builder
		renderDiffPreview(&db, pa.Diffs, innerWidth, "", ViewportCtx{LC: m.msg()})
		if db.Len() > 0 {
			lines = append(lines, "")
			lines = append(lines, styleOverlayBody.Render(strings.TrimSuffix(db.String(), "\n")))
		}
	}
	lines = append(lines, "")
	// 选项(↑↓ 选择):允许一次 / 拒绝
	allowLabel := lc.PermAllow
	denyLabel := lc.PermDeny
	if pa.cursor == 0 {
		lines = append(lines, styleOverlayBody.Render(
			lipgloss.NewStyle().Foreground(colorOK).Render("▌ ")+
				lipgloss.NewStyle().Bold(true).Foreground(colorHeaderAccent).Render(allowLabel)))
		lines = append(lines, styleOverlayBody.Render("  "+denyLabel))
	} else {
		lines = append(lines, styleOverlayBody.Render("  "+allowLabel))
		lines = append(lines, styleOverlayBody.Render(
			lipgloss.NewStyle().Foreground(colorErr).Render("▌ ")+
				lipgloss.NewStyle().Bold(true).Foreground(colorHeaderAccent).Render(denyLabel)))
	}
	lines = append(lines, "")
	lines = append(lines, styleOverlayBody.Render(
		"[↑↓] "+lc.KeyNav+"    [Enter] "+lc.KeyConfirm+"    [Esc] "+lc.KeyDeny))
	return renderOverlayBox(boxWidth, m.overlayAnimFrame, strings.Join(lines, "\n"))
}

// renderQuestionOverlay 渲染问题选择框(↑↓ 选择,Enter 确认,末尾 Other 自定义)。
func (m *model) renderQuestionOverlay(boxWidth int) string {
	pq := m.pendingQuestion
	if pq == nil {
		return ""
	}
	lc := m.msg()
	var lines []string
	title := fmt.Sprintf("❓ "+lc.ToolNQuestions, len(pq.Questions))
	if len(pq.Questions) > 1 {
		title += fmt.Sprintf("  [%d/%d]", pq.currentQ+1, len(pq.Questions))
	}
	lines = append(lines, lipgloss.NewStyle().Bold(true).Foreground(colorHeaderAccent).Render(title))
	lines = append(lines, "")

	if pq.currentQ < len(pq.Questions) {
		q := pq.Questions[pq.currentQ]
		lines = append(lines, styleOverlayBody.Render(
			lipgloss.NewStyle().Bold(true).Render(q.Question)))
		for oi := range q.Options {
			opt := q.Options[oi]
			label := opt.Label
			// 推荐选项 ★ 前缀(与 waveloom huh 一致)
			if strings.HasSuffix(label, "(Recommended)") {
				label = "★ " + label
			}
			desc := ""
			if opt.Description != "" {
				desc = " " + lipgloss.NewStyle().Foreground(colorMuted).Render("— "+opt.Description)
			}
			if pq.cursor == oi {
				// 聚焦行:绿色 ▌ 光标 + 高亮粗体文字
				line := lipgloss.NewStyle().Foreground(colorOK).Render("▌ ") +
					lipgloss.NewStyle().Bold(true).Foreground(colorHeaderAccent).Render(label)
				lines = append(lines, styleOverlayBody.Render(line+desc))
			} else {
				lines = append(lines, styleOverlayBody.Render("  "+label+desc))
			}
		}
		// Other 自定义答案行
		otherLabel := lc.QuestionOtherOption
		if pq.cursor == len(q.Options) {
			lines = append(lines, styleOverlayBody.Render(
				lipgloss.NewStyle().Foreground(colorOK).Render("▌ ")+
					lipgloss.NewStyle().Bold(true).Foreground(colorHeaderAccent).Render(otherLabel)))
		} else {
			lines = append(lines, styleOverlayBody.Render("  "+otherLabel))
		}
		if pq.otherMode {
			lines = append(lines, "")
			lines = append(lines, styleOverlayBody.Render(
				lipgloss.NewStyle().Foreground(colorMuted).Render(lc.QuestionOtherPlaceholder)))
			lines = append(lines, m.otherInput.View())
		}
	}

	lines = append(lines, "")
	if pq.otherMode {
		lines = append(lines, styleOverlayBody.Render(
			"[Enter] "+lc.KeyConfirm+"    [Esc] "+lc.KeyBack))
	} else {
		lines = append(lines, styleOverlayBody.Render(
			"[↑↓/jk] "+lc.KeyToggle+"    [Enter] "+lc.KeyConfirm+"    [Esc] "+lc.KeyDeny))
	}
	return renderOverlayBox(boxWidth, m.overlayAnimFrame, strings.Join(lines, "\n"))
}

// currentOptions 返回当前题的选项列表。
func (pq *PendingQuestion) currentOptions() []dsh.QuestionOption {
	if pq.currentQ < 0 || pq.currentQ >= len(pq.Questions) {
		return nil
	}
	return pq.Questions[pq.currentQ].Options
}

// confirmCurrent 确认光标项:普通选项 → 记录并进下一题;Other → 进入自定义输入。
func (pq *PendingQuestion) confirmCurrent(m *model) {
	opts := pq.currentOptions()
	if pq.cursor < len(opts) {
		if pq.Selection == nil {
			pq.Selection = map[int]int{}
		}
		pq.Selection[pq.currentQ] = pq.cursor
		pq.nextQuestion(m)
		return
	}
	// Other:初始化自定义输入框
	pq.otherMode = true
	ti := textinput.New()
	ti.Prompt = "> "
	ti.Placeholder = m.msg().QuestionOtherPlaceholder
	ti.CharLimit = 500
	ti.SetVirtualCursor(false)
	m.otherInput = ti
	m.otherInput.Focus()
}

// commitCustom 提交 Other 自定义答案并进下一题。
func (pq *PendingQuestion) commitCustom(m *model) {
	if pq.Customs == nil {
		pq.Customs = map[int]string{}
	}
	pq.Customs[pq.currentQ] = strings.TrimSpace(m.otherInput.Value())
	pq.nextQuestion(m)
}

// nextQuestion 推进到下一题;全部答完时提交批次。
func (pq *PendingQuestion) nextQuestion(m *model) {
	pq.otherMode = false
	pq.cursor = 0
	pq.currentQ++
	if pq.currentQ >= len(pq.Questions) {
		m.respondQuestion(pq, false)
	}
}

// handleApprovalKey 处理审批框按键(↑↓ 选择 + Enter 确认 + Esc 拒绝)。
func (m *model) handleApprovalKey(msg tea.KeyPressMsg) (bool, tea.Cmd) {
	pa := m.pendingApproval
	if pa == nil {
		return false, nil
	}
	keyStr := msg.String()
	switch keyStr {
	case "up", "k":
		if pa.cursor > 0 {
			pa.cursor--
		}
		return true, nil
	case "down", "j":
		if pa.cursor < 1 {
			pa.cursor++
		}
		return true, nil
	case "enter":
		m.respondApproval(pa, pa.cursor == 0)
		return true, nil
	case "esc":
		m.respondApproval(pa, false)
		return true, nil
	}
	return false, nil
}

// respondApproval 提交审批决策并关闭覆盖层。
func (m *model) respondApproval(pa *PendingApproval, allowed bool) {
	if m.responder != nil {
		go m.doRespondApproval(pa, allowed)
	}
	m.pendingApproval = nil
	m.overlay = overlayNone
	m.input.Focus()
}

// doRespondApproval 同步执行应答调用(测试可直接调用;生产走 goroutine)。
func (m *model) doRespondApproval(pa *PendingApproval, allowed bool) {
	if err := m.responder.RespondApproval(pa.RpcID, pa.SessionID, pa.ApprovalID, allowed); err != nil {
		if m.program != nil {
			m.program.Send(approvalRespondErrMsg{err: err})
		}
	}
}

// handleQuestionKey 处理问题框按键。
// 选择模式:↑↓ 移动光标,Enter 确认(Other → 自定义输入),Esc 取消。
// Other 模式:字符进 otherInput,Enter 提交自定义答案,Esc 返回选择。
func (m *model) handleQuestionKey(msg tea.KeyPressMsg) (bool, tea.Cmd) {
	pq := m.pendingQuestion
	if pq == nil {
		return false, nil
	}
	keyStr := msg.String()

	if pq.otherMode {
		switch keyStr {
		case "enter":
			pq.commitCustom(m)
			return true, nil
		case "esc":
			// 返回选项选择
			pq.otherMode = false
			m.otherInput.Blur()
			return true, nil
		}
		var cmd tea.Cmd
		m.otherInput, cmd = m.otherInput.Update(msg)
		return true, cmd
	}

	switch keyStr {
	case "up", "k":
		if pq.cursor > 0 {
			pq.cursor--
		}
		return true, nil
	case "down", "j":
		opts := pq.currentOptions()
		if pq.cursor <= len(opts) {
			pq.cursor++
		}
		return true, nil
	case "enter":
		pq.confirmCurrent(m)
		return true, nil
	case "esc":
		m.respondQuestion(pq, true)
		return true, nil
	}
	return false, nil
}

// respondQuestion 提交答案批次并关闭覆盖层。cancelled=true 时提交空批次
// (dsh 协议:空 answers 等价取消,宿主据此收尾 ask 并发出 resolved 帧)。
func (m *model) respondQuestion(pq *PendingQuestion, cancelled bool) {
	if m.responder != nil {
		if cancelled {
			// 取消走 RPC error 通道(code "cancelled"),非空批次
			go m.doCancelQuestion(pq.RpcID)
		} else {
			go m.doRespondQuestion(pq.RpcID, pq.SessionID, collectAnswers(pq))
		}
	}
	m.pendingQuestion = nil
	m.overlay = overlayNone
	m.input.Focus()
}

// doCancelQuestion 同步执行取消应答(测试可直接调用;生产走 goroutine)。
func (m *model) doCancelQuestion(rpcID string) {
	if err := m.responder.RespondQuestionCancel(rpcID); err != nil {
		if m.program != nil {
			m.program.Send(approvalRespondErrMsg{err: err})
		}
	}
}

// collectAnswers 把当前选择投影为应答批次(含 Other 自定义答案)。
func collectAnswers(pq *PendingQuestion) []dsh.QuestionAnswer {
	answers := make([]dsh.QuestionAnswer, 0, len(pq.Questions))
	for qi, q := range pq.Questions {
		ans := dsh.QuestionAnswer{ID: q.ID, Selected: []string{}}
		if custom, ok := pq.Customs[qi]; ok && custom != "" {
			ans.Custom = custom
		} else if oi, ok := pq.Selection[qi]; ok && oi < len(q.Options) {
			ans.Selected = []string{q.Options[oi].Label}
		}
		answers = append(answers, ans)
	}
	return answers
}

// doRespondQuestion 同步执行应答调用(测试可直接调用;生产走 goroutine)。
func (m *model) doRespondQuestion(rpcID, sessionID string, answers []dsh.QuestionAnswer) {
	if err := m.responder.RespondQuestion(rpcID, sessionID, answers); err != nil {
		if m.program != nil {
			m.program.Send(approvalRespondErrMsg{err: err})
		}
	}
}

// approvalRespondErrMsg respond 调用失败。
type approvalRespondErrMsg struct {
	err error
}
