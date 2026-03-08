ALTER TABLE credential_profiles
    ADD COLUMN encrypted_iv TEXT,
    ADD COLUMN encrypted_tag TEXT;
