package cli

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/abdorizak/sm2/internal/agent"
	"github.com/abdorizak/sm2/internal/logger"
)

func newAgentCmd() *cobra.Command {
	return &cobra.Command{
		Use:    "agent",
		Short:  "Run the sm2 agent daemon (normally auto-started)",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			log := logger.New(os.Stderr, "agent")
			return agent.Run(log)
		},
	}
}
