package dsh

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestRespondApprovalWire 验证审批应答的 wire 编码(对照 approvals.schema.ts)。
func TestRespondApprovalWire(t *testing.T) {
	var got ClientResponse
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/respond" {
			t.Errorf("path = %q", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("decode: %v", err)
		}
		w.Write([]byte(`{"accepted":true}`))
	}))
	defer server.Close()

	client := NewClient(server.URL)
	if err := client.RespondApproval(context.Background(), "rpc-a1", "session-1", "apr-7", true); err != nil {
		t.Fatalf("RespondApproval: %v", err)
	}
	if got.RpcID != "rpc-a1" || got.Type != "client-response" {
		t.Fatalf("envelope = %+v", got)
	}
	var value map[string]string
	if err := json.Unmarshal(got.Result.Value, &value); err != nil {
		t.Fatalf("value: %v", err)
	}
	if value["sessionId"] != "session-1" || value["approvalId"] != "apr-7" || value["outcome"] != "allowed-once" {
		t.Fatalf("approval value = %+v", value)
	}

	// rejected 分支
	if err := client.RespondApproval(context.Background(), "rpc-a2", "session-1", "apr-8", false); err != nil {
		t.Fatalf("RespondApproval(reject): %v", err)
	}
	var value2 map[string]string
	if err := json.Unmarshal(got.Result.Value, &value2); err != nil {
		t.Fatalf("value2: %v", err)
	}
	if value2["outcome"] != "rejected" {
		t.Fatalf("reject outcome = %q", value2["outcome"])
	}
}

// TestRespondQuestionWire 验证提问应答的 wire 编码(对照 questions.schema.ts)。
func TestRespondQuestionWire(t *testing.T) {
	var got ClientResponse
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.Write([]byte(`{"accepted":true}`))
	}))
	defer server.Close()

	client := NewClient(server.URL)
	answers := []QuestionAnswer{
		{ID: "q1", Selected: []string{"单元测试"}},
		{ID: "q2", Selected: []string{}, Custom: ""},
	}
	if err := client.RespondQuestion(context.Background(), "rpc-q1", "session-1", answers); err != nil {
		t.Fatalf("RespondQuestion: %v", err)
	}
	if got.RpcID != "rpc-q1" {
		t.Fatalf("rpcId = %q", got.RpcID)
	}
	var value struct {
		SessionID string `json:"sessionId"`
		Answer    struct {
			Answers []QuestionAnswer `json:"answers"`
		} `json:"answer"`
	}
	if err := json.Unmarshal(got.Result.Value, &value); err != nil {
		t.Fatalf("value: %v", err)
	}
	if value.SessionID != "session-1" || len(value.Answer.Answers) != 2 {
		t.Fatalf("question value = %+v", value)
	}
	if value.Answer.Answers[0].ID != "q1" || value.Answer.Answers[0].Selected[0] != "单元测试" {
		t.Fatalf("answer[0] = %+v", value.Answer.Answers[0])
	}
}

// TestRespondQuestionEmpty 空批次(取消)也能编码。
func TestRespondQuestionEmpty(t *testing.T) {
	var got ClientResponse
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.Write([]byte(`{"accepted":true}`))
	}))
	defer server.Close()

	client := NewClient(server.URL)
	if err := client.RespondQuestion(context.Background(), "rpc-q2", "session-1", nil); err != nil {
		t.Fatalf("RespondQuestion(nil): %v", err)
	}
	var value struct {
		Answer struct {
			Answers []QuestionAnswer `json:"answers"`
		} `json:"answer"`
	}
	if err := json.Unmarshal(got.Result.Value, &value); err != nil {
		t.Fatalf("value: %v", err)
	}
	if value.Answer.Answers == nil || len(value.Answer.Answers) != 0 {
		t.Fatalf("empty answers = %+v", value.Answer.Answers)
	}
}
