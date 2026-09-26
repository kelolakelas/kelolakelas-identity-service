ALTER TABLE users
    DROP COLUMN login_locked_until,
    DROP COLUMN failed_login_attempts;
