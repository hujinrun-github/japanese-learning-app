-- Keep source sentences, annotations and learning history when a dialogue moves
-- to speaking practice. Only reading lists exclude linked lessons.
ALTER TABLE lessons
    ADD COLUMN speaking_material_id BIGINT REFERENCES speaking_materials(id) ON DELETE SET NULL;

CREATE INDEX idx_lessons_speaking_material_id ON lessons(speaking_material_id);
