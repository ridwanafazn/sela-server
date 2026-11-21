-- auth_tokens stores single-use email verification and password reset tokens.
-- Only SHA-256 hashes are stored; raw tokens are sent to the user and never
-- persisted.
CREATE TABLE auth_tokens (
    id uuid PRIMARY KEY,
    user_id uuid NOT NULL,
    purpose varchar(40) NOT NULL,
    token_hash text NOT NULL,
    expires_at timestamptz NOT NULL,
    used_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT auth_tokens_user_id_fkey FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE,
    CONSTRAINT auth_tokens_purpose_check CHECK (purpose IN ('email_verification', 'password_reset')),
    CONSTRAINT auth_tokens_token_hash_key UNIQUE (token_hash)
);

CREATE INDEX auth_tokens_user_id_purpose_idx ON auth_tokens (user_id, purpose);
CREATE INDEX auth_tokens_expires_at_idx ON auth_tokens (expires_at);

-- Pre-public profile gates and legal acceptance.
ALTER TABLE profiles
    ADD COLUMN adult_confirmed_at timestamptz,
    ADD COLUMN terms_accepted_at timestamptz,
    ADD COLUMN terms_version varchar(20),
    ADD COLUMN privacy_accepted_at timestamptz,
    ADD COLUMN privacy_version varchar(20);

-- Entitlements are provider-independent. Payment webhooks will update them
-- once a provider is selected.
CREATE TABLE user_entitlements (
    user_id uuid PRIMARY KEY,
    plan varchar(30) NOT NULL DEFAULT 'free',
    status varchar(30) NOT NULL DEFAULT 'active',
    current_period_end timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT user_entitlements_user_id_fkey FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE,
    CONSTRAINT user_entitlements_plan_check CHECK (plan IN ('free', 'premium')),
    CONSTRAINT user_entitlements_status_check CHECK (status IN ('active', 'past_due', 'canceled'))
);
