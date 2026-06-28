// Command sm2 is the sm2 CLI and embedded agent daemon.
package main

import (
	"fmt"
	"os"

	"github.com/abdorizak/sm2/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
