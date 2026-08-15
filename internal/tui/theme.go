package tui

import (
	"os"

	tea "charm.land/bubbletea/v2"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	"charm.land/lipgloss/v2"
)

// ---------------------------------------------------------------------------
// theme.go — 主题功能移植(waveloom tui_command.go / tui.go)
//
// 交互: Ctrl+G 打开主题选择器;↑/↓ 导航;Enter 应用;Esc 关闭。
// 五套主题: auto / dark / light / darkcolorblind / lightcolorblind。
// auto 在 v1 简化为 dark(无环境亮度检测,阶段 4 可接 tea.BackgroundColor)。
// ---------------------------------------------------------------------------

// applyThemeMode 应用主题模式并同步所有主题相关组件。
func (m *model) applyThemeMode(mode string) {
	m.themeMode = mode
	p := paletteFor(mode)
	if mode == "auto" {
		// 与 waveloom 一致:切到 auto 时同步重新查询终端背景色
		// (lipgloss.HasDarkBackground 阻塞等 OSC 11 响应,时序确定;
		// tea 的 BackgroundColorMsg 是异步补充,见 reapplyAutoTheme)
		m.autoDark = lipgloss.HasDarkBackground(os.Stdin, os.Stdout)
		if m.autoIsDark() {
			p = darkPalette
		} else {
			p = lightPalette
		}
	}
	applyTheme(p)
	m.palette = p
	m.syncThemeComponents()
}

// autoIsDark 返回 auto 模式下当前是否应使用深色主题。
// 优先使用 Bubble Tea BackgroundColorMsg 的检测结果(更可靠),
// 未收到时回退到 lipgloss.HasDarkBackground 的初始检测结果。
func (m *model) autoIsDark() bool {
	if m.hasTeaBackground {
		return m.autoDarkFromTea
	}
	return m.autoDark
}

// reapplyAutoTheme 重新根据 auto 检测结果应用主题。
// 仅在 themeMode == "auto" 时调用(Bubble Tea 背景色检测完成后)。
func (m *model) reapplyAutoTheme() {
	var p palette
	if m.autoIsDark() {
		p = darkPalette
	} else {
		p = lightPalette
	}
	applyTheme(p)
	m.palette = p
	m.syncThemeComponents()
}

// syncThemeComponents 同步依赖主题的组件样式(spinners + glamour + 列表)。
func (m *model) syncThemeComponents() {
	// spinners(HUD + 段落流式前缀)
	m.spinner.Style = lipgloss.NewStyle().Foreground(colorOK)
	m.spAsst.Style = lipgloss.NewStyle().Foreground(colorOK)
	m.spThought.Style = lipgloss.NewStyle().Foreground(colorGray)
	m.spTool.Style = lipgloss.NewStyle().Foreground(colorGray)
	m.spSubagent.Style = lipgloss.NewStyle().Foreground(colorGray)
	m.spTodo.Style = lipgloss.NewStyle().Foreground(colorAccentGold)

	// glamour markdown 渲染器
	m.rebuildGlamour(max(m.width-6, 20))

	// 输入框样式(与 waveloom syncThemeComponents 同款)
	inputStyles := m.input.Styles()
	inputStyles.Focused.Prompt = lipgloss.NewStyle().Foreground(colorUser).Bold(true)
	inputStyles.Blurred.Prompt = lipgloss.NewStyle().Foreground(colorUser).Bold(true)
	inputStyles.Focused.Placeholder = lipgloss.NewStyle().Foreground(colorMuted)
	inputStyles.Blurred.Placeholder = lipgloss.NewStyle().Foreground(colorMuted)
	inputStyles.Cursor.Blink = true
	m.input.SetStyles(inputStyles)

	// 主题列表 delegate 样式
	if m.themeDelegate != nil {
		*m.themeDelegate = list.NewDefaultDelegate()
		m.themeDelegate.ShowDescription = false
		m.themeDelegate.SetSpacing(0)
		m.themeDelegate.Styles = listItemStyles()
		m.themeList.SetDelegate(*m.themeDelegate)
	}
}

// toggleThemePicker 打开/关闭主题选择器。
func (m *model) toggleThemePicker() {
	if m.overlay == overlayThemePicker {
		m.overlay = overlayNone
		m.input.Focus()
		return
	}
	m.buildThemeList()
	m.overlay = overlayThemePicker
	m.input.Blur()
}

// buildThemeList 构建主题选择列表覆盖层。
func (m *model) buildThemeList() {
	items := make([]list.Item, len(themeItems))
	selectedIdx := 0
	for i, ti := range themeItems {
		label := ti.label
		if ti.mode == "auto" {
			label = m.msg().PickerThemeAuto
		}
		items[i] = themeItem{label: label, mode: ti.mode}
		if ti.mode == m.themeMode {
			selectedIdx = i
		}
	}

	delegate := list.NewDefaultDelegate()
	delegate.ShowDescription = false
	delegate.SetSpacing(0)
	delegate.Styles = listItemStyles()
	m.themeDelegate = &delegate

	l := list.New(items, delegate, 0, len(items))
	l.SetShowTitle(false)
	l.SetShowPagination(false)
	l.SetShowStatusBar(false)
	l.SetShowFilter(false)
	l.SetShowHelp(false)
	l.KeyMap.Quit = key.NewBinding()
	l.KeyMap.ForceQuit = key.NewBinding()
	if selectedIdx < len(items) {
		l.Select(selectedIdx)
	}
	m.themeList = l
}

// handleThemePickerKey 处理主题选择器按键。返回 (handled, cmd)。
func (m *model) handleThemePickerKey(msg tea.KeyPressMsg) (bool, tea.Cmd) {
	keyStr := msg.String()
	switch keyStr {
	case "up":
		if m.themeList.Index() <= 0 {
			return true, nil
		}
		var cmd tea.Cmd
		m.themeList, cmd = m.themeList.Update(msg)
		return true, cmd
	case "down":
		if m.themeList.Index() >= len(themeItems)-1 {
			return true, nil
		}
		var cmd tea.Cmd
		m.themeList, cmd = m.themeList.Update(msg)
		return true, cmd
	case "enter":
		idx := m.themeList.Index()
		if idx >= 0 && idx < len(themeItems) {
			m.applyThemeMode(themeItems[idx].mode)
		}
		m.overlay = overlayNone
		m.input.Focus()
		// 切到 auto 时重新查询终端背景色(运行中切换配色后能跟上;
		// OSC 11 是查询-响应模式,不会主动推送)
		if m.themeMode == "auto" {
			cmd := m.input.Focus()
			return true, tea.Batch(cmd, func() tea.Msg { return tea.RequestBackgroundColor() })
		}
		return true, nil
	case "esc":
		m.overlay = overlayNone
		m.input.Focus()
		return true, nil
	}
	return false, nil
}
