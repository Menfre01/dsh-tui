<p align="center">
  <strong>English</strong>
  &nbsp;·&nbsp;
  <a href="./docs/README.zh-CN.md">简体中文</a>
</p>

<p align="center">
  <a href="https://github.com/Menfre01/dsh-tui/releases/latest"><img src="https://img.shields.io/github/v/release/Menfre01/dsh-tui?style=flat-square&color=00ADD8&labelColor=161b22" alt="release"/></a>
  <a href="https://github.com/Menfre01/dsh-tui/actions/workflows/ci.yml"><img src="https://github.com/Menfre01/dsh-tui/actions/workflows/ci.yml/badge.svg?style=flat-square&labelColor=161b22" alt="CI"/></a>
  <a href="https://github.com/Menfre01/dsh-tui/releases"><img src="https://img.shields.io/github/downloads/Menfre01/dsh-tui/total?style=flat-square&color=00ADD8&label=GitHub%20downloads&labelColor=161b22" alt="downloads"/></a>
  <a href="https://go.dev"><img src="https://img.shields.io/badge/language-Go-00ADD8?style=flat-square&logo=go&logoColor=white&labelColor=161b22" alt="Go"/></a>
  <a href="./LICENSE"><img src="https://img.shields.io/badge/license-Apache%202.0-8b949e?style=flat-square&labelColor=161b22" alt="license"/></a>
</p>

# dsh-tui

A Go terminal client for [deepseek-harness](https://github.com/deepseek-ai/deepseek-harness).
The rendering layer is ported from [waveloom](https://github.com/Menfre01/waveloom)'s Bubble Tea v2 TUI.

**Pure client architecture**: dsh-tui connects to a dsh host process over HTTP/WS,
fully decoupled from the host — the host runs `dsh web` as a resident, and the web
UI plus any number of dsh-tui windows share the same instance.

## Install

### One-command install script

```bash
# macOS / Linux (installs to ~/.local/bin)
curl -fsSL https://raw.githubusercontent.com/Menfre01/dsh-tui/main/install.sh | sh

# Windows (PowerShell)
powershell -ExecutionPolicy Bypass -File install.ps1
```

The script detects OS/architecture, downloads the matching package from GitHub
Releases, verifies SHA256 (Windows), and handles PATH.

### Homebrew

```bash
brew install menfre01/tap/dsh-tui
```

### Manual install

Download `dsh-tui_<os>_<arch>.tar.gz` (unix) / `.zip` (windows) from
[Releases](https://github.com/Menfre01/dsh-tui/releases), extract, and put the
binary on your PATH.

### Build from source

```bash
make build      # current platform
make release    # cross-compile 3 platforms × 2 arches → dist/ (with checksums.txt)
```

## Quick start

```bash
# Build
make build                       # produces bin/dsh-tui

# Host process (resident; web + TUI share it)
dsh web                          # terminal 1

# TUI client (any directory, multiple instances)
./bin/dsh-tui                    # new session
./bin/dsh-tui --resume <id>      # resume a session (Ctrl+S list: Y copies the full id)
./bin/dsh-tui --url http://192.168.x.x:3080   # connect to a remote host
./bin/dsh-tui --locale zh-CN     # force locale (auto follows host locale.preference)
```

## Key bindings

### Global

| Key | Action |
|---|---|
| Ctrl+C | Quit |
| Ctrl+E / End | Jump to bottom |
| PgUp / PgDn | Page up/down |
| ↑/↓ / mouse wheel | Scroll |
| Esc | Interrupt current turn (session.cancel) |

### Input box

| Key | Action |
|---|---|
| Enter | Send (queued when busy); **Ctrl+Enter toggles mode** (queue↔steer) |
| ↑/↓ | Input history navigation (when idle) |
| Esc Esc (within 500ms) | Clear the input box |
| Paste | Bracketed paste (Cmd/Ctrl+Shift+V) |
| `exit` + Enter | Quit the program |

### Paragraph focus mode

| Key | Action |
|---|---|
| Tab / Shift+Tab | Focus next/previous expandable paragraph |
| ↑/↓ | Move between paragraphs in focus mode |
| Enter | Expand/collapse (bash output, edit diff, grep groups, thought text) |
| Esc | Back to input |

### Overlays

| Key | Action |
|---|---|
| Ctrl+S | Session list: ↑↓ navigate · Enter switch · N new · **Y copy full id** · Esc close |
| Ctrl+G | Theme: ↑↓ pick auto/dark/light/colorblind · Enter apply · Esc close (auto re-queries terminal background; 5s polling) |
| Ctrl+M | Model: ↑↓ pick · Enter select (with default effort) · **E effort panel** · Esc |

### Approval / question overlays

| Key | Action |
|---|---|
| ↑↓ / j / k | Move cursor |
| Enter | Confirm (approval: allow once / deny; question: option / Other custom answer) |
| Esc | Deny / cancel (in Other input mode: back to options) |

## Architecture

```
cmd/dsh-tui/          entry (connection/subscription/callback wiring)
internal/dsh/         wire protocol client (quadrant RPC + dual WS downlink + respond)
internal/tui/         rendering layer (waveloom port: paragraphs/theme/HUD/overlay) + event projection
```

### wire protocol (confirmed against upstream source)

- Unary: `POST /api/<method>`, `{type:"client-request", rpcId, method, payload}`
- Downlink: two receive-only WebSockets (`/api/events.mux` + `/api/events.host`), frames = `server-request`
- Respond: approval/question frames → `POST /api/respond` (rpcId echoed; question cancel via RPC error `cancelled`)
- Trust: loopback Host header passes
- Event source: append-only `SessionEvent` log (user/message, assistant/chunk, tool/call+result, todo/write, turn/end, etc.; plugin-extensible)
- Reconnect: downlink auto-rebuild + `session/subscribed` seq comparison to backfill history gaps
- Projections: `session/projection` frames + session.list projections (host-authoritative contextPressure/sessionStats/tokenUsage/title)

## Host alignment

- **Tool rendering**: bash/read/write/edit(diff)/grep/glob(search)/web_fetch/web_search/skill/ask_user_question/todo_write/job_output/list/kill/read_image/pwsh/ralph/goal/subagent controls — summary-line args + structured suffix (exit code/match count/path count/status)
- **HUD**: ctx progress (projectedTokens first, exact percentage, pressure coloring), turns/elap/tok/cache from host projections, effort shown as `(effort ...)`
- **Config following**: busyEnter (queue/steer, Ctrl+Enter toggles), locale.preference (language), auto theme (terminal background detection + polling)
- **Multi-session**: mux frames filtered by sessionId; send target synced on switch/new — no cross-talk

## Development

```bash
make build      # build (env vars point to workspace caches)
make test       # unit tests (dsh wire + tui interaction/layout/projection)
make vet
make dump       # headless render verification (no TTY)
```

Protocol-drift protection: the wire layer is isolated in `internal/dsh`; the dsh
version is visible via `host.describe.version`. Known compatible: dsh master @ 2026-08-14 (0.0.1).

## Status

- [x] Wire layer (spike-verified against real dsh)
- [x] Rendering layer port (full waveloom visuals: logo/header/HUD/theme)
- [x] Approval (↑↓ select) / question (↑↓ + Other custom) respond loop
- [x] Session switch/new, theme, model selection (with effort panel)
- [x] Reconnect event backfill
- [x] Pure client architecture (shared dsh web host)
- [x] Tool view alignment (diff/read/search structured + all-tool summaries)
- [x] HUD vs host projections (ctx/turns/elap/tok/cache/effort/title)
- [ ] fork/rename/search, agentPreset, subagent views
