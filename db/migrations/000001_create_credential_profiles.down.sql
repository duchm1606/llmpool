CREATE TABLE IF NOT EXISTS credential_profiles (
    id TEXT PRIMARY KEY,
    provider TEXT NOT NULL,
    label TEXT NOT NULL,
    source_path TEXT NOT NULL DEFAULT '',
    email TEXT NOT NULL DEFAULT '',
    account_id TEXT NOT NULL DEFAULT '',
    has_refresh_token BOOLEAN NOT NULL DEFAULT FALSE,
    status TEXT NOT NULL,
    last_refresh_at TIMESTAMPTZ NULL,
    refresh_error TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    secret_access_token TEXT NOT NULL,
    secret_refresh_token TEXT NOT NULL DEFAULT '',
    secret_expires_at TIMESTAMPTZ NULL,
    secret_raw JSONB NOT NULL DEFAULT '{}'::jsonb
);
CREATE INDEX IF NOT EXISTS idx_credential_profiles_provider
  ON credential_profiles(provider);
CREATE INDEX IF NOT EXISTS idx_credential_profiles_status
  ON credential_profiles(status);