ALTER TABLE speaking_materials
    ADD COLUMN IF NOT EXISTS audio_url_legacy TEXT NOT NULL DEFAULT '';

ALTER TABLE speaking_materials
    DROP CONSTRAINT IF EXISTS speaking_materials_type_check;

ALTER TABLE speaking_materials
    ADD CONSTRAINT speaking_materials_type_check
    CHECK (type IN ('shadow', 'free', 'read_aloud', 'picture_description', 'free_talk', 'question_answer'));

ALTER TABLE speaking_records
    DROP CONSTRAINT IF EXISTS speaking_records_type_check;

ALTER TABLE speaking_records
    ADD CONSTRAINT speaking_records_type_check
    CHECK (type IN ('shadow', 'free', 'read_aloud', 'picture_description', 'free_talk', 'question_answer'));
