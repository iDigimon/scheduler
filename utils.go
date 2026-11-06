package scheduler

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

type cronField int

const wildcard cronField = -1

func (f cronField) matches(value int) bool {
	return f == wildcard || int(f) == value
}

type cronRule struct {
	minute, hour, day, month, weekday cronField
}

// ParseCrontab 解析“分 时 日 月 周”五个字段，支持 * 和精确整数。
// 返回的函数按输入的时区查找严格晚于输入的匹配时刻，无解时返回零时间。
func ParseCrontab(crontab string) (func(time.Time) time.Time, error) {
	parts := strings.Fields(crontab)
	if len(parts) != 5 {
		return nil, fmt.Errorf("%w: expected 5 fields (minute hour day month weekday); seconds are not supported", ErrInvalidCrontab)
	}
	var fields [5]cronField
	for i, bounds := range [5][2]int{{0, 59}, {0, 23}, {1, 31}, {1, 12}, {0, 7}} {
		if parts[i] == "*" {
			fields[i] = wildcard
			continue
		}
		n, err := strconv.ParseUint(parts[i], 10, 8)
		if err != nil || int(n) < bounds[0] || int(n) > bounds[1] {
			return nil, fmt.Errorf("%w: field %d (%q) must be * or %d..%d", ErrInvalidCrontab, i+1, parts[i], bounds[0], bounds[1])
		}
		fields[i] = cronField(n)
	}
	rule := cronRule{fields[0], fields[1], fields[2], fields[3], fields[4]}
	if rule.weekday == 7 {
		rule.weekday = 0
	}
	if rule.month != wildcard && rule.day != wildcard {
		// 用闰年校验固定月日，保留 2 月 29 日。
		lastDay := time.Date(2000, time.Month(rule.month)+1, 0, 0, 0, 0, 0, time.UTC).Day()
		if int(rule.day) > lastDay {
			return nil, fmt.Errorf("%w: month %d has no day %d", ErrInvalidCrontab, rule.month, rule.day)
		}
	}
	return rule.next, nil
}

func (r cronRule) next(after time.Time) time.Time {
	// 公历的闰年、日期和星期每 400 年重复一次。
	limit := after.AddDate(400, 0, 0)
	for t := after.Add(time.Nanosecond); !t.After(limit); {
		var step time.Duration
		switch {
		case t.Second() != 0 || t.Nanosecond() != 0:
			step = time.Minute - time.Duration(t.Second())*time.Second - time.Duration(t.Nanosecond())
		case !r.month.matches(int(t.Month())) || !r.day.matches(t.Day()) || !r.weekday.matches(int(t.Weekday())):
			step = time.Duration((24-t.Hour())*60-t.Minute()) * time.Minute
		case !r.hour.matches(t.Hour()):
			step = time.Duration(60-t.Minute()) * time.Minute
		case !r.minute.matches(t.Minute()):
			step = time.Duration((int(r.minute)-t.Minute()+60)%60) * time.Minute
		default:
			return t
		}
		next := t.Add(step)
		// 跨越时区偏移变化时，从变化的瞬间重新匹配，兼容跳时和重复的分钟。
		_, boundary := t.ZoneBounds()
		if boundary.After(t) && boundary.Before(next) {
			next = boundary
		}
		t = next
	}
	return time.Time{}
}

// GetNextRunTime 返回当前时间之后的下一次匹配时刻。
func GetNextRunTime(crontab string) (time.Time, error) {
	next, err := ParseCrontab(crontab)
	if err != nil {
		return time.Time{}, err
	}
	return nextCronTime(next, time.Now())
}

func nextCronTime(next func(time.Time) time.Time, after time.Time) (time.Time, error) {
	t := next(after)
	if t.IsZero() || !t.After(after) {
		return time.Time{}, ErrNoNextRun
	}
	return t, nil
}
