// Package backend holds the concrete core.Backend implementations. A backend is
// just an upstream base URL plus an auth scheme; it is deliberately ignorant of
// wire formats (that is the translator's job). Two schemes cover every provider
// baton targets today, and new providers are added by config, not code.
package backend

import "net/http"

// Passthrough forwards the client's inbound Authorization header untouched. This
// is what lets the Claude lane reuse your Claude Code subscription: baton never
// sees or stores an Anthropic key, it just relays the token Claude Code sent.
type Passthrough struct {
	name    string
	baseURL string
}

// NewPassthrough builds a passthrough backend.
func NewPassthrough(name, baseURL string) *Passthrough {
	return &Passthrough{name: name, baseURL: baseURL}
}

func (b *Passthrough) Name() string    { return b.name }
func (b *Passthrough) BaseURL() string { return b.baseURL }

func (b *Passthrough) Authorize(out *http.Request, inboundAuth string) {
	if inboundAuth != "" {
		out.Header.Set("Authorization", inboundAuth)
	}
}

// APIKey swaps the inbound auth for a stored provider key, as either an
// "Authorization: Bearer" header or an "x-api-key" header, depending on style.
// Used for the OpenCode lane.
type APIKey struct {
	name    string
	baseURL string
	style   string // "bearer" | "x-api-key"
	key     string
}

// NewAPIKey builds an api-key backend. An empty key is allowed at construction
// time (the CLI surfaces "not logged in" separately); requests will simply fail
// upstream with 401, which the proxy translates cleanly.
func NewAPIKey(name, baseURL, style, key string) *APIKey {
	return &APIKey{name: name, baseURL: baseURL, style: style, key: key}
}

func (b *APIKey) Name() string    { return b.name }
func (b *APIKey) BaseURL() string { return b.baseURL }

// HasKey reports whether a credential is configured (used by status checks).
func (b *APIKey) HasKey() bool { return b.key != "" }

func (b *APIKey) Authorize(out *http.Request, _ string) {
	// Never forward the client's Anthropic auth to a third-party provider.
	out.Header.Del("Authorization")
	out.Header.Del("X-Api-Key")
	switch b.style {
	case "x-api-key":
		out.Header.Set("x-api-key", b.key)
	default: // "bearer"
		out.Header.Set("Authorization", "Bearer "+b.key)
	}
}
