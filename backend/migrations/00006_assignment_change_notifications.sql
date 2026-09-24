-- +goose Up
ALTER TABLE notifications DROP CONSTRAINT notifications_kind_check;
ALTER TABLE notifications ADD CONSTRAINT notifications_kind_check
  CHECK (kind IN ('assignment_published', 'assignment_updated', 'assignment_retracted'));

-- +goose Down
DELETE FROM notifications WHERE kind IN ('assignment_updated', 'assignment_retracted');
ALTER TABLE notifications DROP CONSTRAINT notifications_kind_check;
ALTER TABLE notifications ADD CONSTRAINT notifications_kind_check
  CHECK (kind IN ('assignment_published'));
