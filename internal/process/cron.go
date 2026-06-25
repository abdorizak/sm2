package process

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// cronSchedule is a parsed 5-field cron expression stored as bitsets:
// minute hour day-of-month month day-of-week.
type cronSchedule struct {
	min   uint64
	hour  uint64
	dom   uint64
	month uint64
	dow   uint64
}

// parseCron parses a standard 5-field cron expression. Each field supports
// "*", lists (a,b), ranges (a-b), and steps (*/n, a-b/n). Names are not
// supported. Day-of-week is 0-6 with 0 = Sunday.
func parseCron(expr string) (cronSchedule, error) {
	fields := strings.Fields(expr)
	if len(fields) != 5 {
		return cronSchedule{}, fmt.Errorf("cron expression must have 5 fields, got %d", len(fields))
	}
	var s cronSchedule
	var err error
	if s.min, err = parseCronField(fields[0], 0, 59); err != nil {
		return s, fmt.Errorf("minute: %w", err)
	}
	if s.hour, err = parseCronField(fields[1], 0, 23); err != nil {
		return s, fmt.Errorf("hour: %w", err)
	}
	if s.dom, err = parseCronField(fields[2], 1, 31); err != nil {
		return s, fmt.Errorf("day-of-month: %w", err)
	}
	if s.month, err = parseCronField(fields[3], 1, 12); err != nil {
		return s, fmt.Errorf("month: %w", err)
	}
	if s.dow, err = parseCronField(fields[4], 0, 6); err != nil {
		return s, fmt.Errorf("day-of-week: %w", err)
	}
	return s, nil
}

// match reports whether t falls on the schedule (minute resolution).
func (s cronSchedule) match(t time.Time) bool {
	return bitSet(s.min, t.Minute()) &&
		bitSet(s.hour, t.Hour()) &&
		bitSet(s.dom, t.Day()) &&
		bitSet(s.month, int(t.Month())) &&
		bitSet(s.dow, int(t.Weekday()))
}

func bitSet(mask uint64, n int) bool {
	return mask&(1<<uint(n)) != 0
}

func parseCronField(field string, min, max int) (uint64, error) {
	var mask uint64
	for _, part := range strings.Split(field, ",") {
		step := 1
		if i := strings.Index(part, "/"); i >= 0 {
			n, err := strconv.Atoi(part[i+1:])
			if err != nil || n <= 0 {
				return 0, fmt.Errorf("invalid step %q", part)
			}
			step = n
			part = part[:i]
		}

		lo, hi := min, max
		switch {
		case part == "*":
			// full range
		case strings.Contains(part, "-"):
			bounds := strings.SplitN(part, "-", 2)
			a, err1 := strconv.Atoi(bounds[0])
			b, err2 := strconv.Atoi(bounds[1])
			if err1 != nil || err2 != nil {
				return 0, fmt.Errorf("invalid range %q", part)
			}
			lo, hi = a, b
		default:
			v, err := strconv.Atoi(part)
			if err != nil {
				return 0, fmt.Errorf("invalid value %q", part)
			}
			lo, hi = v, v
		}

		if lo < min || hi > max || lo > hi {
			return 0, fmt.Errorf("value out of range [%d-%d]: %q", min, max, part)
		}
		for v := lo; v <= hi; v += step {
			mask |= 1 << uint(v)
		}
	}
	return mask, nil
}
