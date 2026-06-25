package cli

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/cabdirizaaqyare/runix/internal/paths"
)

func newKillCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "kill",
		Short: "Stop the Runix agent and all managed apps",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			data, err := os.ReadFile(paths.PidFile())
			if err != nil {
				if os.IsNotExist(err) {
					fmt.Println("agent is not running")
					return nil
				}
				return err
			}
			pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
			if err != nil {
				return fmt.Errorf("invalid pid file: %w", err)
			}
			// The agent's signal handler stops every app and cleans up the socket.
			if err := syscall.Kill(pid, syscall.SIGTERM); err != nil {
				if err == syscall.ESRCH {
					_ = os.Remove(paths.PidFile())
					fmt.Println("agent was not running (cleaned up stale pid file)")
					return nil
				}
				return err
			}
			fmt.Printf("stopped agent (pid %d)\n", pid)
			return nil
		},
	}
}
