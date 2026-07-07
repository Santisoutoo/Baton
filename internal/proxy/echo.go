package proxy

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"time"

	"baton/internal/logx"
)

// EchoHandler is the validation-step-0 tool. It does NOT forward anything: it
// logs (redacted) whether Claude Code sent an Authorization / x-api-key header —
// which is the one hard assumption behind "use my subscription" — and returns a
// minimal valid Anthropic message so Claude Code doesn't error. Run it with
// `baton serve --echo`, point ANTHROPIC_BASE_URL at it, and watch the logs.
type EchoHandler struct {
	log *slog.Logger
}

// NewEcho builds an echo handler.
func NewEcho(log *slog.Logger) *EchoHandler { return &EchoHandler{log: log} }

func (e *EchoHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/healthz" {
		_, _ = w.Write([]byte(`{"status":"ok"}`))
		return
	}
	body, _ := io.ReadAll(r.Body)

	e.log.Info("echo request",
		"method", r.Method,
		"path", r.URL.Path,
		"has_authorization", r.Header.Get("Authorization") != "",
		"authorization", logx.Redact(r.Header.Get("Authorization")),
		"has_x_api_key", r.Header.Get("X-Api-Key") != "",
		"x_api_key", logx.Redact(r.Header.Get("X-Api-Key")),
		"anthropic_version", r.Header.Get("Anthropic-Version"),
		"anthropic_beta", r.Header.Get("Anthropic-Beta"),
		"body_bytes", len(body),
	)

	// Return a minimal, non-streaming Anthropic message so the client is happy.
	msg := map[string]any{
		"id": "msg_echo", "type": "message", "role": "assistant",
		"model": "baton-echo",
		"content": []map[string]any{
			{"type": "text", "text": "baton echo: request received at " + time.Now().Format(time.RFC3339)},
		},
		"stop_reason": "end_turn", "stop_sequence": nil,
		"usage": map[string]int{"input_tokens": 0, "output_tokens": 0},
	}
	out, _ := json.Marshal(msg)
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(out)
}
