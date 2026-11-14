-- personal_memories and memory_sources store structured, provenance-aware
-- personal context extracted from journals and messages.
CREATE TABLE personal_memories (
    id uuid PRIMARY KEY,
    user_id uuid NOT NULL,
    content text NOT NULL,
    category varchar(100),
    status varchar(30) NOT NULL DEFAULT 'candidate',
    confidence decimal(5,4),
    source_count integer NOT NULL DEFAULT 0,
    supersedes_memory_id uuid,
    last_confirmed_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz,

    CONSTRAINT personal_memories_user_id_fkey FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE,
    CONSTRAINT personal_memories_supersedes_fkey FOREIGN KEY (supersedes_memory_id) REFERENCES personal_memories (id) ON DELETE SET NULL,
    CONSTRAINT personal_memories_status_check CHECK (status IN ('candidate', 'active', 'rejected', 'superseded', 'deleted')),
    CONSTRAINT personal_memories_confidence_check CHECK (confidence IS NULL OR (confidence >= 0 AND confidence <= 1)),
    CONSTRAINT personal_memories_source_count_check CHECK (source_count >= 0)
);

CREATE INDEX personal_memories_user_id_status_updated_at_idx
    ON personal_memories (user_id, status, updated_at DESC);
CREATE INDEX personal_memories_user_id_category_idx
    ON personal_memories (user_id, category);

CREATE TABLE memory_sources (
    id uuid PRIMARY KEY,
    memory_id uuid NOT NULL,
    journal_id uuid,
    message_id uuid,
    source_excerpt text,
    created_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT memory_sources_memory_id_fkey FOREIGN KEY (memory_id) REFERENCES personal_memories (id) ON DELETE CASCADE,
    CONSTRAINT memory_sources_journal_id_fkey FOREIGN KEY (journal_id) REFERENCES journals (id) ON DELETE CASCADE,
    CONSTRAINT memory_sources_message_id_fkey FOREIGN KEY (message_id) REFERENCES messages (id) ON DELETE CASCADE,
    CONSTRAINT memory_sources_one_source_check CHECK (
        (journal_id IS NOT NULL AND message_id IS NULL) OR
        (journal_id IS NULL AND message_id IS NOT NULL)
    )
);

CREATE INDEX memory_sources_memory_id_idx ON memory_sources (memory_id);
