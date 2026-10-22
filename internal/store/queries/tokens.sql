-- name: CreateAPIToken :one
INSERT INTO api_tokens (user_id, name, token_hash, prefix, created_at)
VALUES (?, ?, ?, ?, ?)
RETURNING *;

-- name: GetAPITokenWithUser :one
SELECT sqlc.embed(api_tokens), sqlc.embed(users)
FROM api_tokens JOIN users ON users.id = api_tokens.user_id
WHERE api_tokens.token_hash = ?;

-- name: TouchAPIToken :exec
UPDATE api_tokens SET last_used_at = ?, last_used_ip = ? WHERE id = ?;

-- name: ListUserAPITokens :many
SELECT * FROM api_tokens WHERE user_id = ? ORDER BY created_at DESC, id DESC;

-- name: ListAPITokens :many
SELECT api_tokens.*, users.username
FROM api_tokens JOIN users ON users.id = api_tokens.user_id
ORDER BY api_tokens.created_at DESC, api_tokens.id DESC;

-- name: GetAPIToken :one
SELECT * FROM api_tokens WHERE id = ?;

-- name: DeleteUserAPIToken :execrows
DELETE FROM api_tokens WHERE id = ? AND user_id = ?;

-- name: DeleteAPIToken :execrows
DELETE FROM api_tokens WHERE id = ?;

-- name: CountUserAPITokens :one
SELECT count(*) FROM api_tokens WHERE user_id = ?;

-- name: DeleteAllUserAPITokens :execrows
DELETE FROM api_tokens WHERE user_id = ?;
