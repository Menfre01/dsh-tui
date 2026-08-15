package dsh

import (
	"context"
	"encoding/json"
	"fmt"
)

// jsonUnmarshal is a short alias so raw-value mapping reads stay terse.
func jsonUnmarshal(b []byte, v any) error { return json.Unmarshal(b, v) }

// ---------------------------------------------------------------------------
// host.* / agentPreset.* / skill.* / subagent.* RPC methods (spike-relevant
// subset; the remaining domains land with their UI in later stages).
// ---------------------------------------------------------------------------

// HostDescribeValue is the host.describe response: a one-shot host snapshot.
type HostDescribeValue struct {
	Version          string `json:"version"`
	Cwd              string `json:"cwd"`
	Provider         string `json:"provider,omitempty"`
	Model            string `json:"model,omitempty"`
	AttachedSessions int    `json:"attachedSessions"`
	CanOpenPath      bool   `json:"canOpenPath"`
}

// Describe calls host.describe and caches the version.
func (c *Client) Describe(ctx context.Context) (*HostDescribeValue, error) {
	var out HostDescribeValue
	if err := c.Call(ctx, "host.describe", map[string]any{}, &out); err != nil {
		return nil, err
	}
	c.version = out.Version
	return &out, nil
}

// AgentPreset is one agent preset row (agentPreset.list). The schema is
// permissive: the list item's exact fields are owned by the preset domain.
type AgentPreset struct {
	ID          string          `json:"id"`
	Name        string          `json:"name,omitempty"`
	Description string          `json:"description,omitempty"`
	Raw         jsonRaw         `json:"-"`
}

// UnmarshalJSON keeps the full preset view available for later stages.
func (p *AgentPreset) UnmarshalJSON(b []byte) error {
	type alias AgentPreset
	var a alias
	if err := jsonUnmarshal(b, &a); err != nil {
		return err
	}
	*p = AgentPreset(a)
	p.Raw = append([]byte(nil), b...)
	return nil
}

// AgentPresetListValue is the agentPreset.list response. The upstream value
// shape is a plain array of preset rows; decode permissively.
type AgentPresetListValue struct {
	Presets []AgentPreset `json:"presets"`
}

// ListAgentPresets calls agentPreset.list.
func (c *Client) ListAgentPresets(ctx context.Context) (*AgentPresetListValue, error) {
	// The wire value is a JSON array of preset rows; wrap it into the
	// Presets field by decoding raw and mapping.
	var raw []jsonRaw
	if err := c.Call(ctx, "agentPreset.list", map[string]any{}, &raw); err != nil {
		return nil, err
	}
	out := &AgentPresetListValue{Presets: make([]AgentPreset, 0, len(raw))}
	for _, r := range raw {
		var p AgentPreset
		if err := jsonUnmarshal(r, &p); err != nil {
			return nil, err
		}
		out.Presets = append(out.Presets, p)
	}
	return out, nil
}

// SelectAgentPreset calls agentPreset.select for the given session.
func (c *Client) SelectAgentPreset(ctx context.Context, sessionID, preset string) error {
	return c.Call(ctx, "agentPreset.select", map[string]string{
		"sessionId":   sessionID,
		"agentPreset": preset,
	}, nil)
}

// Skill is one skill row (skill.list).
type Skill struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Raw         jsonRaw         `json:"-"`
}

// ListSkills calls skill.list.
func (c *Client) ListSkills(ctx context.Context) ([]Skill, error) {
	var raw []jsonRaw
	if err := c.Call(ctx, "skill.list", map[string]any{}, &raw); err != nil {
		return nil, err
	}
	out := make([]Skill, 0, len(raw))
	for _, r := range raw {
		var s Skill
		if err := jsonUnmarshal(r, &s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
}

// SubagentRef identifies one child session of a parent.
type SubagentRef struct {
	SessionID string `json:"sessionId"`
	Raw       jsonRaw `json:"-"`
}

// ListSubagents calls subagent.list for a parent session.
func (c *Client) ListSubagents(ctx context.Context, parentSessionID string) ([]SubagentRef, error) {
	var raw []jsonRaw
	if err := c.Call(ctx, "subagent.list", map[string]string{
		"parentSessionId": parentSessionID,
	}, &raw); err != nil {
		return nil, err
	}
	out := make([]SubagentRef, 0, len(raw))
	for _, r := range raw {
		var s SubagentRef
		if err := jsonUnmarshal(r, &s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
}

// SubagentPromptRequest steers a subagent session.
type SubagentPromptRequest struct {
	ParentSessionID string `json:"parentSessionId"`
	ChildSessionID  string `json:"childSessionId"`
	Content         string `json:"content"`
}

// SubagentPrompt calls subagent.prompt.
func (c *Client) SubagentPrompt(ctx context.Context, req SubagentPromptRequest) error {
	return c.Call(ctx, "subagent.prompt", req, nil)
}

// SubagentInterrupt cancels a running subagent.
func (c *Client) SubagentInterrupt(ctx context.Context, parentSessionID, childSessionID string) error {
	return c.Call(ctx, "subagent.interrupt", map[string]string{
		"parentSessionId": parentSessionID,
		"childSessionId":  childSessionID,
	}, nil)
}

// SettingsValue 读取某 namespace 的标量配置值(settings.describe)。
// 用于对齐宿主/Web 行为,如 ui-conversation.busyEnter(queue|steer)。
func (c *Client) SettingsValue(ctx context.Context, ns, key string) (string, error) {
	var out struct {
		Writable   bool `json:"writable"`
		Namespaces []struct {
			NS    string                     `json:"ns"`
			Value map[string]json.RawMessage `json:"value"`
		} `json:"namespaces"`
	}
	if err := c.Call(ctx, "settings.describe", map[string]any{}, &out); err != nil {
		return "", err
	}
	for _, n := range out.Namespaces {
		if n.NS != ns {
			continue
		}
		if raw, ok := n.Value[key]; ok {
			var s string
			if err := json.Unmarshal(raw, &s); err == nil {
				return s, nil
			}
		}
		return "", fmt.Errorf("setting %s/%s: not a scalar string", ns, key)
	}
	return "", fmt.Errorf("setting %s/%s: namespace not found", ns, key)
}
