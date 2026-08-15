package tui

import (
	"encoding/json"
	"testing"

	"github.com/Menfre01/dsh-tui/internal/dsh"
)

// toolCallEv 构造一条 tool/call 事件。
func toolCallEv(callID, name string) *dsh.SessionEvent {
	data, _ := json.Marshal(map[string]any{
		"callId":    callID,
		"name":      name,
		"arguments": `{"command": "echo hi"}`,
	})
	return &dsh.SessionEvent{Type: dsh.EvToolCall, Data: data}
}

// toolResultEvToolCallID 按真实宿主结构构造 tool/result:
// content[0] 是 {type:"tool-result", toolCallId:..., content:[{type:"text",...}]}。
func toolResultEvToolCallID(callID, out string, isErr bool) *dsh.SessionEvent {
	data, _ := json.Marshal(map[string]any{
		"message": map[string]any{
			"role":    "user",
			"content": []any{
				map[string]any{
					"type":        "tool-result",
					"toolCallId":  callID,
					"isError":     isErr,
					"content":     []any{map[string]any{"type": "text", "text": out}},
				},
			},
		},
	})
	return &dsh.SessionEvent{Type: dsh.EvToolResult, Data: data}
}

// TestToolResultToolCallIDField 验证 tool/result 用 toolCallId 字段名时
// 仍能匹配到 tool/call 建立的段落并切换到完成态(协议漂移回归)。
func TestToolResultToolCallIDField(t *testing.T) {
	m := NewModel(ModelConfig{Theme: "dark"})
	_ = m.Init()
	p := NewProjector(m)
	m.AttachProjector(p)

	const callID = "call_00_realHost"
	p.ReplayEvent(toolCallEv(callID, "bash"))
	if len(m.paras) != 1 || m.paras[0].State != stateStreaming {
		t.Fatalf("tool/call 应建立 streaming 段落, got %d paras state=%v", len(m.paras), m.paras[0].State)
	}

	p.ReplayEvent(toolResultEvToolCallID(callID, "done", false))
	if len(m.paras) != 1 {
		t.Fatalf("段落数量变化: %d", len(m.paras))
	}
	para := m.paras[0]
	if para.State != stateCollapsed {
		t.Fatalf("tool/result 后状态 = %v, want stateCollapsed (pending 泄漏)", para.State)
	}
	if para.ToolResult != "done" {
		t.Fatalf("ToolResult = %q, want done", para.ToolResult)
	}
}

// TestToolResultLegacyCallIDField 验证旧字段名 callId 仍兼容。
func TestToolResultLegacyCallIDField(t *testing.T) {
	m := NewModel(ModelConfig{Theme: "dark"})
	_ = m.Init()
	p := NewProjector(m)
	m.AttachProjector(p)

	const callID = "call_00_legacy"
	p.ReplayEvent(toolCallEv(callID, "bash"))
	p.ReplayEvent(toolResultEvToolCallID(callID, "ok", false))
	if m.paras[0].State != stateCollapsed {
		t.Fatalf("legacy callId 路径状态 = %v, want stateCollapsed", m.paras[0].State)
	}
}

// TestToolResultIsErrorFlag 验证 isError 标志把段落置为错误态而非完成态。
func TestToolResultIsErrorFlag(t *testing.T) {
	m := NewModel(ModelConfig{Theme: "dark"})
	_ = m.Init()
	p := NewProjector(m)
	m.AttachProjector(p)

	const callID = "call_00_err"
	p.ReplayEvent(toolCallEv(callID, "bash"))
	p.ReplayEvent(toolResultEvToolCallID(callID, "boom", true))
	if m.paras[0].State != stateError {
		t.Fatalf("isError 结果状态 = %v, want stateError", m.paras[0].State)
	}
}
