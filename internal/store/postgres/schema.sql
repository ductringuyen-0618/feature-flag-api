CREATE TABLE IF NOT EXISTS flags (
    name TEXT PRIMARY KEY,
    description TEXT NOT NULL DEFAULT '',
    enabled BOOLEAN NOT NULL DEFAULT false,
    rollout_percent INT NOT NULL DEFAULT 100 CHECK (rollout_percent >= 0 AND rollout_percent <= 100),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS user_overrides (
    flag_name TEXT NOT NULL REFERENCES flags(name) ON DELETE CASCADE,
    user_id TEXT NOT NULL,
    enabled BOOLEAN NOT NULL,
    PRIMARY KEY (flag_name, user_id)
);

CREATE INDEX IF NOT EXISTS idx_user_overrides_user ON user_overrides (user_id);
