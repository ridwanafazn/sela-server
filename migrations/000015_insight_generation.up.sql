-- Insight generation jobs store their outcome in scheduled_jobs.result. The
-- insight row records the job that produced it so retries are idempotent.
ALTER TABLE scheduled_jobs ADD COLUMN result jsonb;
ALTER TABLE insights ADD COLUMN job_id uuid;

CREATE UNIQUE INDEX insights_job_id_key
    ON insights (job_id)
    WHERE job_id IS NOT NULL;
