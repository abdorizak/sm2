package cli

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/abdorizak/sm2/internal/ipc"
	"github.com/abdorizak/sm2/internal/process"
)

func newDescribeCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "describe <name>",
		Aliases: []string{"info", "desc", "show"},
		Short:   "Show all parameters of an app",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := request(ipc.Request{Action: ipc.ActionDescribe, Name: args[0]})
			if err != nil {
				return err
			}
			if !resp.OK {
				return fmt.Errorf("%s", resp.Error)
			}
			if resp.Detail == nil {
				return fmt.Errorf("no detail returned")
			}
			printDetail(resp.Detail)
			return nil
		},
	}
}

func printDetail(d *ipc.AppDetail) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	row := func(k, v string) {
		if v != "" {
			fmt.Fprintf(w, "  %s\t%s\n", k, v)
		}
	}

	s, spec := d.Status, d.Spec
	fmt.Printf("%s %s\n", green("●"), cyan(s.Name))
	row("namespace", spec.Namespace)
	row("state", colorState(s.State))
	row("command", spec.Command)
	row("directory", spec.Dir)
	if s.PID > 0 {
		row("pid", fmt.Sprintf("%d", s.PID))
		row("cpu", fmt.Sprintf("%.1f%%", s.CPUPercent))
		row("memory", humanizeBytes(s.MemoryBytes))
		row("uptime", s.Uptime)
	}
	row("restarts", fmt.Sprintf("%d", s.Restarts))

	policy := spec.Restart
	if policy == "" {
		policy = "on-failure"
	}
	row("restart policy", policy)
	if spec.MaxRetries > 0 {
		mr := fmt.Sprintf("%d", spec.MaxRetries)
		if spec.MinUptimeMs > 0 {
			mr += fmt.Sprintf(" (quick crashes under %dms in a row)", spec.MinUptimeMs)
		}
		row("max retries", mr)
	}
	if spec.InstanceVar != "" {
		row("instance var", fmt.Sprintf("%s=%d", spec.InstanceVar, spec.Instance))
	}
	if len(spec.FilterEnv) > 0 {
		row("filter env", strings.Join(spec.FilterEnv, " "))
	}
	if spec.KillTimeoutMs > 0 {
		row("kill timeout", fmt.Sprintf("%dms", spec.KillTimeoutMs))
	}
	if spec.RestartDelayMs > 0 {
		mode := "fixed"
		if spec.ExpBackoff {
			mode = "exponential"
		}
		row("restart delay", fmt.Sprintf("%dms (%s)", spec.RestartDelayMs, mode))
	}
	if spec.MaxMemoryBytes > 0 {
		row("max memory", humanizeBytes(spec.MaxMemoryBytes))
	}
	if spec.Watch {
		row("watch", "enabled")
	}
	if spec.CronRestart != "" {
		row("cron restart", spec.CronRestart)
	}
	if spec.HealthCheck != "" {
		check := spec.HealthCheck
		if spec.HealthIntervalMs > 0 {
			check += fmt.Sprintf(" (every %dms)", spec.HealthIntervalMs)
		}
		row("health check", check)
		switch s.Health {
		case "healthy":
			row("health", green("healthy"))
		case "unhealthy":
			row("health", red("unhealthy")+" — "+s.HealthError)
		case "":
			if s.PID > 0 {
				row("health", dim("pending (first check one interval after start)"))
			}
		}
	}
	if len(spec.EnvFiles) > 0 {
		keys, err := process.EnvFileKeys(spec.EnvFiles)
		files := strings.Join(spec.EnvFiles, ", ")
		if err != nil {
			row("env files", files+" "+red("("+err.Error()+")"))
		} else {
			row("env files", files)
			if len(keys) > 0 {
				row("env file keys", strings.Join(keys, " "))
			}
		}
	}
	row("stdout log", d.StdoutLog)
	row("stderr log", d.StderrLog)

	if len(spec.Env) > 0 {
		keys := make([]string, 0, len(spec.Env))
		for k := range spec.Env {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for i, k := range keys {
			label := ""
			if i == 0 {
				label = "env"
			}
			fmt.Fprintf(w, "  %s\t%s=%s\n", label, k, spec.Env[k])
		}
	}
	w.Flush()
}
