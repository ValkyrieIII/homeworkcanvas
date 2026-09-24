package assignments

import (
	"context"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"homeworkcanvas/backend/internal/shared"
	"time"
)

func (s Service) RejectCandidate(ctx context.Context, u, id uuid.UUID, note *string) error {
	var c shared.AssignmentCandidate
	if e := s.DB.WithContext(ctx).First(&c, "id=?", id).Error; e != nil {
		return shared.JSONError("assignments.not_found", "候选不存在", 404)
	}
	if c.ReviewStatus != "pending" {
		return shared.JSONError("assignments.candidate_not_pending", "候选已被审核", 409)
	}
	var src shared.SourceItem
	if e := s.DB.First(&src, "id=?", c.SourceItemID).Error; e != nil {
		return e
	}
	if e := s.Membership.RequireAdmin(ctx, u); e != nil {
		return e
	}
	now := time.Now()
	return s.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&c).Updates(map[string]any{"review_status": "rejected", "reviewed_by": u, "reviewed_at": now, "review_note": note}).Error; err != nil {
			return err
		}
		return completeJobIfReviewed(tx, c.SourceItemID, now)
	})
}
