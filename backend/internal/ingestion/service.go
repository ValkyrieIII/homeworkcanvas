package ingestion

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"io"
	"log/slog"
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
	// CourseInstanceID 是 agent 判定的课程；传统实现留空。
	CourseInstanceID string
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

	// 下面几项给"能自己读原始材料"的抽取器（agent）用；传统实现忽略它们。
	SourcePath string
	SourceName string
	RawText    string
	// Courses 是调用者自己的课程清单，agent 判课程时只能从这里选。
	Courses []CourseOption
}

// CourseOption 是喂给 agent 的课程选项。
type CourseOption struct {
	ID      string
	Name    string
	Term    string
	Teacher string
}

// Extractor 与 HTTP 和文档格式解耦，生产环境通过模型中转服务调用。
type Extractor interface {
	Extract(context.Context, ExtractionInput) ([]Extracted, error)
}

// SourceReadingExtractor 表示抽取器自己读原始材料，不需要系统先把它解析成页，
// 所以也跳过页级证据校验与"未发现作业页"判断。
type SourceReadingExtractor interface {
	Extractor
	ReadsSource() bool
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

type FileSourceResult struct {
	Source shared.SourceItem
	Job    shared.IngestionJob
	Reused bool
}

type MineSource struct {
	ID               uuid.UUID `json:"id"`
	CourseInstanceID uuid.UUID `json:"course_instance_id"`
	CourseName       string    `json:"course_name"`
	SubmittedBy      uuid.UUID `json:"submitted_by"`
	SubmitterName    string    `json:"submitter_name"`
	SourceType       string    `json:"source_type"`
	OriginalFilename *string   `json:"original_filename"`
	Status           string    `json:"status"`
	JobID            uuid.UUID `json:"job_id"`
	Stage            string    `json:"stage"`
	ErrorMessage     *string   `json:"error_message"`
	CreatedAt        time.Time `json:"created_at"`
}

func (s Service) ListMine(ctx context.Context, user uuid.UUID) ([]MineSource, error) {
	rows := make([]MineSource, 0)
	err := s.DB.WithContext(ctx).Table("source_items si").
		Select(`si.id,si.course_instance_id,c.name AS course_name,si.submitted_by,
			u.display_name AS submitter_name,si.source_type,si.original_filename,
			si.status,ij.id AS job_id,ij.stage,ij.error_message,si.created_at`).
		Joins("LEFT JOIN course_instances ci ON ci.id=si.course_instance_id").
		Joins("LEFT JOIN courses c ON c.id=ci.course_id").
		Joins("JOIN users u ON u.id=si.submitted_by").
		Joins("LEFT JOIN ingestion_jobs ij ON ij.source_item_id=si.id").
		Where("si.submitted_by=?", user).Order("si.created_at DESC,si.id DESC").Scan(&rows).Error
	return rows, err
}

func (s Service) ListCourseSources(ctx context.Context, actor, ci uuid.UUID) ([]MineSource, error) {
	if err := s.Membership.RequireAdmin(ctx, actor); err != nil {
		return nil, err
	}
	var rows []MineSource
	err := s.DB.WithContext(ctx).Table("source_items si").
		Select(`si.id,si.course_instance_id,c.name AS course_name,si.submitted_by,
			u.display_name AS submitter_name,si.source_type,si.original_filename,
			si.status,ij.id AS job_id,ij.stage,ij.error_message,si.created_at`).
		Joins("JOIN course_instances ci ON ci.id=si.course_instance_id").
		Joins("JOIN courses c ON c.id=ci.course_id").
		Joins("JOIN users u ON u.id=si.submitted_by").
		Joins("JOIN ingestion_jobs ij ON ij.source_item_id=si.id").
		Where("si.course_instance_id=?", ci).Order("si.created_at DESC,si.id DESC").Scan(&rows).Error
	return rows, err
}

// ListAdminIssues 只汇总失败和长时间未完成的来源，不暴露原始文件内容。
func (s Service) ListAdminIssues(ctx context.Context, actor uuid.UUID, courseID *uuid.UUID, limit int, rawCursor string) ([]MineSource, *string, error) {
	if err := s.Membership.RequireAdmin(ctx, actor); err != nil {
		return nil, nil, err
	}
	q := s.DB.WithContext(ctx).Table("source_items si").
		Select(`si.id,si.course_instance_id,c.name AS course_name,si.submitted_by,
			u.display_name AS submitter_name,si.source_type,si.original_filename,
			si.status,ij.id AS job_id,ij.stage,ij.error_message,si.created_at`).
		Joins("JOIN course_instances ci ON ci.id=si.course_instance_id").
		Joins("JOIN courses c ON c.id=ci.course_id").
		Joins("JOIN users u ON u.id=si.submitted_by").
		Joins("JOIN ingestion_jobs ij ON ij.source_item_id=si.id").
		Where("(si.status='failed' OR (si.status IN ('queued','processing') AND si.created_at < ?))", time.Now().Add(-15*time.Minute))
	if courseID != nil {
		q = q.Where("si.course_instance_id=?", *courseID)
	}
	if rawCursor != "" {
		cursor, err := shared.DecodePageCursor(rawCursor)
		if err != nil || cursor.Time == nil {
			return nil, nil, shared.JSONError("shared.invalid_cursor", "分页位置无效", 400)
		}
		q = q.Where("si.created_at < ? OR (si.created_at = ? AND si.id < ?)", *cursor.Time, *cursor.Time, cursor.ID)
	}
	rows := make([]MineSource, 0)
	err := q.Order("si.created_at DESC,si.id DESC").Limit(limit + 1).Scan(&rows).Error
	if err != nil {
		return nil, nil, err
	}
	var next *string
	if len(rows) > limit {
		rows = rows[:limit]
		last := rows[len(rows)-1]
		value := shared.EncodePageCursor(shared.PageCursor{Time: &last.CreatedAt, ID: last.ID})
		next = &value
	}
	return rows, next, nil
}

func (s Service) ReviewMetrics(ctx context.Context, userID, courseInstanceID uuid.UUID) (ReviewMetrics, error) {
	if e := s.Membership.RequireAdmin(ctx, userID); e != nil {
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

// coursePtr 把可选的课程 id 转成指针：零值表示"来源不带课程"（写 NULL）。
func coursePtr(id uuid.UUID) *uuid.UUID {
	if id == uuid.Nil {
		return nil
	}
	return &id
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
	src := shared.SourceItem{CourseInstanceID: coursePtr(ci), SubmittedBy: u, SourceType: "text", RawText: &text, Checksum: &checksum, Status: "queued"}
	job := shared.IngestionJob{Stage: "queued", Status: "queued", MaxAttempts: 3}
	if e := s.createSourceAndJob(ctx, &src, &job); e != nil {
		return src, job, e
	}
	if e := s.enqueue(&job); e != nil {
		return src, job, s.enqueueFailed(&job, e)
	}
	return src, job, nil
}

// AgentSubmission 是 agent（本机 pi 或集市 agent）投递上来的一组提取结果。
// 材料在 agent 那边已经读完了：这里不解析、不调模型，只负责接住并落成待确认候选。
type AgentSubmission struct {
	SourceKey string      `json:"source_key"`
	Note      string      `json:"note"`
	Items     []AgentItem `json:"items"`
}

type AgentItem struct {
	Title            string          `json:"title"`
	Description      string          `json:"description"`
	OfficialDueAt    *time.Time      `json:"official_due_at"`
	RawDueExpression *string         `json:"raw_due_expression"`
	SubmissionMethod *string         `json:"submission_method"`
	Evidence         []AgentEvidence `json:"evidence"`
	Uncertain        []string        `json:"uncertain_fields"`
}

// AgentEvidence 一条依据：可以是外链，也可以指向某份材料的具体页。
type AgentEvidence struct {
	ExternalURL  *string `json:"external_url,omitempty"`
	OriginalName *string `json:"original_name,omitempty"`
	PageNumber   *int    `json:"page_number,omitempty"`
	TextExcerpt  *string `json:"text_excerpt,omitempty"`
	Note         *string `json:"note,omitempty"`
}

// SubmitAgent 落一条投递。同课程内同一 source_key 只落一次，重复投递返回已有候选。
func (s Service) SubmitAgent(ctx context.Context, u, ci uuid.UUID, in AgentSubmission) (shared.SourceItem, []shared.AssignmentCandidate, error) {
	if e := s.Membership.RequireActiveMember(ctx, u, ci); e != nil {
		return shared.SourceItem{}, nil, e
	}
	if len(in.Items) == 0 {
		return shared.SourceItem{}, nil, shared.JSONError("ingestion.empty_submission", "投递内容为空", 400)
	}
	for i, it := range in.Items {
		if strings.TrimSpace(it.Title) == "" {
			return shared.SourceItem{}, nil, shared.JSONError("ingestion.invalid_item", fmt.Sprintf("第 %d 条缺少标题", i+1), 400)
		}
	}
	var src shared.SourceItem
	var out []shared.AssignmentCandidate
	e := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var checksum *string
		if key := strings.TrimSpace(in.SourceKey); key != "" {
			sum := sha256.Sum256([]byte(key))
			v := hex.EncodeToString(sum[:])
			checksum = &v
			var existing shared.SourceItem
			e := tx.Where("course_instance_id=? AND source_type='agent' AND checksum=?", ci, v).
				Order("created_at ASC, id ASC").First(&existing).Error
			if e == nil {
				src = existing
				return tx.Where("source_item_id=?", existing.ID).Order("ordinal ASC").Find(&out).Error
			}
			if e != gorm.ErrRecordNotFound {
				return e
			}
		}
		note := strings.TrimSpace(in.Note)
		src = shared.SourceItem{CourseInstanceID: coursePtr(ci), SubmittedBy: u, SourceType: "agent", Status: "review_ready", Checksum: checksum}
		if note != "" {
			src.RawText = &note
		}
		if e := tx.Create(&src).Error; e != nil {
			return e
		}
		for i, it := range in.Items {
			evidence, e := json.Marshal(it.Evidence)
			if e != nil {
				return e
			}
			if len(it.Evidence) == 0 {
				evidence = []byte("[]")
			}
			uncertain, e := json.Marshal(it.Uncertain)
			if e != nil {
				return e
			}
			if len(it.Uncertain) == 0 {
				uncertain = []byte("[]")
			}
			c := shared.AssignmentCandidate{
				SourceItemID:     src.ID,
				Ordinal:          i + 1,
				Title:            strings.TrimSpace(it.Title),
				Description:      it.Description,
				OfficialDueAt:    it.OfficialDueAt,
				RawDueExpression: it.RawDueExpression,
				SubmissionMethod: it.SubmissionMethod,
				Evidence:         string(evidence),
				FieldConfidence:  "{}",
				UncertainFields:  string(uncertain),
				ReviewStatus:     "pending",
			}
			if e := tx.Create(&c).Error; e != nil {
				return e
			}
			out = append(out, c)
		}
		return nil
	})
	return src, out, e
}

func (s Service) CreateFileSource(ctx context.Context, u, ci uuid.UUID, file *multipart.FileHeader, message string) (FileSourceResult, error) {
	// ci 为零值表示走统一入口：不要求先选课，课程由 agent 判在候选上。
	if ci != uuid.Nil {
		if e := s.Membership.RequireActiveMember(ctx, u, ci); e != nil {
			return FileSourceResult{}, e
		}
	}
	if file == nil || file.Size <= 0 || file.Size > maxUploadBytes {
		return FileSourceResult{}, shared.JSONError("ingestion.invalid_file", "请选择不超过 25MB 的 PDF、PPT 或 PPTX 文件", 422)
	}
	ext := strings.ToLower(filepath.Ext(file.Filename))
	if ext != ".pdf" && ext != ".ppt" && ext != ".pptx" {
		return FileSourceResult{}, shared.JSONError("ingestion.unsupported_file", "仅支持 PDF、PPT 和 PPTX 文件", 422)
	}
	name, mime, size := filepath.Base(file.Filename), file.Header.Get("Content-Type"), file.Size
	src := shared.SourceItem{CourseInstanceID: coursePtr(ci), SubmittedBy: u, SourceType: "file", OriginalFilename: &name, MimeType: &mime, ByteSize: &size, Status: "queued"}
	message = strings.TrimSpace(message)
	if message != "" {
		src.RawText = &message
	}
	job := shared.IngestionJob{Base: shared.Base{ID: uuid.New()}, Stage: "queued", Status: "queued", MaxAttempts: 3}
	dir, e := s.Files.CreateJobDir(job.ID)
	if e != nil {
		return FileSourceResult{}, e
	}
	path, e := s.Files.SourcePath(job.ID, name)
	if e != nil {
		_ = s.Files.RemoveJob(job.ID)
		return FileSourceResult{}, e
	}
	in, e := file.Open()
	if e != nil {
		_ = s.Files.RemoveJob(job.ID)
		return FileSourceResult{}, e
	}
	defer in.Close()
	out, e := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if e != nil {
		_ = s.Files.RemoveJob(job.ID)
		return FileSourceResult{}, e
	}
	h := sha256.New()
	_, copyErr := io.Copy(io.MultiWriter(out, h), io.LimitReader(in, maxUploadBytes+1))
	closeErr := out.Close()
	if copyErr != nil || closeErr != nil {
		_ = s.Files.RemoveJob(job.ID)
		return FileSourceResult{}, fmt.Errorf("save upload: %v %v", copyErr, closeErr)
	}
	checksum := sourceChecksum(hex.EncodeToString(h.Sum(nil)), message)
	src.Checksum = &checksum
	job.TemporaryLocator = &dir
	newJobID := job.ID
	reused, e := s.createOrReuseFileSource(ctx, &src, &job)
	if e != nil || reused {
		if cleanupErr := s.Files.RemoveJob(newJobID); cleanupErr != nil {
			slog.Error("ingestion_cleanup_error", "job_id", newJobID, "error_type", fmt.Sprintf("%T", cleanupErr))
		}
		if e != nil {
			return FileSourceResult{}, e
		}
		return FileSourceResult{Source: src, Job: job, Reused: true}, nil
	}
	if e := s.enqueue(&job); e != nil {
		return FileSourceResult{Source: src, Job: job}, s.enqueueFailed(&job, e)
	}
	return FileSourceResult{Source: src, Job: job}, nil
}

// Serializes identical uploads across API processes before checking existing work.
// Failed jobs are deliberately excluded so a corrected environment can be retried.
func (s Service) createOrReuseFileSource(ctx context.Context, src *shared.SourceItem, job *shared.IngestionJob) (bool, error) {
	if src.Checksum == nil {
		return false, fmt.Errorf("file checksum is required")
	}
	h := fnv.New64a()
	courseKey := uuid.Nil
	if src.CourseInstanceID != nil {
		courseKey = *src.CourseInstanceID
	}
	_, _ = h.Write(courseKey[:])
	_, _ = h.Write(src.SubmittedBy[:])
	_, _ = h.Write([]byte(*src.Checksum))
	var reused bool
	e := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := tx.Exec("SELECT pg_advisory_xact_lock(?)", int64(h.Sum64())).Error; e != nil {
			return e
		}
		var existing shared.SourceItem
		q := tx.Where("submitted_by=? AND source_type='file' AND checksum=? AND status<>'failed'", src.SubmittedBy, *src.Checksum)
		if src.CourseInstanceID == nil {
			q = q.Where("course_instance_id IS NULL")
		} else {
			q = q.Where("course_instance_id=?", *src.CourseInstanceID)
		}
		e := q.Order("created_at ASC, id ASC").First(&existing).Error
		if e == nil {
			var existingJob shared.IngestionJob
			if e := tx.Where("source_item_id=?", existing.ID).First(&existingJob).Error; e != nil {
				return e
			}
			*src, *job, reused = existing, existingJob, true
			return nil
		}
		if e != gorm.ErrRecordNotFound {
			return e
		}
		if e := tx.Create(src).Error; e != nil {
			return e
		}
		job.SourceItemID = src.ID
		return tx.Create(job).Error
	})
	return reused, e
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
	started := time.Now()
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
	job.Stage = "parsing"
	if e := s.DB.WithContext(ctx).Model(&job).Updates(map[string]any{"stage": job.Stage, "status": "processing", "attempts": job.Attempts, "started_at": &now}).Error; e != nil {
		return e
	}
	slog.Info("ingestion_stage", "job_id", job.ID, "source_id", job.SourceItemID, "stage", job.Stage, "attempt", job.Attempts)
	var src shared.SourceItem
	if e := s.DB.WithContext(ctx).First(&src, "id=?", job.SourceItemID).Error; e != nil {
		return s.fail(&job, "ingestion.source_missing", "来源记录不存在")
	}
	// 统一入口里来源可以不带课程——课程由 agent 判在候选上。
	var courseInstance shared.CourseInstance
	if src.CourseInstanceID != nil {
		if e := s.DB.WithContext(ctx).First(&courseInstance, "id=?", *src.CourseInstanceID).Error; e != nil {
			return s.fail(&job, "ingestion.course_missing", "课程不存在")
		}
	}
	if e := s.DB.WithContext(ctx).Model(&src).Update("status", "processing").Error; e != nil {
		return e
	}
	// 能自己读原始材料的抽取器（agent）不走解析/定位/页级校验这一套：
	// 材料怎么读、证据长什么样，由它自己决定。
	usesSource := false
	if r, ok := s.Extractor.(SourceReadingExtractor); ok {
		usesSource = r.ReadsSource()
	}

	var pages []Page
	if !usesSource {
		parsed, e := s.parse(ctx, &job, &src)
		if e != nil {
			slog.Error("ingestion_parse_error", "job_id", job.ID, "source_id", src.ID, "error_type", fmt.Sprintf("%T", e))
			return s.fail(&job, "ingestion.parse_failed", "文件无法解析，请检查格式后重新上传")
		}
		pages = parsed
		if e := s.setStage(ctx, &job, "locating", started, len(pages)); e != nil {
			return e
		}
		if src.SourceType == "file" && src.RawText == nil {
			pages = locate(pages)
			if len(pages) > 0 {
				if e := RenderPreviews(ctx, *src.OriginalFilename, *job.TemporaryLocator, pages); e != nil {
					return s.fail(&job, "ingestion.preview_failed", "文件页面预览生成失败，请重新上传")
				}
			}
		}
		// Page zero is the accompanying teacher message. When it exists, keep
		// the document pages too: the message may contain the only deadline or
		// assignment signal, so locating the document alone would lose context.
		pages = withTeacherMessage(src, pages)
		if len(pages) == 0 {
			return s.fail(&job, "ingestion.no_assignment_page", "未发现疑似作业页面，请重新上传或改用文字导入")
		}
	}
	if e := s.setStage(ctx, &job, "extracting", started, len(pages)); e != nil {
		return e
	}
	input := ExtractionInput{CurrentDate: chinaToday()}
	if !usesSource {
		bounded, trimmed := boundPages(pages)
		if trimmed > 0 {
			slog.Warn("ingestion_prompt_trimmed", "job_id", job.ID, "trimmed_pages", trimmed, "located_pages", len(pages))
		}
		input.Pages = bounded
	} else if src.OriginalFilename != nil && job.TemporaryLocator != nil {
		input.SourcePath = filepath.Join(*job.TemporaryLocator, "source"+strings.ToLower(filepath.Ext(*src.OriginalFilename)))
		input.SourceName = *src.OriginalFilename
	}
	if src.RawText != nil {
		input.RawText = *src.RawText
	}
	// agent 判课程时只能从调用者自己的课里选，所以把清单一起给它。
	if usesSource {
		if instances, e := s.Membership.ListInstances(ctx, src.SubmittedBy, true); e == nil {
			for _, it := range instances {
				teacher := ""
				if it.TeacherName != nil {
					teacher = *it.TeacherName
				}
				input.Courses = append(input.Courses, CourseOption{ID: it.ID.String(), Name: it.CourseName, Term: it.Term, Teacher: teacher})
			}
		}
	}
	items, e := s.Extractor.Extract(ctx, input)
	if e != nil {
		return s.retryOrFail(&job, "extracting", "ingestion.ai_unavailable", "暂时无法抽取作业候选："+truncateOneLine(e.Error(), 300))
	}
	if e := s.setStage(ctx, &job, "validating", started, len(pages)); e != nil {
		return e
	}
	if len(items) == 0 || (!usesSource && validate(items, pages) != nil) {
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
			var courseID *uuid.UUID
			if id, e := uuid.Parse(item.CourseInstanceID); e == nil {
				courseID = &id
			}
			c := shared.AssignmentCandidate{SourceItemID: src.ID, Ordinal: i + 1, Title: item.Title, Description: item.Description, OfficialDueAt: item.OfficialDueAt, RawDueExpression: &raw, SubmissionMethod: &method, CourseInstanceID: courseID, Evidence: string(ev), FieldConfidence: "{}", UncertainFields: string(unc), ReviewStatus: "pending"}
			if e := tx.Create(&c).Error; e != nil {
				return e
			}
		}
		done := time.Now()
		if e := tx.Model(&job).Updates(map[string]any{"stage": "review_ready", "status": "review_ready", "finished_at": &done, "error_code": nil, "error_message": nil, "temporary_locator": nil}).Error; e != nil {
			return e
		}
		return tx.Model(&src).Update("status", "review_ready").Error
	})
	if e != nil {
		return e
	}
	if src.SourceType == "file" {
		if e := s.Files.RemoveJob(job.ID); e != nil {
			slog.Error("ingestion_cleanup_error", "job_id", job.ID, "error_type", fmt.Sprintf("%T", e))
		}
	}
	slog.Info("ingestion_review_ready", "job_id", job.ID, "source_id", src.ID,
		"attempt", job.Attempts, "candidates", len(items), "duration_ms", time.Since(started).Milliseconds())
	return nil
}

func (s Service) setStage(ctx context.Context, job *shared.IngestionJob, stage string, started time.Time, pages int) error {
	if e := s.DB.WithContext(ctx).Model(job).Update("stage", stage).Error; e != nil {
		return e
	}
	job.Stage = stage
	slog.Info("ingestion_stage", "job_id", job.ID, "source_id", job.SourceItemID,
		"stage", stage, "attempt", job.Attempts, "pages", pages, "elapsed_ms", time.Since(started).Milliseconds())
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

func sourceChecksum(fileChecksum, message string) string {
	if message == "" {
		return fileChecksum
	}
	combined := sha256.Sum256([]byte("file-message-v1\x00" + fileChecksum + "\x00" + message))
	return hex.EncodeToString(combined[:])
}

func withTeacherMessage(src shared.SourceItem, pages []Page) []Page {
	if src.SourceType != "file" || src.RawText == nil {
		return pages
	}
	return append([]Page{{Number: 0, Text: *src.RawText}}, pages...)
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
	slog.Error("ingestion_failed", "job_id", j.ID, "source_id", j.SourceItemID,
		"stage", j.Stage, "attempt", j.Attempts, "code", code)
	// 内存里的 job 也要同步，创建接口在投递失败时会把它作为响应体返回。
	j.Stage, j.Status, j.ErrorCode, j.ErrorMessage, j.FinishedAt, j.TemporaryLocator = "failed", "failed", &code, &msg, &now, nil
	if e := s.DB.Transaction(func(tx *gorm.DB) error {
		if e := tx.Model(j).Updates(map[string]any{"stage": "failed", "status": "failed", "error_code": code, "error_message": msg, "finished_at": &now, "temporary_locator": nil}).Error; e != nil {
			return e
		}
		return tx.Model(&shared.SourceItem{}).Where("id=?", j.SourceItemID).Update("status", "failed").Error
	}); e != nil {
		return e
	}
	if e := s.Files.RemoveJob(j.ID); e != nil {
		slog.Error("ingestion_cleanup_error", "job_id", j.ID, "error_type", fmt.Sprintf("%T", e))
	}
	return nil
}
func (s Service) retryOrFail(j *shared.IngestionJob, stage, code, msg string) error {
	// j.Attempts 是含本次在内的执行次数，Process 里已经记过账，这里不要再加一。
	if j.Attempts >= j.MaxAttempts {
		return s.fail(j, code, msg)
	}
	slog.Warn("ingestion_retry", "job_id", j.ID, "source_id", j.SourceItemID,
		"stage", stage, "attempt", j.Attempts, "max_attempts", j.MaxAttempts, "code", code)
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
		if len(i.Evidence) == 0 {
			return fmt.Errorf("missing evidence")
		}
		for _, e := range i.Evidence {
			if strings.TrimSpace(e.TextExcerpt) == "" {
				return fmt.Errorf("empty evidence")
			}
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
