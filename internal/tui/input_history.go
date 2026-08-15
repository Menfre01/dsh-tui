package tui

import (
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// input_history.go — 输入框历史导航与 Esc 双击清空(对齐 waveloom)
// ---------------------------------------------------------------------------

// saveToHistory 将输入保存到历史列表。跳过空输入和相邻重复。
func (m *model) saveToHistory(input string) {
	input = strings.TrimSpace(input)
	if input == "" {
		return
	}
	if len(m.inputHistory) > 0 && m.inputHistory[0] == input {
		return
	}
	m.inputHistory = append([]string{input}, m.inputHistory...)
	if len(m.inputHistory) > 100 {
		m.inputHistory = m.inputHistory[:100]
	}
	m.historyPos = -1
}

// navigateHistoryUp 向上导航历史(更早的输入)。返回 true 表示已消费。
func (m *model) navigateHistoryUp() bool {
	if len(m.inputHistory) == 0 {
		return false
	}
	if m.historyPos == -1 {
		// 首次进入历史导航,保存当前草稿
		m.historyDraft = m.input.Value()
		m.historyPos = 0
	} else if m.historyPos < len(m.inputHistory)-1 {
		m.historyPos++
	} else {
		// 已到最早记录,不再前进
		return true
	}
	m.input.SetValue(m.inputHistory[m.historyPos])
	m.input.CursorEnd()
	return true
}

// navigateHistoryDown 向下导航历史(更新的输入或回到草稿)。返回 true 表示已消费。
func (m *model) navigateHistoryDown() bool {
	if m.historyPos == -1 {
		return false
	}
	if m.historyPos > 0 {
		m.historyPos--
		m.input.SetValue(m.inputHistory[m.historyPos])
		m.input.CursorEnd()
		return true
	}
	// historyPos == 0,恢复到进入导航前的草稿
	m.historyPos = -1
	m.input.SetValue(m.historyDraft)
	m.input.CursorEnd()
	return true
}

// handleEscInIdle 处理空闲态 Esc:双击(500ms 内)清空输入框。
// 返回 true 表示已消费。运行中/覆盖层打开时由调用方先行处理。
func (m *model) handleEscInIdle() bool {
	if m.running || m.overlay != overlayNone || m.focusIndex >= 0 {
		return false
	}
	now := time.Now()
	if !m.lastEscTime.IsZero() && now.Sub(m.lastEscTime) < 500*time.Millisecond {
		m.input.Reset()
		m.lastEscTime = time.Time{}
		return true
	}
	m.lastEscTime = now
	return true // 单击 Esc 已消费(记录时间,等待双击)
}
