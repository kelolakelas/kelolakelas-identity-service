ALTER TABLE creator_requests ADD COLUMN IF NOT EXISTS decided_by uuid REFERENCES users(id);
ALTER TABLE creator_requests ADD COLUMN IF NOT EXISTS decided_at timestamp;
ALTER TABLE creator_requests ADD COLUMN IF NOT EXISTS rejection_reason text;
ALTER TABLE creator_requests ADD COLUMN IF NOT EXISTS invitation_id uuid UNIQUE REFERENCES tenant_invitations(id);

CREATE TABLE IF NOT EXISTS creator_request_audit (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    request_id uuid NOT NULL UNIQUE REFERENCES creator_requests(id),
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    actor_user_id uuid NOT NULL REFERENCES users(id),
    decision varchar(20) NOT NULL CHECK (decision IN ('approved', 'rejected')),
    rejection_reason text,
    created_at timestamp NOT NULL DEFAULT now()
);
