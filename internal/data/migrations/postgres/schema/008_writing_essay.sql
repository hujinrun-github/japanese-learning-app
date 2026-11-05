ALTER TABLE writing_records
    DROP CONSTRAINT writing_records_type_check,
    ADD CONSTRAINT writing_records_type_check CHECK (type IN ('input', 'sentence', 'essay'));
