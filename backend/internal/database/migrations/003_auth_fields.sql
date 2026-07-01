ALTER TABLE users
    ADD COLUMN IF NOT EXISTS username TEXT;

CREATE UNIQUE INDEX IF NOT EXISTS users_username_unique_idx
    ON users (lower(username))
    WHERE username IS NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS users_email_lower_unique_idx
    ON users (lower(email));

ALTER TABLE sessions
    ADD COLUMN IF NOT EXISTS token_prefix TEXT,
    ADD COLUMN IF NOT EXISTS revoked_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS sessions_token_hash_idx ON sessions(token_hash);
CREATE INDEX IF NOT EXISTS sessions_user_active_idx ON sessions(user_id, expires_at) WHERE revoked_at IS NULL;
