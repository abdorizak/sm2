package cli

import "os"

// colorOn and boxOn are resolved once per invocation from flags + the terminal.
var (
	colorOn bool
	boxOn   bool
)

// setupOutput decides whether to emit ANSI colors and box-drawn tables. Rich
// output needs an interactive terminal (or SM2_FORCE_COLOR, e.g. when piping
// to a pager); color also honors --no-color and NO_COLOR.
func setupOutput(noColor, plain bool) {
	rich := isTTY(os.Stdout) || os.Getenv("SM2_FORCE_COLOR") != ""
	colorOn = rich && !noColor && os.Getenv("NO_COLOR") == ""
	boxOn = rich && !plain
}

func isTTY(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

func paint(code, s string) string {
	if !colorOn {
		return s
	}
	return "\033[" + code + "m" + s + "\033[0m"
}

func bold(s string) string   { return paint("1", s) }
func dim(s string) string    { return paint("2", s) }
func red(s string) string    { return paint("31", s) }
func green(s string) string  { return paint("32", s) }
func yellow(s string) string { return paint("33", s) }
func cyan(s string) string   { return paint("36", s) }

// colorState tints a process state by its meaning.
func colorState(state string) string {
	switch state {
	case "RUNNING":
		return green(state)
	case "FAILED":
		return red(state)
	case "STARTING", "RESTARTING":
		return yellow(state)
	case "STOPPED":
		return dim(state)
	default:
		return state
	}
}
