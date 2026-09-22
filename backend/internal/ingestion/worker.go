package ingestion

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
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
		return s.Process(ctx, id)
	}
}
