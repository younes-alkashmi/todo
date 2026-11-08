-- 0001_init.sql
-- M1 schema: local-only. owner_uid = 'local' until M2 introduces Firebase.

PRAGMA foreign_keys = ON;

-- CREATE TABLE schema_migrations (
--     version    INTEGER PRIMARY KEY,
--     applied_at TEXT NOT NULL
-- );

CREATE TABLE projects (
    id          TEXT PRIMARY KEY,
    owner_uid   TEXT NOT NULL,
    name        TEXT NOT NULL,
    version     INTEGER NOT NULL DEFAULT 1,
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL,
    deleted_at  TEXT
);
CREATE INDEX idx_projects_owner ON projects(owner_uid) WHERE deleted_at IS NULL;

CREATE TABLE tasks (
    id           TEXT PRIMARY KEY,
    owner_uid    TEXT NOT NULL,
    project_id   TEXT REFERENCES projects(id) ON DELETE CASCADE,
    parent_id    TEXT REFERENCES tasks(id)    ON DELETE CASCADE,
    title        TEXT NOT NULL,
    description  TEXT NOT NULL DEFAULT '',
    due_date     TEXT,                          -- YYYY-MM-DD, no time, no tz
    tags_json    TEXT NOT NULL DEFAULT '[]',    -- JSON array of strings
    completed    INTEGER NOT NULL DEFAULT 0,    -- 0/1
    version      INTEGER NOT NULL DEFAULT 1,
    created_at   TEXT NOT NULL,
    updated_at   TEXT NOT NULL,
    deleted_at   TEXT
);
CREATE INDEX idx_tasks_owner   ON tasks(owner_uid) WHERE deleted_at IS NULL;
CREATE INDEX idx_tasks_project ON tasks(project_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_tasks_parent  ON tasks(parent_id)  WHERE deleted_at IS NULL;

-- Durable outbox: one row per local mutation, committed in the same
-- transaction as the entity write. US-01 requires this.
CREATE TABLE outbox (
    mutation_id  TEXT PRIMARY KEY,
    account      TEXT NOT NULL,          -- 'local' in M1, uid in M2
    entity_type  TEXT NOT NULL,          -- 'task' | 'project'
    entity_id    TEXT NOT NULL,
    base_version INTEGER NOT NULL,       -- version before the mutation
    operation    TEXT NOT NULL,          -- 'create'|'update'|'delete'
    payload_json TEXT NOT NULL,
    state        TEXT NOT NULL DEFAULT 'pending',  -- pending|sent|failed
    created_at   TEXT NOT NULL
);
CREATE INDEX idx_outbox_state ON outbox(state, created_at);

-- Enforce one-level hierarchy at the DB level as a belt-and-suspenders.
-- Service layer also checks; this catches bugs in service code.
CREATE TRIGGER tasks_no_second_level
BEFORE INSERT ON tasks
WHEN NEW.parent_id IS NOT NULL
BEGIN
    SELECT RAISE(ABORT, 'parent must be a root task')
    WHERE EXISTS (
        SELECT 1 FROM tasks p
        WHERE p.id = NEW.parent_id AND p.parent_id IS NOT NULL
    );
END;

CREATE TRIGGER tasks_no_self_parent
BEFORE UPDATE OF parent_id ON tasks
WHEN NEW.parent_id = NEW.id
BEGIN
    SELECT RAISE(ABORT, 'a task cannot be its own parent');
END;