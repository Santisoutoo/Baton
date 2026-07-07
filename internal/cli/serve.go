package cli

import (
	"fmt"
	"net/http"

	"baton/internal/config"
	"baton/internal/creds"
	"baton/internal/logx"
	"baton/internal/meter"
	"baton/internal/proxy"
	"baton/internal/registry"
	"baton/internal/router"

	"github.com/spf13/cobra"
)

func newServeCmd() *cobra.Command {
	var port int
	var echo bool
	var level string

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Run the local proxy (bound to 127.0.0.1)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if port == 0 {
				port = cfg.Port
			}
			if level == "" {
				level = cfg.LogLevel
			}
			log := logx.New(level)

			var handler http.Handler
			if echo {
				handler = proxy.NewEcho(log)
				log.Info("starting in ECHO mode (validation step 0): no upstream forwarding")
			} else {
				store := creds.New()
				reg, err := registry.New(cfg, store)
				if err != nil {
					return err
				}
				m, err := meter.Open(config.UsageDBPath())
				if err != nil {
					return err
				}
				defer m.Close()
				handler = proxy.New(cfg, router.New(cfg), reg, m, log)
			}

			// Security: bind to loopback only — never reachable from the network.
			addr := fmt.Sprintf("127.0.0.1:%d", port)
			fmt.Printf("baton proxy listening on http://%s\n", addr)
			fmt.Printf("\n  export ANTHROPIC_BASE_URL=http://%s\n", addr)
			fmt.Printf("  export ENABLE_TOOL_SEARCH=true   # if you use MCP tool search\n\n")

			srv := &http.Server{Addr: addr, Handler: handler}
			return srv.ListenAndServe()
		},
	}
	cmd.Flags().IntVar(&port, "port", 0, "listen port (default from config: 8787)")
	cmd.Flags().BoolVar(&echo, "echo", false, "echo mode: log inbound headers, don't forward (validation)")
	cmd.Flags().StringVar(&level, "log-level", "", "debug|info|warn|error")
	return cmd
}
