package dsh

import "encoding/json"

// ---------------------------------------------------------------------------
// Session events — the append-only session log, the single source of truth
// for rendering. Mirrors SessionEventMap in packages/core/session.
// The log is merge-extensible: unknown event types must be tolerated (a
// required unknown event should refuse reconstruction; for rendering we
// surface it as a system notice when it is not marked ignorable).
// ---------------------------------------------------------------------------

// SessionEvent is one immutable entry in the session log.
type SessionEvent struct {
	Type           string          `json:"type"`
	Seq            int64           `json:"seq"`
	Time           int64           `json:"time"`
	Data           json.RawMessage `json:"data"`
	SourceEventSeqs []int64        `json:"sourceEventSeqs,omitempty"`
	SurfaceOp      json.RawMessage `json:"surfaceOp,omitempty"`
	Ignorable      *bool           `json:"ignorable,omitempty"`
}

// Session event types (core vocabulary; plugins may add more).
const (
	EvTurnStart       = "turn/start"
	EvTurnEnd         = "turn/end"
	EvStepStart       = "step/start"
	EvStepEnd         = "step/end"
	EvUserMessage     = "user/message"
	EvAssistantChunk  = "assistant/chunk"
	EvAssistantMsg    = "assistant/message"
	EvToolCall        = "tool/call"
	EvToolResult      = "tool/result"
	EvTodoWrite       = "todo/write"
	EvRequestHeader   = "request/header"
	EvRequestContext  = "request/context"
	EvSessionEndSeed  = "session/end-seed"
)

// TurnEndReason kinds (TurnEndReasonMap, merge-extensible).
const (
	TurnEndCompleted  = "completed"
	TurnEndAborted    = "aborted"
	TurnEndBlocked    = "blocked"
	TurnEndError      = "error"
	TurnEndMaxTokens  = "max-tokens"
	TurnEndInterrupted = "interrupted"
)

// TurnEndEventData closes turn N with the reason that ended it.
type TurnEndEventData struct {
	Turn   int             `json:"turn"`
	Reason TurnEndReason   `json:"reason"`
}

// TurnEndReason is the discriminated reason payload of turn/end.
type TurnEndReason struct {
	Kind string          `json:"kind"`
	// error kind carries a structured failure.
	Error json.RawMessage `json:"error,omitempty"`
	// aborted kind carries the cancellation cause.
	Reason json.RawMessage `json:"reason,omitempty"`
}

// Message is the shared message representation (dsh-llm Message).
type Message struct {
	ID      string          `json:"id"`
	Role    string          `json:"role"`
	Content []ContentBlock  `json:"content"`
	Source  json.RawMessage `json:"source,omitempty"`
}

// ContentBlock is a loose content block. Core is merge-extensible; only the
// `type` discriminant is strict, the rest stays wide. Fields cover the types
// the TUI renders: text, reasoning, image, tool-result.
type ContentBlock struct {
	Type       string          `json:"type"`
	Text       string          `json:"text,omitempty"`
	Name       string          `json:"name,omitempty"`
	MediaType  string          `json:"mediaType,omitempty"`
	Data       string          `json:"data,omitempty"`
	CallID     string          `json:"callId,omitempty"`
	ToolCallID string          `json:"toolCallId,omitempty"` // 真实宿主 tool/result 用此字段名
	IsError    bool            `json:"isError,omitempty"`
	Content    []ContentBlock  `json:"content,omitempty"`
	Extra      json.RawMessage `json:"-"`
}

// UnmarshalJSON keeps unknown fields available for future use while decoding
// the known surface. (json.RawMessage capture into Extra is implemented so a
// renderer can branch on block types this version does not model.)
func (c *ContentBlock) UnmarshalJSON(b []byte) error {
	type alias ContentBlock
	var a alias
	if err := json.Unmarshal(b, &a); err != nil {
		return err
	}
	*c = ContentBlock(a)
	// Preserve anything not covered by the alias fields for forward compat.
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err == nil {
		kept := map[string]json.RawMessage{}
		for k, v := range raw {
			switch k {
			case "type", "text", "name", "mediaType", "data", "callId", "toolCallId", "isError", "content":
			default:
				kept[k] = v
			}
		}
		if len(kept) > 0 {
			enc, err := json.Marshal(kept)
			if err == nil {
				c.Extra = enc
			}
		}
	}
	return nil
}

// TokenUsage records per-step token accounting (assistant/message.usage and
// usage chunks).
type TokenUsage struct {
	InputTokens  int `json:"inputTokens,omitempty"`
	OutputTokens int `json:"outputTokens,omitempty"`
	TotalTokens  int `json:"totalTokens,omitempty"`
	// Cache read/write tokens when the adapter reports them.
	CacheReadTokens  *int `json:"cacheReadTokens,omitempty"`
	CacheWriteTokens *int `json:"cacheWriteTokens,omitempty"`
}

// StreamChunk is the raw streaming protocol emitted by adapters.
type StreamChunk struct {
	Type            string          `json:"type"`
	Index           int             `json:"index,omitempty"`
	BlockType       string          `json:"blockType,omitempty"`
	Text            string          `json:"text,omitempty"`
	ID              string          `json:"id,omitempty"`
	Name            string          `json:"name,omitempty"`
	ArgumentsDelta  string          `json:"argumentsDelta,omitempty"`
	Block           json.RawMessage `json:"block,omitempty"`
	Usage           *TokenUsage     `json:"usage,omitempty"`
	Reason          string          `json:"reason,omitempty"`
	ReplayState     json.RawMessage `json:"replayState,omitempty"`
}

// Stream chunk types.
const (
	ChunkBlockStart       = "block-start"
	ChunkTextDelta        = "text-delta"
	ChunkReasoningDelta   = "reasoning-delta"
	ChunkToolCallDelta    = "tool-call-delta"
	ChunkBlockEnd         = "block-end"
	ChunkUsage            = "usage"
	ChunkFinish           = "finish"
)

// ToolResultMessage is the tool/result event's model-facing message.
type ToolResultMessage struct {
	Role    string          `json:"role"`
	Content []ContentBlock  `json:"content"`
	Source  json.RawMessage `json:"source,omitempty"`
}

// TodoItem is one todo-list entry (todo/write whole-list snapshot).
type TodoItem struct {
	Content string `json:"content"`
	Status  string `json:"status"` // pending | in_progress | completed
}

// Todo statuses.
const (
	TodoPending    = "pending"
	TodoInProgress = "in_progress"
	TodoCompleted  = "completed"
)

// ---------------------------------------------------------------------------
// Downlink frames — the two logical streams (events.mux / events.host).
// ---------------------------------------------------------------------------

// MuxFrame is a frame on /api/events.mux: raw session-event passthrough plus
// control/approval/question frames. The frame's method name is the payload
// `type` discriminant.
type MuxFrame struct {
	Type      string          `json:"type"`
	SessionID string          `json:"sessionId,omitempty"`
	Event     *SessionEvent   `json:"event,omitempty"`
	View      json.RawMessage `json:"view,omitempty"`
	LastSeq   int64           `json:"lastSeq,omitempty"`

	// approval/requested + approval/resolved
	ApprovalID string `json:"approvalId,omitempty"`
	ToolName   string `json:"toolName,omitempty"`
	CallID     string `json:"callId,omitempty"`
	Reason     string `json:"reason,omitempty"`
	Outcome    string `json:"outcome,omitempty"`

	// question/requested + question/resolved
	Questions     []AskUserQuestionItem `json:"questions,omitempty"`
	QuestionRpcID string                `json:"questionRpcId,omitempty"`

	// session/queue (transient inbox snapshot)
	Items []QueuedInboxItem `json:"items,omitempty"`

	// session/jobs
	Jobs []JobView `json:"jobs,omitempty"`

	// session/projection
	Key   string          `json:"key,omitempty"`
	Value json.RawMessage `json:"value,omitempty"`
	Seq   int64           `json:"seq,omitempty"`

	// stream/error
	Error *RpcError `json:"error,omitempty"`
}

// Mux frame types.
const (
	MuxSessionEvent    = "session/event"
	MuxSessionSub      = "session/subscribed"
	MuxApprovalReq     = "approval/requested"
	MuxApprovalRes     = "approval/resolved"
	MuxQuestionReq     = "question/requested"
	MuxQuestionRes     = "question/resolved"
	MuxSessionQueue    = "session/queue"
	MuxSessionJobs     = "session/jobs"
	MuxSessionProjection = "session/projection"
	MuxSessionProj     = "session/projection"
	MuxStreamError     = "stream/error"
)

// ApprovalOutcome values.
const (
	ApprovalAllowedOnce = "allowed-once"
	ApprovalRejected    = "rejected"
)

// AskUserQuestionItem mirrors @deepseek-ai/dsh-user-questions. The option
// shape follows the waveloom ask_user_question tool (id/label/description);
// wire fields are decoded permissively.
type AskUserQuestionItem struct {
	ID          string          `json:"id,omitempty"`
	Question    string          `json:"question"`
	Header      string          `json:"header,omitempty"`
	MultiSelect bool            `json:"multiSelect,omitempty"`
	Options     []QuestionOption `json:"options,omitempty"`
	Extra       json.RawMessage `json:"-"`
}

// QuestionOption is one answer choice.
type QuestionOption struct {
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
	Extra       json.RawMessage `json:"-"`
}

// QueuedInboxItem is one pending inbox occurrence in the session/queue
// snapshot.
type QueuedInboxItem struct {
	ID        string          `json:"id"`
	Placement string          `json:"placement"` // queued | steering | context
	Message   json.RawMessage `json:"message"`
}

// JobView is one background job row (session/jobs). Fields are permissive.
type JobView struct {
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	State string          `json:"state,omitempty"`
	Extra json.RawMessage `json:"-"`
}

// HostFrame is a frame on /api/events.host: session lifecycle, agent errors,
// and workspace mutations.
type HostFrame struct {
	Type      string          `json:"type"`
	SessionID string          `json:"sessionId,omitempty"`
	Blank     bool            `json:"blank,omitempty"`
	ParentSessionID string   `json:"parentSessionId,omitempty"`
	Origin    string          `json:"origin,omitempty"`
	Cwd       string          `json:"cwd,omitempty"`
	AgentPreset string        `json:"agentPreset,omitempty"`
	Running   bool            `json:"running,omitempty"`
	Message   string          `json:"message,omitempty"`

	// workspace frames
	Workspace     json.RawMessage   `json:"workspace,omitempty"`
	WorkspaceID   string            `json:"workspaceId,omitempty"`
	WorkspaceIDs  []string          `json:"workspaceIds,omitempty"`
	ArchivedIDs   []string          `json:"archivedSessionIds,omitempty"`

	// host/remote-event
	Event string          `json:"event,omitempty"`
	Args  []json.RawMessage `json:"args,omitempty"`

	Error *RpcError `json:"error,omitempty"`
}

// Host frame types.
const (
	HostSessionAdded   = "host/session-added"
	HostSessionRemoved = "host/session-removed"
	HostSessionStatus  = "host/session-status"
	HostAgentError     = "host/agent-error"
	HostWorkspaceChg   = "host/workspace-changed"
	HostWorkspaceRm    = "host/workspace-removed"
	HostWorkspaceOrder = "host/workspace-order-changed"
	HostArchivedChg    = "host/archived-sessions-changed"
	HostRemoteEvent    = "host/remote-event"
)
