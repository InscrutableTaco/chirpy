-- name: CreateUser :one
INSERT INTO users (id, created_at, updated_at, email, hashed_password)
VALUES (
    gen_random_uuid(),
    NOW(),
    NOW(),
    $1,
    $2
)
RETURNING *;

-- name: GetUserByID :one
SELECT id, email, created_at, updated_at
FROM users
WHERE id = $1;

-- name: GetUserByEmail :one
SELECT id, created_at, updated_at, email, hashed_password
FROM users
WHERE email = $1;

-- name: DeleteUsers :exec
DELETE FROM users;

-- name: GetUserFromRefreshToken :one
SELECT users.*
FROM users
JOIN refresh_tokens ON users.id = refresh_tokens.user_id
WHERE refresh_tokens.token = $1
  AND refresh_tokens.expires_at > NOW()
  AND refresh_tokens.revoked_at IS NULL;

-- name: UpdateUser :exec
UPDATE users
SET email = $1, hashed_password = $2
WHERE id = $3;

