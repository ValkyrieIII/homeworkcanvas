-- +goose Up
ALTER TABLE source_items
  ADD COLUMN original_filename text,
  ADD COLUMN mime_type text,
  ADD COLUMN byte_size bigint;
ALTER TABLE ingestion_jobs ADD COLUMN temporary_locator text;
CREATE INDEX idx_source_items_course_created ON source_items(course_instance_id, created_at DESC);
CREATE INDEX idx_ingestion_jobs_status_created ON ingestion_jobs(status, created_at);

-- +goose Down
DROP INDEX IF EXISTS idx_ingestion_jobs_status_created;
DROP INDEX IF EXISTS idx_source_items_course_created;
ALTER TABLE ingestion_jobs DROP COLUMN temporary_locator;
ALTER TABLE source_items DROP COLUMN byte_size, DROP COLUMN mime_type, DROP COLUMN original_filename;
