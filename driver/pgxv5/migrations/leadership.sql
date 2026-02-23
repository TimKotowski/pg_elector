CREATE TABLE IF NOT EXISTS pg_leader (
    name TEXT NOT NULL DEFAULT 'pg_leader',
    namespace TEXT NOT NULL DEFAULT 'default',
    node_id TEXT NOT NULL,
    term INT NOT NULL,
    heartbeat TIMESTAMPTZ NOT NULL,

    PRIMARY KEY (name),
    CONSTRAINT node_id_unique_idx UNIQUE node_id
);

CREATE INDEX heartbeat_check_idx on pg_leader(name, heartbeat)