package tui

import (
	"encoding/json"
	"os"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/Menfre01/dsh-tui/internal/dsh"
)

// ---------------------------------------------------------------------------
// app.go — tui 包的导出入口(main 包使用)
// ---------------------------------------------------------------------------

// ModelConfig 是 TUI model 的创建配置。
type ModelConfig struct {
	CWD   string
	Theme string // "dark" | "light" | "auto" | "darkcolorblind" | "lightcolorblind"
	LC    *Messages
}

// NewModel 创建 TUI model(main 包调用)。
func NewModel(cfg ModelConfig) *model {
	m := &model{
		lc:             cfg.LC,
		themeMode:      cfg.Theme,
		cwd:            cfg.CWD,
		keys:           defaultKeys,
		pinnedToBottom: true,
		focusIndex:     -1, // -1 = 输入框聚焦(与 waveloom 一致)
	}
	if m.themeMode == "" {
		m.themeMode = "dark"
	}
	if m.lc == nil {
		m.lc = &enUS
	}
	// 主题初始化:注入全局颜色/样式变量(缺失时整个 TUI 是黑白的)
	m.autoDark = lipgloss.HasDarkBackground(os.Stdin, os.Stdout)
	m.palette = paletteFor(m.themeMode)
	if m.themeMode == "auto" {
		if m.autoDark {
			m.palette = darkPalette
		} else {
			m.palette = lightPalette
		}
	}
	applyTheme(m.palette)
	return m
}

// AttachProjector 注入事件投影器。
func (m *model) AttachProjector(p *Projector) { m.projector = p }

// SetCallbacks 注入发送/取消回调。
func (m *model) SetCallbacks(onSend func(text, mode string), onCancel func()) {
	m.onSend = onSend
	m.onCancel = onCancel
}

// SetResponder 注入审批/提问应答器。
func (m *model) SetResponder(r Responder) { m.responder = r }

// SetGapCallback 注入断线补洞回调(经投影器转发)。
func (m *model) SetGapCallback(fn func(sessionID string, fromSeq int64)) {
	if m.projector != nil {
		m.projector.SetGapCallback(fn)
	}
}

// SendGapEvents 投递补洞历史(追加回放,不清空现有渲染)。
func (m *model) SendGapEvents(events []HistoryEvent, err error) {
	if m.program != nil {
		m.program.Send(gapEventsMsg{events: events, err: err})
	}
}

// SetSessions 注入会话列表(main 启动时拉取,host/ 帧增量后全量替换)。
// 按 SessionID 去重:即使上游 session.list 本身重复,列表也保持唯一。
func (m *model) SetSessions(sessions []SessionBrief) {
	seen := make(map[string]bool, len(sessions))
	uniq := make([]SessionBrief, 0, len(sessions))
	for _, s := range sessions {
		if s.SessionID == "" || seen[s.SessionID] {
			continue
		}
		seen[s.SessionID] = true
		uniq = append(uniq, s)
	}
	m.sessions = uniq
	if m.sessionListIdx >= len(m.sessions) {
		m.sessionListIdx = max(len(m.sessions)-1, 0)
	}
}

// SetSessionCallbacks 注入会话切换/新建回调。
func (m *model) SetSessionCallbacks(onSwitch func(sessionID string), onNew func()) {
	m.onSwitchSession = onSwitch
	m.onNewSession = onNew
}

// SetRunning 更新运行状态(HUD 显示与 Enter 键可用性)。
func (m *model) SetRunning(running bool) {
	m.running = running
	if m.projector != nil {
		m.projector.SetRunning(running)
	}
}

// ReplayHistory 回放一段历史事件(会话打开/重连补洞共用)。
func (m *model) ReplayHistory(events []HistoryEvent) {
	if m.projector == nil {
		return
	}
	for _, ev := range events {
		if len(ev.View) > 0 {
			m.projector.ReplayEventWithView(&ev.Event, ev.View)
		} else {
			m.projector.ReplayEvent(&ev.Event)
		}
	}
	m.scrollToBottom()
}

// HistoryEvent 是 tui 层的历史事件视图,由 main 包从 dsh.HistoryEntry 转换。
// View 是宿主随事件附带的渲染意图(tool/call、tool/result 的 view 信封)。
type HistoryEvent struct {
	Event dsh.SessionEvent
	View  json.RawMessage
}

// SetSessionInfo 设置 HUD 的会话标识。
func (m *model) SetSessionInfo(sessionID, model string) {
	m.sessionID = sessionID
	if model != "" {
		m.hudModel = model
	}
	if m.projector != nil {
		m.projector.SessionID = sessionID
		m.projector.Model = model
	}
}

// SetProjections 应用宿主投影初始值(session.list 的 projections.values;
// 实时更新走 session/projection 帧)。
func (m *model) SetProjections(values map[string]json.RawMessage) {
	if m.projector == nil || len(values) == 0 {
		return
	}
	for k, v := range values {
		m.projector.onSessionProjection(k, v)
	}
}

// SetThinkingEffort 设置当前思考档位(session.models 的 Current.ReasoningEffort,
// HUD 显示 "(think <effort>)")。
func (m *model) SetThinkingEffort(v string) {
	m.hudThinkingEffort = v
}

// SetBusyEnter 采用宿主 ui-conversation.busyEnter 配置
// (queue=繁忙时 Enter 入队,steer=直接干预);非法值保持默认。
func (m *model) SetBusyEnter(v string) {
	if v == "queue" || v == "steer" {
		m.busyEnter = v
	}
}

// SendFrame 把一条下行帧投递到 TUI 事件循环(从 downlink goroutine 调用)。
func (m *model) SendFrame(frame dsh.ServerRequest) {
	if m.program != nil {
		m.program.Send(DshFrameMsg{Frame: frame})
	}
}

// Program 暴露 tea.Program,供 main 包启动。
func (m *model) Program() *tea.Program { return m.program }

// SetProgram 注入 tea.Program(main 包在创建 program 后调用)。
func (m *model) SetProgram(p *tea.Program) { m.program = p }

// SendDone 投递 prompt 完成消息(downlink goroutine 调用)。
func (m *model) SendDone(err error) {
	if m.program != nil {
		m.program.Send(dshPromptDoneMsg{err: err})
	}
}

// SendCancelDone 投递 cancel 完成消息。
func (m *model) SendCancelDone(err error) {
	if m.program != nil {
		m.program.Send(dshCancelDoneMsg{err: err})
	}
}

// MessagesZhCN 返回简体中文文案实例。
func MessagesZhCN() *Messages { return &zhCN }

// MessagesEnUS 返回英文文案实例。
func MessagesEnUS() *Messages { return &enUS }

// SendSwitchDone 投递会话切换完成消息(切换/新建会话共用)。
func (m *model) SendSwitchDone(target string, hist *dsh.SessionHistoryValue, err error) {
	if m.program != nil {
		m.program.Send(dshSwitchDoneMsg{target: target, hist: hist, err: err})
	}
}

// SetFetchModelsCallback 注入模型目录拉取回调。
func (m *model) SetFetchModelsCallback(fn func()) { m.onFetchModels = fn }

// SendModelsLoaded 投递模型目录加载结果。
func (m *model) SendModelsLoaded(models []ModelChoice, err error) {
	if m.program != nil {
		m.program.Send(modelsLoadedMsg{models: models, err: err})
	}
}

// SendModelSelected 投递模型切换成功(带宿主确认的 effort)。
func (m *model) SendModelSelected(model, effort string) {
	if m.program != nil {
		m.program.Send(modelSelectedMsg{model: model, effort: effort})
	}
}

// Dump 无头渲染一次完整界面(测试/CI 用,不依赖 TTY)。
func (m *model) Dump(width, height int) string {
	_ = m.Init()
	// 走完整 WindowSizeMsg 布局路径,与真实终端一致
	_, _ = m.Update(tea.WindowSizeMsg{Width: width, Height: height})
	return m.View().Content
}
