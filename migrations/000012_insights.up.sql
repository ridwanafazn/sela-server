-- insights, evidence, and feedback. Insight content is immutable; feedback is
-- append-only.
CREATE TABLE insights (
    id uuid PRIMARY KEY,
    user_id uuid NOT NULL,
    title varchar(200) NOT NULL,
    summary text NOT NULL,
    observation text NOT NULL,
    interpretation text NOT NULL,
    hypothesis text,
    confidence decimal(5,4) NOT NULL,
    confidence_label varchar(20) NOT NULL,
    reflection_question text NOT NULL,
    suggested_direction varchar(20) NOT NULL,
    period_start date,
    period_end date,
    generation_type varchar(30) NOT NULL,
    model varchar(100),
    prompt_version varchar(50),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz,

    CONSTRAINT insights_user_id_fkey FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE,
    CONSTRAINT insights_confidence_check CHECK (confidence >= 0 AND confidence <= 1),
    CONSTRAINT insights_confidence_label_check CHECK (confidence_label IN ('strong', 'moderate', 'limited')),
    CONSTRAINT insights_direction_check CHECK (suggested_direction IN ('improve', 'maintain', 'accept')),
    CONSTRAINT insights_generation_type_check CHECK (generation_type IN ('on_demand', 'weekly', 'monthly')),
    CONSTRAINT insights_period_check CHECK (
        period_start IS NULL OR period_end IS NULL OR period_start <= period_end
    )
);

CREATE INDEX insights_user_id_created_at_id_idx
    ON insights (user_id, created_at DESC, id DESC);
CREATE INDEX insights_user_id_period_start_idx
    ON insights (user_id, period_start);

CREATE TABLE insight_evidence (
    id uuid PRIMARY KEY,
    insight_id uuid NOT NULL,
    journal_id uuid,
    message_id uuid,
    memory_id uuid,
    note text NOT NULL,
    source_deleted_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT insight_evidence_insight_id_fkey FOREIGN KEY (insight_id) REFERENCES insights (id) ON DELETE CASCADE,
    CONSTRAINT insight_evidence_journal_id_fkey FOREIGN KEY (journal_id) REFERENCES journals (id) ON DELETE CASCADE,
    CONSTRAINT insight_evidence_message_id_fkey FOREIGN KEY (message_id) REFERENCES messages (id) ON DELETE CASCADE,
    CONSTRAINT insight_evidence_memory_id_fkey FOREIGN KEY (memory_id) REFERENCES personal_memories (id) ON DELETE CASCADE,
    CONSTRAINT insight_evidence_one_source_check CHECK (
        (journal_id IS NOT NULL)::int +
        (message_id IS NOT NULL)::int +
        (memory_id IS NOT NULL)::int = 1
    )
);

CREATE INDEX insight_evidence_insight_id_idx ON insight_evidence (insight_id);

CREATE TABLE insight_feedback (
    id uuid PRIMARY KEY,
    insight_id uuid NOT NULL,
    user_id uuid NOT NULL,
    action varchar(20) NOT NULL,
    note text,
    created_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT insight_feedback_insight_id_fkey FOREIGN KEY (insight_id) REFERENCES insights (id) ON DELETE CASCADE,
    CONSTRAINT insight_feedback_user_id_fkey FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE,
    CONSTRAINT insight_feedback_action_check CHECK (action IN ('confirm', 'correct', 'reject'))
);

CREATE INDEX insight_feedback_insight_id_created_at_idx
    ON insight_feedback (insight_id, created_at);
CREATE INDEX insight_feedback_user_id_created_at_idx
    ON insight_feedback (user_id, created_at);
