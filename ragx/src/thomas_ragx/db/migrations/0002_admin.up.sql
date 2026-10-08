CREATE TABLE admin_users (
    id            uuid        PRIMARY KEY,
    email         text        NOT NULL,
    password_hash text        NOT NULL,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uq_admin_users_email UNIQUE (email)
);

CREATE TABLE ui_sessions (
    token_hash    bytea       PRIMARY KEY,
    admin_user_id uuid        NOT NULL REFERENCES admin_users (id) ON DELETE CASCADE,
    csrf_token    text        NOT NULL,
    expires_at    timestamptz NOT NULL,
    created_at    timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_ui_sessions_expires_at ON ui_sessions (expires_at);
