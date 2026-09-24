package ingestion

import (
	"homeworkcanvas/backend/internal/shared"
	"strings"
	"testing"
)

func TestValidateRequiresMatchingEvidence(t *testing.T) {
	pages := []Page{{Number: 3, Text: "实验一报告于 10 月 15 日提交"}}
	cases := []struct {
		name  string
		item  Extracted
		valid bool
	}{
		{"matching", Extracted{Title: "实验一", Evidence: []Evidence{{PageNumber: 3, TextExcerpt: "10 月 15 日提交"}}}, true},
		{"empty title", Extracted{Evidence: []Evidence{{PageNumber: 3, TextExcerpt: "实验一"}}}, false},
		{"missing evidence", Extracted{Title: "实验一"}, false},
		{"empty excerpt", Extracted{Title: "实验一", Evidence: []Evidence{{PageNumber: 3}}}, false},
		{"wrong page", Extracted{Title: "实验一", Evidence: []Evidence{{PageNumber: 4, TextExcerpt: "实验一"}}}, false},
		{"invented excerpt", Extracted{Title: "实验一", Evidence: []Evidence{{PageNumber: 3, TextExcerpt: "明天提交"}}}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := validate([]Extracted{tc.item}, pages) == nil; got != tc.valid {
				t.Fatalf("valid=%v, want %v", got, tc.valid)
			}
		})
	}
}

func TestCombinedFileAndMessageContext(t *testing.T) {
	message := "10月15日前提交，附件是实验要求"
	file := "实验一：实现三维场景"
	pages := withTeacherMessage(shared.SourceItem{SourceType: "file", RawText: &message}, locate([]Page{{Number: 1, Text: file}}))
	if len(pages) != 1 || pages[0].Number != 0 || pages[0].Text != message {
		t.Fatalf("teacher message missing when document has no assignment page: %+v", pages)
	}
	pages = withTeacherMessage(shared.SourceItem{SourceType: "file", RawText: &message}, []Page{{Number: 1, Text: file}})
	if len(pages) != 2 || pages[0].Number != 0 || pages[1].Number != 1 {
		t.Fatalf("combined message and document pages: %+v", pages)
	}
	if sourceChecksum("file-hash", message) == sourceChecksum("file-hash", "另一个截止时间") || sourceChecksum("file-hash", "") != "file-hash" {
		t.Fatal("file deduplication did not account for teacher message")
	}
}

func TestBoundPagesKeepsUnicodeAndBudget(t *testing.T) {
	pages := []Page{
		{Number: 1, Text: strings.Repeat("中", maxPageChars+1)},
		{Number: 2, Text: strings.Repeat("文", maxPageChars)},
		{Number: 3, Text: strings.Repeat("字", maxPageChars)},
		{Number: 4, Text: strings.Repeat("页", maxPageChars)},
		{Number: 5, Text: "最后一页"},
	}
	got, trimmed := boundPages(pages)
	if len(got) != 4 || trimmed != 3 {
		t.Fatalf("pages=%d trimmed=%d, want 4 and 3", len(got), trimmed)
	}
	total := 0
	for _, p := range got {
		if len([]rune(p.Text)) > maxPageChars {
			t.Fatalf("page %d exceeded page budget", p.Number)
		}
		total += len([]rune(p.Text))
	}
	if total != maxPromptChars || got[3].Number != 4 || len([]rune(got[3].Text)) != 4000 {
		t.Fatalf("total=%d last=%+v", total, got[3].Number)
	}
}

func TestParseDueAtBoundaries(t *testing.T) {
	cases := []struct {
		raw, today, want string
	}{
		{"2026-10-15T23:59:00+08:00", "2026-09-23", "2026-10-15T23:59:00+08:00"},
		{"1月5日 23:59", "2026-12-20", "2027-01-05T23:59:00+08:00"},
		{"12月30日", "2027-01-05", "2026-12-30T00:00:00+08:00"},
		{"2026-02-30", "2026-01-01", ""},
		{"2026-10-15 25:00", "2026-09-23", ""},
	}
	for _, tc := range cases {
		t.Run(tc.raw, func(t *testing.T) {
			got := parseDueAt(tc.raw, tc.today)
			if tc.want == "" {
				if got != nil {
					t.Fatalf("got %s, want invalid", got)
				}
				return
			}
			if got == nil || got.Format("2006-01-02T15:04:05-07:00") != tc.want {
				t.Fatalf("got %v, want %s", got, tc.want)
			}
		})
	}
}
