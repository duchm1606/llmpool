-- name: CreateCredentialProfile :one
INSERT INTO credential_profiles (
    id,
    type,
    account_id,
    enabled,
    email,
    expired,
    last_refresh_at,
    encrypted_profile,
    created_at,
    modified_at
)
VALUES (
    $1,
    $2,
    $3,
    $4,
    $5,
    $6,
    $7,
    $8,
    NOW(),
    NOW()
)
RETURNING
    id,
    type,
    account_id,
    enabled,
    email,
    expired,
    last_refresh_at,
    encrypted_profile,
    created_at,
    modified_at;

-- name: ListCredentialProfiles :many
SELECT
    id,
    type,
    account_id,
    enabled,
    email,
    expired,
    last_refresh_at,
    encrypted_profile,
    created_at,
    modified_at
FROM credential_profiles
ORDER BY modified_at DESC;

-- name: UpdateCredentialProfile :one
UPDATE credential_profiles
SET
    type = $2,
    account_id = $3,
    enabled = $4,
    email = $5,
    expired = $6,
    last_refresh_at = $7,
    encrypted_profile = $8,
    modified_at = NOW()
WHERE id = $1
RETURNING
    id,
    type,
    account_id,
    enabled,
    email,
    expired,
    last_refresh_at,
    encrypted_profile,
    created_at,
    modified_at;
