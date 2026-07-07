package cli

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"time"

	"baton/internal/config"
	"baton/internal/creds"
	"baton/internal/logx"
	"baton/internal/meter"
	"baton/internal/proxy"
	"baton/internal/registry"
	"baton/internal/router"

	"github.com/spf13/cobra"
)

func newClaudeCmd() *cobra.Command {
	return &cobra.Command{
		Use:                "claude [-- claude args...]",
		Short:              "Launch Claude Code through the proxy (sets ANTHROPIC_BASE_URL for you)",
		DisableFlagParsing: true, // forward all args to the claude binary
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			log := logx.New(cfg.LogLevel)

			reg, err := registry.New(cfg, creds.New())
			if err != nil {
				return err
			}
			m, err := meter.Open(config.UsageDBPath())
			if err != nil {
				return err
			}
			defer m.Close()

			addr := fmt.Sprintf("127.0.0.1:%d", cfg.Port)
			srv := &http.Server{Addr: addr, Handler: proxy.New(cfg, router.New(cfg), reg, m, log)}
			go func() {
				if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
					log.Error("proxy server stopped", "err", err)
				}
			}()
			defer srv.Shutdown(context.Background())

			if !waitHealthy(addr, 3*time.Second) {
				return fmt.Errorf("proxy did not become healthy on %s", addr)
			}

			bin, err := exec.LookPath("claude")
			if err != nil {
				return fmt.Errorf("claude not found on PATH: %w", err)
			}
			c := exec.Command(bin, args...)
			c.Env = append(os.Environ(),
				"ANTHROPIC_BASE_URL=http://"+addr,
				"ENABLE_TOOL_SEARCH=true",
			)
			c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
			return c.Run()
		},
	}
}

// waitHealthy polls /healthz until the proxy answers or the timeout elapses.
func waitHealthy(addr string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 250 * time.Millisecond}
	for time.Now().Before(deadline) {
		resp, err := client.Get("http://" + addr + "/healthz")
		if err == nil {
			resp.Body.Close()
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return false
}
