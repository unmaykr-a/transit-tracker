-- +goose Up
CREATE TABLE saved_lines (
    id                    INTEGER PRIMARY KEY,
    user_id               INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    route_id              TEXT NOT NULL,           -- "<mode>:<short_name>"
    direction             INTEGER NOT NULL,
    headsign              TEXT NOT NULL DEFAULT '',
    board_stop_id         TEXT NOT NULL,
    alight_stop_id        TEXT NOT NULL DEFAULT '',
    label                 TEXT NOT NULL DEFAULT '',
    sort                  INTEGER NOT NULL DEFAULT 0,
    notify_threshold_min  INTEGER NOT NULL DEFAULT 0,
    created_at            INTEGER NOT NULL
);
CREATE INDEX saved_lines_user_id ON saved_lines (user_id, sort);

CREATE TABLE saved_places (
    id       INTEGER PRIMARY KEY,
    user_id  INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    label    TEXT NOT NULL,
    lat      REAL NOT NULL,
    lon      REAL NOT NULL,
    sort     INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX saved_places_user_id ON saved_places (user_id, sort);

CREATE TABLE user_prefs (
    user_id         INTEGER PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    walk_speed_kmh  REAL NOT NULL DEFAULT 4.8,
    buffer_min      INTEGER NOT NULL DEFAULT 1
);

-- +goose Down
DROP TABLE user_prefs;
DROP TABLE saved_places;
DROP TABLE saved_lines;
