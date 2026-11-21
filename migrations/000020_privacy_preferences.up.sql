-- Privacy preferences control how personal data is used by AI features.
-- Defaults preserve the pre-4B behavior.
CREATE TABLE privacy_preferences (
    user_id uuid PRIMARY KEY,
    ai_context_mode varchar(30) NOT NULL DEFAULT 'journals_and_memories',
    memory_extraction_enabled boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT privacy_preferences_user_id_fkey FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE,
    CONSTRAINT privacy_preferences_context_mode_check CHECK (ai_context_mode IN ('none', 'journals', 'journals_and_memories'))
);
