-- users stores authentication identity and account lifecycle.
CREATE TABLE users (
    id uuid PRIMARY KEY,
    email citext NOT NULL,
    password_hash text NOT NULL,
    status varchar(30) NOT NULL DEFAULT 'active',
    email_verified_at timestamptz,
    deletion_requested_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz,

    CONSTRAINT users_email_key UNIQUE (email),
    CONSTRAINT users_status_check CHECK (status IN ('active', 'deletion_pending', 'deleted'))
);
