package ingestion

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"gorm.io/gorm"
	"homeworkcanvas/backend/internal/courses"
	"homeworkcanvas/backend/internal/shared"
)

const maxUploadBytes int64 = 25 << 20

// 送进单次抽取的字符预算。按模型上下文 64K token 量级留出余量，
// 没有这层闸时一份上百页的课件会必现 context_length_exceeded。
const (
	maxPromptChars = 40000
	maxPageChars   = 12000
)

type Evidence struct {
	PageNumber  int    `json:"page_number"`
	TextExcerpt string `json:"text_excerpt"`
}
type Extracted struct {
	Title            string
	Description      string
	OfficialDueAt    *time.Time
	RawDueExpression string
	SubmissionMethod string
	Evidence         []Evidence
	Uncertain        []string
}
type Page struct {
	Number      int
	Text        string
	PreviewPath string
}
type ExtractionInput struct {
	CurrentDate string
	Pages       []Page
}

// Extractor 与 HTTP 和文档格式解耦，生产环境通过模型中转服务调用。
type Extractor interface {
	Extract(context.Context, ExtractionInput) ([]Extracted, error)
}
type Service struct {
	DB         *gorm.DB
	Membership courses.Service
	Queue      *asynq.Client
	Extractor  Extractor
	Files      shared.TemporaryFiles
}
type ReviewMetrics struct {
	Pending  int64 `json:"pending"`
	Accepted int64 `json:"accepted"`
	Rejected int64 `json:"rejected"`
}

func (s Service) ReviewMetrics(ctx context.Context, userID, courseInstanceID uuid.UUID) (ReviewMetrics, error) {
	if e := s.Membership.RequireMaintainer(ctx, userID, courseInstanceID); e != nil {
		return ReviewMetrics{}, e
	}
	var rows []struct {
		ReviewStatus string
		Count        int64
	}
	e := s.DB.WithContext(ctx).Table("assignment_candidates ac").Select("ac.review_status, COUNT(*) AS count").Joins("JOIN source_items si ON si.id=ac.source_item_id").Where("si.course_instance_id=?", courseInstanceID).Group("ac.review_status").Scan(&rows).Error
	if e != nil {
		return ReviewMetrics{}, e
	}
	metrics := ReviewMetrics{}
	for _, row := range rows {
		switch row.ReviewStatus {
		case "pending":
			metrics.Pending = row.Count
		case "accepted":
			metrics.Accepted = row.Count
		case "rejected":
			metrics.Rejected = row.Count
		}
	}
	return metrics, nil
}

func (s Service) CreateTextSource(ctx context.Context, u, ci uuid.UUID, text string) (shared.SourceItem, shared.IngestionJob, error) {
	if e := s.Membership.RequireActiveMember(ctx, u, ci); e != nil {
		return shared.SourceItem{}, shared.IngestionJob{}, e
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return shared.SourceItem{}, shared.IngestionJob{}, shared.JSONError("ingestion.invalid_text", "文字内容不能为空", 400)
	}
	sum := sha256.Sum256([]byte(text))
	checksum := hex.EncodeToString(sum[:])
	src := shared.SourceItem{CourseInstanceID: ci, SubmittedBy: u, SourceType: "text", RawText: &text, Checksum: &checksum, Status: "queued"}
	job := shared.IngestionJob{Stage: "queued", Status: "queued", MaxAttempts: 3}
	if e := s.createSourceAndJob(ctx, &src, &job); e != nil {
		return src, job, e
	}
	if e := s.enqueue(&job); e != nil {
		return src, job, s.enqueueFailed(&job, e)
	}
	return src, job, nil
}

func (s Service) CreateFileSource(ctx context.Context, u, ci uuid.UUID, file *multipart.FileHeader) (shared.SourceItem, shared.IngestionJob, error) {
	if e := s.Membership.RequireActiveMember(ctx, u, ci); e != nil {
		return shared.SourceItem{}, shared.IngestionJob{}, e
	}
	if file == nil || file.Size <= 0 || file.Size > maxUploadBytes {
		return shared.SourceItem{}, shared.IngestionJob{}, shared.JSONError("ingestion.invalid_file", "请选择不超过 25MB 的 PDF、PPT 或 PPTX 文件", 422)
	}
	ext := strings.ToLower(filepath.Ext(file.Filename))
	if ext != ".pdf" && ext != ".ppt" && ext != ".pptx" {
		return shared.SourceItem{}, shared.IngestionJob{}, shared.JSONError("ingestion.unsupported_file", "仅支持 PDF、PPT 和 PPTX 文件", 422)
	}
	name, mime, size := filepath.Base(file.Filename), file.Header.Get("Content-Type"), file.Size
	src := shared.SourceItem{CourseInstanceID: ci, SubmittedBy: u, SourceType: "file", OriginalFilename: &name, MimeType: &mime, ByteSize: &size, Status: "queued"}
	job := shared.IngestionJob{Base: shared.Base{ID: uuid.New()}, Stage: "queued", Status: "queued", MaxAttempts: 3}
	dir, e := s.Files.CreateJobDir(job.ID)
	if e != nil {
		return src, job, e
	}
	path, e := s.Files.SourcePath(job.ID, name)
	if e != nil {
		_ = s.Files.RemoveJob(job.ID)
		return src, job, e
	}
	in, e := file.Open()
	if e != nil {
		_ = s.Files.RemoveJob(job.ID)
		return src, job, e
	}
	defer in.Close()
	out, e := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if e != nil {
		_ = s.Files.RemoveJob(job.ID)
		return src, job, e
	}
	h := sha256.New()
	_, copyErr := io.Copy(io.MultiWriter(out, h), io.LimitReader(in, maxUploadBytes+1))
	closeErr := out.Close()
	if copyErr != nil || closeErr != nil {
		_ = s.Files.RemoveJob(job.ID)
		return src, job, fmt.Errorf("save upload: %v %v", copyErr, closeErr)
	}
	checksum := hex.EncodeToString(h.Sum(nil))
	src.Checksum = &checksum
	job.TemporaryLocator = &dir
	if e := s.createSourceAndJob(ctx, &src, &job); e != nil {
		_ = s.Files.RemoveJob(job.ID)
		return src, job, e
	}
	if e := s.enqueue(&job); e != nil {
		return src, job, s.enqueueFailed(&job, e)
	}
	return src, job, nil
}
func (s Service) createSourceAndJob(ctx context.Context, src *shared.SourceItem, job *shared.IngestionJob) error {
	return s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := tx.Create(src).Error; e != nil {
			return e
		}
		job.SourceItemID = src.ID
		return tx.Create(job).Error
	})
}
func (s Service) enqueue(job *shared.IngestionJob) error {
	if s.Queue == nil {
		return fmt.Errorf("queue client is not configured")
	}
	task, e := shared.NewJobTask(job.ID)
	if e != nil {
		return e
	}
	info, e := s.Queue.Enqueue(task, asynq.MaxRetry(job.MaxAttempts))
	if e != nil {
		return e
	}
	_ = s.DB.Model(job).Update("queue_task_id", info.ID)
	job.QueueTaskID = &info.ID
	return nil
}

// 投递失败时任务不能停在 queued 等一个永远不会来的执行，落成 failed 让调用方和前端立刻看到原因。
func (s Service) enqueueFailed(job *shared.IngestionJob, cause error) error {
	return s.fail(job, "ingestion.enqueue_failed", "任务投递失败："+truncateOneLine(cause.Error(), 200))
}

func (s Service) Process(ctx context.Context, jid uuid.UUID) error {
	var job shared.IngestionJob
	if e := s.DB.WithContext(ctx).First(&job, "id=?", jid).Error; e != nil {
		return e
	}
	if job.Status == "review_ready" || job.Status == "completed" || job.Status == "failed" {
		return nil
	}
	now := time.Now()
	// 显式记账，不依赖 GORM 把 map 里的值回写进 job —— 它确实会回写（update.go 的 assignValue），
	// 但那是副作用，一旦这里改成 gorm.Expr 就会静默失效，重试次数跟着错一轮。
	job.Attempts++
	_ = s.DB.Model(&job).Updates(map[string]any{"stage": "parsing", "status": "processing", "attempts": job.Attempts, "started_at": &now}).Error
	var src shared.SourceItem
	if e := s.DB.WithContext(ctx).First(&src, "id=?", job.SourceItemID).Error; e != nil {
		return s.fail(&job, "ingestion.source_missing", "来源记录不存在")
	}
	var courseInstance shared.CourseInstance
	if e := s.DB.WithContext(ctx).First(&courseInstance, "id=?", src.CourseInstanceID).Error; e != nil {
		return s.fail(&job, "ingestion.course_missing", "课程不存在")
	}
	_ = s.DB.Model(&src).Update("status", "processing")
	pages, e := s.parse(ctx, &job, &src)
	if e != nil {
		return s.fail(&job, "ingestion.parse_failed", "文件无法解析，请检查格式后重新上传")
	}
	_ = s.DB.Model(&job).Update("stage", "locating")
	if src.SourceType == "file" {
		pages = locate(pages)
		if len(pages) > 0 {
			if e := RenderPreviews(ctx, *src.OriginalFilename, *job.TemporaryLocator, pages); e != nil {
				return s.fail(&job, "ingestion.preview_failed", "文件页面预览生成失败，请重新上传")
			}
		}
	}
	if len(pages) == 0 {
		return s.fail(&job, "ingestion.no_assignment_page", "未发现疑似作业页面，请重新上传或改用文字导入")
	}
	_ = s.DB.Model(&job).Update("stage", "extracting")
	bounded, trimmed := boundPages(pages)
	if trimmed > 0 {
		log.Printf("ingestion prompt trimmed job_id=%s trimmed_pages=%d located_pages=%d", job.ID, trimmed, len(pages))
	}
	items, e := s.Extractor.Extract(ctx, ExtractionInput{CurrentDate: chinaToday(), Pages: bounded})
	if e != nil {
		return s.retryOrFail(&job, "extracting", "ingestion.ai_unavailable", "暂时无法抽取作业候选："+truncateOneLine(e.Error(), 300))
	}
	_ = s.DB.Model(&job).Update("stage", "validating")
	if len(items) == 0 || validate(items, pages) != nil {
		return s.fail(&job, "ingestion.invalid_result", "抽取结果无法校验")
	}
	e = s.DB.Transaction(func(tx *gorm.DB) error {
		if e := tx.Where("source_item_id=? AND review_status='pending'", src.ID).Delete(&shared.AssignmentCandidate{}).Error; e != nil {
			return e
		}
		for i, item := range items {
			ev, _ := json.Marshal(item.Evidence)
			unc, _ := json.Marshal(item.Uncertain)
			raw, method := item.RawDueExpression, item.SubmissionMethod
			c := shared.AssignmentCandidate{SourceItemID: src.ID, Ordinal: i + 1, Title: item.Title, Description: item.Description, OfficialDueAt: item.OfficialDueAt, RawDueExpression: &raw, SubmissionMethod: &method, Evidence: string(ev), FieldConfidence: "{}", UncertainFields: string(unc), ReviewStatus: "pending"}
			if e := tx.Create(&c).Error; e != nil {
				return e
			}
		}
		done := time.Now()
		return tx.Model(&job).Updates(map[string]any{"stage": "review_ready", "status": "review_ready", "finished_at": &done, "error_code": nil, "error_message": nil, "temporary_locator": nil}).Error
	})
	if e != nil {
		return e
	}
	if src.SourceType == "file" {
		_ = s.Files.RemoveJob(job.ID)
	}
	_ = s.DB.Model(&src).Update("status", "review_ready")
	return nil
}

// boundPages 把定位结果压进 prompt 预算：先按单页上限截断，再在总量用尽后丢弃尾部页面。
// 返回值是被截断或丢弃的页数。校验仍用未截断的原文，因为被截断部分只是前缀，
// 证据片段照样能对上；模型看不到的页面也不会产出候选。
func boundPages(pages []Page) ([]Page, int) {
	out := make([]Page, 0, len(pages))
	budget, trimmed := maxPromptChars, 0
	for _, p := range pages {
		if budget <= 0 {
			trimmed++
			continue
		}
		r, cut := []rune(p.Text), false
		if len(r) > maxPageChars {
			r, cut = r[:maxPageChars], true
		}
		if len(r) > budget {
			r, cut = r[:budget], true
		}
		if cut {
			trimmed++
		}
		budget -= len(r)
		out = append(out, Page{Number: p.Number, Text: string(r)})
	}
	return out, trimmed
}

func (s Service) parse(ctx context.Context, job *shared.IngestionJob, src *shared.SourceItem) ([]Page, error) {
	if src.SourceType == "text" && src.RawText != nil {
		return []Page{{Number: 1, Text: *src.RawText}}, nil
	}
	if src.SourceType != "file" || src.OriginalFilename == nil || job.TemporaryLocator == nil {
		return nil, fmt.Errorf("file expired")
	}
	path, e := s.Files.SourcePath(job.ID, *src.OriginalFilename)
	if e != nil {
		return nil, e
	}
	return ParseDocument(ctx, path, *src.OriginalFilename, *job.TemporaryLocator)
}
func (s Service) fail(j *shared.IngestionJob, code, msg string) error {
	now := time.Now()
	log.Printf("ingestion failed job_id=%s source_id=%s stage=%s attempts=%d code=%s message=%s", j.ID, j.SourceItemID, j.Stage, j.Attempts, code, msg)
	// 内存里的 job 也要同步，创建接口在投递失败时会把它作为响应体返回。
	j.Stage, j.Status, j.ErrorCode, j.ErrorMessage, j.FinishedAt, j.TemporaryLocator = "failed", "failed", &code, &msg, &now, nil
	_ = s.Files.RemoveJob(j.ID)
	if e := s.DB.Model(j).Updates(map[string]any{"stage": "failed", "status": "failed", "error_code": code, "error_message": msg, "finished_at": &now, "temporary_locator": nil}).Error; e != nil {
		return e
	}
	return s.DB.Model(&shared.SourceItem{}).Where("id=?", j.SourceItemID).Update("status", "failed").Error
}
func (s Service) retryOrFail(j *shared.IngestionJob, stage, code, msg string) error {
	// j.Attempts 是含本次在内的执行次数，Process 里已经记过账，这里不要再加一。
	if j.Attempts >= j.MaxAttempts {
		return s.fail(j, code, msg)
	}
	log.Printf("ingestion retry job_id=%s source_id=%s stage=%s attempt=%d/%d code=%s message=%s", j.ID, j.SourceItemID, stage, j.Attempts, j.MaxAttempts, code, msg)
	if e := s.DB.Model(j).Updates(map[string]any{"stage": stage, "status": "queued", "error_code": code, "error_message": msg}).Error; e != nil {
		return e
	}
	return fmt.Errorf("%s", code)
}
func validate(items []Extracted, pages []Page) error {
	valid := map[int]string{}
	for _, p := range pages {
		valid[p.Number] = p.Text
	}
	for _, i := range items {
		if strings.TrimSpace(i.Title) == "" {
			return fmt.Errorf("empty title")
		}
		for _, e := range i.Evidence {
			if t, ok := valid[e.PageNumber]; !ok || !strings.Contains(t, e.TextExcerpt) {
				return fmt.Errorf("invalid evidence")
			}
		}
	}
	return nil
}

// RuleExtractor 是开发环境的兜底实现，只接收定位后的页面，不会直接发布作业。
type RuleExtractor struct{}

func (RuleExtractor) Extract(_ context.Context, in ExtractionInput) ([]Extracted, error) {
	texts := make([]string, 0, len(in.Pages))
	evidence := make([]Evidence, 0, len(in.Pages))
	for _, p := range in.Pages {
		texts = append(texts, p.Text)
		excerpt := strings.TrimSpace(p.Text)
		r := []rune(excerpt)
		if len(r) > 240 {
			excerpt = string(r[:240])
		}
		evidence = append(evidence, Evidence{PageNumber: p.Number, TextExcerpt: excerpt})
	}
	all := strings.Join(texts, "\n")
	title := "待确认作业"
	if m := regexp.MustCompile(`(?i)(作业|实验)\s*([一二三四五六七八九十0-9A-Za-z]+)?`).FindString(all); m != "" {
		title = m
	}
	return []Extracted{{Title: title, Description: all, Evidence: evidence, Uncertain: []string{"official_due_at"}}}, nil
}
