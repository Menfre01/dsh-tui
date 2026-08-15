package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

// ---------------------------------------------------------------------------
// focus.go — 段落焦点模式(Tab 聚焦 → ⏎ 展开/折叠,↑↓ 移动,Esc 返回)
//
// 移植自 waveloom tui.go 的 focusNext/focusPrev/toggleParagraphFocus/
// enterFocusMode/exitFocusMode/isExpandable。渲染层(stateExpanded 分支、
// ctx.Focused 高亮、焦点分隔线)已在移植时完成,本文件补齐交互入口。
// ---------------------------------------------------------------------------

// enterFocusMode 进入段落焦点模式:输入框失焦、placeholder 提示操作。
func (m *model) enterFocusMode() {
	m.input.Blur()
	m.input.Placeholder = m.msg().InputFocusModePlaceholder
}

// exitFocusMode 退出段落焦点模式:输入框恢复焦点、placeholder 恢复。
func (m *model) exitFocusMode() tea.Cmd {
	m.input.Placeholder = m.msg().InputPlaceholder
	return m.input.Focus()
}

// focusNext 把焦点移到下一个可展开段落(环形);无焦点时进入焦点模式。
func (m *model) focusNext() {
	if len(m.paras) == 0 {
		m.focusIndex = -1
		m.exitFocusMode()
		return
	}
	contentWidth := max(m.width-4, 20)
	wasFocused := m.focusIndex >= 0
	start := m.focusIndex + 1
	if start < 0 {
		start = 0
	}
	for i := 0; i < len(m.paras); i++ {
		idx := (start + i) % len(m.paras)
		if isExpandable(&m.paras[idx], contentWidth) {
			m.focusIndex = idx
			if !wasFocused {
				m.enterFocusMode()
			}
			return
		}
	}
	// 无可展开段落 → 归位
	m.focusIndex = -1
	m.exitFocusMode()
}

// focusPrev 把焦点移到上一个可展开段落(环形)。
func (m *model) focusPrev() {
	if len(m.paras) == 0 {
		m.focusIndex = -1
		m.exitFocusMode()
		return
	}
	contentWidth := max(m.width-4, 20)
	wasFocused := m.focusIndex >= 0
	start := m.focusIndex - 1
	if start < 0 {
		start = len(m.paras) - 1
	}
	for i := 0; i < len(m.paras); i++ {
		idx := (start - i + len(m.paras)) % len(m.paras)
		if isExpandable(&m.paras[idx], contentWidth) {
			m.focusIndex = idx
			if !wasFocused {
				m.enterFocusMode()
			}
			return
		}
	}
	m.focusIndex = -1
	m.exitFocusMode()
}

// toggleParagraphFocus 切换当前聚焦段落的展开/折叠状态。
func (m *model) toggleParagraphFocus() {
	if m.focusIndex < 0 || m.focusIndex >= len(m.paras) {
		return
	}
	p := &m.paras[m.focusIndex]
	switch p.Type {
	case paraThought:
		switch p.State {
		case stateCollapsed:
			p.State = stateExpanded
		case stateExpanded:
			p.State = stateCollapsed
		}
	case paraTool:
		switch p.State {
		case stateDone, stateCollapsed:
			p.State = stateExpanded
		case stateError:
			p.State = stateExpanded
		case stateExpanded:
			if p.ToolError != "" || p.ToolDenied {
				p.State = stateError
			} else {
				p.State = stateDone
			}
		}
	case paraSubagent:
		switch p.State {
		case stateDone, stateCollapsed:
			p.State = stateExpanded
		case stateError:
			p.State = stateExpanded
		case stateExpanded:
			p.State = stateDone
		}
	}
	p.renderDirty = true
}

// isExpandable 判断段落是否值得展开/折叠交互。
// 长输出工具(bash 等)与 diff 视图工具(edit/write)可展开;
// 结构化摘要工具(如 grep/glob)预览即完整信息,不可展开。
func isExpandable(p *Paragraph, contentWidth int) bool {
	switch p.Type {
	case paraThought:
		if p.State != stateCollapsed && p.State != stateExpanded {
			return false
		}
		// 折叠预览仅展示前几行;全部内容换行后 ≤ 2 行则无需展开
		if p.State == stateCollapsed {
			return countWrappedLines(p.Text, contentWidth-2) > 2
		}
		return true
	case paraTool:
		switch p.ToolName {
		case "bash", "web_fetch", "web_search", "skill":
			if p.State != stateDone && p.State != stateCollapsed && p.State != stateExpanded && p.State != stateError {
				return false
			}
			if p.State == stateDone || p.State == stateCollapsed || p.State == stateError {
				body := stripToolStatusHeader(p.ToolResult)
				if body == "" && p.ToolError != "" {
					body = p.ToolError
				}
				return countWrappedLines(body, contentWidth-2) >= maxPreviewWrapped
			}
			return true
		case "edit", "write":
			// diff 视图:有结构化 hunks 即可展开看完整红绿 diff
			if p.DiffHunks == nil {
				return false
			}
			return p.State == stateDone || p.State == stateCollapsed || p.State == stateExpanded || p.State == stateError
		case "grep", "glob":
			// search 视图:有结构化分组/路径即可展开看完整结果
			if len(p.SearchGroups) == 0 && len(p.SearchPaths) == 0 {
				return false
			}
			return p.State == stateDone || p.State == stateCollapsed || p.State == stateExpanded || p.State == stateError
		}
		return false
	case paraSubagent:
		return p.State != stateStreaming
	}
	return false
}

// countWrappedLines 计算文本在指定宽度下换行后的总行数。
func countWrappedLines(text string, width int) int {
	if width < 1 {
		width = 1
	}
	total := 0
	for _, line := range strings.Split(text, "\n") {
		total += len(wrapLine(line, width))
	}
	return total
}
