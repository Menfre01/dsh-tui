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
	// 兜底清理:任何路径残留的重复项在打开列表时按 SessionID 收敛
	// (host 帧已去重,此处防御历史进程/上游返回的重复)。
	m.dedupeSessions()
	// 选中索引映射到可见列表:过滤(subagent/非当前 blank)后
	// 原索引可能越界或指向隐藏项,改为保持同一会话的可见位置。
	selected := ""
	if m.sessionListIdx >= 0 && m.sessionListIdx < len(m.sessions) {
		selected = m.sessions[m.sessionListIdx].SessionID
	}
	m.sessionListIdx = 0
	if selected != "" {
		for i, s := range m.visibleSessions() {
			if s.SessionID == selected {
				m.sessionListIdx = i
				break
			}
		}
	}
	m.overlay = overlaySessionList
	m.input.Blur()
}

// dedupeSessions 按 SessionID 去重 m.sessions(保留首个出现的项),
// 并修正 sessionListIdx 指向去重后同一会话。返回是否清理了重复。
func (m *model) dedupeSessions() bool {
	seen := make(map[string]int, len(m.sessions))
	uniq := make([]SessionBrief, 0, len(m.sessions))
	idxMap := make([]int, len(m.sessions))
	for i, s := range m.sessions {
		if s.SessionID == "" {
			idxMap[i] = -1
			continue
		}
		if j, ok := seen[s.SessionID]; ok {
			// 重复:保留首个(通常字段更全),后续丢弃
			idxMap[i] = j
			continue
		}
		seen[s.SessionID] = len(uniq)
		idxMap[i] = len(uniq)
		uniq = append(uniq, s)
	}
	if len(uniq) == len(m.sessions) {
		return false
	}
	cur := m.sessionListIdx
	m.sessions = uniq
	if cur >= 0 && cur < len(idxMap) && idxMap[cur] >= 0 {
		m.sessionListIdx = idxMap[cur]
	} else if m.sessionListIdx >= len(m.sessions) {
		m.sessionListIdx = max(len(m.sessions)-1, 0)
	}
	return true
}

// visibleSessions 返回按 web 可见性规则过滤后的会话列表
// (subagent 隐藏、非当前 blank 隐藏)。导航/渲染统一用它,
// 保证选中索引与显示一一对应。
func (m *model) visibleSessions() []SessionBrief {
	out := make([]SessionBrief, 0, len(m.sessions))
	for _, s := range m.sessions {
		if m.sessionVisible(s) {
			out = append(out, s)
		}
	}
	return out
}

// handleSessionListKey 处理会话列表按键。返回 (handled, cmd)。
func (m *model) handleSessionListKey(msg tea.KeyPressMsg) (bool, tea.Cmd) {
	keyStr := msg.String()
	vis := m.visibleSessions()
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
		if m.sessionListIdx < len(vis)-1 {
			m.sessionListIdx++
		}
		return true, nil
	case "enter":
		if m.sessionListIdx >= 0 && m.sessionListIdx < len(vis) {
			target := vis[m.sessionListIdx].SessionID
			m.overlay = overlayNone
			m.input.Focus()
			if m.onSwitchSession != nil {
				m.onSwitchSession(target)
			}
		}
		return true, nil
	case "y", "c":
		// 复制选中会话的完整 id 到剪贴板(供 --resume 使用)
		if m.sessionListIdx >= 0 && m.sessionListIdx < len(vis) {
			fullID := vis[m.sessionListIdx].SessionID
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

	if len(m.visibleSessions()) == 0 {
		lines = append(lines, styleOverlayBody.Render(lc.PickerNoResults))
	} else {
		// 固定高度窗口:最多可见 maxSessionListVisible 项,选中项滚动跟随
		vis := m.visibleSessions()
		const maxVisible = 8
		start := 0
		if m.sessionListIdx >= maxVisible {
			start = m.sessionListIdx - maxVisible + 1
		}
		end := start + maxVisible
		if end > len(vis) {
			end = len(vis)
		}
		if start > 0 {
			lines = append(lines, styleOverlayBody.Render(
				lipgloss.NewStyle().Foreground(colorMuted).Render(fmt.Sprintf("  ↑ %d more", start))))
		}
		for i := start; i < end; i++ {
			s := vis[i]
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
			// 与 dsh web 对齐:会话列表主体显示 title(投影),短 id + cwd 辅助
			label := shortSessionID(s.SessionID)
			if s.Title != "" {
				label = truncateByDisplayWidth(s.Title, 30) + "  " + label
			}
			line := fmt.Sprintf("%s%s %s %s",
				marker,
				running,
				styleOverlayBody.Render(label),
				styleOverlayBody.Render(cwd))
			if i == m.sessionListIdx {
				line = styleOverlayBody.Bold(true).Render(line)
			}
			lines = append(lines, line)
			if i == m.sessionListIdx {
				// 选中项下方显示完整 id(y 复制)
				lines = append(lines, styleOverlayBody.Render(
					lipgloss.NewStyle().Foreground(colorMuted).Render("   id: " + vis[i].SessionID)))
			}
			// 条目间留空行:列表更疏朗(选中项后接 id 行再留空)
			if i != end-1 {
				lines = append(lines, "")
			}
		}
		if end < len(vis) {
			lines = append(lines, styleOverlayBody.Render(
				lipgloss.NewStyle().Foreground(colorMuted).Render(fmt.Sprintf("  ↓ %d more", len(vis)-end))))
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
