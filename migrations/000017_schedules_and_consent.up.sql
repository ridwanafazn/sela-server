-- insight_schedules stores one user-configurable weekly or monthly reflection
-- schedule. The scheduler advances next_run_at after each fan-out.
CREATE TABLE insight_schedules (
    id uuid PRIMARY KEY,
    user_id uuid NOT NULL,
    cadence varchar(20) NOT NULL,
    timezone varchar(100) NOT NULL,
    enabled boolean NOT NULL DEFAULT true,
    next_run_at timestamptz NOT NULL,
    last_run_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT insight_schedules_user_id_key UNIQUE (user_id),
    CONSTRAINT insight_schedules_user_id_fkey FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE,
    CONSTRAINT insight_schedules_cadence_check CHECK (cadence IN ('weekly', 'monthly'))
);

CREATE INDEX insight_schedules_due_idx
    ON insight_schedules (next_run_at)
    WHERE enabled;

-- memory_consents records explicit, revocable consent for sensitive memory
-- extraction and retrieval. Sensitive content is skipped without consent.
CREATE TABLE memory_consents (
    id uuid PRIMARY KEY,
    user_id uuid NOT NULL,
    scope varchar(50) NOT NULL,
    granted boolean NOT NULL DEFAULT false,
    granted_at timestamptz,
    revoked_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT memory_consents_user_scope_key UNIQUE (user_id, scope),
    CONSTRAINT memory_consents_user_id_fkey FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE,
    CONSTRAINT memory_consents_scope_check CHECK (scope IN ('sensitive_memory'))
);

ALTER TABLE personal_memories
    ADD COLUMN sensitivity varchar(20) NOT NULL DEFAULT 'normal';

ALTER TABLE personal_memories
    ADD CONSTRAINT personal_memories_sensitivity_check
    CHECK (sensitivity IN ('normal', 'sensitive'));

CREATE INDEX personal_memories_user_id_sensitivity_idx
    ON personal_memories (user_id, sensitivity);
