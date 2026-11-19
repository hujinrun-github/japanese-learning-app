ALTER TABLE writing_records
    ADD COLUMN topic_id TEXT REFERENCES writing_topics(id) ON DELETE SET NULL;

-- Older topic submissions stored the title and the mode's prompt as a snapshot.
-- Match the full snapshot and mode so unrelated legacy exercises stay unlinked.
UPDATE writing_records AS record
SET topic_id = topic.id
FROM writing_topics AS topic
WHERE record.topic_id IS NULL
  AND (
      (record.type = 'essay' AND record.question = topic.title || E'\n' || topic.essay_prompt)
      OR (record.type = 'sentence' AND record.question = topic.title || E'\n' || topic.sentence_prompt)
  );

CREATE INDEX writing_records_topic_progress_idx
    ON writing_records (user_id, topic_id)
    WHERE topic_id IS NOT NULL;
