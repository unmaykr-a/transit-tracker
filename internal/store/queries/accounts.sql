-- name: CountUsers :one
SELECT count(*) FROM users;

-- name: CountActiveAdmins :one
SELECT count(*) FROM users WHERE role = 'admin' AND disabled = 0;

-- name: CreateUser :one
INSERT INTO users (username, email, password_hash, role, created_at)
VALUES (?, ?, ?, ?, ?)
RETURNING *;

-- name: GetUser :one
SELECT * FROM users WHERE id = ?;

-- name: GetUserByUsername :one
SELECT * FROM users WHERE username = ?;

-- name: ListUsers :many
SELECT * FROM users ORDER BY username;

-- name: UpdateUserProfile :exec
UPDATE users SET email = ?, locale = ?, theme = ? WHERE id = ?;

-- name: UpdateUserPassword :exec
UPDATE users SET password_hash = ? WHERE id = ?;

-- name: UpdateUserRole :exec
UPDATE users SET role = ? WHERE id = ?;

-- name: UpdateUserDisabled :exec
UPDATE users SET disabled = ? WHERE id = ?;

-- name: DeleteUser :exec
DELETE FROM users WHERE id = ?;

-- name: CreateSession :exec
INSERT INTO sessions (id, user_id, csrf, created_at, last_seen_at, expires_at, user_agent, ip)
VALUES (?, ?, ?, ?, ?, ?, ?, ?);

-- name: GetSessionWithUser :one
SELECT sqlc.embed(sessions), sqlc.embed(users)
FROM sessions JOIN users ON users.id = sessions.user_id
WHERE sessions.id = ?;

-- name: TouchSession :exec
UPDATE sessions SET last_seen_at = ?, expires_at = ?, ip = ? WHERE id = ?;

-- name: ListUserSessions :many
SELECT * FROM sessions WHERE user_id = ? AND expires_at > ? ORDER BY last_seen_at DESC;

-- name: DeleteSession :exec
DELETE FROM sessions WHERE id = ?;

-- name: DeleteUserSession :execrows
DELETE FROM sessions WHERE id = ? AND user_id = ?;

-- name: DeleteUserSessions :exec
DELETE FROM sessions WHERE user_id = ?;

-- name: DeleteOtherUserSessions :exec
DELETE FROM sessions WHERE user_id = ? AND id != ?;

-- name: DeleteExpiredSessions :exec
DELETE FROM sessions WHERE expires_at <= ?;

-- name: CreateInvite :one
INSERT INTO invites (token_hash, role, created_by, created_at, expires_at)
VALUES (?, ?, ?, ?, ?)
RETURNING *;

-- name: GetInviteByHash :one
SELECT * FROM invites WHERE token_hash = ?;

-- name: ListInvites :many
SELECT invites.*, coalesce(creator.username, '') AS created_by_name, coalesce(used.username, '') AS used_by_name
FROM invites
LEFT JOIN users AS creator ON creator.id = invites.created_by
LEFT JOIN users AS used ON used.id = invites.used_by
ORDER BY invites.created_at DESC;

-- name: UseInvite :execrows
UPDATE invites SET used_by = ?, used_at = ? WHERE id = ? AND used_at IS NULL AND expires_at > ?;

-- name: DeleteInvite :execrows
DELETE FROM invites WHERE id = ?;

-- name: ListSettings :many
SELECT * FROM settings;

-- name: UpsertSetting :exec
INSERT INTO settings (key, value) VALUES (?, ?)
ON CONFLICT (key) DO UPDATE SET value = excluded.value;

-- name: DeleteSetting :exec
DELETE FROM settings WHERE key = ?;

-- name: InsertAudit :exec
INSERT INTO audit_log (at, actor_id, actor, action, target, detail) VALUES (?, ?, ?, ?, ?, ?);

-- name: ListAudit :many
SELECT * FROM audit_log
WHERE (CAST(sqlc.arg(action) AS TEXT) = '' OR action = sqlc.arg(action))
  AND (CAST(sqlc.arg(before_id) AS INTEGER) = 0 OR id < sqlc.arg(before_id))
ORDER BY id DESC
LIMIT sqlc.arg(lim);
