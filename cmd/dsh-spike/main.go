// Command dsh-spike validates the wire client against a live dsh host.
// It creates its own session (never touches existing ones), subscribes to the
// two downlink streams, optionally sends one prompt, and prints every frame.
// Usage:
//
//	dsh-spike [--url http://127.0.0.1:3080] [--cwd /path] [--prompt "text"] [--watch 15]
//
// Without --create/--prompt it only observes. --prompt implies creating a new
// session. The spike is stage-0 scaffolding and will be removed once the TUI
// takes over.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/Menfre01/dsh-tui/internal/dsh"
)

func main() {
	url := flag.String("url", fmt.Sprintf("http://127.0.0.1:%d", dsh.DefaultPort), "dsh host base URL")
	cwd := flag.String("cwd", "", "working directory for the created session (default: host cwd)")
	prompt := flag.String("prompt", "", "prompt text to send to the created session")
	watch := flag.Int("watch", 15, "seconds to keep observing after setup")
	flag.Parse()

	// Windows 无 SIGTERM 语义,仅注册 os.Interrupt(Unix 上 Ctrl+C 亦映射到此)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	client := dsh.NewClient(*url)

	// 1. describe
	desc, err := client.Describe(ctx)
	if err != nil {
		fatal("host.describe failed (is `dsh web` running?): %v", err)
	}
	fmt.Printf("== host.describe: version=%s cwd=%s provider=%s model=%s attached=%d canOpenPath=%v\n",
		desc.Version, desc.Cwd, desc.Provider, desc.Model, desc.AttachedSessions, desc.CanOpenPath)

	// 2. list existing sessions
	list, err := client.ListSessions(ctx)
	if err != nil {
		fatal("session.list: %v", err)
	}
	fmt.Printf("== session.list: %d sessions\n", len(list.Items))
	for _, s := range list.Items {
		fmt.Printf("   %s running=%v blank=%v cwd=%s preset=%s\n",
			s.SessionID, s.Running, s.Blank, s.Cwd, s.AgentPreset)
	}

	// 3. subscribe to both downlinks (readiness = both open)
	mux := dsh.NewDownlink(*url, "/api/events.mux")
	host := dsh.NewDownlink(*url, "/api/events.host")
	mux.OnFrame = func(f dsh.ServerRequest) { printMuxFrame(f) }
	host.OnFrame = func(f dsh.ServerRequest) { printHostFrame(f) }
	mux.OnError = func(err error) { fmt.Printf("!! mux error: %v\n", err) }
	host.OnError = func(err error) { fmt.Printf("!! host error: %v\n", err) }
	go func() { _ = mux.Run(ctx) }()
	go func() { _ = host.Run(ctx) }()

	// 4. create a dedicated session (safe: never touches existing ones)
	var sessionID string
	if *prompt != "" || *cwd != "" {
		created, err := client.CreateSession(ctx, dsh.SessionCreateRequest{Cwd: *cwd})
		if err != nil {
			fatal("session.create: %v", err)
		}
		sessionID = created.SessionID
		fmt.Printf("== session.create: %s\n", sessionID)
	}

	// 5. send one prompt and watch the stream
	if *prompt != "" && sessionID != "" {
		time.Sleep(2 * time.Second) // let the mux subscription settle
		fmt.Printf("== session.prompt: %q\n", *prompt)
		val, err := client.Prompt(ctx, dsh.SessionPromptRequest{
			SessionID: sessionID,
			Mode:      dsh.PromptModeQueue,
			Content:   []dsh.PromptContentPart{{Type: "text", Text: *prompt}},
		})
		if err != nil {
			fatal("session.prompt: %v", err)
		}
		fmt.Printf("== prompt accepted=%v command=%+v\n", val.Accepted, val.Command)
	}

	// 6. observe for --watch seconds, or until Ctrl-C
	deadline := time.After(time.Duration(*watch) * time.Second)
	if *watch <= 0 {
		deadline = nil
	}
	fmt.Printf("== observing for %ds (Ctrl-C to stop)...\n", *watch)
	select {
	case <-ctx.Done():
		fmt.Println("\n== interrupted")
	case <-deadline:
		fmt.Println("\n== watch window elapsed")
	}

	// 7. cancel any running work in our session, then exit
	if sessionID != "" {
		if err := client.Cancel(ctx, sessionID); err != nil {
			fmt.Printf("!! session.cancel: %v\n", err)
		} else {
			fmt.Printf("== session.cancel: %s\n", sessionID)
		}
	}
}

func printMuxFrame(f dsh.ServerRequest) {
	var m dsh.MuxFrame
	if err := f.DecodePayload(&m); err != nil {
		fmt.Printf("[mux] %s decode error: %v\n", f.Method, err)
		return
	}
	switch m.Type {
	case dsh.MuxSessionEvent:
		if m.Event == nil {
			fmt.Printf("[mux] session/event (nil event)\n")
			return
		}
		fmt.Printf("[mux] %s session/event seq=%d type=%s\n", m.SessionID, m.Event.Seq, m.Event.Type)
		printEventDetail(m.Event)
	case dsh.MuxApprovalReq:
		fmt.Printf("[mux] %s APPROVAL requested id=%s tool=%s call=%s reason=%q\n",
			m.SessionID, m.ApprovalID, m.ToolName, m.CallID, m.Reason)
	case dsh.MuxApprovalRes:
		fmt.Printf("[mux] %s approval resolved id=%s outcome=%s\n", m.SessionID, m.ApprovalID, m.Outcome)
	case dsh.MuxQuestionReq:
		for _, q := range m.Questions {
			fmt.Printf("[mux] %s QUESTION requested: %s (options=%d)\n", m.SessionID, q.Question, len(q.Options))
		}
	case dsh.MuxQuestionRes:
		fmt.Printf("[mux] %s question resolved rpc=%s outcome=%s\n", m.SessionID, m.QuestionRpcID, m.Outcome)
	case dsh.MuxSessionSub:
		fmt.Printf("[mux] %s subscribed lastSeq=%d\n", m.SessionID, m.LastSeq)
	case dsh.MuxSessionQueue:
		fmt.Printf("[mux] %s queue: %d items\n", m.SessionID, len(m.Items))
	case dsh.MuxSessionJobs:
		fmt.Printf("[mux] %s jobs: %d\n", m.SessionID, len(m.Jobs))
	case dsh.MuxSessionProj:
		fmt.Printf("[mux] %s projection key=%s seq=%d\n", m.SessionID, m.Key, m.Seq)
	case dsh.MuxStreamError:
		fmt.Printf("[mux] stream/error: %v\n", m.Error)
	default:
		fmt.Printf("[mux] %s frame type=%s\n", m.SessionID, m.Type)
	}
}

func printEventDetail(ev *dsh.SessionEvent) {
	switch ev.Type {
	case dsh.EvAssistantChunk:
		var d struct {
			Chunk dsh.StreamChunk `json:"chunk"`
		}
		if err := unmarshal(ev.Data, &d); err != nil {
			return
		}
		switch d.Chunk.Type {
		case dsh.ChunkTextDelta:
			fmt.Printf("        text: %s", d.Chunk.Text)
		case dsh.ChunkReasoningDelta:
			fmt.Printf("        reasoning: %s", d.Chunk.Text)
		case dsh.ChunkToolCallDelta:
			fmt.Printf("        tool-call %s %s += %q\n", d.Chunk.ID, d.Chunk.Name, d.Chunk.ArgumentsDelta)
		case dsh.ChunkBlockStart:
			fmt.Printf("        block-start %d %s\n", d.Chunk.Index, d.Chunk.BlockType)
		case dsh.ChunkFinish:
			fmt.Printf("        finish: %s\n", d.Chunk.Reason)
		case dsh.ChunkUsage:
			fmt.Printf("        usage: %+v\n", d.Chunk.Usage)
		}
	case dsh.EvUserMessage:
		// The wire data IS the UserMessage (no wrapper), confirmed against a
		// live host: {"id","role","content","source"}.
		var m dsh.Message
		if unmarshal(ev.Data, &m) == nil {
			texts := []string{}
			for _, b := range m.Content {
				if b.Type == "text" {
					texts = append(texts, b.Text)
				}
			}
			fmt.Printf("        user: %s\n", strings.Join(texts, ""))
		}
	case dsh.EvAssistantMsg:
		var d struct {
			Usage *dsh.TokenUsage `json:"usage"`
		}
		usage := ""
		if unmarshal(ev.Data, &d) == nil && d.Usage != nil {
			usage = fmt.Sprintf(" usage=%+v", *d.Usage)
		}
		fmt.Printf("        assistant message%s\n", usage)
	case dsh.EvToolCall:
		var d struct {
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
		}
		if unmarshal(ev.Data, &d) == nil {
			fmt.Printf("        tool/call %s %s\n", d.Name, d.Arguments)
		}
	case dsh.EvToolResult:
		var d struct {
			Message dsh.ToolResultMessage `json:"message"`
		}
		if unmarshal(ev.Data, &d) == nil {
			fmt.Printf("        tool/result %s\n", d.Message.Content[0].CallID)
		}
	case dsh.EvTodoWrite:
		var d struct {
			Todos []dsh.TodoItem `json:"todos"`
		}
		if unmarshal(ev.Data, &d) == nil {
			for _, t := range d.Todos {
				fmt.Printf("        todo %-11s %s\n", t.Status, t.Content)
			}
		}
	case dsh.EvTurnEnd:
		var d struct {
			Turn   int             `json:"turn"`
			Reason dsh.TurnEndReason `json:"reason"`
		}
		if unmarshal(ev.Data, &d) == nil {
			fmt.Printf("        turn %d ended: %s\n", d.Turn, d.Reason.Kind)
		}
	}
}

func printHostFrame(f dsh.ServerRequest) {
	var h dsh.HostFrame
	if err := f.DecodePayload(&h); err != nil {
		fmt.Printf("[host] %s decode error: %v\n", f.Method, err)
		return
	}
	switch h.Type {
	case dsh.HostSessionAdded:
		fmt.Printf("[host] session added %s blank=%v cwd=%s\n", h.SessionID, h.Blank, h.Cwd)
	case dsh.HostSessionRemoved:
		fmt.Printf("[host] session removed %s\n", h.SessionID)
	case dsh.HostSessionStatus:
		fmt.Printf("[host] session status %s running=%v\n", h.SessionID, h.Running)
	case dsh.HostAgentError:
		fmt.Printf("[host] agent error %s: %s\n", h.SessionID, h.Message)
	case dsh.HostWorkspaceChg:
		fmt.Printf("[host] workspace changed\n")
	case dsh.HostWorkspaceRm:
		fmt.Printf("[host] workspace removed %s\n", h.WorkspaceID)
	case dsh.HostWorkspaceOrder:
		fmt.Printf("[host] workspace order changed\n")
	case dsh.HostArchivedChg:
		fmt.Printf("[host] archived sessions changed: %d\n", len(h.ArchivedIDs))
	default:
		fmt.Printf("[host] frame type=%s\n", h.Type)
	}
}

func unmarshal(b []byte, v any) error {
	return json.Unmarshal(b, v)
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "dsh-spike: "+format+"\n", args...)
	os.Exit(1)
}
