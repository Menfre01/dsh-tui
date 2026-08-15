package dsh

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// readLines reads one JSON value per line from a fixture file.
func readLines(t *testing.T, name string) []string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	var lines []string
	sc := bufio.NewScanner(strings.NewReader(string(b)))
	sc.Buffer(make([]byte, 1024*1024), 4*1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line != "" {
			lines = append(lines, line)
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scan fixture %s: %v", name, err)
	}
	return lines
}

func decodeServerResponse(t *testing.T, raw string) ServerResponse {
	t.Helper()
	var sr ServerResponse
	if err := json.Unmarshal([]byte(raw), &sr); err != nil {
		t.Fatalf("decode server-response: %v\n%s", err, raw)
	}
	if sr.Type != "server-response" {
		t.Fatalf("type = %q, want server-response", sr.Type)
	}
	return sr
}

func TestDecodeUnaryResponses(t *testing.T) {
	lines := readLines(t, "unary_responses.jsonl")

	// 0: session.create success
	sr := decodeServerResponse(t, lines[0])
	if !sr.Result.OK {
		t.Fatalf("want ok result, got error %v", sr.Result.Error)
	}
	var created SessionCreateValue
	if err := json.Unmarshal(sr.Result.Value, &created); err != nil {
		t.Fatalf("decode session.create value: %v", err)
	}
	if created.SessionID != "session-17" || created.AgentPreset != "coding" {
		t.Fatalf("session.create value = %+v", created)
	}

	// 1: session.list success
	sr = decodeServerResponse(t, lines[1])
	var list SessionListValue
	if err := json.Unmarshal(sr.Result.Value, &list); err != nil {
		t.Fatalf("decode session.list value: %v", err)
	}
	if len(list.Items) != 2 {
		t.Fatalf("items = %d, want 2", len(list.Items))
	}
	first := list.Items[0]
	if first.SessionID != "session-17" || !first.Running || first.Cwd != "/work/demo" {
		t.Fatalf("item[0] = %+v", first)
	}
	if list.Items[1].Origin != "subagent" || !list.Items[1].Blank {
		t.Fatalf("item[1] = %+v", list.Items[1])
	}

	// 2: host.describe success
	sr = decodeServerResponse(t, lines[2])
	var desc HostDescribeValue
	if err := json.Unmarshal(sr.Result.Value, &desc); err != nil {
		t.Fatalf("decode host.describe value: %v", err)
	}
	if desc.Version != "0.1.0" || !desc.CanOpenPath || desc.AttachedSessions != 2 {
		t.Fatalf("describe = %+v", desc)
	}

	// 3: business error
	sr = decodeServerResponse(t, lines[3])
	if sr.Result.OK {
		t.Fatal("want failed result")
	}
	if sr.Result.Error == nil {
		t.Fatal("want error payload")
	}
	if sr.Result.Error.Code != ErrAgentBusy {
		t.Fatalf("code = %q, want %q", sr.Result.Error.Code, ErrAgentBusy)
	}
	if !strings.Contains(sr.Result.Error.Error(), "already has a live agent") {
		t.Fatalf("error text = %q", sr.Result.Error.Error())
	}

	// 4: boolean accept
	sr = decodeServerResponse(t, lines[4])
	var acc struct {
		Accepted bool `json:"accepted"`
	}
	if err := json.Unmarshal(sr.Result.Value, &acc); err != nil || !acc.Accepted {
		t.Fatalf("decode accept value: %v %+v", err, acc)
	}
}

func TestDecodeMuxFrames(t *testing.T) {
	lines := readLines(t, "mux_frames.jsonl")
	wantTypes := []string{
		MuxSessionEvent, MuxSessionEvent, MuxSessionEvent, MuxSessionEvent,
		MuxSessionEvent, MuxSessionEvent, MuxSessionEvent, MuxSessionEvent,
		MuxSessionEvent, MuxSessionEvent, MuxSessionEvent, MuxSessionEvent,
		MuxSessionEvent, MuxSessionEvent, MuxApprovalReq, MuxApprovalRes,
		MuxQuestionReq, MuxQuestionRes, MuxSessionQueue, MuxSessionSub,
		MuxStreamError,
	}
	if len(lines) != len(wantTypes) {
		t.Fatalf("fixture has %d frames, want %d", len(lines), len(wantTypes))
	}

	for i, raw := range lines {
		var frame ServerRequest
		if err := json.Unmarshal([]byte(raw), &frame); err != nil {
			t.Fatalf("line %d: decode server-request: %v", i, err)
		}
		if frame.Type != "server-request" || frame.RpcID == "" || frame.Method == "" {
			t.Fatalf("line %d: malformed envelope %+v", i, frame)
		}
		var mux MuxFrame
		if err := frame.DecodePayload(&mux); err != nil {
			t.Fatalf("line %d: decode mux payload: %v", i, err)
		}
		if mux.Type != wantTypes[i] {
			t.Fatalf("line %d: frame type = %q, want %q", i, mux.Type, wantTypes[i])
		}
		if mux.Type == MuxStreamError && mux.Error == nil {
			t.Fatalf("line %d: stream/error without error", i)
		}
		if mux.SessionID == "" && mux.Type != MuxStreamError {
			t.Fatalf("line %d: missing sessionId", i)
		}
		if mux.Type == MuxSessionEvent {
			if mux.Event == nil {
				t.Fatalf("line %d: session/event without event", i)
			}
		}
	}

	// Spot-check the interesting events by scanning the fixture once more with
	// full decoding of event data.
	var chunks []StreamChunk
	var toolCallName, toolResultCallID string
	var todoStatuses []string
	var approvalTool string
	var questionText string
	for _, raw := range lines {
		var frame ServerRequest
		_ = json.Unmarshal([]byte(raw), &frame)
		var mux MuxFrame
		_ = frame.DecodePayload(&mux)
		if mux.Type != MuxSessionEvent || mux.Event == nil {
			continue
		}
		switch mux.Event.Type {
		case EvAssistantChunk:
			var d struct {
				Chunk StreamChunk `json:"chunk"`
			}
			if err := json.Unmarshal(mux.Event.Data, &d); err != nil {
				t.Fatalf("decode chunk: %v", err)
			}
			chunks = append(chunks, d.Chunk)
		case EvToolCall:
			var d struct {
				Name      string `json:"name"`
				Arguments string `json:"arguments"`
			}
			if err := json.Unmarshal(mux.Event.Data, &d); err != nil {
				t.Fatalf("decode tool/call: %v", err)
			}
			toolCallName = d.Name
			if !strings.Contains(d.Arguments, "go test") {
				t.Fatalf("tool/call arguments = %q", d.Arguments)
			}
		case EvToolResult:
			var d struct {
				Message ToolResultMessage `json:"message"`
			}
			if err := json.Unmarshal(mux.Event.Data, &d); err != nil {
				t.Fatalf("decode tool/result: %v", err)
			}
			if len(d.Message.Content) == 0 {
				t.Fatalf("tool/result has no content")
			}
			toolResultCallID = d.Message.Content[0].CallID
		case EvTodoWrite:
			var d struct {
				Todos []TodoItem `json:"todos"`
			}
			if err := json.Unmarshal(mux.Event.Data, &d); err != nil {
				t.Fatalf("decode todo/write: %v", err)
			}
			for _, td := range d.Todos {
				todoStatuses = append(todoStatuses, td.Status)
			}
		case EvTurnEnd:
			var d struct {
				Reason TurnEndReason `json:"reason"`
			}
			if err := json.Unmarshal(mux.Event.Data, &d); err != nil {
				t.Fatalf("decode turn/end: %v", err)
			}
			if d.Reason.Kind != TurnEndCompleted {
				t.Fatalf("turn/end reason = %q", d.Reason.Kind)
			}
		}
	}

	if len(chunks) != 5 {
		t.Fatalf("chunks = %d, want 5", len(chunks))
	}
	if chunks[0].Type != ChunkBlockStart || chunks[1].Type != ChunkTextDelta || chunks[1].Text != "好的，我先" {
		t.Fatalf("chunk[0..1] = %+v %+v", chunks[0], chunks[1])
	}
	if chunks[2].Type != ChunkReasoningDelta || chunks[3].Type != ChunkToolCallDelta || chunks[3].ID != "call-9" {
		t.Fatalf("reasoning/tool-call chunks = %+v %+v", chunks[2], chunks[4])
	}
	if toolCallName != "bash" || toolResultCallID != "call-9" {
		t.Fatalf("tool call/result correlation broken: %q %q", toolCallName, toolResultCallID)
	}
	if len(todoStatuses) != 2 || todoStatuses[0] != TodoInProgress {
		t.Fatalf("todo statuses = %v", todoStatuses)
	}

	// approval + question frames
	for _, raw := range lines {
		var frame ServerRequest
		_ = json.Unmarshal([]byte(raw), &frame)
		var mux MuxFrame
		_ = frame.DecodePayload(&mux)
		switch mux.Type {
		case MuxApprovalReq:
			approvalTool = mux.ToolName
			if mux.ApprovalID != "apr-1" || mux.CallID != "call-9" {
				t.Fatalf("approval frame = %+v", mux)
			}
		case MuxApprovalRes:
			if mux.Outcome != ApprovalAllowedOnce {
				t.Fatalf("approval outcome = %q", mux.Outcome)
			}
		case MuxQuestionReq:
			if len(mux.Questions) != 1 {
				t.Fatalf("questions = %d", len(mux.Questions))
			}
			questionText = mux.Questions[0].Question
			if len(mux.Questions[0].Options) != 2 {
				t.Fatalf("question options = %d", len(mux.Questions[0].Options))
			}
		case MuxQuestionRes:
			if mux.QuestionRpcID != "aaaa-0017" {
				t.Fatalf("questionRpcId = %q", mux.QuestionRpcID)
			}
		case MuxSessionQueue:
			if len(mux.Items) != 1 || mux.Items[0].Placement != "queued" {
				t.Fatalf("queue frame = %+v", mux.Items)
			}
		case MuxSessionSub:
			if mux.LastSeq != 23 {
				t.Fatalf("subscribed lastSeq = %d", mux.LastSeq)
			}
		}
	}
	if approvalTool != "bash" || questionText == "" {
		t.Fatalf("approval/question spot checks failed: %q %q", approvalTool, questionText)
	}
}

func TestDecodeHostFrames(t *testing.T) {
	lines := readLines(t, "host_frames.jsonl")
	wantTypes := []string{
		HostSessionAdded, HostSessionStatus, HostSessionRemoved, HostAgentError,
		HostWorkspaceChg, HostWorkspaceRm, HostWorkspaceOrder, HostArchivedChg,
	}
	if len(lines) != len(wantTypes) {
		t.Fatalf("fixture has %d frames, want %d", len(lines), len(wantTypes))
	}
	for i, raw := range lines {
		var frame ServerRequest
		if err := json.Unmarshal([]byte(raw), &frame); err != nil {
			t.Fatalf("line %d: %v", i, err)
		}
		var host HostFrame
		if err := frame.DecodePayload(&host); err != nil {
			t.Fatalf("line %d: %v", i, err)
		}
		if host.Type != wantTypes[i] {
			t.Fatalf("line %d: type = %q, want %q", i, host.Type, wantTypes[i])
		}
	}
}

// TestClientCallE2E exercises Call against a fake host: it asserts the
// request envelope on the wire and the value/error paths.
func TestClientCallE2E(t *testing.T) {
	var gotMethod, gotType, gotHost string
	var gotRpcID string
	var gotPayload map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.URL.Path
		gotHost = r.Host
		var req ClientRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode client-request: %v", err)
		}
		gotType = req.Type
		gotRpcID = string(req.RpcID)
		_ = json.Unmarshal(req.Payload, &gotPayload)

		switch r.URL.Path {
		case "/api/host.describe":
			w.Write([]byte(`{"type":"server-response","rpcId":"` + string(req.RpcID) + `","result":{"ok":true,"value":{"version":"9.9","cwd":"/tmp","attachedSessions":0,"canOpenPath":false}}}`))
		case "/api/session.create":
			w.Write([]byte(`{"type":"server-response","rpcId":"` + string(req.RpcID) + `","result":{"ok":false,"error":{"code":"session-not-found","message":"gone","details":{"sessionId":"x"}}}}`))
		default:
			w.WriteHeader(404)
		}
	}))
	defer server.Close()

	client := NewClient(server.URL)

	desc, err := client.Describe(context.Background())
	if err != nil {
		t.Fatalf("Describe: %v", err)
	}
	if desc.Version != "9.9" || client.Version() != "9.9" {
		t.Fatalf("describe = %+v, cached = %q", desc, client.Version())
	}
	if gotType != "client-request" || gotMethod != "/api/host.describe" {
		t.Fatalf("envelope: type=%q method=%q", gotType, gotMethod)
	}
	if gotRpcID == "" || gotRpcID == "fallback-16" {
		t.Fatalf("rpcId = %q", gotRpcID)
	}
	if gotHost == "" {
		t.Fatal("Host header not sent")
	}

	_, err = client.CreateSession(context.Background(), SessionCreateRequest{Cwd: "/tmp/x"})
	var rpcErr *RpcError
	if !errors.As(err, &rpcErr) {
		t.Fatalf("want *RpcError, got %T: %v", err, err)
	}
	if rpcErr.Code != ErrSessionNotFound {
		t.Fatalf("code = %q, want %q", rpcErr.Code, ErrSessionNotFound)
	}
}

// TestRespondE2E exercises /api/respond incl. the not-accepted receipt.
func TestRespondE2E(t *testing.T) {
	var gotEnvelope ClientResponse
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/respond" {
			t.Errorf("path = %q", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&gotEnvelope); err != nil {
			t.Errorf("decode client-response: %v", err)
		}
		switch gotEnvelope.RpcID {
		case "rpc-ok":
			w.Write([]byte(`{"accepted":true}`))
		default:
			w.Write([]byte(`{"accepted":false,"reason":"not-pending"}`))
		}
	}))
	defer server.Close()

	client := NewClient(server.URL)
	ctx := context.Background()

	if err := client.Respond(ctx, "rpc-ok", RpcResult{
		OK:    true,
		Value: json.RawMessage(`{"outcome":"allowed-once"}`),
	}); err != nil {
		t.Fatalf("Respond ok: %v", err)
	}
	if gotEnvelope.Type != "client-response" || gotEnvelope.RpcID != "rpc-ok" {
		t.Fatalf("envelope = %+v", gotEnvelope)
	}

	err := client.Respond(ctx, "rpc-stale", RpcResult{OK: true, Value: json.RawMessage(`{}`)})
	if err == nil || !strings.Contains(err.Error(), "not accepted") {
		t.Fatalf("stale respond err = %v", err)
	}
}

func TestNewRpcIDFormat(t *testing.T) {
	id := string(NewRpcID())
	if len(id) != 36 {
		t.Fatalf("rpc id = %q", id)
	}
	if id[14] != '4' {
		t.Fatalf("rpc id version nibble = %q", id[14])
	}
}
