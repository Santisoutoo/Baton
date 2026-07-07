// Command batuta is a local proxy + CLI that lets Claude Code orchestrate with
// your Claude subscription while OpenCode models handle execution.
package main

import (
	"fmt"
	"os"

	"batuta/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
