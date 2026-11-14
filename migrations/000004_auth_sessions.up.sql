-- auth_sessions stores refresh-session metadata and rotation state. Only
-- hashes are stored; raw refresh tokens never reach the database.
CREATE TABLE auth_sessions (
    id uuid PRIMARY KEY,
    user_id uuid NOT NULL,
    token_family_id uuid NOT NULL,
    refresh_token_hash text NOT NULL,
    expires_at timestamptz NOT NULL,
    revoked_at timestamptz,
    replaced_by uuid,
    created_at timestamptz NOT NULL DEFAULT now(),
    last_used_at timestamptz,

    CONSTRAINT auth_sessions_refresh_token_hash_key UNIQUE (refresh_token_hash),
    CONSTRAINT auth_sessions_user_id_fkey FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE,
    CONSTRAINT auth_sessions_replaced_by_fkey FOREIGN KEY (replaced_by) REFERENCES auth_sessions (id) ON DELETE SET NULL
);

CREATE INDEX auth_sessions_user_id_expires_at_idx ON auth_sessions (user_id, expires_at);
CREATE INDEX auth_sessions_token_family_id_idx ON auth_sessions (token_family_id);
