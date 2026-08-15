// Package dsh implements the deepseek-harness wire protocol client.
//
// The protocol is a four-quadrant RPC model confirmed against
// deepseek-harness master (2026-08): unary calls ride HTTP POST /api/<method>,
// server-initiated frames ride two downlink-only WebSocket streams
// (/api/events.mux and /api/events.host), and answerable server frames are
// answered via POST /api/respond. Every logical message is one member of a
// four-name discriminated union keyed on "type".
package dsh

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// RpcId correlates a request with its response; the initiator mints it.
type RpcId string

// RpcError codes, mirroring RpcErrorDetailsMap in the upstream host api
// package. The details payload is opaque JSON; consumers decode what they
// need per code.
const (
	ErrBadRequest              = "bad-request"
	ErrCancelled               = "cancelled"
	ErrSessionNotFound         = "session-not-found"
	ErrModelUnavailable        = "model-unavailable"
	ErrSessionConflict         = "session-conflict"
	ErrInvalidTimeZone         = "invalid-time-zone"
	ErrWorkspaceAttachFailed   = "workspace-attach-failed"
	ErrWorkspaceNotFound       = "workspace-not-found"
	ErrWorkspaceInvalidPath    = "workspace-invalid-path"
	ErrWorkspaceNameConflict   = "workspace-name-conflict"
	ErrWorkspaceMoveInvalid    = "workspace-move-invalid"
	ErrDirectoryUnreadable     = "directory-unreadable"
	ErrDirectoryExists         = "directory-exists"
	ErrDirectoryCreateFailed   = "directory-create-failed"
	ErrDirectoryPickerUnavail  = "directory-picker-unavailable"
	ErrAgentPresetReadOnly     = "agent-preset-read-only"
	ErrAgentPresetLocked       = "agent-preset-locked"
	ErrAgentPresetConflict     = "agent-preset-conflict"
	ErrAgentPresetNotFound     = "agent-preset-not-found"
	ErrAgentPresetInvalid      = "agent-preset-invalid"
	ErrAgentBusy               = "agent-busy"
	ErrAttachmentError         = "attachment-error"
	ErrQueueItemNotFound       = "queue-item-not-found"
	ErrSteerUnavailable        = "steer-unavailable"
	ErrCommandError            = "command-error"
	ErrUnknownCommand          = "unknown-command"
	ErrSettingsRejected        = "settings-rejected"
	ErrSettingsNotExposed      = "settings-not-exposed"
	ErrSettingsConflict        = "settings-conflict"
	ErrCredentialRejected      = "credential-rejected"
	ErrModelDiscoveryFailed    = "model-discovery-failed"
	ErrTitleInvalid            = "title-invalid"
	ErrForkUnavailable         = "fork-unavailable"
	ErrSubagentParentUnavail   = "subagent-parent-unavailable"
	ErrSubagentNotFound        = "subagent-not-found"
	ErrSubagentCatalogDiag     = "subagent-catalog-diagnostic"
	ErrSubagentNotResumable    = "subagent-not-resumable"
	ErrSubagentUnauthorized    = "subagent-unauthorized"
	ErrSubagentDeliveryUnavail = "subagent-delivery-unavailable"
	ErrInternal                = "internal"
)

// RpcError is the failure branch of an RpcResult. It implements error so it
// can be returned directly from call sites.
type RpcError struct {
	Code    string          `json:"code"`
	Message string          `json:"message"`
	Details json.RawMessage `json:"details"`
}

func (e *RpcError) Error() string {
	if e == nil {
		return "<nil rpc error>"
	}
	if e.Details == nil || string(e.Details) == "null" {
		return fmt.Sprintf("dsh %s: %s", e.Code, e.Message)
	}
	return fmt.Sprintf("dsh %s: %s (%s)", e.Code, e.Message, string(e.Details))
}

// RpcResult is the result slot of every response: {ok:true,value} or
// {ok:false,error}.
type RpcResult struct {
	OK    bool            `json:"ok"`
	Value json.RawMessage `json:"value,omitempty"`
	Error *RpcError       `json:"error,omitempty"`
}

// ---- Wire full forms: the four-member discriminated union ----

// ClientRequest is a call initiated by the client. Wire carrier: the body of
// POST /api/<method>.
type ClientRequest struct {
	Type    string          `json:"type"` // always "client-request"
	RpcID   RpcId           `json:"rpcId"`
	Method  string          `json:"method"`
	Payload json.RawMessage `json:"payload"`
}

// ServerResponse answers a ClientRequest. Wire carrier: the HTTP response
// body of that POST; rpcId is echoed.
type ServerResponse struct {
	Type   string    `json:"type"` // always "server-response"
	RpcID  RpcId     `json:"rpcId"`
	Result RpcResult `json:"result"`
}

// ServerRequest is initiated by the server. Wire carrier: a frame on one of
// the two downlink streams. Answerable interactions (approval/question
// requested) carry a stable rpcId reused on replay; pure pushes use the id to
// identify that one push. Whether a response is expected is determined
// statically by method (a strict dichotomy).
type ServerRequest struct {
	Type    string          `json:"type"` // always "server-request"
	RpcID   RpcId           `json:"rpcId"`
	Method  string          `json:"method"`
	Payload json.RawMessage `json:"payload"`
}

// ClientResponse answers a ServerRequest. Wire carrier: the body of
// POST /api/respond; rpcId is echoed, never minted anew.
type ClientResponse struct {
	Type   string    `json:"type"` // always "client-response"
	RpcID  RpcId     `json:"rpcId"`
	Result RpcResult `json:"result"`
}

// RpcReceipt is the carrier receipt returned for a client-response: the HTTP
// response body of the respond POST.
type RpcReceipt struct {
	Accepted bool   `json:"accepted"`
	Reason   string `json:"reason,omitempty"`
}

// NewRpcID mints a fresh rpc id (UUID v4). The upstream host uses UUIDs; the
// value is opaque to both sides.
func NewRpcID() RpcId {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand failure is unrecoverable; fall back to a zero-padded id
		// rather than crashing the TUI.
		return RpcId(fmt.Sprintf("fallback-%d", len(b)))
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10
	return RpcId(fmt.Sprintf("%s-%s-%s-%s-%s",
		hex.EncodeToString(b[0:4]),
		hex.EncodeToString(b[4:6]),
		hex.EncodeToString(b[6:8]),
		hex.EncodeToString(b[8:10]),
		hex.EncodeToString(b[10:16])))
}

// DecodePayload unmarshals a ServerRequest payload into out.
func (r *ServerRequest) DecodePayload(out any) error {
	return json.Unmarshal(r.Payload, out)
}
