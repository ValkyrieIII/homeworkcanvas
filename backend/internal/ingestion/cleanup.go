package ingestion

import (
	"log/slog"
	"os"
	"time"

	"github.com/google/uuid"
	"homeworkcanvas/backend/internal/shared"
)

// SweepStale 兜底清理临时目录。正常路径上任务一进终态就删目录，但 worker 被杀、
// asynq 丢弃任务、进程崩溃都会留下上传原件，与「任务终态即清理」的承诺冲突。
func (s Service) SweepStale(now time.Time, retention time.Duration) error {
	entries, e := os.ReadDir(s.Files.Root)
	if e != nil {
		if os.IsNotExist(e) {
			return nil
		}
		return e
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		id, e := uuid.Parse(entry.Name())
		if e != nil {
			continue
		}
		var job shared.IngestionJob
		var gone bool
		// 查不到记录时不立刻删：目录先于任务记录存在（上传写入中），那时删就会吞掉正在传的文件。
		if e := s.DB.First(&job, "id=?", id).Error; e == nil &&
			(job.Status == "failed" || job.Status == "review_ready" || job.Status == "completed") {
			gone = true
		} else if info, e := entry.Info(); e == nil && now.Sub(info.ModTime()) >= retention {
			gone = true
		}
		if !gone {
			continue
		}
		slog.Info("ingestion_sweep_remove", "job_id", id)
		if e := s.Files.RemoveJob(id); e != nil {
			return e
		}
	}
	return nil
}
