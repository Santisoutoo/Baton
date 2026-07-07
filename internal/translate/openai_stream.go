package translate

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"baton/internal/core"
)

// transformResponse turns an OpenAI Chat Completions response into an Anthropic
// Messages response: an SSE state machine for streaming, a single message build
// for non-streaming, and an error-shape translation for failures.
func (OpenAI) transformResponse(w http.ResponseWriter, resp *http.Response) (core.Usage, error) {
	var usage core.Usage

	if resp.StatusCode >= 400 {
		return usage, writeAnthropicError(w, resp)
	}
	if !isEventStream(resp.Header) {
		return transformOpenAINonStream(w, resp)
	}
	return transformOpenAIStream(w, resp)
}

// --- non-streaming ---

type openaiCompletion struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Choices []struct {
		Message struct {
			Content   string           `json:"content"`
			ToolCalls []OpenAIToolCall `json:"tool_calls"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
}

func transformOpenAINonStream(w http.ResponseWriter, resp *http.Response) (core.Usage, error) {
	var usage core.Usage
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return usage, err
	}
	var c openaiCompletion
	if err := json.Unmarshal(body, &c); err != nil {
		return usage, err
	}

	var content []map[string]any
	stop := "end_turn"
	if len(c.Choices) > 0 {
		ch := c.Choices[0]
		if ch.Message.Content != "" {
			content = append(content, map[string]any{"type": "text", "text": ch.Message.Content})
		}
		for _, tc := range ch.Message.ToolCalls {
			var input any
			if tc.Function.Arguments != "" {
				_ = json.Unmarshal([]byte(tc.Function.Arguments), &input)
			}
			if input == nil {
				input = map[string]any{}
			}
			content = append(content, map[string]any{
				"type": "tool_use", "id": tc.ID, "name": tc.Function.Name, "input": input,
			})
		}
		stop = mapFinishReason(ch.FinishReason)
	}

	usage = core.Usage{InputTokens: c.Usage.PromptTokens, OutputTokens: c.Usage.CompletionTokens}
	msg := map[string]any{
		"id":            msgID(c.ID),
		"type":          "message",
		"role":          "assistant",
		"model":         c.Model,
		"content":       content,
		"stop_reason":   stop,
		"stop_sequence": nil,
		"usage": map[string]int{
			"input_tokens":  usage.InputTokens,
			"output_tokens": usage.OutputTokens,
		},
	}
	out, _ := json.Marshal(msg)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(out)
	return usage, nil
}

// --- streaming state machine ---

type openaiChunk struct {
	Choices []struct {
		Delta struct {
			Content   string           `json:"content"`
			ToolCalls []OpenAIToolCall `json:"tool_calls"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
}

func transformOpenAIStream(w http.ResponseWriter, resp *http.Response) (core.Usage, error) {
	var usage core.Usage
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher, _ := w.(http.Flusher)

	id := msgID("")
	writeSSE(w, flusher, "message_start", map[string]any{
		"type": "message_start",
		"message": map[string]any{
			"id": id, "type": "message", "role": "assistant", "model": "",
			"content": []any{}, "stop_reason": nil, "stop_sequence": nil,
			"usage": map[string]int{"input_tokens": 0, "output_tokens": 0},
		},
	})

	blockIndex := -1
	blockKind := "" // "" | "text" | "tool"
	openToolOAIndex := -1
	stopReason := "end_turn"

	closeBlock := func() {
		if blockKind != "" {
			writeSSE(w, flusher, "content_block_stop", map[string]any{
				"type": "content_block_stop", "index": blockIndex,
			})
			blockKind = ""
		}
	}

	reader := bufio.NewReader(resp.Body)
	for {
		line, err := reader.ReadString('\n')
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "data:") {
			data := strings.TrimSpace(trimmed[len("data:"):])
			if data == "[DONE]" {
				break
			}
			var ch openaiChunk
			if json.Unmarshal([]byte(data), &ch) == nil {
				if ch.Usage != nil {
					usage.InputTokens = ch.Usage.PromptTokens
					usage.OutputTokens = ch.Usage.CompletionTokens
				}
				for _, c := range ch.Choices {
					if c.Delta.Content != "" {
						if blockKind != "text" {
							closeBlock()
							blockIndex++
							blockKind = "text"
							writeSSE(w, flusher, "content_block_start", map[string]any{
								"type": "content_block_start", "index": blockIndex,
								"content_block": map[string]any{"type": "text", "text": ""},
							})
						}
						writeSSE(w, flusher, "content_block_delta", map[string]any{
							"type": "content_block_delta", "index": blockIndex,
							"delta": map[string]any{"type": "text_delta", "text": c.Delta.Content},
						})
					}
					for _, tc := range c.Delta.ToolCalls {
						// A new tool call starts when its name arrives (or the
						// OpenAI index advances). Then arguments stream as
						// input_json_delta.
						if tc.Function.Name != "" && (blockKind != "tool" || tc.Index != openToolOAIndex) {
							closeBlock()
							blockIndex++
							blockKind = "tool"
							openToolOAIndex = tc.Index
							writeSSE(w, flusher, "content_block_start", map[string]any{
								"type": "content_block_start", "index": blockIndex,
								"content_block": map[string]any{
									"type": "tool_use", "id": tc.ID, "name": tc.Function.Name,
									"input": map[string]any{},
								},
							})
						}
						if tc.Function.Arguments != "" {
							writeSSE(w, flusher, "content_block_delta", map[string]any{
								"type": "content_block_delta", "index": blockIndex,
								"delta": map[string]any{
									"type": "input_json_delta", "partial_json": tc.Function.Arguments,
								},
							})
						}
					}
					if c.FinishReason != nil {
						stopReason = mapFinishReason(*c.FinishReason)
					}
				}
			}
		}
		if err != nil {
			break
		}
	}

	closeBlock()
	writeSSE(w, flusher, "message_delta", map[string]any{
		"type":  "message_delta",
		"delta": map[string]any{"stop_reason": stopReason, "stop_sequence": nil},
		"usage": map[string]int{"output_tokens": usage.OutputTokens},
	})
	writeSSE(w, flusher, "message_stop", map[string]any{"type": "message_stop"})
	return usage, nil
}

// --- helpers ---

func writeSSE(w http.ResponseWriter, flusher http.Flusher, event string, data any) {
	b, err := json.Marshal(data)
	if err != nil {
		return
	}
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, b)
	if flusher != nil {
		flusher.Flush()
	}
}

func writeAnthropicError(w http.ResponseWriter, resp *http.Response) error {
	body, _ := io.ReadAll(resp.Body)
	msg := strings.TrimSpace(string(body))
	if msg == "" {
		msg = http.StatusText(resp.StatusCode)
	}
	env := map[string]any{
		"type": "error",
		"error": map[string]any{
			"type":    "upstream_error",
			"message": msg,
		},
	}
	out, _ := json.Marshal(env)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	_, err := w.Write(out)
	return err
}

func mapFinishReason(r string) string {
	switch r {
	case "stop":
		return "end_turn"
	case "length":
		return "max_tokens"
	case "tool_calls", "function_call":
		return "tool_use"
	case "content_filter":
		return "end_turn"
	default:
		return "end_turn"
	}
}

func msgID(upstream string) string {
	if strings.HasPrefix(upstream, "msg_") {
		return upstream
	}
	return fmt.Sprintf("msg_%d", time.Now().UnixNano())
}
