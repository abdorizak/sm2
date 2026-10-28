package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/abdorizak/sm2/internal/ipc"
	"github.com/abdorizak/sm2/internal/sysinfo"
)

func newSysCmd() *cobra.Command {
	var all, asJSON bool
	cmd := &cobra.Command{
		Use:     "sys [name]",
		Aliases: []string{"system"},
		Short:   "Show everything running on this server (services, databases, containers, apps)",
		Long: "Discover and describe what is running on this machine — no names needed.\n\n" +
			"sm2 sys finds systemd services (PostgreSQL, MySQL, Redis, nginx, …), Docker\n" +
			"containers, your sm2 apps, and any other process listening on a port, then\n" +
			"groups them by kind with version, ports, memory and uptime. Failed services\n" +
			"are listed last. Routine system units are hidden unless you pass --all.\n\n" +
			"Pass a name to describe one service in detail, including its recent logs.\n" +
			"Run with sudo to see ports and logs owned by other users.",
		Args: cobra.MaximumNArgs(1),
		Example: "  sm2 sys                 # everything notable on this server\n" +
			"  sm2 sys --all           # include routine system services\n" +
			"  sm2 sys postgres        # describe one service (+ recent logs)\n" +
			"  sudo sm2 sys            # full detail, incl. other users' ports",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 {
				return runSysDescribe(args[0], asJSON)
			}
			return runSys(all, asJSON)
		},
	}
	cmd.Flags().BoolVarP(&all, "all", "a", false, "include routine system services, exited units and stopped apps")
	cmd.Flags().BoolVar(&asJSON, "json", false, "output as JSON")
	return cmd
}

// agentApps returns the sm2-managed apps when an agent is already running. It
// never starts one: sys is a read-only look at the machine. Under sudo it also
// tries the invoking user's agent, so `sudo sm2 sys` still labels their apps.
func agentApps() []ipc.AppStatus {
	req := ipc.Request{Action: ipc.ActionStatus}
	if resp, err := send(req); err == nil && resp.OK {
		return resp.Apps
	}
	if name := os.Getenv("SUDO_USER"); name != "" && os.Getenv("SM2_HOME") == "" {
		if u, err := user.Lookup(name); err == nil {
			if resp, err := sendTo(filepath.Join(u.HomeDir, ".sm2", "sm2.sock"), req); err == nil && resp.OK {
				return resp.Apps
			}
		}
	}
	return nil
}

func runSys(all, asJSON bool) error {
	r := sysinfo.Discover(sysinfo.Options{All: all, Apps: agentApps()})
	if asJSON {
		return printJSON(r)
	}
	if boxOn {
		printSysBox(r, all)
	} else {
		printSysPlain(r)
	}
	return nil
}

func printJSON(v any) error {
	out, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(out))
	return nil
}

var sysHeaders = []string{
	"name", "type", "service", "pid", "ports", "uptime", "status", "cpu", "mem", "user", "managed by",
}

// sysCells returns the table columns for one service.
func sysCells(s sysinfo.Service, platform string) []string {
	pid, cpu, mem, user := "-", "-", "-", "-"
	if s.PID > 0 {
		pid = strconv.Itoa(s.PID)
	}
	if s.CPUPercent != nil {
		cpu = fmt.Sprintf("%.1f%%", *s.CPUPercent)
	}
	if s.MemoryBytes > 0 {
		mem = humanizeBytes(s.MemoryBytes)
	}
	if s.User != "" {
		user = s.User
	}
	return []string{
		truncate(s.Name, 28), sysType(s), truncate(s.What(), 30), pid, portList(s.Ports),
		sysUptime(s), sysStatus(s), cpu, mem, user, sysManager(s, platform),
	}
}

// sysType is what kind of thing a service is: a database, a cache, a web
// server, … or, for unrecognized software, how it runs.
func sysType(s sysinfo.Service) string {
	switch {
	case s.Kind == "listener":
		return "port"
	case s.Kind != "":
		return s.Kind
	case s.Source == sysinfo.SourceDocker:
		return "container"
	case s.Source == sysinfo.SourceSM2:
		return "app"
	case s.Source == sysinfo.SourceProcess:
		return "process"
	default:
		return "service"
	}
}

func sysManager(s sysinfo.Service, platform string) string {
	switch {
	case s.Kind == "listener":
		return "?"
	case s.Source == sysinfo.SourceProcess && platform == "linux":
		return "none" // nothing will restart it if it dies
	case s.Source == sysinfo.SourceProcess:
		return "-"
	default:
		return string(s.Source)
	}
}

// sysUptime is time up for a live service, or time since a failure/exit.
func sysUptime(s sysinfo.Service) string {
	switch {
	case s.UptimeSec > 0 && !s.Failed && s.State == "running":
		return compactDuration(s.UptimeSec)
	case s.SinceSec > 0 && s.State != "running":
		return compactDuration(s.SinceSec) + " ago"
	default:
		return "-"
	}
}

func sysStatus(s sysinfo.Service) string {
	switch {
	case s.Kind == "listener":
		return "LISTEN"
	case s.Failed && strings.HasPrefix(s.State, "failed"):
		return "FAILED"
	default:
		return strings.ToUpper(s.State)
	}
}

// colorSysStatus tints a status cell by meaning, like colorState for apps.
func colorSysStatus(st string) string {
	switch st {
	case "RUNNING", "LISTEN":
		return green(st)
	case "FAILED", "UNHEALTHY", "DEAD":
		return red(st)
	case "STARTING", "STOPPING", "RELOADING", "RESTARTING":
		return yellow(st)
	default:
		return dim(st)
	}
}

func portList(ports []int) string {
	if len(ports) == 0 {
		return "-"
	}
	const max = 3
	parts := make([]string, 0, max+1)
	for i, p := range ports {
		if i == max {
			parts = append(parts, fmt.Sprintf("+%d", len(ports)-max))
			break
		}
		parts = append(parts, strconv.Itoa(p))
	}
	return strings.Join(parts, ",")
}

// sysRows flattens the report's groups into table rows (already ordered by
// kind, so TYPE reads as a grouping), recording where the failed block starts
// so the box can set it apart with a separator.
func sysRows(r sysinfo.Report) (rows [][]string, failed []bool, breaks map[int]bool) {
	breaks = map[int]bool{}
	for _, g := range r.Groups() {
		if g.ID == "failed" {
			breaks[len(rows)] = true
		}
		for _, s := range g.Services {
			rows = append(rows, sysCells(s, r.Platform))
			failed = append(failed, s.Failed)
		}
	}
	return rows, failed, breaks
}

// printSysBox renders the terminal view: a host summary, then one boxed table
// of services (grouped: databases, caches, web …, failed last).
func printSysBox(r sysinfo.Report, all bool) {
	head := []string{bold(r.Host)}
	if r.OS != "" {
		head = append(head, r.OS)
	}
	if r.HostUptimeSec > 0 {
		head = append(head, "up "+compactDuration(r.HostUptimeSec))
	}
	fmt.Println(strings.Join(head, dim(" · ")))
	if u := hostUsageLine(r.Usage); u != "" {
		fmt.Println(u)
	}

	rows, failedRows, breaks := sysRows(r)
	if len(rows) == 0 {
		fmt.Println("\nnothing notable is running (try sm2 sys --all)")
	} else {
		right := map[int]bool{3: true, 5: true, 7: true, 8: true}
		cols := make([]column, len(sysHeaders))
		for i, h := range sysHeaders {
			cols[i] = column{strings.ToUpper(h), right[i]}
		}
		fmt.Print(renderBoxSections(cols, rows, breaks, func(col int, raw string) string {
			switch col {
			case 0:
				return cyan(raw)
			case 6:
				return colorSysStatus(raw)
			case 7:
				return colorCPU(raw)
			case 10:
				if raw == "none" {
					return yellow(raw)
				}
				return dim(raw)
			}
			return raw
		}))
	}

	failed := 0
	for _, f := range failedRows {
		if f {
			failed++
		}
	}
	summary := fmt.Sprintf("%d shown", len(rows))
	if failed > 0 {
		summary += " · " + red(fmt.Sprintf("%d failed", failed))
	}
	if r.Hidden > 0 && !all {
		summary += fmt.Sprintf(" · %d routine services hidden (sm2 sys --all)", r.Hidden)
	}
	fmt.Println(dim(summary))
	for _, n := range r.Notes {
		fmt.Println(dim("note: " + n))
	}
}

// hostUsageLine summarizes machine-wide load, memory and disk use.
func hostUsageLine(u sysinfo.HostUsage) string {
	var parts []string
	if u.Load1 > 0 || u.CPUs > 0 {
		load := fmt.Sprintf("load %.2f", u.Load1)
		if u.CPUs > 0 {
			load += fmt.Sprintf(" (%d cpus)", u.CPUs)
			if u.Load1 > float64(u.CPUs) {
				load = yellow(load)
			}
		}
		parts = append(parts, load)
	}
	if u.MemTotal > 0 && u.MemUsed > 0 {
		pct := float64(u.MemUsed) / float64(u.MemTotal) * 100
		parts = append(parts, usagePct(fmt.Sprintf("mem %s / %s (%.0f%%)",
			humanizeBytes(u.MemUsed), humanizeBytes(u.MemTotal), pct), pct))
	}
	if u.DiskUsedPct > 0 {
		parts = append(parts, usagePct(fmt.Sprintf("disk %.0f%% used", u.DiskUsedPct), u.DiskUsedPct))
	}
	return strings.Join(parts, dim(" · "))
}

func usagePct(s string, pct float64) string {
	switch {
	case pct >= 90:
		return red(s)
	case pct >= 75:
		return yellow(s)
	default:
		return s
	}
}

// colorCPU highlights services using a lot of CPU (in % of one core).
func colorCPU(raw string) string {
	v, err := strconv.ParseFloat(strings.TrimSuffix(raw, "%"), 64)
	switch {
	case err != nil:
		return dim(raw)
	case v >= 90:
		return red(raw)
	case v >= 50:
		return yellow(raw)
	default:
		return raw
	}
}

// printSysPlain is the pipe-friendly form: one tab-separated row per service.
func printSysPlain(r sysinfo.Report) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, strings.Join(sysHeaders, "\t"))
	rows, _, _ := sysRows(r)
	for _, c := range rows {
		fmt.Fprintln(w, strings.Join(c, "\t"))
	}
	w.Flush()
	for _, n := range r.Notes {
		fmt.Fprintln(os.Stderr, "note: "+n)
	}
}

// runSysDescribe prints everything known about one service.
func runSysDescribe(name string, asJSON bool) error {
	r := sysinfo.Discover(sysinfo.Options{All: true, Apps: agentApps()})
	matches := sysinfo.Find(r.Services, name)
	switch len(matches) {
	case 0:
		return fmt.Errorf("nothing running matches %q (see `sm2 sys --all`)", name)
	case 1:
	default:
		names := make([]string, len(matches))
		for i, m := range matches {
			names[i] = m.Name
		}
		return fmt.Errorf("%q matches several services: %s", name, strings.Join(names, ", "))
	}
	s := matches[0]
	sysinfo.Enrich(&s)
	if asJSON {
		return printJSON(s)
	}

	dot := green("●")
	if s.Failed {
		dot = red("●")
	} else if s.State != "running" {
		dot = dim("●")
	}
	title := cyan(s.Name)
	if w := s.What(); w != "" && w != s.Name {
		title += "  " + w
	}
	if s.Kind != "" && s.Kind != "listener" {
		title += dim(" · " + s.Kind)
	}
	fmt.Printf("%s %s\n", dot, title)

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	row := func(k, v string) {
		if v != "" {
			fmt.Fprintf(w, "  %s\t%s\n", k, v)
		}
	}
	src := string(s.Source)
	switch {
	case s.Unit != "":
		src += " (" + s.Unit + ")"
	case s.ContainerID != "":
		src += " (container " + s.ContainerID + ")"
	case s.Source == sysinfo.SourceProcess && s.Kind != "listener" && r.Platform == "linux":
		src = "none — not supervised by systemd, docker or sm2"
	case s.Source == sysinfo.SourceProcess && s.Kind != "listener":
		src = "not systemd, docker or sm2"
	}
	row("managed by", src)
	state := strings.ToLower(sysStatus(s))
	if up := sysUptime(s); up != "-" {
		if strings.HasSuffix(up, "ago") {
			state += " " + up
		} else {
			state += ", up " + up
		}
	}
	if s.State != "" && strings.HasPrefix(s.State, "failed (") {
		state = s.State + strings.TrimPrefix(state, "failed")
	}
	row("state", state)
	if s.Description != "" && s.Description != s.Image {
		row("description", s.Description)
	}
	if s.Package != "" {
		pkg := s.Package
		if s.Version != "" {
			pkg += " " + s.Version
		}
		row("package", pkg)
	}
	row("image", s.Image)
	if s.PID > 0 {
		row("pid", strconv.Itoa(s.PID))
	}
	row("user", s.User)
	if s.CPUPercent != nil {
		row("cpu", fmt.Sprintf("%.1f%%", *s.CPUPercent))
	}
	if s.MemoryBytes > 0 {
		row("memory", humanizeBytes(s.MemoryBytes))
	}
	row("listening", strings.Join(s.Listen, ", "))
	row("command", s.Command)
	if s.Exe != "" && s.Exe != firstField(s.Command) {
		row("binary", s.Exe)
	}
	row("unit file", s.UnitFile)
	row("started by", s.Parent)
	w.Flush()

	lines, more := sysinfo.Logs(s, 10)
	if len(lines) > 0 {
		fmt.Printf("\n%s\n", bold("recent logs"))
		for _, l := range lines {
			fmt.Println("  " + dim(truncate(l, 160)))
		}
	} else if s.Source == sysinfo.SourceSystemd && os.Geteuid() != 0 {
		fmt.Printf("\n%s\n", dim("no logs visible — try: sudo "+more))
		more = ""
	}
	if more != "" {
		fmt.Printf("\n%s\n", dim("more: "+more))
	}
	return nil
}

// compactDuration renders seconds as 45s, 12m, 5h or 3D (like sm2 status).
func compactDuration(s int64) string {
	switch {
	case s < 60:
		return fmt.Sprintf("%ds", s)
	case s < 3600:
		return fmt.Sprintf("%dm", s/60)
	case s < 86400:
		return fmt.Sprintf("%dh", s/3600)
	default:
		return fmt.Sprintf("%dD", s/86400)
	}
}

func truncate(s string, n int) string {
	if runeLen(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n-1]) + "…"
}

func firstField(s string) string {
	if f := strings.Fields(s); len(f) > 0 {
		return f[0]
	}
	return ""
}
