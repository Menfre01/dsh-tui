package tui

import (
	"os"
	"strings"
)

// ---------------------------------------------------------------------------
// Locale 类型
// ---------------------------------------------------------------------------

// Locale 表示界面语言。
type Locale string

const (
	LocaleZhCN Locale = "zh-CN"
	LocaleEnUS Locale = "en-US"
)

// ---------------------------------------------------------------------------
// Messages — 所有可翻译的 TUI 文案
// ---------------------------------------------------------------------------

// Messages 聚合所有 TUI 界面文案。通过 Locale 索引获取对应语言的实例。
// 命名规范：按功能区域分组，用点号连接（如 Input.Placeholder）。
// 带 % 格式化动词的字段保留 Go fmt 占位符，调用方负责 Sprintf。
type Messages struct {
	// ── Input ──────────────────────────────────────────────
	InputPlaceholder          string
	InputOtherPlaceholder     string
	InputAgentRunning         string
	InputFocusModePlaceholder string
	InputPlanModePlaceholder  string

	// ── Welcome ────────────────────────────────────────────
	WelcomeGuide     string // 空状态时 body 区域的首次引导面板（多行）
	NewContentHint   string // 向上滚动时新内容到达的提示
	SearchTruncatedHint string // search 结果截断提示(含 %d)
	QueueDockHint    string // 排队消息指示(含 %d)
	TerminalTooSmall string // 终端高度 < 10 行时的提示

	// ── System notifications ──────────────────────────────
	SysCompactionDone    string
	SysContextHardLimit  string
	SysSummaryFailed     string
	SysUnknownCommand    string // 含 %s
	SysCommandFailed     string // 含 %v
	SysNewSessionCreated string
	SysSkillActivated    string // 含 %s
	SysSkillLoadFailed   string // 含 %s, %s

	// ── Loop done ─────────────────────────────────────────
	LoopCompleted   string // 含 %s, %s, %s
	LoopMaxTurns    string // 含 %d, %s, %s, %s
	LoopAborted     string // 含 %s
	LoopToolTimeout string // 含 %s, %s, %s
	LoopModelError  string // 含 %s, %v
	LoopToolFatal   string // 含 %s, %v

	// ── Update ────────────────────────────────────────────

	// ── Thought ───────────────────────────────────────────
	ThoughtThinking     string
	ThoughtComplete     string // 含 %d
	ThoughtExpandHint   string // 含 %d
	ThoughtCollapseHint string

	// ── Tool ──────────────────────────────────────────────
	ToolNQuestions       string // 含 %d
	ToolQuestionDeclined string
	ToolTruncated        string
	ToolTruncatedLines   string // 含 %d
	ToolExpandAllHint    string
	ToolCollapseHint     string

	// ── Permission overlay ───────────────────────────────
	PermRequired string
	PermReason   string
	PermAllow    string
	PermAllowAll string
	PermDeny     string

	// ── Question overlay ─────────────────────────────────
	QuestionOtherOption string
	QuestionOtherPlaceholder string
	KeyBack              string

	// ── Theme / Model picker ─────────────────────────────
	PickerSelectTheme  string
	PickerSelectModel  string
	PickerSelectEffort string
	PickerSelectLocale string
	PickerSelectProvider string
	PickerThemeAuto    string
	PickerSelectSession string // 会话列表标题

	// ── File picker ──────────────────────────────────────
	PickerScanning  string
	PickerNoResults string

	// ── Key bindings ─────────────────────────────────────
	KeyNav         string
	KeyConfirm     string
	KeyDeny        string
	KeyCancel      string
	KeyToggle      string
	KeySend        string
	KeyInterrupt   string
	KeyQuit        string
	KeyFocusNext   string
	KeyFocusPrev   string
	KeyScrollUp    string
	KeyScrollDown  string
	KeyPageUp      string
	KeyPageDown    string
	KeyToggleTheme string
	KeyJumpBottom  string
	KeyPicker      string
	KeyPaste       string
	KeyHelp        string
	KeyHelpTitle   string // ? 帮助 overlay 标题
	KeyHistoryUp   string
	KeyHistoryDown string

	// ── Focus separator ──────────────────────────────────
	FocusSeparatorHint string

	// ── Plan mode ─────────────────────────────────────────
	PlanEnterTitle   string
	PlanEnterDesc1   string
	PlanEnterDesc2   string
	PlanEnterConfirm string
	PlanEnterCancel  string
	PlanExitTitle    string
	PlanExitApprove  string
	PlanExitReject   string

	// ── Header ───────────────────────────────────────────
	HeaderSession string

	// ── Todo panel ────────────────────────────────────────
	TodoTitle        string // 含 %d
	TodoHiddenCount  string // 含 %d
	TodoDoneCount    string // 含 %d
	TodoInProgCount  string // 含 %d
	TodoPendingCount string // 含 %d

	// ── Subagent suffix ──────────────────────────────────
	SubagentTurnsFmt string // 含 %d，如 "%d轮" / "%d turns"

	// ── Rewind overlay ────────────────────────────────────
	RewindTitle            string // "Rewind"
	RewindPrompt           string // "Restore the code and/or conversation to the point before…"
	RewindNothingToRestore string // "Nothing to rewind to yet."
	RewindCurrent          string // "(current)"
	RewindConfirmTitle     string // "Rewind"
	RewindConfirmPrompt    string // "Confirm you want to restore to the point before you sent this message:"
	RewindOptionBoth       string // "Restore code and conversation"
	RewindOptionConv       string // "Restore conversation only"
	RewindOptionCode       string // "Restore code only"
	RewindOptionNeverMind  string // "Never mind"
	RewindWarning          string // "Rewinding does not affect files edited manually or via bash."
	RewindRestoring        string // "Restoring…"
	RewindFailed           string // "Failed to restore: %v"
	RewindNoCodeChanges    string // "No code changes"
	RewindFilesChanged     string // "%d files changed"
	RewindSlashDescription string // "Rewind code and/or conversation to a previous point"
}

// ---------------------------------------------------------------------------
// 语言实例
// ---------------------------------------------------------------------------

var zhCN = Messages{
	// Input
	InputPlaceholder:          "输入消息, ⏎ 发送 · ← 会话 · Esc 中断",
	InputOtherPlaceholder:     "输入自定义答案...",
	InputAgentRunning:         "Agent 执行中... Esc 中断",
	InputFocusModePlaceholder: "段落已聚焦 · ⏎ 展开/折叠 · Esc 回到输入",
	InputPlanModePlaceholder:  "[Plan] 输入消息, ⏎ 发送 · Shift+Tab 退出",

	// Welcome
	WelcomeGuide: "" +
		"欢迎使用 dsh-tui — DeepSeek Harness 终端客户端\n" +
		"\n" +
		"  ←  会话列表 — 切换/新建会话\n" +
		"  Ctrl+G 主题 · Ctrl+M 模型 — 切换主题与模型\n" +
		"  ⏎  发送消息 — 让我编写、重构或调试代码\n" +
		"  Esc  中断 — 停止当前回合\n" +
		"\n" +
		"试试: \"介绍一下这个项目\" 或 \"帮我写个单元测试\"\n" +
		"\n" +
		"开始输入即可对话 —",
	NewContentHint:   "↓ 新内容 (Ctrl+E 跳回底部)",
	SearchTruncatedHint: "… 结果被截断,共 %d 条",
	QueueDockHint:    "⏳ %d 条排队:",
	TerminalTooSmall: "终端窗口太小，请调大后重试（最少 10 行）",

	// System
	SysCompactionDone:    "压缩完成。",
	SysContextHardLimit:  "上下文已满（98%）。/reset 重建。",
	SysSummaryFailed:     "摘要连续失败。/reset 重建。",
	SysNewSessionCreated: "新 session 已创建。",
	SysUnknownCommand:    "未知命令: %s。输入框输入 / 查看可用命令。",
	SysCommandFailed:     "命令执行失败: %v",
	SysSkillActivated:    "已激活 Skills: %s",
	SysSkillLoadFailed:   "Skill 加载失败: %s — %s",

	// Loop
	LoopCompleted:   "完成（%s, ↑%s, ↓%s）",
	LoopMaxTurns:    "已达最大轮次（%d轮, %s, ↑%s, ↓%s）。继续对话。",
	LoopAborted:     "已中断（%s）",
	LoopToolTimeout: "工具执行超时（%s %s）%s",
	LoopModelError:  "Model error (%s, %v)",
	LoopToolFatal:   "Tool error (%s, %v)",

	// Thought
	ThoughtThinking:     "思考中...",
	ThoughtComplete:     "▶ 思考完成 (%d tokens) · ⏎ 展开",
	ThoughtExpandHint:   "··· ⏎ 展开 (%d tokens)",
	ThoughtCollapseHint: "▼ ⏎ 折叠",

	// Tool
	ToolNQuestions:       "(%d 问)",
	ToolQuestionDeclined: "(declined)",
	ToolTruncated:        "··· (truncated)",
	ToolTruncatedLines:   "... (truncated to %d lines)",
	ToolExpandAllHint:    "··· ⏎ 展开",
	ToolCollapseHint:     "▼ ⏎ 折叠",

	// Permission
	PermRequired: "▲ Permission Required",
	PermReason:   "Reason: ",
	PermAllow:    "Allow (本次放行)",
	PermAllowAll: "Always Allow (记住，不再询问)",
	PermDeny:     "Deny (本次拒绝)",

	// Question
	QuestionOtherOption: "Other...",
	QuestionOtherPlaceholder: "输入自定义答案…",
	KeyBack:              "返回",

	// Picker
	PickerSelectTheme:  "▲ 选择主题",
	PickerSelectModel:  "▲ 选择模型",
	PickerSelectEffort: "选择档位 -",
	PickerSelectLocale: "▲ 选择界面语言",
	PickerSelectSession: "选择会话",
	PickerSelectProvider: "▲ 选择 Provider",
	PickerThemeAuto:    "Auto（自动检测终端背景色）",
	PickerScanning:     "正在扫描文件...",
	PickerNoResults:    "无匹配文件",

	// Key bindings
	KeyNav:         "导航",
	KeyConfirm:     "确认",
	KeyDeny:        "拒绝",
	KeyCancel:      "取消",
	KeyToggle:      "勾选",
	KeySend:        "发送消息",
	KeyInterrupt:   "中断 agent loop",
	KeyQuit:        "双击退出",
	KeyFocusNext:   "聚焦下一个可交互段落",
	KeyFocusPrev:   "聚焦上一个可交互段落",
	KeyScrollUp:    "输入历史/向上滚动",
	KeyScrollDown:  "输入历史/向下滚动",
	KeyPageUp:      "向上翻页",
	KeyPageDown:    "向下翻页",
	KeyToggleTheme: "切换主题 (dark/light/auto)",
	KeyJumpBottom:  "跳到底部",
	KeyPicker:      "选择文件/目录",
	KeyPaste:       "粘贴",
	KeyHelp:        "快捷键",
	KeyHelpTitle:   "快捷键帮助",
	KeyHistoryUp:   "向上导航输入历史",
	KeyHistoryDown: "向下导航输入历史",

	// Focus separator
	FocusSeparatorHint: " ◆ 段落已聚焦 · ⏎ 展开/折叠 · Esc 退出 ◆ ",

	// Plan mode
	PlanEnterTitle:   "进入 Plan 模式？",
	PlanEnterDesc1:   "Agent 将探索代码库并设计实现方案，",
	PlanEnterDesc2:   "期间无法编辑源文件，方案完成后需你审批。",
	PlanEnterConfirm: "确认",
	PlanEnterCancel:  "取消",
	PlanExitTitle:    "Plan 审批",
	PlanExitApprove:  "批准",
	PlanExitReject:   "拒绝，继续修改",

	// Header
	HeaderSession: "session: ",

	// Todo panel
	TodoTitle:        "Todo — %d/%d 项",
	TodoHiddenCount:  "%d 项隐藏",
	TodoDoneCount:    "%d 完成",
	TodoInProgCount:  "%d 进行中",
	TodoPendingCount: "%d 等待",

	SubagentTurnsFmt: "%d轮",

	// Rewind
	RewindTitle:            "时间回溯",
	RewindPrompt:           "将代码和/或对话回退到…",
	RewindNothingToRestore: "还没有可回退的内容。",
	RewindCurrent:          "(当前)",
	RewindConfirmTitle:     "时间回溯",
	RewindConfirmPrompt:    "确认要回退到发送这条消息之前吗：",
	RewindOptionBoth:       "回退代码和对话",
	RewindOptionConv:       "仅回退对话",
	RewindOptionCode:       "仅回退代码",
	RewindOptionNeverMind:  "取消",
	RewindWarning:          "回退不会影响手动编辑或通过 bash 修改的文件。",
	RewindRestoring:        "回退中…",
	RewindFailed:           "回退失败: %v",
	RewindNoCodeChanges:    "无代码变更",
	RewindFilesChanged:     "%d 个文件变更",
	RewindSlashDescription: "回退代码和/或对话到之前的某个节点",
}

var enUS = Messages{
	InputPlaceholder:          "Type a message, ⏎ send · ← sessions · Esc interrupt",
	InputOtherPlaceholder:     "Type custom answer...",
	InputAgentRunning:         "Agent running... Esc to interrupt",
	InputFocusModePlaceholder: "Paragraph focused · ⏎ expand/collapse · Esc back to input",
	InputPlanModePlaceholder:  "[Plan] Type a message, ⏎ to send · Shift+Tab to exit",

	// Welcome
	WelcomeGuide: "" +
		"Welcome to dsh-tui — the DeepSeek Harness terminal client\n" +
		"\n" +
		"  ←  Sessions — switch or create sessions\n" +
		"  Ctrl+G theme · Ctrl+M model — switch theme and model\n" +
		"  ⏎  Send message — ask me to write, refactor, or debug code\n" +
		"  Esc  Interrupt — stop the current turn\n" +
		"\n" +
		"Try: \"Explain this project\" or \"Add a unit test for ...\"\n" +
		"\n" +
		"Start typing to begin —",
	NewContentHint:   "↓ New content (Ctrl+E to jump back)",
	SearchTruncatedHint: "… results truncated, %d total",
	QueueDockHint:    "⏳ %d queued:",
	TerminalTooSmall: "Terminal too small, please resize (min 10 rows)",

	// System
	SysCompactionDone:    "Compaction complete.",
	SysContextHardLimit:  "Context full (98%). /reset to rebuild.",
	SysSummaryFailed:     "Summary failed repeatedly. /reset to rebuild.",
	SysNewSessionCreated: "New session created.",
	SysUnknownCommand:    "Unknown command: %s. Type /help to see available commands.",
	SysCommandFailed:     "Command failed: %v",
	SysSkillActivated:    "Skills activated: %s",
	SysSkillLoadFailed:   "Skill load failed: %s — %s",

	// Loop
	LoopCompleted:   "Done (%s, ↑%s, ↓%s)",
	LoopMaxTurns:    "Max turns reached (%d turns, %s, ↑%s, ↓%s). Continue.",
	LoopAborted:     "Aborted (%s)",
	LoopToolTimeout: "Tool timeout (%s %s) %s",
	LoopModelError:  "Model error (%s, %v)",
	LoopToolFatal:   "Tool error (%s, %v)",

	// Thought
	ThoughtThinking:     "Thinking...",
	ThoughtComplete:     "▶ Thinking done (%d tokens) · ⏎ to expand",
	ThoughtExpandHint:   "··· ⏎ to expand (%d tokens)",
	ThoughtCollapseHint: "▼ ⏎ to collapse",

	// Tool
	ToolNQuestions:       "(%d questions)",
	ToolQuestionDeclined: "(declined)",
	ToolTruncated:        "··· (truncated)",
	ToolTruncatedLines:   "... (truncated to %d lines)",
	ToolExpandAllHint:    "··· ⏎ to expand",
	ToolCollapseHint:     "▼ ⏎ to collapse",

	// Permission
	PermRequired: "▲ Permission Required",
	PermReason:   "Reason: ",
	PermAllow:    "Allow (this time)",
	PermAllowAll: "Always Allow (remember)",
	PermDeny:     "Deny (this time)",

	// Question
	QuestionOtherOption: "Other...",
	QuestionOtherPlaceholder: "Type a custom answer…",
	KeyBack:              "返回",

	// Picker
	PickerSelectTheme:  "▲ Select Theme",
	PickerSelectModel:  "▲ Select Model",
	PickerSelectEffort: "Select effort -",
	PickerSelectLocale: "▲ Select Language",
	PickerSelectSession: "Select Session",
	PickerSelectProvider: "▲ Select Provider",
	PickerThemeAuto:    "Auto (detect terminal background)",
	PickerScanning:     "Scanning files...",
	PickerNoResults:    "No files found",

	// Key bindings
	KeyNav:         "Navigate",
	KeyConfirm:     "Confirm",
	KeyDeny:        "Deny",
	KeyCancel:      "Cancel",
	KeyToggle:      "Toggle",
	KeySend:        "Send message",
	KeyInterrupt:   "Interrupt agent loop",
	KeyQuit:        "Double-tap to quit",
	KeyFocusNext:   "Focus next interactive paragraph",
	KeyFocusPrev:   "Focus previous interactive paragraph",
	KeyScrollUp:    "History/Scroll up",
	KeyScrollDown:  "History/Scroll down",
	KeyPageUp:      "Page up",
	KeyPageDown:    "Page down",
	KeyToggleTheme: "Toggle theme (dark/light/auto)",
	KeyJumpBottom:  "Jump to bottom",
	KeyPicker:      "Pick file/directory",
	KeyPaste:       "Paste",
	KeyHelp:        "Shortcuts",
	KeyHelpTitle:   "Keyboard Shortcuts",
	KeyHistoryUp:   "Navigate input history up",
	KeyHistoryDown: "Navigate input history down",

	// Focus separator
	FocusSeparatorHint: " ◆ Paragraph focused · ⏎ expand/collapse · Esc exit ◆ ",

	// Plan mode
	PlanEnterTitle:   "Enter plan mode?",
	PlanEnterDesc1:   "Agent will explore the codebase and design an approach,",
	PlanEnterDesc2:   "source edits are blocked until you approve the plan.",
	PlanEnterConfirm: "Confirm",
	PlanEnterCancel:  "Cancel",
	PlanExitTitle:    "Plan Approval",
	PlanExitApprove:  "Approve",
	PlanExitReject:   "Reject, continue editing",

	// Header
	HeaderSession: "session: ",

	TodoTitle:        "Todo — %d/%d items",
	TodoHiddenCount:  "%d hidden",
	TodoDoneCount:    "%d done",
	TodoInProgCount:  "%d in progress",
	TodoPendingCount: "%d pending",

	// Subagent suffix
	SubagentTurnsFmt: "%d turns",

	// Rewind
	RewindTitle:            "Rewind",
	RewindPrompt:           "Restore the code and/or conversation to the point before…",
	RewindNothingToRestore: "Nothing to rewind to yet.",
	RewindCurrent:          "(current)",
	RewindConfirmTitle:     "Rewind",
	RewindConfirmPrompt:    "Confirm you want to restore to the point before you sent this message:",
	RewindOptionBoth:       "Restore code and conversation",
	RewindOptionConv:       "Restore conversation only",
	RewindOptionCode:       "Restore code only",
	RewindOptionNeverMind:  "Never mind",
	RewindWarning:          "Rewinding does not affect files edited manually or via bash.",
	RewindRestoring:        "Restoring…",
	RewindFailed:           "Failed to restore: %v",
	RewindNoCodeChanges:    "No code changes",
	RewindFilesChanged:     "%d files changed",
	RewindSlashDescription: "Rewind code and/or conversation to a previous point",
}

// ---------------------------------------------------------------------------
// Locale 查询
// ---------------------------------------------------------------------------

// messagesFor 返回指定 locale 对应的 Messages 实例。
// 不支持的语言回退到 en-US。
func messagesFor(loc Locale) *Messages {
	switch loc {
	case LocaleZhCN:
		return &zhCN
	case LocaleEnUS:
		return &enUS
	default:
		return &enUS
	}
}

// ---------------------------------------------------------------------------
// 语言检测
// ---------------------------------------------------------------------------

// DetectLocale 从环境变量检测用户语言偏好。
// 优先级：LC_ALL > LANG > 默认 en-US。
// 仅识别 zh_CN / zh-CN / zh 系列为简体中文，其余回退英语。
func DetectLocale() Locale {
	for _, env := range []string{"LC_ALL", "LANG"} {
		val := os.Getenv(env)
		if val == "" {
			continue
		}
		normalized := strings.ToLower(strings.TrimSpace(val))
		// zh_CN.UTF-8 → zh_CN; zh-CN → zh-cn
		if strings.HasPrefix(normalized, "zh_cn") || strings.HasPrefix(normalized, "zh-cn") ||
			normalized == "zh" || strings.HasPrefix(normalized, "zh_") {
			return LocaleZhCN
		}
	}
	return LocaleEnUS
}

// resolveLocale 将 CLI --locale 参数解析为 Locale 值。
// "auto" → 自动检测，其余直接映射。
func resolveLocale(raw string) Locale {
	switch raw {
	case "zh-CN":
		return LocaleZhCN
	case "en-US":
		return LocaleEnUS
	case "auto", "":
		return DetectLocale()
	default:
		return DetectLocale()
	}
}
