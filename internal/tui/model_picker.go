package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
)

// ---------------------------------------------------------------------------
// model_picker.go — 模型选择器(移植自 waveloom tui_command.go,适配 dsh)
//
// 交互: Ctrl+M 打开模型选择器;↑/↓ 导航;Enter 切换;Esc 关闭。
// 数据: main 拉取 session.models 后经 SetModels 注入(按 provider 分组扁平化)。
// ---------------------------------------------------------------------------

// EffortChoice 是一个思考档位(宿主 ModelReasoningEffort)。
type EffortChoice struct {
	ID   string
	Name string
}

// ModelChoice 是模型选择器条目(dsh session.models 扁平化结果)。
type ModelChoice struct {
	Provider string // provider id(如 "deepseek-official")
	Model    string // 模型 id(如 "deepseek-v4-flash")
	Name     string // 显示名
	Effort   string // 模型默认 reasoningEffort(web 语义:选模型自动带默认档)
	Efforts  []EffortChoice // 模型支持的档位(e 键进入 effort 面板)
}

// modelPickerItem 是模型列表项,实现 list.DefaultItem 接口。
type modelPickerItem struct {
	choice ModelChoice
}

func (i modelPickerItem) Title() string       { return i.choice.Name }
func (i modelPickerItem) Description() string { return "" }
func (i modelPickerItem) FilterValue() string { return i.choice.Name }

// SetModels 注入模型目录(扁平化后的 provider 分组列表)。
func (m *model) SetModels(models []ModelChoice) {
	m.modelPickerItems = models
}

// SetModelSelectCallback 注入模型切换回调(main 调 session.selectModel)。
func (m *model) SetModelSelectCallback(fn func(provider, model, effort string)) {
	m.onSelectModel = fn
}

// toggleModelPicker 打开/关闭模型选择器。
func (m *model) toggleModelPicker() {
	if m.overlay == overlayModelPicker {
		m.overlay = overlayNone
		m.input.Focus()
		return
	}
	if m.onFetchModels != nil {
		m.onFetchModels() // 异步拉取最新模型目录(不阻塞)
	}
	m.buildModelPickerList()
	m.overlay = overlayModelPicker
	m.input.Blur()
}

// buildModelPickerList 从 modelPickerItems 构建模型选择列表。
// 当前使用的模型(m.hudModel)在列表中高亮。
func (m *model) buildModelPickerList() {
	items := make([]list.Item, len(m.modelPickerItems))
	selectedIdx := 0
	for i, mc := range m.modelPickerItems {
		items[i] = modelPickerItem{choice: mc}
		if mc.Model == m.hudModel {
			selectedIdx = i
		}
	}

	height := len(items)
	if height > 5 {
		height = 5
	}
	if height < 1 {
		height = 1
	}

	delegate := list.NewDefaultDelegate()
	delegate.ShowDescription = false
	delegate.SetSpacing(0)
	delegate.Styles = listItemStyles()
	m.modelPickerDelegate = &delegate

	l := list.New(items, delegate, 0, height)
	l.SetShowTitle(false)
	l.SetShowPagination(false)
	l.SetShowStatusBar(false)
	l.SetShowFilter(false)
	l.SetShowHelp(false)
	l.KeyMap.Quit = key.NewBinding()
	l.KeyMap.ForceQuit = key.NewBinding()
	if selectedIdx < height {
		l.Select(selectedIdx)
	}
	m.modelPickerList = l
}

// handleModelPickerKey 处理模型选择器按键(含 effort 面板)。返回 (handled, cmd)。
func (m *model) handleModelPickerKey(msg tea.KeyPressMsg) (bool, tea.Cmd) {
	keyStr := msg.String()
	if m.effortPickerMode {
		switch keyStr {
		case "up":
			if m.effortPickerList.Index() <= 0 {
				return true, nil
			}
			var cmd tea.Cmd
			m.effortPickerList, cmd = m.effortPickerList.Update(msg)
			return true, cmd
		case "down":
			if m.effortPickerList.Index() >= m.effortCount()-1 {
				return true, nil
			}
			var cmd tea.Cmd
			m.effortPickerList, cmd = m.effortPickerList.Update(msg)
			return true, cmd
		case "enter":
			mi := m.effortPickerModelIdx
			if mi >= 0 && mi < len(m.modelPickerItems) {
				mc := m.modelPickerItems[mi]
				effort := ""
				if idx := m.effortPickerList.Index(); idx >= 0 && idx < len(mc.Efforts) {
					effort = mc.Efforts[idx].ID
				}
				m.hudModel = mc.Model
				if m.onSelectModel != nil {
					m.onSelectModel(mc.Provider, mc.Model, effort)
				}
			}
			m.effortPickerMode = false
			m.overlay = overlayNone
			m.input.Focus()
			return true, nil
		case "esc":
			// 返回模型列表
			m.effortPickerMode = false
			return true, nil
		}
		return true, nil
	}
	switch keyStr {
	case "up":
		if m.modelPickerList.Index() <= 0 {
			return true, nil
		}
		var cmd tea.Cmd
		m.modelPickerList, cmd = m.modelPickerList.Update(msg)
		return true, cmd
	case "down":
		if m.modelPickerList.Index() >= len(m.modelPickerItems)-1 {
			return true, nil
		}
		var cmd tea.Cmd
		m.modelPickerList, cmd = m.modelPickerList.Update(msg)
		return true, cmd
	case "e":
		// 进入当前模型的 effort 面板(web 的 Effort pane)
		idx := m.modelPickerList.Index()
		if idx >= 0 && idx < len(m.modelPickerItems) && len(m.modelPickerItems[idx].Efforts) > 0 {
			m.effortPickerModelIdx = idx
			m.buildEffortPickerList()
			m.effortPickerMode = true
		}
		return true, nil
	case "enter":
		idx := m.modelPickerList.Index()
		if idx >= 0 && idx < len(m.modelPickerItems) {
			mc := m.modelPickerItems[idx]
			m.hudModel = mc.Model
			if m.onSelectModel != nil {
				m.onSelectModel(mc.Provider, mc.Model, mc.Effort)
			}
		}
		m.overlay = overlayNone
		m.input.Focus()
		return true, nil
	case "esc":
		m.overlay = overlayNone
		m.input.Focus()
		return true, nil
	}
	return false, nil
}

// effortCount 返回当前 effort 面板的条目数。
func (m *model) effortCount() int {
	mi := m.effortPickerModelIdx
	if mi >= 0 && mi < len(m.modelPickerItems) {
		return len(m.modelPickerItems[mi].Efforts)
	}
	return 0
}

// buildEffortPickerList 从当前模型的 Efforts 构建档位列表。
func (m *model) buildEffortPickerList() {
	items := []list.Item{}
	mi := m.effortPickerModelIdx
	if mi >= 0 && mi < len(m.modelPickerItems) {
		for _, e := range m.modelPickerItems[mi].Efforts {
			items = append(items, effortPickerItem{id: e.ID, name: e.Name})
		}
	}
	height := len(items)
	if height > 5 {
		height = 5
	}
	if height < 1 {
		height = 1
	}
	delegate := list.NewDefaultDelegate()
	delegate.ShowDescription = false
	delegate.SetSpacing(0)
	delegate.Styles = listItemStyles()
	l := list.New(items, delegate, 0, height)
	l.SetShowTitle(false)
	l.SetShowPagination(false)
	l.SetShowStatusBar(false)
	l.SetShowFilter(false)
	l.SetShowHelp(false)
	l.KeyMap.Quit = key.NewBinding()
	l.KeyMap.ForceQuit = key.NewBinding()
	m.effortPickerList = l
}

// effortPickerItem 是档位列表项。
type effortPickerItem struct {
	id   string
	name string
}

func (i effortPickerItem) Title() string       { return i.name }
func (i effortPickerItem) Description() string { return "" }
func (i effortPickerItem) FilterValue() string { return i.name }

// normalizeModelName 去掉模型名的 provider 前缀用于显示(如
// "deepseek-official/deepseek-v4-flash" → "deepseek-v4-flash")。
func normalizeModelName(s string) string {
	if idx := strings.Index(s, "/"); idx >= 0 {
		return s[idx+1:]
	}
	return s
}
