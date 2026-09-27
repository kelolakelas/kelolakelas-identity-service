-- KEL-59: active memberships are looked up by user (GetTenantMemberByUserID and
-- other tm.user_id filters). The only composite index on the table is the
-- uq_tenant_members_tenant_user constraint, which leads with tenant_id and so
-- cannot serve a user_id-only lookup.
--
-- Non-unique: one user may belong to several tenants.
-- Built without CONCURRENTLY, like earlier migrations: writes to `tenant_members`
-- wait while it is built, which is acceptable at the current table size, and a
-- failed build can never leave an INVALID index that IF NOT EXISTS would then skip.
CREATE INDEX IF NOT EXISTS idx_tenant_members_user_id
    ON tenant_members (user_id);
