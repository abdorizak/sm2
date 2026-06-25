package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/cabdirizaaqyare/runix/internal/ipc"
)

func newStatusCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:     "status",
		Aliases: []string{"ls", "ps", "list"},
		Short:   "Show the status of all managed applications",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := request(ipc.Request{Action: ipc.ActionStatus})
			if err != nil {
				return err
			}
			if !resp.OK {
				return fmt.Errorf("%s", resp.Error)
			}
			if asJSON {
				out, err := json.MarshalIndent(resp.Apps, "", "  ")
				if err != nil {
					return err
				}
				fmt.Println(string(out))
				return nil
			}
			printStatus(resp.Apps)
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "output as JSON")
	return cmd
}

// printStatus renders a slice of app statuses as a table (or a notice if empty).
func printStatus(apps []ipc.AppStatus) {
	if len(apps) == 0 {
		fmt.Println("no applications are managed")
		return
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NAME\tSTATE\tPID\tCPU\tMEM\tRESTARTS\tUPTIME\tCOMMAND")
	for _, a := range apps {
		pid, cpu, mem := "-", "-", "-"
		if a.PID > 0 {
			pid = fmt.Sprintf("%d", a.PID)
			cpu = fmt.Sprintf("%.1f%%", a.CPUPercent)
			mem = humanizeBytes(a.MemoryBytes)
		}
		uptime := a.Uptime
		if uptime == "" {
			uptime = "-"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%d\t%s\t%s\n",
			a.Name, a.State, pid, cpu, mem, a.Restarts, uptime, a.Command)
	}
	w.Flush()
}

// humanizeBytes renders a byte count as a compact human-readable string.
func humanizeBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%dB", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f%cB", float64(b)/float64(div), "KMGTPE"[exp])
}
