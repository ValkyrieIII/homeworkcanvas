package ingestion

import (
	"regexp"
	"strings"
)

// “实验”“报告”会大量出现在课程说明中，不能单独触发候选页。
// 首版只把明确的作业、提交或截止表达视为强信号。
var assignmentWords = regexp.MustCompile(`(?i)(作业|提交|截止|deadline|due|ddl)`)
var contextWords = regexp.MustCompile(`(?i)(实验|报告)`)
var instructionHeadingWords = regexp.MustCompile(`(?i)(实验工具|实验步骤|实验要求|任务说明)`)
var taskPacketTitleWords = regexp.MustCompile(`(?i)(lab\s*[-_ ]*\d+|实验\s*[-_ ]*\d+|作业)`)
var dateWords = regexp.MustCompile(`(?i)(\d{1,2}[月/.-]\d{1,2}|\d{4}[-/]\d{1,2}[-/]\d{1,2}|周[一二三四五六日天])`)
var submitWords = regexp.MustCompile(`(?i)(提交|上传|发送|课程平台|邮箱)`)

func locate(pages []Page) []Page {
	if isDocumentWideTask(pages) {
		return pages
	}
	chosen := map[int]bool{}
	for i, p := range pages {
		score := 0
		t := strings.TrimSpace(p.Text)
		if assignmentWords.MatchString(t) {
			score += 3
		}
		if dateWords.MatchString(t) {
			score += 2
		}
		if submitWords.MatchString(t) {
			score += 2
		}
		if contextWords.MatchString(t) {
			score++
		}
		if len([]rune(t)) > 80 {
			score++
		}
		if assignmentWords.MatchString(t) && score >= 4 {
			for j := i - 1; j <= i+1; j++ {
				if j >= 0 && j < len(pages) {
					chosen[j] = true
				}
			}
		}
	}
	// 提交页常在任务说明之后：只沿着明确的实验章节标题向前补上下文，
	// 避免把“实验目的”等泛化介绍一并送入抽取器。
	for i := len(pages) - 1; i >= 0; i-- {
		if !chosen[i] {
			continue
		}
		for j := i - 1; j >= 0 && j >= i-3; j-- {
			if !instructionHeadingWords.MatchString(pages[j].Text) {
				break
			}
			chosen[j] = true
		}
	}
	out := make([]Page, 0, len(chosen))
	for i, p := range pages {
		if chosen[i] {
			out = append(out, p)
		}
	}
	return out
}

// Some course documents are themselves a complete lab handout rather than a deck
// containing one assignment notice. Preserve the full evidence set in that case.
func isDocumentWideTask(pages []Page) bool {
	if len(pages) == 0 || !taskPacketTitleWords.MatchString(pages[0].Text) {
		return false
	}
	contextual, strong := 0, false
	for _, page := range pages {
		text := page.Text
		if contextWords.MatchString(text) || assignmentWords.MatchString(text) {
			contextual++
		}
		if assignmentWords.MatchString(text) && (dateWords.MatchString(text) || submitWords.MatchString(text)) {
			strong = true
		}
	}
	return strong && contextual*100 >= len(pages)*70
}
