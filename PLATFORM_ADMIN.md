# Platform-admin access (KEL-93)

Apply migration `000002_platform_admin` before enabling the platform routes. Platform assignments are independent of tenant roles and are never granted by public registration or the tenant Creator seed.

An operator first creates an ordinary user through the existing registration flow (or selects an existing user), verifies ownership through the deployment's normal account procedures, and grants the existing user UUID from a trusted shell with database access:

    go run ./cmd/platform-admin -user-id <existing-user-uuid>

The command requires the normal identity DB configuration, refuses unknown/deleted users, and uses an upsert: running it twice is safe. It never accepts a password and never emits a token. Do not run it in an untrusted environment or expose it as an HTTP endpoint. Restrict database credentials and record the grant in the operator's change log.

The user signs in at `POST /api/v1/platform/auth/login` with the same email/password JSON as ordinary login. This issues a tenantless JWT with `is_platform_admin`; ordinary login remains a tenant/parent login. Use that JWT on `GET /api/v1/platform/me`. Identity checks the current assignment and user record for every request; revoked assignments are denied even with an unexpired token. Gateway gates the namespace and rejects a platform-only JWT at tenant routes. A user with both tenant membership and a platform assignment can use ordinary login for tenant access and platform login for platform access separately.

For recovery after all assignments have been deactivated, have a database-authorized operator re-run the same command for a verified existing user. It reactivates the assignment transactionally. Never add a public grant/recovery route or add platform permissions to the Creator role. To revoke, set `is_active = false` for the target `user_id` in `platform_admin_assignments` using the controlled database change process. Before revoking the last active admin, designate and test a second operator or ensure the database recovery procedure is available. The application intentionally does not expose a revoke endpoint.
