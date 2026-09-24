package notifications

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"homeworkcanvas/backend/internal/shared"
)

type Service struct{ DB *gorm.DB }
type Item struct {
	ID        uuid.UUID       `json:"id"`
	Kind      string          `json:"kind"`
	Payload   json.RawMessage `json:"payload"`
	ReadAt    *time.Time      `json:"read_at"`
	CreatedAt time.Time       `json:"created_at"`
}

// 作业通知与共享内容变更处于同一事务，避免出现内容未变更但通知已发出的情况。
func (s Service) AssignmentPublished(ctx context.Context, tx *gorm.DB, assignment shared.Assignment) error {
	return s.assignmentEvent(ctx, tx, assignment, "assignment_published")
}

func (s Service) AssignmentUpdated(ctx context.Context, tx *gorm.DB, assignment shared.Assignment) error {
	return s.assignmentEvent(ctx, tx, assignment, "assignment_updated")
}

func (s Service) AssignmentRetracted(ctx context.Context, tx *gorm.DB, assignment shared.Assignment) error {
	return s.assignmentEvent(ctx, tx, assignment, "assignment_retracted")
}

func (s Service) assignmentEvent(ctx context.Context, tx *gorm.DB, assignment shared.Assignment, kind string) error {
	var userIDs []uuid.UUID
	if err := tx.WithContext(ctx).Model(&shared.Enrollment{}).Where("course_instance_id=? AND status='active'", assignment.CourseInstanceID).Pluck("user_id", &userIDs).Error; err != nil {
		return err
	}
	payload, err := json.Marshal(map[string]any{"assignment_id": assignment.ID, "course_instance_id": assignment.CourseInstanceID, "title": assignment.Title, "official_due_at": assignment.OfficialDueAt})
	if err != nil {
		return err
	}
	for _, userID := range userIDs {
		n := shared.Notification{UserID: userID, Kind: kind, Payload: string(payload)}
		if err := tx.WithContext(ctx).Create(&n).Error; err != nil {
			return err
		}
	}
	return nil
}
func (s Service) ListMine(ctx context.Context, userID uuid.UUID) ([]Item, error) {
	var rows []Item
	err := s.DB.WithContext(ctx).Table("notifications").Select("id,kind,payload,read_at,created_at").Where("user_id=?", userID).Order("created_at DESC,id DESC").Limit(100).Scan(&rows).Error
	return rows, err
}
func (s Service) MarkRead(ctx context.Context, userID, id uuid.UUID) error {
	now := time.Now()
	result := s.DB.WithContext(ctx).Model(&shared.Notification{}).Where("id=? AND user_id=?", id, userID).Where("read_at IS NULL").Update("read_at", now)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return shared.JSONError("notifications.not_found", "通知不存在", 404)
	}
	return nil
}
