-- +goose Up
-- 统一入口不再要求先选课：来源可以不带课程，课程由 agent 判在候选上。
ALTER TABLE source_items ALTER COLUMN course_instance_id DROP NOT NULL;
ALTER TABLE assignment_candidates ADD COLUMN course_instance_id uuid REFERENCES course_instances(id);

-- +goose Down
ALTER TABLE assignment_candidates DROP COLUMN course_instance_id;
UPDATE source_items
SET course_instance_id = (SELECT id FROM course_instances LIMIT 1)
WHERE course_instance_id IS NULL;
ALTER TABLE source_items ALTER COLUMN course_instance_id SET NOT NULL;
