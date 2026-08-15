package dsh

import (
	"context"
	"encoding/json"
)

// ---------------------------------------------------------------------------
// /api/respond 便捷方法 — 应答 answerable server-requests(approval/question)
//
// 应答结构(来自 upstream zod schemas):
//   approval:  {sessionId, approvalId, outcome: "allowed-once"|"rejected"}
//   question:  {sessionId, answer: {answers: [{id, selected[], custom?}]}}
// rpcId 必须回显服务端帧的 id(question/requested 帧的 rpcId 是问题稳定 id)。
// ---------------------------------------------------------------------------

// ApprovalOutcomes (与 events.go 常量一致,此处为文档对照)。
const (
	OutcomeAllowedOnce = "allowed-once"
	OutcomeRejected    = "rejected"
)

// RespondApproval 应答一个审批请求。allowed=true → allowed-once,否则 rejected。
func (c *Client) RespondApproval(ctx context.Context, rpcID, sessionID, approvalID string, allowed bool) error {
	outcome := OutcomeRejected
	if allowed {
		outcome = OutcomeAllowedOnce
	}
	value, err := json.Marshal(map[string]string{
		"sessionId":  sessionID,
		"approvalId": approvalID,
		"outcome":    outcome,
	})
	if err != nil {
		return &RpcError{Code: ErrInternal, Message: "marshal approval answer: " + err.Error(), Details: []byte("{}")}
	}
	return c.Respond(ctx, rpcID, RpcResult{OK: true, Value: value})
}

// QuestionAnswer 是单个问题的答案(question 应答批次中的一个条目)。
type QuestionAnswer struct {
	ID       string   `json:"id"`
	Selected []string `json:"selected"`
	Custom   string   `json:"custom,omitempty"`
}

// RespondQuestion 应答一次提问(一次 ask 的多题为一批,一起应答)。
// 空 answers 等价取消。
func (c *Client) RespondQuestion(ctx context.Context, rpcID, sessionID string, answers []QuestionAnswer) error {
	if answers == nil {
		answers = []QuestionAnswer{}
	}
	value, err := json.Marshal(map[string]any{
		"sessionId": sessionID,
		"answer": map[string]any{
			"answers": answers,
		},
	})
	if err != nil {
		return &RpcError{Code: ErrInternal, Message: "marshal question answer: " + err.Error(), Details: []byte("{}")}
	}
	return c.Respond(ctx, rpcID, RpcResult{OK: true, Value: value})
}

// RespondQuestionCancel 取消一次提问(dsh 协议:respond 返回 RPC 错误
// code "cancelled",宿主据此 claim cancelled 并收尾 ask;
// 空 answers 批次会被校验拒绝,不是取消方式)。
func (c *Client) RespondQuestionCancel(ctx context.Context, rpcID string) error {
	return c.Respond(ctx, rpcID, RpcResult{
		OK: false,
		Error: &RpcError{
			Code:    "cancelled",
			Message: "user cancelled ask_user_question",
			Details: json.RawMessage("{}"),
		},
	})
}
