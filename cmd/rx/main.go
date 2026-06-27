// Command rx is a short alias for the Runix CLI and embedded agent daemon.
package main

import (
	"fmt"
	"os"

	"github.com/abdorizak/runix/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
