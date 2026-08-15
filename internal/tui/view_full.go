package tui

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// ---------------------------------------------------------------------------
// view_full.go — 从 waveloom tui.go 移植的完整 View 布局
//
// 复刻 waveloom 的 TUI 视觉:logo 字标、header(session/cwd)、
// 段落视口 + 欢迎引导、Footer HUD(spinner/模型/ctx 进度条/cache/tok/
// Loop/M/elap/bal)。适配点:
//   - cm.SessionID() → m.sessionID(main 注入)
//   - todoState.Snapshot() → m.todos(投影器维护)
//   - renderCost 简化(dsh 无费用概念,恒 "--")
//   - 审批/提问覆盖层用 approval.go 的 dsh 驱动版本
// ---------------------------------------------------------------------------

// updateSpinnerFrames 更新检查动画帧。
var updateSpinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// asciiArt 是 "DSH-TUI" 的 header 字标。
var asciiArt = []string{
	`██████╗  ███████╗ ██╗  ██╗         ████████╗ ██╗   ██╗ ██╗`,
	`██╔══██╗ ██╔════╝ ██║  ██║         ╚══██╔══╝ ██║   ██║ ██║`,
	`██║  ██║ ███████╗ ███████║ ██████╗    ██║   ██║   ██║ ██║`,
	`██║  ██║ ╚════██║ ██╔══██║            ██║   ██║   ██║ ██║`,
	`██████╔╝ ███████║ ██║  ██║            ██║   ╚██████╔╝ ██║`,
	`╚═════╝  ╚══════╝ ╚═╝  ╚═╝            ╚═╝    ╚═════╝  ╚═╝`,
}

// viewportCtx 返回段落渲染所需的上下文(spinners + Glamour renderer)。
func (m *model) viewportCtx() ViewportCtx {
	contentWidth := max(m.width-4, 20)
	return ViewportCtx{
		Asst:     m.spAsst,
		Thought:  m.spThought,
		Tool:     m.spTool,
		Subagent: m.spSubagent,
		Glamour:  m.glamourRenderer,
		Width:    contentWidth,
		LC:       m.msg(),
		CWD:      m.cwd,
	}
}

// paletteFor 按主题模式选择色板(dark/light/colorblind 变体)。
func paletteFor(mode string) palette {
	switch mode {
	case "light":
		return lightPalette
	case "darkcolorblind":
		return darkColorBlindPalette
	case "lightcolorblind":
		return lightColorBlindPalette
	default: // dark / auto / 未知
		return darkPalette
	}
}

// View 渲染完整界面(复刻 waveloom 布局)。
func (m *model) View() tea.View {
	if m.height < 10 {
		// 终端太小,无法正常布局,显示提示信息
		msg := m.msg().TerminalTooSmall
		padded := lipgloss.NewStyle().
			Width(m.width).
			Height(m.height).
			Align(lipgloss.Center, lipgloss.Center).
			Foreground(colorMuted).
			Render(msg)
		v := tea.NewView(padded)
		v.AltScreen = true
		return v
	}

	contentWidth := max(m.width-4, 20)

	// 1. 渲染段落内容(全量,后续根据滚动偏移裁剪可见区域)
	ctx := m.viewportCtx()
	allLines, _ := buildViewportContent(m.paras, ctx, m.focusIndex, 0)

	// 将 logo 注入 viewport 顶部(可滚动,内容增多后会被挤出窗口)
	logoLines := m.renderLogoLines(contentWidth)
	allLines = append(logoLines, allLines...)

	// 2. 渲染覆盖层
	var overlayContent string
	switch m.overlay {
	case overlayPermission:
		if m.pendingApproval != nil {
			overlayContent = m.renderApprovalOverlay(contentWidth)
		}
	case overlayQuestion:
		if m.pendingQuestion != nil {
			overlayContent = m.renderQuestionOverlay(contentWidth)
		}
	case overlayThemePicker:
		overlayContent = m.renderThemePickerOverlay(contentWidth)
	case overlayModelPicker:
		overlayContent = m.renderModelPickerOverlay(contentWidth)
	case overlayLocalePicker:
		overlayContent = m.renderLocalePickerOverlay(contentWidth)
	case overlayProviderPicker:
		overlayContent = m.renderProviderPickerOverlay(contentWidth)
	case overlayPlanEnter:
		overlayContent = m.renderPlanEnterOverlay(contentWidth)
	case overlayPlanExit:
		overlayContent = m.renderPlanExitOverlay(contentWidth)
	case overlayHelp:
		overlayContent = m.renderHelpOverlay(contentWidth)
	case overlayRewindSelect:
		overlayContent = m.renderRewindSelectOverlay(contentWidth)
	case overlayRewindConfirm:
		overlayContent = m.renderRewindConfirmOverlay(contentWidth)
	case overlaySessionList:
		overlayContent = m.renderSessionListOverlay(contentWidth)
	}
	var pickerContent string
	if m.pickerVisible {
		pickerContent = m.renderPickerDropdown(contentWidth)
	}

	overlayLines := 0
	if overlayContent != "" {
		overlayLines = strings.Count(overlayContent, "\n") + 1
	}
	pickerLines := 0
	if pickerContent != "" {
		pickerLines = strings.Count(pickerContent, "\n") + 1
	}

	// 3. 计算固定区域高度
	header := m.renderHeader()
	headerHeight := lipgloss.Height(header) + 1 // +1 是 header 后的空行

	footer := m.renderFooter()
	footerHeight := lipgloss.Height(footer) // 2 行(Line 1 + Line 2)

	// 先渲染输入框,获取实际高度(textarea 可能多行)
	separator := m.renderInputSeparator(contentWidth)
	rawView := m.input.View()
	// 将第一行开头的缩进空格替换为 › 前缀(area prompt 统一用缩进空格,
	// 但视觉上希望第一行显示 ›,后续行保持缩进对齐)。
	// 替换策略:查找第一个不由 ANSI 序列开头的 " " 位置。
	if idx := findFirstPromptPos(rawView); idx >= 0 {
		rawView = rawView[:idx] + "› " + rawView[idx+2:]
	}
	inputView := lipgloss.NewStyle().Width(contentWidth).Render(rawView)
	inputHeight := lipgloss.Height(inputView)

	// 固定底部元素(在 styleApp 内):
	// separator(1) + input(inputHeight) + footer(footerHeight)
	fixedBottomHeight := 1 + inputHeight + footerHeight

	// Todo 面板(仅 overlayNone 且 todos >= 3 时显示)
	var todoPanelContent string
	var todoPanelHeight int
	if m.overlay == overlayNone {
		todos := m.todos
		if len(todos) >= 3 {
			todoPanelContent, todoPanelHeight = renderTodoPanel(m.msg(), todos, contentWidth, m.todoExpanded, m.todoFocused, m.spTodo.View())
			fixedBottomHeight += todoPanelHeight
		}
	}

	// 排队消息指示(running 时发送的消息入队)
	queueDock := ""
	if m.overlay == overlayNone {
		queueDock = m.renderQueueDock(contentWidth)
		if queueDock != "" {
			fixedBottomHeight++
		}
	}

	// 新内容提示("↓ 新消息")占据 1 行,bodyHeight 必须减去
	newContentHintLines := 0
	if m.hasNewContent && m.overlay == overlayNone {
		newContentHintLines = 1
	}

	// styleApp 顶部 padding 1 行,底部 0;内区可用高度 = m.height - 1
	innerHeight := m.height - 1
	bodyHeight := innerHeight - headerHeight - fixedBottomHeight - overlayLines - pickerLines - newContentHintLines
	if bodyHeight < 1 {
		bodyHeight = 1
	}
	m.bodyHeight = bodyHeight

	// 4. 根据滚动偏移裁剪可见内容(与 waveloom 对齐:焦点模式不强制滚动,
	// PgUp/PgDn/Ctrl+E 全局生效,滚动自由)
	totalLines := len(allLines)
	maxScrollTop := max(0, totalLines-bodyHeight)

	if m.pinnedToBottom {
		m.scrollTop = maxScrollTop
	} else {
		// 内容增长时 scrollTop 可能超出新的 maxScrollTop
		if m.scrollTop > maxScrollTop {
			m.scrollTop = maxScrollTop
		}
		if m.scrollTop < 0 {
			m.scrollTop = 0
		}
		// 用户已滚动到底部 → 重新锁定,新内容提示自动消失
		if m.scrollTop >= maxScrollTop {
			m.pinnedToBottom = true
			m.hasNewContent = false
			m.scrollTop = maxScrollTop
		}
	}

	var visibleLines []string
	if totalLines > 0 {
		end := m.scrollTop + bodyHeight
		if end > totalLines {
			end = totalLines
		}
		visibleLines = allLines[m.scrollTop:end]
	}

	// 5. 构建 parts:header + body(刚好 bodyHeight 行) + overlays + 固定底部
	parts := []string{header, ""}
	parts = append(parts, visibleLines...)

	// 用空行补足 body 区域到 bodyHeight 行,确保 footer 位置固定
	padLines := bodyHeight - len(visibleLines)
	// 空状态:body 无用户内容时显示欢迎引导(居中,muted 色)。忽略纯系统消息段落。
	hasContent := false
	for _, p := range m.paras {
		if p.Type != paraSystem {
			hasContent = true
			break
		}
	}
	if !hasContent && !m.running && !m.inPlanMode && padLines > 0 {
		guide := m.msg().WelcomeGuide
		guideLines := strings.Split(guide, "\n")
		// 计算垂直居中位置
		guideHeight := len(guideLines)
		guidePos := (padLines - guideHeight) / 2
		if guidePos < 0 {
			guidePos = 0
		}
		welcomeStyle := lipgloss.NewStyle().
			Foreground(colorMuted).
			Width(contentWidth).
			Align(lipgloss.Center)
		for i := 0; i < padLines; i++ {
			idx := i - guidePos
			if idx >= 0 && idx < guideHeight {
				parts = append(parts, welcomeStyle.Render(guideLines[idx]))
			} else {
				parts = append(parts, "")
			}
		}
	} else {
		for i := 0; i < padLines; i++ {
			parts = append(parts, "")
		}
	}

	// 新内容提示:用户向上滚动查看历史时,新内容到达显示跳回提示
	if m.hasNewContent && m.overlay == overlayNone {
		hintStyle := lipgloss.NewStyle().
			Foreground(colorAccentGold).
			Width(contentWidth).
			Align(lipgloss.Center)
		parts = append(parts, hintStyle.Render(m.msg().NewContentHint))
	}

	if overlayContent != "" {
		parts = append(parts, overlayContent)
	}
	if todoPanelContent != "" {
		parts = append(parts, todoPanelContent)
	}
	if pickerContent != "" {
		parts = append(parts, pickerContent)
	}
	if queueDock != "" {
		parts = append(parts, queueDock)
	}
	parts = append(parts, separator, inputView, footer)

	mainBody := lipgloss.JoinVertical(lipgloss.Left, parts...)
	mainContent := styleApp.Render(mainBody)

	v := tea.NewView(mainContent)
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion

	// real cursor 模式:定位输入光标
	if m.overlay == overlayNone {
		if cur := m.input.Cursor(); cur != nil {
			// 布局:styleApp top(1) + header + 空行 + body + newContentHint + overlays + todo + picker + queueDock + separator(1)
			cur.Y += 1 + headerHeight + bodyHeight + newContentHintLines + overlayLines + todoPanelHeight + pickerLines + 1
			if queueDock != "" {
				cur.Y++ // 排队提示条占 1 行
			}
			cur.X += 2 // styleApp 左 padding
			if cur.X > m.width-2 {
				cur.X = m.width - 2
			}
			if cur.Y >= m.height {
				cur.Y = m.height - 1
			}
			v.Cursor = cur
		}
	} else if m.overlay == overlayQuestion && m.pendingQuestion != nil && m.pendingQuestion.otherMode {
		// Other 自定义输入:光标定位在 overlay box 内(对齐 waveloom)。
		// box 顶部 = styleApp top(1) + header + body;
		// 内部偏移按实际 wrap 行数累加(长问题/长选项会换行)。
		if cur := m.otherInput.Cursor(); cur != nil {
			pos := m.otherInput.Position()
			runes := []rune(m.otherInput.Value())
			if pos > len(runes) {
				pos = len(runes)
			}
			cursorX := lipgloss.Width(m.otherInput.Prompt) + lipgloss.Width(string(runes[:pos]))
			// X: styleApp 左 padding(2) + box border(1) + box 左 padding(2) = 5
			cur.X = cursorX + 5
			if cur.X > m.width-2 {
				cur.X = m.width - 2
			}
			// box 顶部 = styleApp top(1) + header + body + 新内容提示(如有)
			boxTop := 1 + headerHeight + bodyHeight + newContentHintLines
			cur.Y = boxTop + otherInputBoxOffset(m, m.width-4)
			if cur.Y >= m.height {
				cur.Y = m.height - 1
			}
			v.Cursor = cur
		}
	}
	return v
}

// otherInputBoxOffset 计算 Other 输入框在 overlay box 内的 Y 偏移:
// border(1)+pad(1)+ 逐行 wrap 计数(title/空行/question/选项/Other/空行/placeholder)。
// 与 renderQuestionOverlay 的渲染保持同步。
func otherInputBoxOffset(m *model, boxWidth int) int {
	pq := m.pendingQuestion
	if pq == nil || pq.currentQ >= len(pq.Questions) {
		return 0
	}
	inner := boxWidth - 6 // border(2) + padding(4)
	if inner < 10 {
		inner = 10
	}
	lc := m.msg()
	y := 2 // border top + padding top

	title := fmt.Sprintf("❓ "+lc.ToolNQuestions, len(pq.Questions))
	if len(pq.Questions) > 1 {
		title += fmt.Sprintf("  [%d/%d]", pq.currentQ+1, len(pq.Questions))
	}
	y += countWrappedLines(title, inner)
	y += 1 // 空行
	y += countWrappedLines(pq.Questions[pq.currentQ].Question, inner)
	for _, opt := range pq.Questions[pq.currentQ].Options {
		line := "  " + opt.Label
		if opt.Description != "" {
			line += " — " + opt.Description
		}
		y += countWrappedLines(line, inner)
	}
	y += countWrappedLines("  "+lc.QuestionOtherOption, inner)
	y += 1 // 空行
	y += countWrappedLines(lc.QuestionOtherPlaceholder, inner)
	return y
}

// ---------------------------------------------------------------------------
// Header 渲染
// ---------------------------------------------------------------------------

// renderQueueDock 渲染排队消息指示。会话繁忙时发送的消息由宿主入队
// (session.prompt mode=queue),session/queue 帧推送快照;对齐 dsh web
// 的 QueueDock 语义。仅显示 placement=queued 的条目。
func (m *model) renderQueueDock(contentWidth int) string {
	var previews []string
	count := 0
	for _, it := range m.queueItems {
		if it.Placement != "queued" {
			continue
		}
		count++
		preview := queueItemPreview(it.Message)
		if preview != "" {
			previews = append(previews, preview)
		}
	}
	if count == 0 {
		return ""
	}
	label := fmt.Sprintf(m.msg().QueueDockHint, count)
	joined := strings.Join(previews, " · ")
	if w := contentWidth - lipgloss.Width(label) - 4; w > 0 {
		joined = truncateByDisplayWidth(joined, w)
	}
	return styleQueueDock.Render(label + " " + joined)
}

// queueItemPreview 从排队消息 JSON 提取首条文本块预览(截断)。
func queueItemPreview(raw json.RawMessage) string {
	var msg struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(raw, &msg); err != nil {
		return ""
	}
	for _, b := range msg.Content {
		if b.Type == "text" && strings.TrimSpace(b.Text) != "" {
			s := strings.TrimSpace(b.Text)
			if displayWidth(s) > 24 {
				s = truncateByDisplayWidth(s, 24) + "…"
			}
			return s
		}
	}
	return ""
}

// renderInputSeparator 渲染输入框上方的分隔行。
// 焦点模式:居中全宽提示(操作指引,需要醒目)
// plan 模式:左侧 "▌Plan" 前缀 + ─ 填充(状态标记,不抢眼)
// 正常:纯横线
func (m *model) renderInputSeparator(contentWidth int) string {
	if m.focusIndex >= 0 {
		hint := m.msg().FocusSeparatorHint
		pad := contentWidth - lipgloss.Width(hint)
		if pad < 2 {
			pad = 2
		}
		left := strings.Repeat("─", pad/2)
		right := strings.Repeat("─", pad-pad/2)
		return styleInputSeparatorLine.Render(left) + styleInputSeparatorHint.Render(hint) + styleInputSeparatorLine.Render(right)
	}
	if m.inPlanMode {
		prefix := styleInputSeparatorPlan.Render("▌Plan")
		rest := strings.Repeat("─", max(contentWidth-lipgloss.Width(prefix), 0))
		return prefix + styleInputSeparatorLine.Render(rest)
	}
	// 空闲态:绿色分隔线提示就绪,与运行中的 muted 分隔线区分
	if !m.running {
		return lipgloss.NewStyle().
			Foreground(colorOK).
			Render(strings.Repeat("─", contentWidth))
	}
	return styleInputSeparatorLine.Render(strings.Repeat("─", contentWidth))
}

// renderLogoLines 返回 logo 的渲染行(不含 session/CWD 元数据),用于插入 viewport 可滚动区域。
func (m *model) renderLogoLines(contentWidth int) []string {
	if contentWidth >= 80 {
		lines := make([]string, 0, 7)
		for i, line := range asciiArt {
			s := lipgloss.NewStyle().
				Foreground(colorLogoGradient[i]).
				Bold(true).
				Width(contentWidth).
				Align(lipgloss.Center).
				Render(line)
			lines = append(lines, s)
		}
		lines = append(lines, "") // logo 后空行分隔
		return lines
	}
	// 窄屏:单行紧凑 logo
	logoLine := lipgloss.NewStyle().
		Foreground(colorLogoGradient[0]).
		Bold(true).
		Width(contentWidth).
		Align(lipgloss.Center).
		Render("dsh-tui")
	return []string{logoLine, ""}
}

// renderHeader 渲染 header:session ID(左) + 版本号(右) + 工作区。
func (m *model) renderHeader() string {
	contentWidth := max(m.width-4, 20)
	var sb strings.Builder

	// 信息行:会话标题(优先)/session ID(左) + 版本号(右)
	sidLine := ""
	if m.sessionTitle != "" {
		// title 投影(宿主权威,如 "你是谁")+ 短 session id
		sidPart := styleHeaderAccent.Render(m.msg().HeaderSession) + styleHeader.Render(m.sessionTitle)
		if short := shortSessionID(m.sessionID); short != "" {
			sidPart += " " + styleMuted.Render("(" + short + ")")
		}
		verStr := styleHeaderAccent.Render(Version)
		leftWidth := lipgloss.Width(sidPart)
		rightWidth := lipgloss.Width(verStr)
		pad := contentWidth - leftWidth - rightWidth
		if pad < 1 {
			pad = 1
		}
		sidLine = lipgloss.NewStyle().Width(contentWidth).Render(
			sidPart + strings.Repeat(" ", pad) + verStr,
		)
	} else if sid := m.sessionID; sid != "" {
		sidPart := styleHeaderAccent.Render(m.msg().HeaderSession) +
			styleHeader.Render(sid)
		verStr := styleHeaderAccent.Render(Version)
		leftWidth := lipgloss.Width(sidPart)
		rightWidth := lipgloss.Width(verStr)
		pad := contentWidth - leftWidth - rightWidth
		if pad < 1 {
			pad = 1
		}
		sidLine = lipgloss.NewStyle().Width(contentWidth).Render(
			sidPart + strings.Repeat(" ", pad) + verStr,
		)
	} else {
		// 无 session 时版本号右对齐
		verStr := styleHeaderAccent.Render(Version)
		sidLine = lipgloss.NewStyle().Width(contentWidth).Align(lipgloss.Right).Render(verStr)
	}
	sb.WriteString(sidLine)
	sb.WriteString("\n")

	// 工作区
	cwdDisplay := m.cwd
	maxCwdLen := contentWidth - 6
	if maxCwdLen < 10 {
		maxCwdLen = 10
	}
	if len(cwdDisplay) > maxCwdLen {
		cwdDisplay = cwdDisplay[:maxCwdLen] + "..."
	}
	cwdPart := styleHeaderAccent.Render("↳ ") + styleHeader.Render(cwdDisplay)

	lineCwd := lipgloss.NewStyle().Width(contentWidth).Render(cwdPart)
	sb.WriteString(lineCwd)

	return sb.String()
}

// ---------------------------------------------------------------------------
// Footer HUD 渲染
// ---------------------------------------------------------------------------

func (m *model) renderFooter() string {
	contentWidth := max(m.width-4, 20)
	sep := "  "

	var sb strings.Builder

	// Line 1: spinner + model name + ctx progress bar
	indicator := styleFooterLabel.Render("•") + " "
	if m.running && m.overlay != overlayQuestion {
		indicator = m.spinner.View() + " "
	}
	modelPart := indicator + styleFooterModel.Render(m.hudModel)
	if m.hudThinkingEffort != "" {
		effStyle := styleThinkHigh
		if m.hudThinkingEffort == "max" {
			effStyle = styleThinkMax
		}
		modelPart += styleFooterLabel.Render("(effort ") + effStyle.Render(m.hudThinkingEffort) + styleFooterLabel.Render(")")
	}
	ctxPart := m.renderCtxBarCompact()

	line1Parts := []string{modelPart, ctxPart}
	line1 := styleFooter.Width(contentWidth).Render(strings.Join(line1Parts, sep))

	// Line 2: cache + tok + turns + messages + latency + balance
	compactingPart := m.renderCacheRate()
	tokensPart := styleFooterLabel.Render("tok") + " " + styleFooterValue.Render(shortTokens(m.hudPromptTokens)+"/"+shortTokens(m.hudComplTokens))
	turnsPart := styleFooterLabel.Render("turns") + " " + styleFooterValue.Render(fmt.Sprintf("%d", m.hudTurns))
	messagesPart := styleFooterLabel.Render("M") + " " + styleFooterValue.Render(fmt.Sprintf("%d", m.hudMessages))
	latencyPart := m.renderLatency()
	line2Parts := []string{compactingPart, tokensPart, turnsPart, messagesPart, latencyPart}
	if m.noticeBanner != "" {
		bannerText := m.noticeBanner
		if m.updating {
			bannerText += " " + updateSpinnerFrames[m.updateTick%len(updateSpinnerFrames)]
		}
		var updateStyle lipgloss.Style
		if strings.HasPrefix(m.noticeBanner, "✗") {
			updateStyle = styleFooterLatRed
		} else {
			updateStyle = lipgloss.NewStyle().Foreground(colorAccentGold)
		}
		line2Parts = append(line2Parts, updateStyle.Render(bannerText))
	}
	line2Content := strings.Join(line2Parts, sep)
	line2 := styleFooter.Width(contentWidth).Render(line2Content)

	sb.WriteString(line1)
	sb.WriteString("\n")
	sb.WriteString(line2)

	return sb.String()
}

// renderCtxBarCompact 渲染固定宽度的上下文窗口进度条(barWidth=20,每格 5%,ratio < 5% 时进度条留空)。
func (m *model) renderCtxBarCompact() string {
	// web 语义:projectedTokens ?? pressureTokens(投影的压力与预估压力)
	currentTokens := m.projectedTokens
	if currentTokens == 0 {
		currentTokens = m.lastPromptTokens
	}
	if currentTokens == 0 {
		return styleFooterLabel.Render("ctx") + " " + styleFooterValueMuted.Render("--")
	}

	barWidth := 20
	ratio := float64(currentTokens) / float64(m.contextLimit)
	if ratio > 1 {
		ratio = 1
	}

	pct := ratio * 100

	// 量化到 5% 步进(20 格,每格 5%),避免部分填充造成视觉误导
	displayRatio := float64(int(ratio*20)) / 20
	m.ctxProgress.SetWidth(barWidth)
	barStr := m.ctxProgress.ViewAs(displayRatio)

	var tokenStyle lipgloss.Style
	switch {
	case pct < 50:
		tokenStyle = styleCtxBarGreenFg
	case pct < 80:
		tokenStyle = styleCtxBarGoldFg
	default:
		tokenStyle = styleCtxBarRedFg
	}

	tokenStr := tokenStyle.Render(fmt.Sprintf("%s/%s",
		formatTokens(currentTokens), formatTokens(m.contextLimit)))

	return styleFooterLabel.Render("ctx") + " " + barStr + " " + tokenStr
}

// renderCacheRate 渲染缓存命中率。
func (m *model) renderCacheRate() string {
	label := styleFooterLabel.Render("cache")
	total := m.hudCacheHit + m.hudCacheMiss
	if total == 0 {
		return label + " " + styleFooterValueMuted.Render("--")
	}

	pct := int(float64(m.hudCacheHit) / float64(total) * 100)

	var valStyle lipgloss.Style
	switch {
	case pct >= 95:
		valStyle = styleCacheGreen
	case pct >= 75:
		valStyle = styleCacheGold
	default:
		valStyle = styleFooterLatRed
	}

	return label + " " + valStyle.Render(fmt.Sprintf("%d%%", pct))
}

// renderLatency 渲染最近一次 loop 耗时(运行中实时计时,结束后显示最终值)。
func (m *model) renderLatency() string {
	label := styleFooterLabel.Render("elap")

	var elapsed int64
	if m.running && !m.turnStartTime.IsZero() {
		elapsed = time.Since(m.turnStartTime).Milliseconds()
	} else {
		elapsed = m.hudLatMs
	}

	if elapsed == 0 {
		return label + " " + styleFooterValueMuted.Render("--")
	}

	var valStyle lipgloss.Style
	switch {
	case elapsed < 120000:
		valStyle = styleFooterValue
	case elapsed < 600000:
		valStyle = styleCacheGold
	default:
		valStyle = styleFooterLatRed
	}

	return label + " " + valStyle.Render(formatDuration(elapsed))
}
