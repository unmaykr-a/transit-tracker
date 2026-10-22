-- name: GetIdentityUser :one
SELECT sqlc.embed(user_identities), sqlc.embed(users)
FROM user_identities JOIN users ON users.id = user_identities.user_id
WHERE user_identities.issuer = ? AND user_identities.subject = ?;

-- name: CreateIdentity :exec
INSERT INTO user_identities (issuer, subject, user_id, email, created_at, last_login)
VALUES (?, ?, ?, ?, ?, ?);

-- name: TouchIdentity :exec
UPDATE user_identities SET last_login = ?, email = ? WHERE issuer = ? AND subject = ?;

-- name: CountUserIdentities :one
SELECT count(*) FROM user_identities WHERE user_id = ?;

-- name: CreateUserNoPassword :one
INSERT INTO users (username, email, password_hash, role, created_at)
VALUES (?, ?, '', ?, ?)
RETURNING *;

-- name: ListUserIdentities :many
SELECT * FROM user_identities WHERE user_id = ? ORDER BY created_at;

-- name: DeleteUserIdentity :execrows
DELETE FROM user_identities WHERE issuer = ? AND subject = ? AND user_id = ?;
