-- journal_embeddings stores derived vectors. Source text is not duplicated;
-- offsets and content_hash allow regeneration.
--
-- No HNSW or IVFFlat index is created yet. The index type and distance metric
-- are chosen after measuring the dataset, per the database design.
CREATE TABLE journal_embeddings (
    id uuid PRIMARY KEY,
    journal_id uuid NOT NULL,
    user_id uuid NOT NULL,
    chunk_index integer NOT NULL,
    start_offset integer NOT NULL,
    end_offset integer NOT NULL,
    content_hash text NOT NULL,
    embedding vector(768) NOT NULL,
    embedding_model varchar(100) NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT journal_embeddings_journal_id_fkey FOREIGN KEY (journal_id) REFERENCES journals (id) ON DELETE CASCADE,
    CONSTRAINT journal_embeddings_user_id_fkey FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE,
    CONSTRAINT journal_embeddings_chunk_index_check CHECK (chunk_index >= 0),
    CONSTRAINT journal_embeddings_offsets_check CHECK (start_offset >= 0 AND end_offset > start_offset),
    CONSTRAINT journal_embeddings_journal_chunk_key UNIQUE (journal_id, chunk_index)
);

CREATE INDEX journal_embeddings_user_id_idx ON journal_embeddings (user_id);
