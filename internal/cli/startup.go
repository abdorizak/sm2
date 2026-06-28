package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/spf13/cobra"
)

const launchdLabel = "com.sm2.agent"

func newStartupCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "startup",
		Short: "Generate a boot service that resurrects saved apps",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			exe, err := os.Executable()
			if err != nil {
				return err
			}
			switch runtime.GOOS {
			case "darwin":
				return writeLaunchd(exe)
			case "linux":
				return writeSystemd(exe)
			default:
				return fmt.Errorf("startup is not supported on %s yet", runtime.GOOS)
			}
		},
	}
}

func newUnstartupCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "unstartup",
		Short: "Remove the sm2 boot service",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			var path, disable string
			switch runtime.GOOS {
			case "darwin":
				path = launchdPath()
				disable = fmt.Sprintf("launchctl unload %s", path)
			case "linux":
				path = systemdPath()
				disable = "systemctl --user disable --now sm2"
			default:
				return fmt.Errorf("startup is not supported on %s", runtime.GOOS)
			}
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				return err
			}
			fmt.Printf("removed %s\n", path)
			fmt.Printf("disable it now with:\n  %s\n", disable)
			return nil
		},
	}
}

func launchdPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "LaunchAgents", launchdLabel+".plist")
}

func systemdPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "systemd", "user", "sm2.service")
}

func writeLaunchd(exe string) error {
	path := launchdPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>%s</string>
    <key>ProgramArguments</key>
    <array>
        <string>%s</string>
        <string>resurrect</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <false/>
</dict>
</plist>
`, launchdLabel, exe)

	if err := os.WriteFile(path, []byte(plist), 0o644); err != nil {
		return err
	}
	fmt.Printf("wrote launchd agent: %s\n", path)
	fmt.Printf("enable it with:\n  launchctl load %s\n", path)
	fmt.Println("then save your process list any time with: sm2 save")
	return nil
}

func writeSystemd(exe string) error {
	path := systemdPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	unit := fmt.Sprintf(`[Unit]
Description=sm2 process agent
After=network.target

[Service]
Type=forking
ExecStart=%s resurrect
ExecStop=%s kill
Restart=on-failure

[Install]
WantedBy=default.target
`, exe, exe)

	if err := os.WriteFile(path, []byte(unit), 0o644); err != nil {
		return err
	}
	fmt.Printf("wrote systemd user unit: %s\n", path)
	fmt.Printf("enable it with:\n  systemctl --user daemon-reload && systemctl --user enable --now sm2\n")
	fmt.Println("then save your process list any time with: sm2 save")
	return nil
}
