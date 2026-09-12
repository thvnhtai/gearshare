-- name: CreateUser :execlastid
INSERT INTO users (email, password_hash, password_algo, display_name, role)
VALUES (?, ?, ?, ?, ?);

-- name: GetUserByEmail :one
SELECT id, email, password_hash, password_algo, display_name, role, created_at, updated_at
FROM users
WHERE email = ?;

-- name: GetUserByID :one
SELECT id, email, password_hash, password_algo, display_name, role, created_at, updated_at
FROM users
WHERE id = ?;
