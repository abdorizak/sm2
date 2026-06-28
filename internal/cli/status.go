package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
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

// statusCells returns the full PM2-style column values for one app.
func statusCells(a ipc.AppStatus) []string {
	pid, cpu, mem, restarts, uptime := statusRow(a)
	ns := a.Namespace
	if ns == "" {
		ns = "default"
	}
	user := a.User
	if user == "" {
		user = "-"
	}
	watching := "disabled"
	if a.Watching {
		watching = "enabled"
	}
	return []string{
		fmt.Sprintf("%d", a.ID), a.Name, ns, "N/A", "fork",
		pid, uptime, restarts, a.State, cpu, mem, user, watching,
	}
}

var statusHeaders = []string{
	"id", "name", "namespace", "version", "mode",
	"pid", "uptime", "↺", "status", "cpu", "mem", "user", "watching",
}

// statusDecorate colors cells by column for the box view.
func statusDecorate(col int, raw string) string {
	switch col {
	case 1: // name
		return cyan(raw)
	case 7: // restarts
		if raw != "0" {
			return yellow(raw)
		}
		return dim(raw)
	case 8: // status
		return colorState(raw)
	case 12: // watching
		if raw == "enabled" {
			return green(raw)
		}
		return dim(raw)
	default:
		return raw
	}
}

func statusBox(apps []ipc.AppStatus) string {
	right := map[int]bool{0: true, 5: true, 6: true, 7: true, 9: true, 10: true}
	cols := make([]column, len(statusHeaders))
	for i, h := range statusHeaders {
		cols[i] = column{strings.ToUpper(h), right[i]}
	}
	rows := make([][]string, 0, len(apps))
	for _, a := range apps {
		rows = append(rows, statusCells(a))
	}
	return renderBox(cols, rows, statusDecorate)
}

func statusPlain(apps []ipc.AppStatus) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, strings.Join(statusHeaders, "\t"))
	for _, a := range apps {
		cells := statusCells(a)
		cells[8] = colorState(cells[8]) // status column
		fmt.Fprintln(w, strings.Join(cells, "\t"))
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
