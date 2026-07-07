// Package core defines the stable contracts (interfaces + shared types) that the
// rest of baton is built on. Everything else depends on this package; this
// package depends on nothing but the standard library. Adding a new provider or
// swapping an implementation means satisfying one of these interfaces — the core
// never learns concrete provider names.
package core

import (
	"net/http"
	"time"
)

// Route is the decision of where a single request goes. It is produced by a
// Router from the request's model field and consumed by the proxy handler.
type Route struct {
	Role       string // informational: "plan" | "execute" (for logs/metering)
	Backend    string // registered backend name, e.g. "anthropic" | "opencode"
	Translator string // registered translator name, e.g. "passthrough" | "openai"
	// Model is the upstream model id to send. Empty means "keep the inbound
	// model unchanged" — used by the passthrough/Claude lane so we never force
	// a tier the user didn't ask for.
	Model string
}

// KeepInboundModel reports whether the route wants the original model preserved.
func (r Route) KeepInboundModel() bool { return r.Model == "" }

// Router maps a request's model (a Claude Code tier like opus/sonnet/haiku, or a
// concrete model id) to a Route.
type Router interface {
	Route(model string) (Route, error)
}

// Backend is an upstream provider: a base URL plus an auth scheme. It knows
// nothing about wire formats (that is the Translator's job).
type Backend interface {
	Name() string
	// BaseURL is the prefix before the version path, e.g.
	// "https://api.anthropic.com" or "https://opencode.ai/zen". The Translator
	// supplies the rest of the path (/v1/messages, /v1/chat/completions, ...).
	BaseURL() string
	// Authorize sets auth on the outgoing upstream request. inboundAuth is the
	// Authorization header value the client (Claude Code) sent, so a passthrough
	// backend can forward the user's subscription token verbatim.
	Authorize(out *http.Request, inboundAuth string)
}

// Translator adapts a wire format. It maps the request path, transforms the
// request body from Anthropic Messages format into the upstream format, and
// streams the upstream response back to the client in Anthropic format.
type Translator interface {
	Name() string
	// UpstreamPath maps the inbound Anthropic path to the upstream path for this
	// wire format (e.g. /v1/messages -> /v1/chat/completions for OpenAI).
	UpstreamPath(inboundPath string) string
	// TransformRequest converts an inbound Anthropic request body to the upstream
	// format. The proxy has already rewritten the top-level "model" field when the
	// route asked for it, so translators only worry about shape.
	TransformRequest(body []byte) ([]byte, error)
	// TransformResponse writes the upstream response to w in Anthropic format,
	// flushing incrementally for SSE, and returns the token usage it observed.
	TransformResponse(w http.ResponseWriter, resp *http.Response) (Usage, error)
}

// Registry hands out the concrete backends and translators built from config.
type Registry interface {
	Backend(name string) (Backend, bool)
	Translator(name string) (Translator, bool)
}

// Usage is one metered request's token counts, tagged with where it went.
type Usage struct {
	Timestamp    time.Time
	Backend      string
	Model        string
	Role         string
	InputTokens  int
	OutputTokens int
}

// Filter selects and groups rows for a usage query.
type Filter struct {
	Since time.Time
	By    string // "model" | "backend" | "day"
}

// Aggregate is one grouped row of a usage query.
type Aggregate struct {
	Key          string
	Requests     int
	InputTokens  int
	OutputTokens int
	CostUSD      float64
}

// MeterStore persists and aggregates usage. SQLite today; swappable tomorrow.
type MeterStore interface {
	Record(Usage) error
	Query(Filter) ([]Aggregate, error)
	Close() error
}

// CredentialStore holds provider API keys. OS keyring today; vault tomorrow.
type CredentialStore interface {
	Get(service string) (string, error)
	Set(service, value string) error
	Delete(service string) error
}
