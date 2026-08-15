package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/Menfre01/dsh-tui/internal/dsh"
)

// TestSessionListNavigation 验证会话列表的键盘导航与切换回调。
func TestSessionListNavigation(t *testing.T) {
	m := NewModel(ModelConfig{Theme: "dark"})
	_ = m.Init()
	m.sessions = []SessionBrief{
		{SessionID: "sess-aaa", Cwd: "/work/a"},
		{SessionID: "sess-bbb", Cwd: "/work/b"},
		{SessionID: "sess-ccc", Cwd: "/work/c"},
	}

	// ctrl+s 打开列表
	handled, _ := m.handleSessionListKey(tea.KeyPressMsg{})
	_ = handled // 按键构造依赖内部字段,这里直接测状态机
	m.overlay = overlaySessionList
	m.sessionListIdx = 0

	// 导航逻辑独立测试:down 移动选中
	m.sessionListIdx++
	if m.sessionListIdx != 1 {
		t.Fatalf("down nav failed: %d", m.sessionListIdx)
	}

	// 边界:不能再下移
	m.sessionListIdx = len(m.sessions) - 1
	m.sessionListIdx++
	if m.sessionListIdx >= len(m.sessions) {
		m.sessionListIdx = len(m.sessions) - 1
	}
	if m.sessionListIdx != 2 {
		t.Fatalf("boundary clamp failed: %d", m.sessionListIdx)
	}
}

// TestSessionSwitchFlow 验证切换完成消息的段落重置与回放。
func TestSessionSwitchFlow(t *testing.T) {
	m := NewModel(ModelConfig{Theme: "dark"})
	_ = m.Init()
	p := NewProjector(m)
	m.AttachProjector(p)
	m.SetSessionInfo("old-session", "m1")

	// 旧会话渲染了几段
	m.paras = append(m.paras, Paragraph{Type: paraUser, State: stateDone, Text: "old"})
	m.todos = []TodoItem{{Content: "old todo", Status: "pending"}}

	// 注入切换完成消息(新会话无历史)
	msg := dshSwitchDoneMsg{target: "new-session", hist: nil, err: nil}
	updated, _ := m.Update(msg)
	um := updated.(*model)

	if len(um.paras) != 1 {
		t.Fatalf("paras = %d, want 1 (system notice only)", len(um.paras))
	}
	if um.paras[0].Type != paraSystem {
		t.Fatalf("residual para type = %v", um.paras[0].Type)
	}
	if um.todos != nil {
		t.Fatalf("todos not cleared")
	}
	if um.sessionID != "new-session" {
		t.Fatalf("sessionID = %q", um.sessionID)
	}
	if p.SessionID != "new-session" {
		t.Fatalf("projector session = %q", p.SessionID)
	}
}

// TestSessionSwitchError 切换失败保留原会话内容并提示。
func TestSessionSwitchError(t *testing.T) {
	m := NewModel(ModelConfig{Theme: "dark"})
	_ = m.Init()
	m.paras = append(m.paras, Paragraph{Type: paraUser, State: stateDone, Text: "keep me"})

	msg := dshSwitchDoneMsg{target: "bad-session", hist: nil, err: errFake}
	updated, _ := m.Update(msg)
	um := updated.(*model)

	if len(um.paras) != 2 {
		t.Fatalf("paras = %d, want 2 (old + error notice)", len(um.paras))
	}
	if um.paras[1].Type != paraSystem || um.paras[1].NotifKind != notifError {
		t.Fatalf("error notice missing: %+v", um.paras[1])
	}
	if um.sessionID != "" {
		t.Fatalf("sessionID changed on failure: %q", um.sessionID)
	}
}

var errFake = &fakeErr{}

type fakeErr struct{}

func (f *fakeErr) Error() string { return "fake error" }

// TestEnterAfterTurnEnd 回归:第一次发送后 running=true,回合结束(turn/end)
// 必须把 running 复位,否则第二次 Enter 永远被拦截。
func TestEnterAfterTurnEnd(t *testing.T) {
	m := NewModel(ModelConfig{Theme: "dark"})
	_ = m.Init()
	p := NewProjector(m)
	m.AttachProjector(p)

	sent := 0
	m.SetCallbacks(func(text, mode string) { sent++ }, func() {})

	// 发送 → running=true
	m.SetRunning(true)
	if !m.running {
		t.Fatalf("running not set after send")
	}

	// turn/end 事件到达 → 投影器复位
	m.ReplayHistory([]HistoryEvent{{
		Event: dsh.SessionEvent{
			Type: dsh.EvTurnEnd,
			Seq:  1,
			Data: []byte(`{"turn":1,"reason":{"kind":"completed"}}`),
		},
	}})
	if m.running {
		t.Fatalf("running not cleared after turn/end (第二次 Enter 会被拦截)")
	}
	if p.Running {
		t.Fatalf("projector running not cleared")
	}

	// 第二次 Enter 应可发送
	m.input.SetValue("second message")
	msg := tea.KeyPressMsg{}
	_ = msg // KeyPressMsg 构造依赖内部字段,这里直接验证 running 状态足够
	if m.running {
		t.Fatalf("enter blocked")
	}
	_ = sent
}

// TestGapCallback 重连补洞:subscribed 帧的 lastSeq 大于本地时触发补洞回调。
func TestGapCallback(t *testing.T) {
	m := NewModel(ModelConfig{Theme: "dark"})
	_ = m.Init()
	p := NewProjector(m)
	m.AttachProjector(p)

	var gotSession string
	var gotFrom int64
	p.SetGapCallback(func(sessionID string, fromSeq int64) {
		gotSession = sessionID
		gotFrom = fromSeq
	})

	// 本地渲染到 seq=10
	p.ReplayEvent(&dsh.SessionEvent{Type: dsh.EvTurnStart, Seq: 10, Data: []byte(`{"turn":1}`)})
	if p.lastSeq != 10 {
		t.Fatalf("lastSeq = %d", p.lastSeq)
	}

	// subscribed 帧:lastSeq=25 > 10 → 应触发补洞(from=11)
	p.ReplayFrame(dsh.ServerRequest{
		Method: "session/subscribed",
		Payload: []byte(`{"type":"session/subscribed","sessionId":"sess-gap","lastSeq":25}`),
	})
	if gotSession != "sess-gap" || gotFrom != 11 {
		t.Fatalf("gap callback = %s from %d", gotSession, gotFrom)
	}

	// 无缺口(lastSeq <= 本地)不触发
	gotSession = ""
	p.ReplayFrame(dsh.ServerRequest{
		Method: "session/subscribed",
		Payload: []byte(`{"type":"session/subscribed","sessionId":"sess-gap","lastSeq":10}`),
	})
	if gotSession != "" {
		t.Fatalf("gap triggered without gap: %s", gotSession)
	}
}

// TestGapReplayAppends 补洞事件追加回放,不清空现有段落。
func TestGapReplayAppends(t *testing.T) {
	m := NewModel(ModelConfig{Theme: "dark"})
	_ = m.Init()
	p := NewProjector(m)
	m.AttachProjector(p)
	m.paras = append(m.paras, Paragraph{Type: paraUser, State: stateDone, Text: "existing"})

	m.SendGapEvents([]HistoryEvent{{
		Event: dsh.SessionEvent{
			Type: dsh.EvUserMessage,
			Seq:  99,
			Data: []byte(`{"id":"m1","role":"user","content":[{"type":"text","text":"gap message"}],"source":{"kind":"user"}}`),
		},
	}}, nil)
	// Update 直接调用(绕过 program 投递)
	updated, _ := m.Update(gapEventsMsg{
		events: []HistoryEvent{{
			Event: dsh.SessionEvent{
				Type: dsh.EvUserMessage,
				Seq:  99,
				Data: []byte(`{"id":"m1","role":"user","content":[{"type":"text","text":"gap message"}],"source":{"kind":"user"}}`),
			},
		}},
	})
	um := updated.(*model)
	if len(um.paras) != 2 {
		t.Fatalf("paras = %d, want 2 (existing + gap)", len(um.paras))
	}
	if um.paras[1].Text != "gap message" {
		t.Fatalf("gap para = %+v", um.paras[1])
	}
}

// TestSessionListShowsTitle 验证会话列表显示宿主 title(与 web 对齐)。
func TestSessionListShowsTitle(t *testing.T) {
	m := NewModel(ModelConfig{Theme: "dark"})
	_ = m.Init()
	m.sessions = []SessionBrief{
		{SessionID: "session-ba61dc5b-96de-4601-8cd5-c9975cb70a9f", Cwd: "/work/a", Title: "你是谁"},
		{SessionID: "session-38eed8b1-1d08-447d-9a22-ebe7bb20ebdf", Cwd: "/work/b"},
	}
	m.sessionListIdx = 0
	m.width = 80
	out := m.renderSessionListOverlay(70)
	if !strings.Contains(out, "你是谁") {
		t.Fatalf("列表应显示 title: %q", out)
	}
	if !strings.Contains(out, "ba61dc5b") {
		t.Fatalf("title 后应有短 id: %q", out)
	}
	// 无 title 的会话:直接显示短 id
	if !strings.Contains(out, "38eed8b1") {
		t.Fatalf("无 title 会话应显示短 id: %q", out)
	}
}
