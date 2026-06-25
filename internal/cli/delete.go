package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/cabdirizaaqyare/runix/internal/ipc"
)

func newDeleteCmd() *cobra.Command {
	var namespace string
	cmd := &cobra.Command{
		Use:     "delete <name|all>",
		Aliases: []string{"del", "rm"},
		Short:   "Stop and remove app(s) from the process list",
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name, err := targetName(args, namespace)
			if err != nil {
				return err
			}
			resp, err := request(ipc.Request{Action: ipc.ActionDelete, Name: name, Namespace: namespace})
			if err != nil {
				return err
			}
			if !resp.OK {
				return fmt.Errorf("%s", resp.Error)
			}
			fmt.Printf("deleted %s\n", describeTarget(name, namespace))
			return nil
		},
	}
	cmd.Flags().StringVar(&namespace, "namespace", "", "act on all apps in this namespace")
	return cmd
}

func newResetCmd() *cobra.Command {
	var namespace string
	cmd := &cobra.Command{
		Use:   "reset <name|all>",
		Short: "Reset the restart counter for app(s)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name, err := targetName(args, namespace)
			if err != nil {
				return err
			}
			resp, err := request(ipc.Request{Action: ipc.ActionReset, Name: name, Namespace: namespace})
			if err != nil {
				return err
			}
			if !resp.OK {
				return fmt.Errorf("%s", resp.Error)
			}
			fmt.Printf("reset counters for %s\n", describeTarget(name, namespace))
			return nil
		},
	}
	cmd.Flags().StringVar(&namespace, "namespace", "", "act on all apps in this namespace")
	return cmd
}
