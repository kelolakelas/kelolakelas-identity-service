CREATE TABLE IF NOT EXISTS creator_requests (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    requester_user_id uuid NOT NULL REFERENCES users(id),
    target_email varchar(255) NOT NULL,
    target_user_id uuid REFERENCES users(id),
    reason text NOT NULL,
    status varchar(20) NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'approved', 'rejected')),
    created_at timestamp NOT NULL DEFAULT now(),
    updated_at timestamp NOT NULL DEFAULT now(),
    CONSTRAINT creator_requests_target_email_normalized CHECK (target_email = lower(trim(target_email))),
    CONSTRAINT creator_requests_reason_nonempty CHECK (length(trim(reason)) > 0)
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_creator_requests_pending_target
    ON creator_requests (tenant_id, target_email) WHERE status = 'pending';
CREATE INDEX IF NOT EXISTS idx_creator_requests_tenant_created
    ON creator_requests (tenant_id, created_at DESC);
