-- People who may sign in, and what each may do.
CREATE TABLE users (
    id             BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    username       TEXT NOT NULL UNIQUE,
    password_hash  TEXT NOT NULL,              -- bcrypt; the password itself is never stored
    role           TEXT NOT NULL CHECK (role IN ('receiver', 'payer', 'admin')),
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- One row per signed-in browser.
CREATE TABLE sessions (
    token_hash  BYTEA PRIMARY KEY,             -- SHA-256 of the cookie value, not the value itself
    user_id     BIGINT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    expires_at  TIMESTAMPTZ NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX sessions_expiry ON sessions (expires_at);
