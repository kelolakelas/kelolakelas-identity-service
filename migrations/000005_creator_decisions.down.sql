DROP TABLE IF EXISTS creator_request_audit;
DROP INDEX IF EXISTS uq_creator_request_invitation;
ALTER TABLE creator_requests DROP COLUMN IF EXISTS invitation_id;
ALTER TABLE creator_requests DROP COLUMN IF EXISTS rejection_reason;
ALTER TABLE creator_requests DROP COLUMN IF EXISTS decided_at;
ALTER TABLE creator_requests DROP COLUMN IF EXISTS decided_by;
