package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/Menfre01/dsh-tui/internal/dsh"
)

// TestEnterSendsWhileRunning 验证 running 时 Enter 也发送(宿主 queue 模式入队)。
func TestEnterSendsWhileRunning(t *testing.T) {
	m := NewModel(ModelConfig{Theme: "dark"})
	_ = m.Init()
	sent := []string{}
	m.onSend = func(s string, _ string) { sent = append(sent, s) }
	m.running = true

	m.input.SetValue("queued message")
	m = pressKey(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if len(sent) != 1 || sent[0] != "queued message" {
		t.Fatalf("running 时发送 = %v, want [queued message]", sent)
	}
	if got := m.input.Value(); got != "" {
		t.Fatalf("发送后输入框应清空: %q", got)
	}
}

// TestQueueDockRender 验证排队消息渲染(仅 queued placement)。
func TestQueueDockRender(t *testing.T) {
	m := NewModel(ModelConfig{Theme: "dark"})
	_ = m.Init()
	msg := `{"role":"user","content":[{"type":"text","text":"第一条排队消息"}]}`
	m.queueItems = []dsh.QueuedInboxItem{
		{ID: "1", Placement: "queued", Message: []byte(msg)},
		{ID: "2", Placement: "steering", Message: []byte(`{"content":[{"type":"text","text":"steer 不显示"}]}`)},
	}
	out := m.renderQueueDock(80)
	if !strings.Contains(out, "1 条排队") && !strings.Contains(out, "1 queued") {
		t.Fatalf("排队指示缺失: %q", out)
	}
	if !strings.Contains(out, "第一条排队消息") {
		t.Fatalf("排队内容缺失: %q", out)
	}
	if strings.Contains(out, "steer 不显示") {
		t.Fatal("steering placement 不应显示在 QueueDock")
	}
}

// TestQueueItemPreview 验证预览提取与截断。
func TestQueueItemPreview(t *testing.T) {
	raw := []byte(`{"content":[{"type":"text","text":"hello"},{"type":"text","text":"world"}]}`)
	if got := queueItemPreview(raw); got != "hello" {
		t.Fatalf("preview = %q, want hello", got)
	}
	long := []byte(`{"content":[{"type":"text","text":"` + strings.Repeat("x", 100) + `"}]}`)
	got := queueItemPreview(long)
	if len(got) > 30 {
		t.Fatalf("preview 未截断: %d", len(got))
	}
	empty := []byte(`{"content":[]}`)
	if got := queueItemPreview(empty); got != "" {
		t.Fatalf("空消息 preview = %q", got)
	}
}

// TestBusyEnterModePolicy 验证发送模式与 dsh web 对齐:
// 空闲→queue;繁忙+Enter→busyEnter 配置;繁忙+Ctrl+Enter→反转。
func TestBusyEnterModePolicy(t *testing.T) {
	cases := []struct {
		name      string
		running   bool
		busyEnter string
		key       tea.KeyPressMsg
		wantMode  string
	}{
		{"idle", false, "queue", tea.KeyPressMsg{Code: tea.KeyEnter}, "queue"},
		{"busy-default-queue", true, "queue", tea.KeyPressMsg{Code: tea.KeyEnter}, "queue"},
		{"busy-steer-config", true, "steer", tea.KeyPressMsg{Code: tea.KeyEnter}, "steer"},
		{"busy-ctrl-enter-reverses-queue", true, "queue", tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModCtrl}, "steer"},
		{"busy-ctrl-enter-reverses-steer", true, "steer", tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModCtrl}, "queue"},
		{"busy-ctrl-enter-idle-keeps-queue", false, "steer", tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModCtrl}, "queue"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := NewModel(ModelConfig{Theme: "dark"})
			_ = m.Init()
			var gotMode string
			m.onSend = func(_ string, mode string) { gotMode = mode }
			m.running = c.running
			m.SetBusyEnter(c.busyEnter)
			m.input.SetValue("hello")
			m.Update(c.key)
			if gotMode != c.wantMode {
				t.Fatalf("mode = %q, want %q", gotMode, c.wantMode)
			}
		})
	}
}

// TestSwitchSyncsRunning 验证切换会话后 running 从会话列表快照同步
// (Esc 中断可用性依赖;resume/切换时宿主 running 状态需落地)。
func TestSwitchSyncsRunning(t *testing.T) {
	m := NewModel(ModelConfig{Theme: "dark"})
	_ = m.Init()
	p := NewProjector(m)
	m.AttachProjector(p)
	m.sessions = []SessionBrief{
		{SessionID: "sess-running", Running: true},
		{SessionID: "sess-idle", Running: false},
	}

	// 切到 running 会话
	m.Update(dshSwitchDoneMsg{target: "sess-running", hist: nil, err: nil})
	if !m.running {
		t.Fatal("切换到 running 会话后 m.running 应为 true")
	}
	// 切到 idle 会话
	m.Update(dshSwitchDoneMsg{target: "sess-idle", hist: nil, err: nil})
	if m.running {
		t.Fatal("切换到 idle 会话后 m.running 应为 false")
	}
}

// TestModelSelectCarriesEffort 验证选模型时回调携带模型默认 effort(web 语义)。
func TestModelSelectCarriesEffort(t *testing.T) {
	m := NewModel(ModelConfig{Theme: "dark"})
	_ = m.Init()
	var gotProvider, gotModel, gotEffort string
	m.onSelectModel = func(provider, model, effort string) {
		gotProvider, gotModel, gotEffort = provider, model, effort
	}
	m.modelPickerItems = []ModelChoice{
		{Provider: "deepseek-official", Model: "deepseek-v4-flash", Name: "flash", Effort: "high"},
		{Provider: "deepseek-official", Model: "deepseek-v4-max", Name: "max", Effort: ""},
	}
	m.buildModelPickerList()

	// 选中第 1 个 → 回调带 effort=high
	m.modelPickerList.Select(0)
	m.handleModelPickerKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if gotProvider != "deepseek-official" || gotModel != "deepseek-v4-flash" {
		t.Fatalf("select = %s/%s", gotProvider, gotModel)
	}
	if gotEffort != "high" {
		t.Fatalf("effort = %q, want high", gotEffort)
	}
	// 选中第 2 个 → effort 空
	m.modelPickerList.Select(1)
	m.handleModelPickerKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if gotEffort != "" {
		t.Fatalf("effort = %q, want 空", gotEffort)
	}
}

// TestEffortPanel 验证 effort 面板:e 进入、↑↓ 选择、Enter 带档位确认、Esc 返回。
func TestEffortPanel(t *testing.T) {
	m := NewModel(ModelConfig{Theme: "dark"})
	_ = m.Init()
	var gotModel, gotEffort string
	m.onSelectModel = func(_, model, effort string) {
		gotModel, gotEffort = model, effort
	}
	m.modelPickerItems = []ModelChoice{
		{
			Provider: "p", Model: "m1", Name: "m1", Effort: "high",
			Efforts: []EffortChoice{{ID: "low", Name: "Low"}, {ID: "high", Name: "High"}, {ID: "max", Name: "Max"}},
		},
	}
	m.buildModelPickerList()
	m.overlay = overlayModelPicker

	// e 进入 effort 面板
	m.handleModelPickerKey(tea.KeyPressMsg{Code: 'e'})
	if !m.effortPickerMode {
		t.Fatal("e 应进入 effort 面板")
	}
	if m.effortCount() != 3 {
		t.Fatalf("effortCount = %d, want 3", m.effortCount())
	}
	// ↓ 选第 2 档(high),Enter 确认
	m.handleModelPickerKey(tea.KeyPressMsg{Code: tea.KeyDown})
	m.handleModelPickerKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if gotModel != "m1" || gotEffort != "high" {
		t.Fatalf("select = %s/%s, want m1/high", gotModel, gotEffort)
	}
	if m.effortPickerMode || m.overlay != overlayNone {
		t.Fatal("确认后应关闭")
	}

	// 再进面板,Esc 返回模型列表
	m.overlay = overlayModelPicker
	m.buildModelPickerList()
	m.handleModelPickerKey(tea.KeyPressMsg{Code: 'e'})
	m.handleModelPickerKey(tea.KeyPressMsg{Code: tea.KeyEsc})
	if m.effortPickerMode {
		t.Fatal("Esc 应返回模型列表")
	}
	if m.overlay != overlayModelPicker {
		t.Fatal("Esc 后仍在模型选择器")
	}
}

// TestModelSelectedUpdatesEffort 验证模型切换成功后 HUD effort 跟随
// (modelSelectedMsg 携带宿主确认的 effort)。
func TestModelSelectedUpdatesEffort(t *testing.T) {
	m := NewModel(ModelConfig{Theme: "dark"})
	_ = m.Init()
	upd, _ := m.Update(modelSelectedMsg{model: "deepseek-v4-max", effort: "max"})
	m = upd.(*model)
	if m.hudModel != "deepseek-v4-max" {
		t.Fatalf("hudModel = %q", m.hudModel)
	}
	if m.hudThinkingEffort != "max" {
		t.Fatalf("hudThinkingEffort = %q, want max", m.hudThinkingEffort)
	}
	// 无 effort 的确认不清空旧值(仅更新)
	m.Update(modelSelectedMsg{model: "deepseek-v4-flash"})
	if m.hudThinkingEffort != "max" {
		t.Fatalf("无 effort 确认不应清空: %q", m.hudThinkingEffort)
	}
}
