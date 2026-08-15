package tui

import (
	"fmt"
	"strings"

	"github.com/atotto/clipboard"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// ---------------------------------------------------------------------------
// session.go — 会话列表与切换(阶段 3 MVP)
//
// 交互: Ctrl+S 打开/关闭会话列表覆盖层;↑/↓ 导航;Enter 切换;N 新建。
// 数据源: main 启动时 session.list + host/ 帧增量(project.go 维护)。
// ---------------------------------------------------------------------------

// sessionListVisible 是否显示会话列表覆盖层。
func (m *model) sessionListVisible() bool { return m.overlay == overlaySessionList }

// toggleSessionList 打开/关闭会话列表。
func (m *model) toggleSessionList() {
	if m.sessionListVisible() {
		m.overlay = overlayNone
		m.input.Focus()
		return
	}
	m.overlay = overlaySessionList
	m.input.Blur()
}

// handleSessionListKey 处理会话列表按键。返回 (handled, cmd)。
func (m *model) handleSessionListKey(msg tea.KeyPressMsg) (bool, tea.Cmd) {
	keyStr := msg.String()
	switch keyStr {
	case "ctrl+s":
		m.toggleSessionList()
		return true, nil
	case "up":
		if m.sessionListIdx > 0 {
			m.sessionListIdx--
		}
		return true, nil
	case "down":
		if m.sessionListIdx < len(m.sessions)-1 {
			m.sessionListIdx++
		}
		return true, nil
	case "enter":
		if m.sessionListIdx >= 0 && m.sessionListIdx < len(m.sessions) {
			target := m.sessions[m.sessionListIdx].SessionID
			m.overlay = overlayNone
			m.input.Focus()
			if m.onSwitchSession != nil {
				m.onSwitchSession(target)
			}
		}
		return true, nil
	case "y", "c":
		// 复制选中会话的完整 id 到剪贴板(供 --resume 使用)
		if m.sessionListIdx >= 0 && m.sessionListIdx < len(m.sessions) {
			fullID := m.sessions[m.sessionListIdx].SessionID
			if err := clipboard.WriteAll(fullID); err == nil {
				m.appendSystem("copied: "+fullID, notifInfo)
			}
		}
		return true, nil
	case "n":
		m.overlay = overlayNone
		m.input.Focus()
		if m.onNewSession != nil {
			m.onNewSession()
		}
		return true, nil
	case "esc":
		m.overlay = overlayNone
		m.input.Focus()
		return true, nil
	}
	return false, nil
}

// renderSessionListOverlay 渲染会话列表覆盖层。
func (m *model) renderSessionListOverlay(boxWidth int) string {
	lc := m.msg()
	var lines []string
	title := lipgloss.NewStyle().Bold(true).Foreground(colorHeaderAccent).Render("☰ " + lc.PickerSelectSession)
	lines = append(lines, title)
	lines = append(lines, "")

	if len(m.sessions) == 0 {
		lines = append(lines, styleOverlayBody.Render(lc.PickerNoResults))
	} else {
		for i, s := range m.sessions {
			marker := "  "
			if i == m.sessionListIdx {
				marker = "› "
			}
			running := ""
			if s.Running {
				running = styleFooterLatRed.Render("●")
			}
			cwd := s.Cwd
			if len(cwd) > 40 {
				cwd = "…" + cwd[len(cwd)-39:]
			}
			line := fmt.Sprintf("%s%s %s %s",
				marker,
				running,
				styleOverlayBody.Render(shortSessionID(s.SessionID)),
				styleOverlayBody.Render(cwd))
			if i == m.sessionListIdx {
				line = styleOverlayBody.Bold(true).Render(line)
			}
			lines = append(lines, line)
			if i == m.sessionListIdx && m.sessionListIdx < len(m.sessions) {
				// 选中项下方显示完整 id(y 复制)
				lines = append(lines, styleOverlayBody.Render(
					lipgloss.NewStyle().Foreground(colorMuted).Render("   id: " + m.sessions[i].SessionID)))
			}
		}
	}

	lines = append(lines, "")
	lines = append(lines, styleOverlayBody.Render(
		"[↑/↓] "+lc.KeyNav+"    [Enter] "+lc.KeyConfirm+"    [N] "+lc.PickerNewSession+"    [Y] "+lc.KeyCopyID+"    [Esc] "+lc.KeyCancel))
	return renderOverlayBox(boxWidth, m.overlayAnimFrame, strings.Join(lines, "\n"))
}

// shortSessionID 截短会话 ID 便于列表显示。
func shortSessionID(id string) string {
	// 跳过 "session-" 前缀,只截 UUID 段
	if strings.HasPrefix(id, "session-") {
		id = id[len("session-"):]
	}
	if len(id) <= 14 {
		return id
	}
	return id[:8] + "…" + id[len(id)-4:]
}
