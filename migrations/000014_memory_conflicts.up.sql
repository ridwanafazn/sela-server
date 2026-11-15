-- memory_conflicts records contradictions between memories without merging
-- either record. The user reviews both memories independently.
CREATE TABLE memory_conflicts (
    id uuid PRIMARY KEY,
    memory_id uuid NOT NULL,
    conflicting_memory_id uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT memory_conflicts_memory_id_fkey FOREIGN KEY (memory_id) REFERENCES personal_memories (id) ON DELETE CASCADE,
    CONSTRAINT memory_conflicts_conflicting_memory_id_fkey FOREIGN KEY (conflicting_memory_id) REFERENCES personal_memories (id) ON DELETE CASCADE,
    CONSTRAINT memory_conflicts_distinct_check CHECK (memory_id <> conflicting_memory_id),
    CONSTRAINT memory_conflicts_pair_key UNIQUE (memory_id, conflicting_memory_id)
);

CREATE INDEX memory_conflicts_conflicting_memory_id_idx
    ON memory_conflicts (conflicting_memory_id);
