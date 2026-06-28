package cli

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/abdorizak/sm2/internal/paths"
)

func newLogsCmd() *cobra.Command {
	var (
		follow bool
		stderr bool
		lines  int
	)

	cmd := &cobra.Command{
		Use:   "logs <name>",
		Short: "Show logs for an application",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := paths.StdoutLog(args[0])
			if stderr {
				path = paths.StderrLog(args[0])
			}

			f, err := os.Open(path)
			if err != nil {
				if os.IsNotExist(err) {
					return fmt.Errorf("no logs for %q yet", args[0])
				}
				return err
			}
			defer f.Close()

			if err := printLastLines(f, lines); err != nil {
				return err
			}
			if !follow {
				return nil
			}
			return tail(f)
		},
	}

	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "stream new log lines as they arrive")
	cmd.Flags().BoolVar(&stderr, "stderr", false, "show the stderr log instead of stdout")
	cmd.Flags().IntVarP(&lines, "lines", "n", 50, "number of trailing lines to show")
	return cmd
}

// printLastLines prints up to n trailing lines of f and leaves the offset at EOF.
func printLastLines(f *os.File, n int) error {
	data, err := io.ReadAll(f)
	if err != nil {
		return err
	}
	lines := splitLines(data)
	if n > 0 && len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	for _, l := range lines {
		fmt.Println(l)
	}
	return nil
}

func splitLines(data []byte) []string {
	var out []string
	s := bufio.NewScanner(bytes.NewReader(data))
	s.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for s.Scan() {
		out = append(out, s.Text())
	}
	return out
}

// tail polls the file for appended content and prints it until interrupted.
func tail(f *os.File) error {
	r := bufio.NewReader(f)
	for {
		line, err := r.ReadString('\n')
		if len(line) > 0 {
			fmt.Print(line)
			continue
		}
		if err == io.EOF {
			time.Sleep(300 * time.Millisecond)
			continue
		}
		if err != nil {
			return err
		}
	}
}
