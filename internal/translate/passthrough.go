package translate

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"net/http"

	"baton/internal/core"
)

// Passthrough is the identity wire adapter: Anthropic in, Anthropic out. It is
// used for the Claude lane (subscription passthrough) and for OpenCode's
// Anthropic-native models (MiniMax/Qwen on Go), where only host+auth+model
// change and the body is never rewritten. Because there is no body transform,
// this is the lowest-overhead path.
type Passthrough struct{}

func (Passthrough) Name() string { return "passthrough" }

// UpstreamPath keeps the inbound Anthropic path unchanged.
func (Passthrough) UpstreamPath(inboundPath string) string { return inboundPath }

// TransformRequest is a no-op: the proxy already applied any model rewrite.
func (Passthrough) TransformRequest(body []byte) ([]byte, error) { return body, nil }

// TransformResponse copies the upstream response to the client verbatim,
// flushing per line for SSE, and extracts token usage as it passes through.
func (Passthrough) TransformResponse(w http.ResponseWriter, resp *http.Response) (core.Usage, error) {
	var usage core.Usage
	copyHeaders(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)

	if !isEventStream(resp.Header) {
		// Non-streaming: buffer, forward, parse usage from the JSON body.
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return usage, err
		}
		_, _ = w.Write(body)
		usage.InputTokens, usage.OutputTokens = usageFromMessageJSON(body)
		return usage, nil
	}

	flusher, _ := w.(http.Flusher)
	reader := bufio.NewReader(resp.Body)
	for {
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			if _, werr := w.Write(line); werr != nil {
				return usage, werr
			}
			scanUsageFromSSELine(line, &usage)
			if flusher != nil {
				flusher.Flush()
			}
		}
		if err != nil {
			if err == io.EOF {
				return usage, nil
			}
			return usage, err
		}
	}
}

// scanUsageFromSSELine updates usage from a single "data: {...}" SSE line of an
// Anthropic stream. input_tokens arrive in message_start, output_tokens grow in
// message_delta events (we keep the latest).
func scanUsageFromSSELine(line []byte, usage *core.Usage) {
	data := bytes.TrimSpace(line)
	if !bytes.HasPrefix(data, []byte("data:")) {
		return
	}
	data = bytes.TrimSpace(data[len("data:"):])
	if len(data) == 0 || data[0] != '{' {
		return
	}
	var ev struct {
		Type    string `json:"type"`
		Message struct {
			Usage struct {
				InputTokens  int `json:"input_tokens"`
				OutputTokens int `json:"output_tokens"`
			} `json:"usage"`
		} `json:"message"`
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}
	if json.Unmarshal(data, &ev) != nil {
		return
	}
	if ev.Message.Usage.InputTokens > 0 {
		usage.InputTokens = ev.Message.Usage.InputTokens
	}
	if ev.Message.Usage.OutputTokens > 0 {
		usage.OutputTokens = ev.Message.Usage.OutputTokens
	}
	if ev.Usage.InputTokens > 0 {
		usage.InputTokens = ev.Usage.InputTokens
	}
	if ev.Usage.OutputTokens > 0 {
		usage.OutputTokens = ev.Usage.OutputTokens
	}
}

// usageFromMessageJSON pulls usage from a non-streaming Anthropic message body.
func usageFromMessageJSON(body []byte) (in, out int) {
	var m struct {
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}
	_ = json.Unmarshal(body, &m)
	return m.Usage.InputTokens, m.Usage.OutputTokens
}

// ---------- shared response helpers ----------

func isEventStream(h http.Header) bool {
	ct := h.Get("Content-Type")
	return bytes.Contains([]byte(ct), []byte("text/event-stream"))
}

// copyHeaders copies upstream headers to the client, skipping hop-by-hop ones.
func copyHeaders(dst, src http.Header) {
	for k, vs := range src {
		switch http.CanonicalHeaderKey(k) {
		case "Connection", "Keep-Alive", "Proxy-Authenticate", "Proxy-Authorization",
			"Te", "Trailer", "Transfer-Encoding", "Upgrade", "Content-Length":
			continue
		}
		for _, v := range vs {
			dst.Add(k, v)
		}
	}
}
