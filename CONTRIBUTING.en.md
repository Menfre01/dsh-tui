# Contributing to dsh-tui

Thanks for contributing!

## Prerequisites

- **Go 1.25+**
- **macOS users**: Xcode Command Line Tools (`xcode-select --install`)
- Build/test use workspace caches (`.gocache`/`.gopath`/`.modcache`), offline-friendly

## Quick start

```sh
git clone git@github.com:Menfre01/dsh-tui.git
cd dsh-tui
make build
make test
make vet
```

## Development flow

### TDD (test-driven development)

- Red → Green → Refactor loop
- Write the test first, then the implementation
- Run `make test` after changing code to keep everything green

### Protocol-drift protection

- The wire layer is isolated in `internal/dsh`; rendering/projection live in `internal/tui`
- When the host protocol changes, check upstream source (`@deepseek-ai/*` packages) before touching the wire layer
- When the host adds a tool, cover `formatToolArgs` (argument summary) and `toolSuffix` (status suffix) so the summary line never shows raw JSON

### Testing requirements

- Fixes on projection/interaction/rendering paths must ship with regression tests
- Layout/interaction tests live in `internal/tui/*_test.go`; wire tests in `internal/dsh/*_test.go`

## Project structure

```
dsh-tui/
├── cmd/dsh-tui/          # CLI entry + TUI wiring
├── cmd/dsh-spike/        # wire protocol spike verification
├── internal/
│   ├── dsh/              # wire protocol client (quadrant RPC + dual WS downlink + respond)
│   └── tui/              # rendering layer (waveloom port) + event projection
├── install.sh            # macOS/Linux installer
├── install.ps1           # Windows installer
└── dist/                 # make release artifacts (not committed)
```

## Releasing

See the Release section of [AGENTS.md](AGENTS.md): pre-release checks → changelog
summary → tag → release.yml auto-publishes (including Homebrew formula push).

## Docs

- README is bilingual: `README.md` (English main, with language nav) + `docs/README.zh-CN.md` (Simplified Chinese) — keep them in sync
- CHANGELOG is bilingual: `CHANGELOG.md` + `CHANGELOG.en.md` — each version entry
  must end with the English anchor (`📝 [Changelog (English)]`)
