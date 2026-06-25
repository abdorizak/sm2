// Package cli wires up the Runix command-line interface.
package cli

import (
	"github.com/spf13/cobra"
)

// version is the Runix build version (overridable via -ldflags).
var version = "0.1.0-dev"

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "runix",
		Short:         "Runix — a universal application operations agent",
		Long:          "Runix runs, monitors and restarts applications written in any language.",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

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
