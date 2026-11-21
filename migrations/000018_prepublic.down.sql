DROP TABLE user_entitlements;
ALTER TABLE profiles
    DROP COLUMN privacy_version,
    DROP COLUMN privacy_accepted_at,
    DROP COLUMN terms_version,
    DROP COLUMN terms_accepted_at,
    DROP COLUMN adult_confirmed_at;
DROP TABLE auth_tokens;
