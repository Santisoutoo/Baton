// Package translate holds the wire-format adapters. A Translator converts an
// inbound Anthropic Messages request into an upstream format and streams the
// upstream response back in Anthropic format. This file holds the shared wire
// structs and small body helpers used across translators.
package translate

import (
	"bytes"
	"encoding/json"
)

// ---------- Anthropic Messages wire ----------

// AnthropicRequest is the subset of the /v1/messages request body baton reads
// or rewrites. Fields we don't understand are preserved when we round-trip via
// a generic map (see rewrite helpers); the struct is used by the OpenAI
// translator which rebuilds the body from scratch.
type AnthropicRequest struct {
	Model         string             `json:"model"`
	System        json.RawMessage    `json:"system,omitempty"` // string OR []ContentBlock
	Messages      []AnthropicMessage `json:"messages"`
	MaxTokens     int                `json:"max_tokens,omitempty"`
	Stream        bool               `json:"stream,omitempty"`
	Temperature   *float64           `json:"temperature,omitempty"`
	TopP          *float64           `json:"top_p,omitempty"`
	StopSequences []string           `json:"stop_sequences,omitempty"`
	Tools         []AnthropicTool    `json:"tools,omitempty"`
	ToolChoice    json.RawMessage    `json:"tool_choice,omitempty"`
}

// AnthropicMessage is one turn. Content is a string or an array of blocks.
type AnthropicMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

// AnthropicTool is a tool definition (name + JSON schema).
type AnthropicTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"input_schema"`
}

// ContentBlock covers the block shapes we translate: text, image, tool_use,
// tool_result. Unknown types are handled leniently by the translator.
type ContentBlock struct {
	Type string `json:"type"`
	// text
	Text string `json:"text,omitempty"`
	// tool_use
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`
	// tool_result
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content   json.RawMessage `json:"content,omitempty"`
	IsError   bool            `json:"is_error,omitempty"`
	// image (source.data) — carried so we can detect and degrade gracefully
	Source json.RawMessage `json:"source,omitempty"`
}

// ---------- OpenAI Chat Completions wire ----------

type OpenAIRequest struct {
	Model         string          `json:"model"`
	Messages      []OpenAIMessage `json:"messages"`
	MaxTokens     int             `json:"max_tokens,omitempty"`
	Stream        bool            `json:"stream,omitempty"`
	Temperature   *float64        `json:"temperature,omitempty"`
	TopP          *float64        `json:"top_p,omitempty"`
	Stop          []string        `json:"stop,omitempty"`
	Tools         []OpenAITool    `json:"tools,omitempty"`
	ToolChoice    json.RawMessage `json:"tool_choice,omitempty"`
	StreamOptions *streamOptions  `json:"stream_options,omitempty"`
}

type streamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

type OpenAIMessage struct {
	Role       string           `json:"role"`
	Content    any              `json:"content,omitempty"` // string for most; omitted for assistant tool calls
	Name       string           `json:"name,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
	ToolCalls  []OpenAIToolCall `json:"tool_calls,omitempty"`
}

type OpenAITool struct {
	Type     string             `json:"type"` // "function"
	Function OpenAIToolFunction `json:"function"`
}

type OpenAIToolFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

type OpenAIToolCall struct {
	Index    int                `json:"index"`
	ID       string             `json:"id,omitempty"`
	Type     string             `json:"type,omitempty"` // "function"
	Function OpenAIToolCallFunc `json:"function"`
}

type OpenAIToolCallFunc struct {
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

// ---------- body helpers ----------

// PeekModel returns the top-level "model" and "stream" fields without fully
// unmarshalling the (possibly large) body.
func PeekModel(body []byte) (model string, stream bool) {
	var probe struct {
		Model  string `json:"model"`
		Stream bool   `json:"stream"`
	}
	_ = json.Unmarshal(body, &probe)
	return probe.Model, probe.Stream
}

// RewriteModel returns body with its top-level "model" set to newModel,
// preserving every other field (including ones baton doesn't model).
func RewriteModel(body []byte, newModel string) ([]byte, error) {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, err
	}
	nm, err := json.Marshal(newModel)
	if err != nil {
		return nil, err
	}
	m["model"] = nm
	return json.Marshal(m)
}

// StripUnsupported removes Anthropic-only fields that OpenAI-compatible upstreams
// choke on (prompt caching markers, extended thinking, metadata). It edits the
// generic map so unrelated fields survive. Used before OpenAI translation as a
// belt-and-suspenders step; the struct rebuild also naturally drops them.
func StripUnsupported(body []byte) []byte {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(body, &m); err != nil {
		return body
	}
	for _, k := range []string{"thinking", "metadata", "anthropic_version", "anthropic_beta"} {
		delete(m, k)
	}
	out, err := json.Marshal(m)
	if err != nil {
		return body
	}
	return out
}

// asString reports whether raw is a JSON string and returns its value.
func asString(raw json.RawMessage) (string, bool) {
	if len(raw) == 0 {
		return "", false
	}
	if raw[0] != '"' {
		return "", false
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return "", false
	}
	return s, true
}

// compactJSON trims insignificant whitespace; handy for stable golden output.
func compactJSON(b []byte) []byte {
	var buf bytes.Buffer
	if err := json.Compact(&buf, b); err != nil {
		return b
	}
	return buf.Bytes()
}
