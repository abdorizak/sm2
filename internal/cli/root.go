// Package cli wires up the sm2 command-line interface.
package cli

import (
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

// version is the sm2 build version (overridable via -ldflags).
var version = "0.1.0-dev"

// invokedName returns the command name as the user typed it, so help and usage
// reflect the binary name (sm2, or a custom symlink).
func invokedName() string {
	if len(os.Args) > 0 {
		if b := filepath.Base(os.Args[0]); b != "" && b != "." && b != "/" {
			return b
		}
	}
	return "sm2"
}

func newRootCmd() *cobra.Command {
	var noColor, plain bool
	root := &cobra.Command{
		Use:           invokedName(),
		Short:         "sm2 — a universal application operations agent",
		Long:          "sm2 runs, monitors and restarts applications written in any language.",
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			setupOutput(noColor, plain)
		},
	}

	root.PersistentFlags().BoolVar(&noColor, "no-color", false, "disable colored output")
	root.PersistentFlags().BoolVar(&plain, "plain", false, "plain table output (no box borders)")

	root.AddCommand(
		newAgentCmd(),
		newStartCmd(),
		newStopCmd(),
		newRestartCmd(),
		newDeleteCmd(),
		newResetCmd(),
		newStatusCmd(),
		newDescribeCmd(),
		newLogsCmd(),
		newFlushCmd(),
		newSignalCmd(),
		newConfigCmd(),
		newNotifyCmd(),
		newSaveCmd(),
		newResurrectCmd(),
		newStartupCmd(),
		newUnstartupCmd(),
		newPingCmd(),
		newKillCmd(),
		newVersionCmd(),
	)
	return root
}

// Execute runs the root command.
func Execute() error {
	return newRootCmd().Execute()
}
