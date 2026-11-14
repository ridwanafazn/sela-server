-- conversations and messages store Companion sessions and their turns.
CREATE TABLE conversations (
    id uuid PRIMARY KEY,
    user_id uuid NOT NULL,
    title varchar(200),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz,

    CONSTRAINT conversations_user_id_fkey FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
);

CREATE INDEX conversations_user_id_updated_at_id_idx
    ON conversations (user_id, updated_at DESC, id DESC);

CREATE TABLE messages (
    id uuid PRIMARY KEY,
    conversation_id uuid NOT NULL,
    user_id uuid NOT NULL,
    role varchar(20) NOT NULL,
    content text NOT NULL,
    idempotency_key uuid,
    metadata jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT messages_conversation_id_fkey FOREIGN KEY (conversation_id) REFERENCES conversations (id) ON DELETE CASCADE,
    CONSTRAINT messages_user_id_fkey FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE,
    CONSTRAINT messages_role_check CHECK (role IN ('user', 'assistant')),
    CONSTRAINT messages_conversation_idempotency_key_key UNIQUE (conversation_id, idempotency_key)
);

CREATE INDEX messages_conversation_id_created_at_id_idx
    ON messages (conversation_id, created_at, id);
CREATE INDEX messages_user_id_created_at_idx
    ON messages (user_id, created_at);
