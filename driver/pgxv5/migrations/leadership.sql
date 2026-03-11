CREATE TABLE IF NOT EXISTS leaders (
    elected_at TIMESTAMPTZ NOT NULL,        -- 8 bytes
    expires_at TIMESTAMPTZ NOT NULL,        -- 8 bytes
    renewed_at TIMESTAMPTZ,                 -- 8 bytes
    term BIGINT NOT NULL,                   -- 8 bytes

    -- variance
    name TEXT NOT NULL DEFAULT 'default',
    leader_id TEXT NOT NULL,

    PRIMARY KEY (name),
);