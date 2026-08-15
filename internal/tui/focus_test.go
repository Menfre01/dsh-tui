package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

// mkToolPara 构造一个工具段落。
func mkToolPara(name, result string, state paraStateEnum, diffs []DiffHunk) Paragraph {
	return Paragraph{
		Type:       paraTool,
		State:      state,
		ToolName:   name,
		ToolResult: result,
		DiffHunks:  diffs,
	}
}

func longResult(n int) string {
	s := ""
	for i := 0; i < n; i++ {
		s += "some output line with enough text to wrap at narrow width\n"
	}
	return s
}

// TestIsExpandable 验证可展开判定:bash 长输出/edit(diff)/thought 可展开,
// grep 短输出不可。
func TestIsExpandable(t *testing.T) {
	cases := []struct {
		para Paragraph
		want bool
	}{
		{mkToolPara("bash", longResult(20), stateDone, nil), true},
		{mkToolPara("bash", "short", stateDone, nil), false},
		{mkToolPara("edit", "applied", stateCollapsed, []DiffHunk{{FilePath: "/x"}}), true},
		{mkToolPara("edit", "applied", stateCollapsed, nil), false},
		{mkToolPara("grep", "a.txt:1\nb.txt:2\n", stateDone, nil), false},
		{mkToolPara("read", "x", stateDone, nil), false},
		{Paragraph{Type: paraThought, State: stateCollapsed, Text: longResult(20)}, true},
		{Paragraph{Type: paraThought, State: stateCollapsed, Text: "short"}, false},
		{Paragraph{Type: paraTool, State: stateStreaming, ToolName: "bash"}, false},
	}
	for i, c := range cases {
		if got := isExpandable(&c.para, 60); got != c.want {
			t.Fatalf("case %d (%s state=%v): isExpandable = %v, want %v", i, c.para.ToolName, c.para.State, got, c.want)
		}
	}
}

// pressKey 注入按键并返回更新后的 model。
func pressKey(t *testing.T, m *model, msg tea.KeyPressMsg) *model {
	updated, _ := m.Update(msg)
	um, ok := updated.(*model)
	if !ok {
		t.Fatalf("Update returned %T", updated)
	}
	return um
}

// TestFocusNextCycle 验证 Tab 聚焦到第一个可展开段落,再 Tab 环移到下一个。
func TestFocusNextCycle(t *testing.T) {
	m := NewModel(ModelConfig{Theme: "dark"})
	_ = m.Init()
	m.paras = []Paragraph{
		mkToolPara("grep", "a:1\n", stateDone, nil),  // 不可展开
		mkToolPara("bash", longResult(20), stateDone, nil), // 可展开
		mkToolPara("edit", "applied", stateCollapsed, []DiffHunk{{FilePath: "/x"}}), // 可展开
	}
	m.width = 80

	// Tab → 聚焦到 bash(idx 1)
	m = pressKey(t, m, tea.KeyPressMsg{Code: tea.KeyTab})
	if m.focusIndex != 1 {
		t.Fatalf("first tab focusIndex = %d, want 1", m.focusIndex)
	}
	// Tab → 环移到 edit(idx 2)
	m = pressKey(t, m, tea.KeyPressMsg{Code: tea.KeyTab})
	if m.focusIndex != 2 {
		t.Fatalf("second tab focusIndex = %d, want 2", m.focusIndex)
	}
	// Tab → 环回 bash(idx 1)
	m = pressKey(t, m, tea.KeyPressMsg{Code: tea.KeyTab})
	if m.focusIndex != 1 {
		t.Fatalf("third tab focusIndex = %d, want 1", m.focusIndex)
	}
}

// TestFocusPrevCycle 验证 Shift+Tab 反向环移。
func TestFocusPrevCycle(t *testing.T) {
	m := NewModel(ModelConfig{Theme: "dark"})
	_ = m.Init()
	m.paras = []Paragraph{
		mkToolPara("bash", longResult(20), stateDone, nil),
		mkToolPara("edit", "applied", stateCollapsed, []DiffHunk{{FilePath: "/x"}}),
	}
	m.width = 80

	m = pressKey(t, m, tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	if m.focusIndex != 1 {
		t.Fatalf("shift+tab focusIndex = %d, want 1 (环形到末尾)", m.focusIndex)
	}
}

// TestEnterTogglesExpand 验证焦点模式下 Enter 展开/折叠,且不发送消息。
func TestEnterTogglesExpand(t *testing.T) {
	m := NewModel(ModelConfig{Theme: "dark"})
	_ = m.Init()
	sent := false
	m.onSend = func(string, string) { sent = true }
	m.paras = []Paragraph{mkToolPara("bash", longResult(20), stateDone, nil)}
	m.width = 80
	m.input.SetValue("hello") // 输入框有内容,验证焦点模式 Enter 不发送

	m = pressKey(t, m, tea.KeyPressMsg{Code: tea.KeyTab}) // 聚焦
	if m.paras[0].State != stateDone {
		t.Fatalf("初始 state = %v", m.paras[0].State)
	}
	m = pressKey(t, m, tea.KeyPressMsg{Code: tea.KeyEnter}) // 展开
	if m.paras[0].State != stateExpanded {
		t.Fatalf("enter 后 state = %v, want expanded", m.paras[0].State)
	}
	if sent {
		t.Fatal("焦点模式 Enter 不应发送消息")
	}
	m = pressKey(t, m, tea.KeyPressMsg{Code: tea.KeyEnter}) // 折叠回 done
	if m.paras[0].State != stateDone {
		t.Fatalf("再 enter 后 state = %v, want done", m.paras[0].State)
	}
}

// TestEscExitsFocusMode 验证焦点模式 Esc 返回输入框,不触发取消。
func TestEscExitsFocusMode(t *testing.T) {
	m := NewModel(ModelConfig{Theme: "dark"})
	_ = m.Init()
	cancelled := false
	m.onCancel = func() { cancelled = true }
	m.paras = []Paragraph{mkToolPara("bash", longResult(20), stateDone, nil)}
	m.width = 80

	m = pressKey(t, m, tea.KeyPressMsg{Code: tea.KeyTab})
	if m.focusIndex < 0 {
		t.Fatal("tab 未进入焦点模式")
	}
	m = pressKey(t, m, tea.KeyPressMsg{Code: tea.KeyEsc})
	if m.focusIndex != -1 {
		t.Fatalf("esc 后 focusIndex = %d, want -1", m.focusIndex)
	}
	if cancelled {
		t.Fatal("焦点模式 Esc 不应触发 cancel")
	}
}

// TestUpDownMoveFocus 验证焦点模式下 ↑↓ 移动焦点而非滚动。
func TestUpDownMoveFocus(t *testing.T) {
	m := NewModel(ModelConfig{Theme: "dark"})
	_ = m.Init()
	m.paras = []Paragraph{
		mkToolPara("bash", longResult(20), stateDone, nil),
		mkToolPara("edit", "applied", stateCollapsed, []DiffHunk{{FilePath: "/x"}}),
	}
	m.width = 80

	m = pressKey(t, m, tea.KeyPressMsg{Code: tea.KeyTab}) // → bash(0)
	m = pressKey(t, m, tea.KeyPressMsg{Code: tea.KeyDown}) // → edit(1)
	if m.focusIndex != 1 {
		t.Fatalf("down focusIndex = %d, want 1", m.focusIndex)
	}
	m = pressKey(t, m, tea.KeyPressMsg{Code: tea.KeyUp}) // → bash(0)
	if m.focusIndex != 0 {
		t.Fatalf("up focusIndex = %d, want 0", m.focusIndex)
	}
}

// TestNoExpandableFocusReset 验证无可展开段落时 Tab 归位到输入框。
func TestNoExpandableFocusReset(t *testing.T) {
	m := NewModel(ModelConfig{Theme: "dark"})
	_ = m.Init()
	m.paras = []Paragraph{
		mkToolPara("grep", "a:1\n", stateDone, nil),
		mkToolPara("glob", "a.go\n", stateDone, nil),
	}
	m.width = 80
	m = pressKey(t, m, tea.KeyPressMsg{Code: tea.KeyTab})
	if m.focusIndex != -1 {
		t.Fatalf("focusIndex = %d, want -1", m.focusIndex)
	}
}

// TestBgProbeTick 验证周期背景轮询:auto 模式发查询并续期 tick。
func TestBgProbeTick(t *testing.T) {
	m := NewModel(ModelConfig{Theme: "auto"})
	_ = m.Init()

	upd, cmd := m.Update(bgProbeMsg{})
	m = upd.(*model)
	if cmd == nil {
		t.Fatal("bgProbeMsg 应返回续期 cmd")
	}
	// 非 auto 模式:不查询但续期
	m.themeMode = "dark"
	_, cmd = m.Update(bgProbeMsg{})
	if cmd == nil {
		t.Fatal("非 auto 也应续期 tick")
	}
}
