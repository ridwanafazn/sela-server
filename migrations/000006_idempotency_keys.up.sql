-- idempotency_keys stores replayable responses for retry-sensitive writes.
-- job_id is included for insight generation but gains its foreign key when
-- scheduled_jobs is introduced in Phase 2.
CREATE TABLE idempotency_keys (
    id uuid PRIMARY KEY,
    user_id uuid NOT NULL,
    key varchar(255) NOT NULL,
    endpoint varchar(200) NOT NULL,
    request_hash text NOT NULL,
    response_status integer,
    response_body jsonb,
    job_id uuid,
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT idempotency_keys_user_id_fkey FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE,
    CONSTRAINT idempotency_keys_user_endpoint_key_key UNIQUE (user_id, endpoint, key)
);

CREATE INDEX idempotency_keys_expires_at_idx ON idempotency_keys (expires_at);
