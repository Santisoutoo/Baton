package proxy

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"baton/internal/config"
	"baton/internal/core"
	"baton/internal/logx"
	"baton/internal/registry"
	"baton/internal/router"
)

type fakeCreds struct{}

func (fakeCreds) Get(string) (string, error) { return "testkey", nil }
func (fakeCreds) Set(string, string) error   { return nil }
func (fakeCreds) Delete(string) error        { return nil }

type fakeMeter struct {
	mu   sync.Mutex
	rows []core.Usage
}

func (m *fakeMeter) Record(u core.Usage) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rows = append(m.rows, u)
	return nil
}
func (m *fakeMeter) Query(core.Filter) ([]core.Aggregate, error) { return nil, nil }
func (m *fakeMeter) Close() error                                { return nil }

func buildHandler(t *testing.T, cfg *config.Config) *Handler {
	t.Helper()
	reg, err := registry.New(cfg, fakeCreds{})
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	h := New(cfg, router.New(cfg), reg, &fakeMeter{}, logx.New("error"))
	h.maxAttempts = 1 // keep tests fast; no backoff loops
	return h
}

// The execution lane (OpenAI-compatible) streams and gets translated to Anthropic.
func TestProxy_ExecuteLaneStreamsAndTranslates(t *testing.T) {
	exec := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/v1/chat/completions") {
			t.Errorf("unexpected upstream path %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		fl, _ := w.(http.Flusher)
		for _, line := range []string{
			`data: {"choices":[{"delta":{"content":"hi"}}]}`,
			`data: {"choices":[{"delta":{},"finish_reason":"stop"}]}`,
			`data: {"usage":{"prompt_tokens":5,"completion_tokens":2}}`,
			`data: [DONE]`,
		} {
			w.Write([]byte(line + "\n\n"))
			if fl != nil {
				fl.Flush()
			}
		}
	}))
	defer exec.Close()

	cfg := config.Defaults()
	be := cfg.Backends["opencode"]
	be.BaseURL = exec.URL
	cfg.Backends["opencode"] = be
	rc := cfg.Roles["execute"]
	rc.Model = "m"
	cfg.Roles["execute"] = rc

	h := buildHandler(t, cfg)

	req := httptest.NewRequest(http.MethodPost, "/v1/messages",
		strings.NewReader(`{"model":"claude-haiku-4-5","stream":true,"max_tokens":10,"messages":[{"role":"user","content":"hi"}]}`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	body := rec.Body.String()
	for _, want := range []string{"event: message_start", `"text":"hi"`, "event: message_stop"} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q in translated stream:\n%s", want, body)
		}
	}
}

// When the execution lane fails, the request falls back to the Claude lane.
func TestProxy_FallbackToClaudeOnExecFailure(t *testing.T) {
	exec := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer exec.Close()

	plan := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/v1/messages") {
			t.Errorf("fallback hit wrong path %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"msg_1","type":"message","role":"assistant","content":[{"type":"text","text":"from-claude"}],"usage":{"input_tokens":3,"output_tokens":4}}`))
	}))
	defer plan.Close()

	cfg := config.Defaults()
	cfg.OnExecError = "fallback-claude"
	ex := cfg.Backends["opencode"]
	ex.BaseURL = exec.URL
	cfg.Backends["opencode"] = ex
	an := cfg.Backends["anthropic"]
	an.BaseURL = plan.URL
	cfg.Backends["anthropic"] = an
	rc := cfg.Roles["execute"]
	rc.Model = "m"
	cfg.Roles["execute"] = rc

	h := buildHandler(t, cfg)

	req := httptest.NewRequest(http.MethodPost, "/v1/messages",
		strings.NewReader(`{"model":"claude-haiku-4-5","max_tokens":10,"messages":[{"role":"user","content":"hi"}]}`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if !strings.Contains(rec.Body.String(), "from-claude") {
		t.Errorf("expected fallback response from claude lane, got:\n%s", rec.Body.String())
	}
}

// count_tokens on the OpenAI lane is answered locally, not forwarded.
func TestProxy_CountTokensLocalOnOpenAILane(t *testing.T) {
	cfg := config.Defaults()
	rc := cfg.Roles["execute"]
	rc.Model = "m"
	cfg.Roles["execute"] = rc
	h := buildHandler(t, cfg)

	req := httptest.NewRequest(http.MethodPost, "/v1/messages/count_tokens",
		strings.NewReader(`{"model":"claude-haiku-4-5","messages":[{"role":"user","content":"hello world"}]}`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if !strings.Contains(rec.Body.String(), "input_tokens") {
		t.Errorf("count_tokens should return input_tokens, got: %s", rec.Body.String())
	}
}
