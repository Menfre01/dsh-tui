// Command dsh-tui 是 deepseek-harness 的终端客户端(纯客户端,与宿主分离)。
//
// 用法:
//
//	dsh-tui [--url URL] ...      连接运行中的 dsh(默认 http://127.0.0.1:3080)
//	dsh-tui --resume <id>        恢复已有 session
//
// 连接后 Enter 发送、Esc 中断、Ctrl+S 会话列表、Ctrl+G 主题、Ctrl+M 模型。
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/Menfre01/dsh-tui/internal/dsh"
	"github.com/Menfre01/dsh-tui/internal/tui"
)

func main() {
	var (
		url       = flag.String("url", fmt.Sprintf("http://127.0.0.1:%d", dsh.DefaultPort), "dsh host base URL")
		cwd       = flag.String("cwd", "", "working directory for a new session (default: host cwd)")
		attach    = flag.String("attach", "", "attach to an existing session id instead of creating one")
		resume    = flag.String("resume", "", "resume a persisted session id (alias of --attach)")
		locale    = flag.String("locale", "auto", "UI locale: auto | zh-CN | en-US")
		theme     = flag.String("theme", "auto", "UI theme: auto | dark | light (auto follows terminal background)")
		maxEvents = flag.Int("history", 200, "max session.history events to replay on open")
		dump      = flag.Bool("dump", false, "render the replayed session once and exit (no TTY needed)")
		version   = flag.Bool("version", false, "print version and exit")
	)
	flag.Parse()
	if *version {
		fmt.Printf("dsh-tui %s\n", tui.Version)
		return
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	client := dsh.NewClient(*url)

	// --resume 与 --attach 同义(恢复会话到 TUI;dsh 对 cold session 自动恢复)
	if *resume != "" && *attach == "" {
		*attach = *resume
	}

	// 1. readiness + host 信息(带重试:dsh 可能刚启动还未就绪)
	var desc *dsh.HostDescribeValue
	{
		var err error
		for attempt := 0; ; attempt++ {
			desc, err = client.Describe(ctx)
			if err == nil {
				break
			}
			if attempt >= 9 {
				fmt.Fprintf(os.Stderr, "dsh-tui: cannot reach dsh at %s (%v).\n", *url, err)
				fmt.Fprintf(os.Stderr, "Start it first: dsh --profile tui (or dsh web)\n")
				os.Exit(1)
			}
			time.Sleep(500 * time.Millisecond)
		}
	}
	hostCwd := desc.Cwd

	// 会话工作目录:默认 = 打开 dsh-tui 的当前目录(与 waveloom 一致);
	// --cwd 显式覆盖;host 目录仅在无法获取 cwd 时兜底。
	workCwd := hostCwd
	if wd, err := os.Getwd(); err == nil {
		workCwd = wd
	}
	if *cwd != "" {
		workCwd = *cwd
	}

	// 2. 会话:附加已有或新建
	var sessionID string
	if *attach != "" {
		sessionID = *attach
	} else {
		req := dsh.SessionCreateRequest{Cwd: workCwd}
		created, err := client.CreateSession(ctx, req)
		if err != nil {
			fmt.Fprintf(os.Stderr, "dsh-tui: session.create: %v\n", err)
			os.Exit(1)
		}
		sessionID = created.SessionID
		fmt.Fprintf(os.Stderr, "session %s created (cwd=%s)\n", sessionID, req.Cwd)
	}

	// 3. TUI model + 投影器
	// 语言:flag 显式指定优先;auto 时读宿主 locale.preference 配置
	lc := messagesForLocale(*locale)
	if lc == nil {
		if v, err := client.SettingsValue(ctx, "locale", "preference"); err == nil && v != "" {
			lc = messagesForLocale(v)
		}
	}
	m := tui.NewModel(tui.ModelConfig{
		CWD:   workCwd,
		Theme: *theme,
		LC:    lc,
	})
	projector := tui.NewProjector(m)
	m.AttachProjector(projector)
	m.SetSessionInfo(sessionID, desc.Model)

	// 4. 回放历史(创建后立刻拉,种子事件已持久化)
	hist, err := client.SessionHistory(ctx, dsh.SessionHistoryRequest{
		SessionID:   sessionID,
		MaxMessages: int64Ptr(*maxEvents),
	})
	if err == nil {
		events := make([]tui.HistoryEvent, 0, len(hist.Events))
		for _, h := range hist.Events {
			events = append(events, tui.HistoryEvent{Event: h.Event, View: json.RawMessage(h.View)})
		}
		m.ReplayHistory(events)
	} else {
		fmt.Fprintf(os.Stderr, "dsh-tui: session.history: %v\n", err)
	}

	// 5. 订阅下行流 → TUI 事件循环
	subscribe := func() (*dsh.Downlink, *dsh.Downlink) {
		mux := dsh.NewDownlink(*url, "/api/events.mux")
		host := dsh.NewDownlink(*url, "/api/events.host")
		mux.OnFrame = func(f dsh.ServerRequest) { m.SendFrame(f) }
		host.OnFrame = func(f dsh.ServerRequest) {
			// host 帧:会话生命周期增量 → 投影器维护会话列表
			m.SendFrame(f)
			projector.ReplayHostFrame(f)
		}
		mux.OnError = func(err error) {
			m.SendFrame(dsh.ServerRequest{Method: "stream/error", Payload: mustJSON(map[string]any{"type": "stream/error", "error": map[string]any{"code": "internal", "message": "mux: " + err.Error(), "details": map[string]any{}}})})
		}
		host.OnError = func(err error) {
			m.SendFrame(dsh.ServerRequest{Method: "stream/error", Payload: mustJSON(map[string]any{"type": "stream/error", "error": map[string]any{"code": "internal", "message": "host: " + err.Error(), "details": map[string]any{}}})})
		}
		go func() { _ = mux.Run(ctx) }()
		go func() { _ = host.Run(ctx) }()
		return mux, host
	}
	subscribe()

	// 6. 回调:发送 / 取消
	m.SetCallbacks(
		func(text, mode string) {
			m.SetRunning(true)
			go func() {
				_, err := client.Prompt(context.Background(), dsh.SessionPromptRequest{
					SessionID: sessionID,
					Mode:      mode,
					Content:   []dsh.PromptContentPart{{Type: "text", Text: text}},
				})
				m.SendDone(err)
			}()
		},
		func() {
			m.SetRunning(false)
			go func() {
				err := client.Cancel(context.Background(), sessionID)
				m.SendCancelDone(err)
			}()
		},
	)
	// 对齐宿主/Web 配置:繁忙时 Enter 的发送模式(queue/steer)
	if v, err := client.SettingsValue(ctx, "ui-conversation", "busyEnter"); err == nil {
		m.SetBusyEnter(v)
	} else {
		fmt.Fprintf(os.Stderr, "dsh-tui: busyEnter config: %v\n", err)
	}
	// 应答器:审批/提问 → /api/respond
	m.SetResponder(&hostResponder{client: client})

	// 8. 会话列表(启动拉取)与切换/新建回调
	if list, err := client.ListSessions(ctx); err == nil {
		briefs := make([]tui.SessionBrief, 0, len(list.Items))
		for _, s := range list.Items {
			briefs = append(briefs, tui.SessionBrief{
				SessionID:   s.SessionID,
				Running:     s.Running,
				Blank:       s.Blank,
				Cwd:         s.Cwd,
				AgentPreset: s.AgentPreset,
			})
			// resume/attach 时同步当前会话的运行状态:
			// 否则会话繁忙时 Esc 中断失效(running 恒 false)
			if s.SessionID == sessionID {
				m.SetRunning(s.Running)
				// 投影初始值(ctx 压力/回合统计等宿主权威值)
				if len(s.Projections) > 0 {
					var proj struct {
						Values map[string]json.RawMessage `json:"values"`
					}
					if err := json.Unmarshal(s.Projections, &proj); err == nil {
						m.SetProjections(proj.Values)
					}
				}
			}
		}
		m.SetSessions(briefs)
	}
	m.SetSessionCallbacks(
		func(target string) {
			// 切换会话:更新发送目标 → 清空段落 → 拉历史 → 重投影
			sessionID = target // 发送/应答等后续 RPC 都去新会话
			go func() {
				hist, err := client.SessionHistory(context.Background(), dsh.SessionHistoryRequest{
					SessionID:   target,
					MaxMessages: int64Ptr(*maxEvents),
				})
				m.SendSwitchDone(target, hist, err)
			}()
		},
		func() {
			// 新建会话
			go func() {
				created, err := client.CreateSession(context.Background(), dsh.SessionCreateRequest{Cwd: workCwd})
				if err == nil {
					sessionID = created.SessionID // 新建后发送目标切到新会话
				}
				m.SendSwitchDone(created.SessionID, nil, err)
			}()
		},
	)
	// 模型选择器:拉取 session.models → 注入;选择 → selectModel
	m.SetModelSelectCallback(func(provider, model, effort string) {
		go func() {
			sel, err := client.SelectModel(context.Background(), sessionID, provider, model, effort)
			if err != nil {
				m.SendDone(err)
			} else {
				m.SendModelSelected(sel.Model, sel.ReasoningEffort)
			}
		}()
	})
	refreshModels := func() {
		go func() {
			models, effort, err := fetchModels(client, sessionID)
			if err == nil {
				m.SetThinkingEffort(effort)
			}
			m.SendModelsLoaded(models, err)
		}()
	}
	m.SetFetchModelsCallback(refreshModels)
	refreshModels() // 启动时预取

	// 断线补洞:重连后 subscribed 帧对比 seq,缺口拉 history 追加回放
	m.SetGapCallback(func(target string, fromSeq int64) {
		go func() {
			// session.history 是尾部向前分页,拉最新一页后按 seq 过滤缺口
			hist, err := client.SessionHistory(context.Background(), dsh.SessionHistoryRequest{
				SessionID:   target,
				MaxMessages: int64Ptr(*maxEvents),
			})
			if err != nil {
				m.SendGapEvents(nil, err)
				return
			}
			events := make([]tui.HistoryEvent, 0, len(hist.Events))
			for _, h := range hist.Events {
				if h.Event.Seq >= fromSeq {
					events = append(events, tui.HistoryEvent{Event: h.Event, View: json.RawMessage(h.View)})
				}
			}
			m.SendGapEvents(events, nil)
		}()
	})

	// 9. 启动 TUI
	if *dump {
		// 无头验证模式:初始化组件后渲染一次 View 并退出。
		fmt.Print(m.Dump(100, 40))
		return
	}
	program := tea.NewProgram(m)
	m.SetProgram(program)
	if _, err := program.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "dsh-tui: %v\n", err)
		os.Exit(1)
	}
}

func messagesForLocale(locale string) *tui.Messages {
	switch strings.ToLower(locale) {
	case "zh-cn", "zh":
		return tui.MessagesZhCN()
	case "en-us", "en":
		return tui.MessagesEnUS()
	default:
		return nil // auto → 包内回退
	}
}

// hostResponder 实现 tui.Responder,把用户决策发往 dsh host。
type hostResponder struct {
	client *dsh.Client
}

func (r *hostResponder) RespondApproval(rpcID, sessionID, approvalID string, allowed bool) error {
	return r.client.RespondApproval(context.Background(), rpcID, sessionID, approvalID, allowed)
}

func (r *hostResponder) RespondQuestion(rpcID, sessionID string, answers []dsh.QuestionAnswer) error {
	return r.client.RespondQuestion(context.Background(), rpcID, sessionID, answers)
}

func (r *hostResponder) RespondQuestionCancel(rpcID string) error {
	return r.client.RespondQuestionCancel(context.Background(), rpcID)
}

func int64Ptr(v int) *int64 {
	i := int64(v)
	return &i
}

// mustJSON 把值编码为 JSON 字节(downlink 错误帧构造用)。
func mustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		return []byte(`{}`)
	}
	return b
}

// fetchModels 拉取 session.models 并扁平化为模型选择列表;
// 同时返回当前会话的 reasoningEffort(宿主权威)。
func fetchModels(client *dsh.Client, sessionID string) ([]tui.ModelChoice, string, error) {
	val, err := client.SessionModels(context.Background(), sessionID)
	if err != nil {
		return nil, "", err
	}
	var out []tui.ModelChoice
	for _, g := range val.Groups {
		for _, mdl := range g.Models {
			effort := ""
			var efforts []tui.EffortChoice
			if mdl.Reasoning != nil {
				effort = mdl.Reasoning.DefaultEffort
				for _, e := range mdl.Reasoning.Efforts {
					efforts = append(efforts, tui.EffortChoice{ID: e.ID, Name: e.Name})
				}
			}
			out = append(out, tui.ModelChoice{
				Provider: g.ID,
				Model:    mdl.ID,
				Name:     mdl.Name,
				Effort:   effort,
				Efforts:  efforts,
			})
		}
	}
	return out, val.Current.ReasoningEffort, nil
}
