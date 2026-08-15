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
