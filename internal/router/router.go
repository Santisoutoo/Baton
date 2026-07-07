// Package router turns config into routing decisions. It is the only place that
// knows the tier→role→backend mapping; the proxy just asks Route(model).
package router

import (
	"fmt"
	"strings"

	"baton/internal/config"
	"baton/internal/core"
)

// Router implements core.Router from a Config.
type Router struct {
	cfg *config.Config
}

// New builds a Router.
func New(cfg *config.Config) *Router { return &Router{cfg: cfg} }

// Route classifies the request's model into a tier, maps tier→role→backend, and
// picks the translator from the backend's wire type.
func (r *Router) Route(model string) (core.Route, error) {
	role := r.roleForModel(model)
	rc, ok := r.cfg.Roles[role]
	if !ok {
		return core.Route{}, fmt.Errorf("role %q not defined in config", role)
	}
	bc, ok := r.cfg.Backends[rc.Backend]
	if !ok {
		return core.Route{}, fmt.Errorf("backend %q (role %q) not defined in config", rc.Backend, role)
	}
	return core.Route{
		Role:       role,
		Backend:    rc.Backend,
		Translator: translatorForType(bc.Type),
		Model:      rc.Model, // "" => keep inbound model
	}, nil
}

// roleForModel maps a model id to a role via its tier, falling back to the
// configured default role (which points at the Claude lane, the safe choice).
func (r *Router) roleForModel(model string) string {
	if role, ok := r.cfg.Tiers[tierOf(model)]; ok {
		return role
	}
	return r.cfg.DefaultRole
}

// tierOf classifies a Claude Code model id into opus/sonnet/haiku. Claude Code
// sends full ids like "claude-opus-4-8" or "claude-haiku-4-5"; a substring match
// is robust across versions. Bare "opus"/"sonnet"/"haiku" also work.
func tierOf(model string) string {
	m := strings.ToLower(model)
	switch {
	case strings.Contains(m, "opus"):
		return "opus"
	case strings.Contains(m, "sonnet"):
		return "sonnet"
	case strings.Contains(m, "haiku"):
		return "haiku"
	default:
		return ""
	}
}

// translatorForType picks the wire adapter for a backend type.
func translatorForType(backendType string) string {
	if strings.EqualFold(backendType, "openai") {
		return "openai"
	}
	return "passthrough"
}
