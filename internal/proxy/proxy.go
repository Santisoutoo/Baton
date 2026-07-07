// Package proxy is the HTTP front door. It receives Anthropic-format traffic from
// Claude Code, asks the Router where each request goes, sends it upstream through
// the chosen Backend+Translator, applies the fallback policy on execution-lane
// failures, and records usage. It orchestrates the core interfaces and knows no
// provider specifics.
package proxy

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"time"

	"baton/internal/config"
	"baton/internal/core"
	"baton/internal/translate"
)

// Handler implements http.Handler.
type Handler struct {
	cfg         *config.Config
	router      core.Router
	reg         core.Registry
	meter       core.MeterStore
	log         *slog.Logger
	client      *http.Client
	planBackend string
	maxAttempts int
}

// New builds the proxy handler.
func New(cfg *config.Config, router core.Router, reg core.Registry, meter core.MeterStore, log *slog.Logger) *Handler {
	plan := "anthropic"
	if rc, ok := cfg.Roles["plan"]; ok && rc.Backend != "" {
		plan = rc.Backend
	}
	return &Handler{
		cfg:         cfg,
		router:      router,
		reg:         reg,
		meter:       meter,
		log:         log,
		client:      &http.Client{}, // no timeout: streaming responses run long
		planBackend: plan,
		maxAttempts: 3,
	}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path == "/healthz":
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	case r.Method == http.MethodPost:
		h.handleMessages(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *Handler) handleMessages(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "cannot read request body")
		return
	}
	model, stream := translate.PeekModel(body)

	route, err := h.router.Route(model)
	if err != nil {
		h.log.Error("routing failed", "model", model, "err", err)
		h.writeError(w, http.StatusInternalServerError, "routing: "+err.Error())
		return
	}

	// count_tokens has no OpenAI equivalent: answer locally on that lane.
	if isCountTokens(r.URL.Path) && route.Translator == "openai" {
		h.writeCountTokens(w, body)
		return
	}

	start := time.Now()
	resp, usedRoute, err := h.sendWithFallback(route, body, r)
	if err != nil {
		h.log.Error("upstream failed", "backend", usedRoute.Backend, "err", err)
		h.writeError(w, http.StatusBadGateway, "upstream: "+err.Error())
		return
	}
	defer resp.Body.Close()

	translator, ok := h.reg.Translator(usedRoute.Translator)
	if !ok {
		h.writeError(w, http.StatusInternalServerError, "unknown translator "+usedRoute.Translator)
		return
	}
	usage, terr := translator.TransformResponse(w, resp)

	effModel := model
	if !usedRoute.KeepInboundModel() {
		effModel = usedRoute.Model
	}
	usage.Timestamp = time.Now()
	usage.Backend = usedRoute.Backend
	usage.Model = effModel
	usage.Role = usedRoute.Role
	h.record(usage)

	h.log.Info("request",
		"lane", usedRoute.Role, "backend", usedRoute.Backend, "model", effModel,
		"stream", stream, "status", resp.StatusCode,
		"in_tokens", usage.InputTokens, "out_tokens", usage.OutputTokens,
		"ttfb_ms", time.Since(start).Milliseconds(), "err", terr,
	)
}

// sendWithFallback sends the request, and if the execution lane fails with a
// retriable/limit status and policy allows, retries once via the Claude lane so
// the session never breaks. Returns the response and the route actually used.
func (h *Handler) sendWithFallback(route core.Route, body []byte, r *http.Request) (*http.Response, core.Route, error) {
	resp, err := h.sendUpstream(route, body, r)

	eligible := route.Role == "execute" && h.cfg.OnExecError == "fallback-claude"
	failed := err != nil || (resp != nil && (resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500))
	if eligible && failed {
		if resp != nil {
			resp.Body.Close()
		}
		h.log.Warn("execute lane failed; falling back to Claude", "backend", route.Backend)
		fb := core.Route{Role: "execute-fallback", Backend: h.planBackend, Translator: "passthrough", Model: ""}
		fbResp, fbErr := h.sendUpstream(fb, body, r)
		return fbResp, fb, fbErr
	}
	return resp, route, err
}

// sendUpstream transforms the request for the route's wire format and sends it,
// retrying transient 429/5xx with a short backoff.
func (h *Handler) sendUpstream(route core.Route, body []byte, r *http.Request) (*http.Response, error) {
	translator, ok := h.reg.Translator(route.Translator)
	if !ok {
		return nil, errString("unknown translator " + route.Translator)
	}
	backend, ok := h.reg.Backend(route.Backend)
	if !ok {
		return nil, errString("unknown backend " + route.Backend)
	}

	b := body
	if !route.KeepInboundModel() {
		if rb, err := translate.RewriteModel(b, route.Model); err == nil {
			b = rb
		}
	}
	if route.Translator == "openai" {
		b = translate.StripUnsupported(b)
	}
	ub, err := translator.TransformRequest(b)
	if err != nil {
		return nil, err
	}
	url := backend.BaseURL() + translator.UpstreamPath(r.URL.Path)
	inboundAuth := inboundAuth(r)

	var resp *http.Response
	for attempt := 0; attempt < h.maxAttempts; attempt++ {
		req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, url, bytes.NewReader(ub))
		if err != nil {
			return nil, err
		}
		setUpstreamHeaders(req, r)
		backend.Authorize(req, inboundAuth)

		resp, err = h.client.Do(req)
		if err == nil && resp.StatusCode != http.StatusTooManyRequests && resp.StatusCode < 500 {
			return resp, nil
		}
		if resp != nil {
			resp.Body.Close()
		}
		if attempt < h.maxAttempts-1 {
			time.Sleep(backoff(attempt))
		}
	}
	return resp, err
}

func (h *Handler) writeCountTokens(w http.ResponseWriter, body []byte) {
	out, _ := json.Marshal(map[string]int{"input_tokens": translate.EstimateInputTokens(body)})
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(out)
}

func (h *Handler) writeError(w http.ResponseWriter, status int, msg string) {
	env := map[string]any{"type": "error", "error": map[string]any{"type": "baton_error", "message": msg}}
	out, _ := json.Marshal(env)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(out)
}

func (h *Handler) record(u core.Usage) {
	if h.meter == nil {
		return
	}
	go func() {
		if err := h.meter.Record(u); err != nil {
			h.log.Debug("meter record failed", "err", err)
		}
	}()
}

// ---------- helpers ----------

// setUpstreamHeaders copies the inbound headers upstream that providers care
// about. Auth headers are copied here and then finalized by backend.Authorize:
// the passthrough backend keeps them (forwarding the subscription token), the
// api-key backend deletes and replaces them.
func setUpstreamHeaders(req, in *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	for _, k := range []string{"Accept", "Anthropic-Version", "Anthropic-Beta", "Authorization", "X-Api-Key", "User-Agent"} {
		if v := in.Header.Get(k); v != "" {
			req.Header.Set(k, v)
		}
	}
}

// inboundAuth returns the auth token the client presented, preferring the
// Authorization header (subscription bearer) and falling back to x-api-key.
func inboundAuth(r *http.Request) string {
	if v := r.Header.Get("Authorization"); v != "" {
		return v
	}
	if v := r.Header.Get("X-Api-Key"); v != "" {
		return v
	}
	return ""
}

func isCountTokens(path string) bool {
	return len(path) >= len("/count_tokens") && path[len(path)-len("/count_tokens"):] == "/count_tokens"
}

func backoff(attempt int) time.Duration {
	// 200ms, 400ms, ...
	return time.Duration(200*(1<<attempt)) * time.Millisecond
}

type errString string

func (e errString) Error() string { return string(e) }
