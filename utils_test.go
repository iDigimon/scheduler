package scheduler

import (
	"testing"
	"time"
)

func TestCronNext(t *testing.T) {
	for _, tc := range []struct{ rule, from, want string }{
		{"* * * * *", "2026-11-06 10:15:30", "2026-11-06 10:16:00"},
		{"0 * * * *", "2026-11-06 10:15:30", "2026-11-06 11:00:00"},
		{"30 * * * *", "2026-11-06 10:30:00", "2026-11-06 11:30:00"},
		{"30 * * * *", "2026-11-06 10:15:30", "2026-11-06 10:30:00"},
		{"0 0 29 2 *", "2096-02-29 00:00:00", "2104-02-29 00:00:00"},
		{"0 0 1 * 1", "2026-11-06 10:15:30", "2027-02-01 00:00:00"},
		{"0 0 * * 0", "2026-11-06 10:15:30", "2026-11-08 00:00:00"},
		{"0 0 * * 7", "2026-11-06 10:15:30", "2026-11-08 00:00:00"},
	} {
		next, err := ParseCrontab(tc.rule)
		if err != nil {
			t.Fatal(err)
		}
		from, err := time.Parse(time.DateTime, tc.from)
		if err != nil {
			t.Fatal(err)
		}
		if got := next(from).Format(time.DateTime); got != tc.want {
			t.Errorf("%s 从 %s 起得到 %s，期望 %s", tc.rule, tc.from, got, tc.want)
		}
	}
}

func TestCronInvalid(t *testing.T) {
	for _, rule := range []string{"", "* *", "60 * * * *", "* 24 * * *", "* * 0 * *", "* * * 13 *", "* * * * 8", "*/5 * * * *", "x * * * *", "-1 * * * *", "0 0 30 2 *"} {
		if _, err := GetNextRunTime(rule); err == nil {
			t.Errorf("GetNextRunTime(%q) 应返回错误", rule)
		}
		if _, err := ParseCrontab(rule); err == nil {
			t.Errorf("ParseCrontab(%q) 应返回错误", rule)
		}
	}
}

func TestCronClockChanges(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ rule, from, want string }{
		{"30 2 * * *", "2026-03-08T01:59:59-05:00", "2026-03-09T02:30:00-04:00"},
		{"30 1 * * *", "2026-11-01T01:45:00-04:00", "2026-11-01T01:30:00-05:00"},
	} {
		from, err := time.Parse(time.RFC3339, tc.from)
		if err != nil {
			t.Fatal(err)
		}
		next, err := ParseCrontab(tc.rule)
		if err != nil {
			t.Fatal(err)
		}
		if got := next(from.In(loc)).Format(time.RFC3339); got != tc.want {
			t.Errorf("%s 得到 %s，期望 %s", tc.rule, got, tc.want)
		}
	}
}
