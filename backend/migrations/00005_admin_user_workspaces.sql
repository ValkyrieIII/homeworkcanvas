-- +goose Up
-- 旧维护者只保留选课关系；管理员必须经明确的管理流程授予。
ALTER TABLE users ADD COLUMN role text NOT NULL DEFAULT 'user' CHECK (role IN ('admin', 'user'));
ALTER TABLE enrollments DROP COLUMN role;

-- +goose Down
ALTER TABLE enrollments ADD COLUMN role text NOT NULL DEFAULT 'student' CHECK (role IN ('maintainer', 'student'));
ALTER TABLE users DROP COLUMN role;
