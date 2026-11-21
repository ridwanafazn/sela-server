-- Per-entry journal analyses are stored separately from the original journal.
-- They are immutable; users can request, inspect, and delete them.
CREATE TABLE journal_analyses (
    id uuid PRIMARY KEY,
    user_id uuid NOT NULL,
    journal_id uuid NOT NULL,
    journal_version integer NOT NULL,
    job_id uuid NOT NULL,
    summary text NOT NULL,
    observation text NOT NULL,
    interpretation text NOT NULL,
    hypothesis text,
    confidence decimal(5,4) NOT NULL,
    confidence_label varchar(20) NOT NULL,
    reflection_question text NOT NULL,
    suggested_direction varchar(20) NOT NULL,
    evidence_note text NOT NULL,
    model varchar(100),
    prompt_version varchar(50),
    created_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT journal_analyses_user_id_fkey FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE,
    CONSTRAINT journal_analyses_journal_id_fkey FOREIGN KEY (journal_id) REFERENCES journals (id) ON DELETE CASCADE,
    CONSTRAINT journal_analyses_journal_version_check CHECK (journal_version > 0),
    CONSTRAINT journal_analyses_confidence_check CHECK (confidence >= 0 AND confidence <= 1),
    CONSTRAINT journal_analyses_confidence_label_check CHECK (confidence_label IN ('strong', 'moderate', 'limited')),
    CONSTRAINT journal_analyses_direction_check CHECK (suggested_direction IN ('improve', 'maintain', 'accept')),
    CONSTRAINT journal_analyses_journal_version_key UNIQUE (journal_id, journal_version)
);

CREATE UNIQUE INDEX journal_analyses_job_id_key ON journal_analyses (job_id);
CREATE INDEX journal_analyses_user_journal_created_idx
    ON journal_analyses (user_id, journal_id, created_at DESC, id DESC);
