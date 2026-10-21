-- tribal-knowledge schema
-- Requires: PostgreSQL 16+ with pgvector extension

CREATE EXTENSION IF NOT EXISTS vector;

-- ============================================================
-- Knowledge graph: nodes and edges
-- ============================================================

CREATE TABLE tk_nodes (
    id          BIGSERIAL PRIMARY KEY,
    node_key    TEXT NOT NULL UNIQUE,       -- e.g. "service:checkout", "resource:prod:logstore:checkout-api"
    node_type   TEXT NOT NULL,              -- service, resource, team, ...
    name        TEXT NOT NULL DEFAULT '',
    attributes  JSONB NOT NULL DEFAULT '{}',
    status      TEXT NOT NULL DEFAULT 'active',  -- active, invalid
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_tk_nodes_type   ON tk_nodes (node_type) WHERE status = 'active';
CREATE INDEX idx_tk_nodes_attrs  ON tk_nodes USING gin (attributes);

CREATE TABLE tk_edges (
    id              BIGSERIAL PRIMARY KEY,
    source_node_id  BIGINT NOT NULL REFERENCES tk_nodes(id),
    target_node_id  BIGINT NOT NULL REFERENCES tk_nodes(id),
    edge_type       TEXT NOT NULL,           -- calls, has_resource, contains, ...
    attributes      JSONB NOT NULL DEFAULT '{}',
    status          TEXT NOT NULL DEFAULT 'active',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (source_node_id, target_node_id, edge_type)
);

CREATE INDEX idx_tk_edges_source ON tk_edges (source_node_id) WHERE status = 'active';
CREATE INDEX idx_tk_edges_target ON tk_edges (target_node_id) WHERE status = 'active';

-- ============================================================
-- Knowledge items (notes): experiences, documents, cases, instructions
-- ============================================================

CREATE TABLE tk_items (
    id          BIGSERIAL PRIMARY KEY,
    item_type   TEXT NOT NULL,               -- experience, document, case, instruction
    title       TEXT NOT NULL DEFAULT '',
    summary     TEXT NOT NULL DEFAULT '',
    content     TEXT NOT NULL DEFAULT '',
    tags        TEXT[] NOT NULL DEFAULT '{}',
    node_ids    BIGINT[] NOT NULL DEFAULT '{}',
    attributes  JSONB NOT NULL DEFAULT '{}', -- last_verified_at, recorded_at, valid_from, ...
    trust       REAL NOT NULL DEFAULT 1.0,
    status      TEXT NOT NULL DEFAULT 'active',  -- active, invalid, superseded
    load_when   TEXT NOT NULL DEFAULT 'relevant', -- always, relevant
    sort_order  INT NOT NULL DEFAULT 0,
    embedding   vector(1536),
    ts_content  tsvector,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_tk_items_type     ON tk_items (item_type) WHERE status = 'active';
CREATE INDEX idx_tk_items_tags     ON tk_items USING gin (tags);
CREATE INDEX idx_tk_items_nodes    ON tk_items USING gin (node_ids);
CREATE INDEX idx_tk_items_ts       ON tk_items USING gin (ts_content);
-- ivfflat index requires data; create after initial seed
-- CREATE INDEX idx_tk_items_embedding ON tk_items USING ivfflat (embedding vector_cosine_ops) WITH (lists = 50);
CREATE INDEX idx_tk_items_attrs    ON tk_items USING gin (attributes);

-- ============================================================
-- Item history for revert-by-session
-- ============================================================

CREATE TABLE tk_item_history (
    id          BIGSERIAL PRIMARY KEY,
    item_id     BIGINT NOT NULL REFERENCES tk_items(id),
    session_id  BIGINT NOT NULL,
    op          TEXT NOT NULL,           -- created, updated, invalidated
    before_data JSONB,
    after_data  JSONB NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_tk_item_history_session ON tk_item_history (session_id);
CREATE INDEX idx_tk_item_history_item    ON tk_item_history (item_id);

-- ============================================================
-- Sessions
-- ============================================================

CREATE TABLE tk_sessions (
    id              BIGSERIAL PRIMARY KEY,
    user_id         BIGINT NOT NULL DEFAULT 0,
    question        TEXT NOT NULL DEFAULT '',
    question_abstract TEXT NOT NULL DEFAULT '',
    client_name     TEXT NOT NULL DEFAULT '',
    bootstrap_version TEXT NOT NULL DEFAULT '',
    git_remote      TEXT NOT NULL DEFAULT '',
    parsed          JSONB NOT NULL DEFAULT '{}',
    conclusion      JSONB,
    status          TEXT NOT NULL DEFAULT 'open',  -- open, closed
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_tk_sessions_user   ON tk_sessions (user_id);
CREATE INDEX idx_tk_sessions_status ON tk_sessions (status);

-- ============================================================
-- Steps (within a session)
-- ============================================================

CREATE TABLE tk_steps (
    id              BIGSERIAL PRIMARY KEY,
    session_id      BIGINT NOT NULL REFERENCES tk_sessions(id),
    seq             INT NOT NULL,
    step_type       TEXT NOT NULL,           -- env_check, plan, query, note
    tool            TEXT NOT NULL DEFAULT '',
    command         TEXT NOT NULL DEFAULT '',
    resource_key    TEXT NOT NULL DEFAULT '',
    result_class    TEXT NOT NULL DEFAULT '',  -- ok, no_data, wrong_mapping, ...
    rows_count      INT NOT NULL DEFAULT 0,
    summary         TEXT NOT NULL DEFAULT '',
    ids_found       JSONB NOT NULL DEFAULT '{}',
    used_item_ids   BIGINT[] NOT NULL DEFAULT '{}',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (session_id, seq)
);

CREATE INDEX idx_tk_steps_session ON tk_steps (session_id);

-- ============================================================
-- Updates (write-back operations within a session)
-- ============================================================

CREATE TABLE tk_updates (
    id              BIGSERIAL PRIMARY KEY,
    session_id      BIGINT NOT NULL REFERENCES tk_sessions(id),
    update_id       TEXT NOT NULL DEFAULT '',   -- client idempotency key
    feedback        JSONB NOT NULL DEFAULT '[]',
    reflection      JSONB NOT NULL DEFAULT '{}',
    operations      JSONB NOT NULL DEFAULT '[]',
    results         JSONB NOT NULL DEFAULT '[]',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_tk_updates_session ON tk_updates (session_id);
CREATE UNIQUE INDEX idx_tk_updates_idempotent ON tk_updates (session_id, update_id) WHERE update_id != '';

-- ============================================================
-- Cases (saved from sessions)
-- ============================================================

CREATE TABLE tk_cases (
    id              BIGSERIAL PRIMARY KEY,
    session_id      BIGINT NOT NULL REFERENCES tk_sessions(id),
    item_id         BIGINT REFERENCES tk_items(id),
    title           TEXT NOT NULL DEFAULT '',
    summary         TEXT NOT NULL DEFAULT '',
    steps_snapshot  JSONB NOT NULL DEFAULT '[]',
    conclusion      JSONB NOT NULL DEFAULT '{}',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_tk_cases_session ON tk_cases (session_id);

-- ============================================================
-- Trigger to keep ts_content up to date
-- ============================================================

CREATE OR REPLACE FUNCTION tk_items_ts_trigger() RETURNS trigger AS $$
BEGIN
    NEW.ts_content :=
        setweight(to_tsvector('simple', coalesce(NEW.title, '')), 'A') ||
        setweight(to_tsvector('simple', coalesce(NEW.summary, '')), 'B') ||
        setweight(to_tsvector('simple', coalesce(array_to_string(NEW.tags, ' '), '')), 'B');
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER tk_items_ts_update
    BEFORE INSERT OR UPDATE ON tk_items
    FOR EACH ROW EXECUTE FUNCTION tk_items_ts_trigger();
