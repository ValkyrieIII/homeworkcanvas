package ingestion

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"homeworkcanvas/backend/internal/shared"
)

func (s Service) Handler() asynq.HandlerFunc {
	return func(ctx context.Context, t *asynq.Task) error {
		var p struct {
			JobID string `json:"job_id"`
		}
		if e := json.Unmarshal(t.Payload(), &p); e != nil {
			return e
		}
		id, e := uuid.Parse(p.JobID)
		if e != nil {
			return e
		}
		if e := s.Process(ctx, id); e != nil {
			return s.settleIfExhausted(ctx, id, e)
		}
		return nil
	}
}

// Process 里没归类的错误（数据库故障等）不走 fail()，直接交给 asynq 重试的话，
// 重试耗尽后任务会永远停在 processing、临时目录也永远留着。这里是这些错误的最后出口。
func (s Service) settleIfExhausted(ctx context.Context, id uuid.UUID, cause error) error {
	var job shared.IngestionJob
	// 收尾不能用 asynq 的 ctx：超时或停机时它已经被取消，落库会失败。
	ctx = context.WithoutCancel(ctx)
	if e := s.DB.WithContext(ctx).First(&job, "id=?", id).Error; e != nil {
		return cause
	}
	if job.Status == "failed" || job.Attempts < job.MaxAttempts {
		return cause
	}
	if e := s.fail(&job, "ingestion.internal_error", "任务处理失败："+truncateOneLine(cause.Error(), 300)); e != nil {
		return e
	}
	return nil
}
