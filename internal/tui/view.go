package tui

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/progress"
	"charm.land/bubbles/v2/spinner"
	"image/color"
	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"charm.land/glamour/v2"
	"charm.land/lipgloss/v2"

	"github.com/Menfre01/dsh-tui/internal/dsh"
)

// ---------------------------------------------------------------------------
// view.go — 最小可用 View/Init/Update 骨架
//
// 从 waveloom tui.go 移植时未整体复制 View(其与 plan/rewind/HUD 耦合),
// 这里提供等价的布局入口,复用已移植的 buildViewportContent 段落渲染。
// 视觉细节(HUD/logo/header)在阶段 4 对齐。
// ---------------------------------------------------------------------------

// Init 初始化组件与命令。
func (m *model) Init() tea.Cmd {
	input := textarea.New()
	input.Placeholder = m.msg().InputPlaceholder
	input.CharLimit = 2048
	input.ShowLineNumbers = false
	input.MaxHeight = 2
	input.EndOfBufferCharacter = ' '
	input.SetPromptFunc(2, func(_ textarea.PromptInfo) string {
		return "  "
	})
	input.SetHeight(2)
	input.SetWidth(0)
	input.SetVirtualCursor(false) // real cursor 避免 virtual cursor 反色 ANSI 泄漏
	input.Focus()
	m.input = input

	// spinners(HUD 与段落流式前缀)
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(colorOK)
	m.spinner = sp

	spAsst := spinner.New()
	spAsst.Spinner = spinner.MiniDot
	spAsst.Style = lipgloss.NewStyle().Foreground(colorOK)
	m.spAsst = spAsst

	spThought := spinner.New()
	spThought.Spinner = spinner.Points
	spThought.Style = lipgloss.NewStyle().Foreground(colorMuted)
	m.spThought = spThought

	spTool := spinner.New()
	spTool.Spinner = spinner.Line
	spTool.Style = lipgloss.NewStyle().Foreground(colorMuted)
	m.spTool = spTool

	spSubagent := spinner.New()
	spSubagent.Spinner = spinner.Jump
	spSubagent.Style = lipgloss.NewStyle().Foreground(colorMuted)
	m.spSubagent = spSubagent

	spTodo := spinner.New()
	spTodo.Spinner = spinner.Dot
	spTodo.Style = lipgloss.NewStyle().Foreground(colorAccentGold)
	m.spTodo = spTodo

	// ctx 进度条(与 waveloom 对齐:全块字符 + 按压力比例着色 + 无百分比)
	m.ctxProgress = progress.New(
		progress.WithFillCharacters('█', '░'),
		progress.WithColorFunc(func(total, current float64) color.Color {
			if total < 0.5 {
				return colorOK
			}
			if total < 0.8 {
				return colorAccentGold
			}
			return colorErr
		}),
		progress.WithoutPercentage(),
	)
	m.ctxProgress.SetWidth(20)

	// 统一主题组件(输入框样式/spinner/glamour 一次到位)
	m.syncThemeComponents()
	m.pinnedToBottom = true

	return tea.Batch(textarea.Blink,
		sp.Tick, spAsst.Tick, spThought.Tick, spTool.Tick, spSubagent.Tick, spTodo.Tick,
		func() tea.Msg { return tea.RequestBackgroundColor() },
		m.bgProbeTick(),
	)
}

// bgProbeMsg 周期背景色轮询触发。
type bgProbeMsg struct{}

// bgProbeTick 返回下一个背景色轮询命令(5s 周期;仅 auto 模式发查询)。
func (m *model) bgProbeTick() tea.Cmd {
	return tea.Tick(5*time.Second, func(time.Time) tea.Msg { return bgProbeMsg{} })
}

// Update 分发消息。阶段 1 处理:窗口尺寸、输入编辑、滚动、发送/取消/退出。
func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		contentWidth := max(msg.Width-4, 20)
		m.input.SetWidth(contentWidth)
		// 重建 Glamour renderer 以适配新宽度(waveloom 同款)
		m.rebuildGlamour(max(msg.Width-6, 20))
		m.scrollToBottom()
		return m, nil

	case tea.MouseMsg:
		// 滚轮仅滚动页面内容(与 waveloom 对齐);
		// 文本选择请使用 Shift+点击(终端标准惯例)。
		mouse := msg.Mouse()
		switch mouse.Button {
		case tea.MouseWheelUp:
			m.scrollUp(3)
		case tea.MouseWheelDown:
			m.scrollDown(3)
		}
		return m, nil

	case tea.PasteMsg:
		// bracketed paste 粘贴文本 → 转发给输入框(textarea 自带 PasteMsg 支持);
		// 焦点模式/覆盖层打开时忽略。
		if m.focusIndex >= 0 || m.overlay != overlayNone {
			return m, nil
		}
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		cmds = append(cmds, cmd)
		return m, tea.Batch(cmds...)

	case tea.KeyPressMsg:
		// 会话列表覆盖层按键优先
		if m.sessionListVisible() {
			if handled, _ := m.handleSessionListKey(msg); handled {
				return m, nil
			}
		}
		// 阻断式覆盖层优先消费按键(审批/提问)
		switch m.overlay {
		case overlayPermission:
			if handled, _ := m.handleApprovalKey(msg); handled {
				return m, nil
			}
		case overlayQuestion:
			if handled, _ := m.handleQuestionKey(msg); handled {
				return m, nil
			}
		case overlayThemePicker:
			if handled, cmd := m.handleThemePickerKey(msg); handled {
				return m, cmd
			}
		case overlayModelPicker:
			if handled, cmd := m.handleModelPickerKey(msg); handled {
				return m, cmd
			}
		}
		switch msg.String() {
		case "ctrl+c":
			// 双击退出;单次进入等待态(阶段 1:直接退出)
			return m, tea.Quit
		case "tab":
			// 段落焦点模式:移到下一个可展开段落(运行中/覆盖层打开时禁用)
			if m.running || m.overlay != overlayNone {
				return m, nil
			}
			m.focusNext()
			return m, nil
		case "shift+tab":
			if m.running || m.overlay != overlayNone {
				return m, nil
			}
			m.focusPrev()
			return m, nil
		case "ctrl+s":
			// 兼容入口(部分终端 Ctrl+S 被 XOFF 流控截获,主入口是空闲 ←)
			m.toggleSessionList()
			return m, nil
		case "left":
			// 空闲态 ← 打开会话列表(输入框为空时;有内容时仍是光标移动)
			if !m.running && m.overlay == overlayNone && m.input.Value() == "" {
				m.toggleSessionList()
				return m, nil
			}
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)
			cmds = append(cmds, cmd)
			return m, tea.Batch(cmds...)
		case "ctrl+g":
			m.toggleThemePicker()
			return m, nil
		case "ctrl+m":
			m.toggleModelPicker()
			return m, nil
		case "enter", "ctrl+enter":
			// 焦点在段落上 → 展开/折叠
			if m.focusIndex >= 0 && m.focusIndex < len(m.paras) {
				m.toggleParagraphFocus()
				return m, nil
			}
			// 发送消息。mode 与 dsh web 对齐(ComposerSubmissionPolicy):
			// 空闲 → queue;繁忙 → 宿主 busyEnter 配置;Ctrl+Enter → 反转。
			text := strings.TrimSpace(m.input.Value())
			if text == "" {
				return m, nil
			}
			// 输入 exit 直接退出(对齐 waveloom,不区分大小写)
			if strings.EqualFold(text, "exit") {
				return m, tea.Quit
			}
			mode := dsh.PromptModeQueue
			if m.running {
				mode = m.busyEnter
				if mode == "" {
					mode = dsh.PromptModeQueue
				}
				if msg.String() == "ctrl+enter" {
					if mode == dsh.PromptModeQueue {
						mode = dsh.PromptModeSteer
					} else {
						mode = dsh.PromptModeQueue
					}
				}
			}
			if m.onSend != nil {
				m.onSend(text, mode)
			}
			m.saveToHistory(text)
			m.input.Reset()
			return m, nil
		case "esc":
			// 焦点在段落上 → 返回输入框
			if m.focusIndex >= 0 {
				m.focusIndex = -1
				cmd := m.exitFocusMode()
				return m, cmd
			}
			if m.running && m.onCancel != nil {
				m.onCancel()
				return m, nil
			}
			// 空闲态:双击 Esc(500ms 内)清空输入框
			m.handleEscInIdle()
			return m, nil
		case "up":
			// 焦点在段落上 → 上一个可展开段落
			if m.focusIndex >= 0 {
				m.focusPrev()
				return m, nil
			}
			// 空闲态 ↑ → 输入历史导航(未处于导航中或还有更早记录时尝试)
			if !m.running && m.overlay == overlayNone {
				if m.historyPos == -1 || m.historyPos < len(m.inputHistory)-1 {
					if m.navigateHistoryUp() {
						return m, nil
					}
				}
			}
			m.scrollUp(1)
			return m, nil
		case "down":
			if m.focusIndex >= 0 {
				m.focusNext()
				return m, nil
			}
			// 空闲态 ↓ → 输入历史导航(仅导航中有效)
			if !m.running && m.overlay == overlayNone {
				if m.navigateHistoryDown() {
					return m, nil
				}
			}
			m.scrollDown(1)
			return m, nil
		case "pgup":
			m.scrollUp(m.bodyHeight)
			return m, nil
		case "pgdown":
			m.scrollDown(m.bodyHeight)
			return m, nil
		case "ctrl+e", "end":
			m.scrollToBottom()
			return m, nil
		}
		// 其余按键交给输入框
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		cmds = append(cmds, cmd)
		return m, tea.Batch(cmds...)

	case bgProbeMsg:
		// 周期轮询终端背景色(auto 主题跟随终端配色切换;
		// OSC 11 是查询-响应模式,不轮询就无法感知运行中变化)
		if m.themeMode == "auto" {
			cmds = append(cmds, func() tea.Msg { return tea.RequestBackgroundColor() })
		}
		cmds = append(cmds, m.bgProbeTick())
		return m, tea.Batch(cmds...)

	case tea.BackgroundColorMsg:
		m.autoDarkFromTea = msg.IsDark()
		m.hasTeaBackground = true
		if m.themeMode == "auto" {
			m.reapplyAutoTheme()
		}
		return m, nil

	case spinner.TickMsg:
		// Spinner 帧动画(全部 6 个 spinner 统一路由,与 waveloom 对齐)。
		// bubbles v2 的 Tick 是一次性命令:必须在此消费 TickMsg 并把各 spinner
		// 返回的下一帧 cmd 重新投递,动画才会持续推进。
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		cmds = append(cmds, cmd)

		m.spAsst, cmd = m.spAsst.Update(msg)
		cmds = append(cmds, cmd)

		m.spThought, cmd = m.spThought.Update(msg)
		cmds = append(cmds, cmd)

		m.spTool, cmd = m.spTool.Update(msg)
		cmds = append(cmds, cmd)

		m.spSubagent, cmd = m.spSubagent.Update(msg)
		cmds = append(cmds, cmd)

		m.spTodo, cmd = m.spTodo.Update(msg)
		cmds = append(cmds, cmd)

		if m.updating {
			m.updateTick++
		}

		return m, tea.Batch(cmds...)

	case DshFrameMsg:
		if m.projector != nil {
			m.projector.ReplayFrame(msg.Frame)
		}
		// 新下行帧(流式增量/tool 事件等)到达:用户不在底部时显示跳回提示
		m.markNewContent()
		return m, nil

	case dshPromptDoneMsg:
		if msg.err != nil {
			m.appendSystem("send failed: "+msg.err.Error(), notifError)
		}
		return m, nil

	case dshCancelDoneMsg:
		if msg.err != nil {
			m.appendSystem("cancel failed: "+msg.err.Error(), notifError)
		}
		return m, nil

	case approvalRespondErrMsg:
		m.appendSystem("respond failed: "+msg.err.Error(), notifError)
		return m, nil

	case dshSwitchDoneMsg:
		if msg.err != nil {
			m.appendSystem("session switch failed: "+msg.err.Error(), notifError)
			return m, nil
		}
		// 清空旧会话渲染 → 重置投影器 → 回放新会话历史
		m.paras = nil
		m.todos = nil
		m.focusIndex = -1 // 段落/焦点状态随会话重置
		m.pinnedToBottom = true
		if m.projector != nil {
			m.projector.Reset()
			m.projector.SessionID = msg.target
		}
		m.sessionID = msg.target
		// 同步新会话运行状态(从会话列表快照;Esc 中断可用性依赖它)
		running := false
		for _, s := range m.sessions {
			if s.SessionID == msg.target {
				running = s.Running
				break
			}
		}
		m.SetRunning(running)
		if msg.hist != nil {
			events := make([]HistoryEvent, 0, len(msg.hist.Events))
			for _, h := range msg.hist.Events {
				events = append(events, HistoryEvent{Event: h.Event, View: json.RawMessage(h.View)})
			}
			m.ReplayHistory(events)
		}
		m.appendSystem(fmt.Sprintf("session %s", msg.target), notifInfo)
		m.scrollToBottom()
		return m, nil

	case modelsLoadedMsg:
		if msg.err != nil {
			m.appendSystem("model list failed: "+msg.err.Error(), notifError)
		} else if len(msg.models) > 0 {
			m.modelPickerItems = msg.models
			if m.overlay == overlayModelPicker {
				m.buildModelPickerList()
			}
		}
		return m, nil

	case modelSelectedMsg:
		m.hudModel = msg.model
		if msg.effort != "" {
			m.hudThinkingEffort = msg.effort // HUD "(effort ...)" 跟随宿主确认值
		}
		return m, nil

	case gapEventsMsg:
		if msg.err != nil {
			m.appendSystem("gap replay failed: "+msg.err.Error(), notifWarn)
		} else if len(msg.events) > 0 {
			m.ReplayHistory(msg.events)
			// 补洞追加的新内容同样触发跳回提示
			m.markNewContent()
		}
		return m, nil
	}

	return m, nil
}

// appendSystem 追加系统通知段落(供 Update 错误路径使用)。
func (m *model) appendSystem(text string, kind systemNotifKind) {
	m.paras = append(m.paras, Paragraph{
		Type:      paraSystem,
		State:     stateDone,
		Text:      text,
		NotifKind: kind,
	})
	m.paras[len(m.paras)-1].renderDirty = true
	if m.pinnedToBottom {
		m.scrollToBottom()
	}
}

// rebuildGlamour 按当前主题与宽度重建 glamour 渲染器(waveloom 同款参数)。
func (m *model) rebuildGlamour(wordWrap int) {
	pal := m.palette
	if pal == (palette{}) {
		pal = paletteFor(m.themeMode)
	}
	if m.glamourRenderer != nil {
		_ = m.glamourRenderer.Close()
	}
	renderer, err := glamour.NewTermRenderer(
		glamour.WithWordWrap(wordWrap),
		glamour.WithStyles(waveloomGlamourStyle(pal)),
		glamour.WithEmoji(),
		glamour.WithChromaFormatter("terminal16m"),
	)
	if err == nil {
		m.glamourRenderer = renderer
	}
}

// ---------------------------------------------------------------------------
// dsh 事件消息(程序 → model)
// ---------------------------------------------------------------------------

// DshFrameMsg 携带一条下行帧(main 包的 downlink goroutine 投递)。
type DshFrameMsg struct {
	Frame dsh.ServerRequest
}

// dshPromptDoneMsg prompt 调用完成。
type dshPromptDoneMsg struct {
	err error
}

// dshCancelDoneMsg cancel 调用完成。
type dshCancelDoneMsg struct {
	err error
}

// dshSwitchDoneMsg 会话切换完成(切换/新建共用)。
type dshSwitchDoneMsg struct {
	target string
	hist   *dsh.SessionHistoryValue
	err    error
}

// modelsLoadedMsg 模型目录加载完成。
type modelsLoadedMsg struct {
	models []ModelChoice
	err    error
}

// modelSelectedMsg 模型切换成功(带宿主确认的 effort)。
type modelSelectedMsg struct {
	model  string
	effort string
}

// gapEventsMsg 断线重连后的补洞历史。
type gapEventsMsg struct {
	events []HistoryEvent
	err    error
}

// keyMatches 兼容 waveloom 按键匹配(供后续阶段使用)。
func keyMatches(msg tea.KeyPressMsg, binding key.Binding) bool {
	return key.Matches(msg, binding)
}
