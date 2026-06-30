package cli

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/abdorizak/sm2/internal/ipc"
	"github.com/abdorizak/sm2/internal/logrotate"
)

func newSetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "set [key value]",
		Short: "Configure log rotation (and other runtime options)",
		Long: "Set a runtime option. Log rotation keys:\n" +
			"  logs.rotate     on|off — master switch (auto-enabled when you set any logs.* key)\n" +
			"  logs.max_size   size like 50M, 1G — rotate a log once it passes this size\n" +
			"  logs.retain     number of rotated files to keep\n" +
			"  logs.compress   true|false — gzip rotated files\n" +
			"  logs.interval   cron (e.g. \"0 0 * * *\") — also rotate on a schedule\n\n" +
			"Run `sm2 set` with no arguments to show the current settings.\n" +
			"Run `sm2 set logs.rotate now` to rotate every log immediately.",
		Args: cobra.MaximumNArgs(2),
		Example: "  sm2 set logs.max_size 50M\n" +
			"  sm2 set logs.retain 7\n" +
			"  sm2 set logs.compress true\n" +
			"  sm2 set logs.interval \"0 0 * * *\"\n" +
			"  sm2 set logs.rotate off\n" +
			"  sm2 set logs.rotate now",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return showRotate()
			}
			if len(args) != 2 {
				return fmt.Errorf("usage: sm2 set <key> <value> (see `sm2 set --help`)")
			}
			return setKey(args[0], args[1])
		},
	}
	return cmd
}

// setKey applies one logs.* option and persists it.
func setKey(key, val string) error {
	// "logs.rotate now" is a manual one-shot rotation, not a setting.
	if key == "logs.rotate" && val == "now" {
		resp, err := request(ipc.Request{Action: ipc.ActionRotateNow})
		if err != nil {
			return err
		}
		if !resp.OK {
			return fmt.Errorf("%s", resp.Error)
		}
		fmt.Printf("rotated %d log file(s)\n", resp.Rotated)
		return nil
	}

	// Start from the current settings so we only change one field.
	cur, err := request(ipc.Request{Action: ipc.ActionRotateGet})
	if err != nil {
		return err
	}
	cfg := ipc.LogRotateConfig{}
	if cur.LogRotate != nil {
		cfg = *cur.LogRotate
	}

	switch key {
	case "logs.rotate":
		on, err := parseOnOff(val)
		if err != nil {
			return err
		}
		cfg.Enabled = on
	case "logs.max_size":
		b, err := logrotate.ParseSize(val)
		if err != nil {
			return fmt.Errorf("logs.max_size: %w", err)
		}
		cfg.MaxSizeBytes = b
		cfg.Enabled = true
	case "logs.retain":
		n, err := strconv.Atoi(val)
		if err != nil || n < 0 {
			return fmt.Errorf("logs.retain must be a non-negative integer")
		}
		cfg.Retain = n
		cfg.Enabled = true
	case "logs.compress":
		on, err := parseOnOff(val)
		if err != nil {
			return err
		}
		cfg.Compress = on
		cfg.Enabled = true
	case "logs.interval":
		if val == "off" || val == "none" {
			val = ""
		}
		cfg.Interval = val
		cfg.Enabled = true
	default:
		return fmt.Errorf("unknown key %q (valid: logs.rotate, logs.max_size, logs.retain, logs.compress, logs.interval)", key)
	}

	resp, err := request(ipc.Request{Action: ipc.ActionRotateSet, LogRotate: &cfg})
	if err != nil {
		return err
	}
	if !resp.OK {
		return fmt.Errorf("%s", resp.Error)
	}
	fmt.Printf("set %s = %s\n", key, val)
	printRotate(resp.LogRotate)
	return nil
}

// showRotate prints the current log-rotation settings.
func showRotate() error {
	resp, err := request(ipc.Request{Action: ipc.ActionRotateGet})
	if err != nil {
		return err
	}
	if !resp.OK {
		return fmt.Errorf("%s", resp.Error)
	}
	printRotate(resp.LogRotate)
	return nil
}

func printRotate(c *ipc.LogRotateConfig) {
	if c == nil {
		fmt.Println("log rotation: not configured")
		return
	}
	state := "off"
	if c.Enabled {
		state = "on"
	}
	max := logrotate.HumanSize(c.MaxSizeBytes)
	if c.MaxSizeBytes <= 0 {
		max = fmt.Sprintf("%s (default)", logrotate.HumanSize(logrotate.DefaultMaxSize))
	}
	retain := c.Retain
	if retain <= 0 {
		retain = logrotate.DefaultRetain
	}
	interval := c.Interval
	if interval == "" {
		interval = "(none)"
	}
	fmt.Printf("log rotation: %s\n", state)
	fmt.Printf("  max_size:  %s\n", max)
	fmt.Printf("  retain:    %d\n", retain)
	fmt.Printf("  compress:  %t\n", c.Compress)
	fmt.Printf("  interval:  %s\n", interval)
}

// parseOnOff accepts on/off, true/false, 1/0, yes/no, enable/disable.
func parseOnOff(v string) (bool, error) {
	switch v {
	case "on", "yes", "enable", "enabled":
		return true, nil
	case "off", "no", "disable", "disabled":
		return false, nil
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return false, fmt.Errorf("expected on/off (got %q)", v)
	}
	return b, nil
}
