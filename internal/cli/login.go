package cli

import (
	"errors"
	"fmt"

	"batuta/internal/creds"

	"github.com/spf13/cobra"
)

func newLoginCmd() *cobra.Command {
	var key string
	cmd := &cobra.Command{
		Use:   "login <service>",
		Short: "Store a provider API key in the OS keyring (e.g. opencode)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			service := args[0]
			if key == "" {
				var err error
				key, err = promptSecret(fmt.Sprintf("API key for %q: ", service))
				if err != nil {
					return err
				}
			}
			if key == "" {
				return errors.New("no key provided")
			}
			if err := creds.New().Set(service, key); err != nil {
				return err
			}
			fmt.Printf("stored key for %q in the OS keyring\n", service)
			return nil
		},
	}
	cmd.Flags().StringVar(&key, "key", "", "the API key (omit to be prompted without echo)")
	return cmd
}

func newLogoutCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "logout <service>",
		Short: "Remove a stored provider API key",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := creds.New().Delete(args[0]); err != nil {
				return err
			}
			fmt.Printf("removed key for %q (if it existed)\n", args[0])
			return nil
		},
	}
}
