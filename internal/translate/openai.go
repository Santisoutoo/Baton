package translate

import (
	"encoding/json"
	"net/http"
	"strings"

	"batuta/internal/core"
)

// OpenAI translates between Anthropic Messages and the OpenAI Chat Completions
// wire format, in both directions (request rebuild + streaming SSE transform).
// It enables any OpenAI-compatible OpenCode model to be driven by Claude Code.
type OpenAI struct{}

func (OpenAI) Name() string { return "openai" }

// UpstreamPath maps the Anthropic messages path to OpenAI chat completions.
// count_tokens has no OpenAI equivalent and is intercepted by the proxy before
// we ever get here, so we still return a sane default.
func (OpenAI) UpstreamPath(inboundPath string) string {
	if strings.HasSuffix(inboundPath, "/count_tokens") {
		return "/v1/chat/completions"
	}
	return "/v1/chat/completions"
}

// TransformRequest rebuilds an Anthropic request as an OpenAI request.
func (OpenAI) TransformRequest(body []byte) ([]byte, error) {
	var req AnthropicRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, err
	}

	out := OpenAIRequest{
		Model:       req.Model,
		MaxTokens:   req.MaxTokens,
		Stream:      req.Stream,
		Temperature: req.Temperature,
		TopP:        req.TopP,
		Stop:        req.StopSequences,
	}
	if req.Stream {
		out.StreamOptions = &streamOptions{IncludeUsage: true}
	}

	// System prompt becomes a leading system message.
	if sys := systemText(req.System); sys != "" {
		out.Messages = append(out.Messages, OpenAIMessage{Role: "system", Content: sys})
	}
	out.Messages = append(out.Messages, messagesToOpenAI(req.Messages)...)

	// Tools.
	for _, t := range req.Tools {
		out.Tools = append(out.Tools, OpenAITool{
			Type: "function",
			Function: OpenAIToolFunction{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.InputSchema,
			},
		})
	}
	if tc := mapToolChoice(req.ToolChoice); tc != nil {
		out.ToolChoice = tc
	}

	return json.Marshal(out)
}

// TransformResponse is implemented in openai_stream.go.
func (o OpenAI) TransformResponse(w http.ResponseWriter, resp *http.Response) (core.Usage, error) {
	return o.transformResponse(w, resp)
}

// ---------- request-side helpers ----------

// systemText flattens an Anthropic system field (string or []block) to text.
func systemText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	if s, ok := asString(raw); ok {
		return s
	}
	var blocks []ContentBlock
	if json.Unmarshal(raw, &blocks) != nil {
		return ""
	}
	var b strings.Builder
	for _, bl := range blocks {
		if bl.Type == "text" {
			b.WriteString(bl.Text)
		}
	}
	return b.String()
}

// messagesToOpenAI expands Anthropic messages (which pack text, tool_use and
// tool_result into block arrays) into the flatter OpenAI message list.
func messagesToOpenAI(msgs []AnthropicMessage) []OpenAIMessage {
	var out []OpenAIMessage
	for _, m := range msgs {
		if s, ok := asString(m.Content); ok {
			out = append(out, OpenAIMessage{Role: m.Role, Content: s})
			continue
		}
		var blocks []ContentBlock
		if json.Unmarshal(m.Content, &blocks) != nil {
			continue
		}

		var text strings.Builder
		var toolCalls []OpenAIToolCall
		var toolResults []OpenAIMessage
		for _, b := range blocks {
			switch b.Type {
			case "text":
				text.WriteString(b.Text)
			case "image":
				// Execution model may not accept images; degrade explicitly.
				text.WriteString("\n[image omitted: not supported by the execution model]\n")
			case "tool_use":
				args := string(b.Input)
				if args == "" {
					args = "{}"
				}
				toolCalls = append(toolCalls, OpenAIToolCall{
					Index: len(toolCalls),
					ID:    b.ID,
					Type:  "function",
					Function: OpenAIToolCallFunc{
						Name:      b.Name,
						Arguments: args,
					},
				})
			case "tool_result":
				toolResults = append(toolResults, OpenAIMessage{
					Role:       "tool",
					ToolCallID: b.ToolUseID,
					Content:    toolResultText(b.Content),
				})
			}
		}

		if m.Role == "assistant" {
			msg := OpenAIMessage{Role: "assistant"}
			if text.Len() > 0 {
				msg.Content = text.String()
			}
			if len(toolCalls) > 0 {
				msg.ToolCalls = toolCalls
			}
			out = append(out, msg)
			continue
		}

		// user turn: emit tool results first (they must follow the assistant's
		// tool_calls), then any free-standing user text.
		out = append(out, toolResults...)
		if text.Len() > 0 {
			out = append(out, OpenAIMessage{Role: "user", Content: text.String()})
		}
	}
	return out
}

// toolResultText flattens an Anthropic tool_result content (string or []block).
func toolResultText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	if s, ok := asString(raw); ok {
		return s
	}
	var blocks []ContentBlock
	if json.Unmarshal(raw, &blocks) == nil {
		var b strings.Builder
		for _, bl := range blocks {
			if bl.Type == "text" {
				b.WriteString(bl.Text)
			}
		}
		if b.Len() > 0 {
			return b.String()
		}
	}
	return string(raw)
}

// mapToolChoice converts Anthropic tool_choice to the OpenAI equivalent.
func mapToolChoice(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	var tc struct {
		Type string `json:"type"`
		Name string `json:"name"`
	}
	if json.Unmarshal(raw, &tc) != nil {
		return nil
	}
	switch tc.Type {
	case "auto":
		return json.RawMessage(`"auto"`)
	case "any":
		return json.RawMessage(`"required"`)
	case "tool":
		v, _ := json.Marshal(map[string]any{
			"type":     "function",
			"function": map[string]string{"name": tc.Name},
		})
		return v
	default:
		return nil
	}
}
