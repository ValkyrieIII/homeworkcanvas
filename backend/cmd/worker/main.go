package main

import (
	"github.com/hibiken/asynq"
	"homeworkcanvas/backend/internal/courses"
	"homeworkcanvas/backend/internal/ingestion"
	"homeworkcanvas/backend/internal/shared"
)

func main() {
	db, e := shared.OpenDatabase()
	if e != nil {
		panic(e)
	}
	cs := courses.Service{DB: db}
	svc := ingestion.Service{DB: db, Membership: cs, Extractor: ingestion.NewConfiguredExtractor(), Files: shared.NewTemporaryFiles()}
	srv := asynq.NewServer(shared.RedisOpt(), asynq.Config{Concurrency: 2})
	mux := asynq.NewServeMux()
	mux.Handle(shared.IngestionTask, svc.Handler())
	if e := srv.Run(mux); e != nil {
		panic(e)
	}
}
