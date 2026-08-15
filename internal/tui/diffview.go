package tui

import (
	"encoding/json"
	"strings"
)

// ---------------------------------------------------------------------------
// diffview.go — 宿主 view(渲染意图)解析与 diff 转换
//
// 宿主在每个 tool/call 与 tool/result 事件旁附带 view 字段
// (ToolEventView: {for, view:{card,...}}),由工具自身的 presenter 派生,
// 见 @deepseek-ai/dsh-tools/src/presentation。本文件把:
//
//	card=diff → DiffHunk[](带行号/增删行的统一 diff,渲染层直接消费)
//	card=read → ReadLine[](带行号的文件窗口)
//
// 宿主 FileDiff 只给 {path, oldText|null, newText},没有行号,需要
// 客户端自行做行级对齐(前后缀匹配 + 中间 LCS,超大中间块回退全删全增)。
// ---------------------------------------------------------------------------

// ToolEventView 是宿主 view 字段的外层信封。
type ToolEventView struct {
	For  string          `json:"for"` // "call" | "result"
	View json.RawMessage `json:"view"`
}

// diffViewCard 是 card=diff 的视图(宿主 DiffResultView / DiffCallView)。
type diffViewCard struct {
	Card  string     `json:"card"`
	Title string     `json:"title"`
	Diffs []fileDiff `json:"diffs"`
}

// fileDiff 是宿主 FileDiff:oldText 为 null 表示新文件创建或整文件覆盖
// (call 时 presenter 拿不到旧内容)。
type fileDiff struct {
	Path    string  `json:"path"`
	OldText *string `json:"oldText"`
	NewText string  `json:"newText"`
}

// readViewCard 是 card=read 的视图(宿主 ReadResultView)。
type readViewCard struct {
	Card       string     `json:"card"`
	Path       string     `json:"path"`
	Offset     int        `json:"offset"`
	Lines      []ReadLine `json:"lines"`
	TotalLines int        `json:"totalLines"`
}

// ReadLine 是宿主 ReadFileLine 的 tui 侧镜像。
type ReadLine struct {
	Number int    `json:"number"`
	Text   string `json:"text"`
}

// parseToolEventView 解析 view 信封;结构不匹配时 ok=false。
func parseToolEventView(raw json.RawMessage) (*ToolEventView, bool) {
	if len(raw) == 0 {
		return nil, false
	}
	var tv ToolEventView
	if err := json.Unmarshal(raw, &tv); err != nil || tv.For == "" {
		return nil, false
	}
	return &tv, true
}

// parseDiffCard 解析 card=diff 视图,返回标题与文件 diff 列表。
func parseDiffCard(view json.RawMessage) (string, []fileDiff, bool) {
	var c diffViewCard
	if err := json.Unmarshal(view, &c); err != nil || c.Card != "diff" {
		return "", nil, false
	}
	return c.Title, c.Diffs, true
}

// parseReadCard 解析 card=read 视图。
func parseReadCard(view json.RawMessage) (*readViewCard, bool) {
	var c readViewCard
	if err := json.Unmarshal(view, &c); err != nil || c.Card != "read" {
		return nil, false
	}
	return &c, true
}

// parseTerminalCard 解析 card=terminal 视图(仅取 result 侧 output)。
type terminalViewCard struct {
	Card     string `json:"card"`
	Title    string `json:"title"`
	Output   string `json:"output"`
	ExitCode *int   `json:"exitCode"` // nil = 宿主未提供(信号终止或未知)
	Signal   string `json:"signal"`
}

func parseTerminalCard(view json.RawMessage) (*terminalViewCard, bool) {
	var c terminalViewCard
	if err := json.Unmarshal(view, &c); err != nil || c.Card != "terminal" {
		return nil, false
	}
	return &c, true
}

// buildDiffHunks 把一个宿主 FileDiff 转成渲染层 DiffHunk(单文件 → 单 hunk)。
// oldText 为 nil(新文件/覆盖)时全部行视为新增。
func buildDiffHunks(fd fileDiff) []DiffHunk {
	oldLines := splitLinesPtr(fd.OldText)
	newLines := splitLines(fd.NewText)

	// 前后缀逐行匹配(相同的未改动行)
	pre := 0
	for pre < len(oldLines) && pre < len(newLines) && oldLines[pre] == newLines[pre] {
		pre++
	}
	suf := 0
	for suf < len(oldLines)-pre && suf < len(newLines)-pre &&
		oldLines[len(oldLines)-1-suf] == newLines[len(newLines)-1-suf] {
		suf++
	}
	midOld := oldLines[pre : len(oldLines)-suf]
	midNew := newLines[pre : len(newLines)-suf]

	// 中间块 LCS 对齐(仅当规模可控;超大块回退全删全增)
	var ops []lcsOp // 按统一 diff 顺序的中间块操作
	if len(midOld)*len(midNew) > 200000 {
		ops = lcsOpsAll(midOld, midNew)
	} else {
		ops = lcsOps(midOld, midNew)
	}

	h := DiffHunk{
		FilePath: fd.Path,
		Lines:    make([]DiffLine, 0, pre+suf+len(ops)),
	}
	oldNum, newNum := 1, 1

	// 前缀上下文行
	for _, l := range oldLines[:pre] {
		h.Lines = append(h.Lines, DiffLine{Kind: DiffCtx, Content: l, OldNum: oldNum, NewNum: newNum})
		oldNum++
		newNum++
	}
	// 中间变更行
	for _, op := range ops {
		switch op.kind {
		case DiffDel:
			h.Lines = append(h.Lines, DiffLine{Kind: DiffDel, Content: op.text, OldNum: oldNum})
			oldNum++
		case DiffAdd:
			h.Lines = append(h.Lines, DiffLine{Kind: DiffAdd, Content: op.text, NewNum: newNum})
			newNum++
		default:
			h.Lines = append(h.Lines, DiffLine{Kind: DiffCtx, Content: op.text, OldNum: oldNum, NewNum: newNum})
			oldNum++
			newNum++
		}
	}
	// 后缀上下文行
	for _, l := range oldLines[len(oldLines)-suf:] {
		h.Lines = append(h.Lines, DiffLine{Kind: DiffCtx, Content: l, OldNum: oldNum, NewNum: newNum})
		oldNum++
		newNum++
	}

	// hunk 头行号 = 第一行的行号(无旧行时用新侧)
	if len(h.Lines) > 0 {
		first := h.Lines[0]
		h.OldStart = first.OldNum
		h.NewStart = first.NewNum
		if h.OldStart == 0 {
			h.OldStart = first.NewNum
		}
		if h.NewStart == 0 {
			h.NewStart = first.OldNum
		}
	}
	h.OldCount = oldNum - 1
	h.NewCount = newNum - 1
	return []DiffHunk{h}
}

// lcsOp 是中间块的一个输出行。
type lcsOp struct {
	kind DiffLineKind
	text string
}

// lcsOps 用 DP LCS 生成中间块的行对齐序列(按统一 diff 顺序)。
// 完整 DP 表:调用方保证 n*m ≤ 200000,内存可控。
func lcsOps(oldL, newL []string) []lcsOp {
	n, m := len(oldL), len(newL)
	dp := make([][]int, n+1)
	for i := range dp {
		dp[i] = make([]int, m+1)
	}
	for i := 1; i <= n; i++ {
		for j := 1; j <= m; j++ {
			if oldL[i-1] == newL[j-1] {
				dp[i][j] = dp[i-1][j-1] + 1
			} else if dp[i][j-1] >= dp[i-1][j] {
				dp[i][j] = dp[i][j-1]
			} else {
				dp[i][j] = dp[i-1][j]
			}
		}
	}
	// 回溯生成序列
	ops := make([]lcsOp, 0, n+m)
	i, j := n, m
	for i > 0 && j > 0 {
		if oldL[i-1] == newL[j-1] {
			ops = append(ops, lcsOp{DiffCtx, oldL[i-1]})
			i--
			j--
		} else if dp[i][j-1] >= dp[i-1][j] {
			ops = append(ops, lcsOp{DiffAdd, newL[j-1]})
			j--
		} else {
			ops = append(ops, lcsOp{DiffDel, oldL[i-1]})
			i--
		}
	}
	for ; i > 0; i-- {
		ops = append(ops, lcsOp{DiffDel, oldL[i-1]})
	}
	for ; j > 0; j-- {
		ops = append(ops, lcsOp{DiffAdd, newL[j-1]})
	}
	// 回溯顺序是反向的,翻转
	for l, r := 0, len(ops)-1; l < r; l, r = l+1, r-1 {
		ops[l], ops[r] = ops[r], ops[l]
	}
	return ops
}

// lcsOpsAll 回退方案:先删全部旧行,再增全部新行。
func lcsOpsAll(oldL, newL []string) []lcsOp {
	ops := make([]lcsOp, 0, len(oldL)+len(newL))
	for _, l := range oldL {
		ops = append(ops, lcsOp{DiffDel, l})
	}
	for _, l := range newL {
		ops = append(ops, lcsOp{DiffAdd, l})
	}
	return ops
}

// splitLinesPtr 拆分 *string 为行数组(nil → 空)。
func splitLinesPtr(s *string) []string {
	if s == nil {
		return nil
	}
	return splitLines(*s)
}

// splitLines 按行拆分,去掉单个尾部换行("" → nil)。
func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	s = strings.TrimSuffix(s, "\n")
	return strings.Split(s, "\n")
}

// ---------------------------------------------------------------------------
// search 卡片(宿主 SearchResultView):grep/glob 的结构化结果。
// matches shape:按文件分组的匹配行;paths shape:扁平路径列表。
// ---------------------------------------------------------------------------

// SearchFileGroup 是一个文件的匹配行分组(matches shape)。
type SearchFileGroup struct {
	Path    string
	Matches []SearchMatch
}

// SearchMatch 是一行匹配(lineNumber 为文件内 1-based 行号)。
type SearchMatch struct {
	LineNumber int
	Line       string
}

// searchViewCard 是 card=search 的视图。
type searchViewCard struct {
	Card      string `json:"card"`
	Shape     string `json:"shape"` // "matches" | "paths"
	Title     string `json:"title"`
	Truncated bool   `json:"truncated"`
	Total     int    `json:"total"`
	Files     []struct {
		Path    string `json:"path"`
		Matches []struct {
			LineNumber int    `json:"lineNumber"`
			Line       string `json:"line"`
		} `json:"matches"`
	} `json:"files"`
	Paths []string `json:"paths"`
}

// parseSearchCard 解析 card=search 视图。
func parseSearchCard(view json.RawMessage) (*searchViewCard, bool) {
	var c searchViewCard
	if err := json.Unmarshal(view, &c); err != nil || c.Card != "search" {
		return nil, false
	}
	return &c, true
}

// Groups 返回 matches shape 的结构化分组。
func (c *searchViewCard) Groups() []SearchFileGroup {
	groups := make([]SearchFileGroup, 0, len(c.Files))
	for _, f := range c.Files {
		g := SearchFileGroup{Path: f.Path, Matches: make([]SearchMatch, 0, len(f.Matches))}
		for _, mm := range f.Matches {
			g.Matches = append(g.Matches, SearchMatch{LineNumber: mm.LineNumber, Line: mm.Line})
		}
		groups = append(groups, g)
	}
	return groups
}
