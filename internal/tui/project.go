package tui

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Menfre01/dsh-tui/internal/dsh"
)

// ---------------------------------------------------------------------------
// project.go — dsh wire 事件 → TUI 段落 的投影层
//
// 这是阶段 1 的核心适配层:把 dsh 的 SessionEvent/MuxFrame 转成 waveloom
// 渲染层的 Paragraph 操作。事件模型对应关系(计划中的映射表):
//
//	user/message(text)          → paraUser
//	assistant/chunk text-delta  → 流式 paraAssistant
//	assistant/chunk reasoning   → 流式 paraThought
//	assistant/message           → 段落 finalize(含 usage)
//	tool/call + tool/result     → paraTool(按 callId 关联)
//	todo/write                  → todo 面板数据
//	turn/end                    → paraSystem 通知
//	其余插件扩展事件            → 按 ignorable 静默
// ---------------------------------------------------------------------------

// Projector 把 dsh 帧投影到 model 的段落列表。
type Projector struct {
	m *model

	// callId → 进行中的工具段落索引(dsh 提供稳定 callId;索引而非指针,
	// 因 m.paras append 扩容会使旧指针失效)
	toolIdx map[string]int

	// callId → tool/call 视图导出的 diff hunks(审批框预览用;
	// result 到达后清理)
	callDiffs map[string][]DiffHunk

	// callId → tool/call 事件时间戳(ms);tool/result 到达时计算耗时
	toolStart map[string]int64

	// 进行中的 assistant/thought 段落索引(-1 = 无)
	assistantIdx int
	thoughtIdx   int

	// 当前会话信息(HUD 显示)
	SessionID string
	Model     string
	Running   bool

	// 断线重连补洞
	lastSeq  int64                 // 当前会话已渲染的最大事件 seq
	onGap    func(sessionID string, fromSeq int64) // main 注入:拉 history 补缺口

	// 本回合 turn/start 事件时间戳(ms);turn/end 时计算回合耗时
	turnStartAt int64
	// 本轮(loop)token 增量:turn/start 重置,turn/end 通知用(waveloom 语义)
	loopPrompt int
	loopCompl  int
}

// NewProjector 创建投影器。
func NewProjector(m *model) *Projector {
	return &Projector{m: m, toolIdx: make(map[string]int), callDiffs: make(map[string][]DiffHunk), toolStart: make(map[string]int64), assistantIdx: -1, thoughtIdx: -1}
}

// Reset 清空投影器状态(会话切换时调用)。
func (p *Projector) Reset() {
	p.toolIdx = make(map[string]int)
	p.callDiffs = make(map[string][]DiffHunk)
	p.toolStart = make(map[string]int64)
	p.assistantIdx = -1
	p.thoughtIdx = -1
	p.SessionID = ""
	p.Running = false
	p.lastSeq = 0
	p.turnStartAt = 0
	p.loopPrompt = 0
	p.loopCompl = 0
}

// SetGapCallback 注入补洞回调(重连后历史缺口拉取)。
func (p *Projector) SetGapCallback(fn func(sessionID string, fromSeq int64)) {
	p.onGap = fn
}

// ReplayEvent 回放单个会话事件(session.history 或 mux 的 session/event 共用)。
func (p *Projector) ReplayEvent(ev *dsh.SessionEvent) {
	// 回放路径:历史 turn/end 的系统通知不重放(否则 Done 通知堆积历史回合)
	p.replayEvent(ev, nil, true)
}

// ReplayEventWithView 回放单个会话事件并附带宿主 view(渲染意图)。
// view 只影响对应 tool 段落的呈现(diff/read 结构化),不影响事件投影本身。
func (p *Projector) ReplayEventWithView(ev *dsh.SessionEvent, view json.RawMessage) {
	p.replayEvent(ev, view, true)
}

func (p *Projector) replayEvent(ev *dsh.SessionEvent, view json.RawMessage, silent bool) {
	if ev.Seq > p.lastSeq {
		p.lastSeq = ev.Seq
	}
	switch ev.Type {
	case dsh.EvUserMessage:
		p.onUserMessage(ev)
	case dsh.EvAssistantChunk:
		p.onAssistantChunk(ev)
	case dsh.EvAssistantMsg:
		p.onAssistantMessage(ev)
	case dsh.EvToolCall:
		p.onToolCall(ev)
		p.applyCallView(resolveCallID(ev), view)
	case dsh.EvToolResult:
		p.onToolResult(ev, view)
	case dsh.EvTodoWrite:
		p.onTodoWrite(ev)
	case dsh.EvTurnEnd:
		p.onTurnEnd(ev, silent)
	case dsh.EvTurnStart:
		// 宿主投影语义(todo projection apply):turn/start → todos 归 null,
		// 每回合维护独立的 todo 列表;全 completed 的面板随新回合开始消失。
		p.m.todos = nil
		p.turnStartAt = ev.Time
		p.loopPrompt = 0
		p.loopCompl = 0
		if !silent {
			// 实时回合:启动 elap 计时(回放不设置,历史时间无意义)
			p.m.turnStartTime = time.Now()
		}
	case dsh.EvStepStart, dsh.EvStepEnd:
		// 结构边界,渲染层无对应段落
	default:
		// 插件扩展事件(compaction/*、permission/preset、session/title 等):
		// ignorable 或纯 log 事件静默;required 未知事件给出系统提示。
		if ev.Ignorable == nil || !*ev.Ignorable {
			// 事件序列号过大的 log-only 事件也不打扰用户 —— 这里仅对
			// 少数已知无意义类型静默,其余显示为系统通知。
			ignored := map[string]bool{
				"request/header":  true,
				"request/context": true,
				"session/end-seed": true,
			}
			if !ignored[ev.Type] {
				// 保持安静:未知事件对渲染无影响
			}
		}
	}
}

// ReplayFrame 处理一条 mux 下行帧(事件 + 控制帧)。
// mux 流是全会话聚合的:必须按当前会话(sessionId)过滤,
// 否则其他会话的事件会串进当前视图。
func (p *Projector) ReplayFrame(frame dsh.ServerRequest) {
	var mux dsh.MuxFrame
	if err := frame.DecodePayload(&mux); err != nil {
		return
	}
	if mux.SessionID != "" && p.SessionID != "" && mux.SessionID != p.SessionID {
		return // 非当前会话的帧:忽略
	}
	switch mux.Type {
	case dsh.MuxSessionEvent:
		if mux.Event != nil {
			if len(mux.View) > 0 {
				p.replayEvent(mux.Event, mux.View, false)
			} else {
				p.replayEvent(mux.Event, nil, false)
			}
		}
	case dsh.MuxSessionSub:
		// 订阅基线:重连后对比本地 seq,缺口拉 history 补上
		if p.onGap != nil && p.lastSeq > 0 && mux.LastSeq > p.lastSeq {
			p.onGap(mux.SessionID, p.lastSeq+1)
		}
	case dsh.MuxApprovalReq:
		p.onApprovalRequested(string(frame.RpcID), mux)
	case dsh.MuxApprovalRes:
		p.onApprovalResolved(mux)
	case dsh.MuxQuestionReq:
		p.onQuestionRequested(string(frame.RpcID), mux)
	case dsh.MuxQuestionRes:
		p.onQuestionResolved(mux)
	case dsh.MuxSessionQueue:
		p.m.queueItems = mux.Items
	case dsh.MuxSessionJobs:
		p.m.jobCount = len(mux.Jobs)
	case dsh.MuxSessionProjection:
		p.onSessionProjection(mux.Key, mux.Value)
	case dsh.MuxStreamError:
		p.appendSystem(fmt.Sprintf("stream error: %s", mux.Error), notifError)
	}
}

// ReplayHostFrame 处理一条 host 下行帧(会话生命周期增量)。
func (p *Projector) ReplayHostFrame(frame dsh.ServerRequest) {
	var h dsh.HostFrame
	if err := frame.DecodePayload(&h); err != nil {
		return
	}
	switch h.Type {
	case dsh.HostSessionAdded:
		// 去重:重连时宿主会重推已 attach 会话的 session-added 帧,
		// 已存在则更新字段而非追加(否则列表累积重复项)
		idx := -1
		for i, s := range p.m.sessions {
			if s.SessionID == h.SessionID {
				idx = i
				break
			}
		}
		if idx >= 0 {
			// host 帧字段可能比 session.list 快照新:全覆盖(blank 显式值,
			// 非空字段增量),保持与 web mergeSummary 语义一致。
			p.m.sessions[idx].Blank = h.Blank
			if h.Origin != "" {
				p.m.sessions[idx].Origin = h.Origin
			}
			if h.Cwd != "" {
				p.m.sessions[idx].Cwd = h.Cwd
			}
			if h.AgentPreset != "" {
				p.m.sessions[idx].AgentPreset = h.AgentPreset
			}
		} else {
			p.m.sessions = append(p.m.sessions, SessionBrief{
				SessionID:  h.SessionID,
				Blank:      h.Blank,
				Origin:     h.Origin,
				Cwd:        h.Cwd,
				AgentPreset: h.AgentPreset,
			})
		}
	case dsh.HostSessionRemoved:
		for i, s := range p.m.sessions {
			if s.SessionID == h.SessionID {
				p.m.sessions = append(p.m.sessions[:i], p.m.sessions[i+1:]...)
				break
			}
		}
	case dsh.HostSessionStatus:
		for i := range p.m.sessions {
			if p.m.sessions[i].SessionID == h.SessionID {
				p.m.sessions[i].Running = h.Running
				break
			}
		}
		if p.SessionID == h.SessionID {
			p.SetRunning(h.Running)
		}
	}
}

// SetRunning 更新会话运行状态。
func (p *Projector) SetRunning(running bool) {
	p.Running = running
	// 与 model.running 保持同步(Enter 可用性/输入分隔线颜色依赖它;
	// 此前 turn/end 只更新了投影器字段,导致第二次 Enter 被 running 拦截)
	p.m.running = running
}

// ---------------------------------------------------------------------------
// 事件处理器
// ---------------------------------------------------------------------------

func (p *Projector) onUserMessage(ev *dsh.SessionEvent) {
	p.m.hudMessages++
	// wire data 就是 UserMessage(无包装),已在 spike 中确认真实结构。
	var msg dsh.Message
	if err := json.Unmarshal(ev.Data, &msg); err != nil {
		return
	}
	// 仅渲染直接用户消息;注入的上下文(AGENTS.md、runtime context)静默,
	// 避免噪音(与 dsh web 的 surface 行为一致)。
	var sourceKind string
	var source = struct{ Kind string `json:"kind"` }{}
	if len(msg.Source) > 0 && json.Unmarshal(msg.Source, &source) == nil {
		sourceKind = source.Kind
	}
	if sourceKind != "" && sourceKind != "user" {
		return
	}
	var text strings.Builder
	for _, b := range msg.Content {
		switch b.Type {
		case "text":
			text.WriteString(b.Text)
		case "image":
			text.WriteString("[image]")
		}
	}
	if text.Len() == 0 {
		return
	}
	p.m.paras = append(p.m.paras, Paragraph{
		Type:  paraUser,
		State: stateDone,
		Text:  text.String(),
	})
	p.markDirtyLast()
}

func (p *Projector) onAssistantChunk(ev *dsh.SessionEvent) {
	var d struct {
		Chunk dsh.StreamChunk `json:"chunk"`
	}
	if err := json.Unmarshal(ev.Data, &d); err != nil {
		return
	}
	switch d.Chunk.Type {
	case dsh.ChunkTextDelta:
		para := p.ensureStreamingPara(&p.assistantIdx, paraAssistant)
		para.Text += d.Chunk.Text
		para.renderDirty = true
	case dsh.ChunkReasoningDelta:
		para := p.ensureStreamingPara(&p.thoughtIdx, paraThought)
		para.Text += d.Chunk.Text
		para.renderDirty = true
	case dsh.ChunkToolCallDelta:
		// 参数增量流式(可选):阶段 1 跳过,等 block-end/tool/call 再建段
	case dsh.ChunkBlockEnd:
		// block-end 携带完整 block;tool-call block 在此建段
		var block struct {
			Type      string `json:"type"`
			ID        string `json:"id"`
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
		}
		if len(d.Chunk.Block) > 0 && json.Unmarshal(d.Chunk.Block, &block) == nil &&
			block.Type == "tool-call" && block.ID != "" {
			p.ensureToolPara(block.ID, block.Name, block.Arguments)
		}
	case dsh.ChunkUsage:
		if d.Chunk.Usage != nil {
			p.m.hudPromptTokens += d.Chunk.Usage.InputTokens
			p.m.hudComplTokens += d.Chunk.Usage.OutputTokens
			p.loopPrompt += d.Chunk.Usage.InputTokens
			p.loopCompl += d.Chunk.Usage.OutputTokens
		}
	case dsh.ChunkFinish:
		// 流结束:段落状态由 assistant/message finalize
	}
}

func (p *Projector) onAssistantMessage(ev *dsh.SessionEvent) {
	p.m.hudMessages++
	var d struct {
		Message dsh.Message  `json:"message"`
		Usage   *dsh.TokenUsage `json:"usage,omitempty"`
	}
	if err := json.Unmarshal(ev.Data, &d); err != nil {
		return
	}
	var text, reasoning strings.Builder
	for _, b := range d.Message.Content {
		switch b.Type {
		case "text":
			text.WriteString(b.Text)
		case "reasoning":
			reasoning.WriteString(b.Text)
		}
	}
	// finalize assistant 段落:以完整 message 为准(覆盖流式拼接,防丢块)
	if p.assistantIdx >= 0 && p.assistantIdx < len(p.m.paras) {
		p.assistantPara().State = stateDone
		if text.Len() > 0 {
			p.assistantPara().Text = text.String()
		}
		p.assistantPara().renderDirty = true
		p.assistantIdx = -1
	} else if text.Len() > 0 {
		p.m.paras = append(p.m.paras, Paragraph{
			Type:  paraAssistant,
			State: stateDone,
			Text:  text.String(),
		})
		p.markDirtyLast()
	}
	// finalize thought 段落
	if p.thoughtIdx >= 0 && p.thoughtIdx < len(p.m.paras) {
		p.thoughtPara().State = stateCollapsed
		if reasoning.Len() > 0 {
			p.thoughtPara().Text = reasoning.String()
		}
		p.thoughtPara().renderDirty = true
		p.thoughtIdx = -1
	} else if reasoning.Len() > 0 {
		p.m.paras = append(p.m.paras, Paragraph{
			Type:  paraThought,
			State: stateCollapsed,
			Text:  reasoning.String(),
		})
		p.markDirtyLast()
	}
	if d.Usage != nil {
		p.m.hudPromptTokens += d.Usage.InputTokens
		p.m.hudComplTokens += d.Usage.OutputTokens
		p.loopPrompt += d.Usage.InputTokens
		p.loopCompl += d.Usage.OutputTokens
		if d.Usage.CacheReadTokens != nil {
			p.m.hudCacheHit += *d.Usage.CacheReadTokens
		}
		if d.Usage.CacheWriteTokens != nil {
			p.m.hudCacheMiss += *d.Usage.CacheWriteTokens
		}
	}
}

func (p *Projector) onToolCall(ev *dsh.SessionEvent) {
	var d struct {
		CallID    string `json:"callId"`
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	}
	if err := json.Unmarshal(ev.Data, &d); err != nil {
		return
	}
	para := p.ensureToolPara(d.CallID, d.Name, d.Arguments)
	p.toolStart[d.CallID] = ev.Time
	if d.Name == "todo_write" {
		// todo_write 专用样式:参数是完整 todo 列表,不显示在摘要行
		para.ToolTodo = true
		para.ToolArgs = todoActiveContent(p.m.todos)
		para.ToolTodoSummary = todoSummaryFromTodos(p.m.todos)
	}
	if d.Name == "ask_user_question" {
		// 问题数从原始 arguments 解析(格式化摘要丢 JSON 结构)
		para.ToolQuestionCount = parseQuestionCount(d.Arguments)
	}
}

// resolveCallID 从 tool/call 事件提取 callId。
func resolveCallID(ev *dsh.SessionEvent) string {
	if ev == nil {
		return ""
	}
	var d struct {
		CallID string `json:"callId"`
	}
	if err := json.Unmarshal(ev.Data, &d); err != nil {
		return ""
	}
	return d.CallID
}

// applyCallView 应用 tool/call 的宿主 view:card=diff 时把将要发生的改动
// 挂到段落上(执行中即可见 diff 预览),同时缓存 callDiffs 供审批框展示。
func (p *Projector) applyCallView(callID string, raw json.RawMessage) {
	if callID == "" {
		return
	}
	tv, ok := parseToolEventView(raw)
	if !ok || tv.For != "call" {
		return
	}
	title, diffs, ok := parseDiffCard(tv.View)
	if !ok {
		return
	}
	var hunks []DiffHunk
	for i := range diffs {
		hunks = append(hunks, buildDiffHunks(diffs[i])...)
	}
	p.callDiffs[callID] = hunks
	_ = title // 标题可由渲染层从参数摘要推断,暂不单独存储
	if idx, ok := p.toolIdx[callID]; ok && idx >= 0 && idx < len(p.m.paras) {
		para := &p.m.paras[idx]
		para.DiffHunks = hunks
		para.renderDirty = true
	}
}

func (p *Projector) onToolResult(ev *dsh.SessionEvent, view json.RawMessage) {
	var d struct {
		Message dsh.ToolResultMessage `json:"message"`
		Error   *struct {
			Name string `json:"name"`
			Code string `json:"code"`
		} `json:"error,omitempty"`
	}
	if err := json.Unmarshal(ev.Data, &d); err != nil {
		return
	}
	callID := resolveResultCallID(&d.Message)
	idx, ok := p.toolIdx[callID]
	if !ok || idx < 0 || idx >= len(p.m.paras) {
		// result 先于 call 到达(理论上不该发生):兜底建段
		return
	}
	para := &p.m.paras[idx]
	delete(p.toolIdx, callID)
	delete(p.callDiffs, callID)

	// 执行耗时:call/result 事件时间戳差值
	if start, ok := p.toolStart[callID]; ok && ev.Time >= start {
		para.ToolDurMs = ev.Time - start
	}
	delete(p.toolStart, callID)

	// 提取结果文本(所有 text 块拼接)
	var out strings.Builder
	isError := false
	if len(d.Message.Content) > 0 {
		isError = d.Message.Content[0].IsError
		for _, b := range d.Message.Content[0].Content {
			switch b.Type {
			case "text":
				out.WriteString(b.Text)
			case "output_text":
				out.WriteString(b.Text)
			case "tool-result":
				// 嵌套
			}
		}
	}
	para.ToolResult = strings.TrimSpace(out.String())
	para.State = stateCollapsed
	if d.Error != nil || isError {
		para.State = stateError
		if d.Error != nil {
			para.ToolError = d.Error.Code
			para.ToolErrorKind = d.Error.Code // suffix 分类(如 FS_NOT_FOUND/ABORTED)
		} else {
			// isError=true 但无顶层 error(宿主仅标记失败):兜底分类
			para.ToolErrorKind = "failed"
		}
		if para.ToolError == "" {
			para.ToolError = "tool error"
		}
		// dsh 宿主不区分 fatal/recoverable 错误:统一红色显示
		para.ToolFatal = true
	}
	para.ToolExitCode = -1 // 默认未提供;terminal view 携带时覆盖
	p.applyResultView(para, view)
	// bash 非零退出码:宿主 isError=false(视为正常结果),但 TUI 按失败
	// 红色显示(exitCode 来自 terminal result view)
	if para.ToolName == "bash" && para.State != stateError && para.ToolExitCode > 0 {
		para.State = stateError
		para.ToolError = fmt.Sprintf("exit code %d", para.ToolExitCode)
		para.ToolErrorKind = fmt.Sprintf("exit=%d", para.ToolExitCode)
		para.ToolFatal = true
	}
	if para.ToolName == "todo_write" {
		// todo_write 专用样式:计数摘要从当前 todo 面板状态推导
		para.ToolTodo = true
		para.ToolArgs = todoActiveContent(p.m.todos)
		para.ToolTodoSummary = todoSummaryFromTodos(p.m.todos)
	}
	para.renderDirty = true
}

// resolveResultCallID 从 tool/result 事件的 message 提取 callId。
// 宿主字段名漂移兼容:toolCallId → callId → source.callId。
func resolveResultCallID(msg *dsh.ToolResultMessage) string {
	callID := ""
	if len(msg.Content) > 0 {
		callID = msg.Content[0].ToolCallID
		if callID == "" {
			callID = msg.Content[0].CallID
		}
	}
	if callID == "" && len(msg.Source) > 0 {
		var src struct {
			CallID string `json:"callId"`
		}
		if err := json.Unmarshal(msg.Source, &src); err == nil {
			callID = src.CallID
		}
	}
	return callID
}

// onSessionProjection 应用宿主投影帧(host-computed,权威值):
// contextPressure → ctx 进度条;sessionStats → HUD 回合/延迟等。
func (p *Projector) onSessionProjection(key string, raw json.RawMessage) {
	if len(raw) == 0 {
		return
	}
	switch key {
	case "title":
		var t string
		if err := json.Unmarshal(raw, &t); err == nil && t != "" {
			p.m.sessionTitle = t
		}
	case "contextPressure":
		var v struct {
			PressureTokens  int `json:"pressureTokens"`
			ProjectedTokens int `json:"projectedTokens"`
			ContextWindow   int `json:"contextWindow"`
		}
		if err := json.Unmarshal(raw, &v); err != nil {
			return
		}
		if v.PressureTokens > 0 {
			p.m.lastPromptTokens = v.PressureTokens
		}
		if v.ProjectedTokens > 0 {
			p.m.projectedTokens = v.ProjectedTokens
		}
		if v.ContextWindow > 0 {
			p.m.contextLimit = v.ContextWindow
		}
	case "tokenUsage":
		// 宿主权威会话累计 token(cache/tok 用其覆盖本地事件累计)
		var v struct {
			UnCachedInput int `json:"uncachedInputTokens"`
			Output        int `json:"outputTokens"`
			CacheRead     int `json:"cacheReadTokens"`
			CacheWrite    int `json:"cacheWriteTokens"`
		}
		if err := json.Unmarshal(raw, &v); err != nil {
			return
		}
		p.m.hudPromptTokens = v.UnCachedInput + v.CacheRead
		p.m.hudComplTokens = v.Output
		p.m.hudCacheHit = v.CacheRead
		p.m.hudCacheMiss = v.CacheWrite
	case "sessionStats":
		// 仅消费 turns(会话回合数);llmMs 是会话累计 LLM 耗时,
		// 与 elap 的"本轮耗时"语义不同,不覆盖
		var v struct {
			Turns int `json:"turns"`
		}
		if err := json.Unmarshal(raw, &v); err != nil {
			return
		}
		if v.Turns > 0 {
			p.m.hudTurns = v.Turns
		}
	}
}

// applyResultView 应用 tool/result 的宿主 view(渲染意图):
//
//	card=diff    → 填充 DiffHunks(替换 call 视图的预览)
//	card=read    → 填充 ReadPath/ReadLines(结构化行号窗口)
//	card=terminal→ 无 text 块时用 output 兜底
func (p *Projector) applyResultView(para *Paragraph, raw json.RawMessage) {
	tv, ok := parseToolEventView(raw)
	if !ok || tv.For != "result" {
		return
	}
	switch {
	case isCard(tv.View, "diff"):
		_, diffs, ok := parseDiffCard(tv.View)
		if !ok {
			return
		}
		hunks := make([]DiffHunk, 0, len(diffs))
		for i := range diffs {
			hunks = append(hunks, buildDiffHunks(diffs[i])...)
		}
		para.DiffHunks = hunks
	case isCard(tv.View, "read"):
		rv, ok := parseReadCard(tv.View)
		if !ok {
			return
		}
		para.ReadPath = rv.Path
		if len(rv.Lines) > 0 {
			para.ReadLines = rv.Lines
		}
	case isCard(tv.View, "search"):
		sv, ok := parseSearchCard(tv.View)
		if !ok {
			return
		}
		if sv.Shape == "matches" {
			para.SearchGroups = sv.Groups()
			para.SearchPaths = nil
		} else {
			para.SearchPaths = sv.Paths
			para.SearchGroups = nil
		}
		para.SearchTruncated = sv.Truncated
		para.SearchTotal = sv.Total
	case isCard(tv.View, "terminal"):
		cv, ok := parseTerminalCard(tv.View)
		if !ok {
			return
		}
		if para.ToolResult == "" && cv.Output != "" {
			para.ToolResult = strings.TrimSpace(cv.Output)
		}
		if cv.ExitCode != nil {
			para.ToolExitCode = *cv.ExitCode
		} else {
			para.ToolExitCode = -1 // 宿主未提供
		}
		para.ToolSignal = cv.Signal
	}
}

// isCard 判断 view 卡片的 card 判别字段。
func isCard(view json.RawMessage, card string) bool {
	var c struct {
		Card string `json:"card"`
	}
	if err := json.Unmarshal(view, &c); err != nil {
		return false
	}
	return c.Card == card
}

// todoSummaryFromTodos 生成计数摘要(已完成/总数,与 todo 面板标题
// Todo — x/y items 语义一致),suffix 只显示计数,任务内容走参数位。
func todoSummaryFromTodos(todos []TodoItem) string {
	total := len(todos)
	if total == 0 {
		return ""
	}
	done := 0
	for i := range todos {
		if todos[i].Status == "completed" {
			done++
		}
	}
	return fmt.Sprintf("%d/%d", done, total)
}

// todoActiveContent 返回进行中任务的内容(无进行中项时为空串)。
func todoActiveContent(todos []TodoItem) string {
	for i := range todos {
		if todos[i].Status == "in_progress" {
			return todos[i].Content
		}
	}
	return ""
}

func (p *Projector) onTodoWrite(ev *dsh.SessionEvent) {
	var d struct {
		Todos []dsh.TodoItem `json:"todos"`
	}
	if err := json.Unmarshal(ev.Data, &d); err != nil {
		return
	}
	items := make([]TodoItem, 0, len(d.Todos))
	for _, t := range d.Todos {
		items = append(items, TodoItem{Content: t.Content, Status: t.Status})
	}
	p.m.todos = items
	// 若 todo/write 在 tool/result 之后到达,刷新最近 todo_write 段落的计数
	p.refreshTodoSummary()
}

// refreshTodoSummary 更新最近一个 todo_write 段落的计数摘要(如有)。
func (p *Projector) refreshTodoSummary() {
	for i := len(p.m.paras) - 1; i >= 0; i-- {
		para := &p.m.paras[i]
		if para.Type == paraTool && para.ToolTodo {
			summary := todoSummaryFromTodos(p.m.todos)
			if summary != para.ToolTodoSummary {
				para.ToolTodoSummary = summary
				para.renderDirty = true
			}
			content := todoActiveContent(p.m.todos)
			if content != para.ToolArgs {
				para.ToolArgs = content
				para.renderDirty = true
			}
			return
		}
	}
}

func (p *Projector) onTurnEnd(ev *dsh.SessionEvent, silent bool) {
	var d struct {
		Turn   int             `json:"turn"`
		Reason dsh.TurnEndReason `json:"reason"`
	}
	if err := json.Unmarshal(ev.Data, &d); err != nil {
		return
	}
	// 回合耗时 = turn/end 与 turn/start 事件时间戳差(回放/实时均准确;
	// hudLatMs 此前从未填充,中断时显示 0ms)
	if p.turnStartAt > 0 && ev.Time >= p.turnStartAt {
		p.m.hudLatMs = ev.Time - p.turnStartAt
	}
	// HUD 回合计数(宿主权威:会话绝对回合号)
	p.m.hudTurns = d.Turn
	p.turnStartAt = 0
	turnDur := formatDuration(int64(p.m.hudLatMs))
	loopIn := formatTokens(p.loopPrompt)
	loopOut := formatTokens(p.loopCompl)

	// 回放历史时不重放回合结束的系统通知(状态收尾照常执行)
	if !silent {
		lc := p.m.msg()
		switch d.Reason.Kind {
		case dsh.TurnEndCompleted:
			// 本轮耗时 + 本轮 token(waveloom 语义);hud* 是会话累计,仅 HUD 用。
			// turn 号(会话累计回合)语义易误读,不在通知中展示。
			p.appendSystem(fmt.Sprintf(lc.LoopCompleted,
				turnDur, loopIn, loopOut), notifInfo)
		case dsh.TurnEndAborted:
			// %s = 本轮耗时(对齐 waveloom);dsh 的 reason.reason.kind(如 user)
			// 作为取消分类未单列展示
			p.appendSystem(fmt.Sprintf(lc.LoopAborted, turnDur), notifWarn)
		case dsh.TurnEndError:
			p.appendSystem(fmt.Sprintf(lc.LoopModelError, "turn", "error"), notifError)
		case dsh.TurnEndMaxTokens:
			p.appendSystem(fmt.Sprintf(lc.LoopMaxTurns, d.Turn,
				turnDur, loopIn, loopOut), notifWarn)
		default:
			p.appendSystem(fmt.Sprintf("turn %d ended (%s)", d.Turn, d.Reason.Kind), notifInfo)
		}
	}
	p.finalizeTurnParas()
	p.SetRunning(false)
}

// finalizeTurnParas 收尾回合结束仍未完成的流式段落。
//
// 中断/异常时 assistant/message、tool/result 事件不会到达,段落停留在
// stateStreaming,前缀 spinner 永远旋转(渲染层聚焦/滚动时会看到)。
// turn/end 是终态信号,此时仍 streaming 的段落必然是泄漏,统一收尾:
// assistant → done,thought → collapsed(保留内容可展开),tool → done。
func (p *Projector) finalizeTurnParas() {
	if p.assistantIdx >= 0 && p.assistantIdx < len(p.m.paras) {
		para := &p.m.paras[p.assistantIdx]
		if para.State == stateStreaming {
			para.State = stateDone
			para.renderDirty = true
		}
		p.assistantIdx = -1
	}
	if p.thoughtIdx >= 0 && p.thoughtIdx < len(p.m.paras) {
		para := &p.m.paras[p.thoughtIdx]
		if para.State == stateStreaming {
			para.State = stateCollapsed
			para.renderDirty = true
		}
		p.thoughtIdx = -1
	}
	for callID, idx := range p.toolIdx {
		if idx >= 0 && idx < len(p.m.paras) {
			para := &p.m.paras[idx]
			if para.State == stateStreaming {
				para.State = stateDone
				para.renderDirty = true
			}
		}
		delete(p.toolIdx, callID)
	}
	// 残留的进行中工具计时一并清理
	for callID := range p.toolStart {
		delete(p.toolStart, callID)
	}
}

// onApprovalRequested 弹出审批确认覆盖层(阻断式)。
func (p *Projector) onApprovalRequested(rpcID string, mux dsh.MuxFrame) {
	// 审批帧本身不带 view;diff 预览从 tool/call 视图缓存中按 callId 反查
	var diffs []DiffHunk
	if mux.CallID != "" {
		diffs = p.callDiffs[mux.CallID]
	}
	p.m.pendingApproval = &PendingApproval{
		RpcID:      rpcID,
		SessionID:  mux.SessionID,
		ApprovalID: mux.ApprovalID,
		ToolName:   mux.ToolName,
		CallID:     mux.CallID,
		Reason:     mux.Reason,
		Diffs:      diffs,
	}
	p.m.overlay = overlayPermission
}

func (p *Projector) onApprovalResolved(mux dsh.MuxFrame) {
	// 应答已由 respond 的返回确认;此帧仅收尾(清除覆盖层)。
	if p.m.pendingApproval != nil && p.m.pendingApproval.ApprovalID == mux.ApprovalID {
		p.m.pendingApproval = nil
		p.m.overlay = overlayNone
		p.m.input.Focus()
	}
}

// onQuestionRequested 弹出问题回答覆盖层(阻断式)。
func (p *Projector) onQuestionRequested(rpcID string, mux dsh.MuxFrame) {
	p.m.pendingQuestion = &PendingQuestion{
		RpcID:     rpcID,
		SessionID: mux.SessionID,
		Questions: mux.Questions,
		Selection: make(map[int]int), // questionIdx → optionIdx(单选)
		Customs:   make(map[int]string),
	}
	p.m.overlay = overlayQuestion
}

func (p *Projector) onQuestionResolved(mux dsh.MuxFrame) {
	if p.m.pendingQuestion != nil && p.m.pendingQuestion.RpcID == mux.QuestionRpcID {
		p.m.pendingQuestion = nil
		p.m.overlay = overlayNone
		p.m.input.Focus()
	}
}

// ---------------------------------------------------------------------------
// 段落辅助
// ---------------------------------------------------------------------------

// assistantPara/thoughtPara 按索引访问当前进行中段落。
func (p *Projector) assistantPara() *Paragraph {
	return &p.m.paras[p.assistantIdx]
}

func (p *Projector) thoughtPara() *Paragraph {
	return &p.m.paras[p.thoughtIdx]
}

// ensureStreamingPara 返回一个流式段落;不存在时创建(含呼吸动画前缀)。
func (p *Projector) ensureStreamingPara(idx *int, typ ParagraphType) *Paragraph {
	if *idx >= 0 && *idx < len(p.m.paras) {
		return &p.m.paras[*idx]
	}
	p.m.paras = append(p.m.paras, Paragraph{
		Type:  typ,
		State: stateStreaming,
	})
	// 存索引而非指针:后续 append 扩容会使旧指针失效
	*idx = len(p.m.paras) - 1
	p.markDirtyLast()
	return &p.m.paras[*idx]
}

// ensureToolPara 按 callId 创建或复用工具段落。
func (p *Projector) ensureToolPara(callID, name, args string) *Paragraph {
	if idx, ok := p.toolIdx[callID]; ok && idx >= 0 && idx < len(p.m.paras) {
		para := &p.m.paras[idx]
		if name != "" {
			para.ToolName = name
		}
		if args != "" {
			para.ToolArgs = formatToolArgs(name, args, p.m.cwd)
		}
		para.renderDirty = true
		return para
	}
	para := Paragraph{
		Type:      paraTool,
		State:     stateStreaming,
		ToolName:  name,
		ToolArgs:  formatToolArgs(name, args, p.m.cwd),
	}
	p.m.paras = append(p.m.paras, para)
	p.toolIdx[callID] = len(p.m.paras) - 1
	p.markDirtyLast()
	return &p.m.paras[p.toolIdx[callID]]
}

// appendSystem 追加系统通知段落。
func (p *Projector) appendSystem(text string, kind systemNotifKind) {
	p.m.paras = append(p.m.paras, Paragraph{
		Type:      paraSystem,
		State:     stateDone,
		Text:      text,
		NotifKind: kind,
	})
	p.markDirtyLast()
}

// markDirtyLast 标记最后一段需要重渲染并滚动到底部(若已锁定)。
func (p *Projector) markDirtyLast() {
	if len(p.m.paras) == 0 {
		return
	}
	p.m.paras[len(p.m.paras)-1].renderDirty = true
	if p.m.pinnedToBottom {
		p.m.scrollToBottom()
	}
}
