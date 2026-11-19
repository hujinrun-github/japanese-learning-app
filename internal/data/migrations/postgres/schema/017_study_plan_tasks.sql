CREATE TABLE IF NOT EXISTS study_plan_task_completions (
    user_id    BIGINT NOT NULL REFERENCES study_plans(user_id) ON DELETE CASCADE,
    study_date DATE NOT NULL,
    slot_id    TEXT NOT NULL,
    task_id    TEXT NOT NULL,
    PRIMARY KEY (user_id, study_date, slot_id, task_id)
);
