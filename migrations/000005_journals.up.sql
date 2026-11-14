-- journals stores original journal entries and server-side drafts. AI output
-- is never written into this table.
CREATE TABLE journals (
    id uuid PRIMARY KEY,
    user_id uuid NOT NULL,
    title varchar(200),
    content text NOT NULL,
    entry_date date NOT NULL,
    status varchar(20) NOT NULL DEFAULT 'active',
    version integer NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz,
    search_vector tsvector GENERATED ALWAYS AS (
        to_tsvector('simple', coalesce(title, '') || ' ' || content)
    ) STORED,

    CONSTRAINT journals_user_id_fkey FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE,
    CONSTRAINT journals_status_check CHECK (status IN ('draft', 'active')),
    CONSTRAINT journals_version_check CHECK (version >= 1)
);

CREATE INDEX journals_user_id_entry_date_id_idx
    ON journals (user_id, entry_date DESC, id DESC);
CREATE INDEX journals_user_id_status_updated_at_idx
    ON journals (user_id, status, updated_at DESC);
CREATE INDEX journals_user_id_updated_at_id_idx
    ON journals (user_id, updated_at, id);
CREATE INDEX journals_search_vector_idx
    ON journals USING GIN (search_vector);
