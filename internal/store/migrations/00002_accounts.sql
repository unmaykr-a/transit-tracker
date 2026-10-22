-- +goose Up
CREATE TABLE users (
    id             INTEGER PRIMARY KEY,
    username       TEXT NOT NULL UNIQUE COLLATE NOCASE,
    email          TEXT NOT NULL DEFAULT '',
    password_hash  TEXT NOT NULL,
    role           TEXT NOT NULL CHECK (role IN ('admin', 'user')),
    locale         TEXT NOT NULL DEFAULT '',
    theme          TEXT NOT NULL DEFAULT '',
    disabled       INTEGER NOT NULL DEFAULT 0,
    created_at     INTEGER NOT NULL  -- unix seconds
);

CREATE TABLE sessions (
    id            TEXT PRIMARY KEY,   -- sha256 of the cookie token, hex
    user_id       INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    csrf          TEXT NOT NULL,
    created_at    INTEGER NOT NULL,
    last_seen_at  INTEGER NOT NULL,
    expires_at    INTEGER NOT NULL,
    user_agent    TEXT NOT NULL DEFAULT '',
    ip            TEXT NOT NULL DEFAULT ''
);
CREATE INDEX sessions_user_id ON sessions (user_id);

CREATE TABLE invites (
    id          INTEGER PRIMARY KEY,
    token_hash  TEXT NOT NULL UNIQUE,
    role        TEXT NOT NULL CHECK (role IN ('admin', 'user')),
    created_by  INTEGER REFERENCES users (id) ON DELETE SET NULL,
    created_at  INTEGER NOT NULL,
    expires_at  INTEGER NOT NULL,
    used_by     INTEGER REFERENCES users (id) ON DELETE SET NULL,
    used_at     INTEGER
);

CREATE TABLE settings (
    key    TEXT PRIMARY KEY,
    value  TEXT NOT NULL   -- JSON
);

CREATE TABLE audit_log (
    id        INTEGER PRIMARY KEY,
    at        INTEGER NOT NULL,
    actor_id  INTEGER REFERENCES users (id) ON DELETE SET NULL,
    actor     TEXT NOT NULL DEFAULT '',  -- username at the time, kept after the user is deleted
    action    TEXT NOT NULL,
    target    TEXT NOT NULL DEFAULT '',
    detail    TEXT NOT NULL DEFAULT ''
);
CREATE INDEX audit_log_at ON audit_log (at);

-- +goose Down
DROP TABLE audit_log;
DROP TABLE settings;
DROP TABLE invites;
DROP TABLE sessions;
DROP TABLE users;
