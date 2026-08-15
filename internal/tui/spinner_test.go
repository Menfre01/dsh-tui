package tui

import (
	"testing"
	"time"

	"charm.land/bubbles/v2/spinner"
)

// TestSpinnerTickAdvance 验证 model.Update 消费 spinner.TickMsg 后:
//  1. spinner 帧推进(View 输出变化);
//  2. 返回下一帧 tick cmd(动画持续,而非一次性);
//  3. 全部 6 个 spinner 同步推进(统一路由)。
func TestSpinnerTickAdvance(t *testing.T) {
	m := NewModel(ModelConfig{Theme: "dark"})
	_ = m.Init()

	before := []string{
		m.spinner.View(),
		m.spAsst.View(),
		m.spThought.View(),
		m.spTool.View(),
		m.spSubagent.View(),
		m.spTodo.View(),
	}

	updated, cmd := m.Update(spinner.TickMsg{Time: time.Now()})
	um, ok := updated.(*model)
	if !ok {
		t.Fatalf("Update returned %T, want *model", updated)
	}
	if cmd == nil {
		t.Fatal("TickMsg handler must return the next tick cmd to keep animating")
	}

	after := []string{
		um.spinner.View(),
		um.spAsst.View(),
		um.spThought.View(),
		um.spTool.View(),
		um.spSubagent.View(),
		um.spTodo.View(),
	}
	for i := range before {
		if after[i] == before[i] {
			t.Fatalf("spinner #%d frame did not advance: %q == %q", i, before[i], after[i])
		}
	}
}

// TestSpinnerTickAdvanceUpdateTick 验证 updating 状态下 updateTick 随 TickMsg 递增
// (更新横幅动画帧推进),非 updating 状态不递增。
func TestSpinnerTickAdvanceUpdateTick(t *testing.T) {
	m := NewModel(ModelConfig{Theme: "dark"})
	_ = m.Init()

	// 非 updating:帧计数不动
	updated, _ := m.Update(spinner.TickMsg{Time: time.Now()})
	if got := updated.(*model).updateTick; got != 0 {
		t.Fatalf("updateTick advanced while not updating: %d", got)
	}

	// updating:每 tick 递增
	m.updating = true
	updated, _ = m.Update(spinner.TickMsg{Time: time.Now()})
	if got := updated.(*model).updateTick; got != 1 {
		t.Fatalf("updateTick = %d, want 1", got)
	}
}
