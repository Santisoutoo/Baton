package cli

import (
	"fmt"

	"batuta/internal/config"
	"batuta/internal/creds"

	"github.com/spf13/cobra"
)

func newInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "First-time setup: log in to OpenCode and pick the execution model",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			store := creds.New()

			// 1) OpenCode key.
			if v, _ := store.Get("opencode"); v == "" {
				key, err := promptSecret("OpenCode API key (from opencode.ai/auth): ")
				if err != nil {
					return err
				}
				if key != "" {
					if err := store.Set("opencode", key); err != nil {
						return err
					}
					fmt.Println("✓ stored OpenCode key")
				}
			} else {
				fmt.Println("✓ OpenCode key already set")
			}

			// 2) Execution model.
			exec, err := promptLine("Execution model id (e.g. deepseek-v4-flash) [enter to skip]: ")
			if err != nil {
				return err
			}
			if exec != "" {
				rc := cfg.Roles["execute"]
				rc.Model = exec
				cfg.Roles["execute"] = rc
			}
			if err := config.Save(cfg, config.GlobalPath()); err != nil {
				return err
			}
			fmt.Printf("✓ config saved to %s\n", config.GlobalPath())

			// 3) Print how to run.
			addr := fmt.Sprintf("127.0.0.1:%d", cfg.Port)
			fmt.Println("\nAll set. To use it:")
			fmt.Printf("  batuta claude          # launches Claude Code through the proxy\n")
			fmt.Println("or manually:")
			fmt.Printf("  batuta serve\n")
			fmt.Printf("  export ANTHROPIC_BASE_URL=http://%s\n", addr)
			return nil
		},
	}
}
