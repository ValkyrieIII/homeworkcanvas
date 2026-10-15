package ingestion

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
)

// PiExtractor 把"读材料 + 抽作业"整体交给本机的 pi coding agent。
// 结果不从它的文字回答里抠：pi 调用扩展注册的 submit_assignments 工具，
// 参数经 schema 校验后写到临时文件，这里再读回来。
type PiExtractor struct {
	Binary    string
	Provider  string
	Extension string
	SkillDir  string
	Timeout   time.Duration
}

func NewPiExtractor() PiExtractor {
	bin := strings.TrimSpace(os.Getenv("PI_BINARY"))
	provider := strings.TrimSpace(os.Getenv("PI_PROVIDER"))
	extension := strings.TrimSpace(os.Getenv("PI_EXTENSION"))
	skill := strings.TrimSpace(os.Getenv("PI_SKILL_DIR"))
	if bin == "" {
		bin = "pi"
	}
	if provider == "" {
		provider = "deepseek"
	}
	if extension == "" {
		// 相对 worker / api 的工作目录（backend/）。
		extension = "../agent/pi-tools/submit_assignments.js"
	}
	if skill == "" {
		skill = "../agent/skills/extract-assignments"
	}
	return PiExtractor{Binary: bin, Provider: provider, Extension: extension, SkillDir: skill, Timeout: 5 * time.Minute}
}

func (PiExtractor) ReadsSource() bool { return true }

func (e PiExtractor) Extract(ctx context.Context, in ExtractionInput) ([]Extracted, error) {
	ctx, cancel := context.WithTimeout(ctx, e.Timeout)
	defer cancel()

	outFile := filepath.Join(os.TempDir(), "pi-assignments-"+uuid.NewString()+".json")
	defer func() { _ = os.Remove(outFile) }()

	args := []string{
		"--provider", e.Provider,
		"--no-session", "-p",
		"--tools", "read,bash,submit_assignments",
	}
	if e.Extension != "" {
		args = append(args, "--extension", e.Extension)
	}
	if e.SkillDir != "" {
		args = append(args, "--skill", e.SkillDir)
	}
	args = append(args, e.prompt(in))

	cmd := exec.CommandContext(ctx, e.Binary, args...)
	cmd.Env = append(os.Environ(), "CANVAS_OUTPUT_FILE="+outFile)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("pi: %w: %s", err, truncateOneLine(stderr.String(), 300))
	}
	data, err := os.ReadFile(outFile)
	if err != nil {
		return nil, fmt.Errorf("pi 没有提交结果: %w: %s", err, truncateOneLine(stderr.String(), 300))
	}
	return parseSubmitted(data)
}

func (e PiExtractor) prompt(in ExtractionInput) string {
	parts := []string{
		"Use your tools to read the course material, then submit every assignment (作业/实验/项目) you find by calling the submit_assignments tool. Do not answer in plain text.",
	}
	if in.SourcePath != "" {
		parts = append(parts,
			"The material is the file at: "+in.SourcePath,
			"It may have no text layer or be a format you must convert first — use read/bash however works best to get at its content.",
		)
	}
	if in.SourceName != "" {
		parts = append(parts, "Original filename: "+in.SourceName)
	}
	if in.RawText != "" {
		parts = append(parts, "Accompanying note from the uploader: "+in.RawText)
	}
	if len(in.Courses) > 0 {
		lines := make([]string, 0, len(in.Courses)+1)
		lines = append(lines, "The user's course list — choose course_instance_id ONLY from here:")
		for _, c := range in.Courses {
			lines = append(lines, "- "+c.ID+" | "+c.Name+" | "+c.Term+" | "+c.Teacher)
		}
		parts = append(parts, strings.Join(lines, " "))
	}
	parts = append(parts, "Today is "+in.CurrentDate+".")
	// 单行：多行参数在 Windows 上传递不可靠。
	return strings.Join(parts, " ")
}

// piCandidate 对应扩展里 submit_assignments 的参数结构。
type piCandidate struct {
	Title            string       `json:"title"`
	Description      string       `json:"description"`
	OfficialDueAt    string       `json:"official_due_at"`
	RawDueExpression string       `json:"raw_due_expression"`
	SubmissionMethod string       `json:"submission_method"`
	CourseInstanceID string       `json:"course_instance_id"`
	Evidence         []piEvidence `json:"evidence"`
	Uncertain        []string     `json:"uncertain_fields"`
}

type piEvidence struct {
	PageNumber   *int   `json:"page_number"`
	TextExcerpt  string `json:"text_excerpt"`
	ExternalURL  string `json:"external_url"`
	OriginalName string `json:"original_name"`
}

func parseSubmitted(data []byte) ([]Extracted, error) {
	var rows []piCandidate
	if e := json.Unmarshal(data, &rows); e != nil {
		return nil, fmt.Errorf("提交结果不是合法 JSON: %w", e)
	}
	items := make([]Extracted, 0, len(rows))
	for _, r := range rows {
		title := strings.TrimSpace(r.Title)
		if title == "" {
			continue
		}
		evidence := make([]Evidence, 0, len(r.Evidence))
		for _, e := range r.Evidence {
			page := 0
			if e.PageNumber != nil {
				page = *e.PageNumber
			}
			evidence = append(evidence, Evidence{PageNumber: page, TextExcerpt: e.TextExcerpt})
		}
		items = append(items, Extracted{
			Title:            title,
			Description:      r.Description,
			OfficialDueAt:    parseISOTime(r.OfficialDueAt),
			RawDueExpression: r.RawDueExpression,
			SubmissionMethod: r.SubmissionMethod,
			CourseInstanceID: strings.TrimSpace(r.CourseInstanceID),
			Evidence:         evidence,
			Uncertain:        r.Uncertain,
		})
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("pi 提交了 0 条可用候选")
	}
	return items, nil
}

func parseISOTime(value string) *time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02 15:04:05", "2006-01-02"} {
		if t, e := time.Parse(layout, value); e == nil {
			return &t
		}
	}
	return nil
}
