package tui

// ---------------------------------------------------------------------------
// compat.go — 本地化的渲染依赖类型。
//
// 这些类型从 waveloom 移植而来（pkg/tool、pkg/todo、pkg/subagent、
// pkg/llm、pkg/pathutil），供渲染层使用。dsh-tui 不再依赖 waveloom 的
// 引擎包；dsh wire 数据在 project.go 中投影为这里的类型。
// ---------------------------------------------------------------------------

import "regexp"

// ---------------------------------------------------------------------------
// Diff 类型（原 waveloom pkg/tool）— edit_file 结构化 diff 渲染
// ---------------------------------------------------------------------------

// DiffLineKind 表示统一 diff 中一行的类型。
type DiffLineKind string

const (
	DiffAdd    DiffLineKind = "+" // 新增行
	DiffDel    DiffLineKind = "-" // 删除行
	DiffCtx    DiffLineKind = " " // 上下文行(未改动)
	DiffHeader DiffLineKind = "@" // hunk 头
)

// DiffLine 表示统一 diff 中的一行。
type DiffLine struct {
	Kind    DiffLineKind
	Content string // 不含前缀的实际内容
	OldNum  int    // 旧文件行号(0 = 不适用)
	NewNum  int    // 新文件行号(0 = 不适用)
}

// DiffHunk 表示一个 diff 块(一段连续的变更 + 上下文)。
type DiffHunk struct {
	FilePath  string // 所属文件路径(多文件编辑时标识 hunk 来源,空表示不适用)
	OldStart  int    // 旧文件起始行号(1-based)
	OldCount  int    // 旧文件覆盖行数
	NewStart  int    // 新文件起始行号(1-based)
	NewCount  int    // 新文件覆盖行数
	Heading   string // hunk 头部函数上下文(如 "func main() {")
	Lines     []DiffLine
	// NoNewlineAtEOF 表示 hunk 末尾的旧文件或新文件不以换行结尾。
	NoNewlineAtEOF bool
}

// Stats 返回该 hunk 的增删统计。
func (h DiffHunk) Stats() (add, del int) {
	for _, l := range h.Lines {
		switch l.Kind {
		case DiffAdd:
			add++
		case DiffDel:
			del++
		}
	}
	return
}

// ---------------------------------------------------------------------------
// TodoItem（原 waveloom pkg/todo）— todo 面板渲染
// ---------------------------------------------------------------------------

// TodoItem 描述一个待办任务。dsh 的 todo/write 事件只携带 Content/Status；
// ID/Description 保留以兼容渲染函数签名。
type TodoItem struct {
	ID          string // 系统自动分配的唯一标识(如 "1", "2", ...)
	Content     string // 祈使句:要完成的事项
	Status      string // pending | in_progress | completed
	Description string // 可选:任务详情/备注
}

// ---------------------------------------------------------------------------
// SubagentEvent（原 waveloom pkg/subagent）— 子 agent 段落渲染
// ---------------------------------------------------------------------------

// SubagentEventKind 区分子 agent 内部事件类型。
type SubagentEventKind int

const (
	SubagentText       SubagentEventKind = iota // agent 输出文本增量
	SubagentToolStart                            // 子 agent 开始执行工具
	SubagentToolResult                           // 子 agent 工具执行结果
	SubagentThought                              // 子 agent 思考过程(dimmed 渲染)
	SubagentToolStream                           // 子 agent 工具流式输出增量(│ 前缀)
)

// SubagentEvent 聚合子 agent 内部产生的一次事件。
type SubagentEvent struct {
	Turn       int
	ToolCallID string
	Kind       SubagentEventKind

	// SubagentText 时使用
	TextDelta string

	// SubagentToolStart / SubagentToolResult 时使用
	ToolName   string
	ToolArgs   string
	ToolResult string
	ToolDurMs  int64
	ToolError  string
}

// ---------------------------------------------------------------------------
// BalanceInfo（原 waveloom pkg/llm）— HUD 余额显示（dsh 无余额概念,
// 阶段 4 替换为 usage 显示;此处保留类型保持渲染函数签名不变）
// ---------------------------------------------------------------------------

// BalanceInfo 表示账户余额查询结果。
type BalanceInfo struct {
	IsAvailable  bool
	BalanceInfos []CurrencyBalance
}

// CurrencyBalance 表示单个币种的余额明细。
type CurrencyBalance struct {
	Currency        string // 货币代码:CNY / USD
	TotalBalance    string // 总的可用余额(包括赠金和充值余额)
	GrantedBalance  string // 未过期的赠金余额
	ToppedUpBalance string // 充值余额
}

// ---------------------------------------------------------------------------
// NormalizeShellCommand（原 waveloom pkg/pathutil）— 剥离命令中的 cd 前缀
// ---------------------------------------------------------------------------

var cdPattern = regexp.MustCompile(`^cd\s+(?:"([^"]*)"|'([^']*)'|([^\s;&]+))\s*(?:&&|;)\s*(.*)$`)

// NormalizeShellCommand 剥离命令中的 cd 前缀,返回归一化后的命令和提取的工作目录。
func NormalizeShellCommand(command string) (normalized string, extractedDir string) {
	matches := cdPattern.FindStringSubmatch(command)
	if matches == nil {
		return command, ""
	}
	dir := matches[1]
	if dir == "" {
		dir = matches[2]
	}
	if dir == "" {
		dir = matches[3]
	}
	return matches[4], dir
}
