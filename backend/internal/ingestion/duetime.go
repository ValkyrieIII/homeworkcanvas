package ingestion

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

// 课程作业都在国内，时间统一按 UTC+8 处理，不再读 course_instances.timezone。
// courseTimezone 只是喂给模型的标签，解释工作全部由本文件的规则完成。
const courseTimezone = "Asia/Shanghai"

var chinaZone = time.FixedZone("UTC+8", 8*60*60)

// 模型给的带偏移时间按字面墙上时间落在 UTC+8：它写 "2026-10-01T00:00:00Z" 想表达的是
// 当地 10 月 1 日零点，而不是 UTC 零点，做偏移换算会整体差 8 小时。
// 整串匹配，避免把正文里的 "2.3" 当成日期；年份可省，时分秒可省。
var duePattern = regexp.MustCompile(`^(?:(\d{4})\s*[-/.年]\s*)?(\d{1,2})\s*[-/.月]\s*(\d{1,2})\s*[日号]?(?:[T ]+(\d{1,2})\s*[:：]\s*(\d{2})(?:\s*[:：]\s*(\d{2}))?(?:\.\d+)?\s*(?:Z|[+-]\d{2}:?\d{2})?)?$`)

// 表达式里出现四位年份即视为页面写明了年份（只看表达式，不看证据页原文，
// 否则 "2025级"、"CS2023" 这类数字会让年份被误判为已明确）。
var explicitYearPattern = regexp.MustCompile(`\d{4}`)

// parseDueAt 解析模型给的截止时间。解析不出来返回 nil，调用方应把该字段标为不确定，
// 而不是丢弃整批候选。缺年份时按"离当前最近的那一次"补。
func parseDueAt(raw, currentDate string) *time.Time {
	m := duePattern.FindStringSubmatch(strings.TrimSpace(raw))
	if m == nil {
		return nil
	}
	year, month, day := atoi(m[1]), atoi(m[2]), atoi(m[3])
	hour, minute, second := atoi(m[4]), atoi(m[5]), atoi(m[6])
	now := chinaNow(currentDate)
	if m[1] == "" {
		year = now.Year()
	}
	t := time.Date(year, time.Month(month), day, hour, minute, second, 0, chinaZone)
	// Go 会把 2月30日、25:00 静默进位，回读一遍才能拒绝不存在的日期。
	if t.Year() != year || t.Month() != time.Month(month) || t.Day() != day ||
		t.Hour() != hour || t.Minute() != minute || t.Second() != second {
		return nil
	}
	if m[1] == "" {
		t = anchorYear(t, now)
	}
	return &t
}

// anchorYear 保留月日时分秒，把年份落到离 now 最近的那一次：1 月上传的 "12月30日"
// 该落在刚过去的 12 月，12 月上传的 "1月5日" 该落在即将到来的 1 月。同样近则取将来。
func anchorYear(t time.Time, now time.Time) time.Time {
	best, bestDelta := t, time.Duration(1<<62)
	for y := now.Year() + 1; y >= now.Year()-1; y-- {
		candidate := time.Date(y, t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), 0, chinaZone)
		delta := candidate.Sub(now)
		if delta < 0 {
			delta = -delta
		}
		if delta < bestDelta {
			best, bestDelta = candidate, delta
		}
	}
	return best
}

func chinaNow(currentDate string) time.Time {
	if t, e := time.ParseInLocation("2006-01-02", currentDate, chinaZone); e == nil {
		return t
	}
	return time.Now().In(chinaZone)
}

func chinaToday() string { return time.Now().In(chinaZone).Format("2006-01-02") }

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}
