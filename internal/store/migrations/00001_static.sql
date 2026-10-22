-- +goose Up
-- Static transit data, replaced wholesale by each import (one transaction, so readers
-- never see a half-imported set).
CREATE TABLE stops (
    id        TEXT PRIMARY KEY,           -- OTP/GTFS stop id; equals the Tallinn SiriID for city stops
    code      TEXT NOT NULL DEFAULT '',   -- national stop code, e.g. 12204-1
    siri_id   TEXT NOT NULL DEFAULT '',
    name      TEXT NOT NULL,
    platform  TEXT NOT NULL DEFAULT '',   -- derived letter within the stop area (A, B, …)
    street    TEXT NOT NULL DEFAULT '',
    lat       REAL NOT NULL,
    lon       REAL NOT NULL,
    modes     TEXT NOT NULL DEFAULT '',   -- comma-separated
    area_id   TEXT NOT NULL,
    aliases   TEXT NOT NULL DEFAULT '',   -- semicolon-separated alternative names (GTFS alias)
    source    TEXT NOT NULL
);
CREATE INDEX stops_area_id ON stops (area_id);
CREATE INDEX stops_code ON stops (code);

CREATE TABLE stop_areas (
    id     TEXT PRIMARY KEY,
    name   TEXT NOT NULL,
    lat    REAL NOT NULL,
    lon    REAL NOT NULL,
    modes  TEXT NOT NULL DEFAULT ''
);

CREATE TABLE routes (
    id          TEXT PRIMARY KEY,         -- "<mode>:<short_name>", e.g. tram:T4
    mode        TEXT NOT NULL,
    short_name  TEXT NOT NULL,
    long_name   TEXT NOT NULL DEFAULT '',
    color       TEXT NOT NULL DEFAULT '',
    text_color  TEXT NOT NULL DEFAULT '',
    source      TEXT NOT NULL,
    sort        INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE patterns (
    id         TEXT PRIMARY KEY,
    route_id   TEXT NOT NULL REFERENCES routes (id) ON DELETE CASCADE,
    direction  INTEGER NOT NULL,
    headsign   TEXT NOT NULL DEFAULT '',
    stop_ids   TEXT NOT NULL,              -- JSON array, in travel order
    shape      TEXT NOT NULL DEFAULT '',   -- encoded polyline (precision 5)
    trips      INTEGER NOT NULL DEFAULT 0, -- popularity, used to pick the main pattern per direction
    sort       INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX patterns_route_id ON patterns (route_id);

CREATE TABLE static_meta (
    key    TEXT PRIMARY KEY,
    value  TEXT NOT NULL
);

-- +goose Down
DROP TABLE static_meta;
DROP TABLE patterns;
DROP TABLE routes;
DROP TABLE stop_areas;
DROP TABLE stops;
