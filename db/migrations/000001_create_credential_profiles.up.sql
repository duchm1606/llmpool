CREATE TABLE IF NOT EXISTS credential_profiles (
    id TEXT PRIMARY KEY,
    type TEXT NOT NULL,
    account_id TEXT NOT NULL,
    enabled BOOLEAN NOT NULL,
    email TEXT NOT NULL,
    expired TIMESTAMPTZ NOT NULL,
    last_refresh_at TIMESTAMPTZ NOT NULL,
    encrypted_profile TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    modified_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_credential_profiles_type_account_id ON credential_profiles(type, account_id);
CREATE INDEX IF NOT EXISTS idx_credential_profiles_type ON credential_profiles(type);
CREATE INDEX IF NOT EXISTS idx_credential_profiles_enabled ON credential_profiles(enabled);
CREATE INDEX IF NOT EXISTS idx_credential_profiles_expired ON credential_profiles(expired);
CREATE INDEX IF NOT EXISTS idx_credential_profiles_modified_at ON credential_profiles(modified_at);
