// Package registry builds the concrete backends and translators from config and
// hands them to the proxy by name. This is the seam that keeps the core free of
// hardcoded provider names: add a [backends.x] table and it appears here, no
// core edits. New wire formats register a new translator.
package registry

import (
	"baton/internal/backend"
	"baton/internal/config"
	"baton/internal/core"
	"baton/internal/translate"
)

// Registry implements core.Registry.
type Registry struct {
	backends    map[string]core.Backend
	translators map[string]core.Translator
}

// New constructs backends from config (pulling API keys from creds for key-auth
// backends) and registers the built-in translators.
func New(cfg *config.Config, creds core.CredentialStore) (*Registry, error) {
	backends := make(map[string]core.Backend, len(cfg.Backends))
	for name, bc := range cfg.Backends {
		switch bc.Auth {
		case "bearer", "x-api-key":
			key := ""
			if bc.Credential != "" && creds != nil {
				key, _ = creds.Get(bc.Credential)
			}
			backends[name] = backend.NewAPIKey(name, bc.BaseURL, bc.Auth, key)
		default: // "passthrough" or unset
			backends[name] = backend.NewPassthrough(name, bc.BaseURL)
		}
	}

	translators := map[string]core.Translator{
		"passthrough": translate.Passthrough{},
		"openai":      translate.OpenAI{},
	}

	return &Registry{backends: backends, translators: translators}, nil
}

func (r *Registry) Backend(name string) (core.Backend, bool) {
	b, ok := r.backends[name]
	return b, ok
}

func (r *Registry) Translator(name string) (core.Translator, bool) {
	t, ok := r.translators[name]
	return t, ok
}
