package dsh

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// DefaultPort is the port dsh web listens on by default.
const DefaultPort = 3080

// Client is the HTTP unary RPC client for a running dsh host. It is safe for
// concurrent use. The Host header is set to the base authority so the
// browser-trust fence (loopback check) passes.
type Client struct {
	baseURL string // e.g. http://127.0.0.1:3080
	http    *http.Client
	version string // host version from host.describe, cached after Describe
}

// NewClient returns a client for baseURL (e.g. "http://127.0.0.1:3080").
func NewClient(baseURL string) *Client {
	baseURL = strings.TrimSuffix(baseURL, "/")
	return &Client{
		baseURL: baseURL,
		http: &http.Client{
			Timeout: 120 * time.Second,
			Transport: &http.Transport{
				DialContext: (&net.Dialer{
					Timeout:   10 * time.Second,
					KeepAlive: 30 * time.Second,
				}).DialContext,
				MaxIdleConns:          4,
				IdleConnTimeout:       60 * time.Second,
				ResponseHeaderTimeout: 120 * time.Second,
			},
		},
	}
}

// URL returns the base URL.
func (c *Client) URL() string { return c.baseURL }

// Version returns the cached host version (populated by Describe).
func (c *Client) Version() string { return c.version }

// Call performs one unary RPC: POST /api/<method> with a client-request
// envelope, decoding the response value into out (which may be nil). A
// business failure returns *RpcError; transport failures return a wrapped
// error with code internal.
func (c *Client) Call(ctx context.Context, method string, payload any, out any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return &RpcError{Code: ErrInternal, Message: "marshal payload: " + err.Error(), Details: []byte("{}")}
	}
	reqBody, err := json.Marshal(ClientRequest{
		Type:    "client-request",
		RpcID:   NewRpcID(),
		Method:  method,
		Payload: body,
	})
	if err != nil {
		return &RpcError{Code: ErrInternal, Message: "marshal request: " + err.Error(), Details: []byte("{}")}
	}

	u := c.baseURL + "/api/" + method
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(reqBody))
	if err != nil {
		return &RpcError{Code: ErrInternal, Message: "build request: " + err.Error(), Details: []byte("{}")}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Host", hostOf(c.baseURL))

	resp, err := c.http.Do(req)
	if err != nil {
		return &RpcError{Code: ErrInternal, Message: "transport: " + err.Error(), Details: []byte("{}")}
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 160*1024*1024))
	if err != nil {
		return &RpcError{Code: ErrInternal, Message: "read response: " + err.Error(), Details: []byte("{}")}
	}
	if resp.StatusCode != http.StatusOK {
		return &RpcError{Code: ErrInternal, Message: fmt.Sprintf("HTTP %d: %s", resp.StatusCode, truncate(respBody, 512)), Details: []byte("{}")}
	}

	var sr ServerResponse
	if err := json.Unmarshal(respBody, &sr); err != nil {
		return &RpcError{Code: ErrInternal, Message: "decode response: " + err.Error(), Details: []byte("{}")}
	}
	if !sr.Result.OK {
		if sr.Result.Error != nil {
			return sr.Result.Error
		}
		return &RpcError{Code: ErrInternal, Message: "rpc failed with no error payload", Details: []byte("{}")}
	}
	if out != nil {
		if err := json.Unmarshal(sr.Result.Value, out); err != nil {
			return &RpcError{Code: ErrInternal, Message: "decode value: " + err.Error(), Details: []byte("{}")}
		}
	}
	return nil
}

// Respond answers an answerable ServerRequest (approval/question requested).
// The rpcId must echo the server frame's id. The HTTP response is an
// RpcReceipt; a non-accepted receipt is returned as an *RpcError.
func (c *Client) Respond(ctx context.Context, rpcID string, result RpcResult) error {
	body, err := json.Marshal(ClientResponse{
		Type:   "client-response",
		RpcID:  RpcId(rpcID),
		Result: result,
	})
	if err != nil {
		return &RpcError{Code: ErrInternal, Message: "marshal respond: " + err.Error(), Details: []byte("{}")}
	}

	u := c.baseURL + "/api/respond"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(body))
	if err != nil {
		return &RpcError{Code: ErrInternal, Message: "build respond request: " + err.Error(), Details: []byte("{}")}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Host", hostOf(c.baseURL))

	resp, err := c.http.Do(req)
	if err != nil {
		return &RpcError{Code: ErrInternal, Message: "respond transport: " + err.Error(), Details: []byte("{}")}
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return &RpcError{Code: ErrInternal, Message: "read respond response: " + err.Error(), Details: []byte("{}")}
	}
	if resp.StatusCode != http.StatusOK {
		return &RpcError{Code: ErrInternal, Message: fmt.Sprintf("respond HTTP %d: %s", resp.StatusCode, truncate(respBody, 512)), Details: []byte("{}")}
	}
	var receipt RpcReceipt
	if err := json.Unmarshal(respBody, &receipt); err != nil {
		return &RpcError{Code: ErrInternal, Message: "decode receipt: " + err.Error(), Details: []byte("{}")}
	}
	if !receipt.Accepted {
		return &RpcError{Code: ErrInternal, Message: "respond not accepted: " + receipt.Reason, Details: []byte("{}")}
	}
	return nil
}

// Ready probes host.describe; it succeeds once a dsh host answers. Used both
// for startup readiness and for the connection health check.
func (c *Client) Ready(ctx context.Context) error {
	return c.Call(ctx, "host.describe", map[string]any{}, nil)
}

// hostOf returns the authority (host:port) of a base URL for the Host header.
func hostOf(baseURL string) string {
	u, err := url.Parse(baseURL)
	if err != nil {
		return strings.TrimPrefix(baseURL, "http://")
	}
	return u.Host
}

func truncate(b []byte, n int) string {
	s := strings.TrimSpace(string(b))
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}
