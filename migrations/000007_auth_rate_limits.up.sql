-- auth_rate_limits stores hashed authentication attempts for DB-backed rate
-- limiting. Raw IP addresses and account identifiers are never stored.
CREATE TABLE auth_rate_limits (
    id uuid PRIMARY KEY,
    action varchar(50) NOT NULL,
    subject_hash text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX auth_rate_limits_lookup_idx
    ON auth_rate_limits (action, subject_hash, created_at DESC);
CREATE INDEX auth_rate_limits_created_at_idx
    ON auth_rate_limits (created_at);
