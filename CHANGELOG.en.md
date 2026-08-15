# Changelog

## [v0.0.2] — 2026-08-15

### Features

- **Auto-update**: background check against GitHub Releases at startup (302 tag comparison, 2s timeout, silent on failure); footer banner announces new versions (⏎ update vX); press Enter on empty input to self-update — downloads the platform package, extracts, backs up and replaces the binary, with automatic rollback on failure; skips chmod on Windows
- **Exit resume hint**: prints the full session id on exit so you can `dsh-tui --resume <id>` later

### Fixes

- Semantic version comparison for update checks: git describe suffixes (`v0.0.1-7-g...-dirty`) no longer trigger false update notices; only the `vX.Y.Z` segment is compared

---

## [v0.0.1] — 2026-08-15

Initial release: a Go terminal client for deepseek-harness (rendering layer ported from waveloom).

### Features

- **Pure client architecture**: connects to a dsh host over HTTP/WS (`dsh web` resident); the web UI and any number of TUI windows share one instance; `--resume <id>` restores sessions
- **wire protocol client**: quadrant RPC + dual WS downlink + respond; reconnect with `session/subscribed` seq comparison to backfill history gaps
- **Rendering layer** (waveloom port): paragraphs/theme/HUD/overlay, logo/header/footer, auto/dark/light/colorblind themes
- **Tool view alignment**: edit diff (red/green), read line-number view, grep/glob search groups, bash exit code/duration, todo_write count summary, job tool status; summary-line args and structured suffixes for all host tools
- **HUD vs host projections**: ctx progress (projectedTokens first, exact percentage, pressure coloring), turns/elap/tok/cache/effort/session title from host projections; tok supports k/M units (≥1M shows x.xM)
- **Session list**: `←` opens/closes when idle (↑↓ navigate, Enter switch, N new); aligned with dsh web visibility rules (only the current blank session visible, subagent sessions hidden), shows host title projection, fixed-height window (8 items + scroll follow + more hints), blank line between items, selected style consistent with other overlays (left accent border + green bold), second line shows the working directory
- **Approval/question overlays**: ↑↓ selection; question supports Other custom answers; approval handles sandbox-escalation allow/deny
- **Input interactions**: history navigation (↑↓), Esc Esc clear, bracketed paste, `exit` quit, Ctrl+Enter queue/steer toggle
- **Config following**: busyEnter (queue/steer), locale.preference (language), auto theme follows terminal background (sync re-query + 5s polling)
- **Multi-session isolation**: mux frames filtered by sessionId; send target synced on switch/new
- **Install scripts**: install.sh (macOS/Linux), install.ps1 (Windows), with `make release` cross-compiled artifacts

### Fixes

- Missing spinner animation (TickMsg routing)
- Tool completion state not updating (toolCallId field drift)
- Streaming paragraph leak after interrupt (turn/end finalize + pointer→index refactor)
- Approval/question respond protocol alignment (question cancel via RPC error `cancelled`)
- Input cursor positioning (QueueDock/overlay offsets, long-text wrapping)
- HUD dead fields (Loop/M/elap/ctx stuck at 0 or `--`)
