package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/abdorizak/sm2/internal/config"
	"github.com/abdorizak/sm2/internal/ipc"
)

func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage the sm2.yaml configuration",
	}

	var path string
	cmd.PersistentFlags().StringVarP(&path, "config", "c", "", "path to sm2.yaml")

	cmd.AddCommand(
		newConfigInitCmd(&path),
		newConfigShowCmd(&path),
		newConfigValidateCmd(&path),
		newConfigReloadCmd(&path),
	)
	return cmd
}

func newConfigInitCmd(path *string) *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Write a starter sm2.yaml",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			target := *path
			if target == "" {
				target = "sm2.yaml"
			}
			if _, err := os.Stat(target); err == nil {
				return fmt.Errorf("%s already exists", target)
			}
			content := config.DefaultConfig(config.FormatFor(target))
			if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
				return err
			}
			fmt.Printf("wrote %s\n", target)
			return nil
		},
	}
}

func newConfigShowCmd(path *string) *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "Print the parsed configuration",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, resolved, err := loadResolved(*path)
			if err != nil {
				return err
			}
			out, err := cfg.Render(config.FormatFor(resolved))
			if err != nil {
				return err
			}
			fmt.Print(string(out))
			return nil
		},
	}
}

func newConfigValidateCmd(path *string) *cobra.Command {
	return &cobra.Command{
		Use:   "validate",
		Short: "Check the configuration for errors",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, resolved, err := loadResolved(*path)
			if err != nil {
				return err
			}
			if err := config.Validate(cfg); err != nil {
				return fmt.Errorf("invalid: %w", err)
			}
			fmt.Printf("%s is valid (%d app(s))\n", resolved, len(cfg.Specs()))
			return nil
		},
	}
}

func newConfigReloadCmd(path *string) *cobra.Command {
	return &cobra.Command{
		Use:   "reload",
		Short: "Apply the configuration to the running agent",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, resolved, err := loadResolved(*path)
			if err != nil {
				return err
			}
			if err := config.Validate(cfg); err != nil {
				return fmt.Errorf("invalid: %w", err)
			}
			abs, err := filepath.Abs(resolved)
			if err != nil {
				return err
			}

			resp, err := request(ipc.Request{Action: ipc.ActionReload, ConfigPath: abs})
			if err != nil {
				return err
			}
			if !resp.OK {
				return fmt.Errorf("%s", resp.Error)
			}
			fmt.Printf("applied %s\n", resolved)
			printStatus(resp.Apps)
			return nil
		},
	}
}

// loadResolved resolves the config path (flag → ./sm2.yaml → ~/.sm2) and loads it.
func loadResolved(flag string) (*config.Config, string, error) {
	resolved := config.ResolvePath(flag)
	if resolved == "" {
		return nil, "", fmt.Errorf("no sm2.yaml found (use --config or run 'sm2 config init')")
	}
	cfg, err := config.Load(resolved)
	if err != nil {
		return nil, "", err
	}
	return cfg, resolved, nil
}
