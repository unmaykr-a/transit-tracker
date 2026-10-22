-- name: DeleteAllStops :exec
DELETE FROM stops;

-- name: DeleteAllStopAreas :exec
DELETE FROM stop_areas;

-- name: DeleteAllPatterns :exec
DELETE FROM patterns;

-- name: DeleteAllRoutes :exec
DELETE FROM routes;

-- name: InsertStop :exec
INSERT INTO stops (id, code, siri_id, name, platform, street, lat, lon, modes, area_id, aliases, source)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: InsertStopArea :exec
INSERT INTO stop_areas (id, name, lat, lon, modes) VALUES (?, ?, ?, ?, ?);

-- name: InsertRoute :exec
INSERT INTO routes (id, mode, short_name, long_name, color, text_color, source, sort)
VALUES (?, ?, ?, ?, ?, ?, ?, ?);

-- name: InsertPattern :exec
INSERT INTO patterns (id, route_id, direction, headsign, stop_ids, shape, trips, sort)
VALUES (?, ?, ?, ?, ?, ?, ?, ?);

-- name: ListStops :many
SELECT * FROM stops ORDER BY id;

-- name: ListStopAreas :many
SELECT * FROM stop_areas ORDER BY id;

-- name: ListRoutes :many
SELECT * FROM routes ORDER BY sort, id;

-- name: ListPatterns :many
SELECT * FROM patterns ORDER BY route_id, sort, id;

-- name: SetMeta :exec
INSERT INTO static_meta (key, value) VALUES (?, ?)
ON CONFLICT (key) DO UPDATE SET value = excluded.value;

-- name: GetMeta :one
SELECT value FROM static_meta WHERE key = ?;
