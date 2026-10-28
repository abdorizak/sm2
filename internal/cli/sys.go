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
		printSysGrouped(r, all)
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

// sysCells returns the display columns for one service:
// name, what, ports, memory, uptime/state, source.
func sysCells(s sysinfo.Service) []string {
	mem := "-"
	if s.MemoryBytes > 0 {
		mem = humanizeBytes(s.MemoryBytes)
	}
	src := string(s.Source)
	switch {
	case s.Kind == "listener":
		src = "-"
	case s.Source == sysinfo.SourceProcess && s.PID > 0:
		src = fmt.Sprintf("pid %d", s.PID)
	}
	return []string{s.Name, truncate(s.What(), 44), portList(s.Ports), mem, sysUptime(s), src}
}

func sysUptime(s sysinfo.Service) string {
	switch {
	case s.Failed && s.SinceSec > 0:
		return fmt.Sprintf("%s %s ago", s.State, compactDuration(s.SinceSec))
	case s.Failed:
		return s.State
	case s.Kind == "listener":
		return "listening"
	case s.State == "running" && s.UptimeSec > 0:
		return "up " + compactDuration(s.UptimeSec)
	case s.State == "running":
		return "up"
	default:
		return s.State
	}
}

func portList(ports []int) string {
	if len(ports) == 0 {
		return "-"
	}
	const max = 4
	parts := make([]string, 0, max+1)
	for i, p := range ports {
		if i == max {
			parts = append(parts, fmt.Sprintf("+%d", len(ports)-max))
			break
		}
		parts = append(parts, ":"+strconv.Itoa(p))
	}
	return strings.Join(parts, " ")
}

// printSysGrouped renders the terminal view: a header line, then one aligned
// block per group (DATABASES, WEB SERVERS, …, FAILED).
func printSysGrouped(r sysinfo.Report, all bool) {
	head := []string{bold(r.Host)}
	if r.OS != "" {
		head = append(head, r.OS)
	}
	if r.HostUptimeSec > 0 {
		head = append(head, "up "+compactDuration(r.HostUptimeSec))
	}
	fmt.Println(strings.Join(head, dim(" · ")))

	groups := r.Groups()
	if len(groups) == 0 {
		fmt.Println("\nnothing notable is running (try sm2 sys --all)")
	}

	// Measure columns across every group so the whole view lines up.
	const ncol = 6
	width := make([]int, ncol)
	for _, s := range r.Services {
		for i, c := range sysCells(s) {
			if l := runeLen(c); l > width[i] {
				width[i] = l
			}
		}
	}

	failed := 0
	for _, g := range groups {
		label := g.Label
		switch g.ID {
		case "failed":
			label = red(label)
		case "unmanaged":
			if r.Platform == "linux" {
				label = yellow(label + " ⚠") // nothing supervises these
			} else {
				label = bold(label)
			}
		default:
			label = bold(label)
		}
		fmt.Printf("\n%s\n", label)
		for _, s := range g.Services {
			if s.Failed {
				failed++
			}
			cells := sysCells(s)
			var b strings.Builder
			b.WriteString("  ")
			for i, c := range cells {
				pad := strings.Repeat(" ", width[i]-runeLen(c))
				switch i {
				case 0:
					c = cyan(c)
				case 4:
					c = sysStateColor(s, c)
				case 5:
					c = dim(c)
				}
				b.WriteString(c)
				if i < ncol-1 {
					b.WriteString(pad + "   ")
				}
			}
			fmt.Println(b.String())
		}
	}

	summary := fmt.Sprintf("%d shown", len(r.Services))
	if failed > 0 {
		summary += " · " + red(fmt.Sprintf("%d failed", failed))
	}
	if r.Hidden > 0 && !all {
		summary += fmt.Sprintf(" · %d routine services hidden (sm2 sys --all)", r.Hidden)
	}
	fmt.Printf("\n%s\n", dim(summary))
	for _, n := range r.Notes {
		fmt.Println(dim("note: " + n))
	}
}

func sysStateColor(s sysinfo.Service, c string) string {
	switch {
	case s.Failed:
		return red(c)
	case strings.HasPrefix(c, "up"):
		return green(c)
	default:
		return yellow(c)
	}
}

// printSysPlain is the pipe-friendly form: one tab-separated row per service.
func printSysPlain(r sysinfo.Report) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "group\tname\twhat\tports\tmemory\tstate\tsource")
	for _, g := range r.Groups() {
		for _, s := range g.Services {
			c := sysCells(s)
			fmt.Fprintf(w, "%s\t%s\n", strings.ToLower(g.Label), strings.Join(c, "\t"))
		}
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
	case s.Source == sysinfo.SourceProcess && s.Kind != "listener":
		src = "none — not supervised by systemd, docker or sm2"
	}
	row("managed by", src)
	row("state", sysUptime(s))
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
