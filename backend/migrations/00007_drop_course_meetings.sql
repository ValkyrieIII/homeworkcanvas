-- +goose Up
-- 课表功能整体移除（2026-10-16 决定）：数据模型、接口与前端一并删除。
DROP TABLE IF EXISTS course_meetings;

-- +goose Down
CREATE TABLE course_meetings (id uuid PRIMARY KEY, course_instance_id uuid NOT NULL REFERENCES course_instances(id), weekday smallint NOT NULL CHECK(weekday BETWEEN 1 AND 7), start_time time NOT NULL, end_time time NOT NULL CHECK(start_time < end_time), location text, valid_from date, valid_to date, created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(), CHECK(valid_from IS NULL OR valid_to IS NULL OR valid_from <= valid_to));
CREATE INDEX idx_meetings_course_weekday ON course_meetings(course_instance_id,weekday);
