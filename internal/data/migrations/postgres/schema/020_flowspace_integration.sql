ALTER TABLE study_plans ADD COLUMN plan_id TEXT NOT NULL DEFAULT ('plan_' || md5(random()::text || clock_timestamp()::text));

CREATE TABLE study_plan_catalog_versions (
    user_id BIGINT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    revision BIGINT NOT NULL CHECK (revision > 0)
);

CREATE TABLE study_plan_task_states (
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    plan_id TEXT NOT NULL,
    study_date DATE NOT NULL,
    slot_id TEXT NOT NULL,
    task_id TEXT NOT NULL,
    completed BOOLEAN NOT NULL,
    revision BIGINT NOT NULL CHECK (revision > 0),
    occurred_at TIMESTAMPTZ NOT NULL,
    reason TEXT NOT NULL,
    PRIMARY KEY(user_id, plan_id, study_date, slot_id, task_id)
);

CREATE TABLE study_plan_task_identities (
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    plan_id TEXT NOT NULL,
    slot_id TEXT NOT NULL,
    task_id TEXT NOT NULL,
    path TEXT NOT NULL,
    retired BOOLEAN NOT NULL DEFAULT FALSE,
    PRIMARY KEY(user_id,plan_id,slot_id,task_id)
);

CREATE TABLE flowspace_connections (
    user_id BIGINT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    connection_id TEXT NOT NULL,
    key_id TEXT NOT NULL,
    source_instance_id TEXT NOT NULL,
    workspace_id TEXT NOT NULL,
    workspace_name TEXT NOT NULL DEFAULT '',
    encrypted_secret TEXT NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE flowspace_outbox (
    sequence BIGSERIAL UNIQUE NOT NULL,
    event_id TEXT PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    connection_id TEXT NOT NULL,
    kind TEXT NOT NULL CHECK (kind IN ('catalog','event')),
    payload_json TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    attempts INTEGER NOT NULL DEFAULT 0,
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    lease_until TIMESTAMPTZ,
    lease_token TEXT NOT NULL DEFAULT '',
    last_error TEXT NOT NULL DEFAULT '',
    remote_status TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    delivered_at TIMESTAMPTZ
);
CREATE INDEX flowspace_outbox_pending_idx ON flowspace_outbox(next_attempt_at,sequence) WHERE status IN ('pending','sending');
CREATE INDEX flowspace_outbox_user_idx ON flowspace_outbox(user_id,sequence DESC);
