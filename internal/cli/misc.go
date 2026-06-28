package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/abdorizak/sm2/internal/ipc"
)

func newPingCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ping",
		Short: "Check that the agent is up (starts it if not)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := request(ipc.Request{Action: ipc.ActionPing})
			if err != nil {
				return err
			}
			if !resp.OK {
				return fmt.Errorf("agent did not respond")
			}
			fmt.Println("pong")
			return nil
		},
	}
}

func newSaveCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "save",
		Aliases: []string{"dump"},
		Short:   "Persist the current process list for later resurrect",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := request(ipc.Request{Action: ipc.ActionSave})
			if err != nil {
				return err
			}
			if !resp.OK {
				return fmt.Errorf("%s", resp.Error)
			}
			fmt.Printf("saved %d app(s)\n", len(resp.Apps))
			return nil
		},
	}
	return cmd
}

func newResurrectCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "resurrect",
		Short: "Restart the apps saved by 'sm2 save'",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := request(ipc.Request{Action: ipc.ActionResurrect})
			if err != nil {
				return err
			}
			if !resp.OK {
				return fmt.Errorf("%s", resp.Error)
			}
			fmt.Println("resurrected saved process list")
			printStatus(resp.Apps)
			return nil
		},
	}
}

func newSignalCmd() *cobra.Command {
	var namespace string
	cmd := &cobra.Command{
		Use:     "signal <signal> <name|all>",
		Aliases: []string{"sendSignal"},
		Short:   "Send a signal to app(s) (e.g. HUP, USR1, TERM)",
		Args:    cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			sig := args[0]
			var name string
			if len(args) == 2 {
				name = args[1]
			}
			resolved, err := targetName(nil, namespace)
			if name != "" {
				resolved = name
			} else if err != nil {
				return err
			}
			resp, err := request(ipc.Request{
				Action:    ipc.ActionSignal,
				Name:      resolved,
				Namespace: namespace,
				Signal:    sig,
			})
			if err != nil {
				return err
			}
			if !resp.OK {
				return fmt.Errorf("%s", resp.Error)
			}
			fmt.Printf("sent %s to %s\n", sig, describeTarget(resolved, namespace))
			return nil
		},
	}
	cmd.Flags().StringVar(&namespace, "namespace", "", "act on all apps in this namespace")
	return cmd
}
