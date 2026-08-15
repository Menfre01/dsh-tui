package dsh

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// Downlink connects one of the two server-to-client event streams
// (/api/events.mux or /api/events.host). The socket is downlink-only: sending
// any application message makes the server close it with 1008, so this client
// never writes. Upstream traffic stays on HTTP (Call/Respond).
//
// The connection generation model mirrors the upstream client: if either
// downlink ends, the whole generation fails and both streams must be rebuilt;
// readiness requires both sockets open plus a successful host.describe.
type Downlink struct {
	// Path is the websocket endpoint path: "/api/events.mux" or
	// "/api/events.host".
	path string
	// conn is the live socket, replaced on reconnect. Guarded by mu.
	mu     sync.Mutex
	conn   *websocket.Conn
	closed bool

	// OnFrame receives every server-request frame (raw JSON, decoded lazily
	// by the consumer). It must not block: consumers buffer or dispatch.
	OnFrame func(frame ServerRequest)

	// OnError reports a stream failure (socket closed unexpectedly, read
	// error). The consumer decides whether to reconnect the whole pair.
	OnError func(err error)

	// ReconnectDelay is the pause before dial retries. Default 1s.
	ReconnectDelay time.Duration

	// DialTimeout bounds a single dial attempt. Default 10s.
	DialTimeout time.Duration

	// URL is the websocket URL, derived from baseURL at construction.
	url string
}

// NewDownlink builds a downlink for the given base URL and ws path
// ("/api/events.mux" or "/api/events.host").
func NewDownlink(baseURL, path string) *Downlink {
	wsURL := strings.Replace(baseURL, "http://", "ws://", 1)
	wsURL = strings.Replace(wsURL, "https://", "wss://", 1)
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return &Downlink{
		path:           path,
		url:            wsURL + path,
		ReconnectDelay: time.Second,
		DialTimeout:    10 * time.Second,
	}
}

// dial connects (or reconnects) the socket. It keeps retrying with
// ReconnectDelay until ctx is done or Close is called.
func (d *Downlink) dial(ctx context.Context) error {
	dialer := websocket.Dialer{
		HandshakeTimeout: d.DialTimeout,
	}
	reqHeaders := make(map[string][]string, 2)
	// The trust fence compares the Host header; keep it identical to the HTTP
	// calls so loopback classification matches.
	reqHeaders["Host"] = []string{hostOf(strings.Replace(d.url, "ws://", "http://", 1))}

	for {
		if d.isClosed() {
			return fmt.Errorf("downlink %s closed", d.path)
		}
		conn, _, err := dialer.DialContext(ctx, d.url, reqHeaders)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			d.reportError(fmt.Errorf("dial %s: %w", d.path, err))
			if !sleepCtx(ctx, d.ReconnectDelay) {
				return ctx.Err()
			}
			continue
		}
		d.mu.Lock()
		if d.closed {
			d.mu.Unlock()
			_ = conn.Close()
			return fmt.Errorf("downlink %s closed", d.path)
		}
		d.conn = conn
		d.mu.Unlock()
		return nil
	}
}

// Run pumps frames until the socket ends or ctx is done. It reconnects
// automatically: after a socket loss it reports the error via OnError and
// redials, so the two-stream pair can be torn down and rebuilt together by
// the caller when it observes the first failure.
func (d *Downlink) Run(ctx context.Context) error {
	for {
		if err := d.dial(ctx); err != nil {
			return err
		}
		if err := d.pump(ctx); err != nil {
			if ctx.Err() != nil || d.isClosed() {
				return ctx.Err()
			}
			d.reportError(err)
		}
	}
}

// pump reads frames and dispatches them until the socket closes.
func (d *Downlink) pump(ctx context.Context) error {
	conn := d.currentConn()
	if conn == nil {
		return fmt.Errorf("downlink %s: no socket", d.path)
	}
	conn.SetReadLimit(160 * 1024 * 1024)
	conn.SetPongHandler(func(string) error { return conn.SetReadDeadline(time.Now().Add(60 * time.Second)) })

	deadline := time.Now().Add(60 * time.Second)
	if err := conn.SetReadDeadline(deadline); err != nil {
		return err
	}

	// A goroutine refreshes the read deadline so an idle-but-open socket is
	// not killed by the deadline while pongs keep arriving.
	stopPing := make(chan struct{})
	defer close(stopPing)
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-stopPing:
				return
			case <-ticker.C:
				_ = conn.SetReadDeadline(time.Now().Add(60 * time.Second))
			}
		}
	}()

	for {
		if err := conn.SetReadDeadline(time.Now().Add(60 * time.Second)); err != nil {
			return err
		}
		_, raw, err := conn.ReadMessage()
		if err != nil {
			// The server closes sockets with a close frame on teardown;
			// anything else is a stream failure.
			if websocket.IsCloseError(err, websocket.CloseNormalClosure, 1008) {
				return fmt.Errorf("downlink %s: server closed: %w", d.path, err)
			}
			return fmt.Errorf("downlink %s: read: %w", d.path, err)
		}
		var frame ServerRequest
		if err := json.Unmarshal(raw, &frame); err != nil {
			d.reportError(fmt.Errorf("downlink %s: decode frame: %w", d.path, err))
			continue
		}
		if d.OnFrame != nil {
			d.OnFrame(frame)
		}
	}
}

func (d *Downlink) currentConn() *websocket.Conn {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.conn
}

func (d *Downlink) isClosed() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.closed
}

func (d *Downlink) reportError(err error) {
	if d.OnError != nil {
		d.OnError(err)
	}
}

// Close terminates the socket and stops reconnects.
func (d *Downlink) Close() error {
	d.mu.Lock()
	d.closed = true
	conn := d.conn
	d.conn = nil
	d.mu.Unlock()
	if conn != nil {
		return conn.Close()
	}
	return nil
}

func sleepCtx(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}
