package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/abdorizak/sm2/internal/ipc"
)

func newRestartCmd() *cobra.Command {
	var (
		namespace string
		updateEnv bool
	)
	cmd := &cobra.Command{
		Use:     "restart <name|all>",
		Aliases: []string{"reload"},
		Short:   "Restart one app, all apps, or a namespace",
		Long: "Restart the targeted app(s).\n\n" +
			"With --update-env, the current shell environment is re-read and applied\n" +
			"before relaunch (the app's explicit config env still takes precedence).\n" +
			"`reload` is an alias; note sm2 restarts the process — it is not a\n" +
			"zero-downtime cluster reload.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name, err := targetName(args, namespace)
			if err != nil {
				return err
			}
			req := ipc.Request{Action: ipc.ActionRestart, Name: name, Namespace: namespace}
			if updateEnv {
				req.UpdateEnv = true
				req.Env = os.Environ()
			}
			resp, err := request(req)
			if err != nil {
				return err
			}
			if !resp.OK {
				return fmt.Errorf("%s", resp.Error)
			}
			suffix := ""
			if updateEnv {
				suffix = " (env refreshed)"
			}
			fmt.Printf("restarted %s%s\n", describeTarget(name, namespace), suffix)
			return nil
		},
	}
	cmd.Flags().StringVar(&namespace, "namespace", "", "act on all apps in this namespace")
	cmd.Flags().BoolVar(&updateEnv, "update-env", false, "re-read the current environment before restarting")
	return cmd
}
