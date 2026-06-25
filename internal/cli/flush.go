package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/cabdirizaaqyare/runix/internal/paths"
)

func newFlushCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "flush [name]",
		Short: "Empty log files (all apps, or one)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var targets []string
			if len(args) == 1 {
				targets = []string{paths.StdoutLog(args[0]), paths.StderrLog(args[0])}
			} else {
				entries, err := os.ReadDir(paths.LogDir())
				if err != nil {
					if os.IsNotExist(err) {
						fmt.Println("no logs to flush")
						return nil
					}
					return err
				}
				for _, e := range entries {
					name := e.Name()
					if strings.HasSuffix(name, ".stdout.log") || strings.HasSuffix(name, ".stderr.log") {
						targets = append(targets, filepath.Join(paths.LogDir(), name))
					}
				}
			}

			flushed := 0
			for _, p := range targets {
				if _, err := os.Stat(p); err != nil {
					continue
				}
				if err := os.Truncate(p, 0); err != nil {
					return fmt.Errorf("flush %s: %w", p, err)
				}
				flushed++
			}
			fmt.Printf("flushed %d log file(s)\n", flushed)
			return nil
		},
	}
}
