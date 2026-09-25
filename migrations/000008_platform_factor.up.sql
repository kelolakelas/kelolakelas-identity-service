ALTER TABLE platform_admin_assignments ADD COLUMN factor_secret bytea,
    ADD COLUMN enrollment_allowed boolean NOT NULL DEFAULT false,
    ADD COLUMN factor_version bigint NOT NULL DEFAULT 0;
-- Existing assignments must be explicitly recovered by an operator before enrollment.
CREATE TABLE platform_factor_challenges (
    token_hash bytea PRIMARY KEY,
    user_id uuid NOT NULL REFERENCES users(id),
    purpose text NOT NULL CHECK (purpose IN ('enroll', 'verify')),
    pending_secret bytea,
    factor_version bigint NOT NULL,
    expires_at timestamptz NOT NULL,
    attempts integer NOT NULL DEFAULT 0,
    consumed_at timestamptz
);
CREATE INDEX platform_factor_challenges_user_idx ON platform_factor_challenges(user_id);
