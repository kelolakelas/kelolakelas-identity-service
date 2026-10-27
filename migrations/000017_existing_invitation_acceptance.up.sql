ALTER TABLE tenant_invitations ADD COLUMN accepted_member_id uuid REFERENCES tenant_members(id);
