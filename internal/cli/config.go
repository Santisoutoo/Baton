package cli

import (
	"fmt"
	"os"

	"batuta/internal/config"

	"github.com/BurntSushi/toml"
	"github.com/spf13/cobra"
)

func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Inspect batuta configuration",
	}
	cmd.AddCommand(
		&cobra.Command{
			Use:   "path",
			Short: "Print the global and project config file paths",
			RunE: func(cmd *cobra.Command, _ []string) error {
				fmt.Printf("global : %s\n", config.GlobalPath())
				if pp := config.ProjectPath(); pp != "" {
					fmt.Printf("project: %s\n", pp)
				} else {
					fmt.Println("project: (none — no .batuta.toml found)")
				}
				return nil
			},
		},
		&cobra.Command{
			Use:   "show",
			Short: "Print the effective (merged) configuration as TOML",
			RunE: func(cmd *cobra.Command, _ []string) error {
				cfg, err := config.Load()
				if err != nil {
					return err
				}
				return toml.NewEncoder(os.Stdout).Encode(cfg)
			},
		},
		&cobra.Command{
			Use:   "init",
			Short: "Write a default global config file if none exists",
			RunE: func(cmd *cobra.Command, _ []string) error {
				path := config.GlobalPath()
				if _, err := os.Stat(path); err == nil {
					fmt.Printf("already exists: %s\n", path)
					return nil
				}
				if err := config.Save(config.Defaults(), path); err != nil {
					return err
				}
				fmt.Printf("wrote %s\n", path)
				return nil
			},
		},
	)
	return cmd
}
