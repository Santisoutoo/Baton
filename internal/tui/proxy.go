package tui

import (
	"net/http"

	"baton/internal/logx"
	"baton/internal/proxy"
	"baton/internal/registry"
	"baton/internal/router"
)

func (m *MainModel) buildFullProxy() http.Handler {
	log := logx.New(m.cfg.LogLevel)

	reg, err := registry.New(m.cfg, m.creds)
	if err != nil {
		log.Error("registry build failed", "err", err)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/healthz" {
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(`{"status":"ok"}`))
				return
			}
			http.Error(w, "proxy init failed: "+err.Error(), http.StatusInternalServerError)
		})
	}

	return proxy.New(m.cfg, router.New(m.cfg), reg, m.meter, log)
}
