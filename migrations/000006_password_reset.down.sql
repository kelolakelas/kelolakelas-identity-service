DROP TABLE IF EXISTS password_reset_tokens;
ALTER TABLE users DROP COLUMN IF EXISTS session_valid_after;
