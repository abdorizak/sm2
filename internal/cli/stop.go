package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/abdorizak/sm2/internal/ipc"
)

func newStopCmd() *cobra.Command {
	var namespace string
	cmd := &cobra.Command{
		Use:   "stop <name|all>",
		Short: "Stop one app, all apps, or a namespace",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name, err := targetName(args, namespace)
			if err != nil {
				return err
			}
			resp, err := request(ipc.Request{Action: ipc.ActionStop, Name: name, Namespace: namespace})
			if err != nil {
				return err
			}
			if !resp.OK {
				return fmt.Errorf("%s", resp.Error)
			}
			fmt.Printf("stopped %s\n", describeTarget(name, namespace))
			return nil
		},
	}
	cmd.Flags().StringVar(&namespace, "namespace", "", "act on all apps in this namespace")
	return cmd
}

// targetName resolves the target from a positional arg and/or namespace flag.
func targetName(args []string, namespace string) (string, error) {
	if len(args) == 1 {
		return args[0], nil
	}
	if namespace != "" {
		return "", nil // namespace-only target
	}
	return "", fmt.Errorf("provide an app name, 'all', or --namespace")
}

func describeTarget(name, namespace string) string {
	switch {
	case name == "all" || (name == "" && namespace != ""):
		if namespace != "" {
			return fmt.Sprintf("namespace %q", namespace)
		}
		return "all apps"
	default:
		return fmt.Sprintf("%q", name)
	}
}
