package main

import (
	"fmt"
	"github.com/hibiken/asynq"
	"homeworkcanvas/backend/internal/courses"
	"homeworkcanvas/backend/internal/ingestion"
	"homeworkcanvas/backend/internal/shared"
	"log/slog"
	"time"
)

func main() {
	_ = shared.LoadDotEnv(".env")
	shared.ConfigureLogging()
	db, e := shared.OpenDatabase()
	if e != nil {
		panic(e)
	}
	cs := courses.Service{DB: db}
	svc := ingestion.Service{DB: db, Membership: cs, Extractor: ingestion.NewConfiguredExtractor(), Files: shared.NewTemporaryFiles()}
	go sweep(svc)
	srv := asynq.NewServer(shared.RedisOpt(), asynq.Config{Concurrency: 2})
	mux := asynq.NewServeMux()
	mux.Handle(shared.IngestionTask, svc.Handler())
	if e := srv.Run(mux); e != nil {
		panic(e)
	}
}

// sweep 先扫一遍再按小时兜底，覆盖 worker 停机期间留下的临时目录。
func sweep(svc ingestion.Service) {
	for {
		if e := svc.SweepStale(time.Now(), 24*time.Hour); e != nil {
			slog.Error("ingestion_sweep_error", "error_type", fmt.Sprintf("%T", e))
		}
		time.Sleep(time.Hour)
	}
}
