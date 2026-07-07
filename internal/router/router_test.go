package router

import (
	"testing"

	"baton/internal/config"
)

func TestRoute(t *testing.T) {
	cfg := config.Defaults()
	rc := cfg.Roles["execute"]
	rc.Model = "deepseek-v4-flash"
	cfg.Roles["execute"] = rc
	r := New(cfg)

	cases := []struct {
		model     string
		wantRole  string
		wantBE    string
		wantTrans string
		wantModel string
	}{
		{"claude-opus-4-8", "plan", "anthropic", "passthrough", ""},
		{"claude-sonnet-5", "plan", "anthropic", "passthrough", ""},
		{"claude-haiku-4-5", "execute", "opencode", "openai", "deepseek-v4-flash"},
		{"totally-unknown", "plan", "anthropic", "passthrough", ""}, // default role
	}
	for _, c := range cases {
		got, err := r.Route(c.model)
		if err != nil {
			t.Fatalf("Route(%q): %v", c.model, err)
		}
		if got.Role != c.wantRole || got.Backend != c.wantBE ||
			got.Translator != c.wantTrans || got.Model != c.wantModel {
			t.Errorf("Route(%q) = %+v, want role=%s be=%s trans=%s model=%s",
				c.model, got, c.wantRole, c.wantBE, c.wantTrans, c.wantModel)
		}
	}
}
