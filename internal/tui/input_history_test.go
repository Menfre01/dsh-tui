package tui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

// TestInputHistoryNavigation 验证 ↑↓ 历史导航:发送记录 → ↑ 回填 → ↓ 回草稿。
func TestInputHistoryNavigation(t *testing.T) {
	m := NewModel(ModelConfig{Theme: "dark"})
	_ = m.Init()
	sent := []string{}
	m.onSend = func(s string, _ string) { sent = append(sent, s) }

	// 发送两条消息(进入历史)
	for _, msg := range []string{"hello", "world"} {
		m.input.SetValue(msg)
		m = pressKey(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	}
	if len(sent) != 2 {
		t.Fatalf("sent = %d, want 2", len(sent))
	}
	if len(m.inputHistory) != 2 || m.inputHistory[0] != "world" {
		t.Fatalf("history = %v", m.inputHistory)
	}

	// 新草稿,↑ 回填最近一条
	m.input.SetValue("draft")
	m = pressKey(t, m, tea.KeyPressMsg{Code: tea.KeyUp})
	if got := m.input.Value(); got != "world" {
		t.Fatalf("↑ 后输入框 = %q, want world", got)
	}
	// 再 ↑ 到更早
	m = pressKey(t, m, tea.KeyPressMsg{Code: tea.KeyUp})
	if got := m.input.Value(); got != "hello" {
		t.Fatalf("再次 ↑ 后输入框 = %q, want hello", got)
	}
	// 再 ↑ 停在最早(不循环)
	m = pressKey(t, m, tea.KeyPressMsg{Code: tea.KeyUp})
	if got := m.input.Value(); got != "hello" {
		t.Fatalf("最早记录后 ↑ 不应再变: %q", got)
	}
	// ↓ 回到较新
	m = pressKey(t, m, tea.KeyPressMsg{Code: tea.KeyDown})
	if got := m.input.Value(); got != "world" {
		t.Fatalf("↓ 后输入框 = %q, want world", got)
	}
	// ↓ 回到草稿
	m = pressKey(t, m, tea.KeyPressMsg{Code: tea.KeyDown})
	if got := m.input.Value(); got != "draft" {
		t.Fatalf("↓ 回草稿 = %q, want draft", got)
	}
	// 草稿态再 ↓ → 不消费(无导航),滚动
	m = pressKey(t, m, tea.KeyPressMsg{Code: tea.KeyDown})
	if m.historyPos != -1 {
		t.Fatalf("historyPos = %d, want -1", m.historyPos)
	}
}

// TestEscDoubleTapClearsInput 验证空闲态双击 Esc 清空输入框。
func TestEscDoubleTapClearsInput(t *testing.T) {
	m := NewModel(ModelConfig{Theme: "dark"})
	_ = m.Init()
	m.input.SetValue("some text")

	// 单击:记录时间,不清空
	m = pressKey(t, m, tea.KeyPressMsg{Code: tea.KeyEsc})
	if got := m.input.Value(); got != "some text" {
		t.Fatalf("单击 esc 后 = %q", got)
	}
	if m.lastEscTime.IsZero() {
		t.Fatal("单击 esc 应记录时间")
	}
	// 双击(模拟 500ms 内):清空
	m.lastEscTime = time.Now()
	m = pressKey(t, m, tea.KeyPressMsg{Code: tea.KeyEsc})
	if got := m.input.Value(); got != "" {
		t.Fatalf("双击 esc 后输入框 = %q, want 空", got)
	}
}

// TestEscDoubleTapSlow 验证超过 500ms 的两次 Esc 不清空(时间重置)。
func TestEscDoubleTapSlow(t *testing.T) {
	m := NewModel(ModelConfig{Theme: "dark"})
	_ = m.Init()
	m.input.SetValue("keep")
	m.lastEscTime = time.Now().Add(-2 * time.Second) // 模拟上次 Esc 在很久前
	m = pressKey(t, m, tea.KeyPressMsg{Code: tea.KeyEsc})
	if got := m.input.Value(); got != "keep" {
		t.Fatalf("慢速双击不应清空: %q", got)
	}
}

// TestInputHistoryTrimAndDedup 验证历史去重与跳过空输入。
func TestInputHistoryTrimAndDedup(t *testing.T) {
	m := NewModel(ModelConfig{Theme: "dark"})
	_ = m.Init()
	m.onSend = func(string, string) {}
	m.input.SetValue("  same  ")
	m = pressKey(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	m.input.SetValue("same")
	m = pressKey(t, m, tea.KeyPressMsg{Code: tea.KeyEnter}) // 重复(trim 后)
	m.input.SetValue("   ")
	m = pressKey(t, m, tea.KeyPressMsg{Code: tea.KeyEnter}) // 空
	if len(m.inputHistory) != 1 {
		t.Fatalf("history = %v, want 1 条", m.inputHistory)
	}
	if m.inputHistory[0] != "same" {
		t.Fatalf("history[0] = %q", m.inputHistory[0])
	}
	if !strings.Contains(m.input.Value(), "") {
		t.Fatal("noop")
	}
}

// TestExitCommand 验证输入 exit(不区分大小写)退出,不发送消息。
func TestExitCommand(t *testing.T) {
	m := NewModel(ModelConfig{Theme: "dark"})
	_ = m.Init()
	sent := false
	m.onSend = func(string, string) { sent = true }

	for _, input := range []string{"exit", "EXIT", " Exit "} {
		m.input.SetValue(input)
		upd, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		m = upd.(*model)
		if cmd == nil {
			t.Fatalf("%q 未返回 Quit 命令", input)
		}
		if sent {
			t.Fatal("exit 不应作为消息发送")
		}
	}
	// 正常消息仍发送
	m.input.SetValue("not exit")
	m = pressKey(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if !sent {
		t.Fatal("非 exit 输入应正常发送")
	}
}
