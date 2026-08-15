<p align="center">
  <a href="../README.md">English</a>
  &nbsp;·&nbsp;
  <strong>简体中文</strong>
</p>

---
# dsh-tui

[deepseek-harness](https://github.com/deepseek-ai/deepseek-harness) 的 Go 终端客户端。
移植自 [waveloom](https://github.com/Menfre01/waveloom) 的 bubbletea v2 渲染层。

**纯客户端架构**:dsh-tui 通过 HTTP/WS 连接 dsh 宿主进程,与宿主完全分离——
宿主用 `dsh web` 常驻,web UI 与任意多个 dsh-tui 窗口共享同一实例。

## 安装

### 一键安装脚本

**macOS / Linux**(安装到 `~/.local/bin`):

```bash
curl -fsSL https://raw.githubusercontent.com/Menfre01/dsh-tui/main/install.sh | sh
```

**Windows**(PowerShell):

```powershell
powershell -ExecutionPolicy Bypass -Command "iex (iwr -UseBasicParsing https://raw.githubusercontent.com/Menfre01/dsh-tui/main/install.ps1)"
```

脚本自动检测 OS/架构,从 GitHub Releases 下载对应包,校验 SHA256(Windows),
并处理 PATH。

### Homebrew

```bash
brew install menfre01/tap/dsh-tui
```

### 手动安装

从 [Releases](https://github.com/Menfre01/dsh-tui/releases) 下载对应平台的
`dsh-tui_<os>_<arch>.tar.gz`(unix)/`.zip`(windows),解压后将二进制放入 PATH。

### 从源码构建

```bash
make build      # 当前平台
make release    # 交叉编译 3 平台 × 2 架构 → dist/(含 checksums.txt)
```

## 快速开始

```bash
# 构建
make build                       # 产出 bin/dsh-tui

# 宿主进程(常驻,web + TUI 共享)
dsh web                          # 终端 1

# TUI 客户端(任意目录,可多开)
./bin/dsh-tui                    # 新建 session
./bin/dsh-tui --resume <id>      # 恢复已有 session(空闲时按 ← 打开会话列表)
./bin/dsh-tui --url http://192.168.x.x:3080   # 连接远程宿主
./bin/dsh-tui --locale zh-CN     # 指定语言(auto 时跟随宿主 locale.preference)
```

## 快捷键

### 全局

| 按键 | 功能 |
|---|---|
| Ctrl+C | 退出 |
| Ctrl+E / End | 跳到底部 |
| PgUp / PgDn | 翻页 |
| ↑/↓ / 滚轮 | 滚动(选中文本请用 **Shift+点击**,终端标准惯例) |
| Esc | 中断当前回合(session.cancel) |

### 输入框

| 按键 | 功能 |
|---|---|
| Enter | 发送(繁忙时入队,宿主 queue 模式);**Ctrl+Enter 反转 mode**(queue↔steer) |
| ↑/↓ | 输入历史导航(空闲时) |
| Esc Esc(500ms 内) | 清空输入框 |
| 粘贴 | bracketed paste(Cmd/Ctrl+Shift+V) |
| `exit` 回车 | 直接退出程序 |

### 段落焦点模式

| 按键 | 功能 |
|---|---|
| Tab / Shift+Tab | 聚焦下一个/上一个可展开段落 |
| ↑/↓ | 焦点模式内移动段落 |
| Enter | 展开/折叠(bash 完整输出、edit diff、grep 分组、thought 全文) |
| Esc | 返回输入框 |

### 弹层

| 按键 | 功能 |
|---|---|
| `←`(空闲且输入为空) | 会话列表:↑↓ 导航 · Enter 切换 · Esc 取消 |
| Ctrl+G | 主题:↑↓ 选 auto/dark/light/colorblind · Enter 应用 · Esc 关闭(auto 同步重查终端背景,5s 轮询跟随) |
| Ctrl+M | 模型:↑↓ 选模型 · Enter 选择(带默认 effort) · **E 进 effort 面板** · Esc |

### 审批/提问框

| 按键 | 功能 |
|---|---|
| ↑↓ / j / k | 移动光标 |
| Enter | 确认(审批:允许一次/拒绝;提问:选项/Other 自定义) |
| Esc | 拒绝/取消(提问 Other 输入模式时返回选项) |

## 架构

```
cmd/dsh-tui/          入口(连接/订阅/回调接线)
internal/dsh/         wire 协议客户端(四象限 RPC + 双 WS downlink + respond)
internal/tui/         渲染层(waveloom 移植:段落/主题/HUD/overlay) + 事件投影
```

### wire 协议(已对照 upstream 源码确认)

- Unary:`POST /api/<method>`,`{type:"client-request", rpcId, method, payload}`
- 下行:两条只收不发的 WebSocket(`/api/events.mux` + `/api/events.host`),帧 = `server-request`
- 应答:审批/提问帧 → `POST /api/respond`(rpcId 回显;提问取消走 RPC error `cancelled`)
- 信任:Host 头为 loopback 即通过
- 事件源:append-only `SessionEvent` 日志(user/message、assistant/chunk、tool/call+result、todo/write、turn/end 等,插件可扩展)
- 断线重连:downlink 自动重建 + `session/subscribed` 帧对比 seq 补拉 history 缺口
- 投影:`session/projection` 帧 + session.list projections(宿主权威的 contextPressure/sessionStats/tokenUsage/title 等)

## 宿主对齐

- **工具渲染**:bash/read/write/edit(diff)/grep/glob(search)/web_fetch/web_search/skill/ask_user_question/todo_write/job_output/list/kill/read_image/pwsh/ralph/goal/subagent 控制等——摘要行参数 + 结构化 suffix(退出码/匹配数/路径数/状态)
- **HUD**:ctx 进度(projectedTokens 优先,精确百分比,压力着色)、turns/elap/tok/cache 来自宿主投影,effort 显示 `(effort ...)`
- **配置跟随**:busyEnter(queue/steer,Ctrl+Enter 反转)、locale.preference(语言)、主题 auto(终端背景检测 + 轮询)
- **多会话**:mux 帧按 sessionId 过滤,切换/新建后发送目标同步,不串台
- **会话列表**:可见性规则对齐 dsh web(blank/subagent 过滤)、宿主 title 投影、固定高度窗口

## 开发

```bash
make build      # 构建(环境变量指向工作区缓存)
make test       # 单测(dsh wire + tui 交互/布局/投影)
make vet
make lint       # golangci-lint(对齐 waveloom 的 linter 集)
make dump       # 无头渲染验证(无 TTY 环境)
```

协议漂移防护:wire 层全部隔离在 `internal/dsh`;dsh 版本可通过 `host.describe.version`
感知。已知兼容:dsh master @ 2026-08-14(0.0.1)。

## 状态

- [x] wire 层(spike 对真实 dsh 验证)
- [x] 渲染层移植(完整 waveloom 视觉:logo/header/HUD/主题)
- [x] 审批(↑↓ 选择)/提问(↑↓ + Other 自定义)应答闭环
- [x] 会话切换/新建/主题/模型选择(含 effort 面板)
- [x] 断线重连事件补洞
- [x] 纯客户端分离架构(dsh web 共享宿主)
- [x] 工具 view 对齐(diff/read/search 结构化 + 全工具摘要)
- [x] HUD 与宿主投影对齐(ctx/turns/elap/tok/cache/effort/标题)
- [ ] fork/rename/search、agentPreset、subagent 视图
