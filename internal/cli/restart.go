package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/cabdirizaaqyare/runix/internal/ipc"
)

func newRestartCmd() *cobra.Command {
	var namespace string
	cmd := &cobra.Command{
		Use:   "restart <name|all>",
		Short: "Restart one app, all apps, or a namespace",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name, err := targetName(args, namespace)
			if err != nil {
				return err
			}
			resp, err := request(ipc.Request{Action: ipc.ActionRestart, Name: name, Namespace: namespace})
			if err != nil {
				return err
			}
			if !resp.OK {
				return fmt.Errorf("%s", resp.Error)
			}
			fmt.Printf("restarted %s\n", describeTarget(name, namespace))
			return nil
		},
	}
	cmd.Flags().StringVar(&namespace, "namespace", "", "act on all apps in this namespace")
	return cmd
}
