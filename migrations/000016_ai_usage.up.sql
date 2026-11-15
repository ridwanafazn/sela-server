-- ai_usage_events records token usage, latency, success, and estimated cost for
-- every AI operation. It supports per-user and global budget checks without
-- storing prompts or private content.
CREATE TABLE ai_usage_events (
    id uuid PRIMARY KEY,
    user_id uuid NOT NULL,
    operation varchar(60) NOT NULL,
    provider varchar(40) NOT NULL,
    model varchar(100) NOT NULL,
    input_tokens integer NOT NULL DEFAULT 0,
    output_tokens integer NOT NULL DEFAULT 0,
    estimated_cost_usd numeric(12,6) NOT NULL DEFAULT 0,
    latency_ms integer NOT NULL DEFAULT 0,
    success boolean NOT NULL,
    error_category varchar(60),
    reference_id uuid,
    created_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT ai_usage_events_user_id_fkey FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE,
    CONSTRAINT ai_usage_events_tokens_check CHECK (input_tokens >= 0 AND output_tokens >= 0),
    CONSTRAINT ai_usage_events_cost_check CHECK (estimated_cost_usd >= 0),
    CONSTRAINT ai_usage_events_latency_check CHECK (latency_ms >= 0)
);

CREATE INDEX ai_usage_events_user_id_created_at_idx
    ON ai_usage_events (user_id, created_at DESC);
CREATE INDEX ai_usage_events_operation_created_at_idx
    ON ai_usage_events (operation, created_at DESC);
CREATE INDEX ai_usage_events_reference_id_idx
    ON ai_usage_events (reference_id)
    WHERE reference_id IS NOT NULL;
