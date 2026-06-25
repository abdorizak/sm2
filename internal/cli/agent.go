package cli

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/abdorizak/runix/internal/agent"
	"github.com/abdorizak/runix/internal/logger"
)

func newAgentCmd() *cobra.Command {
	return &cobra.Command{
		Use:    "agent",
		Short:  "Run the Runix agent daemon (normally auto-started)",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			log := logger.New(os.Stderr, "agent")
			return agent.Run(log)
		},
	}
}
