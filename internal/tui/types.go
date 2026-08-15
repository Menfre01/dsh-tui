package tui

// ---------------------------------------------------------------------------
// types.go — 从 waveloom 移植的 UI 类型(本地化,无引擎依赖)
//
// 来源: cmd/waveloom/tui_permission.go、tui_command.go、pkg/permission、
// pkg/llm、pkg/slashcommand。permission.* 类型在此简化本地化;阶段 2
// 的 dsh 审批/提问适配层将复用这里的结构。
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// 权限/问题消息类型(原 cmd/waveloom/tui_permission.go)
// ---------------------------------------------------------------------------

// Decision 用户对权限请求的决策。
type Decision string

const (
	DecisionAllow Decision = "allow"
	DecisionDeny  Decision = "deny"
)

// DecisionReason 权限决策的原因分类。
type DecisionReason string

const (
	ReasonRule         DecisionReason = "rule"
	ReasonDefault      DecisionReason = "default"
	ReasonSafety       DecisionReason = "safety"
	ReasonSession      DecisionReason = "session"
	ReasonBypass       DecisionReason = "bypass"
	ReasonBuiltinAllow DecisionReason = "builtin_allow"
)

// RuleScope 规则的持久化范围。
type RuleScope string

const (
	ScopeSession RuleScope = "session"
	ScopeConfig  RuleScope = "config"
)

// UserChoice 用户的选择结果。
type UserChoice struct {
	Decision      Decision  // allow 或 deny
	RememberScope RuleScope // "" → 不记住;ScopeSession → session 内记住;ScopeConfig → 持久化到配置文件
	Feedback      string    // 可选的用户反馈文本
}

// QuestionPrompt 是向用户展示的单个选择题。
type QuestionPrompt struct {
	Question    string               `json:"question"`    // 完整问题,以 ? 结尾
	Header      string               `json:"header"`      // 简短标签,≤12 chars
	Options     []QuestionOptionPrompt `json:"options"`   // 2-4 项,label 唯一
	MultiSelect bool                 `json:"multiSelect"` // 是否多选,默认 false
}

// QuestionOptionPrompt 是选择题的单个选项。
type QuestionOptionPrompt struct {
	Label       string `json:"label"`       // 显示文本,1-5 words
	Description string `json:"description"` // 选项解释
}

// QuestionResponse 是用户对单个问题的回答。
type QuestionResponse struct {
	Question string   `json:"question"` // 问题文本(与 QuestionPrompt.Question 对应)
	Answers  []string `json:"answers"`  // 选中的选项 label 列表;单选时为 1 个元素
}

// PlanApproval 用户对 plan 的审批结果。
type PlanApproval struct {
	Approved bool
	Feedback string
}

// permissionReqMsg 权限确认请求。
type permissionReqMsg struct {
	toolName   string
	args       string
	reason     string
	reasonKind DecisionReason
	reply      chan<- UserChoice
}

// questionReqMsg AskUserQuestion 请求。
type questionReqMsg struct {
	questions []QuestionPrompt
	reply     chan<- []QuestionResponse
}

// planEnterReqMsg 进入 plan 模式确认请求。
type planEnterReqMsg struct {
	reply chan<- bool
}

// planExitReqMsg 退出 plan 模式审批请求(含 plan 内容)。
type planExitReqMsg struct {
	plan  string
	reply chan<- PlanApproval
}

// enterPlanModeByUserMsg 用户主动进入 plan 模式的消息。
type enterPlanModeByUserMsg struct{}

// exitPlanModeByUserMsg 用户通过审批界面批准退出 plan 模式的消息。
type exitPlanModeByUserMsg struct{}

// overlayAnimTickMsg 覆盖层动画帧推进(~50ms tick),用于淡入效果。
type overlayAnimTickMsg struct{}

// ---------------------------------------------------------------------------
// 主题选择器(原 cmd/waveloom/tui_command.go)
// ---------------------------------------------------------------------------

// themeItem 是主题选择器列表项,实现 list.DefaultItem 接口。
type themeItem struct {
	label string
	mode  string
}

func (i themeItem) Title() string       { return i.label }
func (i themeItem) Description() string { return "" }
func (i themeItem) FilterValue() string { return i.label }

// themeItems 返回主题选择器的固定选项。label 为占位值,运行时由
// buildThemeList 根据 locale 替换。
var themeItems = []themeItem{
	{label: "Auto", mode: "auto"},
	{label: "Dark", mode: "dark"},
	{label: "Light", mode: "light"},
	{label: "Dark CB", mode: "darkcolorblind"},
	{label: "Light CB", mode: "lightcolorblind"},
}

// providerPickerItemData 从 slash command 传回的 provider 选项。
type providerPickerItemData struct {
	Name    string `json:"name"`
	Model   string `json:"model"`
	BaseURL string `json:"base_url"`
	Current bool   `json:"current"`
}

// ---------------------------------------------------------------------------
// ModelInfo(原 waveloom pkg/llm)— 模型选择器条目
// ---------------------------------------------------------------------------

// ModelInfo 表示从 Provider 的 GET /models 接口获取的模型基本信息。
type ModelInfo struct {
	ID      string `json:"id"`       // 模型标识符,如 "deepseek-v4-pro"
	Object  string `json:"object"`   // 对象类型,其值为 "model"
	OwnedBy string `json:"owned_by"` // 拥有该模型的组织
}

// ---------------------------------------------------------------------------
// CommandInfo(原 waveloom pkg/slashcommand)— 命令选择器条目
// ---------------------------------------------------------------------------

// CommandInfo 是命令的公开信息(给 /help 和自动补全使用)。
type CommandInfo struct {
	Name        string
	Aliases     []string
	Description string
	Args        string // 参数占位符,如 "model";无参数时为空
}
