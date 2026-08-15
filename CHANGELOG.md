# Changelog

## [v0.0.2] — 2026-08-15

### 新增功能

- **自动更新**:启动时后台检查 GitHub Release(302 tag 比对,2s 超时静默),
  footer 提示新版本(⏎ update vX),空输入按 Enter 触发自更新——下载当前
  平台包、解压、备份并替换二进制,失败自动回滚;Windows 跳过 chmod
- **退出恢复提示**:退出时打印完整 session id,方便 `dsh-tui --resume <id>` 恢复

### 修复

- 更新检测语义化版本比较:git describe 后缀(`v0.0.1-7-g...-dirty`)不再
  误报更新;仅比较 `vX.Y.Z` 主版本段

---

## [v0.0.1] — 2026-08-15

首发版本:deepseek-harness 的 Go 终端客户端(渲染层移植自 waveloom)。

### 新增功能

- **纯客户端架构**:通过 HTTP/WS 连接 dsh 宿主(`dsh web` 常驻),web UI 与
  任意多个 TUI 窗口共享同一实例;支持 `--resume <id>` 恢复会话
- **wire 协议客户端**:四象限 RPC + 双 WS downlink + respond 应答;
  断线重连与 `session/subscribed` 帧对比 seq 补拉历史缺口
- **渲染层**(waveloom 移植):段落/主题/HUD/overlay、logo/header/footer、
  auto/dark/light/colorblind 主题
- **工具 view 对齐**:edit 的 diff 红绿渲染、read 行号视图、grep/glob 的
  search 分组、bash 退出码/耗时、todo_write 计数摘要、job 工具状态;
  全部宿主工具的参数摘要与结构化 suffix
- **HUD 与宿主投影对齐**:ctx 进度(projectedTokens 优先,精确百分比,压力着色)、
  turns/elap/tok/cache/effort/会话标题均来自宿主投影;tok 支持
  k/M 单位(≥1M 显示 x.xM)
- **会话列表**:空闲态 `←` 打开/关闭(↑↓ 导航、Enter 切换、N 新建);
  对齐 dsh web 可见性规则(blank 会话仅当前可见、subagent 子会话隐藏)、
  显示宿主 title 投影、固定高度窗口(8 项 + 滚动跟随 + more 提示)、
  条目间留空行、选中样式与其他弹窗一致(左侧边框 + 绿色粗体)、
  选中项第二行显示工作目录
- **审批/提问框**:↑↓ 选择交互;提问支持 Other 自定义答案;审批支持
  sandbox 升级请求的允许/拒绝
- **输入交互**:历史导航(↑↓)、Esc Esc 清空、bracketed paste 粘贴、
  `exit` 退出、Ctrl+Enter 切换 queue/steer
- **配置跟随**:busyEnter(queue/steer)、locale.preference(语言)、
  主题 auto 跟随终端背景(同步重查 + 5s 轮询)
- **多会话隔离**:mux 帧按 sessionId 过滤,切换/新建后发送目标同步
- **安装脚本**:install.sh(macOS/Linux)、install.ps1(Windows),配合
  Makefile `release` 交叉编译产物

### 修复

- spinner 动画缺失(TickMsg 路由)
- tool 完成态不更新(toolCallId 字段漂移)
- 中断后流式段落泄漏(turn/end 收尾 + 段落指针改为索引)
- 审批/提问应答协议对齐(提问取消走 RPC error `cancelled`)
- 输入框光标定位(QueueDock/overlay 偏移、长文本换行)
- HUD 死字段(Loop/M/elap/ctx 恒 0 或 `--`)

---

📝 [Changelog (English)](https://github.com/Menfre01/dsh-tui/blob/main/CHANGELOG.en.md)
