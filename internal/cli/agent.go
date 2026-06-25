package cli

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/cabdirizaaqyare/runix/internal/agent"
	"github.com/cabdirizaaqyare/runix/internal/logger"
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
