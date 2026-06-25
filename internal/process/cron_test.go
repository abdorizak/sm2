package process

import (
	"testing"
	"time"
)

func TestParseCronAndMatch(t *testing.T) {
	cases := []struct {
		expr  string
		when  time.Time
		match bool
	}{
		{"* * * * *", time.Date(2026, 6, 25, 3, 0, 0, 0, time.UTC), true},
		{"0 3 * * *", time.Date(2026, 6, 25, 3, 0, 0, 0, time.UTC), true},
		{"0 3 * * *", time.Date(2026, 6, 25, 3, 1, 0, 0, time.UTC), false},
		{"*/15 * * * *", time.Date(2026, 6, 25, 9, 30, 0, 0, time.UTC), true},
		{"*/15 * * * *", time.Date(2026, 6, 25, 9, 31, 0, 0, time.UTC), false},
		{"0 9-17 * * 1-5", time.Date(2026, 6, 25, 13, 0, 0, 0, time.UTC), true},  // Thursday 13:00
		{"0 9-17 * * 1-5", time.Date(2026, 6, 27, 13, 0, 0, 0, time.UTC), false}, // Saturday
		{"30 0 1,15 * *", time.Date(2026, 6, 15, 0, 30, 0, 0, time.UTC), true},
		{"30 0 1,15 * *", time.Date(2026, 6, 16, 0, 30, 0, 0, time.UTC), false},
	}
	for _, c := range cases {
		s, err := parseCron(c.expr)
		if err != nil {
			t.Fatalf("parseCron(%q) error: %v", c.expr, err)
		}
		if got := s.match(c.when); got != c.match {
			t.Errorf("%q match %v: got %v, want %v", c.expr, c.when, got, c.match)
		}
	}
}

func TestParseCronErrors(t *testing.T) {
	for _, expr := range []string{"* * * *", "60 * * * *", "* 24 * * *", "a b c d e", "*/0 * * * *"} {
		if _, err := parseCron(expr); err == nil {
			t.Errorf("parseCron(%q) expected error, got nil", expr)
		}
	}
}
