package cli

import (
	"strings"
	"unicode/utf8"
)

// column describes one box-table column.
type column struct {
	header string
	right  bool // right-align the cell
}

// renderBox returns a bordered table. Column widths are measured from the raw
// (uncolored) values; decorate(col, raw) supplies the display string, which may
// contain ANSI escapes that do not affect alignment.
func renderBox(cols []column, rows [][]string, decorate func(col int, raw string) string) string {
	n := len(cols)
	width := make([]int, n)
	for i, c := range cols {
		width[i] = runeLen(c.header)
	}
	for _, r := range rows {
		for i := 0; i < n; i++ {
			if l := runeLen(r[i]); l > width[i] {
				width[i] = l
			}
		}
	}

	var b strings.Builder
	border := func(left, mid, right string) {
		b.WriteString(left)
		for i := range cols {
			b.WriteString(strings.Repeat("─", width[i]+2))
			if i < n-1 {
				b.WriteString(mid)
			}
		}
		b.WriteString(right + "\n")
	}
	row := func(get func(i int) (raw, disp string)) {
		b.WriteString("│")
		for i := range cols {
			raw, disp := get(i)
			pad := width[i] - runeLen(raw)
			if pad < 0 {
				pad = 0
			}
			gap := strings.Repeat(" ", pad)
			if cols[i].right {
				b.WriteString(" " + gap + disp + " │")
			} else {
				b.WriteString(" " + disp + gap + " │")
			}
		}
		b.WriteString("\n")
	}

	border("┌", "┬", "┐")
	row(func(i int) (string, string) { return cols[i].header, bold(cols[i].header) })
	border("├", "┼", "┤")
	for _, r := range rows {
		r := r
		row(func(i int) (string, string) {
			raw := r[i]
			if decorate == nil {
				return raw, raw
			}
			return raw, decorate(i, raw)
		})
	}
	border("└", "┴", "┘")
	return b.String()
}

func runeLen(s string) int { return utf8.RuneCountInString(s) }
