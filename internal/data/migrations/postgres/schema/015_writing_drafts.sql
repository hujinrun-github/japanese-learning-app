CREATE TABLE writing_drafts (
    user_id     BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    topic_id    TEXT NOT NULL,
    type        TEXT NOT NULL CHECK (type IN ('essay', 'sentence')),
    user_answer TEXT NOT NULL CHECK (char_length(user_answer) <= 10000),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, topic_id, type)
);

CREATE INDEX writing_drafts_resume_idx ON writing_drafts (user_id, updated_at DESC);
