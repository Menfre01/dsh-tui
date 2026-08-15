package dsh

import (
	"context"
	"encoding/json"
)

// ---------------------------------------------------------------------------
// session.* RPC methods. Types mirror the zod schemas in
// packages/host/apiproxy/src/api/sessions.schema.ts.
// ---------------------------------------------------------------------------

// SessionSummary is one row of session.list.
type SessionSummary struct {
	SessionID      string          `json:"sessionId"`
	UpdatedAt      int64           `json:"updatedAt"`
	Running        bool            `json:"running"`
	Blank          bool            `json:"blank"`
	ParentSessionID string         `json:"parentSessionId,omitempty"`
	Origin         string          `json:"origin,omitempty"`
	Cwd            string          `json:"cwd,omitempty"`
	AgentPreset    string          `json:"agentPreset,omitempty"`
	Projections    jsonRaw         `json:"projections,omitempty"`
}

// SessionListValue is the session.list response.
type SessionListValue struct {
	Items []SessionSummary `json:"items"`
}

// ListSessions calls session.list.
func (c *Client) ListSessions(ctx context.Context) (*SessionListValue, error) {
	var out SessionListValue
	if err := c.Call(ctx, "session.list", map[string]any{}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// SessionCreateRequest is the session.create payload. At most one of
// WorkspaceID / Cwd may be set; SessionID resumes a persisted session.
type SessionCreateRequest struct {
	WorkspaceID string `json:"workspaceId,omitempty"`
	Cwd         string `json:"cwd,omitempty"`
	SessionID   string `json:"sessionId,omitempty"`
	AgentPreset string `json:"agentPreset,omitempty"`
}

// SessionCreateValue is the session.create response.
type SessionCreateValue struct {
	SessionID   string `json:"sessionId"`
	AgentPreset string `json:"agentPreset,omitempty"`
}

// CreateSession calls session.create.
func (c *Client) CreateSession(ctx context.Context, req SessionCreateRequest) (*SessionCreateValue, error) {
	var out SessionCreateValue
	if err := c.Call(ctx, "session.create", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// HistoryEntry is one session.history item: the event plus an optional
// host-computed tool view (render intent, never persisted).
type HistoryEntry struct {
	Event SessionEvent   `json:"event"`
	View  jsonRaw        `json:"view,omitempty"`
}

// SessionHistoryRequest pages backwards from the window tail.
type SessionHistoryRequest struct {
	SessionID   string `json:"sessionId"`
	BeforeSeq   *int64 `json:"beforeSeq,omitempty"`
	MaxMessages *int64 `json:"maxMessages,omitempty"`
}

// SessionHistoryValue is the session.history response. Projections ride the
// tail page only.
type SessionHistoryValue struct {
	Events      []HistoryEntry `json:"events"`
	HasMore     bool           `json:"hasMore"`
	Projections jsonRaw        `json:"projections,omitempty"`
}

// SessionHistory calls session.history.
func (c *Client) SessionHistory(ctx context.Context, req SessionHistoryRequest) (*SessionHistoryValue, error) {
	var out SessionHistoryValue
	if err := c.Call(ctx, "session.history", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// PromptContentPart is one session.prompt content block. The wire content is
// intentionally narrower than the merge-extensible durable core: text and
// raster images only.
type PromptContentPart struct {
	Type      string `json:"type"` // "text" | "image"
	Text      string `json:"text,omitempty"`
	MediaType string `json:"mediaType,omitempty"`
	Data      string `json:"data,omitempty"`
	Name      string `json:"name,omitempty"`
}

// PromptModes: queue enqueues for the next turn; steer edits a pending
// queued item immediately.
const (
	PromptModeQueue = "queue"
	PromptModeSteer = "steer"
)

// SessionPromptRequest is the session.prompt payload.
type SessionPromptRequest struct {
	SessionID     string              `json:"sessionId"`
	Mode          string              `json:"mode"`
	Content       []PromptContentPart `json:"content"`
	ClientTimeZone string             `json:"clientTimeZone,omitempty"`
}

// SessionPromptValue is the session.prompt response; command appears only
// when the prompt dispatched a slash command.
type SessionPromptValue struct {
	Accepted bool                   `json:"accepted"`
	Command  *SessionPromptCommand  `json:"command,omitempty"`
}

// SessionPromptCommand is the command slot of a prompt result.
type SessionPromptCommand struct {
	Kind string `json:"kind"` // "success"
	Text string `json:"text,omitempty"`
}

// Prompt calls session.prompt.
func (c *Client) Prompt(ctx context.Context, req SessionPromptRequest) (*SessionPromptValue, error) {
	var out SessionPromptValue
	if err := c.Call(ctx, "session.prompt", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Queue actions for session.updateQueue.
const (
	QueueActionEdit   = "edit"
	QueueActionRemove = "remove"
	QueueActionSteer  = "steer"
)

// UpdateQueueRequest edits/removes/steers one queued inbox item.
type UpdateQueueRequest struct {
	SessionID string          `json:"sessionId"`
	ItemID    string          `json:"itemId"`
	Action    UpdateQueueAction `json:"action"`
}

// UpdateQueueAction is the discriminated action of an updateQueue call.
type UpdateQueueAction struct {
	Kind    string          `json:"kind"`
	Content []ContentBlock  `json:"content,omitempty"`
}

// UpdateQueue calls session.updateQueue.
func (c *Client) UpdateQueue(ctx context.Context, req UpdateQueueRequest) error {
	var out struct {
		Accepted bool `json:"accepted"`
	}
	return c.Call(ctx, "session.updateQueue", req, &out)
}

// Cancel calls session.cancel (interrupts the running agent).
func (c *Client) Cancel(ctx context.Context, sessionID string) error {
	var out struct {
		Accepted bool `json:"accepted"`
	}
	return c.Call(ctx, "session.cancel", map[string]string{"sessionId": sessionID}, &out)
}

// Rename calls session.rename; returns the normalized title and its event seq.
func (c *Client) Rename(ctx context.Context, sessionID, title string) (string, int64, error) {
	var out struct {
		Title string `json:"title"`
		Seq   int64  `json:"seq"`
	}
	err := c.Call(ctx, "session.rename", map[string]string{
		"sessionId": sessionID,
		"title":     title,
	}, &out)
	return out.Title, out.Seq, err
}

// Fork calls session.fork; atSeq anchors the completed-turn cut.
func (c *Client) Fork(ctx context.Context, sessionID string, atSeq *int64) (string, error) {
	req := map[string]any{"sessionId": sessionID}
	if atSeq != nil {
		req["atSeq"] = *atSeq
	}
	var out struct {
		SessionID string `json:"sessionId"`
	}
	if err := c.Call(ctx, "session.fork", req, &out); err != nil {
		return "", err
	}
	return out.SessionID, nil
}

// ModelSelection identifies one provider/model route.
type ModelSelection struct {
	Provider        string `json:"provider"`
	Model           string `json:"model"`
	ReasoningEffort string `json:"reasoningEffort,omitempty"`
}

// SessionModelsValue is the session.models response.
type SessionModelsValue struct {
	Current  ModelSelection        `json:"current"`
	Routable bool                  `json:"routable"`
	Groups   []ModelProviderGroup  `json:"groups"`
	Failures []ModelCatalogFailure `json:"failures"`
}

// ModelProviderGroup is one successfully loaded provider group.
type ModelProviderGroup struct {
	ID     string              `json:"id"`
	Name   string              `json:"name"`
	Models []ModelCatalogModel `json:"models"`
}

// ModelCatalogModel is one advisory model entry inside a provider group.
type ModelCatalogModel struct {
	ID          string               `json:"id"`
	Name        string               `json:"name"`
	Description string               `json:"description,omitempty"`
	Reasoning   *ModelReasoning      `json:"reasoning,omitempty"`
}

// ModelReasoning carries adapter-owned reasoning efforts.
type ModelReasoning struct {
	Efforts       []ModelReasoningEffort `json:"efforts"`
	DefaultEffort string                 `json:"defaultEffort,omitempty"`
}

// ModelReasoningEffort is one reasoning effort.
type ModelReasoningEffort struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// ModelCatalogFailure is one provider-local catalog failure.
type ModelCatalogFailure struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Message string `json:"message"`
}

// SessionModels calls session.models.
func (c *Client) SessionModels(ctx context.Context, sessionID string) (*SessionModelsValue, error) {
	var out SessionModelsValue
	if err := c.Call(ctx, "session.models", map[string]string{"sessionId": sessionID}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// SelectModel calls session.selectModel.
func (c *Client) SelectModel(ctx context.Context, sessionID, provider, model, reasoningEffort string) (*ModelSelection, error) {
	req := map[string]any{
		"sessionId": sessionID,
		"provider":  provider,
		"model":     model,
	}
	if reasoningEffort != "" {
		req["reasoningEffort"] = reasoningEffort
	}
	var out struct {
		Selected ModelSelection `json:"selected"`
	}
	if err := c.Call(ctx, "session.selectModel", req, &out); err != nil {
		return nil, err
	}
	return &out.Selected, nil
}

// SessionSearchItem is one session.search result.
type SessionSearchItem struct {
	SessionID string `json:"sessionId"`
	Snippet   string `json:"snippet"`
}

// SessionSearchValue is the session.search response.
type SessionSearchValue struct {
	Items   []SessionSearchItem `json:"items"`
	HasMore bool                `json:"hasMore"`
}

// SearchSessions calls session.search.
func (c *Client) SearchSessions(ctx context.Context, query string) (*SessionSearchValue, error) {
	var out SessionSearchValue
	if err := c.Call(ctx, "session.search", map[string]string{"query": query}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// jsonRaw is a typed alias so API value structs can hold opaque blocks
// (projections, tool views) without importing encoding/json everywhere.
// json.RawMessage — not []byte — is the correct wire type: the decoder
// treats RawMessage as a raw copy, while plain []byte expects base64.
type jsonRaw = json.RawMessage
