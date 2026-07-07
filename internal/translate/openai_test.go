package translate

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTransformRequest_SystemUserTools(t *testing.T) {
	in := `{
		"model": "deepseek-v4-flash",
		"system": "You are terse.",
		"max_tokens": 100,
		"stream": true,
		"messages": [
			{"role": "user", "content": "hola"},
			{"role": "assistant", "content": [
				{"type": "text", "text": "hi"},
				{"type": "tool_use", "id": "t1", "name": "get_weather", "input": {"city": "vigo"}}
			]},
			{"role": "user", "content": [
				{"type": "tool_result", "tool_use_id": "t1", "content": "sunny"}
			]}
		],
		"tools": [
			{"name": "get_weather", "description": "w", "input_schema": {"type": "object"}}
		],
		"tool_choice": {"type": "auto"}
	}`

	out, err := OpenAI{}.TransformRequest([]byte(in))
	if err != nil {
		t.Fatalf("TransformRequest: %v", err)
	}

	var req OpenAIRequest
	if err := json.Unmarshal(out, &req); err != nil {
		t.Fatalf("unmarshal openai req: %v", err)
	}

	if req.Model != "deepseek-v4-flash" {
		t.Errorf("model = %q", req.Model)
	}
	if !req.Stream || req.StreamOptions == nil || !req.StreamOptions.IncludeUsage {
		t.Errorf("stream options not set for a streaming request")
	}
	if len(req.Messages) < 4 {
		t.Fatalf("want >=4 messages (system,user,assistant,tool), got %d", len(req.Messages))
	}
	if req.Messages[0].Role != "system" || req.Messages[0].Content != "You are terse." {
		t.Errorf("system message wrong: %+v", req.Messages[0])
	}
	// Find the assistant tool call and the tool result.
	var sawToolCall, sawToolResult bool
	for _, m := range req.Messages {
		if m.Role == "assistant" && len(m.ToolCalls) == 1 {
			if m.ToolCalls[0].Function.Name == "get_weather" {
				sawToolCall = true
			}
		}
		if m.Role == "tool" && m.ToolCallID == "t1" {
			if s, _ := m.Content.(string); s == "sunny" {
				sawToolResult = true
			}
		}
	}
	if !sawToolCall {
		t.Error("assistant tool_call not translated")
	}
	if !sawToolResult {
		t.Error("tool_result not translated to a tool message")
	}
	if len(req.Tools) != 1 || req.Tools[0].Function.Name != "get_weather" {
		t.Errorf("tools not translated: %+v", req.Tools)
	}
	if string(req.ToolChoice) != `"auto"` {
		t.Errorf("tool_choice = %s", req.ToolChoice)
	}
}

func TestTransformResponse_StreamTextAndTool(t *testing.T) {
	// A minimal OpenAI streaming response: some text, then a tool call, then done.
	sse := strings.Join([]string{
		`data: {"choices":[{"delta":{"role":"assistant","content":"Hel"}}]}`,
		`data: {"choices":[{"delta":{"content":"lo"}}]}`,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"c1","function":{"name":"do","arguments":"{\"x\":"}}]}}]}`,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"1}"}}]},"finish_reason":"tool_calls"}]}`,
		`data: {"usage":{"prompt_tokens":11,"completion_tokens":7}}`,
		`data: [DONE]`,
		``,
	}, "\n\n")

	resp := &http.Response{
		StatusCode: 200,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(sse)),
	}
	rec := httptest.NewRecorder()

	usage, err := OpenAI{}.TransformResponse(rec, resp)
	if err != nil {
		t.Fatalf("TransformResponse: %v", err)
	}
	body := rec.Body.String()

	for _, want := range []string{
		"event: message_start",
		`"text":"Hel"`,
		`"text":"lo"`,
		`"text_delta"`,
		`"type":"tool_use"`,
		`"input_json_delta"`,
		`"stop_reason":"tool_use"`,
		"event: message_stop",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("stream output missing %q\n---\n%s", want, body)
		}
	}
	if usage.InputTokens != 11 || usage.OutputTokens != 7 {
		t.Errorf("usage = %+v, want in=11 out=7", usage)
	}
}

func TestTransformResponse_ErrorBecomesAnthropicError(t *testing.T) {
	resp := &http.Response{
		StatusCode: 429,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"rate limited"}}`)),
	}
	rec := httptest.NewRecorder()
	if _, err := (OpenAI{}).TransformResponse(rec, resp); err != nil {
		t.Fatalf("TransformResponse: %v", err)
	}
	if rec.Code != 429 {
		t.Errorf("status = %d, want 429", rec.Code)
	}
	var env struct {
		Type  string `json:"type"`
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal error env: %v", err)
	}
	if env.Type != "error" {
		t.Errorf("error envelope type = %q", env.Type)
	}
}
