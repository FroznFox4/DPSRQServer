CREATE TABLE game_logs (
    id BIGSERIAL PRIMARY KEY,
    user_id INTEGER NOT NULL REFERENCES users(id),
    client_id INTEGER NOT NULL,
    timestamp TIMESTAMPTZ NOT NULL,
    time TEXT,
    kind VARCHAR(20) NOT NULL,
    action VARCHAR(20),
    name TEXT,
    amount BIGINT,
    count INTEGER,
    exp BIGINT,
    raw_text TEXT,
    synced_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(user_id, client_id)
);

CREATE INDEX idx_gl_user_id ON game_logs(user_id);
CREATE INDEX idx_gl_kind ON game_logs(kind);
CREATE INDEX idx_gl_timestamp ON game_logs(timestamp);
CREATE INDEX idx_gl_user_kind ON game_logs(user_id, kind);
