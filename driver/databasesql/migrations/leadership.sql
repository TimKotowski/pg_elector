CREATE TABLE IF NOT EXISTS leaders (
    elected_at TIMESTAMPTZ NOT NULL,        -- 8 bytes
    expires_at TIMESTAMPTZ NOT NULL,        -- 8 bytes

    -- variance
    name TEXT NOT NULL DEFAULT 'default',
    leader_id TEXT NOT NULL,

    PRIMARY KEY (name),
    CONSTRAINT unique_node_id UNIQUE (node_id)
);