-- name: ListSavedLines :many
SELECT * FROM saved_lines WHERE user_id = ? ORDER BY sort, id;

-- name: GetSavedLine :one
SELECT * FROM saved_lines WHERE id = ? AND user_id = ?;

-- name: CountSavedLines :one
SELECT count(*) FROM saved_lines WHERE user_id = ?;

-- name: CreateSavedLine :one
INSERT INTO saved_lines (user_id, route_id, direction, headsign, board_stop_id, alight_stop_id, label, sort, notify_threshold_min, created_at, origin_place_id)
VALUES (sqlc.arg(user_id), sqlc.arg(route_id), sqlc.arg(direction), sqlc.arg(headsign), sqlc.arg(board_stop_id), sqlc.arg(alight_stop_id), sqlc.arg(label),
        (SELECT coalesce(max(s.sort), -1) + 1 FROM saved_lines AS s WHERE s.user_id = sqlc.arg(user_id)), sqlc.arg(notify_threshold_min), sqlc.arg(created_at),
        sqlc.narg(origin_place_id))
RETURNING *;

-- name: CreateSavedTrip :one
INSERT INTO saved_lines (user_id, kind, route_id, direction, board_stop_id, from_area_id, to_area_id, label, sort, notify_threshold_min, created_at, origin_place_id)
VALUES (sqlc.arg(user_id), 'trip', '', 0, '', sqlc.arg(from_area_id), sqlc.arg(to_area_id), sqlc.arg(label),
        (SELECT coalesce(max(s.sort), -1) + 1 FROM saved_lines AS s WHERE s.user_id = sqlc.arg(user_id)), 0, sqlc.arg(created_at),
        sqlc.narg(origin_place_id))
RETURNING *;

-- name: ListTripOrigins :many
SELECT DISTINCT from_area_id FROM saved_lines WHERE kind = 'trip';

-- name: UpdateSavedLine :exec
UPDATE saved_lines
SET label = ?, alight_stop_id = ?, notify_threshold_min = ?, from_area_id = ?, to_area_id = ?, origin_place_id = ?
WHERE id = ? AND user_id = ?;

-- name: SetSavedLineSort :exec
UPDATE saved_lines SET sort = ? WHERE id = ? AND user_id = ?;

-- name: DeleteSavedLine :execrows
DELETE FROM saved_lines WHERE id = ? AND user_id = ?;

-- name: ListBoardStops :many
SELECT DISTINCT board_stop_id FROM saved_lines WHERE kind = 'line';

-- name: ListSavedPlaces :many
SELECT * FROM saved_places WHERE user_id = ? ORDER BY sort, id;

-- name: GetSavedPlace :one
SELECT * FROM saved_places WHERE id = ? AND user_id = ?;

-- name: CreateSavedPlace :one
INSERT INTO saved_places (user_id, label, lat, lon, sort)
VALUES (sqlc.arg(user_id), sqlc.arg(label), sqlc.arg(lat), sqlc.arg(lon),
        (SELECT coalesce(max(p.sort), -1) + 1 FROM saved_places AS p WHERE p.user_id = sqlc.arg(user_id)))
RETURNING *;

-- name: UpdateSavedPlace :exec
UPDATE saved_places SET label = ?, lat = ?, lon = ? WHERE id = ? AND user_id = ?;

-- name: DeleteSavedPlace :execrows
DELETE FROM saved_places WHERE id = ? AND user_id = ?;

-- name: GetUserPrefs :one
SELECT * FROM user_prefs WHERE user_id = ?;

-- name: UpsertUserPrefs :exec
INSERT INTO user_prefs (user_id, walk_speed_kmh, buffer_min) VALUES (?, ?, ?)
ON CONFLICT (user_id) DO UPDATE SET walk_speed_kmh = excluded.walk_speed_kmh, buffer_min = excluded.buffer_min;
