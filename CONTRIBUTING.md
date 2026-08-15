# Contributing to dsh-tui

感谢你的贡献！

## 前置条件

- **Go 1.25+**
- **macOS 用户**：Xcode Command Line Tools（`xcode-select --install`）
- 构建/测试使用工作区缓存（`.gocache`/`.gopath`/`.modcache`），离线友好

## 快速开始

```sh
git clone git@github.com:Menfre01/dsh-tui.git
cd dsh-tui
make build
make test
make vet
```

## 开发流程

### TDD（测试驱动开发）

- Red → Green → Refactor 循环
- 先写测试，再写实现
- 修改代码后运行 `make test` 确保所有测试通过

### 协议漂移防护

- wire 层全部隔离在 `internal/dsh`，渲染/投影在 `internal/tui`
- 宿主协议变更时先对照 upstream 源码（`@deepseek-ai/*` 包）再改 wire 层
- 新增工具渲染时，`formatToolArgs`（参数摘要）与 `toolSuffix`（状态 suffix）必须覆盖，避免摘要行显示原始 JSON

### 测试要求

- 投影/交互/渲染路径的修复必须带回归测试
- 布局/交互测试在 `internal/tui/*_test.go`，wire 测试在 `internal/dsh/*_test.go`

## 项目结构

```
dsh-tui/
├── cmd/dsh-tui/          # CLI 入口 + TUI 接线
├── cmd/dsh-spike/        # wire 协议 spike 验证
├── internal/
│   ├── dsh/              # wire 协议客户端(四象限 RPC + 双 WS downlink + respond)
│   └── tui/              # 渲染层(waveloom 移植)+ 事件投影
├── install.sh            # macOS/Linux 安装脚本
├── install.ps1           # Windows 安装脚本
└── dist/                 # make release 产物(不提交)
```

## 发布

发布流程见 [AGENTS.md](AGENTS.md) 的 Release 规范：前置校验 → changelog 汇总 →
打 tag → release.yml 自动发布（含 Homebrew formula 推送）。

## 文档

- README 中英双语：`README.md`（中文）+ `README.en.md`（英文），修改需同步
- CHANGELOG 中英双语：`CHANGELOG.md` + `CHANGELOG.en.md`，每个版本条目
  末尾需含英文锚点（`📝 [Changelog (English)]`）
