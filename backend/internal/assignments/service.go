package assignments

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"homeworkcanvas/backend/internal/courses"
	"homeworkcanvas/backend/internal/shared"
	"strings"
	"time"
)

type Service struct {
	DB                  *gorm.DB
	Membership          courses.Service
	PublishNotification func(context.Context, *gorm.DB, shared.Assignment) error
	UpdateNotification  func(context.Context, *gorm.DB, shared.Assignment) error
	RetractNotification func(context.Context, *gorm.DB, shared.Assignment) error
}
type CreateInput struct {
	Title            string     `json:"title"`
	Description      string     `json:"description"`
	OfficialDueAt    *time.Time `json:"official_due_at"`
	SubmissionMethod *string    `json:"submission_method"`
	Edited           bool       `json:"edited,omitempty"`
}

func (s Service) CreateManual(ctx context.Context, u, ci uuid.UUID, in CreateInput) (shared.Assignment, error) {
	if e := s.Membership.RequireAdmin(ctx, u); e != nil {
		return shared.Assignment{}, e
	}
	if in.Title == "" {
		return shared.Assignment{}, shared.JSONError("assignments.invalid_input", "作业标题不能为空", 400)
	}
	a := shared.Assignment{CourseInstanceID: ci, Title: in.Title, Description: in.Description, OfficialDueAt: in.OfficialDueAt, SubmissionMethod: in.SubmissionMethod, Status: "published", CreatedBy: u}
	e := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := tx.Create(&a).Error; e != nil {
			return e
		}
		if s.PublishNotification != nil {
			return s.PublishNotification(ctx, tx, a)
		}
		return nil
	})
	return a, e
}

// UpdatePublished 修改已发布作业的共享字段，仅允许管理员执行。
func (s Service) UpdatePublished(ctx context.Context, u, aid uuid.UUID, in CreateInput) (shared.Assignment, error) {
	if err := s.Membership.RequireAdmin(ctx, u); err != nil {
		return shared.Assignment{}, err
	}
	if strings.TrimSpace(in.Title) == "" {
		return shared.Assignment{}, shared.JSONError("assignments.invalid_input", "作业标题不能为空", 400)
	}
	var assignment shared.Assignment
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&assignment, "id=?", aid).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return shared.JSONError("assignments.not_found", "作业不存在", 404)
			}
			return err
		}
		if assignment.Status != "published" {
			return shared.JSONError("assignments.not_published", "已撤回的作业不能编辑", 409)
		}
		changed := assignment.Title != strings.TrimSpace(in.Title) ||
			assignment.Description != in.Description ||
			!sameOptionalTime(assignment.OfficialDueAt, in.OfficialDueAt) ||
			!sameOptionalString(assignment.SubmissionMethod, in.SubmissionMethod)
		if !changed {
			return nil
		}
		assignment.Title = strings.TrimSpace(in.Title)
		assignment.Description = in.Description
		assignment.OfficialDueAt = in.OfficialDueAt
		assignment.SubmissionMethod = in.SubmissionMethod
		if err := tx.Save(&assignment).Error; err != nil {
			return err
		}
		if s.UpdateNotification != nil {
			return s.UpdateNotification(ctx, tx, assignment)
		}
		return nil
	})
	return assignment, err
}

// 撤回保留共享作业记录与来源关系，用户查询只读取 published 状态。
func (s Service) Retract(ctx context.Context, u, aid uuid.UUID) (shared.Assignment, error) {
	if err := s.Membership.RequireAdmin(ctx, u); err != nil {
		return shared.Assignment{}, err
	}
	var assignment shared.Assignment
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&assignment, "id=?", aid).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return shared.JSONError("assignments.not_found", "作业不存在", 404)
			}
			return err
		}
		if assignment.Status != "published" {
			return shared.JSONError("assignments.not_published", "作业已经撤回", 409)
		}
		assignment.Status = "cancelled"
		if err := tx.Save(&assignment).Error; err != nil {
			return err
		}
		if s.RetractNotification != nil {
			return s.RetractNotification(ctx, tx, assignment)
		}
		return nil
	})
	return assignment, err
}

func sameOptionalTime(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Equal(*b)
}

func sameOptionalString(a, b *string) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

type MineItem struct {
	ID               uuid.UUID  `json:"id"`
	CourseInstanceID uuid.UUID  `json:"course_instance_id"`
	CourseName       string     `json:"course_name"`
	Term             string     `json:"term"`
	Title            string     `json:"title"`
	Description      string     `json:"description"`
	OfficialDueAt    *time.Time `json:"official_due_at"`
	SubmissionMethod *string    `json:"submission_method"`
	PersonalDueAt    *time.Time `json:"personal_due_at"`
	EffectiveDueAt   *time.Time `json:"effective_due_at"`
	CompletionStatus string     `json:"completion_status"`
	Status           string     `json:"status"`
}

func (s Service) ListMine(ctx context.Context, u uuid.UUID, bucket string, courseID *uuid.UUID) ([]MineItem, error) {
	q := s.DB.WithContext(ctx).Table("assignments a").Select(`
		a.id, a.course_instance_id, c.name course_name, ci.term,
		a.title, a.description, a.official_due_at, a.submission_method,
		aus.personal_due_at,
		COALESCE(aus.personal_due_at, a.official_due_at) effective_due_at,
		COALESCE(aus.completion_status, 'todo') completion_status,
		a.status
	`)
	q = q.Joins("JOIN course_instances ci ON ci.id=a.course_instance_id")
	q = q.Joins("JOIN courses c ON c.id=ci.course_id")
	q = q.Joins("JOIN enrollments e ON e.course_instance_id=ci.id")
	q = q.Joins("LEFT JOIN assignment_user_states aus ON aus.assignment_id=a.id AND aus.user_id=?", u)
	q = q.Where("e.user_id=? AND e.status='active' AND ci.status='active' AND a.status='published'", u)
	if courseID != nil {
		q = q.Where("a.course_instance_id=?", *courseID)
	}
	switch bucket {
	case "overdue":
		q = q.Where("a.official_due_at < NOW()")
	case "upcoming":
		q = q.Where("a.official_due_at IS NOT NULL AND a.official_due_at >= NOW()")
	case "undated":
		q = q.Where("a.official_due_at IS NULL")
	}
	var r []MineItem
	e := q.Order("COALESCE(aus.personal_due_at,a.official_due_at) ASC NULLS LAST,a.id ASC").Scan(&r).Error
	return r, e
}

type CourseItem struct {
	shared.Assignment
	CourseName string `json:"course_name"`
	Term       string `json:"term"`
}

type AdminItem struct {
	shared.Assignment
	CourseName       string  `json:"course_name"`
	Term             string  `json:"term"`
	SourceType       *string `json:"source_type"`
	OriginalFilename *string `json:"original_filename"`
	Evidence         *string `json:"evidence"`
}

// ListAdmin 按官方截止时间查询全站共享作业；游标保持同一排序。
func (s Service) ListAdmin(ctx context.Context, actor uuid.UUID, status string, courseID *uuid.UUID, limit int, rawCursor string) ([]AdminItem, *string, error) {
	if err := s.Membership.RequireAdmin(ctx, actor); err != nil {
		return nil, nil, err
	}
	if status != "published" && status != "cancelled" {
		return nil, nil, shared.JSONError("assignments.invalid_status", "作业状态不合法", 400)
	}
	q := s.DB.WithContext(ctx).Table("assignments a").
		Select("a.*,c.name AS course_name,ci.term,si.source_type,si.original_filename,ac.evidence").
		Joins("JOIN course_instances ci ON ci.id=a.course_instance_id").
		Joins("JOIN courses c ON c.id=ci.course_id").
		Joins("LEFT JOIN assignment_candidates ac ON ac.id=a.source_candidate_id").
		Joins("LEFT JOIN source_items si ON si.id=ac.source_item_id").
		Where("a.status=?", status)
	if courseID != nil {
		q = q.Where("a.course_instance_id=?", *courseID)
	}
	if rawCursor != "" {
		cursor, err := shared.DecodePageCursor(rawCursor)
		if err != nil {
			return nil, nil, err
		}
		if cursor.Time == nil {
			q = q.Where("a.official_due_at IS NULL AND a.id > ?", cursor.ID)
		} else {
			q = q.Where("a.official_due_at > ? OR (a.official_due_at = ? AND a.id > ?) OR a.official_due_at IS NULL", *cursor.Time, *cursor.Time, cursor.ID)
		}
	}
	rows := make([]AdminItem, 0)
	err := q.Order("a.official_due_at ASC NULLS LAST,a.id ASC").Limit(limit + 1).Scan(&rows).Error
	if err != nil {
		return nil, nil, err
	}
	var next *string
	if len(rows) > limit {
		rows = rows[:limit]
		last := rows[len(rows)-1]
		value := shared.EncodePageCursor(shared.PageCursor{Time: last.OfficialDueAt, ID: last.ID})
		next = &value
	}
	return rows, next, nil
}

func (s Service) ListCourse(ctx context.Context, actor, ci uuid.UUID) ([]CourseItem, error) {
	if err := s.Membership.RequireAdmin(ctx, actor); err != nil {
		return nil, err
	}
	var rows []CourseItem
	err := s.DB.WithContext(ctx).Table("assignments a").
		Select("a.*,c.name AS course_name,ci.term").
		Joins("JOIN course_instances ci ON ci.id=a.course_instance_id").
		Joins("JOIN courses c ON c.id=ci.course_id").
		Where("a.course_instance_id=?", ci).
		Order("a.official_due_at ASC NULLS LAST,a.id ASC").Scan(&rows).Error
	return rows, err
}

type PendingItem struct {
	shared.AssignmentCandidate
	CourseInstanceID uuid.UUID `json:"course_instance_id"`
	CourseName       string    `json:"course_name"`
	Term             string    `json:"term"`
	SourceType       string    `json:"source_type"`
	OriginalFilename *string   `json:"original_filename"`
}

func (s Service) ListPending(ctx context.Context, actor uuid.UUID, courseID *uuid.UUID, limit int, rawCursor string) ([]PendingItem, *string, error) {
	if err := s.Membership.RequireAdmin(ctx, actor); err != nil {
		return nil, nil, err
	}
	rows := make([]PendingItem, 0)
	q := s.DB.WithContext(ctx).Table("assignment_candidates ac").
		Select("ac.*,si.course_instance_id,si.source_type,si.original_filename,c.name AS course_name,ci.term").
		Joins("JOIN source_items si ON si.id=ac.source_item_id").
		Joins("JOIN course_instances ci ON ci.id=si.course_instance_id").
		Joins("JOIN courses c ON c.id=ci.course_id").
		Where("ac.review_status='pending'")
	if courseID != nil {
		q = q.Where("si.course_instance_id=?", *courseID)
	}
	if rawCursor != "" {
		cursor, err := shared.DecodePageCursor(rawCursor)
		if err != nil || cursor.Time == nil {
			return nil, nil, shared.JSONError("shared.invalid_cursor", "分页位置无效", 400)
		}
		q = q.Where("ac.created_at > ? OR (ac.created_at = ? AND ac.id > ?)", *cursor.Time, *cursor.Time, cursor.ID)
	}
	err := q.Order("ac.created_at ASC,ac.id ASC").Limit(limit + 1).Scan(&rows).Error
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

type StateInput struct {
	PersonalDueAt    *time.Time `json:"personal_due_at"`
	CompletionStatus string     `json:"completion_status"`
}

func (s Service) UpdateMyState(ctx context.Context, u, aid uuid.UUID, in StateInput) (shared.AssignmentUserState, error) {
	var a shared.Assignment
	if e := s.DB.WithContext(ctx).First(&a, "id=?", aid).Error; e != nil {
		return shared.AssignmentUserState{}, shared.JSONError("assignments.not_found", "作业不存在", 404)
	}
	if a.Status != "published" {
		return shared.AssignmentUserState{}, shared.JSONError("assignments.not_published", "作业已撤回", 409)
	}
	if e := s.Membership.RequireActiveMember(ctx, u, a.CourseInstanceID); e != nil {
		return shared.AssignmentUserState{}, e
	}
	if in.CompletionStatus == "" {
		in.CompletionStatus = "todo"
	}
	if in.CompletionStatus != "todo" && in.CompletionStatus != "completed" {
		return shared.AssignmentUserState{}, shared.JSONError("assignments.invalid_state", "完成状态不合法", 400)
	}
	var state shared.AssignmentUserState
	now := time.Now()
	e := s.DB.WithContext(ctx).Where("assignment_id=? AND user_id=?", aid, u).First(&state).Error
	if e == gorm.ErrRecordNotFound {
		state = shared.AssignmentUserState{AssignmentID: aid, UserID: u}
	} else if e != nil {
		return state, e
	}
	state.PersonalDueAt = in.PersonalDueAt
	state.CompletionStatus = in.CompletionStatus
	if in.CompletionStatus == "completed" {
		state.CompletedAt = &now
	} else {
		state.CompletedAt = nil
	}
	return state, s.DB.WithContext(ctx).Save(&state).Error
}
func (s Service) AcceptCandidate(ctx context.Context, u, cid uuid.UUID, in CreateInput) (shared.Assignment, error) {
	var out shared.Assignment
	e := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var c shared.AssignmentCandidate
		if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&c, "id=?", cid).Error; e != nil {
			return shared.JSONError("assignments.not_found", "候选不存在", 404)
		}
		if c.ReviewStatus != "pending" {
			return shared.JSONError("assignments.candidate_not_pending", "候选已被审核", 409)
		}
		var src shared.SourceItem
		if e := tx.First(&src, "id=?", c.SourceItemID).Error; e != nil {
			return e
		}
		if e := s.Membership.RequireAdmin(ctx, u); e != nil {
			return e
		}
		if in.Edited {
			if strings.TrimSpace(in.Title) == "" {
				return shared.JSONError("assignments.invalid_input", "作业标题不能为空", 400)
			}
			in.Title = strings.TrimSpace(in.Title)
			if e := tx.Model(&c).Updates(map[string]any{"title": in.Title, "description": in.Description, "official_due_at": in.OfficialDueAt, "submission_method": in.SubmissionMethod}).Error; e != nil {
				return e
			}
		} else {
			if in.Title == "" {
				in.Title = c.Title
			}
			if in.Description == "" {
				in.Description = c.Description
			}
			if in.OfficialDueAt == nil {
				in.OfficialDueAt = c.OfficialDueAt
			}
			if in.SubmissionMethod == nil {
				in.SubmissionMethod = c.SubmissionMethod
			}
		}
		out = shared.Assignment{CourseInstanceID: src.CourseInstanceID, Title: in.Title, Description: in.Description, OfficialDueAt: in.OfficialDueAt, SubmissionMethod: in.SubmissionMethod, Status: "published", SourceCandidateID: &c.ID, CreatedBy: u}
		if e := tx.Create(&out).Error; e != nil {
			return e
		}
		if s.PublishNotification != nil {
			if e := s.PublishNotification(ctx, tx, out); e != nil {
				return e
			}
		}
		now := time.Now()
		if err := tx.Model(&c).Updates(map[string]any{"review_status": "accepted", "reviewed_by": u, "reviewed_at": now}).Error; err != nil {
			return err
		}
		return completeJobIfReviewed(tx, c.SourceItemID, now)
	})
	return out, e
}

func completeJobIfReviewed(tx *gorm.DB, sourceID uuid.UUID, now time.Time) error {
	var pending int64
	if err := tx.Model(&shared.AssignmentCandidate{}).Where("source_item_id=? AND review_status='pending'", sourceID).Count(&pending).Error; err != nil {
		return err
	}
	if pending != 0 {
		return nil
	}
	if err := tx.Model(&shared.IngestionJob{}).Where("source_item_id=? AND status='review_ready'", sourceID).Updates(map[string]any{"status": "completed", "stage": "completed", "finished_at": &now}).Error; err != nil {
		return err
	}
	return tx.Model(&shared.SourceItem{}).Where("id=?", sourceID).Update("status", "completed").Error
}
