WITH ranked AS (
	SELECT
		id,
		ROW_NUMBER() OVER (
			PARTITION BY type, account_id
			ORDER BY modified_at DESC, created_at DESC, id DESC
		) AS row_num
	FROM credential_profiles
)
DELETE FROM credential_profiles cp
USING ranked
WHERE cp.id = ranked.id
	AND ranked.row_num > 1;

DROP INDEX IF EXISTS idx_credential_profiles_type_account_id;

ALTER TABLE credential_profiles
ADD CONSTRAINT uq_credential_profiles_type_account_id UNIQUE (type, account_id);
