-- Generated search vectors keep advanced search deterministic and indexed
-- without storing a second copy of user text.
ALTER TABLE personal_memories
    ADD COLUMN search_vector tsvector GENERATED ALWAYS AS (
        to_tsvector('simple', content)
    ) STORED;

CREATE INDEX personal_memories_search_vector_idx
    ON personal_memories USING GIN (search_vector);

ALTER TABLE insights
    ADD COLUMN search_vector tsvector GENERATED ALWAYS AS (
        to_tsvector('simple',
            coalesce(title, '') || ' ' ||
            coalesce(summary, '') || ' ' ||
            coalesce(observation, '') || ' ' ||
            coalesce(interpretation, '') || ' ' ||
            coalesce(hypothesis, '') || ' ' ||
            coalesce(reflection_question, '')
        )
    ) STORED;

CREATE INDEX insights_search_vector_idx
    ON insights USING GIN (search_vector);

ALTER TABLE journal_analyses
    ADD COLUMN search_vector tsvector GENERATED ALWAYS AS (
        to_tsvector('simple',
            coalesce(summary, '') || ' ' ||
            coalesce(observation, '') || ' ' ||
            coalesce(interpretation, '') || ' ' ||
            coalesce(hypothesis, '') || ' ' ||
            coalesce(reflection_question, '')
        )
    ) STORED;

CREATE INDEX journal_analyses_search_vector_idx
    ON journal_analyses USING GIN (search_vector);
