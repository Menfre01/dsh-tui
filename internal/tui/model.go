package tui

import (
	"context"
	"time"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/progress"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/glamour/v2"

	"github.com/Menfre01/dsh-tui/internal/dsh"
)

// ---------------------------------------------------------------------------
// 快捷键
// ---------------------------------------------------------------------------

// keyMap 定义所有快捷键绑定。
type keyMap struct {
	Enter       key.Binding
	Interrupt   key.Binding
	Quit        key.Binding
	FocusNext   key.Binding
	FocusPrev   key.Binding
	Up          key.Binding
	Down        key.Binding
	PageUp      key.Binding
	PageDown    key.Binding
	ToggleTheme key.Binding
	JumpBottom  key.Binding
	Picker      key.Binding
	Paste       key.Binding
	Help        key.Binding
}

var defaultKeys = keyMap{
	Enter:       key.NewBinding(key.WithKeys("enter"), key.WithHelp("⏎", "Send message")),
	Interrupt:   key.NewBinding(key.WithKeys("esc"), key.WithHelp("Esc", "Interrupt agent loop")),
	Quit:        key.NewBinding(key.WithKeys("ctrl+c"), key.WithHelp("Ctrl+C×2", "Double-tap to quit")),
	FocusNext:   key.NewBinding(key.WithKeys("tab"), key.WithHelp("Tab", "Focus next interactive paragraph")),
	FocusPrev:   key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("Shift+Tab", "Focus previous interactive paragraph")),
	Up:          key.NewBinding(key.WithKeys("up"), key.WithHelp("↑", "Scroll up")),
	Down:        key.NewBinding(key.WithKeys("down"), key.WithHelp("↓", "Scroll down")),
	PageUp:      key.NewBinding(key.WithKeys("pgup"), key.WithHelp("PgUp", "Page up")),
	PageDown:    key.NewBinding(key.WithKeys("pgdown"), key.WithHelp("PgDn", "Page down")),
	ToggleTheme: key.NewBinding(key.WithKeys("ctrl+g"), key.WithHelp("Ctrl+G", "Toggle theme (dark/light/auto)")),
	JumpBottom:  key.NewBinding(key.WithKeys("ctrl+e", "end"), key.WithHelp("Ctrl+E/End", "Jump to bottom")),
	Picker:      key.NewBinding(key.WithKeys("@"), key.WithHelp("@", "Pick file/directory")),
	Paste:       key.NewBinding(key.WithKeys("ctrl+v"), key.WithHelp("Ctrl+V", "Paste")),
	Help:        key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "Shortcuts")),
}

// makeKeyMap 根据 locale 生成带翻译帮助文本的 keyMap。
func makeKeyMap(lc *Messages) keyMap {
	return keyMap{
		Enter:       key.NewBinding(key.WithKeys("enter"), key.WithHelp("⏎", lc.KeySend)),
		Interrupt:   key.NewBinding(key.WithKeys("esc"), key.WithHelp("Esc", lc.KeyInterrupt)),
		Quit:        key.NewBinding(key.WithKeys("ctrl+c"), key.WithHelp("Ctrl+C", lc.KeyQuit)),
		FocusNext:   key.NewBinding(key.WithKeys("tab"), key.WithHelp("Tab", lc.KeyFocusNext)),
		FocusPrev:   key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("Shift+Tab", lc.KeyFocusPrev)),
		Up:          key.NewBinding(key.WithKeys("up"), key.WithHelp("↑", lc.KeyScrollUp)),
		Down:        key.NewBinding(key.WithKeys("down"), key.WithHelp("↓", lc.KeyScrollDown)),
		PageUp:      key.NewBinding(key.WithKeys("pgup"), key.WithHelp("PgUp", lc.KeyPageUp)),
		PageDown:    key.NewBinding(key.WithKeys("pgdown"), key.WithHelp("PgDn", lc.KeyPageDown)),
		ToggleTheme: key.NewBinding(key.WithKeys("ctrl+g"), key.WithHelp("Ctrl+G", lc.KeyToggleTheme)),
		JumpBottom:  key.NewBinding(key.WithKeys("ctrl+e", "end"), key.WithHelp("Ctrl+E/End", lc.KeyJumpBottom)),
		Picker:      key.NewBinding(key.WithKeys("@"), key.WithHelp("@", lc.KeyPicker)),
		Paste:       key.NewBinding(key.WithKeys("ctrl+v"), key.WithHelp("Ctrl+V", lc.KeyPaste)),
		Help:        key.NewBinding(key.WithKeys("?"), key.WithHelp("?", lc.KeyHelp)),
	}
}

// ---------------------------------------------------------------------------
// rewindMsg — rewind 消息选择器中可回退的一条用户消息
// ---------------------------------------------------------------------------

type rewindMsg struct {
	MessageID    string // 消息 UUID
	Content      string // 消息文本摘要
	TimeAgo      string // 相对时间描述
	FilesChanged int    // 该消息变更的文件数
	FileSummary  string // 文件变更摘要
}

// ---------------------------------------------------------------------------
// model — 渲染层状态(裁剪版)
//
// 从 waveloom cmd/waveloom/tui.go 移植的渲染字段子集;引擎字段
// (llmClient/registry/guard/sandbox 等)已删除。dsh-tui 的事件投影层
// (project.go)负责把 dsh wire 帧转成 paras 等字段。
// ---------------------------------------------------------------------------

type model struct {
	// 国际化
	lc *Messages // 当前语言的文案实例(nil 时回退到 enUS)

	// 主题色板(applyTheme 后生效,glamour/spinner 同步用)
	palette palette

	// auto 模式背景检测(与 waveloom 一致)
	autoDark        bool // 启动时 lipgloss 查询结果
	autoDarkFromTea bool // BackgroundColorMsg 结果(true = 深色)
	hasTeaBackground bool // 是否已收到 BackgroundColorMsg

	// 段落模型(消息内容的数据源)
	paras []Paragraph

	// dsh 会话状态投影(project.go 维护)
	todos      []TodoItem          // todo/write 快照
	queueItems []dsh.QueuedInboxItem // session/queue 快照
	jobCount   int                  // session/jobs 快照

	// Glamour markdown 渲染器
	glamourRenderer *glamour.TermRenderer

	// 布局与滚动
	width          int
	height         int
	bodyHeight     int
	scrollTop      int
	pinnedToBottom bool
	hasNewContent  bool

	// 覆盖层状态
	overlay          Overlay
	overlayAnimFrame int // 0=刚弹出, 1=过渡, 2=完成

	permDelegate     *list.DefaultDelegate

	// 主题选择器覆盖层
	themeMode     string
	themeList     list.Model
	themeDelegate *list.DefaultDelegate

	// 模型选择器覆盖层
	modelPickerList     list.Model
	modelPickerItems    []ModelChoice
	effortPickerMode    bool // effort 面板模式(e 键进入)
	effortPickerList    list.Model
	effortPickerModelIdx int // effort 面板对应的模型索引
	onSelectModel       func(provider, model, effort string) // main 注入:selectModel
	onFetchModels       func()                        // main 注入:异步拉取模型目录

	// 语言选择器覆盖层
	localeList     list.Model
	localeDelegate *list.DefaultDelegate

	// Provider 选择器覆盖层
	providerPickerList     list.Model
	providerPickerDelegate *list.DefaultDelegate
	providerPickerItems    []providerPickerItemData

	// 文件选择器
	pickerVisible         bool
	pickerFilter          string
	pickerItems           []pickerItem
	pickerAllItems        []pickerItem
	pickerScanGen         int
	pickerScanCancel      context.CancelFunc
	pickerLastScannedBase string
	pickerDismissValue    string
	pickerLastValue       string
	pickerScanning        bool
	pickerList            list.Model
	pickerDelegate        *list.DefaultDelegate

	// 命令选择器(/ 触发)
	commandPickerVisible      bool
	commandPickerList         list.Model
	commandPickerDelegate     *list.DefaultDelegate
	commandPickerItems        []CommandInfo
	commandPickerFilter       string
	commandPickerDismissValue string
	commandPickerLastValue    string

	// HUD 会话级累积(footer 显示用)
	hudModel          string
	hudThinkingEffort string // thinking 档位,空表示关闭
	hudPromptTokens   int
	hudComplTokens    int
	hudTurns          int
	hudMessages       int
	hudCacheHit       int
	hudCacheMiss      int
	hudLatMs          int64


	// rewind 状态(预留;dsh 无 rewind 能力)
	rewindMessages    []rewindMsg
	rewindScrollOffset int
	rewindSelectedIdx  int
	rewindTargetMsgID  string
	rewindMaxVisible   int

	// Bubbletea 组件
	program    *tea.Program
	keys       keyMap
	help       help.Model
	spinner    spinner.Model // 通用 spinner(HUD 加载指示)
	focusIndex int           // 段落焦点:-1 = 输入框,>=0 = 段落索引
	spAsst     spinner.Model // assistant 流式前缀动画
	spThought  spinner.Model // thought 流式前缀动画
	spTool     spinner.Model // tool 执行中前缀动画
	spSubagent spinner.Model // subagent 执行中前缀动画(独立视觉)
	spTodo     spinner.Model // todo in_progress 前缀动画
	ctxProgress progress.Model

	input               textarea.Model
	otherInput          textinput.Model
	otherInputLastValue string

	// 输入历史(↑↓ 导航,对齐 waveloom)
	inputHistory []string // 已提交的输入,最新在前
	historyPos   int      // 当前历史位置(-1 = 不在历史导航中)
	historyDraft string   // 进入历史导航前的草稿
	lastEscTime  time.Time // 上次空闲态按 Esc 的时间(双击清空输入框)

	// 工作目录(会话 cwd,用于 @ 文件选择器)
	cwd string

	// 会话标识(header 显示,main 注入)
	sessionID    string
	sessionTitle string // 宿主 title 投影(header 优先显示)

	// 布局/模式状态(复刻 waveloom View 所需)
	inPlanMode     bool // plan 模式标记(预留;dsh 无 plan 语义)
	todoExpanded   bool // todo 面板展开
	todoFocused    bool // todo 面板聚焦
	noticeBanner   string // 版本更新提示(预留)
	updating       bool
	updateTick     int
	lastPromptTokens int  // ctx bar 实时值(压力)
	projectedTokens  int  // ctx bar 优先值(web 语义:projectedTokens ?? pressureTokens)
	contextLimit   int    // 上下文窗口 token 上限
	turnStartTime  time.Time // 本轮启动时间(延迟计算)


	// dsh 连接回调(main 包注入)
	running   bool // 会话是否运行中(agent busy)
	onSend    func(text, mode string) // mode: queue | steer
	// busyEnter 是宿主 ui-conversation 配置:繁忙时 Enter 的发送模式
	// (queue=入队,steer=直接干预),默认 queue,与 dsh web 对齐。
	busyEnter string
	onCancel  func()
	responder Responder // /api/respond 应答器(main 注入)

	// 审批/提问覆盖层(dsh approval/requested、question/requested 驱动)
	pendingApproval  *PendingApproval
	pendingQuestion  *PendingQuestion

	// 事件投影器(main 包创建后注入)
	projector *Projector

	// 会话管理(阶段 3)
	sessions       []SessionBrief // 会话列表(main 拉取 + host/ 帧增量)
	sessionListIdx int            // 会话列表选中索引
	onSwitchSession func(sessionID string) // main 注入:切换会话
	onNewSession    func()                // main 注入:新建会话
}

// SessionBrief 是会话列表条目。
type SessionBrief struct {
	SessionID string
	Running   bool
	Blank     bool
	Cwd       string
	AgentPreset string
}

// msg 返回当前语言的 Messages 实例,nil 时回退 enUS。
func (m *model) msg() *Messages {
	if m.lc != nil {
		return m.lc
	}
	return &enUS
}

// closeCommandPicker 关闭命令选择器。
func (m *model) closeCommandPicker() {
	m.commandPickerVisible = false
	m.commandPickerDismissValue = m.input.Value()
	m.commandPickerLastValue = ""
	m.commandPickerItems = nil
}

// ---------------------------------------------------------------------------
// 滚动控制
// ---------------------------------------------------------------------------

// scrollUp 向上滚动 delta 行(查看更早的内容)。
func (m *model) scrollUp(delta int) {
	if delta <= 0 {
		return
	}
	m.pinnedToBottom = false
	m.scrollTop -= delta
	if m.scrollTop < 0 {
		m.scrollTop = 0
	}
}

// scrollDown 向下滚动 delta 行(查看更新的内容)。
func (m *model) scrollDown(delta int) {
	if delta <= 0 {
		return
	}
	m.scrollTop += delta
}

// scrollToBottom 滚动到底部并锁定自动跟随。
func (m *model) scrollToBottom() {
	m.pinnedToBottom = true
	m.hasNewContent = false
	// scrollTop 由 View() 根据 maxScrollTop 自动计算
}

// markNewContent 在用户向上滚动查看历史且有新内容到达时设置提示标记。
func (m *model) markNewContent() {
	if !m.pinnedToBottom {
		m.hasNewContent = true
	}
}
