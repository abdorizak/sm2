package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/abdorizak/sm2/internal/config"
	"github.com/abdorizak/sm2/internal/pm2"
)

func newImportCmd() *cobra.Command {
	var (
		out     string
		envName string
		force   bool
	)
	cmd := &cobra.Command{
		Use:   "import <ecosystem file>",
		Short: "Convert a pm2 ecosystem file to sm2.yaml",
		Long: "Convert a pm2 ecosystem file (ecosystem.config.js, .cjs, .mjs, .json or .yml)\n" +
			"into an sm2.yaml, so a pm2 setup moves over without rewriting it.\n\n" +
			"Every app keeps its command, working directory, environment, instances,\n" +
			"restart and memory limits, watch and cron settings. pm2 options with no sm2\n" +
			"equivalent are listed as notes and written as comments into the file.\n\n" +
			"A .js file is code, so reading it needs Node.js (it is evaluated once,\n" +
			"exactly as pm2 would). The result doesn't need Node or pm2.",
		Example: "  sm2 import ecosystem.config.js\n" +
			"  sm2 import ecosystem.config.js --env production   # like pm2 start --env production\n" +
			"  sm2 import ecosystem.config.js -o -               # print instead of writing",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			src, err := filepath.Abs(args[0])
			if err != nil {
				return err
			}
			target := out
			if target == "" {
				target = filepath.Join(filepath.Dir(src), "sm2.yaml")
			}
			toStdout := target == "-"
			outDir := filepath.Dir(src)
			if !toStdout {
				if target, err = filepath.Abs(target); err != nil {
					return err
				}
				outDir = filepath.Dir(target)
				if _, err := os.Stat(target); err == nil && !force {
					return fmt.Errorf("%s already exists (use --force to overwrite, or -o another path)", target)
				}
			}

			res, err := pm2.Convert(src, outDir, envName)
			if err != nil {
				return err
			}
			if toStdout {
				os.Stdout.Write(res.YAML)
				for _, n := range res.Notes {
					fmt.Fprintln(os.Stderr, "note: "+n)
				}
				return nil
			}
			if err := os.WriteFile(target, res.YAML, 0o644); err != nil {
				return err
			}

			fmt.Printf("%s converted %d app(s) from %s → %s\n", green("✓"), len(res.Apps),
				filepath.Base(src), target)
			if len(res.Notes) > 0 {
				fmt.Println(bold("\nnotes (also written as comments in the file):"))
				for _, n := range res.Notes {
					fmt.Println("  " + yellow("•") + " " + n)
				}
			}

			// Validate here, but paths (directory, env_file) are checked on
			// this machine, so a config written for a server may not pass
			// locally. Report rather than fail.
			if cfg, err := config.Load(target); err != nil {
				fmt.Println(red("\nthe result doesn't parse: " + err.Error()))
			} else if err := config.Validate(cfg); err != nil {
				fmt.Println(yellow("\ncheck before applying: " + err.Error()))
			}

			rel := target
			if r, err := filepath.Rel(mustGetwd(), target); err == nil && len(r) < len(target) {
				rel = r
			}
			fmt.Println(bold("\nnext:"))
			fmt.Printf("  sm2 config validate -c %s\n", rel)
			fmt.Printf("  pm2 delete %s          %s\n", filepath.Base(src), dim("# stop the pm2 copies first (ports!)"))
			fmt.Printf("  sm2 config reload -c %s\n", rel)
			fmt.Printf("  sm2 save && sm2 startup   %s\n", dim("# survive reboots (then: pm2 unstartup)"))
			return nil
		},
	}
	cmd.Flags().StringVarP(&out, "output", "o", "", "where to write (default: sm2.yaml next to the ecosystem file; - for stdout)")
	cmd.Flags().StringVar(&envName, "env", "", "apply pm2's env_<name> block on top of env (like pm2 start --env <name>)")
	cmd.Flags().BoolVar(&force, "force", false, "overwrite an existing sm2.yaml")
	return cmd
}

func mustGetwd() string {
	wd, _ := os.Getwd()
	return wd
}
