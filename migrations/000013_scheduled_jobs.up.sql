-- scheduled_jobs stores durable background work. Workers claim eligible jobs
-- with an expiring lock.
CREATE TABLE scheduled_jobs (
    id uuid PRIMARY KEY,
    user_id uuid,
    job_type varchar(100) NOT NULL,
    payload jsonb,
    status varchar(30) NOT NULL DEFAULT 'pending',
    attempts integer NOT NULL DEFAULT 0,
    max_attempts integer NOT NULL DEFAULT 3,
    run_at timestamptz NOT NULL,
    locked_at timestamptz,
    locked_by varchar(200),
    last_error text,
    idempotency_key uuid,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT scheduled_jobs_user_id_fkey FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE,
    CONSTRAINT scheduled_jobs_status_check CHECK (status IN ('pending', 'processing', 'completed', 'retry', 'failed')),
    CONSTRAINT scheduled_jobs_attempts_check CHECK (attempts >= 0 AND attempts <= max_attempts),
    CONSTRAINT scheduled_jobs_max_attempts_check CHECK (max_attempts > 0)
);

CREATE INDEX scheduled_jobs_status_run_at_idx ON scheduled_jobs (status, run_at);
CREATE INDEX scheduled_jobs_user_id_job_type_idx ON scheduled_jobs (user_id, job_type);
CREATE UNIQUE INDEX scheduled_jobs_user_idempotency_key_idx
    ON scheduled_jobs (user_id, idempotency_key)
    WHERE idempotency_key IS NOT NULL;
CREATE INDEX scheduled_jobs_claimable_idx
    ON scheduled_jobs (run_at)
    WHERE status IN ('pending', 'retry');
