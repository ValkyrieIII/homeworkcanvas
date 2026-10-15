-- +goose Up
-- 学期改成结构化两列：年份 + 季节。字符串拼接只是展示层的产物，不再入库。
ALTER TABLE course_instances ADD COLUMN term_year smallint;
ALTER TABLE course_instances ADD COLUMN term_season text;

-- 从旧值回填（形如 2026秋季学期）
UPDATE course_instances
SET term_year = regexp_replace(term, '^([0-9]{4})(春|秋)季学期$', '\1')::smallint,
    term_season = regexp_replace(term, '^([0-9]{4})(春|秋)季学期$', '\2')
WHERE term ~ '^[0-9]{4}(春|秋)季学期$';

ALTER TABLE course_instances ALTER COLUMN term_year SET NOT NULL;
ALTER TABLE course_instances ALTER COLUMN term_season SET NOT NULL;
ALTER TABLE course_instances ADD CONSTRAINT course_instances_term_year_check CHECK (term_year BETWEEN 2000 AND 2100);
ALTER TABLE course_instances ADD CONSTRAINT course_instances_term_season_check CHECK (term_season IN ('春', '秋'));
ALTER TABLE course_instances DROP CONSTRAINT IF EXISTS course_instances_term_format_check;
ALTER TABLE course_instances DROP COLUMN term;

-- +goose Down
ALTER TABLE course_instances ADD COLUMN term text;
UPDATE course_instances SET term = term_year::text || term_season || '季学期';
ALTER TABLE course_instances ALTER COLUMN term SET NOT NULL;
ALTER TABLE course_instances DROP CONSTRAINT course_instances_term_year_check;
ALTER TABLE course_instances DROP CONSTRAINT course_instances_term_season_check;
ALTER TABLE course_instances DROP COLUMN term_year;
ALTER TABLE course_instances DROP COLUMN term_season;
