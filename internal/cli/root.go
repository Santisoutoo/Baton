// Package cli wires baton's command-line surface (cobra). Each command loads the
// layered config and the credential store as needed; the commands stay thin and
// defer real work to the internal packages.
package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// Execute runs the root command. Returns a non-nil error on failure.
func Execute() error {
	return newRoot().Execute()
}

func newRoot() *cobra.Command {
	root := &cobra.Command{
		Use:   "baton",
		Short: "baton — Claude Code orchestrates, OpenCode executes",
		Long: "baton is a local proxy that lets Claude Code plan/orchestrate with your\n" +
			"Claude subscription while cheaper OpenCode models do the execution.\n" +
			"Point ANTHROPIC_BASE_URL at `baton serve` and keep using Claude Code as-is.",
		SilenceUsage: true,
	}
	root.AddCommand(
		newServeCmd(),
		newLoginCmd(),
		newLogoutCmd(),
		newStatusCmd(),
		newUsageCmd(),
		newModelsCmd(),
		newConfigCmd(),
		newInitCmd(),
		newClaudeCmd(),
	)
	return root
}

// promptSecret reads a secret from the terminal without echoing. If stdin is not
// a terminal (piped), it reads a single line instead.
func promptSecret(prompt string) (string, error) {
	fmt.Fprint(os.Stderr, prompt)
	if term.IsTerminal(int(os.Stdin.Fd())) {
		b, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(os.Stderr)
		return strings.TrimSpace(string(b)), err
	}
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	return strings.TrimSpace(line), err
}

// promptLine reads a visible line from the terminal.
func promptLine(prompt string) (string, error) {
	fmt.Fprint(os.Stderr, prompt)
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	return strings.TrimSpace(line), err
}
