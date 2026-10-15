-- +goose Up
-- 学期是「YYYY春/秋季学期」这样的格式化值，不需要预先定义，所以只用 CHECK 管格式。
-- 挡住 2026-fall 这类写法，否则课程列表的 ORDER BY term DESC 排序会失去意义。
ALTER TABLE course_instances
  ADD CONSTRAINT course_instances_term_format_check
  CHECK (term ~ '^[0-9]{4}(春|秋)季学期$');

-- +goose Down
ALTER TABLE course_instances DROP CONSTRAINT course_instances_term_format_check;
