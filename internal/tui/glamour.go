package tui

import (
	"fmt"
	"image/color"

	"charm.land/glamour/v2/ansi"
	glamourstyles "charm.land/glamour/v2/styles"
)

// ---------------------------------------------------------------------------
// glamour.go — 从 waveloom tui.go 移植的 Glamour 样式定制
// ---------------------------------------------------------------------------

// waveloomGlamourStyle 返回基于当前 Waveloom 色板定制的 Glamour 样式配置。
// Margin 统一清零(由 TUI 前缀缩进控制对齐),关键块级颜色映射到 Waveloom 色板。
func waveloomGlamourStyle(p palette) ansi.StyleConfig {
	var base ansi.StyleConfig
	if p.GlamourStyle == "light" {
		base = glamourstyles.LightStyleConfig
	} else {
		base = glamourstyles.DraculaStyleConfig
	}

	// 清零 margin
	zero := uint(0)
	emptyHex := ""
	base.Document.Margin = &zero
	base.CodeBlock.Margin = &zero
	base.Paragraph.Margin = &zero
	base.Heading.Margin = &zero

	// 提取 Waveloom 色板 hex 值
	bodyColor := colorHex(p.Body)
	grayColor := colorHex(p.Gray)
	mutedColor := colorHex(p.Muted)
	toolCode := colorHex(p.ToolCode)
	toolCodeBg := colorHex(p.ToolCodeBg)
	accent := colorHex(p.AccentGold)
	headerAccent := colorHex(p.HeaderAccent)

	// ── 正文段落 ──
	base.Paragraph.Color = &bodyColor

	// ── 引用块 ──
	base.BlockQuote.Color = &grayColor
	base.BlockQuote.Prefix = "│ "

	// ── 强调 / 加粗 ──
	base.Emph.Color = &accent
	base.Emph.Italic = boolPtr(true)
	base.Strong.Color = &headerAccent
	base.Strong.Bold = boolPtr(true)

	// ── 删除线 ──
	base.Strikethrough.Color = &mutedColor
	base.Strikethrough.CrossedOut = boolPtr(true)

	// ── 水平分割线 ──
	base.HorizontalRule.Color = &grayColor
	base.HorizontalRule.Format = "\n────\n"

	// ── 列表符号 / 编号 ──
	base.Item.Color = &accent
	base.Enumeration.Color = &accent

	// ── 表格 ──
	base.Table.Color = &grayColor

	// ── 行内代码 ──
	base.Code.Color = &toolCode
	base.Code.BackgroundColor = &toolCodeBg

	// ── 代码块 ──
	base.CodeBlock.Color = &toolCode
	if base.CodeBlock.Chroma != nil {
		base.CodeBlock.Chroma.Background.BackgroundColor = &toolCodeBg
		base.CodeBlock.Chroma.Text.Color = &toolCode
		base.CodeBlock.Chroma.Error.Color = &toolCode
		base.CodeBlock.Chroma.Error.BackgroundColor = &emptyHex
	}

	// ── 标题 H1–H6 ──
	base.Heading.Color = &headerAccent
	base.H1.BackgroundColor = &accent
	base.H2.Color = &headerAccent
	base.H3.Color = &headerAccent
	base.H4.Color = &headerAccent
	base.H5.Color = &headerAccent
	base.H6.Color = &headerAccent

	// ── 链接 ──
	base.Link.Color = &accent
	base.LinkText.Color = &accent

	return base
}

// boolPtr 返回 *bool,用于 Glamour style 字段。
func boolPtr(b bool) *bool { return &b }

// colorHex 从 color.Color 提取 hex 字符串。
func colorHex(c color.Color) string {
	r, g, b, _ := c.RGBA()
	return fmt.Sprintf("#%02x%02x%02x", uint8(r>>8), uint8(g>>8), uint8(b>>8))
}
