package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/abdorizak/sm2/internal/ipc"
)

func newNotifyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "notify",
		Short: "Manage notifications (Discord)",
	}
	cmd.AddCommand(newNotifyDiscordCmd(), newNotifyTestCmd(), newNotifyStatusCmd())
	return cmd
}

func newNotifyDiscordCmd() *cobra.Command {
	var (
		webhook string
		disable bool
	)
	cmd := &cobra.Command{
		Use:   "discord",
		Short: "Enable Discord notifications with a webhook (or --disable)",
		Args:  cobra.NoArgs,
		Example: "  sm2 notify discord --webhook \"https://discord.com/api/webhooks/…\"\n" +
			"  sm2 notify discord --disable",
		RunE: func(cmd *cobra.Command, args []string) error {
			var dc ipc.DiscordConfig
			if disable {
				// Keep the stored webhook so re-enabling doesn't need it again.
				cur, err := request(ipc.Request{Action: ipc.ActionNotifyGet})
				if err == nil && cur.Discord != nil {
					dc.Webhook = cur.Discord.Webhook
				}
				dc.Enabled = false
			} else {
				if webhook == "" {
					return fmt.Errorf("--webhook is required (or use --disable)")
				}
				dc.Enabled = true
				dc.Webhook = webhook
			}

			resp, err := request(ipc.Request{Action: ipc.ActionNotifySet, Discord: &dc})
			if err != nil {
				return err
			}
			if !resp.OK {
				return fmt.Errorf("%s", resp.Error)
			}
			if dc.Enabled {
				fmt.Println("Discord notifications enabled")
			} else {
				fmt.Println("Discord notifications disabled")
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&webhook, "webhook", "", "Discord webhook URL")
	cmd.Flags().BoolVar(&disable, "disable", false, "turn Discord notifications off")
	return cmd
}

func newNotifyTestCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "test",
		Short: "Send a test notification",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := request(ipc.Request{Action: ipc.ActionNotifyTest})
			if err != nil {
				return err
			}
			if !resp.OK {
				return fmt.Errorf("%s", resp.Error)
			}
			fmt.Println("sent test notification")
			return nil
		},
	}
}

func newNotifyStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show the current notification settings",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := request(ipc.Request{Action: ipc.ActionNotifyGet})
			if err != nil {
				return err
			}
			if !resp.OK {
				return fmt.Errorf("%s", resp.Error)
			}
			d := resp.Discord
			if d == nil || d.Webhook == "" {
				fmt.Println("discord: not configured")
				return nil
			}
			state := "disabled"
			if d.Enabled {
				state = "enabled"
			}
			fmt.Printf("discord: %s (webhook %s)\n", state, maskWebhook(d.Webhook))
			return nil
		},
	}
}

// maskWebhook hides the secret token, showing only a short suffix.
func maskWebhook(url string) string {
	if len(url) <= 12 {
		return "set"
	}
	return url[:24] + "…" + url[len(url)-4:]
}
