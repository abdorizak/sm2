package cli

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/abdorizak/sm2/internal/ipc"
)

func newStartCmd() *cobra.Command {
	var (
		command       string
		dir           string
		cwd           string
		envFlags      []string
		restart       string
		maxRetries    int
		maxRestarts   int
		instances     int
		namespace     string
		noAutorestart bool
		noAutostart   bool
		killTimeout   time.Duration
		restartDelay  time.Duration
		expBackoff    time.Duration
		maxMemory     string
		watch         bool
		ignoreWatch   []string
		cronRestart   string
	)

	cmd := &cobra.Command{
		Use:   "start <name>",
		Short: "Start and supervise an application",
		Args:  cobra.ExactArgs(1),
		Example: "  sm2 start api --cmd \"./api\"\n" +
			"  sm2 start web --cmd \"npm run start\" --restart always -i 2\n" +
			"  sm2 start job --cmd \"./job\" --cron-restart \"0 3 * * *\"",
		RunE: func(cmd *cobra.Command, args []string) error {
			if command == "" {
				return fmt.Errorf("--cmd is required")
			}
			env, err := parseEnv(envFlags)
			if err != nil {
				return err
			}

			workdir := dir
			if cwd != "" {
				workdir = cwd
			}

			retries := maxRetries
			if cmd.Flags().Changed("max-restarts") {
				retries = maxRestarts
			}

			if noAutorestart {
				restart = "never"
			}

			var maxMemBytes int64
			if maxMemory != "" {
				maxMemBytes, err = parseBytes(maxMemory)
				if err != nil {
					return err
				}
			}

			restartDelayMs := int(restartDelay / time.Millisecond)
			exp := false
			if cmd.Flags().Changed("exp-backoff-restart-delay") {
				exp = true
				restartDelayMs = int(expBackoff / time.Millisecond)
			}

			base := ipc.AppSpec{
				Command:        command,
				Dir:            workdir,
				Namespace:      namespace,
				Env:            env,
				Restart:        restart,
				MaxRetries:     retries,
				NoAutostart:    noAutostart,
				KillTimeoutMs:  int(killTimeout / time.Millisecond),
				RestartDelayMs: restartDelayMs,
				ExpBackoff:     exp,
				MaxMemoryBytes: maxMemBytes,
				Watch:          watch,
				IgnoreWatch:    ignoreWatch,
				CronRestart:    cronRestart,
			}

			if instances < 1 {
				instances = 1
			}
			names := instanceNames(args[0], instances)
			for _, name := range names {
				spec := base
				spec.Name = name
				resp, err := request(ipc.Request{Action: ipc.ActionStart, App: &spec})
				if err != nil {
					return err
				}
				if !resp.OK {
					return fmt.Errorf("%s", resp.Error)
				}
				fmt.Printf("started %q\n", name)
			}
			return nil
		},
	}

	f := cmd.Flags()
	f.StringVar(&command, "cmd", "", "command to run (required)")
	f.StringVar(&dir, "dir", "", "working directory")
	f.StringVar(&cwd, "cwd", "", "working directory (alias of --dir)")
	f.StringArrayVarP(&envFlags, "env", "e", nil, "environment variable KEY=VALUE (repeatable)")
	f.StringVar(&restart, "restart", "on-failure", "restart policy: always | on-failure | never")
	f.IntVar(&maxRetries, "max-retries", 0, "max restarts before giving up (0 = unlimited)")
	f.IntVar(&maxRestarts, "max-restarts", 0, "alias of --max-retries")
	f.IntVarP(&instances, "instances", "i", 1, "number of instances to launch")
	f.StringVar(&namespace, "namespace", "", "group the app under a namespace")
	f.BoolVar(&noAutorestart, "no-autorestart", false, "never restart automatically (same as --restart never)")
	f.BoolVar(&noAutostart, "no-autostart", false, "register the app without starting it")
	f.DurationVar(&killTimeout, "kill-timeout", 0, "grace period before SIGKILL (e.g. 10s)")
	f.DurationVar(&restartDelay, "restart-delay", 0, "fixed delay between restarts (e.g. 500ms)")
	f.DurationVar(&expBackoff, "exp-backoff-restart-delay", 0, "exponential backoff base delay (e.g. 200ms)")
	f.StringVar(&maxMemory, "max-memory-restart", "", "restart if memory exceeds this (e.g. 150M, 1G)")
	f.BoolVar(&watch, "watch", false, "restart when files change")
	f.StringArrayVar(&ignoreWatch, "ignore-watch", nil, "path fragment to ignore when watching (repeatable)")
	f.StringVar(&cronRestart, "cron-restart", "", "restart on a cron schedule (5-field, e.g. \"0 3 * * *\")")
	return cmd
}

// instanceNames returns the process names for n instances of base. A single
// instance keeps the bare name; multiple are suffixed -0..-(n-1).
func instanceNames(base string, n int) []string {
	if n <= 1 {
		return []string{base}
	}
	names := make([]string, n)
	for i := 0; i < n; i++ {
		names[i] = fmt.Sprintf("%s-%d", base, i)
	}
	return names
}

func parseEnv(pairs []string) (map[string]string, error) {
	if len(pairs) == 0 {
		return nil, nil
	}
	env := make(map[string]string, len(pairs))
	for _, p := range pairs {
		k, v, ok := strings.Cut(p, "=")
		if !ok || k == "" {
			return nil, fmt.Errorf("invalid --env %q, want KEY=VALUE", p)
		}
		env[k] = v
	}
	return env, nil
}

// parseBytes parses a human byte size like 150M, 1G, 512K, or a raw integer.
func parseBytes(s string) (int64, error) {
	s = strings.TrimSpace(strings.ToUpper(s))
	if s == "" {
		return 0, fmt.Errorf("empty size")
	}
	mult := int64(1)
	switch s[len(s)-1] {
	case 'K':
		mult = 1 << 10
		s = s[:len(s)-1]
	case 'M':
		mult = 1 << 20
		s = s[:len(s)-1]
	case 'G':
		mult = 1 << 30
		s = s[:len(s)-1]
	case 'B':
		s = s[:len(s)-1]
	}
	n, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("invalid size %q", s)
	}
	return int64(n * float64(mult)), nil
}
