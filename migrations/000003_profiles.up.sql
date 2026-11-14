-- profiles stores the MVP profile and communication preferences.
CREATE TABLE profiles (
    id uuid PRIMARY KEY,
    user_id uuid NOT NULL,
    display_name varchar(100),
    language varchar(10) NOT NULL DEFAULT 'id',
    timezone varchar(100) NOT NULL DEFAULT 'Asia/Jakarta',
    communication_style varchar(30) NOT NULL DEFAULT 'adaptive',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT profiles_user_id_key UNIQUE (user_id),
    CONSTRAINT profiles_user_id_fkey FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE,
    CONSTRAINT profiles_communication_style_check CHECK (
        communication_style IN ('adaptive', 'direct', 'gentle', 'balanced')
    )
);
