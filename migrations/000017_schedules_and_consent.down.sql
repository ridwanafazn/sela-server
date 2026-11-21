DROP INDEX personal_memories_user_id_sensitivity_idx;
ALTER TABLE personal_memories DROP CONSTRAINT personal_memories_sensitivity_check;
ALTER TABLE personal_memories DROP COLUMN sensitivity;
DROP TABLE memory_consents;
DROP TABLE insight_schedules;
