DROP INDEX IF EXISTS journal_analyses_search_vector_idx;
ALTER TABLE journal_analyses DROP COLUMN search_vector;

DROP INDEX IF EXISTS insights_search_vector_idx;
ALTER TABLE insights DROP COLUMN search_vector;

DROP INDEX IF EXISTS personal_memories_search_vector_idx;
ALTER TABLE personal_memories DROP COLUMN search_vector;
