-- +goose Up
-- agent 投递是第三种来源类型（前两种：用户上传的文件、粘贴的文字）。
ALTER TABLE source_items DROP CONSTRAINT source_items_source_type_check;
ALTER TABLE source_items ADD CONSTRAINT source_items_source_type_check CHECK (source_type IN ('text', 'file', 'agent'));

-- +goose Down
ALTER TABLE source_items DROP CONSTRAINT source_items_source_type_check;
ALTER TABLE source_items ADD CONSTRAINT source_items_source_type_check CHECK (source_type IN ('text', 'file'));
