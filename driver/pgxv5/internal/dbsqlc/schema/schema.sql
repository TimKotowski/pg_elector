CREATE TABLE leaders (
    elected_at TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    renewed_at TIMESTAMPTZ,

    name TEXT PRIMARY KEY,
    leader_id TEXT NOT NULL
);