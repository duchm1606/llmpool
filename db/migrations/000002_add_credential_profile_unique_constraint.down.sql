ALTER TABLE credential_profiles
DROP CONSTRAINT IF EXISTS uq_credential_profiles_type_account_id;

CREATE UNIQUE INDEX IF NOT EXISTS idx_credential_profiles_type_account_id ON credential_profiles(type, account_id);
