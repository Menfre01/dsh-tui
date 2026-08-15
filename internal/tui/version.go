package tui

import "fmt"

// Version 是 TUI 版本号。编译时可通过 ldflags 注入:
//
//	go build -ldflags "-X github.com/Menfre01/dsh-tui/internal/tui.Version=0.1.0"
var Version = "dev"

// shortTokens 将 token 数格式化为紧凑的人类可读形式:
// <1000 → 原值;≥1000 → x.xk;≥1M(或 k 四舍五入到 1000.0k)→ x.xM。
func shortTokens(n int) string {
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}
	if n >= 1_000_000 {
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	}
	return fmt.Sprintf("%.1fk", float64(n)/1000)
}
