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
		Use:   invokedName(),
		Short: "sm2 — run and supervise any application",
		Long: "sm2 runs your apps — any language, any command — and keeps them alive.\n" +
			"It is a single binary: the CLI auto-starts a background agent that\n" +
			"supervises your processes, restarts them when they die, and reports status.\n\n" +
			"Start an app by passing its command after the name:\n" +
			"  sm2 start web -- npm run start\n" +
			"  sm2 start api --restart always -- ./api",
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			setupOutput(noColor, plain)
		},
	}

	root.PersistentFlags().BoolVar(&noColor, "no-color", false, "disable colored output")
	root.PersistentFlags().BoolVar(&plain, "plain", false, "plain table output (no box borders)")

	// Group commands so `sm2 --help` reads as clear sections.
	const (
		gLifecycle = "lifecycle"
		gInspect   = "inspect"
		gConfig    = "config"
	)
	root.AddGroup(
		&cobra.Group{ID: gLifecycle, Title: "Run & control apps:"},
		&cobra.Group{ID: gInspect, Title: "Inspect:"},
		&cobra.Group{ID: gConfig, Title: "Config, notifications & boot:"},
	)

	add := func(group string, cmds ...*cobra.Command) {
		for _, c := range cmds {
			c.GroupID = group
			root.AddCommand(c)
		}
	}
	add(gLifecycle, newStartCmd(), newStopCmd(), newRestartCmd(), newDeleteCmd(), newResetCmd(), newSignalCmd())
	add(gInspect, newStatusCmd(), newDescribeCmd(), newLogsCmd(), newFlushCmd(), newPingCmd())
	add(gConfig, newConfigCmd(), newNotifyCmd(), newSaveCmd(), newResurrectCmd(), newStartupCmd(), newUnstartupCmd(), newKillCmd())

	// ungrouped (appear under "Additional Commands"): update, version, hidden agent.
	root.AddCommand(newUpdateCmd(), newVersionCmd(), newAgentCmd())
	return root
}

// Execute runs the root command.
func Execute() error {
	return newRootCmd().Execute()
}
