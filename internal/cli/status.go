package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/abdorizak/sm2/internal/ipc"
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

// printStatus renders app statuses: a colored box on a terminal, a plain
// tab-separated table when piped (so grep/awk still work).
func printStatus(apps []ipc.AppStatus) {
	if len(apps) == 0 {
		fmt.Println("no applications are managed")
		return
	}
	if boxOn {
		fmt.Print(statusBox(apps))
		return
	}
	statusPlain(apps)
}

func statusRow(a ipc.AppStatus) (pid, cpu, mem, restarts, uptime string) {
	pid, cpu, mem = "-", "-", "-"
	if a.PID > 0 {
		pid = fmt.Sprintf("%d", a.PID)
		cpu = fmt.Sprintf("%.1f%%", a.CPUPercent)
		mem = humanizeBytes(a.MemoryBytes)
	}
	restarts = fmt.Sprintf("%d", a.Restarts)
	uptime = a.Uptime
	if uptime == "" {
		uptime = "-"
	}
	return
}

func statusBox(apps []ipc.AppStatus) string {
	cols := []column{
		{"NAME", false}, {"STATE", false}, {"PID", true}, {"CPU", true},
		{"MEM", true}, {"↺", true}, {"UPTIME", true}, {"COMMAND", false},
	}
	rows := make([][]string, 0, len(apps))
	for _, a := range apps {
		pid, cpu, mem, restarts, uptime := statusRow(a)
		rows = append(rows, []string{
			a.Name, a.State, pid, cpu, mem, restarts, uptime, truncate(a.Command, 36),
		})
	}
	decorate := func(col int, raw string) string {
		switch col {
		case 1: // STATE
			return colorState(raw)
		case 0: // NAME
			return cyan(raw)
		case 5: // restarts
			if raw != "0" {
				return yellow(raw)
			}
			return dim(raw)
		default:
			return raw
		}
	}
	return renderBox(cols, rows, decorate)
}

func statusPlain(apps []ipc.AppStatus) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NAME\tSTATE\tPID\tCPU\tMEM\tRESTARTS\tUPTIME\tCOMMAND")
	for _, a := range apps {
		pid, cpu, mem, restarts, uptime := statusRow(a)
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			a.Name, colorState(a.State), pid, cpu, mem, restarts, uptime, a.Command)
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
