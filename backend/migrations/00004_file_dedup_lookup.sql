-- +goose Up
CREATE INDEX idx_source_items_file_dedup_lookup
  ON source_items(course_instance_id, submitted_by, checksum, created_at, id)
  WHERE source_type = 'file' AND status <> 'failed';

-- +goose Down
DROP INDEX IF EXISTS idx_source_items_file_dedup_lookup;
