ALTER TABLE users ADD COLUMN IF NOT EXISTS session_valid_after timestamp with time zone;

CREATE TABLE IF NOT EXISTS password_reset_tokens (
    token_hash varchar(64) PRIMARY KEY,
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    expires_at timestamp with time zone NOT NULL,
    used_at timestamp with time zone,
    created_at timestamp with time zone NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_password_reset_tokens_user ON password_reset_tokens(user_id);
