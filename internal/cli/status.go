package cli

import (
	"fmt"

	"baton/internal/config"
	"baton/internal/creds"

	"github.com/spf13/cobra"
)

func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show config paths, routing, and which credentials are set",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			store := creds.New()

			fmt.Println("config:")
			fmt.Printf("  global : %s\n", config.GlobalPath())
			if pp := config.ProjectPath(); pp != "" {
				fmt.Printf("  project: %s\n", pp)
			}
			fmt.Printf("  port   : %d\n", cfg.Port)
			fmt.Printf("  on_exec_error: %s\n", cfg.OnExecError)

			fmt.Println("\nroles:")
			for name, rc := range cfg.Roles {
				model := rc.Model
				if model == "" {
					model = "(keep inbound model)"
				}
				fmt.Printf("  %-8s -> backend %-10s model %s\n", name, rc.Backend, model)
			}

			fmt.Println("\ntiers:")
			for tier, role := range cfg.Tiers {
				fmt.Printf("  %-7s -> %s\n", tier, role)
			}

			fmt.Println("\nbackends:")
			for name, bc := range cfg.Backends {
				credState := "n/a (passthrough)"
				if bc.Credential != "" {
					if v, _ := store.Get(bc.Credential); v != "" {
						credState = "key set"
					} else {
						credState = "MISSING — run: baton login " + bc.Credential
					}
				}
				fmt.Printf("  %-10s type=%-9s url=%-28s auth=%-11s %s\n",
					name, bc.Type, bc.BaseURL, bc.Auth, credState)
			}
			return nil
		},
	}
}
