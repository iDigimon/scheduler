package scheduler

import (
	"errors"
	"strconv"
	"strings"
	"time"
)

// ParseCrontab 解析 crontab 表达式并返回下一次执行时间的计算函数
// crontab 表达式格式：分 时 日 月 周
// 支持的特殊字符：*
// 例如："0 0 * * *" 表示每天午夜执行
func ParseCrontab(crontab string) (func(time.Time) time.Time, error) {
	// 检查 crontab 表达式是否为空
	if crontab == "" {
		return nil, errors.New("crontab expression cannot be empty")
	}

	// 分割 crontab 表达式
	parts := strings.Fields(crontab)
	if len(parts) != 5 {
		return nil, errors.New("invalid crontab expression format, expected 5 fields")
	}

	// 解析分钟、小时、日期、月份、星期字段
	minuteStr := parts[0]
	hourStr := parts[1]
	dayOfMonthStr := parts[2]
	monthStr := parts[3]
	dayOfWeekStr := parts[4]

	// 返回计算下一次执行时间的函数
	return func(lastRun time.Time) time.Time {
		// 从上次执行时间的下一分钟开始查找下一次执行时间
		next := lastRun.Add(1 * time.Minute)
		next = time.Date(next.Year(), next.Month(), next.Day(), next.Hour(), 0, 0, 0, next.Location())

		// 最多查找一年
		maxIterations := 366 * 24 // 最多检查一年的时间
		iterations := 0

		for iterations < maxIterations {
			// 检查是否满足 crontab 规则
			if matchesCrontabRule(next, minuteStr, hourStr, dayOfMonthStr, monthStr, dayOfWeekStr) {
				return next
			}

			// 检查下一小时
			next = next.Add(1 * time.Hour)
			iterations++
		}

		// 如果在最大迭代次数内没有找到匹配的时间，返回默认时间
		return time.Time{}
	}, nil
}

// matchesCrontabRule 检查给定时间是否满足 crontab 规则
func matchesCrontabRule(t time.Time, minuteStr, hourStr, dayOfMonthStr, monthStr, dayOfWeekStr string) bool {
	// 检查分钟
	if !matchesField(t.Minute(), minuteStr) {
		return false
	}

	// 检查小时
	if !matchesField(t.Hour(), hourStr) {
		return false
	}

	// 检查日期
	if !matchesField(t.Day(), dayOfMonthStr) {
		return false
	}

	// 检查月份
	if !matchesField(int(t.Month()), monthStr) {
		return false
	}

	// 检查星期（注意：Go 的 time.Weekday 中星期日是 0，而 crontab 中通常星期日是 0 或 7）
	dayOfWeek := int(t.Weekday())
	if dayOfWeek == 0 { // 如果是星期日，也尝试匹配 7
		return matchesField(7, dayOfWeekStr) || matchesField(0, dayOfWeekStr)
	}
	return matchesField(dayOfWeek, dayOfWeekStr)
}

// matchesField 检查给定值是否匹配字段规则
func matchesField(value int, fieldRule string) bool {
	// 如果规则是 *, 则匹配所有值
	if fieldRule == "*" {
		return true
	}

	// 尝试将规则解析为整数
	ruleValue, err := strconv.Atoi(fieldRule)
	if err == nil {
		return value == ruleValue
	}

	// TODO: 支持更复杂的规则，如范围、列表、步长等
	// 目前只支持 * 和精确值
	return false
}

// GetNextRunTime 根据 crontab 表达式获取下一次执行时间
func GetNextRunTime(crontab string) (time.Time, error) {
	nextRunFunc, err := ParseCrontab(crontab)
	if err != nil {
		return time.Time{}, err
	}

	return nextRunFunc(time.Now()), nil
}