package tui

import "testing"

// TestShortTokens 验证 token 紧凑格式:原值 / k / M 单位。
func TestShortTokens(t *testing.T) {
	cases := []struct {
		in   int
		want string
	}{
		{0, "0"},
		{999, "999"},
		{1000, "1.0k"},
		{1234, "1.2k"},
		{999999, "1000.0k"},
		{1_000_000, "1.0M"},
		{1_234_567, "1.2M"},
		{12_800_000, "12.8M"},
	}
	for _, c := range cases {
		if got := shortTokens(c.in); got != c.want {
			t.Errorf("shortTokens(%d) = %q, want %q", c.in, got, c.want)
		}
	}
}
