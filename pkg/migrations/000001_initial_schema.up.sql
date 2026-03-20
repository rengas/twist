CREATE TABLE IF NOT EXISTS tasks (
    id            SERIAL PRIMARY KEY,
    title         TEXT    NOT NULL DEFAULT '',
    prompt        TEXT    NOT NULL DEFAULT '',
    spec          TEXT    NOT NULL DEFAULT '',
    branch        TEXT    NOT NULL DEFAULT '',
    pr_url        TEXT    NOT NULL DEFAULT '',
    status        TEXT    NOT NULL DEFAULT 'prompt',
    approved      BOOLEAN NOT NULL DEFAULT FALSE,
    session_id    TEXT    NOT NULL DEFAULT '',
    worktree_path TEXT    NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS design_versions (
    id         SERIAL PRIMARY KEY,
    version    INTEGER   NOT NULL,
    content    TEXT      NOT NULL DEFAULT '',
    task_id    INTEGER   NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    summary    TEXT      NOT NULL DEFAULT ''
);
