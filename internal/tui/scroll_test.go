package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// fillContent 塞入足够内容使窗口可滚动。
func fillContent(m *model) {
	for i := 0; i < 50; i++ {
		m.paras = append(m.paras, Paragraph{Type: paraUser, State: stateDone, Text: strings.Repeat("line ", 10)})
	}
	m.scrollToBottom()
	_ = m.View() // 触发裁剪与 bodyHeight 计算
}

// TestScrollBaseline 验证基础滚动:up 减少 scrollTop、down 增加、到底重新锁定。
func TestScrollBaseline(t *testing.T) {
	m := NewModel(ModelConfig{Theme: "dark"})
	_ = m.Init()
	upd, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
	m = upd.(*model)
	fillContent(m)

	before := m.scrollTop
	m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	after := m.scrollTop
	if after >= before {
		t.Fatalf("up 未滚动: %d -> %d", before, after)
	}
	if m.pinnedToBottom {
		t.Fatal("up 后不应仍锁定底部")
	}

	// 连续 up 到顶
	for i := 0; i < 200; i++ {
		m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	}
	if m.scrollTop != 0 {
		t.Fatalf("up 到顶后 scrollTop=%d", m.scrollTop)
	}
	// down 回到底部 → View() 重新锁定
	for i := 0; i < 200; i++ {
		m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	}
	_ = m.View() // 触发 clamp 与重新锁定
	if !m.pinnedToBottom {
		t.Fatal("滚到底后应重新锁定底部")
	}
}

// TestScrollPageKeysInFocusMode 验证焦点模式下 PgUp/PgDn 仍全局滚动
// (对齐 waveloom:焦点模式不强制滚动位置)。
func TestScrollPageKeysInFocusMode(t *testing.T) {
	m := NewModel(ModelConfig{Theme: "dark"})
	_ = m.Init()
	upd, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
	m = upd.(*model)
	m.paras = append(m.paras,
		mkToolPara("bash", longResult(20), stateDone, nil),
		Paragraph{Type: paraUser, State: stateDone, Text: strings.Repeat("line ", 30)},
	)
	m.scrollToBottom()
	_ = m.View()

	// 进入焦点模式
	m = pressKey(t, m, tea.KeyPressMsg{Code: tea.KeyTab})
	if m.focusIndex < 0 {
		t.Fatal("tab 未聚焦")
	}
	// 向上滚动一页(焦点模式不拦截 PgUp)
	m = pressKey(t, m, tea.KeyPressMsg{Code: tea.KeyPgUp})
	if m.scrollTop >= m.bodyHeight*2 {
		t.Fatalf("PgUp 在焦点模式无效: scrollTop=%d", m.scrollTop)
	}
	// 焦点模式 ↑↓ 移动焦点而非滚动
	topBefore := m.scrollTop
	m = pressKey(t, m, tea.KeyPressMsg{Code: tea.KeyUp})
	if m.scrollTop != topBefore {
		t.Fatalf("焦点模式 ↑ 不应滚动: %d -> %d", topBefore, m.scrollTop)
	}
}

// TestMouseWheelScroll 验证滚轮滚动(对齐 waveloom:WheelUp/Down → 3 行)。
func TestMouseWheelScroll(t *testing.T) {
	m := NewModel(ModelConfig{Theme: "dark"})
	_ = m.Init()
	upd, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
	m = upd.(*model)
	fillContent(m)

	top := m.scrollTop
	// 滚轮向上(WheelUp = 查看更早内容,scrollTop 减少)
	upd, _ = m.Update(tea.MouseClickMsg{Button: tea.MouseWheelUp})
	m = upd.(*model)
	if got := m.scrollTop; got != top-3 {
		t.Fatalf("wheel up scrollTop = %d, want %d", got, top-3)
	}
	if m.pinnedToBottom {
		t.Fatal("滚轮向上后不应锁定底部")
	}
	// 滚轮向下
	upd, _ = m.Update(tea.MouseClickMsg{Button: tea.MouseWheelDown})
	m = upd.(*model)
	if got := m.scrollTop; got != top {
		t.Fatalf("wheel down scrollTop = %d, want %d", got, top)
	}
}

// TestNewContentHint 验证不在底部时新帧到达触发跳回提示,回底部清除。
func TestNewContentHint(t *testing.T) {
	m := NewModel(ModelConfig{Theme: "dark"})
	_ = m.Init()
	upd, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
	m = upd.(*model)
	fillContent(m)
	if m.hasNewContent {
		t.Fatal("初始不应有提示标记")
	}

	// 向上滚动离开底部
	m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	if m.hasNewContent {
		t.Fatal("滚动本身不应触发提示")
	}

	// 新下行帧到达 → 提示标记
	upd, _ = m.Update(DshFrameMsg{})
	m = upd.(*model)
	if !m.hasNewContent {
		t.Fatal("滚动后新帧到达应设置 hasNewContent")
	}
	// 回到底部 → 清除
	m.scrollToBottom()
	if m.hasNewContent {
		t.Fatal("回到底部后应清除提示")
	}
}

// TestPasteIntoInput 验证 bracketed paste 文本进入输入框。
func TestPasteIntoInput(t *testing.T) {
	m := NewModel(ModelConfig{Theme: "dark"})
	_ = m.Init()

	upd, _ := m.Update(tea.PasteMsg{Content: "line1\nline2\nline3"})
	m = upd.(*model)
	if got := m.input.Value(); got != "line1\nline2\nline3" {
		t.Fatalf("粘贴后输入框 = %q", got)
	}
}

// TestPasteIgnoredInFocusMode 验证焦点模式下粘贴被忽略。
func TestPasteIgnoredInFocusMode(t *testing.T) {
	m := NewModel(ModelConfig{Theme: "dark"})
	_ = m.Init()
	m.paras = []Paragraph{mkToolPara("bash", longResult(20), stateDone, nil)}
	m.width = 80
	m = pressKey(t, m, tea.KeyPressMsg{Code: tea.KeyTab}) // 进入焦点模式
	upd, _ := m.Update(tea.PasteMsg{Content: "ignored"})
	m = upd.(*model)
	if got := m.input.Value(); got != "" {
		t.Fatalf("焦点模式粘贴不应进输入框: %q", got)
	}
}
