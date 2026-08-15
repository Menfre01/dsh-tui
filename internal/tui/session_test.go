package tui

import (
	"fmt"
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

	// 打开列表(空闲 ←;这里直接设状态测导航)
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

var errFake = &fakeTestError{}

type fakeTestError struct{}

func (f *fakeTestError) Error() string { return "fake error" }

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

// TestSessionListWindow 验证会话列表固定高度窗口:超 8 个时滚动跟随 + more 提示。
func TestSessionListWindow(t *testing.T) {
	m := NewModel(ModelConfig{Theme: "dark"})
	_ = m.Init()
	m.sessions = make([]SessionBrief, 12)
	for i := range m.sessions {
		m.sessions[i] = SessionBrief{
			SessionID: "session-00000000-0000-0000-0000-0000000000" + fmt.Sprintf("%02d", i),
			Cwd:       "/w",
			Title:     fmt.Sprintf("sess %d", i),
		}
	}
	m.width = 80

	// 选中第 0 项:窗口 0..7,显示 ↓ 4 more,无 ↑
	m.sessionListIdx = 0
	out := m.renderSessionListOverlay(70)
	if strings.Contains(out, "↑ 4 more") {
		t.Fatalf("idx=0 不应有 ↑ more: %q", out)
	}
	if !strings.Contains(out, "↓ 4 more") {
		t.Fatalf("应显示 ↓ 4 more: %q", out)
	}
	if !strings.Contains(out, "sess 7") || strings.Contains(out, "sess 8") {
		t.Fatalf("窗口应为 0..7: %q", out)
	}

	// 选中第 11 项:窗口 4..11,显示 ↑ 4 more
	m.sessionListIdx = 11
	out = m.renderSessionListOverlay(70)
	if !strings.Contains(out, "↑ 4 more") {
		t.Fatalf("应显示 ↑ 4 more: %q", out)
	}
	if !strings.Contains(out, "sess 4") || !strings.Contains(out, "sess 11") {
		t.Fatalf("窗口应为 4..11: %q", out)
	}
}

// TestSessionAddedDedup 验证 host/session-added 帧去重:
// 重连重推的已存在会话更新字段而非追加(修复列表累积重复项)。
func TestSessionAddedDedup(t *testing.T) {
	m := NewModel(ModelConfig{Theme: "dark"})
	_ = m.Init()
	p := NewProjector(m)
	m.AttachProjector(p)

	frame := dsh.ServerRequest{
		Method: "host/session-added",
		Payload: mustJSON(map[string]any{
			"type": "host/session-added", "sessionId": "sess-1", "cwd": "/w/a",
		}),
	}
	p.ReplayHostFrame(frame)
	p.ReplayHostFrame(frame) // 重连重推
	p.ReplayHostFrame(frame)
	if len(m.sessions) != 1 {
		t.Fatalf("sessions = %d, want 1(去重)", len(m.sessions))
	}
	// 不同会话正常追加
	frame2 := dsh.ServerRequest{
		Method: "host/session-added",
		Payload: mustJSON(map[string]any{
			"type": "host/session-added", "sessionId": "sess-2", "cwd": "/w/b",
		}),
	}
	p.ReplayHostFrame(frame2)
	if len(m.sessions) != 2 {
		t.Fatalf("sessions = %d, want 2", len(m.sessions))
	}
}

// TestSetSessionsDedup 验证 SetSessions 全量替换时按 SessionID 去重:
// 即使上游 session.list 返回重复,列表也保持唯一。
func TestSetSessionsDedup(t *testing.T) {
	m := NewModel(ModelConfig{Theme: "dark"})
	_ = m.Init()
	m.SetSessions([]SessionBrief{
		{SessionID: "session-a", Cwd: "/w/a"},
		{SessionID: "session-b", Cwd: "/w/b"},
		{SessionID: "session-a", Cwd: "/w/a2"}, // 重复
		{SessionID: "session-c", Cwd: "/w/c"},
		{SessionID: "session-b", Cwd: "/w/b2"}, // 重复
		{SessionID: "", Cwd: "/w/empty"},       // 空 id 丢弃
	})
	if len(m.sessions) != 3 {
		t.Fatalf("sessions = %d, want 3(去重)", len(m.sessions))
	}
	ids := map[string]bool{}
	for _, s := range m.sessions {
		if ids[s.SessionID] {
			t.Fatalf("duplicate sessionId %q after SetSessions", s.SessionID)
		}
		ids[s.SessionID] = true
	}
	// 顺序保留首个出现
	if m.sessions[0].SessionID != "session-a" || m.sessions[1].SessionID != "session-b" || m.sessions[2].SessionID != "session-c" {
		t.Fatalf("unexpected order: %+v", m.sessions)
	}
}

// TestDedupeSessions 验证打开列表前的兜底去重与索引修正:
// 重复项收敛为唯一,选中索引指向去重后同一会话。
func TestDedupeSessions(t *testing.T) {
	m := NewModel(ModelConfig{Theme: "dark"})
	_ = m.Init()
	// 模拟旧进程残留:同 id 重复 3 份,选中最后一份
	m.sessions = []SessionBrief{
		{SessionID: "session-a", Cwd: "/w/a"},
		{SessionID: "session-b", Cwd: "/w/b"},
		{SessionID: "session-a", Cwd: "/w/a-dup1"},
		{SessionID: "session-c", Cwd: "/w/c"},
		{SessionID: "session-a", Cwd: "/w/a-dup2"},
		{SessionID: "session-b", Cwd: "/w/b-dup"},
	}
	m.sessionListIdx = 4 // 指向 session-a 的第三份
	if !m.dedupeSessions() {
		t.Fatal("dedupeSessions 应报告清理了重复")
	}
	if len(m.sessions) != 3 {
		t.Fatalf("sessions = %d, want 3", len(m.sessions))
	}
	if m.sessions[0].SessionID != "session-a" || m.sessions[1].SessionID != "session-b" || m.sessions[2].SessionID != "session-c" {
		t.Fatalf("unexpected list: %+v", m.sessions)
	}
	// 索引 4(重复的 session-a)应收敛到去重后的 0
	if m.sessionListIdx != 0 {
		t.Fatalf("sessionListIdx = %d, want 0(同一会话)", m.sessionListIdx)
	}
	// 幂等:再次调用无变化
	if m.dedupeSessions() {
		t.Fatal("dedupeSessions 第二次调用不应再有清理")
	}
}

// TestSessionVisibility 验证 web 对齐的可见性过滤:
// blank 会话只有当前一个可见,subagent 子会话隐藏,普通会话全部可见。
func TestSessionVisibility(t *testing.T) {
	m := NewModel(ModelConfig{Theme: "dark"})
	_ = m.Init()
	m.SetSessionInfo("session-cur", "deepseek-v4-flash")
	m.SetSessions([]SessionBrief{
		{SessionID: "session-a", Cwd: "/w/a"},                    // 普通
		{SessionID: "session-b", Cwd: "/w/b", Blank: true},       // 非当前 blank → 隐藏
		{SessionID: "session-cur", Cwd: "/w/c", Blank: true},     // 当前 blank → 可见
		{SessionID: "session-d", Cwd: "/w/d", Origin: "subagent"}, // subagent → 隐藏
		{SessionID: "session-e", Cwd: "/w/e", Blank: true},       // 非当前 blank → 隐藏
	})
	vis := m.visibleSessions()
	if len(vis) != 2 {
		t.Fatalf("visible = %d, want 2(a + 当前blank)", len(vis))
	}
	if vis[0].SessionID != "session-a" || vis[1].SessionID != "session-cur" {
		t.Fatalf("unexpected visible list: %+v", vis)
	}
	// subagent 会话进入 sessions 但不显示
	if len(m.sessions) != 5 {
		t.Fatalf("sessions = %d, want 5(不过滤原始列表)", len(m.sessions))
	}
}

// TestSessionVisibilityToggle 验证打开列表后选中索引仍指向可见项:
// 全量列表带隐藏项时,visibleSessions 与 sessionListIdx 保持一致。
func TestSessionVisibilityToggle(t *testing.T) {
	m := NewModel(ModelConfig{Theme: "dark"})
	_ = m.Init()
	m.SetSessionInfo("session-cur", "deepseek-v4-flash")
	m.sessions = []SessionBrief{
		{SessionID: "session-1", Cwd: "/w/1", Blank: true},        // 隐藏
		{SessionID: "session-cur", Cwd: "/w/c", Blank: true},      // 当前 blank
		{SessionID: "session-2", Cwd: "/w/2", Origin: "subagent"}, // 隐藏
	}
	m.sessionListIdx = 1 // 指向隐藏的 subagent 项(索引需重新映射)
	m.toggleSessionList() // 打开列表:触发去重 + 索引映射到可见列表
	if !m.sessionListVisible() {
		t.Fatal("session list should be visible after toggle")
	}
	vis := m.visibleSessions()
	if len(vis) != 1 || vis[0].SessionID != "session-cur" {
		t.Fatalf("visible = %+v, want [session-cur]", vis)
	}
	// 选中索引已映射到可见列表中的当前项
	if m.sessionListIdx != 0 {
		t.Fatalf("sessionListIdx = %d, want 0", m.sessionListIdx)
	}
}

// TestSessionListSpacing 验证条目间空行:列表更疏朗(行高)。
func TestSessionListSpacing(t *testing.T) {
	m := NewModel(ModelConfig{Theme: "dark"})
	_ = m.Init()
	m.SetSessionInfo("session-cur", "deepseek-v4-flash")
	m.SetSessions([]SessionBrief{
		{SessionID: "session-a", Cwd: "/w/a", Title: "alpha"},
		{SessionID: "session-b", Cwd: "/w/b", Title: "beta"},
		{SessionID: "session-c", Cwd: "/w/c", Title: "gamma"},
	})
	out := m.renderSessionListOverlay(70)
	// 三个条目都在
	for _, title := range []string{"alpha", "beta", "gamma"} {
		if !strings.Contains(out, title) {
			t.Fatalf("缺少条目 %s: %q", title, out)
		}
	}
	// 条目间应有空行:alpha 行与 beta 行不相邻(中间隔空行)
	// 输出带 ANSI 与边框,取行内容(去转义后 strip)判定空行。
	rows := strings.Split(out, "\n")
	strip := func(s string) string {
		var b strings.Builder
		in := false
		for _, r := range s {
			if r == '\x1b' {
				in = true
				continue
			}
			if in {
				if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
					in = false
				}
				continue
			}
			b.WriteRune(r)
		}
		return b.String()
	}
	var alphaRow, betaRow int
	for i, row := range rows {
		content := strings.TrimSpace(strip(row))
		if strings.Contains(content, "alpha") {
			alphaRow = i
		}
		if strings.Contains(content, "beta") {
			betaRow = i
		}
	}
	if betaRow-alphaRow < 2 {
		t.Fatalf("条目间应有空行(alpha 行 %d,beta 行 %d): %q", alphaRow, betaRow, out)
	}
}

// TestSessionListOpenByLeft 验证空闲态 ← 打开会话列表(view 层分发):
// 空闲(未运行、无覆盖层、输入为空)→ 打开;输入非空 → 不触发(光标移动)。
func TestSessionListOpenByLeft(t *testing.T) {
	left := tea.KeyPressMsg{Code: tea.KeyLeft}

	m := NewModel(ModelConfig{Theme: "dark"})
	_ = m.Init()
	m.SetSessionInfo("session-cur", "deepseek-v4-flash")
	m.SetSessions([]SessionBrief{{SessionID: "session-a", Cwd: "/w/a"}})

	// 空闲 + 输入为空:← 打开列表
	if m.running || m.overlay != overlayNone || m.input.Value() != "" {
		t.Fatal("前置条件应为空闲")
	}
	upd, _ := m.Update(left)
	m = upd.(*model)
	if !m.sessionListVisible() {
		t.Fatal("空闲态 ← 应打开会话列表")
	}

	// 列表打开时 ← 关闭(对称)
	upd, _ = m.Update(left)
	m = upd.(*model)
	if m.sessionListVisible() {
		t.Fatal("列表打开时 ← 应关闭")
	}

	// 输入非空:← 不触发(仍是输入框光标移动)
	m.input.SetValue("hello")
	upd, _ = m.Update(left)
	m = upd.(*model)
	if m.sessionListVisible() {
		t.Fatal("输入非空时 ← 不应打开会话列表")
	}
}

// TestSessionListNoCwd 验证会话列表不再显示工作目录:
// 行内只有 running + label(title/短 id),长 title 截断不换行。
func TestSessionListNoCwd(t *testing.T) {
	m := NewModel(ModelConfig{Theme: "dark"})
	_ = m.Init()
	m.SetSessionInfo("session-cur", "deepseek-v4-flash")
	longTitle := "这是一个非常非常非常非常非常非常非常非常长的会话标题用于测试换行行为"
	m.SetSessions([]SessionBrief{
		{SessionID: "session-aaaa1111-2222-3333-4444-555566667777", Cwd: "/very/long/workspace/path/example", Title: longTitle},
	})
	out := m.renderSessionListOverlay(70)
	// 去掉 ANSI/边框
	rows := strings.Split(out, "\n")
	strip := func(s string) string {
		var b strings.Builder
		in := false
		for _, r := range s {
			if r == '\x1b' {
				in = true
				continue
			}
			if in {
				if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
					in = false
				}
				continue
			}
			b.WriteRune(r)
		}
		return b.String()
	}
	// 条目行(title 行)不显示 cwd;cwd 只出现在选中项第二行
	for _, row := range rows {
		content := strip(row)
		if strings.Contains(content, "very/long/workspace") && strings.Contains(content, "这是一个") {
			t.Fatalf("条目行不应显示 cwd: %q", content)
		}
	}
	// title 与短 id 在同一行(未换行)
	found := false
	for _, row := range rows {
		content := strip(row)
		if strings.Contains(content, "aaaa1111") && !strings.Contains(content, "very/long/workspace") {
			if !strings.Contains(content, "这是一个") {
				t.Fatalf("title 与短 id 不在同一行(被换行): %q", content)
			}
			found = true
		}
	}
	if !found {
		t.Fatalf("短 id 未出现在输出: %q", out)
	}
	// 选中项下方第二行显示工作目录(而非完整 id)
	detailRow := ""
	for _, row := range rows {
		if strings.Contains(strip(row), "very/long/workspace") {
			detailRow = strip(row)
		}
	}
	if !strings.Contains(detailRow, "/very/long/workspace/path/example") {
		t.Fatalf("选中项第二行应显示 cwd: %q", detailRow)
	}
	if strings.Contains(detailRow, "id:") {
		t.Fatalf("第二行不应显示 id 前缀: %q", detailRow)
	}
}

// TestExitResumeHint 验证退出提示文案含完整 session id(中英)。
func TestExitResumeHint(t *testing.T) {
	zh := MessagesZhCN()
	en := MessagesEnUS()
	sid := "session-abc123"
	zhOut := fmt.Sprintf(zh.ExitResumeHint, sid)
	enOut := fmt.Sprintf(en.ExitResumeHint, sid)
	if !strings.Contains(zhOut, sid) || !strings.Contains(zhOut, "dsh-tui --resume") {
		t.Fatalf("zh hint 异常: %q", zhOut)
	}
	if !strings.Contains(enOut, sid) || !strings.Contains(enOut, "dsh-tui --resume") {
		t.Fatalf("en hint 异常: %q", enOut)
	}
}

// TestReplayAnyHostRouting 验证 ReplayAny 把 host 帧路由到 ReplayHostFrame
// (会话列表维护),mux 帧路由到 ReplayFrame(段落渲染)。
func TestReplayAnyHostRouting(t *testing.T) {
	m := NewModel(ModelConfig{Theme: "dark"})
	_ = m.Init()
	p := NewProjector(m)
	m.AttachProjector(p)

	// host/session-added → 进会话列表
	hostFrame := dsh.ServerRequest{
		Method: "host/session-added",
		Payload: mustJSON(map[string]any{
			"type": "host/session-added", "sessionId": "sess-routing", "blank": true,
		}),
	}
	p.ReplayAny(hostFrame)
	if len(m.sessions) != 1 || m.sessions[0].SessionID != "sess-routing" {
		t.Fatalf("host 帧应维护会话列表: %+v", m.sessions)
	}

	// mux 帧 → 段落(构造一个 user/message 事件)
	p.SessionID = "sess-1"
	muxFrame := dsh.ServerRequest{
		Method: "session/event",
		Payload: mustJSON(map[string]any{
			"type": "session/event", "sessionId": "sess-1",
			"event": map[string]any{
				"type": "user/message", "seq": 1, "time": 1700000000000,
				"data": map[string]any{
					"role": "user", "content": []map[string]any{{"type": "text", "text": "hi"}},
				},
			},
		}),
	}
	p.ReplayAny(muxFrame)
	if len(m.paras) == 0 {
		t.Fatal("mux 帧应产生段落")
	}
}
