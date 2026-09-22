-- +goose Up
CREATE TABLE notifications (
  id uuid PRIMARY KEY,
  user_id uuid NOT NULL REFERENCES users(id),
  kind text NOT NULL CHECK(kind IN ('assignment_published')),
  payload jsonb NOT NULL DEFAULT '{}',
  read_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_notifications_user_read_created ON notifications(user_id, read_at, created_at DESC);

-- +goose Down
DROP TABLE IF EXISTS notifications;
