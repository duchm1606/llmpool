ALTER TABLE credential_profiles
    DROP COLUMN IF EXISTS encrypted_iv,
    DROP COLUMN IF EXISTS encrypted_tag;
