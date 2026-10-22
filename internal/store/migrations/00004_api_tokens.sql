-- +goose Up
-- Personal API tokens (Home Assistant and scripts). Only the sha256 of the
-- token is stored; prefix is the first characters, shown to tell tokens apart.
CREATE TABLE api_tokens (
    id            INTEGER PRIMARY KEY,
    user_id       INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    name          TEXT NOT NULL,
    token_hash    TEXT NOT NULL UNIQUE,
    prefix        TEXT NOT NULL,
    created_at    INTEGER NOT NULL,
    last_used_at  INTEGER,
    last_used_ip  TEXT NOT NULL DEFAULT ''
);
CREATE INDEX api_tokens_user_id ON api_tokens (user_id);

-- +goose Down
DROP TABLE api_tokens;
