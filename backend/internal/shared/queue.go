package shared

import (
	"encoding/json"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"os"
)

const IngestionTask = "ingestion:process"

func RedisOpt() asynq.RedisClientOpt {
	return asynq.RedisClientOpt{Addr: env("REDIS_ADDR", "127.0.0.1:6379"), Password: os.Getenv("REDIS_PASSWORD"), DB: 0}
}
func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
func NewJobTask(id uuid.UUID) (*asynq.Task, error) {
	b, e := json.Marshal(map[string]string{"job_id": id.String()})
	return asynq.NewTask(IngestionTask, b), e
}
