DROP INDEX IF EXISTS insights_job_id_key;
ALTER TABLE insights DROP COLUMN IF EXISTS job_id;
ALTER TABLE scheduled_jobs DROP COLUMN IF EXISTS result;
